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

func init() {
	fromStaticSignature := semantic.FunctionPolySignature{
		Parameters: map[string]semantic.PolyType{
			"nrows": semantic.Int,
		}, // user params
		Required: semantic.LabelSet{"nrows"},
		Return:   flux.TableObjectType,
	}
	// telling the flux runtime about the objects that we're creating
	flux.RegisterPackageValue("static", "from", flux.FunctionValue(FromStaticKind, createFromStaticOpSpec, fromStaticSignature))
	flux.RegisterOpSpec(FromStaticKind, newFromStaticOp)
	plan.RegisterProcedureSpec(FromStaticKind, newFromStaticProcedure, FromStaticKind)
	execute.RegisterSource(FromStaticKind, createFromStaticSource)
}

// unique name for mapping
const FromStaticKind = "fromStatic"

// storing user params that are declared elsewhere
// op spec represents what the user has told us;
type FromStaticOpSpec struct {
	nrows int64
}

func createFromStaticOpSpec(args flux.Arguments, administration *flux.Administration) (flux.OperationSpec, error) {
	spec := new(FromStaticOpSpec) // reading flux.args and extracting params
	var err error
	if spec.nrows, err = args.GetRequiredInt("nrows"); err != nil {
		return nil, err
	}
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
	nrows int64
}

// uses op spec to initialize procedure spec
func newFromStaticProcedure(qs flux.OperationSpec, pa plan.Administration) (plan.ProcedureSpec, error) {
	spec, ok := qs.(*FromStaticOpSpec)
	if !ok {
		return nil, fmt.Errorf("invalid spec type %T", qs)
	}
	return &FromStaticProcedureSpec{nrows: spec.nrows}, nil
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
	StaticDecoder := StaticDecoder{administration: a, keyColumn: "T1", valueColumn: "V1", key: "tag1", nrows: spec.nrows}
	return execute.CreateSourceFromDecoder(&StaticDecoder, dsid, a)
}

type StaticDecoder struct {
	administration execute.Administration
	keyColumn      string
	valueColumn    string
	key            string
	nrows          int64
}

func (s *StaticDecoder) Connect(ctx context.Context) error {
	return nil
}
func (s *StaticDecoder) Fetch(ctx context.Context) (bool, error) {
	return false, nil
}
func (s *StaticDecoder) Decode(ctx context.Context) (flux.Table, error) {
	return BuildStaticTable(s.keyColumn, s.valueColumn, s.key, s.nrows, s.administration)
}
func (s *StaticDecoder) Close() error {
	return nil
}

func BuildStaticTable(keyColumn, valueColumn, key string, nrows int64, a execute.Administration) (flux.Table, error) {
	// group keys help ID a table
	groupKey := execute.NewGroupKeyBuilder(nil)
	groupKey.AddKeyValue(keyColumn, values.NewString(key))
	gk, err := groupKey.Build()
	if err != nil {
		return nil, err
	}
	// Create a new table builder indexed by the group key.
	builder := execute.NewColListTableBuilder(gk, a.Allocator())
	if _, err = builder.AddCol(flux.ColMeta{Label: keyColumn, Type: flux.TString}); err != nil {
		return nil, err
	}
	if _, err = builder.AddCol(flux.ColMeta{Label: valueColumn, Type: flux.TFloat}); err != nil {
		return nil, err
	}

	// Add a row of data by appending one value to each column.
	for i := 0; i < int(nrows); i++ {
		if err = builder.AppendString(0, key); err != nil {
			return nil, err
		}
		if err = builder.AppendFloat(1, float64(i)); err != nil {
			return nil, err
		}
	}
	return builder.Table()
}
