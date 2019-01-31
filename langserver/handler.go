package langserver

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sourcegraph/go-lsp"
	"github.com/sourcegraph/jsonrpc2"
	"go.uber.org/zap"
)

type Handler struct{}

func (h *Handler) Handle(ctx context.Context, logger *zap.Logger, conn *jsonrpc2.Conn, req *jsonrpc2.Request) (result interface{}, err error) {
	if req.ID.IsString {
		logger = logger.With(zap.String("id", req.ID.Str))
	} else {
		logger = logger.With(zap.Uint64("id", req.ID.Num))
	}
	logger = logger.With(zap.String("method", req.Method))

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

		logger.Info("Initialize", zap.Int("processId", params.ProcessID), zap.String("path", string(params.RootURI)))
		if err := h.reset(params); err != nil {
			return nil, err
		}
		return lsp.InitializeResult{}, nil
	}
	return nil, &jsonrpc2.Error{
		Code:    jsonrpc2.CodeMethodNotFound,
		Message: fmt.Sprintf("method not supported: %s", req.Method),
	}
}

func (h *Handler) reset(params lsp.InitializeParams) error {
	return nil
}
