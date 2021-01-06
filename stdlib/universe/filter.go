package universe

import (
	"context"
	"fmt"

	"github.com/apache/arrow/go/arrow/array"
	"github.com/apache/arrow/go/arrow/bitutil"
	arrowmem "github.com/apache/arrow/go/arrow/memory"
	"github.com/influxdata/flux"
	"github.com/influxdata/flux/arrow"
	"github.com/influxdata/flux/codes"
	"github.com/influxdata/flux/compiler"
	"github.com/influxdata/flux/execute"
	"github.com/influxdata/flux/internal/arrowutil"
	"github.com/influxdata/flux/internal/errors"
	"github.com/influxdata/flux/internal/execute/executekit"
	"github.com/influxdata/flux/internal/execute/function"
	"github.com/influxdata/flux/internal/execute/table"
	"github.com/influxdata/flux/memory"
	"github.com/influxdata/flux/runtime"
	"github.com/influxdata/flux/semantic"
	"github.com/influxdata/flux/values"
)

const FilterKind = "filter"

func init() {
	filterSignature := runtime.MustLookupBuiltinType("universe", "filter")
	function.RegisterTransformation("universe", FilterKind, &FilterProcedureSpec{}, filterSignature)
	// plan.RegisterPhysicalRules(
	// 	RemoveTrivialFilterRule{},
	// )
}

type OnEmptyMode int

const (
	DropEmptyTables OnEmptyMode = iota
	KeepEmptyTables
)

func (m *OnEmptyMode) ReadArg(name string, arg values.Value, a *flux.Administration) error {
	switch mode := arg.Str(); mode {
	case "keep":
		*m = KeepEmptyTables
		return nil
	case "drop", "":
		*m = DropEmptyTables
		return nil
	default:
		return errors.Newf(codes.Invalid, "onEmpty must be keep or drop, was %q", mode)
	}
}

type FilterProcedureSpec struct {
	Tables      *function.TableObject `flux:"tables,required"`
	Fn          function.Function     `flux:"fn,required"`
	OnEmptyMode OnEmptyMode           `flux:"onEmpty"`
}

func (s *FilterProcedureSpec) CreateTransformation(id execute.DatasetID, a execute.Administration) (execute.Transformation, execute.Dataset, error) {
	fn := execute.NewRowPredicateFn(s.Fn.Fn, compiler.ToScope(s.Fn.Scope))
	t, d := executekit.NewNarrowTransformation(id, &filterTransformation{
		fn:              fn,
		ctx:             a.Context(),
		keepEmptyTables: s.OnEmptyMode == KeepEmptyTables,
	}, a.Allocator())
	return t, d, nil
}

func (s *FilterProcedureSpec) PlanDetails() string {
	if expr, ok := s.Fn.Fn.GetFunctionBodyExpression(); ok {
		return fmt.Sprintf("%v", semantic.Formatted(expr))
	}
	return "<non-Expression>"
}

type filterTransformation struct {
	ctx             context.Context
	fn              *execute.RowPredicateFn
	keepEmptyTables bool
	alloc           *memory.Allocator
}

func NewFilterTransformation(ctx context.Context, spec *FilterProcedureSpec, id execute.DatasetID, alloc *memory.Allocator) (execute.Transformation, execute.Dataset, error) {
	fn := execute.NewRowPredicateFn(spec.Fn.Fn, compiler.ToScope(spec.Fn.Scope))
	t, d := executekit.NewNarrowTransformation(id, &filterTransformation{
		fn:              fn,
		ctx:             ctx,
		keepEmptyTables: spec.OnEmptyMode == KeepEmptyTables,
	}, alloc)
	return t, d, nil
}

