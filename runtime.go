package flux

import (
	"context"
	"fmt"
	flatbuffers "github.com/google/flatbuffers/go"

	"github.com/influxdata/flux/ast"
	"github.com/influxdata/flux/codes"
	"github.com/influxdata/flux/internal/errors"
	"github.com/influxdata/flux/internal/fbsemantic"
	"github.com/influxdata/flux/interpreter"
	"github.com/influxdata/flux/libflux/go/libflux"
	"github.com/influxdata/flux/parser"
	"github.com/influxdata/flux/semantic"
	"github.com/influxdata/flux/values"
)

// defaultRuntime contains the preregistered packages and builtin values
// required to execute a flux script.
var defaultRuntime = &runtime{}

// stdlibTypeEnvironment is where the type environment will be stored for lookup
var stdlibTypeEnvironment = TypeEnvMap(fbsemantic.GetRootAsTypeEnvironment(libflux.EnvStdlib(), 0))


// runtime contains the flux runtime for interpreting and
// executing queries.
type runtime struct {
	pkgs      map[string]*semantic.Package
	builtins  map[string]map[string]values.Value
	finalized bool
}

func (r *runtime) RegisterPackage(pkg *ast.Package) error {
	if r.finalized {
		return errors.New(codes.Internal, "already finalized, cannot register builtin package")
	}

	if r.pkgs == nil {
		r.pkgs = make(map[string]*semantic.Package)
	}

	if _, ok := r.pkgs[pkg.Path]; ok {
		return errors.Newf(codes.Internal, "duplicate builtin package %q", pkg.Path)
	}

	if ast.Check(pkg) > 0 {
		err := ast.GetError(pkg)
		return errors.Wrapf(err, codes.Inherit, "failed to parse builtin package %q", pkg.Path)
	}

	ap, err := parser.ToHandle(pkg)
	if err != nil {
		return err
	}

	root, err := AnalyzePackage(ap)
	if err != nil {
		return err
	}
	r.pkgs[pkg.Path] = root
	return nil
}

func (r *runtime) RegisterPackageValue(pkgpath, name string, value values.Value) error {
	return r.registerPackageValue(pkgpath, name, value, false)
}

func (r *runtime) ReplacePackageValue(pkgpath, name string, value values.Value) error {
	return r.registerPackageValue(pkgpath, name, value, true)
}

func (r *runtime) registerPackageValue(pkgpath, name string, value values.Value, replace bool) error {
	if r.finalized {
		return errors.Newf(codes.Internal, "already finalized, cannot register builtin package value")
	}

	if r.builtins == nil {
		r.builtins = make(map[string]map[string]values.Value)
	}

	pkg, ok := r.builtins[pkgpath]
	if !ok {
		pkg = make(map[string]values.Value)
		r.builtins[pkgpath] = pkg
	}

	if _, ok := pkg[name]; ok && !replace {
		return errors.Newf(codes.Internal, "duplicate builtin package value %q %q", pkgpath, name)
	} else if !ok && replace {
		return errors.Newf(codes.Internal, "missing builtin package value %q %q", pkgpath, name)
	}
	pkg[name] = value
	return nil
}

func (r *runtime) Prelude() values.Scope {
	if !r.finalized {
		panic("builtins not finalized")
	}
	importer := StdLib()
	scope, err := r.newScopeFor("main", importer)
	if err != nil {
		panic(err)
	}
	return scope
}

func (r *runtime) Eval(ctx context.Context, astPkg *ast.Package, opts ...ScopeMutator) ([]interpreter.SideEffect, values.Scope, error) {
	h, err := parser.ToHandle(astPkg)
	if err != nil {
		return nil, nil, err
	}
	return r.evalHandle(ctx, h, opts...)
}

func (r *runtime) evalHandle(ctx context.Context, h *libflux.ASTPkg, opts ...ScopeMutator) ([]interpreter.SideEffect, values.Scope, error) {
	semPkg, err := AnalyzePackage(h)
	if err != nil {
		return nil, nil, err
	}

	// Construct the initial scope for this package.
	importer := &importer{r: r}
	scope, err := r.newScopeFor("main", importer)
	if err != nil {
		return nil, nil, err
	}

	// Mutate the scope with any additional options.
	for _, opt := range opts {
		opt(scope)
	}

	// Execute the interpreter over the package.
	itrp := interpreter.NewInterpreter(nil)
	sideEffects, err := itrp.Eval(ctx, semPkg, scope, importer)
	if err != nil {
		return nil, nil, err
	}
	return sideEffects, scope, nil
}

