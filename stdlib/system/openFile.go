package system

import (
	"fmt"
	"github.com/influxdata/flux"
	"github.com/influxdata/flux/semantic"
	"github.com/influxdata/flux/values"
	"os"
)

const (
	filepath = "filepath"
	mode     = "mode"
)

var openFile = values.NewFunction(
	"openFile",
	semantic.NewFunctionPolyType(semantic.FunctionPolySignature{
		Parameters: map[string]semantic.PolyType{filepath: semantic.String, mode: semantic.String},
		Required:   semantic.LabelSet{filepath, mode},
		Return:     semantic.Stream,
	}),
	func(args values.Object) (values.Value, error) {
		path, ok := args.Get("filepath")
		if !ok {
			return nil, fmt.Errorf("missing argument path")
		}

		mode, ok := args.Get("mode")
		if !ok {
			return nil, fmt.Errorf("missing argument mode")
		}

		if path.Type().Nature() == semantic.String {

			switch mode.Str() {
			case values.ReadOnly:
				rs, err := os.Open(path.Str())
				if err != nil {
					return nil, err
				}
				return values.NewReadStream(rs), nil
			case values.ReadSeek:
				rs, err := os.Open(path.Str())
				if err != nil {
					return nil, err
				}
				return values.NewReadSeekStream(rs), nil
			case values.WriteOnly:
				rs, err := os.Create(path.Str())
				if err != nil {
					return nil, err
				}
				return values.NewWriteStream(rs), nil
			}
		}

		return nil, fmt.Errorf("cannot open file")
	}, false,
)

func init() {
	flux.RegisterPackageValue("system", "openFile", openFile)
}
