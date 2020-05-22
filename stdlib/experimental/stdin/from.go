package stdin

import (
	"context"
	"github.com/influxdata/flux/values"

	"os"

	"github.com/influxdata/flux"
	"github.com/influxdata/flux/codes"
	"github.com/influxdata/flux/execute"
	"github.com/influxdata/flux/internal/errors"
	"github.com/influxdata/flux/plan"
	"github.com/influxdata/flux/runtime"
	lp "github.com/influxdata/line-protocol"
	"github.com/influxdata/flux/memory"
)

const FromStdinKind = "fromStdin"

type FromStdinOpSpec struct {}

func init() {
	fromStdinSignature := runtime.MustLookupBuiltinType("experimental/stdin", "from")
	runtime.RegisterPackageValue("experimental/stdin", "from", flux.MustValue(flux.FunctionValue(FromStdinKind, createFromStdinOpSpec, fromStdinSignature)))
	flux.RegisterOpSpec(FromStdinKind, func() flux.OperationSpec { return FromStdinOpSpec{} })
	plan.RegisterProcedureSpec(FromStdinKind, newFromStdinProcedure, FromStdinKind)
	execute.RegisterSource(FromStdinKind, createFromStdinSource)
}

func createFromStdinOpSpec(args flux.Arguments, a *flux.Administration) (flux.OperationSpec, error) {
	return FromStdinOpSpec{}, nil
}

func (s FromStdinOpSpec) Kind() flux.OperationKind {
	return FromStdinKind
}


type FromStdinProcedureSpec struct {
	plan.DefaultCost
}

func newFromStdinProcedure(qs flux.OperationSpec, pa plan.Administration) (plan.ProcedureSpec, error) {
	_, ok := qs.(FromStdinOpSpec)
	if !ok {
		return nil, errors.Newf(codes.Internal, "invalid spec type %T", qs)
	}

	return &FromStdinProcedureSpec{}, nil
}

func (s *FromStdinProcedureSpec) Kind() plan.ProcedureKind {
	return FromStdinKind
}

func (s *FromStdinProcedureSpec) Copy() plan.ProcedureSpec {
	return new(FromStdinProcedureSpec)
}

func createFromStdinSource(prSpec plan.ProcedureSpec, id execute.DatasetID, a execute.Administration) (execute.Source, error) {
	_, ok := prSpec.(*FromStdinProcedureSpec)

	if !ok {
		return nil, errors.Newf(codes.Internal, "invalid spec type %T", prSpec)
	}
	return CreateSource(id, a)
}

func CreateSource(id execute.DatasetID, a execute.Administration) (execute.Source, error) {
	parser := lp.NewStreamParser(os.Stdin)

	deps := flux.GetDependencies(a.Context())
	s := &StdinSource{
		id:   id,
		deps: deps,
		mem:  a.Allocator(),
		par: parser,
	}
	return s, nil
}

type StdinSource struct {
	id   execute.DatasetID
	deps flux.Dependencies
	mem  *memory.Allocator
	ts   execute.TransformationSet
	par *lp.StreamParser
}


func (s *StdinSource) AddTransformation(t execute.Transformation) {
	s.ts = append(s.ts, t)
}

func (s *StdinSource) Run(ctx context.Context) {
	table, err := s.processResults()

	err = s.ts.Process(s.id, table)
	// end of stream
	if err == lp.EOF {
		s.ts.Finish(s.id, nil)
	}
	if err != nil {
		s.ts.Finish(s.id, err)
	}
	s.ts.Finish(s.id, nil)
}

func (s *StdinSource) processResults() (table flux.Table, err error) {
	met, err := s.par.Next()

	if err == lp.EOF {
		// return
	}

	if err != nil {
		return nil, err
	}

	groupKey := execute.NewGroupKeyBuilder(nil)
	groupKey.AddKeyValue("_measurement", values.New(met.Name()))
	//groupKey.AddKeyValue("_field", values.New(met.Field))

	gk, err := groupKey.Build()
	if err != nil {
		return nil, err
	}

	builder := execute.NewColListTableBuilder(gk, s.mem)

	builder.AddCol(flux.ColMeta{
		Label: "_time",
		Type:  flux.TTime,
	})
	builder.AddCol(flux.ColMeta{
		Label: "_measurement",
		Type:  flux.TString,
	})

	tagList := met.TagList()

	// Add all tags to Col list
	for _, tag := range tagList {
		if tag == nil {
			continue
		}
		builder.AddCol(flux.ColMeta{
			Label: tag.Key,
			Type:  flux.TString,
		})
	}

	fieldList := met.FieldList()
	// loop over fields
	for _, field := range fieldList {
		if field == nil {
			continue
		}

		fluxType := flux.TInvalid
		switch field.Value.(type) {
		case int64:
			fluxType = flux.TInt
		case uint64:
			fluxType = flux.TUInt
		case float64:
			fluxType = flux.TFloat
		case bool:
			fluxType = flux.TBool
		case string:
			fluxType = flux.TString
		default:
			return nil, errors.New(codes.Internal, "type not implemented")
		}
		builder.AddCol(flux.ColMeta{
			Label: field.Key,
			Type:  fluxType,
		})
	}

	//time
	builder.AppendTime(0, values.ConvertTime(met.Time()))

	//name
	builder.AppendValue(1, values.New(met.Name()))


	for i, tag := range tagList {
		if tag == nil {
			continue
		}
		builder.AppendValue(i+2, values.New(tag.Value))
	}


	for i, field := range fieldList {
		if field == nil {
			continue
		}
		builder.AppendValue(i + 2 + len(tagList), values.New(field.Value))
	}

	return builder.Table()
}