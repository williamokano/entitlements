package httpx

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// shutdownTimeout bounds how long graceful shutdown waits for in-flight
// requests to drain before forcing close.
const shutdownTimeout = 10 * time.Second

// Server wraps http.Server with context-driven graceful shutdown.
type Server struct {
	httpSrv *http.Server
	logger  *slog.Logger
}

// NewServer builds a Server listening on addr and serving h.
func NewServer(addr string, h http.Handler, logger *slog.Logger) *Server {
	return &Server{
		httpSrv: &http.Server{
			Addr:              addr,
			Handler:           h,
			ReadHeaderTimeout: 5 * time.Second,
		},
		logger: logger,
	}
}

// Run listens on the server's address and serves until ctx is canceled, then
// shuts down gracefully. It returns nil on clean shutdown.
func (s *Server) Run(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.httpSrv.Addr)
	if err != nil {
		return err
	}
	return s.Serve(ctx, ln)
}

// Serve serves on ln until ctx is canceled, then shuts down gracefully. It is
// separate from Run so tests can supply their own listener (e.g. on port 0).
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("http server listening", "addr", ln.Addr().String())
		if err := s.httpSrv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		s.logger.Info("shutting down http server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		return s.httpSrv.Shutdown(shutdownCtx)
	}
}
