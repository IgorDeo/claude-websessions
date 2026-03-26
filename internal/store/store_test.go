package store_test

import (
	"testing"
	"time"

	"github.com/IgorDeo/claude-websessions/internal/store"
)

func TestStore_SaveAndListSessions(t *testing.T) {
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	rec := store.SessionRecord{
		ID: "s1", Name: "my-session", ClaudeID: "claude-abc", WorkDir: "/home/user/project",
		StartTime: time.Now().Add(-5 * time.Minute), EndTime: time.Now(),
		ExitCode: 0, Status: "completed",
	}
	if err := s.SaveSession(rec); err != nil {
		t.Fatal(err)
	}
	records, err := s.ListSessions(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].ID != "s1" {
		t.Errorf("expected ID s1, got %s", records[0].ID)
	}
}

func TestStore_SaveAndListNotifications(t *testing.T) {
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	n := store.NotificationRecord{
		SessionID: "s1", EventType: "completed", Timestamp: time.Now(), Read: false,
	}
	if err := s.SaveNotification(n); err != nil {
		t.Fatal(err)
	}
	records, err := s.ListNotifications(10, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(records))
	}
	if records[0].EventType != "completed" {
		t.Errorf("expected event completed, got %s", records[0].EventType)
	}
}

func TestStore_MarkNotificationRead(t *testing.T) {
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	n := store.NotificationRecord{
		SessionID: "s1", EventType: "errored", Timestamp: time.Now(), Read: false,
	}
	if err := s.SaveNotification(n); err != nil {
		t.Fatal(err)
	}
	records, _ := s.ListNotifications(10, false)
	if err := s.MarkNotificationRead(records[0].ID); err != nil {
		t.Fatal(err)
	}
	unread, _ := s.ListNotifications(10, false)
	if len(unread) != 0 {
		t.Errorf("expected 0 unread, got %d", len(unread))
	}
}

func TestStore_SaveAuditLog(t *testing.T) {
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close() //nolint:errcheck
	err = s.LogAudit("create_session", "s1", "192.168.1.1")
	if err != nil {
		t.Fatal(err)
	}
}
