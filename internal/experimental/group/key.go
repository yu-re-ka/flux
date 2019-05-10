package group

import (
	"fmt"
	"sort"
	"strings"

	"github.com/influxdata/flux"
	"github.com/influxdata/flux/execute"
	"github.com/influxdata/flux/values"
)

type Key interface {
	Col(j int) flux.ColMeta
	NCols() int
	Index(label string) int

	IsNull(j int) bool
	ValueBool(j int) bool
	ValueUInt(j int) uint64
	ValueInt(j int) int64
	ValueFloat(j int) float64
	ValueString(j int) string
	ValueDuration(j int) values.Duration
	ValueTime(j int) values.Time
	Value(j int) values.Value

	Equal(o Key) bool
	Sorted() SortedKey
	String() string
}

type SortedKey interface {
	Key
	Less(o SortedKey) bool
}

type groupKey struct {
	cols   []flux.ColMeta
	values []values.Value
	sorted []int // maintains a list of the sorted indexes
}

func NewKey(cols []flux.ColMeta, values []values.Value) Key {
	return newKey(cols, values)
}

func newKey(cols []flux.ColMeta, values []values.Value) *groupKey {
	sorted := make([]int, len(cols))
	for i := range cols {
		sorted[i] = i
	}
	sort.Slice(sorted, func(i, j int) bool {
		return cols[sorted[i]].Label < cols[sorted[j]].Label
	})
	return &groupKey{
		cols:   cols,
		values: values,
		sorted: sorted,
	}
}

func (k *groupKey) NCols() int {
	return len(k.cols)
}
func (k *groupKey) Col(idx int) flux.ColMeta {
	return k.cols[idx]
}
func (k *groupKey) Index(label string) int {
	return execute.ColIdx(label, k.cols)
}
func (k *groupKey) IsNull(j int) bool {
	return k.values[j].IsNull()
}
func (k *groupKey) Value(j int) values.Value {
	return k.values[j]
}
func (k *groupKey) ValueBool(j int) bool {
	return k.values[j].Bool()
}
func (k *groupKey) ValueUInt(j int) uint64 {
	return k.values[j].UInt()
}
func (k *groupKey) ValueInt(j int) int64 {
	return k.values[j].Int()
}
func (k *groupKey) ValueFloat(j int) float64 {
	return k.values[j].Float()
}
func (k *groupKey) ValueString(j int) string {
	return k.values[j].Str()
}
func (k *groupKey) ValueDuration(j int) execute.Duration {
	return k.values[j].Duration()
}
func (k *groupKey) ValueTime(j int) execute.Time {
	return k.values[j].Time()
}

func (k *groupKey) Sorted() SortedKey {
	panic("implement me")
}

func (k *groupKey) Less(other Key) bool {
	return groupKeyLess(k, other)
}

func (k *groupKey) Equal(other Key) bool {
	return groupKeyEqual(k, other)
}

func (k *groupKey) String() string {
	return groupKeyString(k)
}

func groupKeyEqual(a, b Key) bool {
	a, b = a.Sorted(), b.Sorted()

	if a.NCols() != b.NCols() {
		return false
	}
	for i := 0; i < a.NCols(); i++ {
		if a.Col(i) != b.Col(i) {
			return false
		}
		if anull, bnull := a.IsNull(i), b.IsNull(i); anull && bnull {
			// Both key columns are null, consider them equal
			// So that rows are assigned to the same table.
			continue
		} else if anull || bnull {
			return false
		}

		switch a.Col(i).Type {
		case flux.TBool:
			if a.ValueBool(i) != b.ValueBool(i) {
				return false
			}
		case flux.TInt:
			if a.ValueInt(i) != b.ValueInt(i) {
				return false
			}
		case flux.TUInt:
			if a.ValueUInt(i) != b.ValueUInt(i) {
				return false
			}
		case flux.TFloat:
			if a.ValueFloat(i) != b.ValueFloat(i) {
				return false
			}
		case flux.TString:
			if a.ValueString(i) != b.ValueString(i) {
				return false
			}
		case flux.TTime:
			if a.ValueTime(i) != b.ValueTime(i) {
				return false
			}
		}
	}
	return true
}

