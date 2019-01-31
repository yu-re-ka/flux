package langserver

import (
	"context"
	"io"

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

func (s *Server) Serve(rw io.ReadWriteCloser) error {
	stream := jsonrpc2.NewBufferedStream(rw, jsonrpc2.VSCodeObjectCodec{})
	handler := jsonrpc2.HandlerWithError(func(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) (result interface{}, err error) {
		return s.handler.Handle(ctx, s.logger, conn, req)
	})
	conn := jsonrpc2.NewConn(context.TODO(), stream, handler)
	<-conn.DisconnectNotify()
	return nil
}
