package execute

import (
	"github.com/apache/arrow/go/arrow/array"
	"github.com/influxdata/flux"
	"github.com/influxdata/flux/arrow"
)

// TableViewKey is the key for a given TableView.
type TableViewKey = flux.GroupKey

// TableView is a view of a Table.
// The view is divided into a set of rows with a common
// set of columns known as the group key.
// The view does not provide a full view of the entire group key
// and a Table is not guaranteed to have rows ordered by the group key.
type TableView struct {
	buf arrow.TableBuffer
}

// Key returns the columns which are common for each row this view.
func (v TableView) Key() TableViewKey {
	return v.buf.Key()
}

// NCols returns the number of columns in this TableView.
func (v TableView) NCols() int {
	return len(v.buf.Columns)
}

// Len returns the number of rows.
func (v TableView) Len() int {
	return v.buf.Len()
}

// Col returns the metadata associated with the column.
func (v TableView) Col(j int) flux.ColMeta {
	return v.buf.Columns[j]
}

// Values returns the array of values in this TableView.
// This will retain a new reference to the array which
// must be released afterwards.
func (v TableView) Values(j int) array.Interface {
	values := v.buf.Values[j]
	values.Retain()
	return values
}

// Retain will retain a reference to this TableView.
func (v TableView) Retain() {
	v.buf.Retain()
}

// Release will release a reference to this buffer.
func (v TableView) Release() {
	v.buf.Release()
}

// Reserve will ensure that there is space to
// add n additional columns to the TableView.
func (v *TableView) Reserve(n int) {
	if sz := len(v.buf.Columns); cap(v.buf.Columns) < sz+n {
		meta := make([]flux.ColMeta, sz, sz+n)
		copy(meta, v.buf.Columns)
		v.buf.Columns = meta

		values := make([]array.Interface, sz, sz+n)
		copy(values, v.buf.Values)
		v.buf.Values = values
	}
}

func (v *TableView) AddColumn(label string, typ flux.ColType, values array.Interface) {
	v.buf.Columns = append(v.buf.Columns, flux.ColMeta{
		Label: label,
		Type:  typ,
	})
	v.buf.Values = append(v.buf.Values, values)
}
