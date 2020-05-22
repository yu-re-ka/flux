package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/influxdata/flux/ast"
	_ "github.com/influxdata/flux/builtin"
	"github.com/influxdata/flux/codes"
	"github.com/influxdata/flux/internal/errors"
	"github.com/influxdata/flux/interpreter"
	"github.com/influxdata/flux/libflux/go/libflux"
	"github.com/influxdata/flux/runtime"
	"github.com/influxdata/flux/semantic"
	"github.com/influxdata/flux/values"
	"github.com/spf13/cobra"
)

var cmd = &cobra.Command{
	Use:  "flux",
	Args: cobra.MinimumNArgs(1),
	RunE: runE,
}

type packageLoader struct {
	main      *libflux.ASTPkg
	imports   map[string]*libflux.ASTPkg
	loadOrder []string
}

func (pl *packageLoader) LoadImportPath(path string) error {
	if _, ok := pl.imports[path]; ok {
		return nil
	}

	dirpath := filepath.Join("stdlib", path)
	files, err := ioutil.ReadDir(dirpath)
	if err != nil {
		return err
	}

	var fname string
	for _, f := range files {
		if strings.HasSuffix(f.Name(), "_test.flux") {
			continue
		} else if strings.HasSuffix(f.Name(), ".flux") {
			if fname != "" {
				return errors.New(codes.Invalid, "multiple flux files in this package")
			}
			fname = filepath.Join(dirpath, f.Name())
		}
	}

	if fname == "" {
		return errors.New(codes.Invalid, "no flux files in this path")
	}

	src, err := ioutil.ReadFile(fname)
	if err != nil {
		return err
	}
	astPkg := libflux.Parse(fname, string(src))
	if pl.imports == nil {
		pl.imports = make(map[string]*libflux.ASTPkg)
	}
	pl.imports[path] = astPkg
	pl.loadOrder = append(pl.loadOrder, path)
	return pl.LoadImportsFor(astPkg)
}

func (pl *packageLoader) LoadImportsFor(pkg *libflux.ASTPkg) error {
	data, err := pkg.MarshalJSON()
	if err != nil {
		return err
	}

	var astPkg ast.Package
	if err := json.Unmarshal(data, &astPkg); err != nil {
		return err
	}

	for _, f := range astPkg.Files {
		for _, imp := range f.Imports {
			if err := pl.LoadImportPath(imp.Path.Value); err != nil {
				return err
			}
		}
	}
	return nil
}

var prelude = []string{
	"universe",
	"influxdata/influxdb",
}

func (pl *packageLoader) LoadPrelude() error {
	for _, path := range prelude {
		if err := pl.LoadImportPath(path); err != nil {
			return err
		}
	}
	return nil
}

func (pl *packageLoader) LoadMainPackage(fname string) error {
	preludeOrder := pl.loadOrder
	pl.loadOrder = nil

	src, err := ioutil.ReadFile(fname)
	if err != nil {
		return err
	}

	astPkg := libflux.Parse(fname, string(src))
	if err := pl.LoadImportsFor(astPkg); err != nil {
		return err
	}
	pl.loadOrder = append(pl.loadOrder, preludeOrder...)
	pl.main = astPkg
	return nil
}

func (pl *packageLoader) EvalImports() (*libflux.SemanticPkgSet, error) {
	pkgset := libflux.NewSemanticPkgSet()
	for i := len(pl.loadOrder) - 1; i >= 0; i-- {
		path := pl.loadOrder[i]
		pkg, err := libflux.AnalyzePackage(path, pl.imports[path], pkgset)
		if err != nil {
			return nil, err
		}

		if err := pkgset.Add(pkg); err != nil {
			return nil, err
		}
	}
	return pkgset, nil
}

func (pl *packageLoader) Eval(ctx context.Context, imports *libflux.SemanticPkgSet) ([]interpreter.SideEffect, values.Scope, error) {
	pkg, err := libflux.AnalyzePackage("main", pl.main, imports)
	if err != nil {
		return nil, nil, err
	}

	data, err := pkg.MarshalFB()
	if err != nil {
		return nil, nil, err
	}

	semPkg, err := semantic.DeserializeFromFlatBuffer(data)
	if err != nil {
		return nil, nil, err
	}

	imp := &importer{imports: imports}
	scope, err := runtime.Default.NewScopeFor("main", imp)
	if err != nil {
		return nil, nil, err
	}

	itrp := interpreter.NewInterpreter(nil)
	sideEffects, err := itrp.Eval(ctx, semPkg, scope, imp)
	if err != nil {
		return nil, nil, err
	}
	return sideEffects, scope, nil
}

