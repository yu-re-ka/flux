package execute

import (
	"github.com/influxdata/flux/execute"
)

// Transformation is a method of transforming a Table or Tables into another Table.
// The Transformation is kept at a bare-minimum to keep it simple.
// It contains one method which tells it to process the next message received from an upstream.
// The Message can then be typecast into the proper underlying message type.
//
// It is recommended to use one of the Transformation types that implement a specific type
// of transformation.
//
// For backwards compatibility, Transformation also implements execute.Transformation.
type Transformation interface {
	execute.Transformation
	ProcessMessage(m execute.Message) error
}

// AggregateTransformation implements a transformation that aggregates
// the results from multiple TableView values and then outputs a Table
// with the same group key.
//
// This is similar to NarrowTransformation that it does not modify the group key,
// but different because it will only output a table when the key is flushed.
type AggregateTransformation interface {
	// Process will process the TableView with the state from the previous
	// time a table with this group key was invoked.
	// If this group key has never been invoked before, the
	// state will be nil.
	// The transformation should return the new state and a boolean
	// value of true if the state was created or modified.
	// If false is returned, the new state will be discarded and any
	// old state will be kept.
	Process(view TableView, state interface{}) (interface{}, bool, error)
}

// GroupTransformation is a transformation that can modify the group key.
// Other than modifying the group key, it will output a TableView for each TableView
// that it processes.
type GroupTransformation interface {
	// Process will process the TableView and it may output a new TableView with a new group key.
	Process(view TableView) (TableView, bool, error)
}
