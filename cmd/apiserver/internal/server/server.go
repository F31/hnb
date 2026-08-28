package server

import (
	"context"
	"log"
	"net/http"
	"time"
)

type Server struct {
	httpServer *http.Server
	handler    http.Handler
	drainCh    chan struct{}
}

func New(addr string, handler http.Handler) *Server {
	return &Server{
		httpServer: &http.Server{
			Addr:         addr,
			Handler:      handler,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 60 * time.Second,
			IdleTimeout:  120 * time.Second,
		},
		drainCh: make(chan struct{}),
	}
}

func (s *Server) Start() error {
	log.Printf("[server] listening on %s", s.httpServer.Addr)
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	log.Println("[server] shutting down...")
	s.httpServer.SetKeepAlivesEnabled(false)

	// Drain signal for load balancer
	close(s.drainCh)

	shutdownCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	return s.httpServer.Shutdown(shutdownCtx)
}

func (s *Server) DrainCh() <-chan struct{} {
	return s.drainCh
}
