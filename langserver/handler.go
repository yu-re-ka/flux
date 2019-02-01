package langserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"time"

	"github.com/influxdata/flux/complete"
	"github.com/sourcegraph/go-lsp"
	"github.com/sourcegraph/jsonrpc2"
	"go.uber.org/zap"
)

type langHandler struct {
	logger   *zap.Logger
	shutdown bool

	workspace string
}

func NewHandler(logger *zap.Logger) jsonrpc2.Handler {
	h := &langHandler{
		logger: logger,
	}
	return jsonrpc2.HandlerWithError(h.Handle)
}

func Run(ctx context.Context, rwc io.ReadWriteCloser, logger *zap.Logger) error {
	stream := jsonrpc2.NewBufferedStream(rwc, jsonrpc2.VSCodeObjectCodec{})
	jsonrpcConn := jsonrpc2.NewConn(context.Background(), stream, NewHandler(logger))
	select {
	case <-ctx.Done():
		jsonrpcConn.Close()
		return ctx.Err()
	case <-jsonrpcConn.DisconnectNotify():
		return nil
	}
}

func (h *langHandler) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) (result interface{}, err error) {
	logger := h.logger
	if req.ID.IsString {
		logger = logger.With(zap.String("id", req.ID.Str))
	} else {
		logger = logger.With(zap.Uint64("id", req.ID.Num))
	}
	logger = logger.With(zap.String("method", req.Method))

	defer func(start time.Time) {
		dur := time.Since(start)
		logger.Info("Request received", zap.Duration("dur", dur))
	}(time.Now())

	if h.shutdown && req.Method != "exit" {
		return nil, &jsonrpc2.Error{
			Code:    jsonrpc2.CodeInvalidRequest,
			Message: "server is shutdown",
		}
	}

	switch req.Method {
	case "initialize":
		// TODO(jsternberg): Keep track if the server was already initialized.
		if req.Params == nil {
			return nil, &jsonrpc2.Error{Code: jsonrpc2.CodeInvalidParams}
		}
		var params lsp.InitializeParams
		if err := json.Unmarshal(*req.Params, &params); err != nil {
			return nil, err
		}

		logger.Info("Initialized", zap.Int("processId", params.ProcessID), zap.String("path", string(params.RootURI)))
		return lsp.InitializeResult{
			Capabilities: lsp.ServerCapabilities{
				CompletionProvider: &lsp.CompletionOptions{
					TriggerCharacters: []string{"."},
				},
			},
		}, nil
	case "initialized":
		return nil, nil
	case "shutdown":
		h.shutdown = true
		return nil, nil
	case "exit":
		if err := conn.Close(); err != nil {
			return nil, err
		}
		return nil, nil
	case "textDocument/completion":
		if req.Params == nil {
			return nil, &jsonrpc2.Error{Code: jsonrpc2.CodeInvalidParams}
		}
		var params lsp.CompletionParams
		if err := json.Unmarshal(*req.Params, &params); err != nil {
			return nil, err
		}
		return h.completions(params.TextDocument.URI)
	}

	return nil, &jsonrpc2.Error{
		Code:    jsonrpc2.CodeMethodNotFound,
		Message: fmt.Sprintf("method not supported: %s", req.Method),
	}
}

func (h *langHandler) completions(uri lsp.DocumentURI) (lsp.CompletionList, error) {
	text, err := h.getText(uri)
	if err != nil {
		return lsp.CompletionList{}, err
	}
	list, err := complete.StaticComplete(text)
	if err != nil {
		return lsp.CompletionList{}, err
	}
	items := make([]lsp.CompletionItem, 0, len(list))
	for _, item := range list {
		items = append(items, lsp.CompletionItem{
			Label: item,
		})
	}
	return lsp.CompletionList{
		Items: items,
	}, nil
}

func (h *langHandler) getText(uri lsp.DocumentURI) (string, error) {
	u, err := url.Parse(string(uri))
	if err != nil {
		return "", err
	}

	if u.Scheme != "file" {
		return "", fmt.Errorf("invalid uri scheme: %s", u.Scheme)
	}

	data, err := ioutil.ReadFile(u.Path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
