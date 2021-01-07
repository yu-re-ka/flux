package bytecode

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/influxdata/flux"
	bctypes "github.com/influxdata/flux/bytecode/types"
	"github.com/influxdata/flux/interpreter"
	"github.com/influxdata/flux/memory"
	"github.com/influxdata/flux/semantic"
	"github.com/influxdata/flux/values"

	"github.com/influxdata/flux/codes"
	"github.com/influxdata/flux/execute"
	"github.com/influxdata/flux/internal/errors"
	"github.com/influxdata/flux/internal/spec"
	"github.com/influxdata/flux/metadata"
	"github.com/influxdata/flux/plan"
	"github.com/opentracing/opentracing-go"
	"go.uber.org/zap"

	"github.com/influxdata/flux/values/objects"
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
	s.arr = append(s.arr, se)
}
func (s *stack) PopSideEffects() []interpreter.SideEffect {
	i := s.arr[len(s.arr)-1]
	s.arr = s.arr[:len(s.arr)-1]
	return i.([]interpreter.SideEffect)
}

func (s *stack) PushValue(val values.Value) {
	s.arr = append(s.arr, val)
}
func (s *stack) PopValue() values.Value {
	i := s.arr[len(s.arr)-1]
	s.arr = s.arr[:len(s.arr)-1]
	return i.(values.Value)
}

func (s *stack) PushQuery(q flux.Query) {
	s.arr = append(s.arr, q)
}
func (s *stack) PopQuery() flux.Query {
	i := s.arr[len(s.arr)-1]
	s.arr = s.arr[:len(s.arr)-1]
	return i.(flux.Query)
}

func (s *stack) PushInt(i int) {
	s.arr = append(s.arr, i)
}
func (s *stack) PopInt() int {
	i := s.arr[len(s.arr)-1]
	s.arr = s.arr[:len(s.arr)-1]
	return i.(int)
}

