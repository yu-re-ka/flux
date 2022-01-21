package experimental

import (
	"context"
	"fmt"
	"math"
	"sync"

	"github.com/influxdata/flux"
	"github.com/influxdata/flux/codes"
	"github.com/influxdata/flux/execute"
	"github.com/influxdata/flux/internal/errors"
	"github.com/influxdata/flux/plan"
	"github.com/influxdata/flux/runtime"
)

const ChainKind = "experimentalChain"

type ChainOpSpec struct {
	first  *flux.TableObject
	second *flux.TableObject
}

func init() {
	toSQLSignature := runtime.MustLookupBuiltinType("experimental", "chain")
	runtime.RegisterPackageValue("experimental", "chain", flux.MustValue(flux.FunctionValueWithSideEffect(ChainKind, createChainOpSpec, toSQLSignature)))
	flux.RegisterOpSpec(ChainKind, func() flux.OperationSpec { return &ChainOpSpec{} })
	plan.RegisterProcedureSpecWithSideEffect(ChainKind, newChainProcedure, ChainKind)
	execute.RegisterTransformation(ChainKind, createChainTransformation)
}

func getTable(args flux.Arguments, a *flux.Administration, function string, name string) (*flux.TableObject, error) {
	t, ok := args.Get(name)
	if !ok {
		return nil, errors.Newf(codes.Invalid, "argument '%s' not present", name)
	}
	p, ok := t.(*flux.TableObject)
	if !ok {
		return nil, errors.New(codes.Invalid, "got input to %s is not a table object", function)
	}
	a.AddParent(p)

	return p, nil
}

func (o *ChainOpSpec) ReadArgs(args flux.Arguments, a *flux.Administration) error {

	first, err := getTable(args, a, "chain", "first")
	if err != nil {
		return err
	}
	o.first = first

	second, err := getTable(args, a, "chain", "second")
	if err != nil {
		return err
	}
	o.second = second

	return nil
}

func createChainOpSpec(args flux.Arguments, a *flux.Administration) (flux.OperationSpec, error) {
	s := new(ChainOpSpec)
	if err := s.ReadArgs(args, a); err != nil {
		return nil, err
	}
	return s, nil
}

func (ChainOpSpec) Kind() flux.OperationKind {
	return ChainKind
}

type ChainProcedureSpec struct {
	plan.DefaultCost
	Spec *ChainOpSpec
}

func (o *ChainProcedureSpec) Kind() plan.ProcedureKind {
	return ChainKind
}

func (o *ChainProcedureSpec) Copy() plan.ProcedureSpec {
	res := &ChainProcedureSpec{
		Spec: &ChainOpSpec{
			first:  o.Spec.first,
			second: o.Spec.second,
		},
	}
	return res
}

func newChainProcedure(qs flux.OperationSpec, a plan.Administration) (plan.ProcedureSpec, error) {
	spec, ok := qs.(*ChainOpSpec)
	if !ok && spec != nil {
		return nil, errors.Newf(codes.Internal, "invalid spec type %T", qs)
	}
	return &ChainProcedureSpec{Spec: spec}, nil
}

func createChainTransformation(id execute.DatasetID, mode execute.AccumulationMode, spec plan.ProcedureSpec, a execute.Administration) (execute.Transformation, execute.Dataset, error) {
	s, ok := spec.(*ChainProcedureSpec)
	if !ok {
		return nil, nil, errors.Newf(codes.Internal, "invalid spec type %T", spec)
	}

	parents := a.Parents()
	if len(parents) != 2 {
		return nil, nil, errors.New(codes.Internal, "chain expects two parents")
	}

	cache := execute.NewTableBuilderCache(a.Allocator())
	d := execute.NewDataset(id, mode, cache)
	t, err := NewChainTransformation(a.Context(), id, s, d, cache, parents)

	if err != nil {
		return nil, nil, err
	}
	return t, d, nil
}

type mergeChainParentState struct {
	mark       execute.Time
	processing execute.Time
	finished   bool
}

type chainTable struct {
	id    execute.DatasetID
	table flux.Table
}

type chainTransformation struct {
	mu sync.Mutex

	execute.ExecutionNode
	d     execute.Dataset
	cache execute.TableBuilderCache
	ctx   context.Context

	parentState map[execute.DatasetID]*mergeChainParentState

	first  chainTable
	second chainTable

	passthrough *execute.PassthroughDataset
}

func NewChainTransformation(ctx context.Context, id execute.DatasetID, spec *ChainProcedureSpec, d execute.Dataset, cache execute.TableBuilderCache, parents []execute.DatasetID) (*chainTransformation, error) {
	return &chainTransformation{
		d:     d,
		cache: cache,
		ctx:   ctx,

		first: chainTable{
			id: parents[0],
		},
		second: chainTable{
			id: parents[1],
		},

		passthrough: execute.NewPassthroughDataset(id),
	}, nil
}

func (t *chainTransformation) RetractTable(id execute.DatasetID, key flux.GroupKey) error {
	return t.d.RetractTable(key)
}

func (t *chainTransformation) Process(id execute.DatasetID, tbl flux.Table) error {
	fmt.Printf("%v\n", id)
	if t.first.table == nil && id == t.second.id {
		t.second.table = tbl
		return nil
	}

	if id == t.first.id {
		err := tbl.Do(func(cr flux.ColReader) error {
			return nil
		})

		if err != nil {
			return err
		}
	}

	if t.second.table != nil {
		return t.passthrough.Process(t.second.table)
	}

	if id == t.second.id {
		return t.passthrough.Process(tbl)
	}

	return nil
}

func (t *chainTransformation) UpdateWatermark(id execute.DatasetID, mark execute.Time) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.parentState[id].mark = mark

	min := execute.Time(math.MaxInt64)
	for _, state := range t.parentState {
		if state.mark < min {
			min = state.mark
		}
	}

	return t.d.UpdateWatermark(min)
}

func (t *chainTransformation) UpdateProcessingTime(id execute.DatasetID, pt execute.Time) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.parentState[id].processing = pt

	min := execute.Time(math.MaxInt64)
	for _, state := range t.parentState {
		if state.processing < min {
			min = state.processing
		}
	}

	return t.d.UpdateProcessingTime(min)
}

func (t *chainTransformation) Finish(id execute.DatasetID, err error) {
	t.d.Finish(err)
}
