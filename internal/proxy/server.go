package proxy

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/splusq/resemble/internal/cache"
)

type Server struct {
	httpServer *http.Server
	handler    *Handler
	listener   net.Listener
}

func NewServer(addr, upstreamURL, mode string, ttl time.Duration, store *cache.Store, keyOpts cache.KeyOptions) *Server {
	handler := NewHandler(store, upstreamURL, mode, ttl, keyOpts)

	return &Server{
		httpServer: &http.Server{
			Addr:         addr,
			Handler:      handler,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 5 * time.Minute,
			IdleTimeout:  120 * time.Second,
		},
		handler: handler,
	}
}

func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.httpServer.Addr, err)
	}
	s.listener = ln

	log.Printf("resemble: proxy listening on %s", ln.Addr().String())
	log.Printf("resemble: upstream: %s", s.handler.upstreamURL)
	log.Printf("resemble: mode: %s", s.handler.mode)

	go func() {
		if err := s.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("resemble: server error: %v", err)
		}
	}()

	return nil
}

func (s *Server) Addr() string {
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return s.httpServer.Addr
}

func (s *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.httpServer.Shutdown(ctx)
}
