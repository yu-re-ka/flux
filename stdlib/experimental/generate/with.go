package generate

import (
	"context"

	"github.com/apache/arrow/go/arrow/memory"
	"github.com/influxdata/flux"
	"github.com/influxdata/flux/array"
	"github.com/influxdata/flux/arrow"
	"github.com/influxdata/flux/codes"
	"github.com/influxdata/flux/compiler"
	"github.com/influxdata/flux/dependencies/rand"
	"github.com/influxdata/flux/execute"
	"github.com/influxdata/flux/execute/table"
	"github.com/influxdata/flux/internal/errors"
	"github.com/influxdata/flux/interpreter"
	"github.com/influxdata/flux/plan"
	"github.com/influxdata/flux/runtime"
	"github.com/influxdata/flux/semantic"
	"github.com/influxdata/flux/values"
)

const (
	pkgpath  = "experimental/generate"
	WithKind = pkgpath + ".with"
)

func init() {
	withSignature := runtime.MustLookupBuiltinType(pkgpath, "with")
	runtime.RegisterPackageValue(pkgpath, "with", flux.MustValue(flux.FunctionValue("with", createWithOpSpec, withSignature)))
	plan.RegisterProcedureSpec(WithKind, createWithProcedureSpec, WithKind)
	execute.RegisterSource(WithKind, createSource)
}

func createWithOpSpec(args flux.Arguments, a *flux.Administration) (flux.OperationSpec, error) {
	spec := &withOpSpec{}

	if n, err := args.GetRequiredInt("n"); err != nil {
		return nil, err
	} else if n <= 0 {
		return nil, errors.Newf(codes.Invalid, "n must be greater than zero, got %d", n)
	} else {
		spec.N = n
	}

	if seed, ok, err := args.GetInt("seed"); err != nil {
		return nil, err
	} else if ok {
		spec.Seed = &seed
	}

	if cardinality, err := args.GetRequiredObject("cardinality"); err != nil {
		return nil, err
	} else {
		spec.Keys = make([]string, 0, cardinality.Len())
		spec.Cardinality = make([]int64, 0, cardinality.Len())
		cardinality.Range(func(name string, v values.Value) {
			if err != nil {
				return
			}

			if n := v.Type().Nature(); n != semantic.Int {
				err = errors.Newf(codes.Invalid, "cardinality values must be of type %s, got %s for key %s", semantic.Int, n, name)
				return
			} else if cardinality := v.Int(); cardinality <= 0 {
				err = errors.Newf(codes.Invalid, "cardinality value must be greater than zero, got %d for key %s", cardinality, name)
				return
			}
			spec.Keys = append(spec.Keys, name)
			spec.Cardinality = append(spec.Cardinality, v.Int())
		})
	}

	if keyFn, err := args.GetRequiredFunction("key"); err != nil {
		return nil, err
	} else {
		spec.KeyFn, err = interpreter.ResolveFunction(keyFn)
		if err != nil {
			return nil, err
		}
	}

	if valuesFn, err := args.GetRequiredFunction("values"); err != nil {
		return nil, err
	} else {
		spec.ValuesFn, err = interpreter.ResolveFunction(valuesFn)
		if err != nil {
			return nil, err
		}
	}
	return spec, nil
}

type withOpSpec struct {
	WithProcedureSpec
}

func (w *withOpSpec) Kind() flux.OperationKind {
	return WithKind
}

func createWithProcedureSpec(spec flux.OperationSpec, a plan.Administration) (plan.ProcedureSpec, error) {
	s, ok := spec.(*withOpSpec)
	if !ok {
		return nil, errors.Newf(codes.Internal, "invalid spec type %T", spec)
	}

	ns := s.WithProcedureSpec
	return &ns, nil
}

type WithProcedureSpec struct {
	plan.DefaultCost
	Keys        []string
	Cardinality []int64
	KeyFn       interpreter.ResolvedFunction
	ValuesFn    interpreter.ResolvedFunction
	N           int64
	Seed        *int64
}

func (s *WithProcedureSpec) Kind() plan.ProcedureKind {
	return WithKind
}

func (s *WithProcedureSpec) Copy() plan.ProcedureSpec {
	ns := *s
	ns.Keys = make([]string, len(s.Keys))
	copy(ns.Keys, s.Keys)
	ns.Cardinality = make([]int64, len(s.Cardinality))
	copy(ns.Cardinality, s.Cardinality)
	ns.KeyFn = s.KeyFn.Copy()
	ns.ValuesFn = s.ValuesFn.Copy()
	return &ns
}

type Source struct {
	spec       *WithProcedureSpec
	keyFn      compiler.Func
	keyFnInput values.Object
	schemaArg  values.Object
	mem        memory.Allocator
}