func (t *filterTransformation) Process(view table.View, d *executekit.Dataset, mem arrowmem.Allocator) error {
	// Prepare the function for the column types.
	cols := view.Cols()
	fn, err := t.fn.Prepare(cols)
	if err != nil {
		// TODO(nathanielc): Should we not fail the query for failed compilation?
		return err
	}

	// Retrieve the inferred input type for the function.
	// If all of the inferred inputs are part of the group
	// key, we can evaluate a record with only the group key.
	if t.canFilterByKey(fn, view) {
		return t.filterByKey(view, d)
	}

	// Prefill the columns that can be inferred from the group key.
	// Retrieve the input type from the function and record the indices
	// that need to be obtained from the columns.
	record := values.NewObject(fn.InputType())
	indices := make([]int, 0, view.NCols()-len(view.Key().Cols()))
	for j, c := range view.Cols() {
		if idx := execute.ColIdx(c.Label, view.Key().Cols()); idx >= 0 {
			record.Set(c.Label, view.Key().Value(idx))
			continue
		}
		indices = append(indices, j)
	}

	// Filter the table and pass in the indices we have to read.
	return t.filterTable(d, fn, view, record, indices)
}

func (t *filterTransformation) canFilterByKey(fn *execute.RowPredicatePreparedFn, view table.View) bool {
	inType := fn.InferredInputType()
	nargs, err := inType.NumProperties()
	if err != nil {
		panic(err)
	}

	for i := 0; i < nargs; i++ {
		prop, err := inType.RecordProperty(i)
		if err != nil {
			panic(err)
		}

		// Determine if this key is even valid. If it is not
		// in the table at all, we don't care if it is missing
		// since it will always be missing.
		label := prop.Name()
		if !view.HasCol(label) {
			continue
		}

		// Look for a column with this name in the group key.
		if execute.ColIdx(label, view.Key().Cols()) < 0 {
			// If we cannot find this referenced column in the group
			// key, then it is provided by the table and we need to
			// evaluate each row individually.
			return false
		}
	}

	// All referenced keys were part of the group key.
	return true
}

func (t *filterTransformation) filterByKey(view table.View, d *executekit.Dataset) error {
	key := view.Key()
	cols := key.Cols()
	fn, err := t.fn.Prepare(cols)
	if err != nil {
		return err
	}

	record, err := values.BuildObjectWithSize(len(cols), func(set values.ObjectSetter) error {
		for j, c := range cols {
			set(c.Label, key.Value(j))
		}
		return nil
	})
	if err != nil {
		return err
	}

	v, err := fn.Eval(t.ctx, record)
	if err != nil {
		return err
	}

	if !v {
		if !t.keepEmptyTables {
			return nil
		}
		// If we are supposed to keep empty tables, produce
		// an empty buffer with this group key so later transformations
		// see the group key.
		buffer := arrow.EmptyBuffer(view.Key(), view.Cols())
		return d.Process(table.ViewFromBuffer(buffer))
	}
	view.Retain()
	return d.Process(view)
}

func (t *filterTransformation) filterTable(d *executekit.Dataset, fn *execute.RowPredicatePreparedFn, in table.View, record values.Object, indices []int) error {
	buffer := in.Buffer()
	bitset, err := t.filter(fn, &buffer, record, indices)
	if err != nil {
		return err
	}
	defer bitset.Release()

	n := bitutil.CountSetBits(bitset.Buf(), 0, bitset.Len())
	if n == 0 {
		if t.keepEmptyTables {
			buffer := arrow.EmptyBuffer(in.Key(), in.Cols())
			return d.Process(table.ViewFromBuffer(buffer))
		}
		return nil
	}

	// Produce arrays for each column.
	buffer = arrow.TableBuffer{
		GroupKey: in.Key(),
		Columns:  in.Cols(),
		Values:   make([]array.Interface, in.NCols()),
	}
	for j, col := range in.Cols() {
		arr := in.Borrow(j)
		if in.Key().HasCol(col.Label) {
			buffer.Values[j] = arrow.Slice(arr, 0, int64(n))
			continue
		}
		buffer.Values[j] = arrowutil.Filter(arr, bitset.Bytes(), t.alloc)
	}
	return d.Process(table.ViewFromBuffer(buffer))
}

