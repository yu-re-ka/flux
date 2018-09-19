package futhark

//go:generate gcc -o libagg.so -fPIC -shared -framework OpenCL agg.c
