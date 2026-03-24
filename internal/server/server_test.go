package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/IgorDeo/claude-websessions/internal/config"
	"github.com/IgorDeo/claude-websessions/internal/notification"
	"github.com/IgorDeo/claude-websessions/internal/server"
	"github.com/IgorDeo/claude-websessions/internal/session"
)

func newTestServer() *server.Server {
	cfg := &config.Config{
		Server:   config.ServerConfig{Port: 0, Host: "127.0.0.1"},
		Sessions: config.SessionsConfig{OutputBufferSize: 1024, DefaultDir: "/tmp"},
	}
	mgr := session.NewManager(1024)
	bus := notification.NewBus()
	sink := notification.NewInAppSink(100)
	return server.New(cfg, mgr, bus, sink)
}

func TestServer_IndexPage(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestServer_StaticFiles(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("GET", "/static/htmx.min.js", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
