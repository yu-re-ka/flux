package futhark

//go:generate futhark-opencl --library agg.fut

// #cgo darwin LDFLAGS: -L. -lagg -framework OpenCL
// #cgo !darwin LDFLAGS: -L. -lagg -lOpenCL -lm
// #include "agg.h"
import "C"

import (
	"github.com/influxdata/flux"
	"github.com/influxdata/flux/execute"
)

// AggFunc is a function the performs an aggregate operation using
type AggFunc func(*C.struct_futhark_context, *C.double, *C.struct_futhark_f64_1d)

type Aggregator struct {
	cfg *C.struct_futhark_context_config
	agg AggFunc
}

func NewAggregator(agg AggFunc) *Aggregator {
	return &Aggregator{
		//TODO(nathanielc): Create config sturct to map to futhark_context_config
		// Using defaults for now.
		cfg: C.futhark_context_config_new(),
		agg: agg,
	}
}

func (a *Aggregator) NewBoolAgg() execute.DoBoolAgg {
	return nil
}

func (a *Aggregator) NewIntAgg() execute.DoIntAgg {
	return nil
}

func (a *Aggregator) NewUIntAgg() execute.DoUIntAgg {
	return nil
}

func (a *Aggregator) NewFloatAgg() execute.DoFloatAgg {
	return NewDoFloatAgg(a.cfg, a.agg)
}

func (a *Aggregator) NewStringAgg() execute.DoStringAgg {
	return nil
}

type DoFloatAgg struct {
	ctx *C.struct_futhark_context
	out float64
	agg AggFunc
}

func NewDoFloatAgg(cfg *C.struct_futhark_context_config, agg AggFunc) *DoFloatAgg {
	var ctx = C.futhark_context_new(cfg)

	return &DoFloatAgg{
		ctx: ctx,
		agg: agg,
	}
}

func (f *DoFloatAgg) Type() flux.DataType {
	return flux.TFloat
}

func (f *DoFloatAgg) DoFloat(data []float64) {
	var in = C.futhark_new_f64_1d(f.ctx, (*C.double)(&data[0]), (C.int)(len(data)))
	f.agg(f.ctx, (*C.double)(&f.out), in)
	C.futhark_free_f64_1d(f.ctx, in)
}

func (f *DoFloatAgg) ValueFloat() float64 {
	return f.out
}

var Sum = func(ctx *C.struct_futhark_context, out *C.double, in *C.struct_futhark_f64_1d) {
	C.futhark_entry_sum(ctx, out, in)
}
var Mean = func(ctx *C.struct_futhark_context, out *C.double, in *C.struct_futhark_f64_1d) {
	C.futhark_entry_mean(ctx, out, in)
}
