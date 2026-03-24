//go:build gui

package main

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

//go:embed icon.png
var iconPNG []byte

//go:embed webview-helper
var helperBin []byte

func ensureJSCSignalEnv() {} // no longer needed — helper is a separate process

func openGUI(url string) error {
	// Extract helper binary to temp dir
	tmpDir, err := os.MkdirTemp("", "websessions-gui-*")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	helperPath := filepath.Join(tmpDir, "webview-helper")
	if err := os.WriteFile(helperPath, helperBin, 0755); err != nil {
		return fmt.Errorf("writing helper binary: %w", err)
	}

	// Write icon to temp dir
	iconPath := filepath.Join(tmpDir, "icon.png")
	os.WriteFile(iconPath, iconPNG, 0644)

	// Spawn helper as a separate process — no Go runtime signal conflicts
	cmd := exec.Command(helperPath, url, iconPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
