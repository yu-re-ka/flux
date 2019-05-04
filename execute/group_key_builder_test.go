package execute_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/influxdata/flux"
	"github.com/influxdata/flux/values"

	"github.com/influxdata/flux/execute"
)

func TestGroupKeyBuilder_Empty(t *testing.T) {
	var gkb execute.GroupKeyBuilder
	gkb.AddKeyValue("_measurement", values.NewString("cpu"))

	key, err := gkb.Build()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if got, want := key.NCols(), 1; got != want {
		t.Fatalf("unexpected number of columns -want/+got:\n\t- %d\n\t+ %d", want, got)
	}

	if got, want := keyCols(key), []flux.ColMeta{
		{Label: "_measurement", Type: flux.TString},
	}; !cmp.Equal(want, got) {
		t.Fatalf("unexpected columns -want/+got:\n%s", cmp.Diff(want, got))
	}

	if got, want := keyValues(key), []values.Value{
		values.NewString("cpu"),
	}; !cmp.Equal(want, got) {
		t.Fatalf("unexpected columns -want/+got:\n%s", cmp.Diff(want, got))
	}
}

func TestGroupKeyBuilder_Nil(t *testing.T) {
	gkb := execute.NewGroupKeyBuilder(nil)
	gkb.AddKeyValue("_measurement", values.NewString("cpu"))

	key, err := gkb.Build()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if got, want := key.NCols(), 1; got != want {
		t.Fatalf("unexpected number of columns -want/+got:\n\t- %d\n\t+ %d", want, got)
	}

	if got, want := keyCols(key), []flux.ColMeta{
		{Label: "_measurement", Type: flux.TString},
	}; !cmp.Equal(want, got) {
		t.Fatalf("unexpected columns -want/+got:\n%s", cmp.Diff(want, got))
	}

	if got, want := keyValues(key), []values.Value{
		values.NewString("cpu"),
	}; !cmp.Equal(want, got) {
		t.Fatalf("unexpected columns -want/+got:\n%s", cmp.Diff(want, got))
	}
}

func TestGroupKeyBuilder_Existing(t *testing.T) {
	gkb := execute.NewGroupKeyBuilder(
		execute.NewGroupKey(
			[]flux.ColMeta{
				{
					Label: "_measurement",
					Type:  flux.TString,
				},
			},
			[]values.Value{
				values.NewString("cpu"),
			},
		),
	)
	gkb.AddKeyValue("_field", values.NewString("usage_user"))

	key, err := gkb.Build()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if got, want := key.NCols(), 2; got != want {
		t.Fatalf("unexpected number of columns -want/+got:\n\t- %d\n\t+ %d", want, got)
	}

	if got, want := keyCols(key), []flux.ColMeta{
		{Label: "_measurement", Type: flux.TString},
		{Label: "_field", Type: flux.TString},
	}; !cmp.Equal(want, got) {
		t.Fatalf("unexpected columns -want/+got:\n%s", cmp.Diff(want, got))
	}

	if got, want := keyValues(key), []values.Value{
		values.NewString("cpu"),
		values.NewString("usage_user"),
	}; !cmp.Equal(want, got) {
		t.Fatalf("unexpected columns -want/+got:\n%s", cmp.Diff(want, got))
	}
}

func keyCols(key flux.GroupKey) []flux.ColMeta {
	cols := make([]flux.ColMeta, key.NCols())
	for j := range cols {
		cols[j] = key.Col(j)
	}
	return cols
}

func keyValues(key flux.GroupKey) []values.Value {
	values := make([]values.Value, key.NCols())
	for j := range values {
		values[j] = key.Value(j)
	}
	return values
}
