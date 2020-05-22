package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/influxdata/flux/ast"
	"github.com/influxdata/flux/codes"
	"github.com/influxdata/flux/internal/errors"
	"github.com/influxdata/flux/libflux/go/libflux"
	"github.com/spf13/cobra"
)

var cmd = &cobra.Command{
	Use:  "flux",
	Args: cobra.MinimumNArgs(1),
	RunE: runE,
}

// func getImports(pkg *libflux.ASTPkg)

type Package struct {
	main    *libflux.ASTPkg
	imports []*libflux.ASTPkg
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

func (pl *packageLoader) LoadPrelude() error {
	return pl.LoadImportPath("universe")
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
	return nil
}

func (pl *packageLoader) EvalImports() (*libflux.SemanticPkgSet, error) {
	pkgset := libflux.NewSemanticPkgSet()
	for i := len(pl.loadOrder) - 1; i >= 0; i-- {
		path := pl.loadOrder[i]
		pkg, err := libflux.AnalyzePackage(path, pl.imports[path])
		if err != nil {
			return nil, err
		}

		if err := pkgset.Add(pkg); err != nil {
			return nil, err
		}
	}
	return pkgset, nil
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
	fmt.Println(pl.loadOrder)
	if err := pl.LoadMainPackage(args[0]); err != nil {
		return err
	}
	fmt.Println(pl.loadOrder)

	imports, err := pl.EvalImports()
	if err != nil {
		return err
	}
	imports.Free()
	return nil
}

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
