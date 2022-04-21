package influxdb

import (
	"github.com/influxdata/flux"
	"github.com/influxdata/flux/codes"
	"github.com/influxdata/flux/internal/errors"
	"github.com/influxdata/flux/runtime"
	"github.com/influxdata/flux/stdlib/influxdata/influxdb"
)

func init() {
	fromSignature := runtime.MustLookupBuiltinType("experimental/influxdb", "preview")

	runtime.RegisterPackageValue("experimental/influxdb", "preview", flux.MustValue(flux.FunctionValue("preview", createPreviewOpSpec, fromSignature)))
}

func createPreviewOpSpec(args flux.Arguments, a *flux.Administration) (flux.OperationSpec, error) {
	spec := new(influxdb.FromOpSpec)

	if b, ok, err := influxdb.GetNameOrID(args, "bucket", "bucketID"); err != nil {
		return nil, err
	} else if !ok {
		return nil, errors.New(codes.Invalid, "must specify bucket or bucketID")
	} else {
		spec.Bucket = b
	}

	if o, ok, err := influxdb.GetNameOrID(args, "org", "orgID"); err != nil {
		return nil, err
	} else if ok {
		spec.Org = &o
	}

	if h, ok, err := args.GetString("host"); err != nil {
		return nil, err
	} else if ok {
		spec.Host = &h
	}

	if token, ok, err := args.GetString("token"); err != nil {
		return nil, err
	} else if ok {
		spec.Token = &token
	}
	spec.Preview = true
	return spec, nil
}
