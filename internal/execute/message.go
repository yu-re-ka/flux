package execute

import "github.com/influxdata/flux/execute"

// MessageType designates a specific message type.
type MessageType = execute.MessageType

const (
	// CloseMsg is sent when there will be no more messages
	// from the upstream Dataset.
	CloseMsg MessageType = iota << 10

	// AbortMsg is sent when an upstream error occurred
	// and no more messages will be sent.
	AbortMsg

	// ProcessMsg is sent when a new TableView is ready to
	// be processed from the upstream Dataset.
	ProcessMsg

	// FlushKeyMsg is sent when the upstream Dataset wishes
	// to flush the data associated with a key presently stored
	// in the Dataset.
	FlushKeyMsg

	// WatermarkKeyMsg is sent when the upstream Dataset will send
	// no more rows with a time older than the time in the watermark
	// for the given key.
	WatermarkKeyMsg
)

// Message is a message sent from a Dataset to another Dataset.
type Message = execute.Message

type message struct {
	src DatasetID
}

func (m message) SrcDatasetID() DatasetID {
	return m.src
}

type CloseMessage struct {
	message
}

type AbortMessage struct {
	message
	Err error
}

type ProcessMessage struct {
	message
	TableView TableView
}

type FlushKeyMessage struct {
	message
	Key TableViewKey
}

type WatermarkKeyMessage struct {
	message
	TimeColumn string
	Watermark  int64
	Key        TableViewKey
}

func (m CloseMessage) Type() MessageType        { return CloseMsg }
func (m AbortMessage) Type() MessageType        { return AbortMsg }
func (m ProcessMessage) Type() MessageType      { return ProcessMsg }
func (m FlushKeyMessage) Type() MessageType     { return FlushKeyMsg }
func (m WatermarkKeyMessage) Type() MessageType { return WatermarkKeyMsg }
