package rpc

import (
	"context"
	"net/http"
	"time"

	"github.com/relab/hotstuff/logging"
)

// Server represents the JSON-RPC server
type Server struct {
	handler *Handler
	server  *http.Server
	logger  logging.Logger
}

// NewServer creates a new JSON-RPC server
func NewServer(handler *Handler, addr string) *Server {
	mux := http.NewServeMux()
	mux.Handle("/", handler)
	
	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	return &Server{
		handler: handler,
		server:  server,
		logger:  logging.New("rpc-server"),
	}
}

// Start starts the RPC server
func (s *Server) Start() error {
	s.logger.Infof("Starting JSON-RPC server on %s", s.server.Addr)
	
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Errorf("RPC server error: %v", err)
		}
	}()
	
	s.logger.Infof("JSON-RPC server started successfully")
	s.logger.Infof("Ethereum JSON-RPC API available at: http://%s", s.server.Addr)
	s.logger.Infof("You can now connect MetaMask to: http://%s", s.server.Addr)
	
	return nil
}

// Stop stops the RPC server
func (s *Server) Stop() error {
	s.logger.Infof("Stopping JSON-RPC server...")
	
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	if err := s.server.Shutdown(ctx); err != nil {
		s.logger.Errorf("Error shutting down RPC server: %v", err)
		return err
	}
	
	s.logger.Infof("JSON-RPC server stopped successfully")
	return nil
}

// GetAddr returns the server address
func (s *Server) GetAddr() string {
	return s.server.Addr
}
