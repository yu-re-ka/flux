package main

import (
	"fmt"
	"net"
	"net/http"

	"github.com/NYTimes/gziphandler"
	"github.com/influxdata/flux"
	"github.com/influxdata/flux/csv"
	"github.com/influxdata/flux/internal/gen"
	"github.com/urjitbhatia/cozgo"
)

func Handle(w http.ResponseWriter, r *http.Request) {
	cozgo.Begin("handler")
	defer cozgo.End("handler")

	tables, err := gen.Input(r.Context(), gen.Schema{
		Tags: []gen.Tag{
			{Name: "_measurement", Cardinality: 1},
			{Name: "_field", Cardinality: 1},
			{Name: "t0", Cardinality: 5},
			{Name: "t1", Cardinality: 10},
		},
		NumPoints: 5000,
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "%s\n", err)
		return
	}

	results := flux.NewMapResultIterator(map[string]flux.Result{
		"_result": &result{tables: tables},
	})

	enc := csv.NewMultiResultEncoder(csv.DefaultEncoderConfig())
	_, _ = enc.Encode(w, results)
}

func main() {
	l, err := net.Listen("tcp", ":9317")
	if err != nil {
		panic(err)
	}

	h := gziphandler.GzipHandler(http.HandlerFunc(Handle))
	if err := http.Serve(l, h); err != nil {
		panic(err)
	}
}

type result struct {
	tables flux.TableIterator
}

func (r *result) Name() string {
	return ""
}

func (r *result) Tables() flux.TableIterator {
	return r.tables
}
