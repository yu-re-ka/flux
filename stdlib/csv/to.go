package csv

import (
	"fmt"
	"os"

	"github.com/influxdata/flux"
	"github.com/influxdata/flux/execute"
	"github.com/influxdata/flux/plan"
	"github.com/influxdata/flux/semantic"
	CSV "github.com/influxdata/flux/csv"
)

const (
	ToCSVKind           = "toCSV"
)

func init() {
	toCSVSignature := flux.FunctionSignature(
		map[string]semantic.PolyType{
			"file":     semantic.String,
		},
		[]string{"file"},
	)

	flux.RegisterPackageValue("csv", "to", flux.FunctionValueWithSideEffect(ToCSVKind, createToCSVOpSpec, toCSVSignature))
	flux.RegisterOpSpec(ToCSVKind, func() flux.OperationSpec { return &ToCSVOpSpec{} })
	plan.RegisterProcedureSpecWithSideEffect(ToCSVKind, newToCSVProcedure, ToCSVKind)
	execute.RegisterTransformation(ToCSVKind, createToCSVTransformation)
}

// this is used so we can get better validation on marshaling, innerToCSVOpSpec and ToCSVOpSpec
// need to have identical fields
type innerToCSVOpSpec ToCSVOpSpec

type ToCSVOpSpec struct {
	File string `json:"file"`
}

func (o *ToCSVOpSpec) ReadArgs(args flux.Arguments) error {
	var err error
	o.File, err = args.GetRequiredString("file")
	if err != nil {
		return err
	}
	return err
}

func createToCSVOpSpec(args flux.Arguments, a *flux.Administration) (flux.OperationSpec, error) {
	if err := a.AddParentFromArgs(args); err != nil {
		return nil, err
	}
	s := new(ToCSVOpSpec)
	if err := s.ReadArgs(args); err != nil {
		return nil, err
	}
	return s, nil
}

func (ToCSVOpSpec) Kind() flux.OperationKind {
	return ToCSVKind
}

type ToCSVProcedureSpec struct {
	plan.DefaultCost
	Spec *ToCSVOpSpec
}

func (o *ToCSVProcedureSpec) Kind() plan.ProcedureKind {
	return ToCSVKind
}

func (o *ToCSVProcedureSpec) Copy() plan.ProcedureSpec {
	s := o.Spec
	res := &ToCSVProcedureSpec{
		Spec: &ToCSVOpSpec{
			File: s.File,
		},
	}
	return res
}

func newToCSVProcedure(qs flux.OperationSpec, a plan.Administration) (plan.ProcedureSpec, error) {
	spec, ok := qs.(*ToCSVOpSpec)
	if !ok && spec != nil {
		return nil, fmt.Errorf("invalid spec type %T", qs)
	}
	return &ToCSVProcedureSpec{Spec: spec}, nil
}

func createToCSVTransformation(id execute.DatasetID, mode execute.AccumulationMode, spec plan.ProcedureSpec, a execute.Administration) (execute.Transformation, execute.Dataset, error) {
	s, ok := spec.(*ToCSVProcedureSpec)
	if !ok {
		return nil, nil, fmt.Errorf("invalid spec type %T", spec)
	}

	file, err := os.OpenFile(s.Spec.File, os.O_CREATE | os.O_WRONLY, 0644)
	if err != nil {
		return nil, nil, err
	}

	cache := execute.NewTableBuilderCache(a.Allocator())
	d := execute.NewDataset(id, mode, cache)
	t := NewToCSVTransformation( d, cache, s, file )
	return t, d, nil
}

type Result struct {
	Tbls []flux.Table
}

func (o Result) Name() string {
	return "_result";
}

func (o Result) Tables() flux.TableIterator {
	return TableIterator{ Tables: o.Tbls }
}

type TableIterator struct {
	Tables []flux.Table
}

func (ti TableIterator) Do(f func(flux.Table) error) error {
	for _, t := range ti.Tables {
		if err := f(t); err != nil {
			return err
		}
	}
	return nil
}

type ToCSVTransformation struct {
	d       execute.Dataset
	cache   execute.TableBuilderCache
	spec    *ToCSVProcedureSpec
	file    *os.File
	tables  []flux.Table
}

func NewToCSVTransformation(d execute.Dataset, cache execute.TableBuilderCache, spec *ToCSVProcedureSpec, file *os.File ) *ToCSVTransformation {
	return &ToCSVTransformation{
		d:      d,
		cache:  cache,
		spec:   spec,
		file:   file,
		tables: make( []flux.Table, 0 ),
	}
}

func (t *ToCSVTransformation) UpdateWatermark(id execute.DatasetID, pt execute.Time) error {
	return t.d.UpdateWatermark(pt)
}

func (t *ToCSVTransformation) RetractTable(id execute.DatasetID, key flux.GroupKey) error {
	return t.d.RetractTable(key)
}

func (t *ToCSVTransformation) UpdateProcessingTime(id execute.DatasetID, pt execute.Time) error {
	return t.d.UpdateProcessingTime(pt)
}

func (t *ToCSVTransformation) Process(id execute.DatasetID, tbl flux.Table) error {
	t.tables = append( t.tables, tbl )
	return nil
}

func (t *ToCSVTransformation) Finish(id execute.DatasetID, err error) {
	result := Result{ Tbls: t.tables }

	encoder := CSV.NewResultEncoder(CSV.DefaultEncoderConfig())
	encoder.Encode(t.file, result)

	t.file.Close()
	t.d.Finish(err)
}
