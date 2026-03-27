package store

import (
	"path/filepath"
	"testing"
)

func TestOutputSaveLoadRoundTrip(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close() //nolint:errcheck

	sessionID := "test-session-1"
	data := []byte("hello terminal output\r\nline 2\r\n")

	if err := st.SaveOutput(sessionID, data); err != nil {
		t.Fatalf("SaveOutput: %v", err)
	}

	got, err := st.LoadOutput(sessionID)
	if err != nil {
		t.Fatalf("LoadOutput: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("LoadOutput = %q, want %q", got, data)
	}
}

func TestOutputLoadMissingReturnsNil(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close() //nolint:errcheck

	got, err := st.LoadOutput("nonexistent-session")
	if err != nil {
		t.Fatalf("LoadOutput on missing: %v", err)
	}
	if got != nil {
		t.Errorf("LoadOutput on missing = %q, want nil", got)
	}
}

func TestOutputDelete(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close() //nolint:errcheck

	sessionID := "test-session-delete"
	data := []byte("some output data")

	if err := st.SaveOutput(sessionID, data); err != nil {
		t.Fatalf("SaveOutput: %v", err)
	}

	if err := st.DeleteOutput(sessionID); err != nil {
		t.Fatalf("DeleteOutput: %v", err)
	}

	got, err := st.LoadOutput(sessionID)
	if err != nil {
		t.Fatalf("LoadOutput after delete: %v", err)
	}
	if got != nil {
		t.Errorf("LoadOutput after delete = %q, want nil", got)
	}
}

func TestOutputSaveOverwrite(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close() //nolint:errcheck

	sessionID := "test-session-overwrite"
	first := []byte("first output")
	second := []byte("second output — overwritten")

	if err := st.SaveOutput(sessionID, first); err != nil {
		t.Fatalf("SaveOutput first: %v", err)
	}
	if err := st.SaveOutput(sessionID, second); err != nil {
		t.Fatalf("SaveOutput second: %v", err)
	}

	got, err := st.LoadOutput(sessionID)
	if err != nil {
		t.Fatalf("LoadOutput: %v", err)
	}
	if string(got) != string(second) {
		t.Errorf("LoadOutput = %q, want %q", got, second)
	}
}
