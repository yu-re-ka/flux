package complete

import (
	"errors"
	"sort"

	"github.com/influxdata/flux/parser"
	"github.com/influxdata/flux/semantic"
)

func StaticComplete(fluxScript string) ([]string, error) {
	tree := parser.ParseSource(fluxScript)
	semTree, err := semantic.New(tree)
	if err != nil {
		return nil, err
	}
	typeSol, err := semantic.InferTypes(semTree, nil)
	if err != nil {
		return nil, err
	}
	var a semantic.Node
	semantic.Walk(semantic.CreateVisitor(func(n semantic.Node) {
		if va, ok := n.(*semantic.NativeVariableAssignment); ok {
			if va.Identifier.Name == "a" {
				a = va.Init
			}
		}
	}), semTree)
	if a == nil {
		return nil, errors.New("a not found")
	}
	typ, err := typeSol.TypeOf(a)
	if err != nil {
		return nil, err
	}
	props := typ.Properties()
	list := make([]string, 0, len(props))
	for p := range props {
		list = append(list, p)
	}
	sort.Strings(list)
	return list, nil
}
