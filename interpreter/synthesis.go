package interpreter

import (
	"context"
//	"fmt"

	bctypes "github.com/influxdata/flux/bytecode/types"
	"github.com/influxdata/flux/ast"
	"github.com/influxdata/flux/semantic"
	"github.com/influxdata/flux/values"
	"github.com/influxdata/flux/internal/errors"
	"github.com/influxdata/flux/codes"
)

type LoadValue struct {
	Value values.Value
}

type AppendSideEffect struct {
	Node semantic.Node
}

type ScopeLookup struct {
	Name string
}

type CallOp struct {
	Nargs int
	RetVal values.Value
}

func (itrp *Interpreter) Code() []bctypes.OpCode {
	return itrp.code
}

func (itrp *Interpreter) appendCode( in byte, args interface{} ) {
	itrp.code = append(itrp.code, bctypes.OpCode{In: in, Args: args})
}

func (itrp *Interpreter) Synthesis(ctx context.Context, node semantic.Node, scope values.Scope, importer Importer) error {
	itrp.appendCode( bctypes.IN_CONS_SIDE_EFFECTS, 0 )

	itrp.sideEffects = itrp.sideEffects[:0]
	if err := itrp.synRoot(ctx, node, scope, importer); err != nil {
		return err
	}

	itrp.appendCode( bctypes.IN_PROGRAM_START, 0 )
	return nil
}


func (itrp *Interpreter) SynthesisTo(ctx context.Context, sideEffect SideEffect) error {
	itrp.appendCode( bctypes.IN_CONS_SIDE_EFFECTS, 0 )

	lv := LoadValue{
		Value: sideEffect.Value,
	}
	itrp.appendCode( bctypes.IN_LOAD_VALUE, lv )

	ase := AppendSideEffect{
		Node: sideEffect.Node,
	}

	itrp.appendCode( bctypes.IN_APPEND_SIDE_EFFECT, ase )

	itrp.appendCode( bctypes.IN_PROGRAM_START, 0 )
	return nil
}

func (itrp *Interpreter) synRoot(ctx context.Context, node semantic.Node, scope values.Scope, importer Importer) error {
	switch n := node.(type) {
	case *semantic.Package:
		return itrp.synPackage(ctx, n, scope, importer)
	case *semantic.File:
		return itrp.synFile(ctx, n, scope, importer)
	default:
		return errors.Newf(codes.Internal, "unsupported root node %T", node)
	}
}

func (itrp *Interpreter) synPackage(ctx context.Context, pkg *semantic.Package, scope values.Scope, importer Importer) error {
	for _, file := range pkg.Files {
		if err := itrp.synFile(ctx, file, scope, importer); err != nil {
			return err
		}
	}
	return nil
}

func (itrp *Interpreter) synFile(ctx context.Context, file *semantic.File, scope values.Scope, importer Importer) error {
	if err := itrp.doPackageClause(file.Package); err != nil {
		return err
	}
	for _, i := range file.Imports {
		if err := itrp.synImport(i, scope, importer); err != nil {
			return err
		}
	}
	for _, stmt := range file.Body {
		err := itrp.synStatement(ctx, stmt, scope)
		if err != nil {
			return err
		}
	}
	return nil
}

func (itrp *Interpreter) synImport(dec *semantic.ImportDeclaration, scope values.Scope, importer Importer) error {
	path := dec.Path.Value
	pkg, err := importer.ImportPackageObject(path)
	if err != nil {
		return err
	}
	name := pkg.Name()
	if dec.As != nil {
		name = dec.As.Name
	}
	scope.Set(name, pkg)
	return nil
}

