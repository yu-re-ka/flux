package universe

import (
	"fmt"
	"time"
	"math"

	"github.com/influxdata/flux"
	"github.com/influxdata/flux/execute"
	"github.com/influxdata/flux/interpreter"
	"github.com/influxdata/flux/plan"
	"github.com/influxdata/flux/semantic"
	"github.com/influxdata/flux/values"
)

const LinearRegressionKind = "linReg"

type LinearRegressionOpSpec struct {
	Column []string      `json:"columns"`
}

func init() {
	linearRegressionSignature := flux.FunctionSignature(
		map[string]semantic.PolyType{
			"columns":  semantic.NewArrayPolyType(semantic.String),
		},
	)

	flux.RegisterPackageValue("universe", LinearRegressionKind, flux.FunctionValue(LinearRegressionKind, createLinearRegressionOpSpec, linRegSignature))
	flux.RegisterOpSpec(LinearRegressionKind, newLinearRegressionOp)
	plan.RegisterProcedureSpec(LinearRegressionKind, newLinearRegressionProcedure, LinearRegressionKind)
	execute.RegisterTransformation(LinearRegressionKind, createLinearRegressionTransformation)
}

func createLinearRegressionOpSpec(args flux.Arguments, a *flux.Administration) (flux.OperationSpec, error) {
	if err := a.AddParentFromArgs(args); err != nil {
		return nil, err
	}

	spec := new(LinearRegressionOpSpec)

	// if shift, err := args.GetRequiredDuration("duration"); err != nil {
	// 	return nil, err
	// } else {
	// 	spec.Shift = shift
	// }

	if cols, ok, err := args.GetArray("columns", semantic.String); err != nil {
		return nil, err
	} else if ok {
		columns, err := interpreter.ToStringArray(cols)
		if err != nil {
			return nil, err
		}
		spec.Column = col
	} else {
		spec.Column = execute.DefaultValueColLabel,
			// execute.DefaultStopColLabel,
			// execute.DefaultStartColLabel,
		}
	}
	return spec, nil
}

func newLinearRegressionOp() flux.OperationSpec {
	return new(LinearRegressionOpSpec)
}

func (s *LinearRegressionOpSpec) Kind() flux.OperationKind {
	return LinearRegressionKind
}

type LinearRegressionProcedureSpec struct {
	plan.DefaultCost
	// Shift   flux.Duration
	Column string
	// Now     time.Time
}

