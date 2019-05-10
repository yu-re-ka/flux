package group_test

import (
	"testing"
	"time"

	"github.com/influxdata/flux"
	"github.com/influxdata/flux/execute"
	"github.com/influxdata/flux/internal/experimental/group"
	"github.com/influxdata/flux/values"
)

func BenchmarkGroupKey(b *testing.B) {
	for i := 0; i < b.N; i++ {
		benchmarkGroupKey()
	}
}

func benchmarkGroupKey() {
	start := values.Time(0 * time.Second)
	stop := values.Time(300 * time.Second)
	parent := execute.NewGroupKey(
		[]flux.ColMeta{
			{Label: "_measurement", Type: flux.TString},
			{Label: "_field", Type: flux.TString},
			{Label: execute.DefaultStartColLabel, Type: flux.TTime},
			{Label: execute.DefaultStopColLabel, Type: flux.TTime},
			{Label: "host", Type: flux.TString},
			{Label: "region", Type: flux.TString},
		},
		[]values.Value{
			values.NewString("cpu"),
			values.NewString("usage_user"),
			values.NewTime(start),
			values.NewTime(stop),
			values.NewString("server01"),
			values.NewString("us-west-1"),
		},
	)

	for start := start; start < stop; start += values.Time(time.Second) {
		_ = newWindowGroupKey(parent, start, stop+1)
	}
}

func newWindowGroupKey(parent flux.GroupKey, start, stop values.Time) flux.GroupKey {
	cols := make([]flux.ColMeta, len(parent.Cols()))
	vs := make([]values.Value, len(parent.Cols()))
	for j, c := range parent.Cols() {
		cols[j] = c
		switch c.Label {
		case execute.DefaultStartColLabel:
			vs[j] = values.NewTime(start)
		case execute.DefaultStopColLabel:
			vs[j] = values.NewTime(stop)
		default:
			vs[j] = parent.Value(j)
		}
	}
	return execute.NewGroupKey(cols, vs)
}

func BenchmarkKey(b *testing.B) {
	for i := 0; i < b.N; i++ {
		benchmarkKey()
	}
}

func benchmarkKey() {
	start := values.Time(0 * time.Second)
	stop := values.Time(300 * time.Second)
	parent := group.NewKey(
		[]flux.ColMeta{
			{Label: "_measurement", Type: flux.TString},
			{Label: "_field", Type: flux.TString},
			{Label: execute.DefaultStartColLabel, Type: flux.TTime},
			{Label: execute.DefaultStopColLabel, Type: flux.TTime},
			{Label: "host", Type: flux.TString},
			{Label: "region", Type: flux.TString},
		},
		[]values.Value{
			values.NewString("cpu"),
			values.NewString("usage_user"),
			values.NewTime(start),
			values.NewTime(stop),
			values.NewString("server01"),
			values.NewString("us-west-1"),
		},
	)

	for start := start; start < stop; start += values.Time(time.Second) {
		_ = group.Range(parent, start, stop)
	}
}
