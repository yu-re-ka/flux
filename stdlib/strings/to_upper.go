package strings

import (
	"fmt"
	"strings"

	"github.com/influxdata/flux"
	"github.com/influxdata/flux/semantic"
	"github.com/influxdata/flux/values"
)

func init() {
	flux.RegisterPackageValue("strings", "toUpper", values.NewFunction(
		"toUpper",
		semantic.NewFunctionType(semantic.FunctionSignature{
			Parameters: map[string]semantic.Type{stringArg: semantic.String},
			Required:   semantic.LabelSet{stringArg},
			Return:     semantic.String,
		}),
		func(args values.Object) (values.Value, error) {
			var str string

			v, ok := args.Get(stringArg)
			if !ok {
				return nil, errMissingV
			}

			if v.Type().Nature() == semantic.String {
				str = v.Str()

				str = strings.ToUpper(str)
				return values.NewString(str), nil
			}

			return nil, fmt.Errorf("cannot convert argument of type %v to upper case", v.Type().Nature())
		},
		false,
	))
}
