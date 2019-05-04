package flux

import "github.com/influxdata/flux/values"

type GroupKey interface {
	NCols() int
	Col(idx int) ColMeta

	HasCol(label string) bool
	Index(label string) int
	LabelValue(label string) values.Value

	IsNull(j int) bool
	ValueBool(j int) bool
	ValueUInt(j int) uint64
	ValueInt(j int) int64
	ValueFloat(j int) float64
	ValueString(j int) string
	ValueDuration(j int) values.Duration
	ValueTime(j int) values.Time
	Value(j int) values.Value

	Sorted() GroupKey

	String() string
}

func GroupKeyEqual(a, b GroupKey) bool {
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
		case TBool:
			if a.ValueBool(i) != b.ValueBool(i) {
				return false
			}
		case TInt:
			if a.ValueInt(i) != b.ValueInt(i) {
				return false
			}
		case TUInt:
			if a.ValueUInt(i) != b.ValueUInt(i) {
				return false
			}
		case TFloat:
			if a.ValueFloat(i) != b.ValueFloat(i) {
				return false
			}
		case TString:
			if a.ValueString(i) != b.ValueString(i) {
				return false
			}
		case TTime:
			if a.ValueTime(i) != b.ValueTime(i) {
				return false
			}
		}
	}
	return true
}

// GroupKeyLess determines if the former key is lexicographically less than the
// latter.
func GroupKeyLess(a, b GroupKey) bool {
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
		case TBool:
			if av, bv := a.ValueBool(i), b.ValueBool(i); av != bv {
				return bv
			}
		case TInt:
			if av, bv := a.ValueInt(i), b.ValueInt(i); av != bv {
				return av < bv
			}
		case TUInt:
			if av, bv := a.ValueUInt(i), b.ValueUInt(i); av != bv {
				return av < bv
			}
		case TFloat:
			if av, bv := a.ValueFloat(i), b.ValueFloat(i); av != bv {
				return av < bv
			}
		case TString:
			if av, bv := a.ValueString(i), b.ValueString(i); av != bv {
				return av < bv
			}
		case TTime:
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
