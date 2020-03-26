package cmd

import (
	"context"
	"log"
	"net/http"
	_ "net/http/pprof"
	"runtime"

	"github.com/influxdata/flux"
	_ "github.com/influxdata/flux/builtin"
	"github.com/influxdata/flux/dependencies/filesystem"
	"github.com/influxdata/flux/repl"
	"github.com/spf13/cobra"
)

var pprof bool
// replCmd represents the repl command
var replCmd = &cobra.Command{
	Use:   "repl",
	Short: "Launch a Flux REPL",
	Long:  "Launch a Flux REPL (Read-Eval-Print-Loop)",
	Run: func(cmd *cobra.Command, args []string) {
		deps := flux.NewDefaultDependencies()
		deps.Deps.FilesystemService = filesystem.SystemFS
		// inject the dependencies to the context.
		// one useful example is socket.from, kafka.to, and sql.from/sql.to where we need
		// to access the url validator in deps to validate the user-specified url.
		ctx := deps.Inject(context.Background())
		r := repl.New(ctx, deps)
		if pprof {
			runtime.SetMutexProfileFraction(1)
			go func() {
				log.Println(http.ListenAndServe("localhost:6060", nil))
			}()
		}
		r.Run()
	},
}

func init() {
	rootCmd.AddCommand(replCmd)
	replCmd.Flags().BoolVarP(&pprof, "pprof", "", false, "enable pprof")
}