func createSource(spec plan.ProcedureSpec, id execute.DatasetID, a execute.Administration) (execute.Source, error) {
	s, ok := spec.(*WithProcedureSpec)
	if !ok {
		return nil, errors.Newf(codes.Internal, "invalid spec type %T", spec)
	}
	return NewSource(s, id, a.Allocator())
}

func NewSource(spec *WithProcedureSpec, id execute.DatasetID, mem memory.Allocator) (execute.Source, error) {
	source := &Source{
		spec: spec,
		mem:  mem,
	}
	if err := source.initializeKeyFunc(); err != nil {
		return nil, err
	}
	return execute.CreateSourceFromIterator(source, id)
}

func (s *Source) Do(ctx context.Context, f func(flux.Table) error) error {
	// If we were given a seed, inject it here so random numbers
	// will use the seed.
	if s.spec.Seed != nil {
		ctx = rand.Seed(ctx, *s.spec.Seed)
	}

	// Keep the cardinality state and iterate through it sequentially.
	// Initialize the cardinality state. The default value for these
	// is zero and we wanted to initialize as zero anyway.
	cardinality := make([]int64, len(s.spec.Cardinality))
	for {
		// For each series iteration, we generate a single key.
		key, err := s.createSeriesKey(ctx, cardinality)
		if err != nil {
			return err
		}

		// Generate the values for this group key.
		if err := s.generateValues(ctx, key, f); err != nil {
			return err
		}

		if !s.nextSeries(ctx, cardinality) {
			break
		}
	}
	return nil
}

func (s *Source) initializeKeyFunc() error {
	schemaPropertyTypes := make([]semantic.PropertyType, len(s.spec.Keys))
	for i, key := range s.spec.Keys {
		schemaPropertyTypes[i] = semantic.PropertyType{
			Key:   []byte(key),
			Value: semantic.BasicInt,
		}
	}
	schemaType := semantic.NewObjectType(schemaPropertyTypes)

	inType := semantic.NewObjectType([]semantic.PropertyType{{
		Key:   []byte("schema"),
		Value: schemaType,
	}})
	scope := compiler.ToScope(s.spec.KeyFn.Scope)
	fn, err := compiler.Compile(scope, s.spec.KeyFn.Fn, inType)
	if err != nil {
		return err
	}
	s.keyFn = fn
	s.keyFnInput = values.NewObject(inType)
	s.schemaArg = values.NewObject(schemaType)
	s.keyFnInput.Set("schema", s.schemaArg)
	return nil
}

func (s *Source) nextSeries(ctx context.Context, cardinality []int64) bool {
	if ctx.Err() != nil {
		return false
	}

	// Work through the array backwards.
	// We attempt to increment the cardinality of each value.
	// If we reach the cardinality limit for that key, we reset
	// to zero and continue to go backwards.
	// If we reset everything, we return false.
	for i := len(cardinality) - 1; i >= 0; i-- {
		cardinality[i]++
		if cardinality[i] < s.spec.Cardinality[i] {
			return true
		}
		cardinality[i] = 0
	}
	return false
}

func (s *Source) createSeriesKey(ctx context.Context, cardinality []int64) (flux.GroupKey, error) {
	for i, n := range cardinality {
		s.schemaArg.Set(s.spec.Keys[i], values.NewInt(n))
	}
	out, err := s.keyFn.Eval(ctx, s.keyFnInput)
	if err != nil {
		return nil, err
	}

	keyObj := out.Object()
	cols := make([]flux.ColMeta, 0, keyObj.Len())
	vs := make([]values.Value, 0, keyObj.Len())
	keyObj.Range(func(name string, v values.Value) {
		if err != nil {
			return
		}

		colType := flux.ColumnType(v.Type())
		if colType == flux.TInvalid {
			err = errors.Newf(codes.FailedPrecondition, "key value of type %s for column %s is not suitable for a column", v.Type(), name)
			return
		}
		cols = append(cols, flux.ColMeta{
			Label: name,
			Type:  colType,
		})
		vs = append(vs, v)
	})
	if err != nil {
		return nil, err
	}
	return execute.NewGroupKey(cols, vs), nil
}

