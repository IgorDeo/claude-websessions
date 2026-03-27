package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// --- verifyChecksum tests ---

func TestVerifyChecksum_EmptyExpected(t *testing.T) {
	// Empty expected hash skips verification; any file (or nonexistent) should pass.
	if err := verifyChecksum("/nonexistent/path", ""); err != nil {
		t.Fatalf("expected nil for empty hash, got: %v", err)
	}
}

func TestVerifyChecksum_Correct(t *testing.T) {
	content := []byte("hello world")
	sum := sha256.Sum256(content)
	expected := hex.EncodeToString(sum[:])

	f := writeTempFile(t, content)
	if err := verifyChecksum(f, expected); err != nil {
		t.Fatalf("expected no error for correct checksum, got: %v", err)
	}
}

func TestVerifyChecksum_Mismatch(t *testing.T) {
	content := []byte("hello world")
	f := writeTempFile(t, content)

	wrongHash := "0000000000000000000000000000000000000000000000000000000000000000"
	err := verifyChecksum(f, wrongHash)
	if err == nil {
		t.Fatal("expected error for mismatched checksum, got nil")
	}
}

func TestVerifyChecksum_FileNotFound(t *testing.T) {
	err := verifyChecksum("/nonexistent/path/to/file", "abc123")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestVerifyChecksum_UppercaseHashAccepted(t *testing.T) {
	content := []byte("test data")
	sum := sha256.Sum256(content)
	// Provide uppercase hex — verifyChecksum should normalize
	expected := hex.EncodeToString(sum[:])
	expectedUpper := fmt.Sprintf("%X", sum[:])

	f := writeTempFile(t, content)
	// Lowercase should work
	if err := verifyChecksum(f, expected); err != nil {
		t.Fatalf("lowercase hash failed: %v", err)
	}
	// Uppercase should also work because we ToLower the expected
	if err := verifyChecksum(f, expectedUpper); err != nil {
		t.Fatalf("uppercase hash failed: %v", err)
	}
}

// --- safeSelfUpdate tests ---

func TestSafeSelfUpdate_RejectsEmptyData(t *testing.T) {
	err := safeSelfUpdate("/tmp/some-binary", []byte{}, "")
	if err == nil {
		t.Fatal("expected error for empty newData, got nil")
	}
}

func TestSafeSelfUpdate_BasicReplace(t *testing.T) {
	dir := t.TempDir()
	exePath := filepath.Join(dir, "mybin")

	// Write the "current" binary
	original := []byte("original binary content")
	if err := os.WriteFile(exePath, original, 0755); err != nil {
		t.Fatal(err)
	}

	newBin := []byte("new binary content v2")
	if err := safeSelfUpdate(exePath, newBin, ""); err != nil {
		t.Fatalf("safeSelfUpdate failed: %v", err)
	}

	// New binary should be in place
	got, err := os.ReadFile(exePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(newBin) {
		t.Fatalf("expected new binary, got: %s", got)
	}

	// .bak should exist for manual rollback
	bakPath := exePath + ".bak"
	bak, err := os.ReadFile(bakPath)
	if err != nil {
		t.Fatalf(".bak file should exist for manual rollback: %v", err)
	}
	if string(bak) != string(original) {
		t.Fatalf("expected original content in .bak, got: %s", bak)
	}

	// .update temp file should be gone
	if _, err := os.Stat(exePath + ".update"); !os.IsNotExist(err) {
		t.Fatal(".update temp file should be removed after successful update")
	}
}

func TestSafeSelfUpdate_ChecksumMismatchCleansUp(t *testing.T) {
	dir := t.TempDir()
	exePath := filepath.Join(dir, "mybin")

	if err := os.WriteFile(exePath, []byte("original"), 0755); err != nil {
		t.Fatal(err)
	}

	wrongHash := "0000000000000000000000000000000000000000000000000000000000000000"
	err := safeSelfUpdate(exePath, []byte("new binary"), wrongHash)
	if err == nil {
		t.Fatal("expected checksum error, got nil")
	}

	// Original binary should be unchanged
	got, _ := os.ReadFile(exePath)
	if string(got) != "original" {
		t.Fatalf("original binary should be untouched after checksum failure, got: %s", got)
	}

	// .update temp file should be cleaned up
	if _, err := os.Stat(exePath + ".update"); !os.IsNotExist(err) {
		t.Fatal(".update temp file should be removed after checksum failure")
	}
}

func TestSafeSelfUpdate_CorrectChecksumSucceeds(t *testing.T) {
	dir := t.TempDir()
	exePath := filepath.Join(dir, "mybin")

	if err := os.WriteFile(exePath, []byte("original"), 0755); err != nil {
		t.Fatal(err)
	}

	newBin := []byte("new binary with checksum")
	sum := sha256.Sum256(newBin)
	hash := hex.EncodeToString(sum[:])

	if err := safeSelfUpdate(exePath, newBin, hash); err != nil {
		t.Fatalf("expected success with correct checksum, got: %v", err)
	}

	got, _ := os.ReadFile(exePath)
	if string(got) != string(newBin) {
		t.Fatalf("expected new binary content, got: %s", got)
	}
}

// --- fetchExpectedHash tests ---

func TestFetchExpectedHash_ReturnsHashForMatchingAsset(t *testing.T) {
	checksumContent := "abc123def456  websessions-linux-amd64\ndeadbeef1234  websessions-darwin-arm64\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/releases/download/v1.2.3/checksums.txt" {
			fmt.Fprint(w, checksumContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	downloadURL := srv.URL + "/releases/download/v1.2.3/websessions-linux-amd64"
	got := fetchExpectedHash(downloadURL)
	if got != "abc123def456" {
		t.Fatalf("expected 'abc123def456', got %q", got)
	}
}

func TestFetchExpectedHash_ReturnsEmptyWhenAssetNotListed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "abc123  other-asset\n")
	}))
	defer srv.Close()

	downloadURL := srv.URL + "/releases/download/v1.2.3/websessions-linux-amd64"
	got := fetchExpectedHash(downloadURL)
	if got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestFetchExpectedHash_ReturnsEmptyOn404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	downloadURL := srv.URL + "/releases/download/v1.2.3/websessions-linux-amd64"
	got := fetchExpectedHash(downloadURL)
	if got != "" {
		t.Fatalf("expected empty string on 404, got %q", got)
	}
}

func TestFetchExpectedHash_ReturnsEmptyOnNetworkError(t *testing.T) {
	// Use a URL that won't connect
	got := fetchExpectedHash("http://127.0.0.1:1/nonexistent")
	if got != "" {
		t.Fatalf("expected empty string on network error, got %q", got)
	}
}

func TestFetchExpectedHash_EmptyURLReturnsEmpty(t *testing.T) {
	got := fetchExpectedHash("")
	if got != "" {
		t.Fatalf("expected empty string for empty URL, got %q", got)
	}
}

// --- helpers ---

func writeTempFile(t *testing.T, content []byte) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "checksum-test-*")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}
