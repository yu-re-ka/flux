package functions_test

import (
	"testing"

	"github.com/influxdata/flux/functions/futhark"
)

func BenchmarkKurtosisFutharkBySize(b *testing.B) {
	AggFuncBySizeBenchmarkHelper(
		b,
		futhark.NewAggregator(futhark.Kurtosis),
	)
}
