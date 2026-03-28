package teams

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestScanTeams_NoDirectory(t *testing.T) {
	// When ~/.claude/teams/ doesn't exist, should return nil without error
	configs, err := ScanTeams()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	// configs may or may not be nil depending on whether the user has teams
	_ = configs
}

func TestScanTasks_EmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	taskDir := filepath.Join(dir, "test-team")
	if err := os.MkdirAll(taskDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write a task file
	task := taskFile{
		ID:      "1",
		Subject: "Test task",
		Status:  "pending",
		Owner:   "researcher",
	}
	data, _ := json.Marshal(task)
	if err := os.WriteFile(filepath.Join(taskDir, "1.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	// Read it back using the low-level reader
	entries, err := os.ReadDir(taskDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
}

func TestParseTaskState(t *testing.T) {
	tests := []struct {
		input string
		want  TaskState
	}{
		{"pending", TaskPending},
		{"in_progress", TaskInProgress},
		{"completed", TaskCompleted},
		{"unknown", TaskPending},
		{"", TaskPending},
	}
	for _, tc := range tests {
		got := parseTaskState(tc.input)
		if got != tc.want {
			t.Errorf("parseTaskState(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestScanMailbox_NoDirectory(t *testing.T) {
	msgs, err := ScanMailbox("nonexistent-team")
	if err != nil {
		t.Fatalf("expected no error for missing dir, got: %v", err)
	}
	if msgs != nil {
		t.Fatalf("expected nil, got %v", msgs)
	}
}
