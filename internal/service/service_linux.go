//go:build linux

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

func IsInstalled() bool {
	_, err := os.Stat(unitPath())
	return err == nil
}

func IsActive() bool {
	out, err := exec.Command("systemctl", "--user", "is-active", unitName).Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "active"
}

func IsEnabled() bool {
	out, err := exec.Command("systemctl", "--user", "is-enabled", unitName).Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "enabled"
}

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

	dir := filepath.Dir(unitPath())
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating systemd dir: %w", err)
	}

	if err := os.WriteFile(unitPath(), []byte(unit), 0644); err != nil {
		return fmt.Errorf("writing unit file: %w", err)
	}

	return exec.Command("systemctl", "--user", "daemon-reload").Run()
}

func Uninstall() error {
	exec.Command("systemctl", "--user", "stop", unitName).Run()
	exec.Command("systemctl", "--user", "disable", unitName).Run()

	if err := os.Remove(unitPath()); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing unit file: %w", err)
	}

	return exec.Command("systemctl", "--user", "daemon-reload").Run()
}

func Enable() error {
	return exec.Command("systemctl", "--user", "enable", unitName).Run()
}

func Disable() error {
	return exec.Command("systemctl", "--user", "disable", unitName).Run()
}

func Start() error {
	return exec.Command("systemctl", "--user", "start", unitName).Run()
}

func Stop() error {
	return exec.Command("systemctl", "--user", "stop", unitName).Run()
}

func Status() string {
	return statusString(IsInstalled(), IsActive(), IsEnabled())
}
