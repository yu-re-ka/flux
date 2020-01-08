package semantic

import (
	flatbuffers "github.com/google/flatbuffers/go"
	"github.com/influxdata/flux/codes"
	"github.com/influxdata/flux/internal/errors"
	"github.com/influxdata/flux/libflux/go/libflux"
	"github.com/influxdata/flux/semantic/internal/fbsemantic"
)

// LookupBuiltInType returns the type of the builtin value for a given Flux package.
// pkg is path
// name is the func name
func LookupBuiltInType(pkg, name string) MonoType {
	byteArr := libflux.EnvStdlib()

	env := fbsemantic.GetRootAsTypeEnvironment(byteArr, 0)

	table := new(flatbuffers.Table)
	// perform a lookup on flatbuffers TypeEnvironment
	prop, err := lookup(env, pkg, name)
	if err != nil {
		return NewMonoType(table, fbsemantic.MonoTypeNONE)
	}

	// create a new MonoType wrapper for flatbuffers MonoType

	monotype, err := NewMonoType(table, prop.V(table))
	if err != nil {
		return fbsemantic.MonoTypeNONE
	}

	// return fb polytype within semantic wrapper
	return monotype
}

// lookup is a helper function that performs a lookup of a package identifier in
// a flatbuffers TypeEnvironment. It first checks for the package using path string
// and then checks for the package identifier, the "name" string in this case, returning
// the corresponding MonoType if found
func lookup(env *fbsemantic.TypeEnvironment, pkg, name string) (*fbsemantic.Prop, error) {
	// check for package
	typeAssign := new(fbsemantic.TypeAssignment)
	if pkgErr := foundPackage(env, typeAssign, pkg); pkgErr != nil {
		return nil, pkgErr
	}

	polytype := typeAssign.Ty(nil)
	// grab PolyType expr
	if polytype.ExprType() != fbsemantic.Row {
		// expr is monotype of type row
		return nil, errors.Newf(codes.Internal, "")
	}

	// create row type
	table := new(flatbuffers.Table)
	if !polytype.Expr(table) {
		return nil, errors.Newf(codes.Internal, "")
	}
	row := new(fbsemantic.MonoTypeRow)
	row.Init(table.Bytes, table.Pos)

	// check for package identifier in row props
	prop := new(fbsemantic.Prop)
	if propErr := foundProp(row, prop, name); propErr != nil {
		return nil, propErr
	}

	return prop, nil
}

// foundPackage is a helper function that iterates over type assignments and checks
// for a given package returning an error if the package is not found
func foundPackage(env *fbsemantic.TypeEnvironment, obj *fbsemantic.TypeAssignment, pkg string) error {
	l := env.AssignmentsLength()
	for i := 0; i < l; i++ {
		if !env.Assignments(obj, i) {
			return errors.Newf(codes.Internal, "", i)
		} else {
			if string(obj.Id()) == pkg {
				return nil
			}
		}
	}
	return errors.Newf(codes.Internal, "")
}

// foundProp is a helper function that iterates over row properties and checks
// for a given package identifier, returning an error if the identifier is not found
func foundProp(row *fbsemantic.Row, obj *fbsemantic.Prop, name string) error {
	l := row.PropsLength()
	for i := 0; i < l; i++ {
		if !row.Props(obj, i) {
			return errors.Newf(codes.Internal, "", i)
		} else {
			if string(obj.Id()) == name {
				return nil
			}
		}
	}
	return errors.Newf(codes.Internal, "")
}
