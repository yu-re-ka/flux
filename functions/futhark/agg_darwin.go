package futhark

import "C"

//go:generate futhark-opencl --library agg.fut -o agg_gpu
//go:generate futhark-c --library agg.fut -o agg_cpu

//go:generate gsed -i s/futhark/futhark_gpu/g agg_gpu.c agg_gpu.h
//go:generate gsed -i s/futhark/futhark_cpu/g agg_cpu.c agg_cpu.h

//go:generate gcc -o libagg.so -fPIC -shared -framework OpenCL agg_gpu.c agg_cpu.c

var deviceStr = C.CString("AMD")
