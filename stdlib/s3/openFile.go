package s3

import (
	"fmt"

	"github.com/influxdata/flux"
	"github.com/influxdata/flux/semantic"
	"github.com/influxdata/flux/values"
)

const (
	akid      = "akid"
	secretKey = "secretKey"
	bucket    = "bucket"
	region    = "region"
	filepath  = "filepath"
	mode      = "mode"
)

var openFile = values.NewFunction(
	"openFile",
	semantic.NewFunctionPolyType(semantic.FunctionPolySignature{
		Parameters: map[string]semantic.PolyType{akid: semantic.String, secretKey: semantic.String,
			region: semantic.String, bucket: semantic.String, filepath: semantic.String, mode: semantic.String},
		Required: semantic.LabelSet{akid, secretKey, region, bucket, filepath, mode},
		Return:   semantic.Stream,
	}),
	func(args values.Object) (values.Value, error) {
		id, ok := args.Get("akid")
		if !ok {
			return nil, fmt.Errorf("missing argument %q", id)
		}

		sk, ok := args.Get("secretKey")
		if !ok {
			return nil, fmt.Errorf("missing argument %q", sk)
		}

		r, ok := args.Get("region")
		if !ok {
			return nil, fmt.Errorf("missing argument %q", r)
		}

		b, ok := args.Get("bucket")
		if !ok {
			return nil, fmt.Errorf("missing argument %q", b)
		}

		path, ok := args.Get("filepath")
		if !ok {
			return nil, fmt.Errorf("missing argument %q", path)
		}

		mode, ok := args.Get("mode")
		if !ok {
			return nil, fmt.Errorf("missing argument %q", mode)
		}

		if id.Type().Nature() == semantic.String && sk.Type().Nature() == semantic.String &&
			r.Type().Nature() == semantic.String && b.Type().Nature() == semantic.String && path.Type().Nature() == semantic.String {

			rws := NewReadWriteSeeker(id.Str(), sk.Str(), r.Str(), b.Str(), path.Str())
			if err := rws.Open(); err != nil {
				return nil, err
			}

			switch mode.Str() {
			case values.ReadOnly:
				return values.NewReadStream(&rws), nil
			case values.ReadSeek:
				return values.NewReadSeekStream(&rws), nil
			case values.WriteOnly:
				return values.NewWriteStream(&rws), nil
			}
		}

		return nil, fmt.Errorf("cannot open file")
	}, false,
)

func init() {
	flux.RegisterPackageValue("s3", "openFile", openFile)
}
