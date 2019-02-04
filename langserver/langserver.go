package langserver

import (
	"context"
	"net"

	"go.uber.org/zap"
)

type Server struct {
	logger *zap.Logger
}

func New(l *zap.Logger) *Server {
	return &Server{
		logger: l,
	}
}

func (s *Server) Serve(l net.Listener) error {
	for {
		conn, err := l.Accept()
		if err != nil {
			return err
		}

		logger := s.logger.With(zap.Stringer("addr", conn.RemoteAddr()))
		go func() {
			_ = Run(context.Background(), conn, logger)
		}()
	}
}
