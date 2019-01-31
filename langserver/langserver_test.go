package langserver_test

import (
	"context"
	"fmt"
	"io"
	"os"
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
	r1, w1 := io.Pipe()
	r2, w2 := io.Pipe()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)

		rwc := langserver.ReadWriter(r1, w2)
		handler := langserver.Handler{}
		server := langserver.New(handler, newLogger(os.Stdout))
		if err := server.Serve(rwc); err != nil {
			t.Error(err)
		}
	}()

	rwc := langserver.ReadWriter(r2, w1)
	stream := jsonrpc2.NewBufferedStream(rwc, jsonrpc2.VSCodeObjectCodec{})
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

func TestServer_Serve_Shutdown(t *testing.T) {
	r1, w1 := io.Pipe()
	r2, w2 := io.Pipe()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)

		rwc := langserver.ReadWriter(r1, w2)
		handler := langserver.Handler{}
		server := langserver.New(handler, zap.NewNop())
		if err := server.Serve(rwc); err != nil {
			t.Error(err)
		}
	}()

	rwc := langserver.ReadWriter(r2, w1)
	stream := jsonrpc2.NewBufferedStream(rwc, jsonrpc2.VSCodeObjectCodec{})
	client := jsonrpc2.NewConn(ctx, stream, nil)

	var (
		params lsp.InitializeParams
		res    lsp.InitializeResult
	)
	if err := client.Call(ctx, "initialize", params, &res); err != nil {
		t.Error(err)
	}

	// TODO(jsternberg): Check the server capabilities.

	if err := client.Call(ctx, "shutdown", nil, nil); err != nil {
		t.Error(err)
	}

	if err := client.Call(ctx, "request", nil, nil); err == nil {
		t.Error("expected error")
	} else if err := err.(*jsonrpc2.Error); err.Code != jsonrpc2.CodeInvalidRequest {
		t.Errorf("unexpected error code: want=%d got=%d", jsonrpc2.CodeInvalidRequest, err.Code)
	}

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
