package ledger

import (
	"fmt"
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

	return execute.CreateSourceFromDecoder(
		&LedgerSource{
			name:      name,
			raw:       raw,
			allocator: a.Allocator(),
		},
		dsid,
		a,
	)
}

type LedgerSource struct {
	name      string
	raw       string
	allocator *memory.Allocator
	read      bool
}

// Connect to underlying data.
func (s *LedgerSource) Connect() error {
	return nil
}

// Fetch reports if there are more tables to consume.
// The ledger source only ever returns a single table.
func (s *LedgerSource) Fetch() (bool, error) {
	if s.read {
		return false, nil
	}
	s.read = true
	return true, nil
}

// Decode produces a flux.Table.
func (s *LedgerSource) Decode() (flux.Table, error) {
	tree := parse.New(s.name, string(s.raw))
	if err := tree.Parse(); err != nil {
		return nil, err
	}
	builder := execute.NewColListTableBuilder(
		execute.NewGroupKey(nil, nil),
		s.allocator,
	)
	max := maxDepth(tree)
	valueCol := buildCols(builder, max)
	for _, n := range tree.Root.Nodes {
		xn, ok := n.(*parse.XactNode)
		if !ok {
			continue // skip non transaction nodes
		}
		build(builder, xn, valueCol)
	}

	return builder.Table()
}

// Closes the ledger source.
func (s *LedgerSource) Close() error {
	return nil
}

// maxDepth returns the maximum account depth of tree.
func maxDepth(tree *parse.Tree) int {
	maxDepth := 0
	for _, n := range tree.Root.Nodes {
		xn, ok := n.(*parse.XactNode)
		if !ok {
			continue // skip non transaction nodes
		}
		depth := xactMaxDepth(xn)
		if depth > maxDepth {
			maxDepth = depth
		}
	}
	return maxDepth
}

// xactMaxDepth returns the maximum account depth of n.
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

const (
	// constant indexes of the ledger schema
	timeCol      = 0
	commodityCol = 1
	txCol        = 2
	clearedCol   = 3
	pendingCol   = 4
	lStart       = 5
)

// buildCols adds the columns to the builder
// based on the ledger schema and max accont depth.
func buildCols(builder execute.TableBuilder, max int) (valueCol int) {
	builder.AddCol(flux.ColMeta{
		Label: execute.DefaultTimeColLabel,
		Type:  flux.TTime,
	})
	builder.AddCol(flux.ColMeta{
		Label: "commodity",
		Type:  flux.TString,
	})
	builder.AddCol(flux.ColMeta{
		Label: "tx",
		Type:  flux.TString,
	})
	builder.AddCol(flux.ColMeta{
		Label: "cleared",
		Type:  flux.TBool,
	})
	builder.AddCol(flux.ColMeta{
		Label: "pending",
		Type:  flux.TBool,
	})
	for i := 0; i < max; i++ {
		builder.AddCol(flux.ColMeta{
			Label: "l" + strconv.Itoa(i),
			Type:  flux.TString,
		})
	}
	valueCol, _ = builder.AddCol(flux.ColMeta{
		Label: execute.DefaultValueColLabel,
		Type:  flux.TFloat,
	})
	return
}

// build appends rows to builder based on the postings in the n.
func build(builder execute.TableBuilder, n *parse.XactNode, valueCol int) error {
	for _, p := range n.Postings {
		builder.AppendTime(timeCol, values.ConvertTime(n.Date))
		builder.AppendString(txCol, n.Description)
		builder.AppendBool(clearedCol, n.IsCleared)
		builder.AppendBool(pendingCol, n.IsPending)
		accs := accounts(p.Account)
		for i, a := range accs {
			builder.AppendString(lStart+i, a)
		}
		for i := lStart + len(accs); i < valueCol; i++ {
			builder.AppendNil(i)
		}
		if p.Amount == nil {
			amountSum := 0.0
			commodity := ""
			for _, p := range n.Postings {
				if p.Amount != nil {
					if commodity == "" {
						commodity = p.Amount.Commodity
					}
					if commodity != p.Amount.Commodity {
						return errors.New("when multiple commodities are present all amounts must be specified explicitly")
					}
					amountSum += amount(p.Amount)
				}
			}
			builder.AppendFloat(valueCol, -amountSum)
			builder.AppendString(commodityCol, commodity)
		} else {
			builder.AppendFloat(valueCol, amount(p.Amount))
			builder.AppendString(commodityCol, p.Amount.Commodity)
		}
	}
	return nil
}

// amount returns the the value of a given amount node.
func amount(n *parse.AmountNode) float64 {
	f, _ := strconv.ParseFloat(n.Quantity, 64)
	if n.Negative {
		return -f
	}
	return f
}

// accounts parses an account string into its list of sub accounts
func accounts(a string) []string {
	return strings.Split(a, ":")
}
