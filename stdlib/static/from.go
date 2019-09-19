package static

import (
	"context"

	"fmt"

	"github.com/influxdata/flux"
	"github.com/influxdata/flux/execute"
	"github.com/influxdata/flux/plan"
	"github.com/influxdata/flux/semantic"
	"github.com/influxdata/flux/values"
	_ "github.com/lib/pq"
)

// unique name for mapping
const FromStaticKind = "fromStatic"

// storing user params that are declared elsewhere
// op spec represents what the user has told us;
type FromStaticOpSpec struct {
}

func init() {
	fromStaticSignature := semantic.FunctionPolySignature{
		Parameters: map[string]semantic.PolyType{}, // user params
		Return:     flux.TableObjectType,
	}
	// telling the flux runtime about the objects that we're creating
	flux.RegisterPackageValue("static", "from", flux.FunctionValue(FromStaticKind, createFromStaticOpSpec, fromStaticSignature))
	flux.RegisterOpSpec(FromStaticKind, newFromStaticOp)
	plan.RegisterProcedureSpec(FromStaticKind, newFromStaticProcedure, FromStaticKind)
	execute.RegisterSource(FromStaticKind, createFromStaticSource)
}

func createFromStaticOpSpec(args flux.Arguments, administration *flux.Administration) (flux.OperationSpec, error) {
	spec := new(FromStaticOpSpec) // reading flux.args and extracting params

	return spec, nil
}

func newFromStaticOp() flux.OperationSpec {
	return new(FromStaticOpSpec)
}

func (s *FromStaticOpSpec) Kind() flux.OperationKind {
	return FromStaticKind
}

// procedure spec is internal representation of the entire file; used by the planner
type FromStaticProcedureSpec struct {
	plan.DefaultCost
}

// uses op spec to initialize procedure spec
func newFromStaticProcedure(qs flux.OperationSpec, pa plan.Administration) (plan.ProcedureSpec, error) {
	_, ok := qs.(*FromStaticOpSpec)
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
	return ns
}

// uses a procedure spec to create a source object for flux runtime
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
	groupKey := execute.NewGroupKeyBuilder(nil)
	groupKey.AddKeyValue("tag1", values.NewString("T1"))
	gk, err := groupKey.Build()
	if err != nil {
		return nil, err
	}

	builder := execute.NewColListTableBuilder(gk, c.administration.Allocator())
	builder.AddCol(flux.ColMeta{
		Label: "tag1",
		Type:  flux.TString,
	})
	builder.AddCol(flux.ColMeta{
		Label: "F1",
		Type:  flux.TFloat,
	})
	builder.AppendString(0, "T1")

	builder.AppendFloat(1, 1.0)
	return builder.Table()
}

func (c *StaticIterator) Close() error {
	return nil
}