// synStatement returns the resolved value of a top-level statement
func (itrp *Interpreter) synStatement(ctx context.Context, stmt semantic.Statement, scope values.Scope) error {
	scope.SetReturn(values.InvalidValue)
	switch s := stmt.(type) {
	case *semantic.OptionStatement:
		_, err := itrp.doOptionStatement(ctx, s, scope)
		return err
	case *semantic.BuiltinStatement:
		// Nothing to do
		return nil
	case *semantic.TestStatement:
		_, err := itrp.doTestStatement(ctx, s, scope)
		return err
	case *semantic.NativeVariableAssignment:
		_, err := itrp.synVariableAssignment(ctx, s, scope)
		return err
	case *semantic.MemberAssignment:
		_, err := itrp.doMemberAssignment(ctx, s, scope)
		return err
	case *semantic.ExpressionStatement:
		v, err := itrp.synExpression(ctx, s.Expression, scope)
		if err != nil {
			return err
		}

		// Only in the main package are all unassigned package
		// level expressions coerced into producing side effects.
		if itrp.pkgName == PackageMain {
			// Add the side effect. The node is static and therefore will
			// come from the instruction's arguments. The value comes from
			// the stack.
			ase := AppendSideEffect{
				Node: s,
			}
			itrp.appendCode( bctypes.IN_APPEND_SIDE_EFFECT, ase )
		}

		scope.SetReturn(v)
		return nil
	case *semantic.ReturnStatement:
		v, err := itrp.synExpression(ctx, s.Argument, scope)
		if err != nil {
			return err
		}
		scope.SetReturn(v)
	default:
		return errors.Newf(codes.Internal, "unsupported statement type %T", stmt)
	}
	return nil
}

func (itrp *Interpreter) synVariableAssignment(ctx context.Context, dec *semantic.NativeVariableAssignment, scope values.Scope) (values.Value, error) {
	value, err := itrp.synExpression(ctx, dec.Init, scope)
	if err != nil {
		return nil, err
	}
	scope.Set(dec.Identifier.Name, value)
	return value, nil
}

