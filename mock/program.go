package mock

import (
	"context"

	"github.com/influxdata/flux"
	"github.com/influxdata/flux/dependencies"
)

// Program is a mock program that can be returned by the mock compiler.
// It will construct a mock query that will then be passed to ExecuteFn.
type Program struct {
	StartFn   func(ctx context.Context, deps dependencies.Interface) (*Query, error)
	ExecuteFn func(ctx context.Context, q *Query, deps dependencies.Interface)
}

func (p *Program) Start(ctx context.Context, deps dependencies.Interface) (flux.Query, error) {
	startFn := p.StartFn
	if startFn == nil {
		var cancel func()
		ctx, cancel = context.WithCancel(ctx)
		startFn = func(ctx context.Context, deps dependencies.Interface) (*Query, error) {
			results := make(chan flux.Result)
			q := &Query{
				ResultsCh: results,
				CancelFn:  cancel,
				Canceled:  make(chan struct{}),
			}
			go func() {
				defer close(results)
				if p.ExecuteFn != nil {
					p.ExecuteFn(ctx, q, deps)
				}
			}()
			return q, nil
		}
	}
	return startFn(ctx, deps)
}
