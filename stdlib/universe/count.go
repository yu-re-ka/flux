package universe

import (
	"github.com/apache/arrow/go/arrow/array"
	"github.com/apache/arrow/go/arrow/memory"
	"github.com/influxdata/flux"
	"github.com/influxdata/flux/arrow"
	"github.com/influxdata/flux/codes"
	"github.com/influxdata/flux/execute"
	"github.com/influxdata/flux/internal/errors"
	"github.com/influxdata/flux/internal/execute/execkit"
	"github.com/influxdata/flux/internal/execute/table"
	"github.com/influxdata/flux/plan"
	"github.com/influxdata/flux/runtime"
)

const CountKind = "count"

type CountOpSpec struct {
	execute.AggregateConfig
}

func init() {
	countSignature := runtime.MustLookupBuiltinType("universe", "count")
	runtime.RegisterPackageValue("universe", CountKind, flux.MustValue(flux.FunctionValue(CountKind, CreateCountOpSpec, countSignature)))
	flux.RegisterOpSpec(CountKind, newCountOp)
	plan.RegisterProcedureSpec(CountKind, newCountProcedure, CountKind)
	execute.RegisterTransformation(CountKind, createCountTransformation)
}

func CreateCountOpSpec(args flux.Arguments, a *flux.Administration) (flux.OperationSpec, error) {
	if err := a.AddParentFromArgs(args); err != nil {
		return nil, err
	}
	s := new(CountOpSpec)
	if err := s.AggregateConfig.ReadArgs(args); err != nil {
		return nil, err
	}
	return s, nil
}

func newCountOp() flux.OperationSpec {
	return new(CountOpSpec)
}

func (s *CountOpSpec) Kind() flux.OperationKind {
	return CountKind
}

type CountProcedureSpec struct {
	execute.AggregateConfig
}

func newCountProcedure(qs flux.OperationSpec, a plan.Administration) (plan.ProcedureSpec, error) {
	spec, ok := qs.(*CountOpSpec)
	if !ok {
		return nil, errors.Newf(codes.Internal, "invalid spec type %T", qs)
	}
	return &CountProcedureSpec{
		AggregateConfig: spec.AggregateConfig,
	}, nil
}

func (s *CountProcedureSpec) Kind() plan.ProcedureKind {
	return CountKind
}

func (s *CountProcedureSpec) Copy() plan.ProcedureSpec {
	return &CountProcedureSpec{
		AggregateConfig: s.AggregateConfig,
	}
}

func (s *CountProcedureSpec) AggregateMethod() string {
	return CountKind
}
func (s *CountProcedureSpec) ReAggregateSpec() plan.ProcedureSpec {
	return new(SumProcedureSpec)
}

// TriggerSpec implements plan.TriggerAwareProcedureSpec
func (s *CountProcedureSpec) TriggerSpec() plan.TriggerSpec {
	return plan.NarrowTransformationTriggerSpec{}
}

type CountAgg struct {
	count int64
}

func createCountTransformation(id execute.DatasetID, mode execute.AccumulationMode, spec plan.ProcedureSpec, a execute.Administration) (execute.Transformation, execute.Dataset, error) {
	s, ok := spec.(*CountProcedureSpec)
	if !ok {
		return nil, nil, errors.Newf(codes.Internal, "invalid spec type %T", spec)
	}

	return execkit.NewAggregateTransformation(id, &countTransformation{
		columns: s.Columns,
	}, a.Allocator())
}

type countTransformation struct {
	columns []string
}

func (c *countTransformation) Aggregate(view table.View, state interface{}, mem memory.Allocator) (interface{}, bool, error) {
	var counts []int64
	if state != nil {
		counts = state.([]int64)
	} else {
		counts = make([]int64, len(c.columns))
	}

	for i, label := range c.columns {
		idx := execute.ColIdx(label, view.Cols())
		if idx >= 0 {
			arr := view.Borrow(idx)
			counts[i] += int64(arr.Len() - arr.NullN())
		} else {
			return nil, false, errors.Newf(codes.FailedPrecondition, "column %q does not exist", label)
		}
	}
	return counts, true, nil
}

func (c *countTransformation) Compute(key flux.GroupKey, state interface{}, d *execkit.Dataset, mem memory.Allocator) error {
	if state == nil {
		return nil
	}

	cols := make([]flux.ColMeta, len(key.Cols()), len(key.Cols())+len(c.columns))
	copy(cols, key.Cols())
	for _, label := range c.columns {
		cols = append(cols, flux.ColMeta{
			Type:  flux.TInt,
			Label: label,
		})
	}

	b := table.NewArrowBuilder(key, mem)
	b.Init(cols)
	for j := range key.Cols() {
		if err := arrow.AppendValue(b.Builders[j], key.Value(j)); err != nil {
			return err
		}
	}

	counts := state.([]int64)
	for j, count := range counts {
		if err := arrow.AppendInt(b.Builders[j+len(key.Cols())], count); err != nil {
			return err
		}
	}

	out, err := b.Buffer()
	if err != nil {
		return err
	}
	return d.Process(table.ViewFromBuffer(out))
}

func (a *CountAgg) NewBoolAgg() execute.DoBoolAgg {
	return new(CountAgg)
}
func (a *CountAgg) NewIntAgg() execute.DoIntAgg {
	return new(CountAgg)
}
func (a *CountAgg) NewUIntAgg() execute.DoUIntAgg {
	return new(CountAgg)
}
func (a *CountAgg) NewFloatAgg() execute.DoFloatAgg {
	return new(CountAgg)
}
func (a *CountAgg) NewStringAgg() execute.DoStringAgg {
	return new(CountAgg)
}

func (a *CountAgg) DoBool(vs *array.Boolean) {
	a.count += int64(vs.Len())
}
func (a *CountAgg) DoUInt(vs *array.Uint64) {
	a.count += int64(vs.Len())
}
func (a *CountAgg) DoInt(vs *array.Int64) {
	a.count += int64(vs.Len())
}
func (a *CountAgg) DoFloat(vs *array.Float64) {
	a.count += int64(vs.Len())
}
func (a *CountAgg) DoString(vs *array.Binary) {
	a.count += int64(vs.Len())
}

func (a *CountAgg) Type() flux.ColType {
	return flux.TInt
}
func (a *CountAgg) ValueInt() int64 {
	return a.count
}
func (a *CountAgg) IsNull() bool {
	return false
}
