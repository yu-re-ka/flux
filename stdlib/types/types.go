package types

import (
	"context"
	"fmt"

	"github.com/influxdata/flux/runtime"
	"github.com/influxdata/flux/values"
)

var of = values.NewFunction(
	"of",
	runtime.MustLookupBuiltinType("types", "of"),
	func(ctx context.Context, args values.Object) (values.Value, error) {
        arg := "a"
		v, ok := args.Get(arg)
		if !ok {
			return nil, fmt.Errorf("missing argument %q", arg)
		}

        return values.NewString(v.Type().Nature().String()), nil
	}, false,
)

func init() {
	runtime.RegisterPackageValue("types", "of", of)
}
