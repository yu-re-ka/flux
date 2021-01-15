package reflect

import (
	"math/rand"
	"testing"
	"time"

	"github.com/influxdata/flux/semantic"
)

var Result struct {
	Value   Value
	Nature  semantic.Nature
	Float64 float64
	Int64   int64
}

func BenchmarkNewFloat(b *testing.B) {
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		Result.Value = NewFloat(rnd.Float64())
	}
}

func BenchmarkValue_Float(b *testing.B) {
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	v := NewFloat(rnd.Float64())

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		Result.Float64 = v.Float()
	}
}

func BenchmarkValue_Float_Nature(b *testing.B) {
	v := NewFloat(0)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		Result.Nature = v.Nature()
	}
}

func BenchmarkValue_Int(b *testing.B) {
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	n := rnd.Int63()
	for n >= 0 && n < 256 {
		n = rnd.Int63()
	}
	v := NewInt(n)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		Result.Int64 = v.Int()
	}
}

func BenchmarkNewString(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		Result.Value = NewString("abc")
	}
}