// TimeBounds implements plan.BoundsAwareProcedureSpec
// func (s *LinearRegressionProcedureSpec) TimeBounds(predecessorBounds *plan.Bounds) *plan.Bounds {
// 	if predecessorBounds != nil {
// 		return predecessorBounds.Shift(values.Duration(s.Shift))
// 	}
// 	return nil
// }
func (s *LinearRegressionProcedureSpec) Kind() plan.ProcedureKind {
	return LinearRegressionKind

func newLinearRegressionProcedure(qs flux.OperationSpec, pa plan.Administration) (plan.ProcedureSpec, error) {
	spec, ok := qs.(*LinearRegressionOpSpec)
	if !ok {
		return nil, fmt.Errorf("invalid spec type %T", qs)
	}

	return &LinearRegressionProcedureSpec{
		// Shift:   spec.Shift,
		Column: spec.Column,
		// Now:     pa.Now(),
	}, nil
}

func (s *LinearRegressionProcedureSpec) Kind() plan.ProcedureKind {
	return LinearRegressionKind
}

func (s *LinearRegressionProcedureSpec) Copy() plan.ProcedureSpec {
	ns := new(LinearRegressionProcedureSpec)
	*ns = *s

	// if s.Column != nil {
	// 	ns.Column = make([]string, len(s.Column))
	// 	copy(ns.Column, s.Column)
	// }
	return ns
}

// TriggerSpec implements plan.TriggerAwareProcedureSpec
func (s *LinearRegressionProcedureSpec) TriggerSpec() plan.TriggerSpec {
	return plan.NarrowTransformationTriggerSpec{}
}

func createLinearRegressionTransformation(id execute.DatasetID, mode execute.AccumulationMode, spec plan.ProcedureSpec, a execute.Administration) (execute.Transformation, execute.Dataset, error) {
	s, ok := spec.(*LinearRegressionProcedureSpec)
	if !ok {
		return nil, nil, fmt.Errorf("invalid spec type %T", spec)
	}
	cache := execute.NewTableBuilderCache(a.Allocator())
	d := execute.NewDataset(id, mode, cache)
	t := NewShiftTransformation(d, cache, s)
	return t, d, nil
}

type linRegTransformation struct {
	d       execute.Dataset
	cache   execute.TableBuilderCache

	column string
}

func NewShiftTransformation(d execute.Dataset, cache execute.TableBuilderCache, spec *LinearRegressionProcedureSpec) *shiftTransformation {
	return &shiftTransformation{
		d:       d,
		cache:   cache,
		column: spec.Column,
	}
}

func (t *shiftTransformation) RetractTable(id execute.DatasetID, key flux.GroupKey) error {
	return t.d.RetractTable(key)
}
func MeanVariance(x, weights []float64) (mean, variance float64) {
	// This uses the corrected two-pass algorithm (1.7), from "Algorithms for computing
	// the sample variance: Analysis and recommendations" by Chan, Tony F., Gene H. Golub,
	// and Randall J. LeVeque.

	// Note that this will panic if the slice lengths do not match.
	mean = Mean(x, weights)
	var (
		ss           float64
		compensation float64
	)
	if weights == nil {
		for _, v := range x {
			d := v - mean
			ss += d * d
			compensation += d
		}
		variance = (ss - compensation*compensation/float64(len(x))) / float64(len(x)-1)
		return mean, variance
	}

func Mean(x, weights []float64) float64 {
	if weights == nil {
		return floats.Sum(x) / float64(len(x))
	}
	if len(x) != len(weights) {
		panic("stat: slice length mismatch")
	}
	var (
		sumValues  float64
		sumWeights float64
	)
	for i, w := range weights {
		sumValues += w * x[i]
		sumWeights += w
	}
	return sumValues / sumWeights
}
func covarianceMeans(x, y, weights []float64, xu, yu float64) float64 {
	var (
		ss            float64
		xcompensation float64
		ycompensation float64
	)
	if weights == nil {
		for i, xv := range x {
			yv := y[i]
			xd := xv - xu
			yd := yv - yu
			ss += xd * yd
			xcompensation += xd
			ycompensation += yd
		}
		// xcompensation and ycompensation are from Chan, et. al.
		// referenced in the MeanVariance function. They are analogous
		// to the second term in (1.7) in that paper.
		return (ss - xcompensation*ycompensation/float64(len(x))) / float64(len(x)-1)
	}


func LinearRegression(x, y, weights []float64, origin bool) (alpha, beta float64) {
	if len(x) != len(y) {
		panic("stat: slice length mismatch")
	}
	if weights != nil && len(weights) != len(x) {
		panic("stat: slice length mismatch")
	}

	w := 1.0
	if origin {
		var x2Sum, xySum float64
		for i, xi := range x {
			if weights != nil {
				w = weights[i]
			}
			yi := y[i]
			xySum += w * xi * yi
			x2Sum += w * xi * xi
		}
		beta = xySum / x2Sum

		return 0, beta
	}

xu, xv := MeanVariance(x, weights)
	yu := Mean(y, weights)
	cov := covarianceMeans(x, y, weights, xu, yu)
	beta = cov / xv
	alpha = yu - beta*xu
	return alpha, beta
}


func (t *shiftTransformation) Process(id execute.DatasetID, tbl flux.Table) error {
	// key := tbl.Key()
	// // Update key
	// cols := make([]flux.ColMeta, len(key.Cols()))
	// vs := make([]values.Value, len(key.Cols()))
	// for j, c := range key.Cols() {
	// 	if execute.ContainsStr(t.columns, c.Label) {false
	// 		if c.Type != flux.TTime {
	// 			return fmt.Errorf("column %q is not of type time", c.Label)
	// 		}
	// 		cols[j] = c
	// 		vs[j] = values.NewTime(key.ValueTime(j).Add(t.shift))
	// 	} else {
	// 		cols[j] = c
	// 		vs[j] = key.Value(j)
	// 	}
	// }
	// key = execute.NewGroupKey(cols, vs)

	builder, created := t.cache.TableBuilder(key)
	if !created {
		return fmt.Errorf("linReg found duplicate table with key: %v", tbl.Key())
	}
	if err := execute.AddTableCols(tbl, builder); err != nil {
		return err

		cols := tbl.Cols()
			lr := make([]*linReg, len(cols))
			for j, c := range cols {
				for _, label := range t.spec.Columns {
					if c.Label == label {
						lr[j] = &linReg{}
						break
					}
				}
	}

	var y []float;
	
	tbl.Do(func(cr flux.ColReader) error {
		l := cr.Len()
		for j, c := range cols {
			switch c.Type {
			case flux.TFloat:
				append(x, cr.Floats(j))
			}
		}
	})

	var x []float = makeRange(1, len(y) + 1)

	var alpha, beta = LinearRegression(x, y, nil, false)
}

func makeRange(min, max int) []int {
    a := make([]int, max-min+1)
    for i := range a {
        a[i] = min + i
    }
    return a
}

type linReg struct {
	intVal   int64
	uintVal  uint64
	floatVal float64
}
func (t *shiftTransformation) UpdateWatermark(id execute.DatasetID, mark execute.Time) error {
	return t.d.UpdateWatermark(mark)
}

func (t *shiftTransformation) UpdateProcessingTime(id execute.DatasetID, pt execute.Time) error {
	return t.d.UpdateProcessingTime(pt)
}

func (t *shiftTransformation) Finish(id execute.DatasetID, err error) {
	t.d.Finish(err)
}

func (t *shiftTransformation) SetParents(ids []execute.DatasetID) {}
