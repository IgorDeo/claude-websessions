package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// SandboxInfo represents a Docker Desktop sandbox VM.
type SandboxInfo struct {
	Name       string   `json:"name"`
	Agent      string   `json:"agent"`
	Status     string   `json:"status"`
	SocketPath string   `json:"socket_path,omitempty"`
	Workspaces []string `json:"workspaces"`
}

// sandboxListResponse is the wrapper object returned by docker sandbox ls --json.
type sandboxListResponse struct {
	VMs []SandboxInfo `json:"vms"`
}

// IsAvailable checks if docker CLI exists and docker sandbox (Desktop) is available.
// Returns (available, version, error).
func IsAvailable() (bool, string, error) {
	_, err := exec.LookPath("docker")
	if err != nil {
		return false, "", nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "sandbox", "version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, "", nil
	}
	version := strings.TrimSpace(string(out))
	return true, version, nil
}

// FindSandboxForWorkDir returns the sandbox that has workDir in its workspaces list.
func FindSandboxForWorkDir(workDir string) (*SandboxInfo, error) {
	sandboxes, err := ListSandboxes()
	if err != nil {
		return nil, err
	}
	for i, s := range sandboxes {
		if s.Agent != "claude" {
			continue
		}
		for _, ws := range s.Workspaces {
			if ws == workDir {
				return &sandboxes[i], nil
			}
		}
	}
	return nil, nil
}

// SandboxExists checks if a named sandbox exists.
func SandboxExists(name string) (bool, error) {
	sandboxes, err := ListSandboxes()
	if err != nil {
		return false, err
	}
	for _, s := range sandboxes {
		if s.Name == name {
			return true, nil
		}
	}
	return false, nil
}

// SandboxCreate creates a new sandbox VM for the given workspace directory.
// Returns the sandbox name assigned by Docker Desktop.
func SandboxCreate(workDir string) (string, error) {
	cmd := exec.Command("docker", "sandbox", "create", "claude", workDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("sandbox create: %s: %w", strings.TrimSpace(string(out)), err)
	}

	// Look up the sandbox name that was just created for this workDir
	info, err := FindSandboxForWorkDir(workDir)
	if err != nil {
		return "", fmt.Errorf("finding created sandbox: %w", err)
	}
	if info == nil {
		return "", fmt.Errorf("sandbox created but not found in listing for %s", workDir)
	}
	return info.Name, nil
}

// SandboxCopyCredentials copies Claude credential files into the sandbox.
// Skips files that don't exist on the host.
func SandboxCopyCredentials(name string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("getting home dir: %w", err)
	}

	files := []struct {
		src string
		dst string
	}{
		{filepath.Join(home, ".claude", ".credentials.json"), "~/.claude/.credentials.json"},
		{filepath.Join(home, ".claude.json"), "~/.claude.json"},
		{filepath.Join(home, ".claude", ".settings.json"), "~/.claude/.settings.json"},
	}

	for _, f := range files {
		data, err := os.ReadFile(f.src)
		if err != nil {
			continue // skip missing files
		}
		content := strings.ReplaceAll(string(data), "'", "'\\''")
		dir := filepath.Dir(f.dst)
		script := fmt.Sprintf("mkdir -p %s && echo '%s' > %s", dir, content, f.dst)
		cmd := exec.Command("docker", "sandbox", "exec", name, "bash", "-c", script)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("copying %s: %s: %w", f.src, strings.TrimSpace(string(out)), err)
		}
	}
	return nil
}

// SandboxName derives the expected sandbox name for a workspace directory.
// Docker Desktop uses the pattern "claude-<basename>".
func SandboxName(workDir string) string {
	return "claude-" + filepath.Base(workDir)
}

// SandboxStop stops a sandbox VM.
func SandboxStop(name string) error {
	cmd := exec.Command("docker", "sandbox", "stop", name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sandbox stop: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// SandboxRemove removes a sandbox VM.
func SandboxRemove(name string) error {
	cmd := exec.Command("docker", "sandbox", "rm", name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sandbox rm: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// ListSandboxes parses output of docker sandbox ls --json.
// The output is {"vms": [...]}.
func ListSandboxes() ([]SandboxInfo, error) {
	cmd := exec.Command("docker", "sandbox", "ls", "--json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("sandbox ls: %s: %w", strings.TrimSpace(string(out)), err)
	}

	var resp sandboxListResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("parsing sandbox list: %w", err)
	}
	return resp.VMs, nil
}
