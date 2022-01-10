package universe

import (
	"github.com/apache/arrow/go/arrow/memory"
	"github.com/influxdata/flux"
	"github.com/influxdata/flux/array"
	"github.com/influxdata/flux/arrow"
	"github.com/influxdata/flux/codes"
	"github.com/influxdata/flux/execute"
	"github.com/influxdata/flux/execute/table"
	"github.com/influxdata/flux/internal/errors"
	"github.com/influxdata/flux/plan"
)

const TopKind = "top"

func init() {

	plan.RegisterProcedureSpec(TopKind, newTopProcedure, TopKind)
	execute.RegisterTransformation(TopKind, createTopTransformation)
}

type TopProcedureSpec struct {
	plan.DefaultCost
	N      int64 `json:"n"`
	Offset int64 `json:"offset"`
}

func newTopProcedure(qs flux.OperationSpec, pa plan.Administration) (plan.ProcedureSpec, error) {
	spec, ok := qs.(*TopOpSpec)
	if !ok {
		return nil, errors.Newf(codes.Internal, "invalid spec type %T", qs)
	}
	return &TopProcedureSpec{
		N:      spec.N,
		Offset: spec.Offset,
	}, nil
}

func (s *TopProcedureSpec) Kind() plan.ProcedureKind {
	return TopKind
}
func (s *TopProcedureSpec) Copy() plan.ProcedureSpec {
	ns := new(TopProcedureSpec)
	*ns = *s
	return ns
}

// TriggerSpec implements plan.TriggerAwareProcedureSpec
func (s *TopProcedureSpec) TriggerSpec() plan.TriggerSpec {
	return plan.NarrowTransformationTriggerSpec{}
}

func createTopTransformation(id execute.DatasetID, mode execute.AccumulationMode, spec plan.ProcedureSpec, a execute.Administration) (execute.Transformation, execute.Dataset, error) {
	s, ok := spec.(*TopProcedureSpec)
	if !ok {
		return nil, nil, errors.Newf(codes.Internal, "invalid spec type %T", spec)
	}
	tr := &topTransformation{
		n: s.N,
	}
	return execute.NewAggregateTransformation(id, tr, a.Allocator())
}

type topTransformation struct {
	n int64
}

func (s *topTransformation) Aggregate(chunk table.Chunk, state interface{}, mem memory.Allocator) (interface{}, bool, error) {
	key := chunk.Key()
	buffer := state.(*arrow.TableBuffer)
	if buffer == nil {
		buffer = &arrow.TableBuffer{
			GroupKey: key,
			Columns:  chunk.Cols(),
			Values:   make([]array.Interface, chunk.NCols()),
		}
	}

	return buffer, true, nil
}

func (s *topTransformation) Compute(key flux.GroupKey, state interface{}, d *execute.TransportDataset, mem memory.Allocator) error {
	buffer := state.(*arrow.TableBuffer)
	out := table.ChunkFromBuffer(buffer)
	return d.Process(out)
}

func (s *topTransformation) Close() error { return nil }
