package execute

import (
	"github.com/apache/arrow/go/arrow/memory"
	"github.com/influxdata/flux"
	"github.com/influxdata/flux/execute"
)

// NarrowTransformation implements a transformation that processes
// a TableView and does not modify its group key.
type NarrowTransformation interface {
	// Process will process the TableView and it may output a new TableView.
	Process(view TableView, mem memory.Allocator) (TableView, bool, error)
}

type narrowTransformation struct {
	t     NarrowTransformation
	d     *Dataset
	label string
}

// NewNarrowTransformation constructs a Transformation and Dataset
// using the NarrowTransformation implementation.
func NewNarrowTransformation(id DatasetID, t NarrowTransformation, mem memory.Allocator) (execute.Transformation, execute.Dataset) {
	tr := &narrowTransformation{
		t: t,
		d: NewDataset(id, mem),
	}
	return tr, tr.d
}

// ProcessMessage will process the incoming message.
func (n *narrowTransformation) ProcessMessage(m Message) error {
	switch m := m.(type) {
	case CloseMessage:
		n.d.Close()
	case AbortMessage:
		n.d.Abort(m.Err)
	case ProcessMessage:
		view, ok, err := n.t.Process(m.TableView, n.d.mem)
		if err != nil {
			return err
		} else if ok {
			n.d.Process(view)
		}
	}
	return nil
}

// Process is implemented to remain compatible with legacy upstreams.
// It converts the incoming stream into a set of appropriate messages.
func (n *narrowTransformation) Process(id execute.DatasetID, tbl flux.Table) error {
	panic("implement me")
}

// Finish is implemented to remain compatible with legacy upstreams.
func (n *narrowTransformation) Finish(id execute.DatasetID, err error) {
	if err != nil {
		n.d.Abort(err)
		return
	}
	n.d.Close()
}

func (n *narrowTransformation) RetractTable(id execute.DatasetID, key flux.GroupKey) error {
	return nil
}
func (n *narrowTransformation) UpdateWatermark(id execute.DatasetID, t execute.Time) error {
	return nil
}
func (n *narrowTransformation) UpdateProcessingTime(id execute.DatasetID, t execute.Time) error {
	return nil
}
func (n *narrowTransformation) SetLabel(label string) {
	n.label = label
}
func (n *narrowTransformation) Label() string {
	return n.label
}
