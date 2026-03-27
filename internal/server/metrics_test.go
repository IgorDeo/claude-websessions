package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServer_Metrics(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := w.Body.String()

	checks := []string{
		"websessions_sessions_total",
		"# TYPE",
		"websessions_uptime_seconds",
		"websessions_sessions_active",
		"websessions_goroutines",
		"websessions_memory_alloc_bytes",
		"websessions_notifications_pending",
	}

	for _, check := range checks {
		if !strings.Contains(body, check) {
			t.Errorf("expected response to contain %q, but it did not\nBody:\n%s", check, body)
		}
	}

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Errorf("expected Content-Type text/plain, got %q", ct)
	}
}

func TestServer_Metrics_NoSessions(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, `websessions_sessions_total{state="none"} 0`) {
		t.Errorf("expected none state fallback when no sessions exist\nBody:\n%s", body)
	}
}
