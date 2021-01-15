package reflect

import (
	"math"
	"runtime/debug"

	"github.com/influxdata/flux/codes"
	"github.com/influxdata/flux/internal/errors"
	"github.com/influxdata/flux/semantic"
)

type Value struct {
	// t holds the type nature of this value.
	// This determines how the value is read.
	t semantic.Nature
	// v holds the byte representation of this value
	// if it has one.
	v uint64
	// data holds any allocated memory for more complex
	// types such as containers.
	data interface{}
}

func (v Value) Nature() semantic.Nature {
	return v.t
}

func (v Value) Str() string {
	CheckKind(v.t, semantic.String)
	return v.data.(string)
}

func (v Value) Bytes() []byte {
	CheckKind(v.t, semantic.Bytes)
	return v.data.([]byte)
}

func (v Value) Int() int64 {
	CheckKind(v.t, semantic.Int)
	return int64(v.v)
}

func (v Value) Uint() uint64 {
	CheckKind(v.t, semantic.UInt)
	return v.v
}

func (v Value) Float() float64 {
	CheckKind(v.t, semantic.Float)
	return math.Float64frombits(v.v)
}

func (v Value) Bool() bool {
	CheckKind(v.t, semantic.Bool)
	return v.v != 0
}

func (v Value) Array() Array {
	CheckKind(v.t, semantic.Array)
	return v.data.(Array)
}

func UnexpectedKind(got, exp semantic.Nature) error {
	return errors.Newf(codes.Internal, "unexpected kind: got %q expected %q, trace: %s", got, exp, string(debug.Stack()))
}

// CheckKind panics if got != exp.
func CheckKind(got, exp semantic.Nature) {
	if got != exp {
		panic(UnexpectedKind(got, exp))
	}
}

func NewString(v string) Value {
	return Value{
		t:    semantic.String,
		data: v,
	}
}

func NewInt(v int64) Value {
	return Value{
		t: semantic.Int,
		v: uint64(v),
	}
}

func NewFloat(v float64) Value {
	return Value{
		t: semantic.Float,
		v: math.Float64bits(v),
	}
}

func NewBool(v bool) Value {
	return Value{
		t: semantic.Bool,
		v: boolbit(v),
	}
}

func boolbit(v bool) uint64 {
	if v {
		return 1
	} else {
		return 0
	}
}
