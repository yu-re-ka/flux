package execute

import (
	"github.com/apache/arrow/go/arrow/memory"
	"github.com/influxdata/flux"
	"github.com/influxdata/flux/execute"
	"github.com/influxdata/flux/internal/execute/table"
)

type legacyTransformation struct {
	t     execute.Transformation
	cache table.BuilderCache
}

// NewLegacyTransformation is used to convert an execute.Transformation
// into one that Dataset understands how to communicate with.
func NewLegacyTransformation(t execute.Transformation, mem memory.Allocator) Transformation {
	return &legacyTransformation{
		t: t,
		cache: table.BuilderCache{
			New: func(key flux.GroupKey) table.Builder {
				return table.NewBufferedBuilder(key, mem)
			},
		},
	}
}

func (l *legacyTransformation) ProcessMessage(m execute.Message) error {
	switch m := m.(type) {
	case CloseMessage:
		l.Finish(m.Source(), nil)
		return nil
	case AbortMessage:
		l.Finish(m.Source(), m.Err)
		return nil
	case ProcessMessage:
		// Retrieve the buffered builder and append the
		// table view to it. The view is implemented using
		// arrow.TableBuffer which is compatible with
		// flux.ColReader so we can append it directly.
		b, _ := table.GetBufferedBuilder(m.TableView.Key(), &l.cache)
		return b.AppendBuffer(&m.TableView.buf)
	case FlushKeyMessage:
		// Retrieve the buffered builder for the given key
		// and send the data to the next transformation.
		tbl, err := l.cache.Table(m.Key)
		if err != nil {
			return err
		}
		l.cache.ExpireTable(m.Key)
		return l.t.Process(m.Source(), tbl)
	default:
		// Ignore other messages.
		return nil
	}
}

func (l *legacyTransformation) RetractTable(id execute.DatasetID, key flux.GroupKey) error {
	return l.t.RetractTable(id, key)
}

func (l *legacyTransformation) Process(id execute.DatasetID, tbl flux.Table) error {
	return l.t.Process(id, tbl)
}

func (l *legacyTransformation) UpdateWatermark(id execute.DatasetID, t execute.Time) error {
	return l.t.UpdateWatermark(id, t)
}

func (l *legacyTransformation) UpdateProcessingTime(id execute.DatasetID, t execute.Time) error {
	return l.t.UpdateProcessingTime(id, t)
}

func (l *legacyTransformation) Finish(id execute.DatasetID, err error) {
	l.t.Finish(id, err)
}

func (l *legacyTransformation) SetLabel(label string) {
	l.t.SetLabel(label)
}

func (l *legacyTransformation) Label() string {
	return l.t.Label()
}
