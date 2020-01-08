package semantic_test

import (
	"testing"

	flatbuffers "github.com/google/flatbuffers/go"
	"github.com/influxdata/flux/semantic"
	"github.com/influxdata/flux/semantic/internal/fbsemantic"
)

func TestLookup(t *testing.T) {
	table := new(flatbuffers.Table)
	name := "lookup"
	want := semantic.MonoType{
		mt:  fbsemantic.MonoTypeFun,
		tbl: table,
	}
	t.Run(name, func(t *testing.T) {
		if got := semantic.LookupBuiltInType("stdlib/strings", "strings.flux"); want != got {
			t.Fatalf("unexpected result -want/+got\n\t- %s\n\t+ %s", want, got)
		}
	})
}