// newScopeFor constructs a new scope for the given package using the
// passed in importer.
func (r *runtime) newScopeFor(pkgpath string, imp interpreter.Importer) (values.Scope, error) {
	// Construct the prelude scope from the prelude paths.
	// If we are importing part of the prelude, we do not
	// include it as part of the prelude and will stop
	// including values as soon as we hit the prelude.
	// This allows us to import all previous paths when loading
	// the prelude, but avoid a circular import.
	preludeScope := values.NewScope()
	for _, path := range prelude {
		if path == pkgpath {
			break
		}

		p, err := imp.ImportPackageObject(path)
		if err != nil {
			return nil, err
		}
		p.Range(preludeScope.Set)
	}

	// Build an object with the initial set of identifiers
	// from the known builtin values.
	object := values.NewObjectWithValues(r.builtins[pkgpath])
	scope := values.NewNestedScope(preludeScope, object)
	return scope, nil
}

func (r *runtime) Stdlib() interpreter.Importer {
	if !r.finalized {
		panic("builtins not finalized")
	}
	return &importer{r: r}
}

func (r *runtime) Finalize() error {
	if r.finalized {
		return errors.New(codes.Internal, "already finalized")
	}
	r.finalized = true
	// TODO(algow): Should we bother with any validations?
	// The only one we're missing is validating that all of the referenced
	// builtins are included and that all registered builtins are referenced,
	// but we don't actually execute anything until we evaluate a script.
	return nil
}



type envKey struct {
	Package string
	Prop    string
}

// LookupBuiltinType returns the type of the builtin value for a given
// Flux stdlib package. Returns an error if lookup fails.
func LookupBuiltinType(pkg, name string) (semantic.MonoType, error) {
	key := envKey{
		Package: pkg,
		Prop:    name,
	}
	prop, ok := stdlibTypeEnvironment[key]
	if !ok {
		return semantic.MonoType{}, errors.Newf(codes.Internal, "Expected to find Prop for %v %v, but Prop was missing.", pkg, name)
	}
	var table flatbuffers.Table
	if !prop.V(&table) {
		return semantic.MonoType{}, errors.Newf(codes.Internal, "Prop value is not valid: pkg %v name %v", pkg, name)
	}
	monotype, err := semantic.NewMonoType(table, prop.VType())
	if err != nil {
		return semantic.MonoType{}, err
	}
	// return fb polytype within semantic wrapper
	return monotype, nil
}

// MustLookupBuiltinType validates that call to LookupBuiltInType was
// successful. If there is an error with lookup, then panic.
func MustLookupBuiltinType(pkg, name string) semantic.MonoType {
	mt, err := LookupBuiltinType(pkg, name)
	if err != nil {
		panic(err)
	}
	return mt
}

// TypeEnvMap creates a global map of the TypeEnvironment
func TypeEnvMap(env *fbsemantic.TypeEnvironment) map[envKey]*fbsemantic.Prop {
	envMap := make(map[envKey]*fbsemantic.Prop)
	var table flatbuffers.Table
	l := env.AssignmentsLength()

	for i := 0; i < l; i++ {
		newAssign := new(fbsemantic.TypeAssignment)
		_ = env.Assignments(newAssign, i) // this call assigns a value to newAssign
		assignId := string(newAssign.Id())
		polytype := newAssign.Ty(nil)
		if polytype.ExprType() != fbsemantic.MonoTypeRow {
			panic(fmt.Errorf("expected PolyType Expr of %v to be fbsemantic.MonoTypeRow; found fbsemantic.%v",
				assignId,
				fbsemantic.EnumNamesMonoType[polytype.ExprType()],
			))
		}
		if !polytype.Expr(&table) {
			panic(fmt.Errorf(
				"PolyType does not have a MonoType; something went wrong. Assignment: %v MonoType: %v",
				assignId,
				fbsemantic.EnumNamesMonoType[polytype.ExprType()],
			))
		}

		// initialize table before use in row
		row := new(fbsemantic.Row)
		row.Init(table.Bytes, table.Pos)
		propLen := row.PropsLength()

		for j := 0; j < propLen; j++ {
			newProp := new(fbsemantic.Prop)
			_ = row.Props(newProp, j) // this call assigns value to newProp
			propKey := string(newProp.K())
			key := envKey{
				Package: assignId,
				Prop:    propKey,
			}
			envMap[key] = newProp
		}

	}
	return envMap
}

// AnalyzeSource parses and analyzes the given Flux source,
// using libflux.
func AnalyzeSource(fluxSrc string) (*semantic.Package, error) {
	ast := libflux.Parse(fluxSrc)
	return AnalyzePackage(ast)
}

func AnalyzePackage(astPkg *libflux.ASTPkg) (*semantic.Package, error) {
	defer astPkg.Free()
	sem, err := libflux.Analyze(astPkg)
	if err != nil {
		return nil, err
	}
	defer sem.Free()
	bs, err := sem.MarshalFB()
	if err != nil {
		return nil, err
	}
	return semantic.DeserializeFromFlatBuffer(bs)
}
