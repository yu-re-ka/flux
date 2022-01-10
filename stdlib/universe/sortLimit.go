package universe

import (
	"github.com/apache/arrow/go/arrow/memory"
	"github.com/influxdata/flux"
	"github.com/influxdata/flux/array"
	"github.com/influxdata/flux/arrow"
	"github.com/influxdata/flux/codes"
	"github.com/influxdata/flux/execute"
	"github.com/influxdata/flux/execute/table"
	"github.com/influxdata/flux/internal/arrowutil"
	"github.com/influxdata/flux/internal/errors"
	"github.com/influxdata/flux/internal/mutable"
	"github.com/influxdata/flux/plan"
)

const SortLimitKind = "sortLimit"

func init() {
	execute.RegisterTransformation(SortLimitKind, createSortLimitTransformation)
}

type SortLimitProcedureSpec struct {
	plan.DefaultCost
	N       int64
	Columns []string
	Desc    bool
}

func (s *SortLimitProcedureSpec) Kind() plan.ProcedureKind {
	return SortLimitKind
}
func (s *SortLimitProcedureSpec) Copy() plan.ProcedureSpec {
	ns := *s
	ns.Columns = make([]string, len(s.Columns))
	copy(ns.Columns, s.Columns)
	return &ns
}

// TriggerSpec implements plan.TriggerAwareProcedureSpec
func (s *SortLimitProcedureSpec) TriggerSpec() plan.TriggerSpec {
	return plan.NarrowTransformationTriggerSpec{}
}

func createSortLimitTransformation(id execute.DatasetID, mode execute.AccumulationMode, spec plan.ProcedureSpec, a execute.Administration) (execute.Transformation, execute.Dataset, error) {
	s, ok := spec.(*SortLimitProcedureSpec)
	if !ok {
		return nil, nil, errors.Newf(codes.Internal, "invalid spec type %T", spec)
	}
	tr := &sortLimitTransformation{
		n:       int(s.N),
		cols:    s.Columns,
		compare: arrowutil.Compare,
	}
	if s.Desc {
		tr.compare = arrowutil.CompareDesc
	}
	return execute.NewAggregateTransformation(id, tr, a.Allocator())
}

type sortLimitTransformation struct {
	mem     memory.Allocator
	n       int
	cols    []string
	compare arrowutil.CompareFunc
}

func (s *sortLimitTransformation) Aggregate(chunk table.Chunk, state interface{}, mem memory.Allocator) (interface{}, bool, error) {
	sortCols := make([]int, 0, len(s.cols))
	for _, col := range s.cols {
		if idx := execute.ColIdx(col, chunk.Cols()); idx >= 0 {
			// If the sort key is part of the group key, skip it anyway.
			// They are all sorted anyway.
			if chunk.Key().HasCol(col) {
				continue
			}
			sortCols = append(sortCols, idx)
		}
	}
	chunkOrder := arrowutil.Sort(chunk, sortCols, s.compare, s.mem)

	sorted := state.(*mutableTableBuffer)
	if sorted == nil {
		sorted = &mutableTableBuffer{
			GroupKey: chunk.Key(),
			Columns:  chunk.Cols(),
			Values:   make([]mutableArray, chunk.NCols()),
		}
		// Reserve space for the final table buffer
		for j := range sorted.Values {
			sorted.Values[j] = makeMutableArray(sorted.Columns[j].Type, s.mem)
			sorted.Values[j].Reserve(s.n)
		}
	}

	// Sort chunk into sorted, in place
	s.mergeSort(sorted, chunkOrder, chunk, sortCols)

	return sorted, true, nil
}

func (s *sortLimitTransformation) Compute(key flux.GroupKey, state interface{}, d *execute.TransportDataset, mem memory.Allocator) error {
	buffer := state.(*arrow.TableBuffer)
	// TODO create immutable buffer
	out := table.ChunkFromBuffer(*buffer)
	return d.Process(out)
}

func (s *sortLimitTransformation) Close() error { return nil }

// mergeSort adds values from chunk into sorted in place.
func (s *sortLimitTransformation) mergeSort(sorted *mutableTableBuffer, chunkOrder *array.Int, chunk table.Chunk, cols []int) {
	x := 0
	y := 0
	col := 0

	for x < sorted.Len() && y < chunk.Len() {
		s.compare(sorted.Values[col], chunk.Values(col), x, y)
	}
	if x < s.n && chunk.Len()-y > 0 {
		// Copy the rest of chunk into sorted
	}

}

type mutableTableBuffer struct {
	GroupKey flux.GroupKey
	Columns  []flux.ColMeta
	Values   []mutableArray
}

type mutableArray interface {
	array.Interface
	// Reserve will reserve additional capacity in the array for
	// the number of elements to be appended.
	//
	// This does not change the length of the array, but only the capacity.
	Reserve(int)
}

func makeMutableArray(colType flux.ColType, mem memory.Allocator) mutableArray {
	switch colType {
	case flux.TInt, flux.TTime:
		return mutable.NewInt64Array(mem)
	case flux.TUInt:
		return mutable.NewUint64Array(mem)
	case flux.TFloat:
		return mutable.NewFloat64Array(mem)
	case flux.TString:
		// TODO
		//return mutable.NewFloat64Array(mem)
		return nil
	case flux.TBool:
		// TODO
		//return mutable.NewFloat64Array(mem)
		return nil
	default:
		return nil
	}
}
