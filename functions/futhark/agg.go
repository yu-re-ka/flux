package futhark

// #cgo darwin LDFLAGS: -L. -lagg -framework OpenCL
// #cgo !darwin LDFLAGS: -L. -lagg -lOpenCL -lm
//
// #include "agg_gpu.h"
// #include "agg_cpu.h"
import "C"

import (
	"github.com/influxdata/flux"
	"github.com/influxdata/flux/execute"
)

var (
	ctx_gpu *C.struct_futhark_gpu_context
	ctx_cpu *C.struct_futhark_cpu_context
)

func init() {
	cfg_gpu := C.futhark_gpu_context_config_new()
	C.futhark_gpu_context_config_set_device(cfg_gpu, deviceStr)
	ctx_gpu = C.futhark_gpu_context_new(cfg_gpu)

	cfg_cpu := C.futhark_cpu_context_config_new()
	ctx_cpu = C.futhark_cpu_context_new(cfg_cpu)
}

type AggFunc struct {
	cpu func(*C.double, *C.struct_futhark_cpu_f64_1d)
	gpu func(*C.double, *C.struct_futhark_gpu_f64_1d)
}

type Aggregator struct {
	agg AggFunc
}

func NewAggregator(agg AggFunc) *Aggregator {
	return &Aggregator{
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
	return NewDoFloatAgg(a.agg)
}

func (a *Aggregator) NewStringAgg() execute.DoStringAgg {
	return nil
}

type DoFloatAgg struct {
	out float64
	agg AggFunc
}

func NewDoFloatAgg(agg AggFunc) *DoFloatAgg {
	return &DoFloatAgg{
		agg: agg,
	}
}

func (f *DoFloatAgg) Type() flux.DataType {
	return flux.TFloat
}

func (f *DoFloatAgg) DoFloat(data []float64) {
	if len(data) < 1e6 {
		in := C.futhark_cpu_new_f64_1d(ctx_cpu, (*C.double)(&data[0]), C.int(len(data)))
		f.agg.cpu((*C.double)(&f.out), in)
		C.futhark_cpu_free_f64_1d(ctx_cpu, in)
	} else {
		in := C.futhark_gpu_new_f64_1d(ctx_gpu, (*C.double)(&data[0]), C.int(len(data)))
		f.agg.gpu((*C.double)(&f.out), in)
		C.futhark_gpu_free_f64_1d(ctx_gpu, in)
	}
}

func (f *DoFloatAgg) ValueFloat() float64 {
	return f.out
}

var Sum = AggFunc{
	cpu: func(out *C.double, in *C.struct_futhark_cpu_f64_1d) { C.futhark_cpu_entry_sum(ctx_cpu, out, in) },
	gpu: func(out *C.double, in *C.struct_futhark_gpu_f64_1d) { C.futhark_gpu_entry_sum(ctx_gpu, out, in) },
}

var Mean = AggFunc{
	cpu: func(out *C.double, in *C.struct_futhark_cpu_f64_1d) { C.futhark_cpu_entry_mean(ctx_cpu, out, in) },
	gpu: func(out *C.double, in *C.struct_futhark_gpu_f64_1d) { C.futhark_gpu_entry_mean(ctx_gpu, out, in) },
}

var Stddev = AggFunc{
	cpu: func(out *C.double, in *C.struct_futhark_cpu_f64_1d) { C.futhark_cpu_entry_stddev(ctx_cpu, out, in) },
	gpu: func(out *C.double, in *C.struct_futhark_gpu_f64_1d) { C.futhark_gpu_entry_stddev(ctx_gpu, out, in) },
}

var Skew = AggFunc{
	cpu: func(out *C.double, in *C.struct_futhark_cpu_f64_1d) { C.futhark_cpu_entry_skew(ctx_cpu, out, in) },
	gpu: func(out *C.double, in *C.struct_futhark_gpu_f64_1d) { C.futhark_gpu_entry_skew(ctx_gpu, out, in) },
}

var Kurtosis = AggFunc{
	cpu: func(out *C.double, in *C.struct_futhark_cpu_f64_1d) { C.futhark_cpu_entry_kurtosis(ctx_cpu, out, in) },
	gpu: func(out *C.double, in *C.struct_futhark_gpu_f64_1d) { C.futhark_gpu_entry_kurtosis(ctx_gpu, out, in) },
}
