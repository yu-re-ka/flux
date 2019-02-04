package html

import (
	"errors"
	"fmt"
	"html"

	"github.com/influxdata/flux"
	"github.com/influxdata/flux/semantic"
	"github.com/influxdata/flux/values"
)

const defaultArg = "v"

var errMissingArg = errors.New("missing argument")

func stringFunc(f func(string) string) func(args values.Object) (values.Value, error) {
	return func(args values.Object) (values.Value, error) {
		var str string
		v, ok := args.Get(defaultArg)
		if !ok {
			return nil, errMissingArg
		}

		switch v.Type().Nature() {
		case semantic.String:
			str = v.Str()
		default:
			return nil, fmt.Errorf("cannot convert %v to string", v.Type())
		}
		return values.NewString(f(str)), nil
	}
}

var stringfuncType = semantic.NewFunctionType(
	semantic.FunctionSignature{
		Parameters: map[string]semantic.Type{defaultArg: semantic.String},
		Required:   []string{defaultArg},
		Return:     semantic.String,
	})

func init() {
	// escape is a flux function that escapes html strings for UTF-8 text to make it friendly such as changing ">" into "&gt;"
	flux.RegisterPackageValue(
		"html",
		"escape",
		values.NewFunction(
			"escape",
			stringfuncType,
			stringFunc(html.EscapeString),
			false))
	// unescape is a flux function that unescapes html strings for ampersand codes into UTF-8 text.  It is the inverse of escape.
	flux.RegisterPackageValue(
		"html",
		"unescape",
		values.NewFunction(
			"unescape",
			stringfuncType,
			stringFunc(html.UnescapeString),
			false))
}
