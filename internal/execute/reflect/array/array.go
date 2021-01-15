package array

import (
	"github.com/apache/arrow/go/arrow/array"
	"github.com/influxdata/flux/semantic"
)

type Base interface {
	Type() semantic.MonoType
	NullN() int
	IsNull(i int) bool
	IsValid(i int) bool
	Data() array.Interface
	Len() int
	Retain()
	Release()
}

type (
	Int64   = array.Int64
	Float64 = array.Float64
)
