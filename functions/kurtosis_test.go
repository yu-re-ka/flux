package functions_test

import (
	"testing"

	"github.com/influxdata/flux/execute/executetest"
	"github.com/influxdata/flux/functions/futhark"
)

func BenchmarkSkew(b *testing.B) {
	executetest.AggFuncBenchmarkHelper(
		b,
		new(functions.Kurtosis),
		NormalData,
		0.0032200673020400935,
	)
}
func BenchmarkKurtosisBySize(b *testing.B) {
	AggFuncBySizeBenchmarkHelper(
		b,
		futhark.NewAggregator(futhark.Kurtosis),
	)
}
