package executetest

import (
	"github.com/influxdata/flux/dependencies"
	"github.com/influxdata/flux/dependencies/dependenciestest"
)

func NewTestExecuteDependencies() dependencies.Interface {
	return dependenciestest.Default()
}
