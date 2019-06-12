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
	tbls, err := d.parseTable()
	if err != nil {
		return err
	}
	for _, tbl := range tbls {
		err := f(tbl)
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *resultDecoder) parseTable() (tbls []flux.Table, err error) {
	// Use a file level max so that all tables have the same schema
	max := 0
	for _, n := range d.tree.Root.Nodes {
		xn, ok := n.(*parse.XactNode)
		if !ok {
			// Ignore non transaction nodes
			continue
		}
		depth := maxDepth(xn)
		if depth > max {
			max = depth
		}
	}

	// Build each table
	for _, n := range d.tree.Root.Nodes {
		xn, ok := n.(*parse.XactNode)
		if !ok {
			// Ignore non transaction nodes
			continue
		}
		tbl, err := d.buildTbl(xn, max)
		if err != nil {
			return nil, err
		}
		tbls = append(tbls, tbl)
	}
	return
}

func maxDepth(n *parse.XactNode) int {
	max := 0
	for _, p := range n.Postings {
		accs := accounts(p.Account)
		if l := len(accs); l > max {
			max = l
		}
	}
	return max
}

const (
	timeCol      = 0
	commodityCol = 1
	txCol        = 2
	clearedCol   = 3
	pendingCol   = 4
	lStart       = 5
)

func (d *resultDecoder) buildTbl(n *parse.XactNode, max int) (flux.Table, error) {
	// Determine cols
	var cols []flux.ColMeta
	cols = append(cols, flux.ColMeta{
		Label: execute.DefaultTimeColLabel,
		Type:  flux.TTime,
	})
	cols = append(cols, flux.ColMeta{
		Label: "commodity",
		Type:  flux.TString,
	})
	cols = append(cols, flux.ColMeta{
		Label: "tx",
		Type:  flux.TString,
	})
	cols = append(cols, flux.ColMeta{
		Label: "cleared",
		Type:  flux.TBool,
	})
	cols = append(cols, flux.ColMeta{
		Label: "pending",
		Type:  flux.TBool,
	})
	lStop := 0
	for i := 0; i < max; i++ {
		cols = append(cols, flux.ColMeta{
			Label: "l" + strconv.Itoa(i),
			Type:  flux.TString,
		})
		lStop = len(cols)
	}
	valueCol := len(cols)
	cols = append(cols, flux.ColMeta{
		Label: execute.DefaultValueColLabel,
		Type:  flux.TFloat,
	})
	// Determine key
	com := commodity(n)
	key := execute.NewGroupKey(cols[timeCol:timeCol+3], []values.Value{
		values.NewTime(values.ConvertTime(n.Date)),
		values.NewString(com),
		values.NewString(n.Description),
	})

	// Create builder
	builder := execute.NewColListTableBuilder(key, d.a)
	for _, c := range cols {
		builder.AddCol(c)
	}
	// build table
	buildXactNode(builder, n, com, lStop, valueCol)

	return builder.Table()
}

func buildXactNode(builder execute.TableBuilder, n *parse.XactNode, com string, lStop, valueCol int) {
	amountSum := 0.0
	for _, p := range n.Postings {
		if p.Amount != nil {
			amountSum += amount(p.Amount)
		}
	}
	for _, p := range n.Postings {
		builder.AppendTime(timeCol, values.ConvertTime(n.Date))
		builder.AppendString(commodityCol, com)
		builder.AppendString(txCol, n.Description)
		builder.AppendBool(clearedCol, n.IsCleared)
		builder.AppendBool(pendingCol, n.IsPending)
		accs := accounts(p.Account)
		for i, a := range accs {
			builder.AppendString(lStart+i, a)
		}
		for i := lStart + len(accs); i < lStop; i++ {
			builder.AppendNil(i)
		}
		if p.Amount == nil {
			builder.AppendFloat(valueCol, -amountSum)
		} else {
			builder.AppendFloat(valueCol, amount(p.Amount))
		}
	}
}

func commodity(n *parse.XactNode) (c string) {
	for _, p := range n.Postings {
		if p.Amount != nil && p.Amount.Commodity != "" {
			return p.Amount.Commodity
		}
	}
	return
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
