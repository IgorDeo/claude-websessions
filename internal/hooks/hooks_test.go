package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// settingsPathOverride allows tests to redirect settings I/O to a temp file.
// The hooks package uses settingsPath() which reads os.UserHomeDir(), so we
// override HOME for each test to isolate it.

func setupTestHome(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	claudeDir := filepath.Join(tmp, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", tmp)
	return tmp
}

func TestInstallCreatesHooks(t *testing.T) {
	home := setupTestHome(t)

	err := Install("http://localhost:8080")
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Verify settings file was created
	data, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("reading settings: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("parsing settings: %v", err)
	}

	hooks, ok := raw["hooks"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'hooks' key in settings")
	}

	notif, ok := hooks["Notification"].([]interface{})
	if !ok {
		t.Fatal("expected 'Notification' key in hooks")
	}
	if len(notif) != 1 {
		t.Fatalf("expected 1 Notification entry, got %d", len(notif))
	}

	entry := notif[0].(map[string]interface{})
	if entry["matcher"] != "permission_prompt" {
		t.Errorf("unexpected matcher: %v", entry["matcher"])
	}

	hooksList := entry["hooks"].([]interface{})
	if len(hooksList) != 1 {
		t.Fatalf("expected 1 hook command, got %d", len(hooksList))
	}

	hookCmd := hooksList[0].(map[string]interface{})
	if hookCmd["type"] != "command" {
		t.Errorf("unexpected hook type: %v", hookCmd["type"])
	}
	cmd := hookCmd["command"].(string)
	if !containsMarker(cmd) {
		t.Error("hook command missing websessions-hook marker")
	}
	if !contains(cmd, "http://localhost:8080") {
		t.Error("hook command missing base URL")
	}
}

func TestIsInstalled(t *testing.T) {
	setupTestHome(t)

	settings, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if settings.IsInstalled() {
		t.Error("expected IsInstalled=false on empty settings")
	}

	if err := Install("http://localhost:8080"); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	settings, err = Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !settings.IsInstalled() {
		t.Error("expected IsInstalled=true after Install")
	}
}

func TestInstallIdempotent(t *testing.T) {
	setupTestHome(t)

	if err := Install("http://localhost:8080"); err != nil {
		t.Fatalf("first Install failed: %v", err)
	}
	if err := Install("http://localhost:8080"); err != nil {
		t.Fatalf("second Install failed: %v", err)
	}

	settings, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	hooks := settings.raw["hooks"].(map[string]interface{})
	notif := hooks["Notification"].([]interface{})
	if len(notif) != 1 {
		t.Errorf("expected 1 Notification entry after double install, got %d", len(notif))
	}
}

func TestInstallUpdatesURL(t *testing.T) {
	setupTestHome(t)

	if err := Install("http://localhost:8080"); err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	if err := Install("http://localhost:9090"); err != nil {
		t.Fatalf("Install with new URL failed: %v", err)
	}

	settings, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	hooks := settings.raw["hooks"].(map[string]interface{})
	notif := hooks["Notification"].([]interface{})
	entry := notif[0].(map[string]interface{})
	hooksList := entry["hooks"].([]interface{})
	cmd := hooksList[0].(map[string]interface{})["command"].(string)

	if !contains(cmd, "http://localhost:9090") {
		t.Error("expected URL to be updated to 9090")
	}
	if contains(cmd, "http://localhost:8080") {
		t.Error("old URL 8080 should not be present")
	}
}

func TestUninstall(t *testing.T) {
	setupTestHome(t)

	if err := Install("http://localhost:8080"); err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	if err := Uninstall(); err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}

	settings, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if settings.IsInstalled() {
		t.Error("expected IsInstalled=false after Uninstall")
	}

	// hooks key should be cleaned up (Notification removed since it's empty)
	hooks, ok := settings.raw["hooks"].(map[string]interface{})
	if ok && len(hooks) > 0 {
		t.Errorf("expected hooks map to be empty after uninstall, got %v", hooks)
	}
}

func TestUninstallNoSettingsFile(t *testing.T) {
	setupTestHome(t)

	// Uninstall when no settings file exists should not error
	err := Uninstall()
	if err != nil {
		t.Fatalf("Uninstall with no settings file should not error: %v", err)
	}
}

func TestUninstallPreservesOtherHooks(t *testing.T) {
	home := setupTestHome(t)

	// Write settings with both websessions hooks and custom hooks
	settings := map[string]interface{}{
		"hooks": map[string]interface{}{
			"Notification": []interface{}{
				map[string]interface{}{
					"matcher": "custom_matcher",
					"hooks": []interface{}{
						map[string]interface{}{
							"type":    "command",
							"command": "echo custom hook",
						},
					},
				},
			},
		},
	}
	data, _ := json.MarshalIndent(settings, "", "  ")
	_ = os.WriteFile(filepath.Join(home, ".claude", "settings.json"), data, 0644)

	// Install websessions hooks
	if err := Install("http://localhost:8080"); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Uninstall should only remove websessions hooks
	if err := Uninstall(); err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	hooks := loaded.raw["hooks"].(map[string]interface{})
	notif := hooks["Notification"].([]interface{})
	if len(notif) != 1 {
		t.Fatalf("expected 1 remaining hook, got %d", len(notif))
	}
	entry := notif[0].(map[string]interface{})
	hooksList := entry["hooks"].([]interface{})
	cmd := hooksList[0].(map[string]interface{})["command"].(string)
	if cmd != "echo custom hook" {
		t.Errorf("custom hook was modified: %s", cmd)
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	home := setupTestHome(t)

	// Write invalid JSON
	_ = os.WriteFile(filepath.Join(home, ".claude", "settings.json"), []byte("{invalid"), 0644)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoadMissingFile(t *testing.T) {
	setupTestHome(t)

	settings, err := Load()
	if err != nil {
		t.Fatalf("Load should not error for missing file: %v", err)
	}
	if settings.IsInstalled() {
		t.Error("expected IsInstalled=false for empty settings")
	}
}

func TestContainsMarker(t *testing.T) {
	tests := []struct {
		cmd  string
		want bool
	}{
		{"", false},
		{"echo hello", false},
		{"curl http://example.com # websessions-hook", true},
		{"websessions-hook", true},
		{"prefix-websessions-hook-suffix", true},
	}
	for _, tt := range tests {
		got := containsMarker(tt.cmd)
		if got != tt.want {
			t.Errorf("containsMarker(%q) = %v, want %v", tt.cmd, got, tt.want)
		}
	}
}
