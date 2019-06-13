package ledger

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	"github.com/abourget/ledger/parse"
	"github.com/influxdata/flux"
	"github.com/influxdata/flux/execute"
	"github.com/influxdata/flux/memory"
	"github.com/influxdata/flux/plan"
	"github.com/influxdata/flux/semantic"
	"github.com/influxdata/flux/values"
	"github.com/pkg/errors"
)

const FromLedgerKind = "fromLedger"

type FromLedgerOpSpec struct {
	Raw  string `json:"raw"`
	File string `json:"file"`
}

func init() {
	fromLedgerSignature := semantic.FunctionPolySignature{
		Parameters: map[string]semantic.PolyType{
			"raw":  semantic.String,
			"file": semantic.String,
		},
		Required: nil,
		Return:   flux.TableObjectType,
	}
	flux.RegisterPackageValue("ledger", "from", flux.FunctionValue(FromLedgerKind, createFromLedgerOpSpec, fromLedgerSignature))
	flux.RegisterOpSpec(FromLedgerKind, newFromLedgerOp)
	plan.RegisterProcedureSpec(FromLedgerKind, newFromLedgerProcedure, FromLedgerKind)
	execute.RegisterSource(FromLedgerKind, createFromLedgerSource)
}

func createFromLedgerOpSpec(args flux.Arguments, a *flux.Administration) (flux.OperationSpec, error) {
	spec := new(FromLedgerOpSpec)

	if raw, ok, err := args.GetString("raw"); err != nil {
		return nil, err
	} else if ok {
		spec.Raw = raw
	}

	if file, ok, err := args.GetString("file"); err != nil {
		return nil, err
	} else if ok {
		spec.File = file
	}

	if spec.Raw == "" && spec.File == "" {
		return nil, errors.New("must provide raw text or filename")
	}

	if spec.Raw != "" && spec.File != "" {
		return nil, errors.New("must provide exactly one of the parameters raw or file")
	}

	if spec.File != "" {
		if _, err := os.Stat(spec.File); err != nil {
			return nil, errors.Wrap(err, "failed to stat ledger file: ")
		}
	}

	return spec, nil
}

func newFromLedgerOp() flux.OperationSpec {
	return new(FromLedgerOpSpec)
}

func (s *FromLedgerOpSpec) Kind() flux.OperationKind {
	return FromLedgerKind
}

type FromLedgerProcedureSpec struct {
	plan.DefaultCost
	Raw  string
	File string
}

func newFromLedgerProcedure(qs flux.OperationSpec, pa plan.Administration) (plan.ProcedureSpec, error) {
	spec, ok := qs.(*FromLedgerOpSpec)
	if !ok {
		return nil, fmt.Errorf("invalid spec type %T", qs)
	}

	return &FromLedgerProcedureSpec{
		Raw:  spec.Raw,
		File: spec.File,
	}, nil
}

func (s *FromLedgerProcedureSpec) Kind() plan.ProcedureKind {
	return FromLedgerKind
}

func (s *FromLedgerProcedureSpec) Copy() plan.ProcedureSpec {
	ns := new(FromLedgerProcedureSpec)
	ns.Raw = s.Raw
	ns.File = s.File
	return ns
}

func createFromLedgerSource(prSpec plan.ProcedureSpec, dsid execute.DatasetID, a execute.Administration) (execute.Source, error) {
	spec, ok := prSpec.(*FromLedgerProcedureSpec)
	if !ok {
		return nil, fmt.Errorf("invalid spec type %T", prSpec)
	}

	name := "raw"
	raw := spec.Raw
	// if spec.File non-empty then spec.Ledger is empty
	if spec.File != "" {
		ledgerBytes, err := ioutil.ReadFile(spec.File)
		if err != nil {
			return nil, errors.Wrap(err, "ledger.from() failed to read file")
		}
		name = spec.File
		raw = string(ledgerBytes)
	}

	return &LedgerSource{
		id:        dsid,
		name:      name,
		raw:       raw,
		allocator: a.Allocator(),
	}, nil
}

type LedgerSource struct {
	id        execute.DatasetID
	name      string
	raw       string
	ts        []execute.Transformation
	allocator *memory.Allocator
}

func (c *LedgerSource) AddTransformation(t execute.Transformation) {
	c.ts = append(c.ts, t)
}

func (c *LedgerSource) Run(ctx context.Context) {
	var err error
	var max execute.Time
	maxSet := false
	for _, t := range c.ts {
		// For each downstream transformation, instantiate a new result
		// decoder. This way a table instance goes to one and only one
		// transformation. Unlike other sources, tables from ledger sources
		// are not read-only. They contain mutable state and therefore
		// cannot be shared among goroutines.
		decoder := newResultDecoder(c.name, c.allocator)
		result, decodeErr := decoder.Decode(strings.NewReader(c.raw))
		if decodeErr != nil {
			err = decodeErr
			goto FINISH
		}
		err = result.Tables().Do(func(tbl flux.Table) error {
			err := t.Process(c.id, tbl)
			if err != nil {
				return err
			}
			if idx := execute.ColIdx(execute.DefaultStopColLabel, tbl.Key().Cols()); idx >= 0 {
				if stop := tbl.Key().ValueTime(idx); !maxSet || stop > max {
					max = stop
					maxSet = true
				}
			}
			return nil
		})
		if err != nil {
			goto FINISH
		}
	}

	if maxSet {
		for _, t := range c.ts {
			if err = t.UpdateWatermark(c.id, max); err != nil {
				goto FINISH
			}
		}
	}

FINISH:
	for _, t := range c.ts {
		err = errors.Wrap(err, "error in ledger.from()")
		t.Finish(c.id, err)
	}
}

