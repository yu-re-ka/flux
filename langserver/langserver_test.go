package langserver_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/influxdata/flux/langserver"
	"github.com/jsternberg/zap-logfmt"
	"github.com/sourcegraph/go-lsp"
	"github.com/sourcegraph/jsonrpc2"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

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

func TestServer_Serve(t *testing.T) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		handler := langserver.Handler{}
		server := langserver.New(handler, zap.NewNop())
		if err := server.Serve(l); err != nil {
			t.Error(err)
		}
	}()

	conn, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	stream := jsonrpc2.NewBufferedStream(conn, jsonrpc2.VSCodeObjectCodec{})
	client := jsonrpc2.NewConn(ctx, stream, nil)

	var (
		params lsp.InitializeParams
		res    lsp.InitializeResult
	)
	if err := client.Call(ctx, "initialize", params, &res); err != nil {
		t.Error(err)
	}

	// TODO(jsternberg): Check the server capabilities.

	if err := client.Notify(ctx, "exit", nil); err != nil {
		t.Error(err)
	}

	if err := client.Close(); err != nil {
		t.Error(err)
	}

	timer := time.NewTimer(100 * time.Millisecond)
	select {
	case <-done:
		timer.Stop()
	case <-timer.C:
		t.Errorf("server did not exit")
	}
}
