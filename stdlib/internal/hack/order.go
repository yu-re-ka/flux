package hack

import (
	"fmt"

	"github.com/apache/arrow/go/arrow/memory"
	"github.com/influxdata/flux"
	"github.com/influxdata/flux/execute"
	"github.com/influxdata/flux/plan"
	"github.com/influxdata/flux/runtime"
)

const OrderKind = "internal/hack.order"

type OrderOpSpec struct{}

func init() {
	orderSignature := runtime.MustLookupBuiltinType("internal/hack", "order")

	runtime.RegisterPackageValue("internal/hack", "order", flux.MustValue(flux.FunctionValue(OrderKind, createOrderOpSpec, orderSignature)))
	flux.RegisterOpSpec(OrderKind, newOrderOp)
	plan.RegisterProcedureSpec(OrderKind, newOrderProcedure, OrderKind)
	execute.RegisterTransformation(OrderKind, createOrderTransformation)
}

func createOrderOpSpec(args flux.Arguments, a *flux.Administration) (flux.OperationSpec, error) {
	if err := a.AddParentFromArgs(args); err != nil {
		return nil, err
	}

	return new(OrderOpSpec), nil
}

func newOrderOp() flux.OperationSpec {
	return new(OrderOpSpec)
}

func (s *OrderOpSpec) Kind() flux.OperationKind {
	return OrderKind
}

type OrderProcedureSpec struct {
	plan.DefaultCost
}

func newOrderProcedure(qs flux.OperationSpec, pa plan.Administration) (plan.ProcedureSpec, error) {
	_, ok := qs.(*OrderOpSpec)
	if !ok {
		return nil, fmt.Errorf("invalid spec type %T", qs)
	}

	return new(OrderProcedureSpec), nil
}

func (s *OrderProcedureSpec) Kind() plan.ProcedureKind {
	return OrderKind
}

func (s *OrderProcedureSpec) Copy() plan.ProcedureSpec {
	return new(OrderProcedureSpec)
}

// TriggerSpec implements plan.TriggerAwareProcedureSpec
func (s *OrderProcedureSpec) TriggerSpec() plan.TriggerSpec {
	return plan.NarrowTransformationTriggerSpec{}
}

func createOrderTransformation(id execute.DatasetID, mode execute.AccumulationMode, spec plan.ProcedureSpec, a execute.Administration) (execute.Transformation, execute.Dataset, error) {
	s, ok := spec.(*OrderProcedureSpec)
	if !ok {
		return nil, nil, fmt.Errorf("invalid spec type %T", spec)
	}
	t, d := NewOrderTransformation(id, s, a.Allocator())
	return t, d, nil
}

type orderTransformation struct {
	d              *execute.PassthroughDataset
	tables         *execute.GroupLookup
	watermark      *execute.Time
	processingTime *execute.Time
}

func NewOrderTransformation(id execute.DatasetID, spec *OrderProcedureSpec, mem memory.Allocator) (execute.Transformation, execute.Dataset) {
	t := &orderTransformation{
		d:      execute.NewPassthroughDataset(id),
		tables: execute.NewGroupLookup(),
	}
	return t, t.d
}

func (t *orderTransformation) RetractTable(id execute.DatasetID, key flux.GroupKey) error {
	return t.d.RetractTable(key)
}

func (t *orderTransformation) Process(id execute.DatasetID, tbl flux.Table) error {
	cpy, err := execute.CopyTable(tbl)
	if err != nil {
		return err
	}
	t.tables.Set(cpy.Key(), cpy)
	return nil
}

func (t *orderTransformation) UpdateWatermark(id execute.DatasetID, mark execute.Time) error {
	t.watermark = &mark
	return nil
}
func (t *orderTransformation) UpdateProcessingTime(id execute.DatasetID, pt execute.Time) error {
	t.processingTime = &pt
	return nil
}
func (t *orderTransformation) Finish(id execute.DatasetID, err error) {
	defer t.tables.Clear()

	if err != nil {
		return
	}

	t.tables.Range(func(key flux.GroupKey, value interface{}) {
		if err != nil {
			return
		}
		table := value.(flux.Table)
		err = t.d.Process(table)
	})
	t.d.Finish(err)
}
