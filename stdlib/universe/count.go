package universe

import (
	"github.com/apache/arrow/go/arrow/memory"
	"github.com/influxdata/flux"
	"github.com/influxdata/flux/arrow"
	"github.com/influxdata/flux/codes"
	"github.com/influxdata/flux/execute"
	"github.com/influxdata/flux/internal/errors"
	"github.com/influxdata/flux/internal/execute/executekit"
	"github.com/influxdata/flux/internal/execute/function"
	"github.com/influxdata/flux/internal/execute/table"
	"github.com/influxdata/flux/runtime"
)

const CountKind = "count"

type CountOpSpec struct {
	execute.AggregateConfig
}

func init() {
	countSignature := runtime.MustLookupBuiltinType("universe", "count")
	function.RegisterTransformation("universe", CountKind, &CountProcedureSpec{}, countSignature)
}

type CountProcedureSpec struct {
	Tables  *function.TableObject `flux:"tables,required"`
	Columns []string              `flux:"columns"`
	Column  string                `flux:"column"`
}

func (s *CountProcedureSpec) CreateTransformation(id execute.DatasetID, a execute.Administration) (execute.Transformation, execute.Dataset, error) {
	columns := s.Columns
	if len(s.Columns) == 0 {
		if s.Column != "" {
			columns = []string{s.Column}
		} else {
			columns = []string{execute.DefaultValueColLabel}
		}
	}
	t, d := executekit.NewAggregateTransformation(id, &countTransformation{
		Columns: columns,
	}, a.Allocator())
	return t, d, nil
}

type countState struct {
	Label string
	Value int64
}

type countTransformation struct {
	Columns []string
}

func (c countTransformation) Aggregate(view table.View, state interface{}, mem memory.Allocator) (interface{}, bool, error) {
	if state == nil {
		cs := make([]countState, len(c.Columns))
		for i, column := range c.Columns {
			cs[i].Label = column
		}
		state = cs
	}
	cs := state.([]countState)

	for i, column := range c.Columns {
		idx := view.Index(column)
		if idx < 0 {
			return nil, false, errors.New(codes.FailedPrecondition, "missing column %s", column)
		}

		values := view.Values(idx)
		cs[i].Value += int64(values.Len() - values.NullN())
	}
	return cs, true, nil
}

func (c countTransformation) Compute(key flux.GroupKey, state interface{}, d *executekit.Dataset, mem memory.Allocator) error {
	builder := table.NewArrowBuilder(key, mem)
	for _, col := range key.Cols() {
		_, _ = builder.AddCol(col)
	}

	cs := state.([]countState)
	for _, s := range cs {
		_, _ = builder.AddCol(flux.ColMeta{
			Label: s.Label,
			Type:  flux.TInt,
		})
	}

	for i, v := range key.Values() {
		_ = arrow.AppendValue(builder.Builders[i], v)
	}
	offset := len(key.Cols())
	for i, s := range cs {
		_ = arrow.AppendInt(builder.Builders[i+offset], s.Value)
	}

	buffer, err := builder.Buffer()
	if err != nil {
		return err
	}
	return d.Process(table.ViewFromBuffer(buffer))
}
