package futhark

import "C"

//go:generate gcc -o libagg.so -fPIC -shared -framework OpenCL agg.c

var deviceStr = C.CString("AMD")