func (s *stack) PushScope(sc values.Scope) {
	s.arr = append(s.arr, sc)
}
func (s *stack) PopScope() values.Scope {
	i := s.arr[len(s.arr)-1]
	s.arr = s.arr[:len(s.arr)-1]
	return i.(values.Scope)
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

func emptyObject() values.Object {
	vsMap := make(map[string]values.Value)
	return values.NewObjectWithValues(vsMap)
}

func objectFromRow(idx int, cr flux.ColReader) values.Object {
	vsMap := make(map[string]values.Value, len(cr.Cols()))
	for j, c := range cr.Cols() {
		var v values.Value
		switch c.Type {
		case flux.TString:
			if vs := cr.Strings(j); vs.IsValid(idx) {
				v = values.New(vs.ValueString(idx))
			} else {
				v = values.NewNull(semantic.BasicString)
			}
		case flux.TInt:
			if vs := cr.Ints(j); vs.IsValid(idx) {
				v = values.New(vs.Value(idx))
			} else {
				v = values.NewNull(semantic.BasicInt)
			}
		case flux.TUInt:
			if vs := cr.UInts(j); vs.IsValid(idx) {
				v = values.New(vs.Value(idx))
			} else {
				v = values.NewNull(semantic.BasicUint)
			}
		case flux.TFloat:
			if vs := cr.Floats(j); vs.IsValid(idx) {
				v = values.New(vs.Value(idx))
			} else {
				v = values.NewNull(semantic.BasicFloat)
			}
		case flux.TBool:
			if vs := cr.Bools(j); vs.IsValid(idx) {
				v = values.New(vs.Value(idx))
			} else {
				v = values.NewNull(semantic.BasicBool)
			}
		case flux.TTime:
			if vs := cr.Times(j); vs.IsValid(idx) {
				v = values.New(values.Time(vs.Value(idx)))
			} else {
				v = values.NewNull(semantic.BasicTime)
			}
		default:
			execute.PanicUnknownType(c.Type)
		}
		vsMap[c.Label] = v
	}
	return values.NewObjectWithValues(vsMap)
}

func Execute(ctx context.Context, alloc *memory.Allocator, now time.Time, code []bctypes.OpCode, logger *zap.Logger, scope values.Scope) (flux.Query, error) {
	fmt.Printf("-> execution starting\n")

	stack := &stack{}

loop:
	for ip := 0; ip < len(code); {
		b := code[ip]
		switch b.In {
		case bctypes.IN_NONE:
			/* 0, not an instruction */
			panic("IN_NONE")

		case bctypes.IN_CALL:
			callOp := b.Args.(interpreter.CallOp)
			call := callOp.Call
			pipe := callOp.Pipe
			properties := callOp.Properties

			var pipeValue values.Value
			propertyValues := make([]values.Value, len(properties))

			// Pipe evaluated last, popped first.
			if pipe != nil {
				pipeValue = stack.PopValue()
			}

			// Popping call args requires iterating in reverse.
			for i := len(properties) - 1; i >= 0; i-- {
				propertyValues[i] = stack.PopValue()
			}

			callee := stack.PopValue()

			ft := callee.Type()
			if ft.Nature() != semantic.Function {
				return nil, errors.Newf(codes.Invalid, "cannot call function: %s: value is of type %v", call.Callee.Location(), callee.Type())
			}

			// Determine which argument matches the pipe argument.
			var pipeArgument string
			if pipe != nil {
				n, err := ft.NumArguments()
				if err != nil {
					return nil, err
				}

				for i := 0; i < n; i++ {
					arg, err := ft.Argument(i)
					if err != nil {
						return nil, err
					}
					if arg.Pipe() {
						pipeArgument = string(arg.Name())
						break
					}
				}

				if pipeArgument == "" {
					return nil, errors.New(codes.Invalid, "pipe parameter value provided to function with no pipe parameter defined")
				}
			}

			argsObj, err := values.BuildObject(func(set values.ObjectSetter) error {
				// Pipe evaluated last, popped first.
				if pipe != nil {
					set(pipeArgument, pipeValue)
				}

				// Popping call args requires iterating in reverse.
				for i := len(properties) - 1; i >= 0; i-- {
					p := properties[i]

					if pipe != nil && p.Key.Key() == pipeArgument {
						return errors.Newf(codes.Invalid, "pipe argument also specified as a keyword parameter: %q", p.Key.Key())
					}

					set(p.Key.Key(), propertyValues[i])
				}

				return nil
			})
			if err != nil {
				return nil, err
			}

			f := callee.Function()

			// Check if the function is an interpFunction and rebind it.
			// This is needed so that any side effects produced when
			// calling this function are bound to the correct interpreter.
			// if af, ok := f.(function); ok {
			//	af.itrp = itrp
			//	f = af
			// }

			// Call the function. We attach source location information
			// to this call so it can be available for the function if needed.
			// We do not attach this source location information when evaluating
			// arguments as this source location information is only
			// for the currently called function.
			fname := functionName(call)

			// ctx = withStackEntry(ctx, fname, call.Location())

			fmt.Printf("-- IN_CALL: %v\n", callee)

			// Bit of a hack here. The call interface doesn't accept the scope because it uses the one
			// set during the original interpretation pass. That pass is now a synthesis with a bogus scope, the real
			// scope is in our state here. So bypass the abstraction and pass it in.
			if sf, ok := f.(interpreter.SynthesizedFunction); ok {
				// Abandoning the interface here. Need to pass a scope and return the new one.
				value, retScope, err := sf.PrepCall(ctx, argsObj, scope)
				if err != nil {
					return nil, errors.Wrapf(err, codes.Inherit, "error calling function %q @%s", fname, call.Location())
				}

				// If the function is a synthesized function, then the return
				// value of the call is the bytecode offset we need to call to.

				// Preserve the scope.
				stack.PushScope(scope)

				// Replace with the nested scope computed during Call()
				scope = retScope

				// Push return location, the next instruction
				stack.PushInt(ip + 1)

				// Jump to targs. Skip the increment.
				ip = int(value.Int())

				fmt.Printf("-> this is a synthesized call, jumping to offset %v\n", ip)
				continue loop
			} else {
				value, err := f.Call(ctx, argsObj)
				if err != nil {
					return nil, errors.Wrapf(err, codes.Inherit, "error calling function %q @%s", fname, call.Location())
				}

				// If not a synthesized call, then the above Call() invocation
				// actually called the function. The return value comes back.
				stack.PushValue(value)
			}

		case bctypes.IN_RET:
			fmt.Printf("-- IN_RET\n")

			// Return value is on the top of the stack.
			retVal := stack.PopValue()
			fmt.Printf("-> got retval %v\n", retVal)

			// Return location is next.
			ip = stack.PopInt()
			fmt.Printf("-> return to %v\n", ip)

			// Scope is last.
			scope = stack.PopScope()
			fmt.Printf("-> scope is %v\n", scope)

			// Now put the return value back.
			stack.PushValue(retVal)

			// Continue because we have modified ip and do not need to advance
			// it.
			continue loop

		case bctypes.IN_SCOPE_LOOKUP:
			scopeLookup := b.Args.(interpreter.ScopeLookup)
			name := scopeLookup.Name

			fmt.Printf("-- IN_SCOPE_LOOKUP: %v\n", name)
			value, ok := scope.Lookup(name)
			if !ok {
				return nil, errors.Newf(codes.Invalid, "undefined identifier %q", name)
			}
			stack.PushValue(value)

		case bctypes.IN_SCOPE_SET:
			scopeSet := b.Args.(interpreter.ScopeSet)
			name := scopeSet.Name

			fmt.Printf("-- IN_SCOPE_SET: %v\n", name)

			value := stack.PopValue()

			scope.Set(name, value)

		case bctypes.IN_POP:
			fmt.Printf("-- IN_POP\n")
			stack.Pop()

		case bctypes.IN_CONS_SIDE_EFFECTS:
			fmt.Printf("-- IN_CONS_SIDE_EFFECTS\n")

			stack.PushSideEffects(make([]interpreter.SideEffect, 0))

		case bctypes.IN_LOAD_VALUE:
			loadValue := b.Args.(interpreter.LoadValue)
			value := loadValue.Value
			fmt.Printf("-- IN_LOAD_VALUE: %v\n", value)

			stack.PushValue(value)

		case bctypes.IN_APPEND_SIDE_EFFECT:
			fmt.Printf("-- IN_APPEND_SIDE_EFFECT\n")

			// Semanic node comes from the instruction arguments
			appendSideEffect := b.Args.(interpreter.AppendSideEffect)
			node := appendSideEffect.Node

			// Value comes from the stack. The side effects to add to is one deeper.
			value := stack.PopValue()
			sideEffects := stack.PopSideEffects()

			sideEffects = append(sideEffects, interpreter.SideEffect{Node: node, Value: value})

			// Result on stack.
			stack.PushSideEffects(sideEffects)

		case bctypes.IN_FIND_RECORD:
			fmt.Printf("-- IN_FIND_RECORD\n")

			// Take a query from the stack, drain it, push a record. Ripped from table_fns.go
			query := stack.PopQuery()

			var tv *objects.Table
			var found bool
			var val values.Object
			var err error
			const rowIdx int = 0
			for res := range query.Results() {
				if err := res.Tables().Do(func(tbl flux.Table) error {
					defer tbl.Done()
					if found {
						// the result is filled, you can skip other tables
						return nil
					}

					found = true

					if found {
						tv, err = objects.NewTable(tbl)
						if err != nil {
							return err
						}
					} else {
						_ = tbl.Do(func(flux.ColReader) error { return nil })
					}
					return nil
				}); err != nil {
					return nil, err
				}
			}

			if tv == nil {
				val = emptyObject()
			} else {
				tbl := tv.Table()

				err := tbl.Do(func(cr flux.ColReader) error {
					if rowIdx < 0 || int(rowIdx) >= cr.Len() {
						val = emptyObject()
						return nil
					}
					val = objectFromRow(int(rowIdx), cr)
					return nil
				})

				if err != nil {
					return nil, err
				}
			}

			stack.PushValue(val)

		case bctypes.IN_EXECUTE_FLUX:
			fmt.Printf("-- IN_EXECUTE_FLUX\n")

			sideEffects := stack.PopSideEffects()

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

			stack.PushQuery(q)

		case bctypes.IN_STOP:
			fmt.Printf("-- IN_STOP\n")
			break loop
		}

		ip = ip + 1
	}

	query := stack.PopQuery()
	stack.PanicIfNotEmpty()
	return query, nil
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
