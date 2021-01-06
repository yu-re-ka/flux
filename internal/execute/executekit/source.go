package executekit

// Source is used to generate messages for the source Dataset.
type Source interface {
	// Next will advance the source and generate the next
	// set of messages. If the Source will generate no additional
	// messages, this method should return false.
	//
	// This method is intended to be called repeatedly whenever
	// more data is required from the Source. This method should
	// try to produce the minimum number of messages that make
	// sense given the context. This allows downstream transformations
	// to process the data without creating a substantial backlog.
	//
	// Sources should not attempt to perform backpressure.
	// Backpressure will be applied automatically by the runtime.
	Next() bool

	// Err holds any error encountered while processing the source.
	Err() error
}
