package values

import (
	"github.com/influxdata/flux/semantic"
	"io"
	"regexp"
)

type Stream interface {
	Value
	io.ReadSeeker
}

type stream struct {
	t semantic.Type
	r *io.ReadSeeker
}

func (s stream) Type() semantic.Type {
	return s.t
}

func (s stream) PolyType() semantic.PolyType {
	return s.t.PolyType()
}

func (s stream) IsNull() bool {
	return false
}

func (s stream) Str() string {
	panic(UnexpectedKind(semantic.Object, semantic.String))
}

func (s stream) Int() int64 {
	panic(UnexpectedKind(semantic.Object, semantic.Int))
}

func (s stream) UInt() uint64 {
	panic(UnexpectedKind(semantic.Object, semantic.UInt))
}

func (s stream) Float() float64 {
	panic(UnexpectedKind(semantic.Object, semantic.Float))
}

func (s stream) Bool() bool {
	panic(UnexpectedKind(semantic.Object, semantic.Bool))
}

func (s stream) Time() Time {
	panic(UnexpectedKind(semantic.Object, semantic.Time))
}

func (s stream) Duration() Duration {
	panic(UnexpectedKind(semantic.Object, semantic.Duration))
}

func (s stream) Regexp() *regexp.Regexp {
	panic(UnexpectedKind(semantic.Object, semantic.Regexp))
}

func (s stream) Array() Array {
	panic(UnexpectedKind(semantic.Object, semantic.Array))
}

func (s stream) Object() Object {
	panic(UnexpectedKind(semantic.Object, semantic.Object))
}

func (s stream) Function() Function {
	panic(UnexpectedKind(semantic.Object, semantic.Function))
}

func (s stream) Equal(rhs Value) bool {
	return s.Type() == rhs.Type() && s.r == rhs.Stream().r
}

func (s stream) Read(p []byte) (n int, err error) {
	panic("implement me")
}

func (s stream) Seek(offset int64, whence int) (int64, error) {
	panic("implement me")
}

func NewStream(rs *io.ReadSeeker) Stream {
	return &stream{
		t: semantic.Object,
		r: rs,
	}
}

