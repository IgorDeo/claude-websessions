package discovery_test

import (
	"testing"

	"github.com/IgorDeo/claude-websessions/internal/discovery"
)

func TestParseProcessInfo(t *testing.T) {
	info, err := discovery.ParseCmdline("/usr/local/bin/claude --resume abc123 --session-id xyz")
	if err != nil { t.Fatal(err) }
	if info.Binary != "/usr/local/bin/claude" { t.Errorf("expected claude binary, got %s", info.Binary) }
}

func TestParseProcessInfo_NotClaude(t *testing.T) {
	_, err := discovery.ParseCmdline("/usr/bin/vim somefile.go")
	if err == nil { t.Error("expected error for non-claude process") }
}

func TestIsClaudeBinary(t *testing.T) {
	tests := []struct{ path string; expect bool }{
		{"/usr/local/bin/claude", true}, {"/home/user/.npm/bin/claude", true},
		{"/usr/bin/vim", false}, {"claude", true},
	}
	for _, tt := range tests {
		if got := discovery.IsClaudeBinary(tt.path); got != tt.expect {
			t.Errorf("IsClaudeBinary(%q) = %v, want %v", tt.path, got, tt.expect)
		}
	}
}
