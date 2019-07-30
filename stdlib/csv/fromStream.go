package csv

import (
	"fmt"
	"io"
	"io/ioutil"

	"github.com/influxdata/flux"
	"github.com/influxdata/flux/execute"
	"github.com/influxdata/flux/plan"
	"github.com/influxdata/flux/semantic"
	"github.com/pkg/errors"
)

const fromCSVStreamKind = "fromCSVStream"

type fromCSVStreamOpSpec struct {
	CSV string `json:"csv"`
}

func init() {
	fromCSVStreamSignature := semantic.FunctionPolySignature{
		Parameters: map[string]semantic.PolyType{
			"data": semantic.Stream,
		},
		Required: nil,
		Return:   flux.TableObjectType,
	}
	flux.RegisterPackageValue("csv", "fromStream", flux.FunctionValue(fromCSVStreamKind, createfromCSVStreamOpSpec, fromCSVStreamSignature))
	flux.RegisterOpSpec(fromCSVStreamKind, newfromCSVStreamOp)
	plan.RegisterProcedureSpec(fromCSVStreamKind, newfromCSVStreamProcedure, fromCSVStreamKind)
	execute.RegisterSource(fromCSVStreamKind, createfromCSVStreamSource)
}

func createfromCSVStreamOpSpec(args flux.Arguments, a *flux.Administration) (flux.OperationSpec, error) {
	spec := new(fromCSVStreamOpSpec)

	if stream, ok := args.Get("data"); ok {
		if stream.Type().Nature() == semantic.Stream {
			s, _ := stream.(io.Reader)
			bytes, err := ioutil.ReadAll(s)
			if err != nil {
				return nil, err
			}
			spec.CSV = string(bytes)
		} else {
			return nil, errors.New("data argument must be a read stream.   ")
		}

	} else {
		return nil, errors.New("required argument data not found")
	}

	return spec, nil
}

func newfromCSVStreamOp() flux.OperationSpec {
	return new(fromCSVStreamOpSpec)
}

func (s *fromCSVStreamOpSpec) Kind() flux.OperationKind {
	return fromCSVStreamKind
}

type fromCSVStreamProcedureSpec struct {
	plan.DefaultCost
	CSV string
}

func newfromCSVStreamProcedure(qs flux.OperationSpec, pa plan.Administration) (plan.ProcedureSpec, error) {
	spec, ok := qs.(*fromCSVStreamOpSpec)
	if !ok {
		return nil, fmt.Errorf("invalid spec type %T", qs)
	}

	return &fromCSVStreamProcedureSpec{
		CSV: spec.CSV,
	}, nil
}

func (s *fromCSVStreamProcedureSpec) Kind() plan.ProcedureKind {
	return fromCSVStreamKind
}

func (s *fromCSVStreamProcedureSpec) Copy() plan.ProcedureSpec {
	ns := new(fromCSVStreamProcedureSpec)
	ns.CSV = s.CSV
	return ns
}

func createfromCSVStreamSource(prSpec plan.ProcedureSpec, dsid execute.DatasetID, a execute.Administration) (execute.Source, error) {
	spec, ok := prSpec.(*fromCSVStreamProcedureSpec)
	if !ok {
		return nil, fmt.Errorf("invalid spec type %T", prSpec)
	}

	csvText := spec.CSV
	// if spec.File non-empty then spec.CSV is empty

	csvStreamSource := CSVSource{id: dsid, tx: csvText}

	return &csvStreamSource, nil
}