func (s *Source) generateValues(ctx context.Context, key flux.GroupKey, f func(flux.Table) error) error {
	keyPropertyTypes := make([]semantic.PropertyType, len(key.Cols()))
	for i, col := range key.Cols() {
		keyPropertyTypes[i] = semantic.PropertyType{
			Key:   []byte(col.Label),
			Value: flux.SemanticType(col.Type),
		}
	}
	keyType := semantic.NewObjectType(keyPropertyTypes)
	inType := semantic.NewObjectType([]semantic.PropertyType{
		{Key: []byte("key"), Value: keyType},
		{Key: []byte("index"), Value: semantic.BasicInt},
	})
	scope := compiler.ToScope(s.spec.ValuesFn.Scope)
	fn, err := compiler.Compile(scope, s.spec.ValuesFn.Fn, inType)
	if err != nil {
		return err
	}

	// TODO(jsternberg): We can't rely on the types because the interpreter
	// can potentially muck them up before we ever get access to them.
	// At the same time, this code is good and would work much better
	// if the interpreter was refactored not to muck up the types.
	// Keeping this implementation into a commented out function.
	// cols, err := s.determineColumns(fn)
	// if err != nil {
	// 	return err
	// }

	arg0 := values.NewObject(keyType)
	for i, col := range key.Cols() {
		arg0.Set(col.Label, key.Value(i))
	}
	input := values.NewObject(inType)
	input.Set("key", arg0)

	// Initialize a shared buffer that we can use for each iteration.
	buffer := arrow.TableBuffer{
		GroupKey: key,
		// Inferring the columns ahead of time doesn't work so don't do this yet.
		// Columns:  cols,
	}

	var builders []array.Builder
	// Again, doesn't work until the interpreter sends us good data.
	// builders := make([]array.Builder, len(cols))
	// for i, col := range cols {
	// 	builders[i] = arrow.NewBuilder(col.Type, s.mem)
	// }

	b := table.NewBufferedBuilder(key, s.mem)
	for i, n := int64(0), s.spec.N; i < n; i += table.BufferSize {
		count := n - i
		if count > table.BufferSize {
			count = table.BufferSize
		}

		if err := s.generateValueBuffer(ctx, fn, &buffer, input, &builders, count); err != nil {
			return err
		}

		if err := b.AppendBuffer(&buffer); err != nil {
			return err
		}

		if err := ctx.Err(); err != nil {
			return err
		}
	}

	tbl, err := b.Table()
	if err != nil {
		return err
	}
	return f(tbl)
}

// func (s *Source) determineColumns(fn compiler.Func) ([]flux.ColMeta, error) {
// 	fnType := fn.Type()
// 	ncols, err := fnType.NumProperties()
// 	if err != nil {
// 		return nil, err
// 	}
//
// 	cols := make([]flux.ColMeta, ncols)
// 	for i := 0; i < ncols; i++ {
// 		prop, err := fnType.RecordProperty(i)
// 		if err != nil {
// 			return nil, err
// 		}
//
// 		valueType, err := prop.TypeOf()
// 		if err != nil {
// 			return nil, err
// 		}
//
// 		colType := flux.ColumnType(valueType)
// 		if colType == flux.TInvalid {
// 			return nil, errors.Newf(codes.FailedPrecondition, "column type for column %s with type %s is not suitable for a column", prop.Name(), valueType)
// 		}
// 		cols[i] = flux.ColMeta{
// 			Label: prop.Name(),
// 			Type:  colType,
// 		}
// 	}
// 	return cols, nil
// }

func (s *Source) determineColumns(record values.Object) (cols []flux.ColMeta, builders []array.Builder, err error) {
	cols = make([]flux.ColMeta, 0, record.Object().Len())
	record.Object().Range(func(name string, v values.Value) {
		if err != nil {
			return
		}

		colType := flux.ColumnType(v.Type())
		if colType == flux.TInvalid {
			err = errors.Newf(codes.FailedPrecondition, "column type for column %s with type %s is not suitable for a column", name, v.Type())
			return
		}
		cols = append(cols, flux.ColMeta{
			Label: name,
			Type:  colType,
		})
	})

	builders = make([]array.Builder, len(cols))
	for i, col := range cols {
		builders[i] = arrow.NewBuilder(col.Type, s.mem)
	}
	return cols, builders, nil
}

func (s *Source) generateValueBuffer(ctx context.Context, fn compiler.Func, buffer *arrow.TableBuffer, input values.Object, builders *[]array.Builder, n int64) error {
	key, cols := buffer.Key(), buffer.Cols()

	for i := int64(0); i < n; i++ {
		input.Set("index", values.NewInt(i))
		record, err := fn.Eval(ctx, input)
		if err != nil {
			return err
		}

		// If we were able to initialize this stuff from the type information,
		// we could move this out of the loop.
		if i == 0 {
			if cols == nil {
				cols, *builders, err = s.determineColumns(record.Object())
				if err != nil {
					return err
				}
				buffer.Columns = cols
			}

			// Builders are reused, so they will be initialized with the columns
			// and require a resize on every call to this method.
			for _, b := range *builders {
				b.Resize(int(n))
			}
		}

		record.Object().Range(func(name string, v values.Value) {
			if err != nil {
				return
			}
			if keyV := key.LabelValue(name); keyV != nil {
				if !keyV.Equal(v) {
					err = errors.Newf(codes.FailedPrecondition, "wrong value for key column %s was returned", name)
					return
				}
			}
			j := execute.ColIdx(name, cols)
			err = arrow.AppendValue((*builders)[j], v)
		})

		if err != nil {
			return err
		}
	}

	buffer.Values = make([]array.Array, len(*builders))
	for i, b := range *builders {
		buffer.Values[i] = b.NewArray()
	}
	return nil
}
