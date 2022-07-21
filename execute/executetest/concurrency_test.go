package executetest

import (
	"context"
	glog "log"
	"testing"
	"time"

	"github.com/influxdata/flux/plan"
	"go.uber.org/zap/zaptest"

	"github.com/influxdata/flux"
	"github.com/influxdata/flux/dependencies/dependenciestest"
	"github.com/influxdata/flux/dependency"
	"github.com/influxdata/flux/execute"
	_ "github.com/influxdata/flux/fluxinit/static"
	fluxfeature "github.com/influxdata/flux/internal/feature"
	"github.com/influxdata/flux/internal/pkg/feature"
	"github.com/influxdata/flux/internal/spec"
	"github.com/influxdata/flux/parser"
	"github.com/influxdata/flux/runtime"
	"github.com/influxdata/flux/stdlib/influxdata/influxdb"
	"github.com/influxdata/flux/stdlib/universe"
)

type flagger map[string]interface{}

func compile(fluxText string, now time.Time) (context.Context, *flux.Spec, error) {
	ctx, deps := dependency.Inject(context.Background(), dependenciestest.Default())
	defer deps.Finish()
	spec, err := spec.FromScript(ctx, runtime.Default, now, fluxText)
	return ctx, spec, err
}

const ParallelFromRemoteTestKind = "parallel-from-remote-test"

type ParallelFromRemoteProcedureSpec struct {
	*influxdb.FromRemoteProcedureSpec
	factor int
}

func (src *ParallelFromRemoteProcedureSpec) OutputAttributes() plan.PhysicalAttributes {
	if src.factor > 1 {
		return plan.PhysicalAttributes{
			plan.ParallelRunKey: plan.ParallelRunAttribute{Factor: src.factor},
		}
	}
	return nil
}

func (src *ParallelFromRemoteProcedureSpec) Kind() plan.ProcedureKind {
	return ParallelFromRemoteTestKind
}

func (src *ParallelFromRemoteProcedureSpec) Copy() plan.ProcedureSpec {
	return src
}

func (src *ParallelFromRemoteProcedureSpec) Cost(inStats []plan.Statistics) (plan.Cost, plan.Statistics) {
	return plan.Cost{}, plan.Statistics{}
}

type parallelizeFromTo struct {
	Factor int
}

func (parallelizeFromTo) Name() string {
	return "parallelizeFromTo"
}

func (parallelizeFromTo) Pattern() plan.Pattern {
	return plan.Pat(influxdb.ToKind, plan.Pat(influxdb.FromRemoteKind))
}

func (rule parallelizeFromTo) Rewrite(ctx context.Context, pn plan.Node) (plan.Node, bool, error) {
	glog.Printf("-------- trying to rewrite into parallel form")

	toNode := pn
	toSpec := toNode.ProcedureSpec().(*influxdb.ToProcedureSpec)

	fromNode := toNode.Predecessors()[0]
	fromSpec := fromNode.ProcedureSpec().(*influxdb.FromRemoteProcedureSpec)

	physicalFromNode := fromNode.(*plan.PhysicalPlanNode)
	if attr := plan.GetOutputAttribute(physicalFromNode, plan.ParallelRunKey); attr != nil {
		return pn, false, nil
	}

	glog.Printf("-------- rewriting into parallel form")

	newFromNode := plan.CreateUniquePhysicalNode(ctx, "from", &ParallelFromRemoteProcedureSpec{
		FromRemoteProcedureSpec: fromSpec.Copy().(*influxdb.FromRemoteProcedureSpec),
		factor:                  rule.Factor,
	})
	newToNode := plan.CreateUniquePhysicalNode(ctx, "to", toSpec.Copy().(*influxdb.ToProcedureSpec))
	mergeNode := plan.CreateUniquePhysicalNode(ctx, "partitionMerge", &universe.PartitionMergeProcedureSpec{Factor: rule.Factor})

	newFromNode.AddSuccessors(newToNode)
	newToNode.AddPredecessors(newFromNode)

	newToNode.AddSuccessors(mergeNode)
	mergeNode.AddPredecessors(newToNode)

	return mergeNode, true, nil
}

