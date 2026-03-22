package api

import (
	"log/slog"
	"net/http"
)

// Server is the HTTP API server that hosts Connect-go services.
type Server struct {
	mux  *http.ServeMux
	addr string
}

// NewServer creates a new API server listening on addr.
func NewServer(addr string) *Server {
	return &Server{
		mux:  http.NewServeMux(),
		addr: addr,
	}
}

// Handle registers a handler on the server's mux at the given pattern.
func (s *Server) Handle(pattern string, handler http.Handler) {
	s.mux.Handle(pattern, handler)
}

// Handler returns the mux wrapped with middleware.
func (s *Server) Handler() http.Handler {
	var h http.Handler = s.mux
	h = recoveryMiddleware(h)
	h = loggingMiddleware(h)
	h = corsMiddleware(h, []string{"*"})
	return h
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	slog.Info("starting server", "addr", s.addr)
	return http.ListenAndServe(s.addr, s.Handler())
}