func (itrp *Interpreter) synExpression(ctx context.Context, expr semantic.Expression, scope values.Scope) (ret values.Value, err error) {
	switch e := expr.(type) {
	case semantic.Literal:
		return itrp.synLiteral(e)
	case *semantic.StringExpression:
		return itrp.doStringExpression(ctx, e, scope)
	case *semantic.ArrayExpression:
		return itrp.synArray(ctx, e, scope)
	case *semantic.DictExpression:
		return itrp.doDict(ctx, e, scope)
	case *semantic.IdentifierExpression:
		value, ok := scope.Lookup(e.Name)
		if !ok {
			return nil, errors.Newf(codes.Invalid, "undefined identifier %q", e.Name)
		}

		sl := ScopeLookup{
			Name: e.Name,
		}

		itrp.appendCode( bctypes.IN_SCOPE_LOOKUP, sl )
		return value, nil

	case *semantic.CallExpression:
		return itrp.synCall(ctx, e, scope)
	case *semantic.MemberExpression:
		obj, err := itrp.synExpression(ctx, e.Object, scope)
		if err != nil {
			return nil, err
		}
		if typ := obj.Type().Nature(); typ != semantic.Object {
			return nil, errors.Newf(codes.Invalid, "cannot access property %q on value of type %s", e.Property, typ)
		}
		v, _ := obj.Object().Get(e.Property)
		if pkg, ok := v.(*Package); ok {
			// If the property of a member expression represents a package, then the object itself must be a package.
			return nil, errors.Newf(codes.Invalid, "cannot access imported package %q of imported package %q", pkg.Name(), obj.(*Package).Name())
		}
		return v, nil
	case *semantic.IndexExpression:
		arr, err := itrp.synExpression(ctx, e.Array, scope)
		if err != nil {
			return nil, err
		}
		idx, err := itrp.synExpression(ctx, e.Index, scope)
		if err != nil {
			return nil, err
		}
		ix := int(idx.Int())
		l := arr.Array().Len()
		if ix < 0 || ix >= l {
			return nil, errors.Newf(codes.Invalid, "cannot access element %v of array of length %v", ix, l)
		}
		return arr.Array().Get(ix), nil
	case *semantic.ObjectExpression:
		return itrp.doObject(ctx, e, scope)
	case *semantic.UnaryExpression:
		v, err := itrp.synExpression(ctx, e.Argument, scope)
		if err != nil {
			return nil, err
		}
		switch e.Operator {
		case ast.NotOperator:
			if v.Type().Nature() != semantic.Bool {
				return nil, errors.Newf(codes.Invalid, "operand to unary expression is not a boolean value, got %v", v.Type())
			}
			return values.NewBool(!v.Bool()), nil
		case ast.SubtractionOperator:
			switch t := v.Type().Nature(); t {
			case semantic.Int:
				return values.NewInt(-v.Int()), nil
			case semantic.Float:
				return values.NewFloat(-v.Float()), nil
			case semantic.Duration:
				return values.NewDuration(v.Duration().Mul(-1)), nil
			default:
				return nil, errors.Newf(codes.Invalid, "operand to unary expression is not a number value, got %v", v.Type())
			}
		case ast.ExistsOperator:
			return values.NewBool(!v.IsNull()), nil
		default:
			return nil, errors.Newf(codes.Invalid, "unsupported operator %q to unary expression", e.Operator)
		}
	case *semantic.BinaryExpression:
		l, err := itrp.synExpression(ctx, e.Left, scope)
		if err != nil {
			return nil, err
		}

		r, err := itrp.synExpression(ctx, e.Right, scope)
		if err != nil {
			return nil, err
		}

		bf, err := values.LookupBinaryFunction(values.BinaryFuncSignature{
			Operator: e.Operator,
			Left:     l.Type().Nature(),
			Right:    r.Type().Nature(),
		})
		if err != nil {
			return nil, err
		}
		return bf(l, r)
	case *semantic.LogicalExpression:
		l, err := itrp.synExpression(ctx, e.Left, scope)
		if err != nil {
			return nil, err
		}
		if l.Type().Nature() != semantic.Bool {
			return nil, errors.Newf(codes.Invalid, "left operand to logcial expression is not a boolean value, got %v", l.Type())
		}
		left := l.Bool()

		if e.Operator == ast.AndOperator && !left {
			// Early return
			return values.NewBool(false), nil
		} else if e.Operator == ast.OrOperator && left {
			// Early return
			return values.NewBool(true), nil
		}

		r, err := itrp.synExpression(ctx, e.Right, scope)
		if err != nil {
			return nil, err
		}
		if r.Type().Nature() != semantic.Bool {
			return nil, errors.New(codes.Invalid, "right operand to logical expression is not a boolean value")
		}
		right := r.Bool()

		switch e.Operator {
		case ast.AndOperator:
			return values.NewBool(left && right), nil
		case ast.OrOperator:
			return values.NewBool(left || right), nil
		default:
			return nil, errors.Newf(codes.Invalid, "invalid logical operator %v", e.Operator)
		}
	case *semantic.ConditionalExpression:
		t, err := itrp.synExpression(ctx, e.Test, scope)
		if err != nil {
			return nil, err
		}
		if t.Type().Nature() != semantic.Bool {
			return nil, errors.New(codes.Invalid, "conditional test expression is not a boolean value")
		}
		if t.Bool() {
			return itrp.synExpression(ctx, e.Consequent, scope)
		}
		return itrp.synExpression(ctx, e.Alternate, scope)
	case *semantic.FunctionExpression:
		// In the case of builtin functions this function value is shared across all query requests
		// and as such must NOT be a pointer value.
		fv := function{
			e:     e,
			scope: scope,
			itrp:  itrp,
		}

		lv := LoadValue{
			Value: fv,
		}
		itrp.appendCode( bctypes.IN_LOAD_VALUE, lv )
		return fv, nil
	default:
		return nil, errors.Newf(codes.Internal, "unsupported expression %T", expr)
	}
}

