package service

import (
	"os"
	"path/filepath"
	"runtime"
)

// Name returns the platform service manager name for display.
func Name() string {
	if runtime.GOOS == "darwin" {
		return "launchd"
	}
	return "systemd"
}

func binaryPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return exe, nil
	}
	return resolved, nil
}

// statusString returns a human-readable status from the active/enabled flags.
func statusString(installed, active, enabled bool) string {
	if !installed {
		return "not installed"
	}
	switch {
	case active && enabled:
		return "running (enabled)"
	case active && !enabled:
		return "running (not enabled)"
	case !active && enabled:
		return "stopped (enabled)"
	default:
		return "stopped (disabled)"
	}
}
