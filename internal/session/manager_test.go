package session_test

import (
	"testing"

	"github.com/igor-deoalves/websessions/internal/session"
)

func TestManager_CreateSession(t *testing.T) {
	mgr := session.NewManager(10 * 1024 * 1024)
	s, err := mgr.Create("test-session", "/tmp", "echo", []string{"hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.ID != "test-session" {
		t.Errorf("expected ID test-session, got %s", s.ID)
	}
	if s.GetState() != session.StateRunning {
		t.Errorf("expected state running, got %s", s.GetState())
	}
	if s.PID == 0 {
		t.Error("expected non-zero PID")
	}
	mgr.Wait(s.ID)
	if s.GetState() != session.StateCompleted {
		t.Errorf("expected state completed, got %s", s.GetState())
	}
}

func TestManager_ListSessions(t *testing.T) {
	mgr := session.NewManager(10 * 1024 * 1024)
	mgr.Create("s1", "/tmp", "echo", []string{"1"})
	mgr.Create("s2", "/tmp", "echo", []string{"2"})
	sessions := mgr.List()
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestManager_GetSession(t *testing.T) {
	mgr := session.NewManager(10 * 1024 * 1024)
	mgr.Create("test", "/tmp", "echo", []string{"hi"})
	s, ok := mgr.Get("test")
	if !ok {
		t.Fatal("expected to find session")
	}
	if s.ID != "test" {
		t.Errorf("expected ID test, got %s", s.ID)
	}
	_, ok = mgr.Get("nonexistent")
	if ok {
		t.Error("expected not to find nonexistent session")
	}
}

func TestManager_KillSession(t *testing.T) {
	mgr := session.NewManager(10 * 1024 * 1024)
	s, err := mgr.Create("sleepy", "/tmp", "sleep", []string{"60"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err = mgr.Kill(s.ID)
	if err != nil {
		t.Fatalf("kill error: %v", err)
	}
	mgr.Wait(s.ID)
	if s.GetState() != session.StateErrored {
		t.Errorf("expected state errored after kill, got %s", s.GetState())
	}
}
