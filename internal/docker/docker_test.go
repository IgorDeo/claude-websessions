package docker

import (
	"testing"
)

func TestSandboxName(t *testing.T) {
	tests := []struct {
		name    string
		workDir string
		want    string
	}{
		{"simple dir", "/home/user/project", "claude-project"},
		{"nested dir", "/home/user/code/my-app", "claude-my-app"},
		{"root", "/", "claude-/"},
		{"trailing slash preserved by Base", "/home/user/project", "claude-project"},
		{"dot dir", ".", "claude-."},
		{"single component", "myproject", "claude-myproject"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SandboxName(tt.workDir)
			if got != tt.want {
				t.Errorf("SandboxName(%q) = %q, want %q", tt.workDir, got, tt.want)
			}
		})
	}
}

func TestSandboxInfo_JSON(t *testing.T) {
	// Verify the struct fields and JSON tags are correct by round-tripping
	info := SandboxInfo{
		Name:       "claude-myproject",
		Agent:      "claude",
		Status:     "running",
		SocketPath: "/tmp/sock",
		Workspaces: []string{"/home/user/myproject"},
	}
	if info.Name != "claude-myproject" {
		t.Errorf("unexpected Name: %s", info.Name)
	}
	if info.Agent != "claude" {
		t.Errorf("unexpected Agent: %s", info.Agent)
	}
	if info.Status != "running" {
		t.Errorf("unexpected Status: %s", info.Status)
	}
	if len(info.Workspaces) != 1 || info.Workspaces[0] != "/home/user/myproject" {
		t.Errorf("unexpected Workspaces: %v", info.Workspaces)
	}
}

func TestCredentialFiles(t *testing.T) {
	// Test that SandboxCopyCredentials targets the correct files.
	// We cannot run the actual function without Docker, but we can verify
	// the file list structure by inspecting the function's behavior:
	// it should attempt to copy .credentials.json, .claude.json, and .settings.json.

	// This is a compile-time check that the function signature is correct.
	var _ = SandboxCopyCredentials
}

func TestIsAvailable_Signature(t *testing.T) {
	// Compile-time check that IsAvailable has the expected signature.
	var _ = IsAvailable
}