func TestConcurrency(t *testing.T) {
	type runWith struct {
		concurrencyQuota int
	}

	now := parser.MustParseTime("2022-01-01T10:00:00Z").Value

	testcases := []struct {
		name                     string
		fromPlan                 int
		flux                     string
		flagger                  flagger
		parallelizeFactor        int
		queryConcurrencyIncrease int
		wantConcurrencyQuota     int
	}{
		{
			name:                 "from-plan",
			flux:                 `from(bucket: "bucket", host: "host") |> range( start: 0 )`,
			fromPlan:             9,
			wantConcurrencyQuota: 9,
		},
		{
			name:                 "one-result",
			flux:                 `from(bucket: "bucket", host: "host") |> range( start: 0 ) |> filter( fn: (r) => r.key == "value" )`,
			wantConcurrencyQuota: 1,
		},
		{
			name: "two-results",
			flux: `
				from(bucket: "bucket", host: "host") |> range( start: 0 ) |> filter( fn: (r) => r.key == "value" ) 
				from(bucket: "bucket", host: "host") |> range( start: 0 ) |> filter( fn: (r) => r.key == "value" )
			`,
			wantConcurrencyQuota: 2,
		},
		{
			name: "five-results",
			flux: `
				from(bucket: "bucket", host: "host") |> range( start: 0 )
				from(bucket: "bucket", host: "host") |> range( start: 0 )
				from(bucket: "bucket", host: "host") |> range( start: 0 )
				from(bucket: "bucket", host: "host") |> range( start: 0 )
				from(bucket: "bucket", host: "host") |> range( start: 0 )
			`,
			wantConcurrencyQuota: 5,
		},
		{
			name: "five-yields",
			flux: `
				from(bucket: "bucket", host: "host") |> range( start: 0 ) |> yield( name: "n1" )
				from(bucket: "bucket", host: "host") |> range( start: 0 ) |> yield( name: "n2" )
				from(bucket: "bucket", host: "host") |> range( start: 0 ) |> yield( name: "n3" )
				from(bucket: "bucket", host: "host") |> range( start: 0 ) |> yield( name: "n4" )
				from(bucket: "bucket", host: "host") |> range( start: 0 ) |> yield( name: "n5" )
			`,
			wantConcurrencyQuota: 5,
		},
		{
			name: "chained-yields",
			flux: `
				from(bucket: "bucket", host: "host") |> range( start: 0 ) |> yield( name: "n1" ) |> yield( name: "n2" ) |> yield( name: "n3" )
				from(bucket: "bucket", host: "host") |> range( start: 0 ) |> yield( name: "n4" ) |> yield( name: "n5" ) 
			`,
			wantConcurrencyQuota: 2,
		},
		{
			name: "one-union",
			flux: `
				s1 = from(bucket: "bucket", host: "host") |> range( start: 0 )
				s2 = from(bucket: "bucket", host: "host") |> range( start: 0 )
				union( tables: [ s1, s2 ] )
			`,
			wantConcurrencyQuota: 2,
		},
		{
			name: "two-union",
			flux: `
				s1 = from(bucket: "bucket", host: "host") |> range( start: 0 )
				s2 = from(bucket: "bucket", host: "host") |> range( start: 0 )
				s3 = from(bucket: "bucket", host: "host") |> range( start: 0 )
				s4 = from(bucket: "bucket", host: "host") |> range( start: 0 )
				union( tables: [ s1, s2 ] )
				union( tables: [ s1, s2, s3, s4 ] )
			`,
			wantConcurrencyQuota: 6,
		},
		{
			name: "two-union-behind-yield-1",
			flux: `
				s1 = from(bucket: "bucket", host: "host") |> range( start: 0 )
				s2 = from(bucket: "bucket", host: "host") |> range( start: 0 )
				s3 = from(bucket: "bucket", host: "host") |> range( start: 0 )
				s4 = from(bucket: "bucket", host: "host") |> range( start: 0 )
				union( tables: [ s1, s2 ] ) |> yield(name: "n1")
				union( tables: [ s1, s2, s3, s4 ] )
			`,
			wantConcurrencyQuota: 6,
		},
		{
			name: "two-union-behind-yield-2",
			flux: `
				s1 = from(bucket: "bucket", host: "host") |> range( start: 0 )
				s2 = from(bucket: "bucket", host: "host") |> range( start: 0 )
				s3 = from(bucket: "bucket", host: "host") |> range( start: 0 )
				s4 = from(bucket: "bucket", host: "host") |> range( start: 0 )
				union( tables: [ s1, s2 ] ) |> yield(name: "n1") |> yield( name: "n2" )
				union( tables: [ s1, s2, s3, s4 ] ) |> yield( name: "n3" )
			`,
			wantConcurrencyQuota: 6,
		},
		{
			name: "inline-yield-1",
			flux: `
				from(bucket: "bucket", host: "host")
					|> range( start: 0 )
					|> yield( name: "n1" )
					|> filter( fn: (r) => r.t == "tv" )
			`,
			wantConcurrencyQuota: 2,
		},
		{
			name: "inline-yield-2",
			flux: `
				from(bucket: "bucket", host: "host")
					|> range( start: 0 )
					|> yield( name: "n1" )
					|> filter( fn: (r) => r.t == "tv" )
					|> yield( name: "n2" )
			`,
			wantConcurrencyQuota: 2,
		},
		{
			name: "inline-yield-3",
			flux: `
				from(bucket: "bucket", host: "host")
					|> range( start: 0 )
					|> yield( name: "n1" )
					|> yield( name: "n2" )
					|> filter( fn: (r) => r.t == "tv" )
					|> yield( name: "n3" )
			`,
			wantConcurrencyQuota: 2,
		},
		{
			name: "inline-yield-4",
			flux: `
				from(bucket: "bucket", host: "host")
					|> range( start: 0 )
					|> yield( name: "n1" )
					|> filter( fn: (r) => r.t == "tv" )
					|> yield( name: "n2" )
					|> filter( fn: (r) => r.t == "tv" )
					|> yield( name: "n3" )
			`,
			wantConcurrencyQuota: 3,
		},
		{
			name: "inline-yield-5",
			flux: `
				s1 = from(bucket: "bucket", host: "host") |> range( start: 0 )
				s2 = from(bucket: "bucket", host: "host") |> range( start: 0 )
				s3 = from(bucket: "bucket", host: "host") |> range( start: 0 )
				s4 = from(bucket: "bucket", host: "host") |> range( start: 0 )
				union( tables: [ s1, s2 ] )
					|> yield(name: "n1")
					|> yield( name: "n2" ) 
					|> filter( fn: (r) => r.t == "tv" )
				union( tables: [ s1, s2, s3, s4 ] )
			`,
			wantConcurrencyQuota: 7,
		},
		{
			name:                     "increase-1",
			flux:                     `from(bucket: "bucket", host: "host") |> range( start: 0 ) |> filter( fn: (r) => r.key == "value" )`,
			queryConcurrencyIncrease: 1,
			wantConcurrencyQuota:     2,
		},
		{
			name: "increase-2",
			flux: `
				from(bucket: "bucket", host: "host") |> range( start: 0 )
				from(bucket: "bucket", host: "host") |> range( start: 0 )
			`,
			queryConcurrencyIncrease: 2,
			wantConcurrencyQuota:     4,
		},
		{
			name: "increase-3",
			flux: `
				s1 = from(bucket: "bucket", host: "host") |> range( start: 0 )
				s2 = from(bucket: "bucket", host: "host") |> range( start: 0 )
				s3 = from(bucket: "bucket", host: "host") |> range( start: 0 )
				s4 = from(bucket: "bucket", host: "host") |> range( start: 0 )
				union( tables: [ s1, s2 ] )
					|> yield(name: "n1")
					|> yield( name: "n2" ) 
					|> filter( fn: (r) => r.t == "tv" )
				union( tables: [ s1, s2, s3, s4 ] )
			`,
			queryConcurrencyIncrease: 3,
			wantConcurrencyQuota:     10,
		},
		{
			name: "parallelize-none-accounted-for",
			flux: `
				from(bucket: "bucket", host: "host")
					|> range(start: 0)
					|> to(bucket:"other-bucket")
					|> filter( fn: (r) => r.t == "tv" )
			`,
			parallelizeFactor:    4,
			wantConcurrencyQuota: 9,
		},
		{
			name: "parallelize-some-accounted-for",
			flux: `
				from(bucket: "bucket", host: "host")
					|> range(start: 0)
					|> to(bucket:"other-bucket")
			`,
			parallelizeFactor:    4,
			wantConcurrencyQuota: 9,
		},
		{
			name: "parallelize-behind-yield-all-accounted-for",
			flux: `
				from(bucket: "bucket", host: "host")
					|> range(start: 0)
					|> to(bucket:"other-bucket")
					|> yield(name: "n1")
				from(bucket: "bucket", host: "host")
					|> range(start: 0)
					|> to(bucket:"other-bucket")
					|> yield(name: "n2")
				from(bucket: "bucket", host: "host")
					|> range(start: 0)
					|> to(bucket:"other-bucket")
					|> yield(name: "n3")
			`,
			parallelizeFactor:    4,
			wantConcurrencyQuota: 12,
		},
		{
			name: "two-union-parallel-some-accounted-for",
			flux: `
				s1 = from(bucket: "bucket", host: "host") |> range( start: 0 )
				s2 = from(bucket: "bucket", host: "host") |> range( start: 0 )
				s3 = from(bucket: "bucket", host: "host") |> range( start: 0 )
				s4 = from(bucket: "bucket", host: "host") |> range( start: 0 )
				union( tables: [ s1, s2 ] )
				union( tables: [ s1, s2, s3, s4 ] )
				from(bucket: "bucket", host: "host")
					|> range(start: 0)
					|> to(bucket:"other-bucket")
					|> yield(name: "n1")
			`,
			parallelizeFactor:    4,
			wantConcurrencyQuota: 15,
		},
		{
			name: "parallel-behind-union-none-accounted-for",
			flux: `
				s1 = from(bucket: "bucket", host: "host")
					|> range(start: 0)
					|> to(bucket:"other-bucket")
				s2 = from(bucket: "bucket", host: "host")
					|> range(start: 0)
					|> to(bucket:"other-bucket")
				s3 = from(bucket: "bucket", host: "host") |> range( start: 0 )
				s4 = from(bucket: "bucket", host: "host") |> range( start: 0 )
				union( tables: [ s1, s2 ] )
				union( tables: [ s1, s2, s3, s4 ] )
			`,
			parallelizeFactor:    4,
			wantConcurrencyQuota: 14,
		},
		{
			name: "parallel-behind-union-all-accounted-for",
			flux: `
				s1 = from(bucket: "bucket", host: "host")
					|> range(start: 0)
					|> to(bucket:"other-bucket")
				s2 = from(bucket: "bucket", host: "host")
					|> range(start: 0)
					|> to(bucket:"other-bucket")
				s3 = from(bucket: "bucket", host: "host") |> range( start: 0 )
				s4 = from(bucket: "bucket", host: "host") |> range( start: 0 )
				union( tables: [ s1, s2 ] )
				union( tables: [ s1, s2, s3, s4 ] )
				from(bucket: "bucket", host: "host")
					|> range(start: 0)
					|> to(bucket:"other-bucket")
				from(bucket: "bucket", host: "host")
					|> range(start: 0)
					|> to(bucket:"other-bucket")
				from(bucket: "bucket", host: "host")
					|> range(start: 0)
					|> to(bucket:"other-bucket")
			`,
			parallelizeFactor:    4,
			wantConcurrencyQuota: 18,
		},
	}

	for _, tc := range testcases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, fluxSpec, err := compile(tc.flux, now)

			flagger := TestFlagger{}
			flagger[fluxfeature.QueryConcurrencyIncrease().Key()] = tc.queryConcurrencyIncrease
			ctx = feature.Inject(ctx, flagger)

			if err != nil {
				t.Fatalf("could not compile flux query: %v", err)
			}

			logicalPlanner := plan.NewLogicalPlanner()
			initPlan, err := logicalPlanner.CreateInitialPlan(fluxSpec)
			if err != nil {
				t.Fatal(err)
			}
			logicalPlan, err := logicalPlanner.Plan(context.Background(), initPlan)
			if err != nil {
				t.Fatal(err)
			}

			var physicalPlanner plan.PhysicalPlanner
			if tc.parallelizeFactor > 0 {
				physicalPlanner = plan.NewPhysicalPlanner(plan.OnlyPhysicalRules(
					&influxdb.FromRemoteRule{}, &influxdb.MergeRemoteRangeRule{}, &parallelizeFromTo{Factor: tc.parallelizeFactor}))
			} else {
				physicalPlanner = plan.NewPhysicalPlanner(plan.OnlyPhysicalRules(&influxdb.FromRemoteRule{}, &influxdb.MergeRemoteRangeRule{}))
				//physicalPlanner = plan.NewPhysicalPlanner()
			}
			physicalPlan, err := physicalPlanner.Plan(context.Background(), logicalPlan)
			if err != nil {
				t.Fatal(err)
			}

			if tc.fromPlan > 0 {
				physicalPlan.Resources.ConcurrencyQuota = tc.fromPlan
			}

			// Construct a basic execution state and choose the default resources.
			concurrencyQuota := execute.WhatIsConcurrency(ctx, physicalPlan, zaptest.NewLogger(t))

			if concurrencyQuota != tc.wantConcurrencyQuota {
				t.Errorf("Expected concurrency quota of %v, but execution state has %v",
					tc.wantConcurrencyQuota, concurrencyQuota)
			}
		})
	}
}