func (itrp *Interpreter) synCall(ctx context.Context, call *semantic.CallExpression, scope values.Scope) (values.Value, error) {
	println("-> call expression")

	callee, err := itrp.synExpression(ctx, call.Callee, scope)
	if err != nil {
		return nil, err
	}
	ft := callee.Type()
	if ft.Nature() != semantic.Function {
		return nil, errors.Newf(codes.Invalid, "cannot call function: %s: value is of type %v", call.Callee.Location(), callee.Type())
	}
	argObj, err := itrp.synArguments(ctx, call.Arguments, scope, ft, call.Pipe)
	if err != nil {
		return nil, err
	}

	f := callee.Function()

	// Check if the function is an interpFunction and rebind it.
	// This is needed so that any side effects produced when
	// calling this function are bound to the correct interpreter.
	if af, ok := f.(function); ok {
		af.itrp = itrp
		f = af
	}

	// Call the function. We attach source location information
	// to this call so it can be available for the function if needed.
	// We do not attach this source location information when evaluating
	// arguments as this source location information is only
	// for the currently called function.
	fname := functionName(call)
	ctx = withStackEntry(ctx, fname, call.Location())
	value, err := f.Call(ctx, argObj)
	if err != nil {
		return nil, errors.Wrapf(err, codes.Inherit, "error calling function %q @%s", fname, call.Location())
	}

	if f.HasSideEffect() {
		itrp.sideEffects = append(itrp.sideEffects, SideEffect{Node: call, Value: value})
	}

	nargs := len(call.Arguments.Properties)
	if call.Pipe != nil {
		nargs += 1
	}

	co := CallOp{
		Nargs: nargs,
		RetVal: value,
	}
	itrp.appendCode( bctypes.IN_CALL, co )

	return value, nil
}

func (itrp *Interpreter) synArguments(ctx context.Context, args *semantic.ObjectExpression, scope values.Scope, funcType semantic.MonoType, pipe semantic.Expression) (values.Object, error) {
	if label, nok := itrp.checkForDuplicates(args.Properties); nok {
		return nil, errors.Newf(codes.Invalid, "duplicate keyword parameter specified: %q", label)
	}

	if pipe == nil && (args == nil || len(args.Properties) == 0) {
		typ := semantic.NewObjectType(nil)
		return values.NewObject(typ), nil
	}

	// Determine which argument matches the pipe argument.
	var pipeArgument string
	if pipe != nil {
		n, err := funcType.NumArguments()
		if err != nil {
			return nil, err
		}

		for i := 0; i < n; i++ {
			arg, err := funcType.Argument(i)
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

	// fmt.Printf("synArguments: pipe is %v\n", )

	for _, p := range args.Properties {
		_, err := itrp.synExpression(ctx, p.Value, scope)
		if err != nil {
			return nil, err
		}

	}
	if pipe != nil {
		_, err := itrp.synExpression(ctx, pipe, scope)
		if err != nil {
			return nil, err
		}
	}

	return values.BuildObject(func(set values.ObjectSetter) error {
		for _, p := range args.Properties {
			if pipe != nil && p.Key.Key() == pipeArgument {
				return errors.Newf(codes.Invalid, "pipe argument also specified as a keyword parameter: %q", p.Key.Key())
			}
			value, err := itrp.doExpression(ctx, p.Value, scope)
			if err != nil {
				return err
			}
			set(p.Key.Key(), value)
		}

		if pipe != nil {
			value, err := itrp.doExpression(ctx, pipe, scope)
			if err != nil {
				return err
			}
			set(pipeArgument, value)
		}
		return nil
	})
}

func (itrp *Interpreter) synLiteral(lit semantic.Literal) (values.Value, error) {
	value, err := itrp.doLiteral(lit)
	if err != nil {
		return nil, err
	}

	lv := LoadValue{
		Value: value,
	}
	itrp.appendCode( bctypes.IN_LOAD_VALUE, lv )

	return value, nil
}

func (itrp *Interpreter) synArray(ctx context.Context, a *semantic.ArrayExpression, scope values.Scope) (values.Value, error) {
	elements := make([]values.Value, len(a.Elements))

	for i, el := range a.Elements {
		v, err := itrp.doExpression(ctx, el, scope)
		if err != nil {
			return nil, err
		}
		elements[i] = v
	}
	value := values.NewArrayWithBacking(a.TypeOf(), elements)

	lv := LoadValue{
		Value: value,
	}
	itrp.appendCode( bctypes.IN_LOAD_VALUE, lv )

	return value, nil
}

