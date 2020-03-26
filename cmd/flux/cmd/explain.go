package cmd

import (
	"context"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/influxdata/flux"
	_ "github.com/influxdata/flux/builtin"
	"github.com/influxdata/flux/dependencies/filesystem"
	"github.com/influxdata/flux/internal/spec"
	"github.com/influxdata/flux/plan"
	"github.com/influxdata/flux/repl"
	"github.com/influxdata/flux/stdlib/influxdata/influxdb"
	"github.com/spf13/cobra"
)

// compileCmd represents the compile command
var explainCmd = &cobra.Command{
	Use:   "explain",
	Short: "Display the query plan for a Flux script if it has one.",
	Long:  "Display the query plan for a Flux script if it has one (use @ as prefix to the file)",
	Args:  cobra.ExactArgs(1),
	RunE:  explain,
}

func init() {
	rootCmd.AddCommand(explainCmd)
}

type DefaultFromOrgAttributes struct {
}

func (d DefaultFromOrgAttributes) Name() string {
	return "DefaultFromOrgAttributes"
}

func (d DefaultFromOrgAttributes) Pattern() plan.Pattern {
	return plan.Any()
}

func (d DefaultFromOrgAttributes) Rewrite(n plan.Node) (plan.Node, bool, error) {
	spec, ok := n.ProcedureSpec().(influxdb.ProcedureSpec)
	if !ok {
		return n, false, nil
	}

	changed := false
	if spec.GetOrg() == nil {
		spec.SetOrg(&influxdb.NameOrID{Name: "influxdata"})
		changed = true
	}
	return n, changed, nil
}

func printNodeTree(n plan.Node) {
	for _, pre := range n.Predecessors() {
		printNodeTree(pre)
	}
	fmt.Printf("[%s] - %s\n\t%+v\n\n", n.Kind(), n.ID(), n.ProcedureSpec())
}

func explain(cmd *cobra.Command, args []string) error {
	scriptSource := args[0]

	var script string
	if scriptSource[0] == '@' {
		scriptBytes, err := ioutil.ReadFile(scriptSource[1:])
		if err != nil {
			return err
		}
		script = string(scriptBytes)
	} else {
		script = scriptSource
	}

	deps := flux.NewDefaultDependencies()
	deps.Deps.FilesystemService = filesystem.SystemFS
	// inject the dependencies to the context.
	// one useful example is socket.from, kafka.to, and sql.from/sql.to where we need
	// to access the url validator in deps to validate the user-specified url.
	ctx := deps.Inject(context.Background())
	r := repl.New(ctx, deps)
	ses, err := r.Eval(script)
	if err != nil {
		return err
	}
	spec, err := spec.FromEvaluation(ctx, ses, time.Now())
	plan.RegisterLogicalRules(DefaultFromOrgAttributes{})
	planner := plan.PlannerBuilder{}.Build()
	ps, err := planner.Plan(spec)

	for node, _ := range ps.Roots {
		printNodeTree(node)
		fmt.Println()
	}
	return nil
}
