package interpreter

import (
	"context"

	bctypes "github.com/influxdata/flux/bytecode/types"
	"github.com/influxdata/flux/semantic"
	"github.com/influxdata/flux/values"
	"github.com/influxdata/flux/internal/errors"
	"github.com/influxdata/flux/codes"
)

type LoadValue struct {
	Value values.Value
}

type AppendSideEffect struct {
	Node  semantic.Node
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

	// First push the value.
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
		val, err := itrp.doStatement(ctx, stmt, scope)
		if err != nil {
			return err
		}
		if es, ok := stmt.(*semantic.ExpressionStatement); ok {
			// Only in the main package are all unassigned package
			// level expressions coerced into producing side effects.
			if itrp.pkgName == PackageMain {

				// First push the value.
				lv := LoadValue{
					Value: val,
				}
				itrp.appendCode( bctypes.IN_LOAD_VALUE, lv )

				// Add the side effect. The node is static and therefore will
				// come from the instructio's arguments. The value comes from
				// the stack.
				ase := AppendSideEffect{
					Node: es,
				}
				itrp.appendCode( bctypes.IN_APPEND_SIDE_EFFECT, ase )

				itrp.sideEffects = append(itrp.sideEffects, SideEffect{Node: es, Value: val})
			}
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

//	// Packages can import side effects
//	itrp.sideEffects = append(itrp.sideEffects, pkg.SideEffects()...)
	return nil
}

