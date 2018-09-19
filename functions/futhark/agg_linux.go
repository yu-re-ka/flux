package futhark

import "C"

//go:generate gcc -o libagg.so -fPIC -shared -lOpenCL agg.c

var deviceStr = C.CString("NVidia")
