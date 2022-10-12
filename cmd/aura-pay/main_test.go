package main

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

func TestRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	killServer := newKillServer(":19999", cancel)
	go killServer.Start()
	go runService(ctx)

	<-ctx.Done()

	killServer.server.Shutdown(context.Background())
}

func newKillServer(addr string, cancel context.CancelFunc) *killServer {
	return &killServer{
		server: http.Server{
			Addr: addr,
		},
		cancel: cancel,
	}
}

func (s *killServer) Start() {
	s.server.Handler = s

	if err := s.server.ListenAndServe(); err != nil {
		fmt.Printf("killServer Error: %s", err)
	}
}

func (s *killServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	s.cancel()
}

type killServer struct {
	server http.Server
	cancel context.CancelFunc
}