type resultDecoder struct {
	name string
	tree *parse.Tree
	a    *memory.Allocator

	timeCol,
	commodityCol,
	txCol,
	clearedCol,
	pendingCol,
	lStart,
	lStop,
	valueCol int
}

func newResultDecoder(name string, a *memory.Allocator) *resultDecoder {
	return &resultDecoder{
		name: name,
		a:    a,
	}
}

func (d *resultDecoder) Decode(r io.Reader) (flux.Result, error) {
	raw, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	tree := parse.New(d.name, string(raw))
	if err := tree.Parse(); err != nil {
		return nil, err
	}
	d.tree = tree
	return d, nil
}

// implement flux.Result

func (d *resultDecoder) Name() string {
	return d.name
}
func (d *resultDecoder) Tables() flux.TableIterator {
	return d
}

// implement flux.TableIterator

func (d *resultDecoder) Do(f func(flux.Table) error) error {
	tbl, err := d.parseTable()
	if err != nil {
		return err
	}
	return f(tbl)
}

func (d *resultDecoder) parseTable() (flux.Table, error) {
	builder := execute.NewColListTableBuilder(
		execute.NewGroupKey(nil, nil),
		d.a,
	)
	maxDepth := 0
	for _, n := range d.tree.Root.Nodes {
		depth := nodeMaxDepth(n)
		if depth > maxDepth {
			maxDepth = depth
		}
	}
	err := d.buildCols(builder, maxDepth)
	if err != nil {
		return nil, err
	}
	for _, n := range d.tree.Root.Nodes {
		d.build(builder, n)
	}

	return builder.Table()
}

func nodeMaxDepth(n parse.Node) int {
	max := 0
	switch n := n.(type) {
	case *parse.XactNode:
		d := xactMaxDepth(n)
		if d > max {
			max = d
		}
	}
	return max
}
func xactMaxDepth(n *parse.XactNode) int {
	max := 0
	for _, p := range n.Postings {
		accs := accounts(p.Account)
		if l := len(accs); l > max {
			max = l
		}
	}
	return max
}

func (d *resultDecoder) buildCols(builder execute.TableBuilder, max int) error {
	var err error
	d.timeCol, err = builder.AddCol(flux.ColMeta{
		Label: execute.DefaultTimeColLabel,
		Type:  flux.TTime,
	})
	if err != nil {
		return err
	}
	d.commodityCol, err = builder.AddCol(flux.ColMeta{
		Label: "commodity",
		Type:  flux.TString,
	})
	if err != nil {
		return err
	}
	d.txCol, err = builder.AddCol(flux.ColMeta{
		Label: "tx",
		Type:  flux.TString,
	})
	if err != nil {
		return err
	}
	d.clearedCol, err = builder.AddCol(flux.ColMeta{
		Label: "cleared",
		Type:  flux.TBool,
	})
	if err != nil {
		return err
	}
	d.pendingCol, err = builder.AddCol(flux.ColMeta{
		Label: "pending",
		Type:  flux.TBool,
	})
	if err != nil {
		return err
	}
	for i := 0; i < max; i++ {
		j, err := builder.AddCol(flux.ColMeta{
			Label: "l" + strconv.Itoa(i),
			Type:  flux.TString,
		})
		if err != nil {
			return err
		}
		if i == 0 {
			d.lStart = j
		}
		d.lStop = j
	}
	d.valueCol, err = builder.AddCol(flux.ColMeta{
		Label: execute.DefaultValueColLabel,
		Type:  flux.TFloat,
	})
	if err != nil {
		return err
	}
	return nil
}

func (d *resultDecoder) build(builder execute.TableBuilder, n parse.Node) {
	switch n := n.(type) {
	case *parse.XactNode:
		d.buildXactNode(builder, n)
	}
}

func (d *resultDecoder) buildXactNode(builder execute.TableBuilder, n *parse.XactNode) {
	amountSum := 0.0
	defaultCom := ""
	for _, p := range n.Postings {
		if p.Amount != nil {
			amountSum += amount(p.Amount)
			defaultCom = p.Amount.Commodity
		}
	}
	for _, p := range n.Postings {
		builder.AppendTime(d.timeCol, values.ConvertTime(n.Date))
		builder.AppendString(d.txCol, n.Description)
		builder.AppendBool(d.clearedCol, n.IsCleared)
		builder.AppendBool(d.pendingCol, n.IsPending)
		accs := accounts(p.Account)
		for i, a := range accs {
			builder.AppendString(d.lStart+i, a)
		}
		for i := d.lStart + len(accs); i <= d.lStop; i++ {
			builder.AppendNil(i)
		}
		if p.Amount == nil {
			builder.AppendFloat(d.valueCol, -amountSum)
			builder.AppendString(d.commodityCol, defaultCom)
		} else {
			builder.AppendFloat(d.valueCol, amount(p.Amount))
			builder.AppendString(d.commodityCol, p.Amount.Commodity)
		}
	}
}

func amount(n *parse.AmountNode) float64 {
	f, _ := strconv.ParseFloat(n.Quantity, 64)
	if n.Negative {
		return -f
	}
	return f
}

func accounts(a string) []string {
	return strings.Split(a, ":")
}
