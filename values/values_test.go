package values_test

import (
	"fmt"
	"math/rand"
	"regexp"
	"testing"
	"time"

	"github.com/influxdata/flux/semantic"
	"github.com/influxdata/flux/values"
)

func TestNew(t *testing.T) {
	for _, tt := range []struct {
		v    interface{}
		want values.Value
	}{
		{v: "abc", want: values.NewString("abc")},
		{v: int64(4), want: values.NewInt(4)},
		{v: uint64(4), want: values.NewUInt(4)},
		{v: float64(6.0), want: values.NewFloat(6.0)},
		{v: true, want: values.NewBool(true)},
		{v: values.Time(1000), want: values.NewTime(values.Time(1000))},
		{v: values.ConvertDurationNsecs(1), want: values.NewDuration(values.ConvertDurationNsecs(1))},
		{v: regexp.MustCompile(`.+`), want: values.NewRegexp(regexp.MustCompile(`.+`))},
	} {
		t.Run(fmt.Sprint(tt.want.Type()), func(t *testing.T) {
			if want, got := tt.want, values.New(tt.v); !want.Equal(got) {
				t.Fatalf("unexpected value -want/+got\n\t- %s\n\t+ %s", want, got)
			}
		})
	}
}

func TestNewNull(t *testing.T) {
	v := values.NewNull(semantic.BasicString)
	if want, got := true, v.IsNull(); want != got {
		t.Fatalf("unexpected value -want/+got\n\t- %v\n\t+ %v", want, got)
	}
}

var Result struct {
	Value   values.Value
	Nature  semantic.Nature
	Float64 float64
}

func BenchmarkNewFloat(b *testing.B) {
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		Result.Value = values.NewFloat(rnd.Float64())
	}
}

func BenchmarkValue_Float(b *testing.B) {
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	v := values.NewFloat(rnd.Float64())

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		Result.Float64 = v.Float()
	}
}

func BenchmarkValue_Float_Nature(b *testing.B) {
	v := values.NewFloat(0)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		Result.Nature = v.Type().Nature()
	}
}
