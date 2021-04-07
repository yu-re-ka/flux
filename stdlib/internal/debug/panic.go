package debug

import (
	"fmt"

	"github.com/influxdata/flux"
	"github.com/influxdata/flux/codes"
	"github.com/influxdata/flux/execute"
	"github.com/influxdata/flux/internal/errors"
	"github.com/influxdata/flux/plan"
	"github.com/influxdata/flux/runtime"
)

const PanicKind = "internal/debug.panic"

type PanicOpSpec struct {
	Msg string
}

func init() {
	panicSignature := runtime.MustLookupBuiltinType("internal/debug", "panic")

	runtime.RegisterPackageValue("internal/debug", "panic", flux.MustValue(flux.FunctionValue(PanicKind, createPanicOpSpec, panicSignature)))
	flux.RegisterOpSpec(PanicKind, newPanicOp)
	plan.RegisterProcedureSpec(PanicKind, newPanicProcedure, PanicKind)
	execute.RegisterTransformation(PanicKind, createPanicTransformation)
}

func createPanicOpSpec(args flux.Arguments, a *flux.Administration) (flux.OperationSpec, error) {
	if err := a.AddParentFromArgs(args); err != nil {
		return nil, err
	}

	spec := new(PanicOpSpec)

	if msg, err := args.GetRequiredString("msg"); err != nil {
		return nil, err
	} else {
		spec.Msg = msg
	}

	return spec, nil
}

func newPanicOp() flux.OperationSpec {
	return new(PanicOpSpec)
}

func (s *PanicOpSpec) Kind() flux.OperationKind {
	return PanicKind
}

type PanicProcedureSpec struct {
	plan.DefaultCost
	Msg string
}

func newPanicProcedure(qs flux.OperationSpec, pa plan.Administration) (plan.ProcedureSpec, error) {
	spec, ok := qs.(*PanicOpSpec)
	if !ok {
		return nil, fmt.Errorf("invalid spec type %T", qs)
	}

	return &PanicProcedureSpec{
		Msg: spec.Msg,
	}, nil
}

func (s *PanicProcedureSpec) Kind() plan.ProcedureKind {
	return PanicKind
}

func (s *PanicProcedureSpec) Copy() plan.ProcedureSpec {
	spec := *s
	return &spec
}

// TriggerSpec implements plan.TriggerAwareProcedureSpec
func (s *PanicProcedureSpec) TriggerSpec() plan.TriggerSpec {
	return plan.NarrowTransformationTriggerSpec{}
}

func createPanicTransformation(id execute.DatasetID, mode execute.AccumulationMode, spec plan.ProcedureSpec, a execute.Administration) (execute.Transformation, execute.Dataset, error) {
	s, ok := spec.(*PanicProcedureSpec)
	if !ok {
		return nil, nil, fmt.Errorf("invalid spec type %T", spec)
	}
	t, d := NewPanicTransformation(id, s)
	return t, d, nil
}

type panicTransformation struct {
	execute.ExecutionNode
	d   *execute.PassthroughDataset
	msg string
}

func NewPanicTransformation(id execute.DatasetID, spec *PanicProcedureSpec) (execute.Transformation, execute.Dataset) {
	t := &panicTransformation{
		d:   execute.NewPassthroughDataset(id),
		msg: spec.Msg,
	}
	return t, t.d
}

func (t *panicTransformation) RetractTable(id execute.DatasetID, key flux.GroupKey) error {
	return t.d.RetractTable(key)
}

func (t *panicTransformation) Process(id execute.DatasetID, tbl flux.Table) error {
	panic(errors.Newf(codes.Invalid, "%s", t.msg))
}

func (t *panicTransformation) UpdateWatermark(id execute.DatasetID, mark execute.Time) error {
	return t.d.UpdateWatermark(mark)
}
func (t *panicTransformation) UpdateProcessingTime(id execute.DatasetID, pt execute.Time) error {
	return t.d.UpdateProcessingTime(pt)
}
func (t *panicTransformation) Finish(id execute.DatasetID, err error) {
	panic(errors.Newf(codes.Invalid, "%s", t.msg))
}
