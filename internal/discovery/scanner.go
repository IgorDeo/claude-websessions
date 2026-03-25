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
// by looking at .jsonl files in ~/.claude/projects/<project>/
// If processStartTime is provided, picks the file whose modification time is closest
// to (but after) the process start time, to avoid picking a different session's file
// when multiple Claude instances share the same working directory.
func ResolveClaudeSessionID(workDir string) string {
	return resolveSessionID(workDir, time.Time{})
}

// ResolveClaudeSessionIDForProcess resolves the session ID using the process start time
// to disambiguate when multiple sessions share the same working directory.
func ResolveClaudeSessionIDForProcess(workDir string, processStartTime time.Time) string {
	return resolveSessionID(workDir, processStartTime)
}

func resolveSessionID(workDir string, processStartTime time.Time) string {
	if workDir == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	projectName := strings.ReplaceAll(workDir, "/", "-")
	projectName = strings.ReplaceAll(projectName, ".", "-")
	projectDir := filepath.Join(home, ".claude", "projects", projectName)

	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return ""
	}

	type candidate struct {
		id      string
		modTime time.Time
	}
	var candidates []candidate
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		candidates = append(candidates, candidate{
			id:      strings.TrimSuffix(entry.Name(), ".jsonl"),
			modTime: info.ModTime(),
		})
	}
	if len(candidates) == 0 {
		return ""
	}

	// If no process start time, just pick the most recently modified
	if processStartTime.IsZero() {
		var bestID string
		var bestTime time.Time
		for _, c := range candidates {
			if c.modTime.After(bestTime) {
				bestTime = c.modTime
				bestID = c.id
			}
		}
		return bestID
	}

	// With process start time: pick the file that was active when the process started.
	// Find files modified after the process started (active sessions), then pick the one
	// whose mod time is closest to the start time (the one that started being written
	// around the same time the process launched).
	var bestID string
	var bestDelta time.Duration = 1<<63 - 1 // max duration
	for _, c := range candidates {
		if c.modTime.Before(processStartTime) {
			continue // file hasn't been written since process started
		}
		delta := c.modTime.Sub(processStartTime)
		if delta < bestDelta {
			bestDelta = delta
			bestID = c.id
		}
	}
	// If no files found after process start, fallback to most recent
	if bestID == "" {
		var bestTime time.Time
		for _, c := range candidates {
			if c.modTime.After(bestTime) {
				bestTime = c.modTime
				bestID = c.id
			}
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
			info.ClaudeID = ResolveClaudeSessionIDForProcess(info.WorkDir, info.StartTime)
		}
		results = append(results, *info)
	}
	return results, nil
}

func scanDarwin() ([]ProcessInfo, error) {
	// Use stable two-field format to find candidate PIDs
	out, err := exec.Command("ps", "-eo", "pid,comm").Output()
	if err != nil { return nil, fmt.Errorf("running ps: %w", err) }
	var results []ProcessInfo
	lines := strings.Split(string(out), "\n")
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 2 { continue }
		if !IsClaudeBinary(fields[1]) { continue }
		pid, err := strconv.Atoi(fields[0])
		if err != nil { continue }

		// Get full command line for this PID
		cmdOut, err := exec.Command("ps", "-o", "command=", "-p", strconv.Itoa(pid)).Output()
		if err != nil { continue }
		cmdline := strings.TrimSpace(string(cmdOut))
		info, err := ParseCmdline(cmdline)
		if err != nil { continue }
		info.PID = pid

		// Get working directory via lsof (standard on macOS)
		info.WorkDir = darwinCwd(pid)

		if info.ClaudeID == "" && info.WorkDir != "" {
			info.ClaudeID = ResolveClaudeSessionIDForProcess(info.WorkDir, info.StartTime)
		}
		results = append(results, *info)
	}
	return results, nil
}

// darwinCwd returns the current working directory of a process on macOS using lsof.
func darwinCwd(pid int) string {
	out, err := exec.Command("lsof", "-a", "-p", strconv.Itoa(pid), "-d", "cwd", "-Fn").Output()
	if err != nil { return "" }
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "n") && len(line) > 1 {
			return line[1:]
		}
	}
	return ""
}
