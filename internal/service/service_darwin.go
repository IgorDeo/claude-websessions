//go:build darwin

package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const plistName = "com.websessions.plist"

func plistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", plistName)
}

func IsInstalled() bool {
	_, err := os.Stat(plistPath())
	return err == nil
}

func IsActive() bool {
	out, err := exec.Command("launchctl", "list").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "com.websessions")
}

func IsEnabled() bool {
	return IsInstalled()
}

func Install() error {
	binPath, err := binaryPath()
	if err != nil {
		return fmt.Errorf("resolving binary path: %w", err)
	}

	home, _ := os.UserHomeDir()
	configPath := filepath.Join(home, ".websessions", "config.yaml")

	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.websessions</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>--config</string>
		<string>%s</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<dict>
		<key>SuccessfulExit</key>
		<false/>
	</dict>
	<key>StandardOutPath</key>
	<string>%s/.websessions/websessions.log</string>
	<key>StandardErrorPath</key>
	<string>%s/.websessions/websessions.log</string>
</dict>
</plist>
`, binPath, configPath, home, home)

	dir := filepath.Dir(plistPath())
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating LaunchAgents dir: %w", err)
	}

	if err := os.WriteFile(plistPath(), []byte(plist), 0644); err != nil {
		return fmt.Errorf("writing plist: %w", err)
	}

	return nil
}

func Uninstall() error {
	// Unload first if loaded
	exec.Command("launchctl", "unload", plistPath()).Run()

	if err := os.Remove(plistPath()); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing plist: %w", err)
	}
	return nil
}

func Enable() error {
	if !IsInstalled() {
		if err := Install(); err != nil {
			return err
		}
	}
	return exec.Command("launchctl", "load", plistPath()).Run()
}

func Disable() error {
	return exec.Command("launchctl", "unload", plistPath()).Run()
}

func Start() error {
	if !IsInstalled() {
		if err := Install(); err != nil {
			return err
		}
	}
	return exec.Command("launchctl", "load", plistPath()).Run()
}

func Stop() error {
	return exec.Command("launchctl", "unload", plistPath()).Run()
}

func Status() string {
	return statusString(IsInstalled(), IsActive(), IsEnabled())
}
