package langserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"

	"github.com/sourcegraph/go-lsp"
	"github.com/sourcegraph/jsonrpc2"
	"go.uber.org/zap"
)

type Server struct {
	handler *Handler
	logger  *zap.Logger
}

func New(h Handler, l *zap.Logger) *Server {
	return &Server{
		handler: &h,
		logger:  l,
	}
}

func (s *Server) Serve(l net.Listener) error {
	for {
		conn, err := l.Accept()
		if err != nil {
			return err
		}
		go s.serve(conn)
	}
}

func (s *Server) serve(rw io.ReadWriteCloser) error {
	stream := jsonrpc2.NewBufferedStream(rw, jsonrpc2.VSCodeObjectCodec{})
	handler := jsonrpc2.HandlerWithError(s.handle)
	conn := jsonrpc2.NewConn(context.TODO(), stream, handler)
	<-conn.DisconnectNotify()
	return nil
}

func (s *Server) handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) (result interface{}, err error) {
	logger := s.logger
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
		if err := s.reset(params); err != nil {
			return nil, err
		}
		return lsp.InitializeResult{}, nil
	case "exit":
	}
	return nil, &jsonrpc2.Error{
		Code:    jsonrpc2.CodeMethodNotFound,
		Message: fmt.Sprintf("method not supported: %s", req.Method),
	}
}

func (s *Server) reset(params lsp.InitializeParams) error {
	return nil
}
