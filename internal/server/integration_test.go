//go:build integration

package server_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/IgorDeo/claude-websessions/internal/config"
	"github.com/IgorDeo/claude-websessions/internal/notification"
	"github.com/IgorDeo/claude-websessions/internal/server"
	"github.com/IgorDeo/claude-websessions/internal/session"
)

func TestIntegration_CreateSessionAndStream(t *testing.T) {
	cfg := &config.Config{
		Server:   config.ServerConfig{Port: 0, Host: "127.0.0.1"},
		Sessions: config.SessionsConfig{OutputBufferSize: 1024 * 1024, DefaultDir: "/tmp"},
		Auth:     config.AuthConfig{Enabled: false},
	}
	mgr := session.NewManager(cfg.Sessions.OutputBufferSize)
	bus := notification.NewBus()
	sink := notification.NewInAppSink(100)
	srv := server.New(cfg, mgr, bus, sink)

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	_, err := mgr.Create("test-echo", "/tmp", "echo", []string{"hello from integration test"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws/test-echo"
	u, _ := url.Parse(wsURL)

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ws read: %v", err)
	}

	if !strings.Contains(string(msg), "hello from integration test") {
		t.Errorf("expected echo output in WS message, got: %q", string(msg))
	}

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}