func (t *filterTransformation) filter(fn *execute.RowPredicatePreparedFn, cr flux.ColReader, record values.Object, indices []int) (*arrowmem.Buffer, error) {
	cols, l := cr.Cols(), cr.Len()
	bitset := arrowmem.NewResizableBuffer(t.alloc)
	bitset.Resize(l)
	for i := 0; i < l; i++ {
		for _, j := range indices {
			record.Set(cols[j].Label, execute.ValueForRow(cr, i, j))
		}

		val, err := fn.Eval(t.ctx, record)
		if err != nil {
			bitset.Release()
			return nil, errors.Wrap(err, codes.Inherit, "failed to evaluate filter function")
		}
		bitutil.SetBitTo(bitset.Buf(), i, val)
	}
	return bitset, nil
}

// RemoveTrivialFilterRule removes Filter nodes whose predicate always evaluates to true.
// type RemoveTrivialFilterRule struct{}
//
// func (RemoveTrivialFilterRule) Name() string {
// 	return "RemoveTrivialFilterRule"
// }
//
// func (RemoveTrivialFilterRule) Pattern() plan.Pattern {
// 	return plan.Pat(FilterKind, plan.Any())
// }
//
// func (RemoveTrivialFilterRule) Rewrite(ctx context.Context, filterNode plan.Node) (plan.Node, bool, error) {
// 	filterSpec := filterNode.ProcedureSpec().(*FilterProcedureSpec)
// 	if filterSpec.Fn.Fn == nil ||
// 		filterSpec.Fn.Fn.Block == nil ||
// 		filterSpec.Fn.Fn.Block.Body == nil {
// 		return filterNode, false, nil
// 	}
//
// 	if bodyExpr, ok := filterSpec.Fn.Fn.GetFunctionBodyExpression(); !ok {
// 		// Not an expression.
// 		return filterNode, false, nil
// 	} else if expr, ok := bodyExpr.(*semantic.BooleanLiteral); !ok || !expr.Value {
// 		// Either not a boolean at all, or evaluates to false.
// 		return filterNode, false, nil
// 	}
//
// 	anyNode := filterNode.Predecessors()[0]
// 	return anyNode, true, nil
// }

// MergeFiltersRule merges Filter nodes whose body is a single return to create one Filter node.
// type MergeFiltersRule struct{}
//
// func (MergeFiltersRule) Name() string {
// 	return "MergeFiltersRule"
// }
//
// func (MergeFiltersRule) Pattern() plan.Pattern {
// 	return plan.Pat(FilterKind, plan.Pat(FilterKind, plan.Any()))
// }
//
// func (MergeFiltersRule) Rewrite(ctx context.Context, filterNode plan.Node) (plan.Node, bool, error) {
// 	// conditions
// 	filterSpec1 := filterNode.ProcedureSpec().(*FilterProcedureSpec)
// 	bodyExpr1, ok := filterSpec1.Fn.Fn.GetFunctionBodyExpression()
// 	if !ok {
// 		// Not an expression.
// 		return filterNode, false, nil
// 	}
// 	filterSpec2 := filterNode.Predecessors()[0].ProcedureSpec().(*FilterProcedureSpec)
// 	bodyExpr2, ok := filterSpec2.Fn.Fn.GetFunctionBodyExpression()
// 	if !ok {
// 		// Not an expression.
// 		return filterNode, false, nil
// 	}
// 	//checks if the fields of OnEmptyMode are different and only allows merge if 1) they are the same 2) keep is the Predecessors field
// 	if filterSpec1.OnEmptyMode != filterSpec2.OnEmptyMode && !filterSpec2.OnEmptyMode {
// 		return filterNode, false, nil
// 	}
//
// 	// created an instance of LogicalExpression to 'and' two different arguments
// 	expr := &semantic.LogicalExpression{Left: bodyExpr1, Operator: ast.AndOperator, Right: bodyExpr2}
// 	// set a new variables that converted the single body statement to a return type that can used with expr
// 	ret := filterSpec2.Fn.Fn.Block.Body[0].(*semantic.ReturnStatement)
// 	ret.Argument = expr
// 	// return the pred node
// 	anyNode := filterNode.Predecessors()[0]
// 	return anyNode, true, nil
// }