type importer struct {
	imports *libflux.SemanticPkgSet
	pkgs    map[string]*interpreter.Package
}

func (imp *importer) Import(path string) (semantic.MonoType, error) {
	p, err := imp.ImportPackageObject(path)
	if err != nil {
		return semantic.MonoType{}, err
	}
	return p.Type(), nil
}

func (imp *importer) ImportPackageObject(path string) (*interpreter.Package, error) {
	// If this package has been imported previously, return the import now.
	if p, ok := imp.pkgs[path]; ok {
		if p == nil {
			return nil, errors.Newf(codes.Invalid, "detected cyclical import for package path %q", path)
		}
		return p, nil
	}

	// Mark down that we are currently evaluating this package
	// so that we can detect a circular import.
	if imp.pkgs == nil {
		imp.pkgs = make(map[string]*interpreter.Package)
	}
	imp.pkgs[path] = nil

	// If this package is part of the prelude, fill in a fake
	// empty package to resolve cyclical imports.
	for _, ppath := range []string{"universe", "influxdata/influxdb"} {
		if ppath == path {
			imp.pkgs[path] = interpreter.NewPackage(path)
			break
		}
	}

	// Find the package for the given import path.
	semPkgHandle, ok := imp.imports.Get(path)
	if !ok {
		return nil, errors.Newf(codes.Invalid, "invalid import path %s", path)
	}

	data, err := semPkgHandle.MarshalFB()
	if err != nil {
		return nil, err
	}

	semPkg, err := semantic.DeserializeFromFlatBuffer(data)
	if err != nil {
		return nil, err
	}

	// Construct the prelude scope from the prelude paths.
	// If we are importing part of the prelude, we do not
	// include it as part of the prelude and will stop
	// including values as soon as we hit the prelude.
	// This allows us to import all previous paths when loading
	// the prelude, but avoid a circular import.
	scope, err := runtime.Default.NewScopeFor(path, imp)
	if err != nil {
		return nil, err
	}

	// Run the interpreter on the package to construct the values
	// created by the package. Pass in the previously initialized
	// packages as importable packages as we evaluate these in order.
	itrp := interpreter.NewInterpreter(nil)
	if _, err := itrp.Eval(context.Background(), semPkg, scope, imp); err != nil {
		return nil, err
	}
	obj := newObjectFromScope(scope)
	imp.pkgs[path] = interpreter.NewPackageWithValues(itrp.PackageName(), path, obj)
	return imp.pkgs[path], nil
}

// func loadPackage(path string) error {
// 	dirpath := filepath.Join("stdlib", path)
// }
//
// func loadMainPackage(fname string) (*Package, error) {
// 	src, err := ioutil.ReadFile(fname)
// 	if err != nil {
// 		return nil, err
// 	}
//
// 	ast := libflux.Parse(fname, string(src))
// 	pkg := &Package{
// 		main: ast,
// 	}
// }

func runE(cmd *cobra.Command, args []string) error {
	pl := &packageLoader{}
	if err := pl.LoadPrelude(); err != nil {
		return err
	}
	if err := pl.LoadMainPackage(args[0]); err != nil {
		return err
	}

	imports, err := pl.EvalImports()
	if err != nil {
		return err
	}

	sideEffects, scope, err := pl.Eval(context.Background(), imports)
	if err != nil {
		return err
	}
	fmt.Println(sideEffects)
	fmt.Println(scope.Return())
	return nil
}

func newObjectFromScope(scope values.Scope) values.Object {
	obj, _ := values.BuildObject(func(set values.ObjectSetter) error {
		scope.LocalRange(func(k string, v values.Value) {
			// Packages should not expose the packages they import.
			if _, ok := v.(values.Package); ok {
				return
			}
			set(k, v)
		})
		return nil
	})
	return obj
}

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
