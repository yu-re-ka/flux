package flux

import "context"

// Dependency is an external dependency that can
// be injected into the Flux interpreter.
type Dependency interface {
	Inject(ctx context.Context) context.Context
}
