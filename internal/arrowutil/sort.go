package arrowutil

import (
	"sort"

	"github.com/apache/arrow/go/arrow/memory"
	"github.com/influxdata/flux"
	"github.com/influxdata/flux/array"
	"github.com/influxdata/flux/internal/execute/table"
	"github.com/influxdata/flux/internal/mutable"
)

func Sort(cr flux.ColReader, cols []int, compare CompareFunc, mem memory.Allocator) *array.Int {
	// Construct the indices.
	indices := mutable.NewInt64Array(mem)
	indices.Resize(cr.Len())

	// Retrieve the raw slice and initialize the offsets.
	offsets := indices.Int64Values()
	for i := range offsets {
		offsets[i] = int64(i)
	}

	// Sort the offsets by using the comparison method.
	sort.SliceStable(offsets, func(i, j int) bool {
		i, j = int(offsets[i]), int(offsets[j])
		for _, col := range cols {
			arr := table.Values(cr, col)
			if cmp := compare(arr, arr, i, j); cmp != 0 {
				return cmp < 0
			}
		}
		return false
	})

	// Return the now sorted indices.
	return indices.NewInt64Array()
}
