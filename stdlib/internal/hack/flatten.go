package hack

import (
	"fmt"

	"github.com/apache/arrow/go/arrow/memory"
	"github.com/influxdata/flux"
	"github.com/influxdata/flux/execute"
	"github.com/influxdata/flux/internal/arrowutil"
	"github.com/influxdata/flux/internal/execute/table"
	"github.com/influxdata/flux/plan"
	"github.com/influxdata/flux/runtime"
)

const FlattenKind = "internal/hack.flatten"

type FlattenOpSpec struct{}

func init() {
	flattenSignature := runtime.MustLookupBuiltinType("internal/hack", "flatten")

	runtime.RegisterPackageValue("internal/hack", "flatten", flux.MustValue(flux.FunctionValue(FlattenKind, createFlattenOpSpec, flattenSignature)))
	flux.RegisterOpSpec(FlattenKind, newFlattenOp)
	plan.RegisterProcedureSpec(FlattenKind, newFlattenProcedure, FlattenKind)
	execute.RegisterTransformation(FlattenKind, createFlattenTransformation)
}

func createFlattenOpSpec(args flux.Arguments, a *flux.Administration) (flux.OperationSpec, error) {
	if err := a.AddParentFromArgs(args); err != nil {
		return nil, err
	}

	return new(FlattenOpSpec), nil
}

func newFlattenOp() flux.OperationSpec {
	return new(FlattenOpSpec)
}

func (s *FlattenOpSpec) Kind() flux.OperationKind {
	return FlattenKind
}

type FlattenProcedureSpec struct {
	plan.DefaultCost
}

func newFlattenProcedure(qs flux.OperationSpec, pa plan.Administration) (plan.ProcedureSpec, error) {
	_, ok := qs.(*FlattenOpSpec)
	if !ok {
		return nil, fmt.Errorf("invalid spec type %T", qs)
	}

	return new(FlattenProcedureSpec), nil
}

func (s *FlattenProcedureSpec) Kind() plan.ProcedureKind {
	return FlattenKind
}

func (s *FlattenProcedureSpec) Copy() plan.ProcedureSpec {
	return new(FlattenProcedureSpec)
}

// TriggerSpec implements plan.TriggerAwareProcedureSpec
func (s *FlattenProcedureSpec) TriggerSpec() plan.TriggerSpec {
	return plan.NarrowTransformationTriggerSpec{}
}

func createFlattenTransformation(id execute.DatasetID, mode execute.AccumulationMode, spec plan.ProcedureSpec, a execute.Administration) (execute.Transformation, execute.Dataset, error) {
	s, ok := spec.(*FlattenProcedureSpec)
	if !ok {
		return nil, nil, fmt.Errorf("invalid spec type %T", spec)
	}
	t, d := NewFlattenTransformation(id, s, a.Allocator())
	return t, d, nil
}

type flattenTransformation struct {
	d   *execute.PassthroughDataset
	mem memory.Allocator
}

func NewFlattenTransformation(id execute.DatasetID, spec *FlattenProcedureSpec, mem memory.Allocator) (execute.Transformation, execute.Dataset) {
	t := &flattenTransformation{
		d: execute.NewPassthroughDataset(id),
	}
	return t, t.d
}

func (t *flattenTransformation) RetractTable(id execute.DatasetID, key flux.GroupKey) error {
	return t.d.RetractTable(key)
}

func (t *flattenTransformation) Process(id execute.DatasetID, tbl flux.Table) error {
	b := table.NewArrowBuilder(tbl.Key(), t.mem)
	for _, col := range tbl.Cols() {
		if _, err := b.AddCol(col); err != nil {
			return err
		}
	}

	if err := tbl.Do(func(cr flux.ColReader) error {
		for j := range cr.Cols() {
			if err := t.copyValues(b, cr, j); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}

	cpy, err := b.Table()
	if err != nil {
		return err
	}
	return t.d.Process(cpy)
}

func (t *flattenTransformation) copyValues(b *table.ArrowBuilder, cr flux.ColReader, j int) error {
	idx, err := b.CheckCol(cr.Cols()[j])
	if err != nil {
		return err
	}

	arr := table.Values(cr, j)
	arrowutil.Copy(b.Builders[idx], arr)
	return nil
}

func (t *flattenTransformation) UpdateWatermark(id execute.DatasetID, mark execute.Time) error {
	return t.d.UpdateWatermark(mark)
}
func (t *flattenTransformation) UpdateProcessingTime(id execute.DatasetID, pt execute.Time) error {
	return t.d.UpdateProcessingTime(pt)
}
func (t *flattenTransformation) Finish(id execute.DatasetID, err error) {
	t.d.Finish(err)
}
