package execute

import (
	"fmt"
	"sort"
	"strings"

	"github.com/influxdata/flux"
	"github.com/influxdata/flux/values"
)

type groupKey struct {
	cols   []flux.ColMeta
	values []values.Value
	sorted []int // maintains a list of the sorted indexes
}

func NewGroupKey(cols []flux.ColMeta, values []values.Value) flux.GroupKey {
	return newGroupKey(cols, values)
}

func newGroupKey(cols []flux.ColMeta, values []values.Value) *groupKey {
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
func (k *groupKey) HasCol(label string) bool {
	return ColIdx(label, k.cols) >= 0
}
func (k *groupKey) Index(label string) int {
	return ColIdx(label, k.cols)
}
func (k *groupKey) LabelValue(label string) values.Value {
	if !k.HasCol(label) {
		return nil
	}
	return k.Value(ColIdx(label, k.cols))
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
func (k *groupKey) ValueDuration(j int) Duration {
	return k.values[j].Duration()
}
func (k *groupKey) ValueTime(j int) Time {
	return k.values[j].Time()
}

func (k *groupKey) Sorted() flux.GroupKey {
	return sortedGroupKey{k: k}
}

func (k *groupKey) Less(other flux.GroupKey) bool {
	return flux.GroupKeyLess(k, other)
}

func (k *groupKey) Equal(other flux.GroupKey) bool {
	return flux.GroupKeyEqual(k, other)
}

func (k *groupKey) String() string {
	var b strings.Builder
	b.WriteRune('{')
	for j, c := range k.cols {
		if j != 0 {
			b.WriteRune(',')
		}
		fmt.Fprintf(&b, "%s=%v", c.Label, k.values[j])
	}
	b.WriteRune('}')
	return b.String()
}

type sortedGroupKey struct {
	k *groupKey
}

func (k sortedGroupKey) NCols() int {
	return k.k.NCols()
}

func (k sortedGroupKey) Col(idx int) flux.ColMeta {
	return k.k.Col(k.k.sorted[idx])
}

func (k sortedGroupKey) HasCol(label string) bool {
	return k.k.HasCol(label)
}

func (k sortedGroupKey) Index(label string) int {
	for i, j := range k.k.sorted {
		if k.k.cols[j].Label == label {
			return i
		}
	}
	return -1
}

func (k sortedGroupKey) LabelValue(label string) values.Value {
	return k.k.LabelValue(label)
}

func (k sortedGroupKey) IsNull(j int) bool {
	return k.k.IsNull(k.k.sorted[j])
}

func (k sortedGroupKey) ValueBool(j int) bool {
	return k.k.ValueBool(k.k.sorted[j])
}

func (k sortedGroupKey) ValueUInt(j int) uint64 {
	return k.k.ValueUInt(k.k.sorted[j])
}

func (k sortedGroupKey) ValueInt(j int) int64 {
	return k.k.ValueInt(k.k.sorted[j])
}

func (k sortedGroupKey) ValueFloat(j int) float64 {
	return k.k.ValueFloat(k.k.sorted[j])
}

func (k sortedGroupKey) ValueString(j int) string {
	return k.k.ValueString(k.k.sorted[j])
}

func (k sortedGroupKey) ValueDuration(j int) values.Duration {
	return k.k.ValueDuration(k.k.sorted[j])
}

func (k sortedGroupKey) ValueTime(j int) values.Time {
	return k.k.ValueTime(k.k.sorted[j])
}

func (k sortedGroupKey) Value(j int) values.Value {
	return k.k.Value(k.k.sorted[j])
}

func (k sortedGroupKey) Sorted() flux.GroupKey {
	return k
}

func (k sortedGroupKey) Less(other flux.GroupKey) bool {
	return flux.GroupKeyLess(k, other)
}

func (k sortedGroupKey) Equal(other flux.GroupKey) bool {
	return flux.GroupKeyEqual(k, other)
}

func (k sortedGroupKey) String() string {
	return k.String()
}
