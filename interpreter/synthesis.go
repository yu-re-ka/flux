package interpreter

import (
	"context"

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

type ScopeSet struct {
	Name string
}

type CallOp struct {
	Call *semantic.CallExpression
	Properties []*semantic.Property
	Pipe semantic.Expression
}

type SynthesizedFunction struct {
	function
	InputScope values.Scope
	Nested values.Scope
}

type toSynthesize struct {
	F SynthesizedFunction
	Offset int
}

func (itrp *Interpreter) Code() []bctypes.OpCode {
	return itrp.code
}

func (itrp *Interpreter) appendCode( in byte, args interface{} ) {
	itrp.code = append(itrp.code, bctypes.OpCode{In: in, Args: args})
}

func (itrp *Interpreter) Synthesis(ctx context.Context, node semantic.Node, scope values.Scope, importer Importer) error {
	itrp.disableFuncSynthesis = false

	itrp.funcsToSynthesize = make([]toSynthesize, 0)

	itrp.appendCode( bctypes.IN_CONS_SIDE_EFFECTS, 0 )

	itrp.sideEffects = itrp.sideEffects[:0]
	if err := itrp.synRoot(ctx, node, scope, importer); err != nil {
		return err
	}

	itrp.appendCode( bctypes.IN_EXECUTE_FLUX, 0 )
	itrp.appendCode( bctypes.IN_STOP, 0 )

	if itrp.pkgName != PackageMain {
		return nil
	}

	// Synthesize functions. Note that this array grows as we process it, so
	// iterate it with an integer.
	for f := 0; f < len(itrp.funcsToSynthesize); f++ {
		itrp.funcsToSynthesize[f].Offset = len(itrp.code)
		nested := scope.Nest(nil)

		/* Need an appropriate scope based on the */

		for _, stmt := range itrp.funcsToSynthesize[f].F.IEEEEEE().Block.Body {
			// Making a fake scope. Shouldn't really need this, at this point.
			err := itrp.synStatement(ctx, stmt, nested);
			if err != nil {
				return err
			}
		}
		itrp.appendCode( bctypes.IN_RET, 0 )
	}

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

	itrp.appendCode( bctypes.IN_EXECUTE_FLUX, 0 )
	itrp.appendCode( bctypes.IN_STOP, 0 )
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
		err := itrp.synVariableAssignment(ctx, s, scope)
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

func (itrp *Interpreter) synVariableAssignment(ctx context.Context, dec *semantic.NativeVariableAssignment, scope values.Scope) error {
	_, err := itrp.synExpression(ctx, dec.Init, scope)
	if err != nil {
		return err
	}

	sl := ScopeSet{
		Name: dec.Identifier.Name,
	}

	itrp.appendCode( bctypes.IN_SCOPE_SET, sl )

	return nil
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
		sl := ScopeLookup{
			Name: e.Name,
		}

		itrp.appendCode( bctypes.IN_SCOPE_LOOKUP, sl )
		return nil, nil

	case *semantic.CallExpression:
		// Catching firstRecord here and generating code appropriately
		callee, ok := e.Callee.(*semantic.IdentifierExpression)
		if ok && callee.Name == "findRecord" {
			// Must be a pipe and no arguments.
			if e.Pipe == nil {
				return nil, errors.Newf(codes.Invalid,
						"findRecord must have a pipe in" )
			}

			// We need to end up with a list of side effects. To do that we
			// must create an empty list first as it needs to be deeper on the
			// stack for the append.
			itrp.appendCode( bctypes.IN_CONS_SIDE_EFFECTS, 0 )

			// Synthesize the pipe in. Output here will be a table object. want
			// to turn this into a list of side effects for the execution
			// instruction.
			_, err := itrp.synExpression(ctx, e.Pipe, scope)
			if err != nil {
				return nil, err
			}

			// Append the table object as a side effect to the above created list.
			ase := AppendSideEffect{ Node: e }
			itrp.appendCode( bctypes.IN_APPEND_SIDE_EFFECT, ase )

			// Now we can invoke execution.
			itrp.appendCode( bctypes.IN_EXECUTE_FLUX, 0 )

			// Drain the results.
			itrp.appendCode( bctypes.IN_FIND_RECORD, 0 )

			return nil, nil
		} else {
			return itrp.synCall(ctx, e, scope)
		}
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
		fv := SynthesizedFunction{
			function: function{
				e:     e,
				scope: scope,
				itrp:  itrp,
				funcIndex: len(itrp.funcsToSynthesize),
			},
		}

		if !itrp.disableFuncSynthesis {
			itrp.funcsToSynthesize = append(itrp.funcsToSynthesize, toSynthesize{F: fv})
		}

		lv := LoadValue{
			Value: fv,
		}
		itrp.appendCode( bctypes.IN_LOAD_VALUE, lv )
		return nil, nil
	default:
		return nil, errors.Newf(codes.Internal, "unsupported expression %T", expr)
	}
}

func (f SynthesizedFunction) Function() values.Function {
	return f
}

func (f SynthesizedFunction) IEEEEEE() *semantic.FunctionExpression {
	return f.e
}

func (f SynthesizedFunction) prepareScope(ctx context.Context, args Arguments, inputScope values.Scope) (values.Scope, error) {
	blockScope := inputScope.Nest(nil)
	if f.e.Parameters != nil {
	PARAMETERS:
		for _, p := range f.e.Parameters.List {
			if f.e.Defaults != nil {
				for _, d := range f.e.Defaults.Properties {
					if d.Key.Key() == p.Key.Name {
						v, ok := args.Get(p.Key.Name)
						if !ok {
							// Use default value
							var err error
							// evaluate default expressions outside the block scope
							v, err = f.itrp.doExpression(ctx, d.Value, inputScope)
							if err != nil {
								return nil, err
							}
						}
						blockScope.Set(p.Key.Name, v)
						continue PARAMETERS
					}
				}
			}
			v, err := args.GetRequired(p.Key.Name)
			if err != nil {
				return nil, err
			}
			blockScope.Set(p.Key.Name, v)
		}
	}

	// Validate the function block.
	if !isValidFunctionBlock(f.e.Block) {
		return nil, errors.New(codes.Invalid, "return statement is not the last statement in the block")
	}

	//f.Nested = blockScope.Nest(nil)

	return blockScope.Nest(nil), nil
}

func (f SynthesizedFunction) PrepCall(ctx context.Context, args values.Object, scope values.Scope) (values.Value, values.Scope, error) {
	bytecodeOffset := int64(f.itrp.funcsToSynthesize[f.funcIndex].Offset)

	argsNew := newArguments(args)
	retScope, err := f.prepareScope(ctx, argsNew, scope)
	if err != nil {
		return nil, nil, err
	}

	return values.NewInt(bytecodeOffset), retScope, nil
}


func (itrp *Interpreter) synCall(ctx context.Context, call *semantic.CallExpression, scope values.Scope) (values.Value, error) {
	_, err := itrp.synExpression(ctx, call.Callee, scope)
	if err != nil {
		return nil, err
	}

	_, _, err = itrp.synArguments(ctx, call.Arguments, scope, call.Pipe)
	if err != nil {
		return nil, err
	}

	co := CallOp{
		Call: call,
		Properties: call.Arguments.Properties,
		Pipe: call.Pipe,
	}
	itrp.appendCode( bctypes.IN_CALL, co )

	return nil, nil
}

func (itrp *Interpreter) synArguments(ctx context.Context, args *semantic.ObjectExpression, scope values.Scope, pipe semantic.Expression) (values.Object, string, error) {
	if label, nok := itrp.checkForDuplicates(args.Properties); nok {
		return nil, "", errors.Newf(codes.Invalid, "duplicate keyword parameter specified: %q", label)
	}

	if pipe == nil && (args == nil || len(args.Properties) == 0) {
		typ := semantic.NewObjectType(nil)
		return values.NewObject(typ), "", nil
	}

//	// Determine which argument matches the pipe argument.
//	var pipeArgument string
//	if pipe != nil {
//		n, err := funcType.NumArguments()
//		if err != nil {
//			return nil, "", err
//		}
//
//		for i := 0; i < n; i++ {
//			arg, err := funcType.Argument(i)
//			if err != nil {
//				return nil, "", err
//			}
//			if arg.Pipe() {
//				pipeArgument = string(arg.Name())
//				break
//			}
//		}
//
//		if pipeArgument == "" {
//			return nil, "", errors.New(codes.Invalid, "pipe parameter value provided to function with no pipe parameter defined")
//		}
//	}

	for _, p := range args.Properties {
		itrp.disableFuncSynthesis = true
		_, err := itrp.synExpression(ctx, p.Value, scope)
		if err != nil {
			return nil, "", err
		}
		itrp.disableFuncSynthesis = false

	}
	if pipe != nil {
		itrp.disableFuncSynthesis = true
		_, err := itrp.synExpression(ctx, pipe, scope)
		if err != nil {
			return nil, "", err
		}
		itrp.disableFuncSynthesis = false
	}

//	value, err := values.BuildObject(func(set values.ObjectSetter) error {
//		for _, p := range args.Properties {
//			if pipe != nil && p.Key.Key() == pipeArgument {
//				return errors.Newf(codes.Invalid, "pipe argument also specified as a keyword parameter: %q", p.Key.Key())
//			}
//			value, err := itrp.doExpression(ctx, p.Value, scope)
//			if err != nil {
//				return err
//			}
//			set(p.Key.Key(), value)
//		}
//
//		if pipe != nil {
//			value, err := itrp.doExpression(ctx, pipe, scope)
//			if err != nil {
//				return err
//			}
//			set(pipeArgument, value)
//		}
//		return nil
//	})

	return nil, "", nil
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

