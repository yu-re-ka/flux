package futhark

//go:generate gcc -o libagg.so -fPIC -shared -lOpenCL agg.c
