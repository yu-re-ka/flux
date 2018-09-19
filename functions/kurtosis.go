package functions

import (
	"fmt"

	"github.com/influxdata/flux/functions/futhark"

	"github.com/influxdata/flux"
	"github.com/influxdata/flux/execute"
	"github.com/influxdata/flux/plan"
)

const KurtosisKind = "kurtosis"

type KurtosisOpSpec struct {
	execute.AggregateConfig
}

var kurtosisSignature = execute.DefaultAggregateSignature()

func init() {
	flux.RegisterFunction(KurtosisKind, createKurtosisOpSpec, kurtosisSignature)
	flux.RegisterOpSpec(KurtosisKind, newKurtosisOp)
	plan.RegisterProcedureSpec(KurtosisKind, newKurtosisProcedure, KurtosisKind)
	execute.RegisterTransformation(KurtosisKind, createKurtosisTransformation)
}

func createKurtosisOpSpec(args flux.Arguments, a *flux.Administration) (flux.OperationSpec, error) {
	if err := a.AddParentFromArgs(args); err != nil {
		return nil, err
	}

	s := new(KurtosisOpSpec)
	if err := s.AggregateConfig.ReadArgs(args); err != nil {
		return nil, err
	}

	return s, nil
}

func newKurtosisOp() flux.OperationSpec {
	return new(KurtosisOpSpec)
}

func (s *KurtosisOpSpec) Kind() flux.OperationKind {
	return KurtosisKind
}

type KurtosisProcedureSpec struct {
	execute.AggregateConfig
}

func newKurtosisProcedure(qs flux.OperationSpec, a plan.Administration) (plan.ProcedureSpec, error) {
	spec, ok := qs.(*KurtosisOpSpec)
	if !ok {
		return nil, fmt.Errorf("invalid spec type %T", qs)
	}
	return &KurtosisProcedureSpec{
		AggregateConfig: spec.AggregateConfig,
	}, nil
}

func (s *KurtosisProcedureSpec) Kind() plan.ProcedureKind {
	return KurtosisKind
}
func (s *KurtosisProcedureSpec) Copy() plan.ProcedureSpec {
	return &KurtosisProcedureSpec{
		AggregateConfig: s.AggregateConfig,
	}
}

func createKurtosisTransformation(id execute.DatasetID, mode execute.AccumulationMode, spec plan.ProcedureSpec, a execute.Administration) (execute.Transformation, execute.Dataset, error) {
	s, ok := spec.(*KurtosisProcedureSpec)
	if !ok {
		return nil, nil, fmt.Errorf("invalid spec type %T", spec)
	}
	agg := futhark.NewAggregator(futhark.Kurtosis)
	t, d := execute.NewAggregateTransformationAndDataset(id, mode, agg, s.AggregateConfig, a.Allocator())
	return t, d, nil
}
