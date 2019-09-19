package sql

import (
	"context"

	"fmt"
	"github.com/influxdata/flux"
	"github.com/influxdata/flux/execute"
	"github.com/influxdata/flux/plan"
	"github.com/influxdata/flux/semantic"
	_ "github.com/lib/pq"
)

const FromStaticKind = "fromStatic"

type FromStaticOpSpec struct {
}

func init() {
	fromStaticSignature := semantic.FunctionPolySignature{
		Parameters: map[string]semantic.PolyType{},
		Return:     flux.TableObjectType,
	}
	flux.RegisterPackageValue("static", "from", flux.FunctionValue(FromStaticKind, createFromStaticOpSpec, fromStaticSignature))
	flux.RegisterOpSpec(FromStaticKind, newFromStaticOp)
	plan.RegisterProcedureSpec(FromStaticKind, newFromStaticProcedure, FromStaticKind)
	execute.RegisterSource(FromStaticKind, createFromStaticSource)
}

func createFromStaticOpSpec(args flux.Arguments, administration *flux.Administration) (flux.OperationSpec, error) {
	spec := new(FromStaticOpSpec)

	return spec, nil
}

func newFromStaticOp() flux.OperationSpec {
	return new(FromStaticOpSpec)
}

func (s *FromStaticOpSpec) Kind() flux.OperationKind {
	return FromStaticKind
}

type FromStaticProcedureSpec struct {
	plan.DefaultCost
}

func newFromStaticProcedure(qs flux.OperationSpec, pa plan.Administration) (plan.ProcedureSpec, error) {
	spec, ok := qs.(*FromStaticOpSpec)
	if !ok {
		return nil, fmt.Errorf("invalid spec type %T", qs)
	}

	return &FromStaticProcedureSpec{}, nil
}

func (s *FromStaticProcedureSpec) Kind() plan.ProcedureKind {
	return FromStaticKind
}

func (s *FromStaticProcedureSpec) Copy() plan.ProcedureSpec {
	ns := new(FromStaticProcedureSpec)
	ns.DriverName = s.DriverName
	ns.DataSourceName = s.DataSourceName
	ns.Query = s.Query
	return ns
}

func createFromStaticSource(prSpec plan.ProcedureSpec, dsid execute.DatasetID, a execute.Administration) (execute.Source, error) {
	spec, ok := prSpec.(*FromStaticProcedureSpec)
	if !ok {
		return nil, fmt.Errorf("invalid spec type %T", prSpec)
	}

	StaticIterator := StaticIterator{id: dsid, spec: spec, administration: a}

	return execute.CreateSourceFromDecoder(&StaticIterator, dsid, a)
}

type StaticIterator struct {
	id             execute.DatasetID
	administration execute.Administration
	spec           *FromStaticProcedureSpec
	reader         *execute.RowReader
}

var _ execute.SourceDecoder = (*StaticIterator)(nil)

func (c *StaticIterator) Connect(ctx context.Context) error {

	return nil
}

func (c *StaticIterator) Fetch(ctx context.Context) (bool, error) {
	return false, nil
}

func (c *StaticIterator) Decode(ctx context.Context) (flux.Table, error) {
	groupKey := execute.NewGroupKey(nil, nil)
	builder := execute.NewColListTableBuilder(groupKey, c.administration.Allocator())

	return builder.Table()
}

func (c *StaticIterator) Close() error {
	return c.db.Close()
}
