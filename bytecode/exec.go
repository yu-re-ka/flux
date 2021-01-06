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
	"github.com/influxdata/flux/values"
	"github.com/influxdata/flux/semantic"

	"github.com/influxdata/flux/internal/errors"
	"github.com/influxdata/flux/internal/spec"
	"github.com/influxdata/flux/plan"
	"github.com/influxdata/flux/codes"
	"github.com/opentracing/opentracing-go"
	"github.com/influxdata/flux/metadata"
	"github.com/influxdata/flux/execute"
	"go.uber.org/zap"
)

type stack struct {
	arr []interface{}
}

func (s *stack) PanicIfNotEmpty() {
	if len(s.arr) != 0 {
		panic("bytecode execution stack was not empty")
	}
}

func (s *stack) PushSideEffects(se []interpreter.SideEffect) {
	s.arr = append( s.arr, se )
}
func (s *stack) PopSideEffects() []interpreter.SideEffect {
	i := s.arr[len(s.arr)-1]
	s.arr = s.arr[:len(s.arr)-1]
	return i.([]interpreter.SideEffect)
}

func (s *stack) PushValue(val values.Value) {
	s.arr = append( s.arr, val )
}
func (s *stack) PopValue() values.Value {
	i := s.arr[len(s.arr)-1]
	s.arr = s.arr[:len(s.arr)-1]
	return i.(values.Value)
}

func (s *stack) Pop() {
	s.arr = s.arr[:len(s.arr)-1]
}


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

func functionName(call *semantic.CallExpression) string {
	switch callee := call.Callee.(type) {
	case *semantic.IdentifierExpression:
		return callee.Name
	case *semantic.MemberExpression:
		return callee.Property
	default:
		return "<anonymous function>"
	}
}

func Execute(ctx context.Context, alloc *memory.Allocator, now time.Time, code []bctypes.OpCode, logger *zap.Logger, scope values.Scope) (flux.Query, error) {
	println("-> execution starting")

	stack := &stack{}

	for _, b := range code {
		switch b.In {
		case bctypes.IN_NONE:
			/* 0, not an instruction */
			panic("IN_NONE")

		case bctypes.IN_CALL:
			callOp := b.Args.(interpreter.CallOp)
			call := callOp.Call
			pipeArgument := callOp.PipeArgument
			pipe  := callOp.Pipe
			properties := callOp.Properties

			argObj, err := values.BuildObject(func(set values.ObjectSetter) error {
				// Pipe evaluated last, popped first.
				if pipe != nil {
					value := stack.PopValue()
					set(pipeArgument, value)
				}

				// Popping call args requires iterating in reverse.
				for i := len(properties)-1; i >= 0; i-- {
					p := properties[i]
					if pipe != nil && p.Key.Key() == pipeArgument {
						return errors.Newf(codes.Invalid, "pipe argument also specified as a keyword parameter: %q", p.Key.Key())
					}
					value := stack.PopValue()
					set(p.Key.Key(), value)
				}

				return nil
			})
			if err != nil {
				return nil, err
			}

			callee := stack.PopValue()

			f := callee.Function()

			// Check if the function is an interpFunction and rebind it.
			// This is needed so that any side effects produced when
			// calling this function are bound to the correct interpreter.
//			if af, ok := f.(function); ok {
//				af.itrp = itrp
//				f = af
//			}

			// Call the function. We attach source location information
			// to this call so it can be available for the function if needed.
			// We do not attach this source location information when evaluating
			// arguments as this source location information is only
			// for the currently called function.
			fname := functionName(call)

			// ctx = withStackEntry(ctx, fname, call.Location())

			value, err := f.Call(ctx, argObj)
			if err != nil {
				return nil, errors.Wrapf(err, codes.Inherit, "error calling function %q @%s", fname, call.Location())
			}
			fmt.Printf("-- IN_CALL: %v\n", callee)

			// This is cheating. Push the return value computed during interpretation.
			stack.PushValue( value )

		case bctypes.IN_SCOPE_LOOKUP:
			scopeLookup := b.Args.(interpreter.ScopeLookup)
			name := scopeLookup.Name

			fmt.Printf("-- IN_SCOPE_LOOKUP: %v\n", name)
			value, ok := scope.Lookup( name )
			if !ok {
				return nil, errors.Newf(codes.Invalid, "undefined identifier %q", name)
			}
			stack.PushValue(value)

		case bctypes.IN_POP:
			println("-- IN_POP")
			stack.Pop()

		case bctypes.IN_CONS_SIDE_EFFECTS:
			println("-- IN_CONS_SIDE_EFFECTS")

			stack.PushSideEffects( make([]interpreter.SideEffect, 0) )

		case bctypes.IN_LOAD_VALUE:
			loadValue := b.Args.(interpreter.LoadValue)
			value := loadValue.Value
			fmt.Printf("-- IN_LOAD_VALUE: %v\n", value)

			stack.PushValue( value )

		case bctypes.IN_APPEND_SIDE_EFFECT:
			println("-- IN_APPEND_SIDE_EFFECT")

			// Semanic node comes from the instruction arguments
			appendSideEffect := b.Args.(interpreter.AppendSideEffect)
			node := appendSideEffect.Node

			// Value comes from the stack. The side effects to add to is one deeper.
			value := stack.PopValue()
			sideEffects := stack.PopSideEffects()

			sideEffects = append( sideEffects, interpreter.SideEffect{Node: node, Value: value} )

			// Result on stack.
			stack.PushSideEffects( sideEffects )

		case bctypes.IN_PROGRAM_START:
			fmt.Printf("-- IN_PROGRAM_START\n")

			sideEffects := stack.PopSideEffects()

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

	stack.PanicIfNotEmpty()
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
