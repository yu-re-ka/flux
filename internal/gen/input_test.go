package gen_test

import (
	"context"
	"testing"

	"github.com/influxdata/flux"
	"github.com/influxdata/flux/execute/executetest"
	"github.com/influxdata/flux/internal/gen"
	"github.com/influxdata/flux/memory"
)

func TestInput_TableTest(t *testing.T) {
	executetest.RunTableTests(t, executetest.TableTest{
		NewFn: func(ctx context.Context, alloc *memory.Allocator) flux.TableIterator {
			schema := gen.Schema{
				Tags: []gen.Tag{
					{Name: "_measurement", Cardinality: 1},
					{Name: "_field", Cardinality: 1},
					{Name: "t0", Cardinality: 100},
				},
				NumPoints: 100,
				Alloc:     alloc,
			}
			tables, err := gen.Input(context.Background(), schema)
			if err != nil {
				t.Fatal(err)
			}
			return tables
		},
		IsDone: func(tbl flux.Table) bool {
			return tbl.(interface{ IsDone() bool }).IsDone()
		},
	})
}
