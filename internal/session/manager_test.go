package session_test

import (
	"testing"
	"time"

	"github.com/igor-deoalves/websessions/internal/session"
)

func TestManager_CreateSession(t *testing.T) {
	if !session.TmuxIsAvailable() {
		t.Skip("tmux not available")
	}
	mgr := session.NewManager(10 * 1024 * 1024)

	// Use bash -c so tmux has a real shell to run
	s, err := mgr.Create("test-create", "/tmp", "bash", []string{"-c", "echo hello && sleep 2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer mgr.Kill(s.ID)

	if s.ID != "test-create" {
		t.Errorf("expected ID test-create, got %s", s.ID)
	}
	if s.GetState() != session.StateRunning {
		t.Errorf("expected state running, got %s", s.GetState())
	}
	if s.TmuxSession == "" {
		t.Error("expected non-empty TmuxSession")
	}
}

func TestManager_ListSessions(t *testing.T) {
	if !session.TmuxIsAvailable() {
		t.Skip("tmux not available")
	}
	mgr := session.NewManager(10 * 1024 * 1024)

	mgr.Create("test-list-1", "/tmp", "bash", []string{"-c", "sleep 5"})
	mgr.Create("test-list-2", "/tmp", "bash", []string{"-c", "sleep 5"})
	defer mgr.Kill("test-list-1")
	defer mgr.Kill("test-list-2")

	sessions := mgr.List()
	if len(sessions) < 2 {
		t.Errorf("expected at least 2 sessions, got %d", len(sessions))
	}
}

func TestManager_GetSession(t *testing.T) {
	if !session.TmuxIsAvailable() {
		t.Skip("tmux not available")
	}
	mgr := session.NewManager(10 * 1024 * 1024)

	mgr.Create("test-get", "/tmp", "bash", []string{"-c", "sleep 5"})
	defer mgr.Kill("test-get")

	s, ok := mgr.Get("test-get")
	if !ok {
		t.Fatal("expected to find session")
	}
	if s.ID != "test-get" {
		t.Errorf("expected ID test-get, got %s", s.ID)
	}
	_, ok = mgr.Get("nonexistent")
	if ok {
		t.Error("expected not to find nonexistent session")
	}
}

func TestManager_KillSession(t *testing.T) {
	if !session.TmuxIsAvailable() {
		t.Skip("tmux not available")
	}
	mgr := session.NewManager(10 * 1024 * 1024)

	s, err := mgr.Create("test-kill", "/tmp", "bash", []string{"-c", "sleep 60"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = mgr.Kill(s.ID)
	if err != nil {
		t.Fatalf("kill error: %v", err)
	}

	// Give tmux a moment to clean up
	time.Sleep(500 * time.Millisecond)
}
