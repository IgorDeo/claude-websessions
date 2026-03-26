package session_test

import (
	"testing"

	"github.com/IgorDeo/claude-websessions/internal/session"
)

func TestRingBuf_WriteAndRead(t *testing.T) {
	rb := session.NewRingBuf(1024)
	data := []byte("hello world")
	n, err := rb.Write(data)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(data) {
		t.Errorf("wrote %d, expected %d", n, len(data))
	}
	got := rb.Bytes()
	if string(got) != "hello world" {
		t.Errorf("got %q, want %q", got, "hello world")
	}
}

func TestRingBuf_Overflow(t *testing.T) {
	rb := session.NewRingBuf(10)
	_, _ = rb.Write([]byte("abcdefghij")) // fills buffer
	_, _ = rb.Write([]byte("XYZ"))        // overwrites first 3 bytes
	got := rb.Bytes()
	if string(got) != "defghijXYZ" {
		t.Errorf("got %q, want %q", got, "defghijXYZ")
	}
}

func TestRingBuf_Empty(t *testing.T) {
	rb := session.NewRingBuf(1024)
	got := rb.Bytes()
	if len(got) != 0 {
		t.Errorf("expected empty, got %d bytes", len(got))
	}
}

func TestRingBuf_ExactFill(t *testing.T) {
	rb := session.NewRingBuf(5)
	_, _ = rb.Write([]byte("abcde"))
	got := rb.Bytes()
	if string(got) != "abcde" {
		t.Errorf("got %q, want %q", got, "abcde")
	}
}

func TestRingBuf_MultipleSmallWrites(t *testing.T) {
	rb := session.NewRingBuf(10)
	_, _ = rb.Write([]byte("abc"))
	_, _ = rb.Write([]byte("def"))
	_, _ = rb.Write([]byte("ghij"))
	// buffer is full: "abcdefghij"
	_, _ = rb.Write([]byte("kl"))
	// overwrites: "cdefghijkl"
	got := rb.Bytes()
	if string(got) != "cdefghijkl" {
		t.Errorf("got %q, want %q", got, "cdefghijkl")
	}
}
