package querytest

import (
	"context"
	"github.com/influxdata/flux/dependencies/dependenciestest"
	"io"

	"github.com/influxdata/flux"
)

type Querier struct{}

func (q *Querier) Query(ctx context.Context, w io.Writer, c flux.Compiler, d flux.Dialect) (int64, error) {
	program, err := c.Compile(ctx)
	if err != nil {
		return 0, err
	}
	query, err := program.Start(ctx, dependenciestest.Default())
	if err != nil {
		return 0, err
	}
	results := flux.NewResultIteratorFromQuery(query)
	defer results.Release()

	encoder := d.Encoder()
	return encoder.Encode(w, results)
}

func NewQuerier() *Querier {
	return &Querier{}
}
