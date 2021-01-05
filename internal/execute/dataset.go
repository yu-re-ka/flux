package execute

import (
	"github.com/apache/arrow/go/arrow/memory"
	"github.com/influxdata/flux"
	"github.com/influxdata/flux/execute"
	"github.com/influxdata/flux/internal/execute/table"
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
	ts    []execute.Transport
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
	if t, ok := t.(execute.Transport); ok {
		d.ts = append(d.ts, t)
		return
	}
	d.ts = append(d.ts, execute.WrapTransformationInTransport(t, d.mem))
}

func (d *Dataset) sendMessage(m execute.Message) error {
	for _, t := range d.ts {
		if err := t.ProcessMessage(m); err != nil {
			return err
		}
	}
	return nil
}

func (d *Dataset) Close() error {
	m := &finishMsg{
		srcMessage: srcMessage(d.id),
	}
	return d.sendMessage(m)
}

func (d *Dataset) Abort(err error) error {
	m := &finishMsg{
		srcMessage: srcMessage(d.id),
		err:        err,
	}
	return d.sendMessage(m)
}

func (d *Dataset) Process(view table.View) error {
	m := &processViewMsg{
		srcMessage: srcMessage(d.id),
		view:       view,
	}
	return d.sendMessage(m)
}

func (d *Dataset) UpdateWatermarkForKey(key flux.GroupKey, column string, t execute.Time) error {
	m := &watermarkKeyMsg{
		srcMessage: srcMessage(d.id),
		columnName: column,
		watermark:  int64(t),
		key:        key,
	}
	return d.sendMessage(m)
}

func (d *Dataset) RetractTable(key flux.GroupKey) error      { return nil }
func (d *Dataset) UpdateProcessingTime(t execute.Time) error { return nil }
func (d *Dataset) UpdateWatermark(mark execute.Time) error   { return nil }
func (d *Dataset) Finish(err error)                          {}
func (d *Dataset) SetTriggerSpec(t plan.TriggerSpec)         {}
