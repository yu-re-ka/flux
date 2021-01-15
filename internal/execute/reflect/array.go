package reflect

import (
	"github.com/apache/arrow/go/arrow/array"
	"github.com/influxdata/flux/semantic"
)

type Array interface {
	Type() semantic.MonoType
	Data() array.Interface
	Retain()
	Release()
}
