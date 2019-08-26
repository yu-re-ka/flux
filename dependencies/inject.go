package dependencies

import "context"

type dependency interface {
	Inject(ctx context.Context) context.Context
}

// Inject will iterate over the dependency map and inject any
// injectable dependencies into the context.
func Inject(ctx context.Context, deps map[string]interface{}) context.Context {
	for _, dep := range deps {
		if d, ok := dep.(dependency); ok {
			ctx = d.Inject(ctx)
		}
	}
	return ctx
}
