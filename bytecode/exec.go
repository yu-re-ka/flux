package bytecode

import (
	"context"
	"time"
	"fmt"
	"sync"

	bctypes "github.com/influxdata/flux/bytecode/types"
	"github.com/influxdata/flux/memory"
	"github.com/influxdata/flux"
	"github.com/influxdata/flux/interpreter"

	"github.com/influxdata/flux/internal/errors"
	"github.com/influxdata/flux/internal/spec"
	"github.com/influxdata/flux/plan"
	"github.com/influxdata/flux/codes"
	"github.com/opentracing/opentracing-go"
	"github.com/influxdata/flux/metadata"
	"github.com/influxdata/flux/execute"
	"go.uber.org/zap"
)

// query implements the flux.Query interface.
type query struct {
	results chan flux.Result
	stats   flux.Statistics
	alloc   *memory.Allocator
	span    opentracing.Span
	cancel  func()
	err     error
	wg      sync.WaitGroup
}

func (q *query) Results() <-chan flux.Result {
	return q.results
}

func (q *query) Done() {
	q.cancel()
	q.wg.Wait()
	q.stats.MaxAllocated = q.alloc.MaxAllocated()
	q.stats.TotalAllocated = q.alloc.TotalAllocated()
	if q.span != nil {
		q.span.Finish()
		q.span = nil
	}
}

func (q *query) Cancel() {
	q.cancel()
}

func (q *query) Err() error {
	return q.err
}

func (q *query) Statistics() flux.Statistics {
	return q.stats
}

func (q *query) ProfilerResults() (flux.ResultIterator, error) {
	return nil, nil
}

func Execute(ctx context.Context, alloc *memory.Allocator, now time.Time, code []bctypes.OpCode, logger *zap.Logger) (flux.Query, error) {
	println("-> execution starting")

	for _, b := range code {
		switch b.In {
		case bctypes.IN_NONE:
			/* ignore. */

		case bctypes.IN_PROGRAM_START:
			println("-- IN_PROGRAM_START")

			programStart := b.Args.(interpreter.ProgramStart)
			sideEffects := programStart.SideEffects

			println("-> starting bytecode program")

			// Producing flux spec: side effects -> *flux.Spec
			var sp *flux.Spec
			var err error

			sp, err = spec.FromEvaluation(ctx, sideEffects, now)
			if err != nil {
				return nil, errors.Wrap(err, codes.Inherit, "error in query specification while starting program")
			}

			// Planning: *flux.Spec -> plan.Spec
			var ps *plan.Spec

			// TODO: need to get plan options from the execution dependencies.
			// These are set during evaluation and need to be retrieved along
			// with now.
			pb := plan.PlannerBuilder{}

			// planOptions := nil //o.planOptions
			// lopts := planOptions.logical
			// popts := planOptions.physical
			// pb.AddLogicalOptions(lopts...)
			// pb.AddPhysicalOptions(popts...)

			ps, err = pb.Build().Plan(ctx, sp)
			if err != nil {
				return nil, errors.Wrap(err, codes.Inherit, "error in building plan while starting program")
			}

			ctx, cancel := context.WithCancel(ctx)

			// This span gets closed by the query when it is done.
			s, cctx := opentracing.StartSpanFromContext(ctx, "execute")
			results := make(chan flux.Result)
			q := &query{
				results: results,
				alloc:   alloc,
				span:    s,
				cancel:  cancel,
				stats: flux.Statistics{
					Metadata: make(metadata.Metadata),
				},
			}

			if execute.HaveExecutionDependencies(ctx) {
				deps := execute.GetExecutionDependencies(ctx)
				q.stats.Metadata.AddAll(deps.Metadata)
			}

			q.stats.Metadata.Add("flux/query-plan",
				fmt.Sprintf("%v", plan.Formatted(ps, plan.WithDetails())))

			// Execute
			e := execute.NewExecutor(logger)
			resultMap, md, err := e.Execute(cctx, ps, q.alloc)
			if err != nil {
				s.Finish()
				return nil, err
			}

			// There was no error so send the results downstream.
			q.wg.Add(1)
			go processResults(cctx, q, resultMap)

			// Begin reading from the metadata channel.
			q.wg.Add(1)
			go readMetadata(q, md)

			return q, nil
		}
	}

	return nil, nil
}

func processResults(ctx context.Context, q *query, resultMap map[string]flux.Result) {
	defer q.wg.Done()
	defer close(q.results)

	for _, res := range resultMap {
		select {
		case q.results <- res:
		case <-ctx.Done():
			q.err = ctx.Err()
			return
		}
	}
}

func readMetadata(q *query, metaCh <-chan metadata.Metadata) {
	defer q.wg.Done()
	for md := range metaCh {
		q.stats.Metadata.AddAll(md)
	}
}
