package complete_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/influxdata/flux/complete"
)

func TestStaticComplete(t *testing.T) {
	testCases := []struct {
		name string
		flux string
		want []string
	}{
		{
			name: "simple",
			flux: "a={b:1}",
			want: []string{"b"},
		},
		{
			name: "simpletwo",
			flux: "a={c:1, x:5}",
			want: []string{"c", "x"},
		},
	}
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := complete.StaticComplete(tc.flux)
			if err != nil {
				t.Fatal(err)
			}
			if !cmp.Equal(tc.want, got) {
				t.Errorf("unexpected complete list: -want/+got\n%s", cmp.Diff(tc.want, got))
			}
		})
	}
}
