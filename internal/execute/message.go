package execute

import (
	"github.com/influxdata/flux"
	"github.com/influxdata/flux/execute"
	"github.com/influxdata/flux/internal/execute/table"
)

type srcMessage execute.DatasetID

func (m srcMessage) SrcDatasetID() DatasetID {
	return DatasetID(m)
}
func (m srcMessage) Ack() {}

type finishMsg struct {
	srcMessage
	err error
}

func (m *finishMsg) Type() execute.MessageType {
	return execute.FinishType
}
func (m *finishMsg) Error() error {
	return m.err
}
func (m *finishMsg) Dup() execute.Message {
	return m
}

type processViewMsg struct {
	srcMessage
	view table.View
}

func (m *processViewMsg) Type() execute.MessageType {
	return execute.ProcessViewType
}
func (m *processViewMsg) View() table.View {
	return m.view
}
func (m *processViewMsg) Ack() {
	m.view.Release()
}
func (m *processViewMsg) Dup() execute.Message {
	m.view.Retain()
	return m
}

type flushKeyMsg struct {
	srcMessage
	key flux.GroupKey
}

func (m *flushKeyMsg) Type() execute.MessageType {
	return execute.FlushKeyType
}
func (m *flushKeyMsg) Key() flux.GroupKey {
	return m.key
}
func (m *flushKeyMsg) Dup() execute.Message {
	return m
}

type watermarkKeyMsg struct {
	srcMessage
	columnName string
	watermark  int64
	key        flux.GroupKey
}

func (m *watermarkKeyMsg) Type() execute.MessageType {
	return execute.WatermarkKeyType
}
func (m *watermarkKeyMsg) ColumnName() string {
	return m.columnName
}
func (m *watermarkKeyMsg) Time() int64 {
	return m.watermark
}
func (m *watermarkKeyMsg) Key() flux.GroupKey {
	return m.key
}
func (m *watermarkKeyMsg) Dup() execute.Message {
	return m
}
