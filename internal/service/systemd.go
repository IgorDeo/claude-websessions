package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const unitName = "websessions.service"

func unitPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user", unitName)
}

func binaryPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	// Resolve symlinks
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return exe, nil
	}
	return resolved, nil
}

// IsInstalled checks if the systemd unit file exists.
func IsInstalled() bool {
	_, err := os.Stat(unitPath())
	return err == nil
}

// IsActive checks if the service is currently running.
func IsActive() bool {
	out, err := exec.Command("systemctl", "--user", "is-active", unitName).Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "active"
}

// IsEnabled checks if the service is enabled (starts on login).
func IsEnabled() bool {
	out, err := exec.Command("systemctl", "--user", "is-enabled", unitName).Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "enabled"
}

// Install creates the systemd user unit file.
func Install() error {
	binPath, err := binaryPath()
	if err != nil {
		return fmt.Errorf("resolving binary path: %w", err)
	}

	home, _ := os.UserHomeDir()
	configPath := filepath.Join(home, ".websessions", "config.yaml")

	unit := fmt.Sprintf(`[Unit]
Description=websessions — Claude Code Session Manager
After=network.target

[Service]
Type=simple
ExecStart=%s --config %s
Restart=on-failure
RestartSec=5
Environment=HOME=%s

[Install]
WantedBy=default.target
`, binPath, configPath, home)

	// Create directory
	dir := filepath.Dir(unitPath())
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating systemd dir: %w", err)
	}

	if err := os.WriteFile(unitPath(), []byte(unit), 0644); err != nil {
		return fmt.Errorf("writing unit file: %w", err)
	}

	// Reload systemd
	return exec.Command("systemctl", "--user", "daemon-reload").Run()
}

// Uninstall removes the unit file and reloads systemd.
func Uninstall() error {
	// Stop and disable first
	exec.Command("systemctl", "--user", "stop", unitName).Run()
	exec.Command("systemctl", "--user", "disable", unitName).Run()

	if err := os.Remove(unitPath()); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing unit file: %w", err)
	}

	return exec.Command("systemctl", "--user", "daemon-reload").Run()
}

// Enable enables the service to start on login.
func Enable() error {
	return exec.Command("systemctl", "--user", "enable", unitName).Run()
}

// Disable disables the service from starting on login.
func Disable() error {
	return exec.Command("systemctl", "--user", "disable", unitName).Run()
}

// Start starts the service.
func Start() error {
	return exec.Command("systemctl", "--user", "start", unitName).Run()
}

// Stop stops the service.
func Stop() error {
	return exec.Command("systemctl", "--user", "stop", unitName).Run()
}

// Status returns a human-readable status string.
func Status() string {
	if !IsInstalled() {
		return "not installed"
	}
	active := IsActive()
	enabled := IsEnabled()
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
