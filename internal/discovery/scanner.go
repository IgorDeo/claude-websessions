package discovery

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type ProcessInfo struct {
	PID       int
	Binary    string
	WorkDir   string
	Args      []string
	ClaudeID  string
	StartTime time.Time
}

func IsClaudeBinary(path string) bool {
	return filepath.Base(path) == "claude"
}

func ParseCmdline(cmdline string) (*ProcessInfo, error) {
	parts := strings.Fields(cmdline)
	if len(parts) == 0 { return nil, fmt.Errorf("empty cmdline") }
	if !IsClaudeBinary(parts[0]) { return nil, fmt.Errorf("not a claude process: %s", parts[0]) }
	info := &ProcessInfo{Binary: parts[0], Args: parts[1:]}
	for i, arg := range parts {
		switch arg {
		case "--resume":
			if i+1 < len(parts) { info.ClaudeID = parts[i+1] }
		case "--session-id":
			if i+1 < len(parts) && info.ClaudeID == "" { info.ClaudeID = parts[i+1] }
		}
	}
	return info, nil
}

// ResolveClaudeSessionID finds the active Claude session ID for a working directory
// by looking at the most recently modified .jsonl file in ~/.claude/projects/<project>/
func ResolveClaudeSessionID(workDir string) string {
	if workDir == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	// Convert path to claude's project folder: /home/user.name/foo -> -home-user-name-foo
	projectName := strings.ReplaceAll(workDir, "/", "-")
	projectName = strings.ReplaceAll(projectName, ".", "-")
	projectDir := filepath.Join(home, ".claude", "projects", projectName)

	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return ""
	}

	var bestID string
	var bestTime time.Time
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(bestTime) {
			bestTime = info.ModTime()
			bestID = strings.TrimSuffix(entry.Name(), ".jsonl")
		}
	}
	return bestID
}

func Scan() ([]ProcessInfo, error) {
	switch runtime.GOOS {
	case "linux": return scanLinux()
	case "darwin": return scanDarwin()
	default: return nil, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func scanLinux() ([]ProcessInfo, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil { return nil, fmt.Errorf("reading /proc: %w", err) }
	var results []ProcessInfo
	for _, entry := range entries {
		if !entry.IsDir() { continue }
		pid, err := strconv.Atoi(entry.Name())
		if err != nil { continue }
		cmdlineBytes, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		if err != nil { continue }
		cmdline := strings.ReplaceAll(string(cmdlineBytes), "\x00", " ")
		cmdline = strings.TrimSpace(cmdline)
		info, err := ParseCmdline(cmdline)
		if err != nil { continue }
		info.PID = pid
		cwd, err := os.Readlink(filepath.Join("/proc", entry.Name(), "cwd"))
		if err == nil { info.WorkDir = cwd }
		// Resolve session ID from project files if not in args
		if info.ClaudeID == "" && info.WorkDir != "" {
			info.ClaudeID = ResolveClaudeSessionID(info.WorkDir)
		}
		results = append(results, *info)
	}
	return results, nil
}

func scanDarwin() ([]ProcessInfo, error) {
	out, err := exec.Command("ps", "aux").Output()
	if err != nil { return nil, fmt.Errorf("running ps: %w", err) }
	var results []ProcessInfo
	lines := strings.Split(string(out), "\n")
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 11 { continue }
		pid, err := strconv.Atoi(fields[1])
		if err != nil { continue }
		command := strings.Join(fields[10:], " ")
		info, err := ParseCmdline(command)
		if err != nil { continue }
		info.PID = pid
		results = append(results, *info)
	}
	return results, nil
}
