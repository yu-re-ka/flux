package execute

import (
	"github.com/apache/arrow/go/arrow/memory"
	"github.com/influxdata/flux"
	"github.com/influxdata/flux/execute"
	"github.com/influxdata/flux/plan"
)

type DatasetID = execute.DatasetID

// Dataset holds data for a specific node and drives data
// sent to downstream transformations.
//
// When data is processed from an upstream Dataset,
// it sends a message to the associated Transformation which
// will then use the Dataset to store data or send messages
// to the next transformation.
//
// This Dataset also implements a shim for execute.Dataset
// so it can be integrated with the existing execution engine.
// These methods are stubs and do not do anything.
type Dataset struct {
	id    DatasetID
	ts    []Transformation
	cache *execute.RandomAccessGroupLookup
	mem   memory.Allocator
}

func NewDataset(id DatasetID, mem memory.Allocator) *Dataset {
	return &Dataset{
		id:    id,
		cache: execute.NewRandomAccessGroupLookup(),
		mem:   mem,
	}
}

func (d *Dataset) AddTransformation(t execute.Transformation) {
	if t, ok := t.(Transformation); ok {
		d.ts = append(d.ts, t)
		return
	}
	d.ts = append(d.ts, NewLegacyTransformation(t, d.mem))
}

func (d *Dataset) Close() {
}

func (d *Dataset) Abort(err error) {

}

func (d *Dataset) Process(view TableView) {
}

func (d *Dataset) RetractTable(key flux.GroupKey) error      { return nil }
func (d *Dataset) UpdateProcessingTime(t execute.Time) error { return nil }
func (d *Dataset) UpdateWatermark(mark execute.Time) error   { return nil }
func (d *Dataset) Finish(err error)                          {}
func (d *Dataset) SetTriggerSpec(t plan.TriggerSpec)         {}