// groupKeyLess determines if the former key is lexicographically less than the
// latter.
func groupKeyLess(a, b Key) bool {
	a, b = a.Sorted(), b.Sorted()

	min := a.NCols()
	if b.NCols() < min {
		min = b.NCols()
	}

	for i := 0; i < min; i++ {
		aCol, bCol := a.Col(i), b.Col(i)
		if aCol.Label != bCol.Label {
			// The labels at the current index are different
			// so whichever one is greater is the one missing
			// a value and the one missing a value is the less.
			// That causes this next conditional to look wrong.
			return aCol.Label > bCol.Label
		}

		// The labels are identical. If the types are different,
		// then resolve the ordering based on the type.
		// TODO(jsternberg): Make this official in some way and part of the spec.
		if aCol.Type != bCol.Type {
			return aCol.Type < bCol.Type
		}

		// If a value is null, it is less than.
		if anull, bnull := a.IsNull(i), b.IsNull(i); anull && bnull {
			continue
		} else if anull {
			return true
		} else if bnull {
			return false
		}

		// Neither value is null and they are the same type so compare.
		switch aCol.Type {
		case flux.TBool:
			if av, bv := a.ValueBool(i), b.ValueBool(i); av != bv {
				return bv
			}
		case flux.TInt:
			if av, bv := a.ValueInt(i), b.ValueInt(i); av != bv {
				return av < bv
			}
		case flux.TUInt:
			if av, bv := a.ValueUInt(i), b.ValueUInt(i); av != bv {
				return av < bv
			}
		case flux.TFloat:
			if av, bv := a.ValueFloat(i), b.ValueFloat(i); av != bv {
				return av < bv
			}
		case flux.TString:
			if av, bv := a.ValueString(i), b.ValueString(i); av != bv {
				return av < bv
			}
		case flux.TTime:
			if av, bv := a.ValueTime(i), b.ValueTime(i); av != bv {
				return av < bv
			}
		}
	}

	// In this case, min columns have been compared and found to be equal.
	// Whichever key has the greater number of columns is lexicographically
	// greater than the other.
	return a.NCols() < b.NCols()
}

func groupKeyString(k Key) string {
	var b strings.Builder
	b.WriteRune('{')
	for j, n := 0, k.NCols(); j < n; j++ {
		c := k.Col(j)
		if j != 0 {
			b.WriteRune(',')
		}
		fmt.Fprintf(&b, "%s=%v", c.Label, k.Value(j))
	}
	b.WriteRune('}')
	return b.String()
}

// Range will replace or add the start and stop
// columns to the group key.
func Range(key Key, start, stop values.Time) Key {
	startIdx := key.Index(execute.DefaultStartColLabel)
	stopIdx := key.Index(execute.DefaultStopColLabel)
	ncols := key.NCols()
	if startIdx == -1 {
		startIdx = ncols
		ncols++
	}
	if stopIdx == -1 {
		stopIdx = ncols
		ncols++
	}
	return &groupKeyRange{
		k:        key,
		startIdx: startIdx,
		stopIdx:  stopIdx,
		start:    start,
		stop:     stop,
		ncols:    ncols,
	}
}

type groupKeyRange struct {
	k                 Key
	startIdx, stopIdx int
	start, stop       values.Time
	ncols             int
}

func (k *groupKeyRange) NCols() int {
	return k.ncols
}

func (k *groupKeyRange) Col(idx int) flux.ColMeta {
	if idx == k.startIdx {
		return flux.ColMeta{
			Label: execute.DefaultStartColLabel,
			Type:  flux.TTime,
		}
	} else if idx == k.stopIdx {
		return flux.ColMeta{
			Label: execute.DefaultStopColLabel,
			Type:  flux.TTime,
		}
	}
	return k.k.Col(idx)
}

func (k *groupKeyRange) Index(label string) int {
	if label == execute.DefaultStartColLabel {
		return k.startIdx
	} else if label == execute.DefaultStopColLabel {
		return k.stopIdx
	}
	return k.k.Index(label)
}

func (k *groupKeyRange) IsNull(j int) bool {
	if k.startIdx == j || k.stopIdx == j {
		return false
	}
	return k.k.IsNull(j)
}

func (k *groupKeyRange) ValueBool(j int) bool {
	return k.k.ValueBool(j)
}

func (k *groupKeyRange) ValueUInt(j int) uint64 {
	return k.k.ValueUInt(j)
}

func (k *groupKeyRange) ValueInt(j int) int64 {
	return k.k.ValueInt(j)
}

func (k *groupKeyRange) ValueFloat(j int) float64 {
	return k.k.ValueFloat(j)
}

func (k *groupKeyRange) ValueString(j int) string {
	return k.k.ValueString(j)
}

func (k *groupKeyRange) ValueDuration(j int) values.Duration {
	return k.k.ValueDuration(j)
}

func (k *groupKeyRange) ValueTime(j int) values.Time {
	if k.startIdx == j {
		return k.start
	} else if k.stopIdx == j {
		return k.stop
	}
	return k.k.ValueTime(j)
}

func (k *groupKeyRange) Value(j int) values.Value {
	if k.startIdx == j {
		return values.NewTime(k.start)
	} else if k.stopIdx == j {
		return values.NewTime(k.stop)
	}
	return k.k.Value(j)
}

func (k *groupKeyRange) Equal(o Key) bool {
	panic("implement me")
}

func (k *groupKeyRange) Sorted() SortedKey {
	panic("implement me")
}

func (k *groupKeyRange) String() string {
	return groupKeyString(k)
}
