package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/igor-deoalves/websessions/internal/config"
	"github.com/igor-deoalves/websessions/internal/notification"
	"github.com/igor-deoalves/websessions/internal/server"
	"github.com/igor-deoalves/websessions/internal/session"
)

func newTestServer() *server.Server {
	cfg := &config.Config{
		Server:   config.ServerConfig{Port: 0, Host: "127.0.0.1"},
		Sessions: config.SessionsConfig{OutputBufferSize: 1024, DefaultDir: "/tmp"},
		Auth:     config.AuthConfig{Enabled: false},
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

func TestServer_AuthMiddleware(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{Port: 0, Host: "127.0.0.1"},
		Auth:   config.AuthConfig{Enabled: true, Token: "secret"},
	}
	mgr := session.NewManager(1024)
	bus := notification.NewBus()
	sink := notification.NewInAppSink(100)
	srv := server.New(cfg, mgr, bus, sink)
	// Without token
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without token, got %d", w.Code)
	}
	// With token
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer secret")
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with token, got %d", w.Code)
	}
}
