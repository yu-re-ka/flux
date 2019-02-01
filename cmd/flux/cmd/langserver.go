package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/influxdata/flux/langserver"
	"github.com/jsternberg/zap-logfmt"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// langserverCmd represents the langserver command
var langserverCmd = &cobra.Command{
	Use:   "langserver",
	Short: "Start a Flux language server",
	Long:  "Start a Flux language server with the LSP protocol",
	RunE:  runLangserver,
}

func init() {
	rootCmd.AddCommand(langserverCmd)
}

const timeFormat = "2006-01-02T15:04:05.000000Z07:00"

func newLogger(w io.Writer) *zap.Logger {
	config := zap.NewProductionEncoderConfig()
	config.EncodeTime = func(ts time.Time, encoder zapcore.PrimitiveArrayEncoder) {
		encoder.AppendString(ts.UTC().Format(timeFormat))

	}
	config.EncodeDuration = func(d time.Duration, encoder zapcore.PrimitiveArrayEncoder) {
		val := float64(d) / float64(time.Millisecond)
		encoder.AppendString(fmt.Sprintf("%.3fms", val))
	}
	config.LevelKey = "lvl"

	encoder := zaplogfmt.NewEncoder(config)
	return zap.New(zapcore.NewCore(
		encoder,
		zapcore.Lock(zapcore.AddSync(w)),
		zapcore.DebugLevel,
	))
}

func runLangserver(cmd *cobra.Command, args []string) error {
	logger := newLogger(os.Stderr)
	return langserver.Run(context.TODO(), langserver.ReadWriter(os.Stdin, os.Stdout), logger)
}
