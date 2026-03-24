package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const hookMarker = "websessions-hook"

// ClaudeSettings represents the relevant parts of ~/.claude/settings.json
type ClaudeSettings struct {
	raw map[string]interface{}
}

type HookEntry struct {
	Matcher string      `json:"matcher"`
	Hooks   []HookCmd   `json:"hooks"`
}

type HookCmd struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

func settingsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "settings.json")
}

func Load() (*ClaudeSettings, error) {
	data, err := os.ReadFile(settingsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return &ClaudeSettings{raw: make(map[string]interface{})}, nil
		}
		return nil, fmt.Errorf("reading settings: %w", err)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing settings: %w", err)
	}
	return &ClaudeSettings{raw: raw}, nil
}

func (s *ClaudeSettings) Save() error {
	data, err := json.MarshalIndent(s.raw, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling settings: %w", err)
	}
	return os.WriteFile(settingsPath(), data, 0644)
}

// IsInstalled checks if websessions hooks are already registered.
func (s *ClaudeSettings) IsInstalled() bool {
	hooks, ok := s.raw["hooks"].(map[string]interface{})
	if !ok {
		return false
	}
	for _, eventHooks := range hooks {
		entries, ok := eventHooks.([]interface{})
		if !ok {
			continue
		}
		for _, entry := range entries {
			entryMap, ok := entry.(map[string]interface{})
			if !ok {
				continue
			}
			hooksList, ok := entryMap["hooks"].([]interface{})
			if !ok {
				continue
			}
			for _, h := range hooksList {
				hMap, ok := h.(map[string]interface{})
				if !ok {
					continue
				}
				cmd, _ := hMap["command"].(string)
				if containsMarker(cmd) {
					return true
				}
			}
		}
	}
	return false
}

func containsMarker(cmd string) bool {
	return len(cmd) > 0 && contains(cmd, hookMarker)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Install adds websessions hooks to Claude's settings.
// Preserves all existing hooks.
func Install(baseURL string) error {
	settings, err := Load()
	if err != nil {
		return err
	}

	if settings.IsInstalled() {
		// Update the URL in case it changed
		settings.updateHookURLs(baseURL)
		return settings.Save()
	}

	hooks, ok := settings.raw["hooks"].(map[string]interface{})
	if !ok {
		hooks = make(map[string]interface{})
		settings.raw["hooks"] = hooks
	}

	// Only hook into permission prompts — the only event that truly needs attention.
	// Stop fires after every turn (too noisy), PreToolUse is informational.
	addHook(hooks, "Notification", "permission_prompt", baseURL, "waiting")

	return settings.Save()
}

func addHook(hooks map[string]interface{}, event, matcher, baseURL, eventType string) {
	// Claude Code hooks receive JSON on stdin with session_id and cwd.
	// We read stdin, extract fields with python, and POST to websessions.
	cmd := fmt.Sprintf(
		`python3 -c "import sys,json; d=json.load(sys.stdin); print(json.dumps({'event':'%s','session_id':d.get('session_id',''),'project':d.get('cwd',d.get('project',''))}))" | curl -s -X POST %s/api/hook -H "Content-Type: application/json" -d @- # %s`,
		eventType, baseURL, hookMarker,
	)

	entry := map[string]interface{}{
		"matcher": matcher,
		"hooks": []interface{}{
			map[string]interface{}{
				"type":    "command",
				"command": cmd,
			},
		},
	}

	existing, ok := hooks[event].([]interface{})
	if !ok {
		existing = []interface{}{}
	}
	existing = append(existing, entry)
	hooks[event] = existing
}

func (s *ClaudeSettings) updateHookURLs(baseURL string) {
	hooks, ok := s.raw["hooks"].(map[string]interface{})
	if !ok {
		return
	}
	for event, eventHooks := range hooks {
		entries, ok := eventHooks.([]interface{})
		if !ok {
			continue
		}
		for _, entry := range entries {
			entryMap, ok := entry.(map[string]interface{})
			if !ok {
				continue
			}
			hooksList, ok := entryMap["hooks"].([]interface{})
			if !ok {
				continue
			}
			for _, h := range hooksList {
				hMap, ok := h.(map[string]interface{})
				if !ok {
					continue
				}
				cmd, _ := hMap["command"].(string)
				if containsMarker(cmd) {
					// Rebuild the command with the new URL
					var eventType string
					switch event {
					case "Notification":
						eventType = "waiting"
					case "Stop":
						eventType = "completed"
					case "PreToolUse":
						eventType = "tool_use"
					}
					hMap["command"] = fmt.Sprintf(
						`python3 -c "import sys,json; d=json.load(sys.stdin); print(json.dumps({'event':'%s','session_id':d.get('session_id',''),'project':d.get('cwd',d.get('project',''))}))" | curl -s -X POST %s/api/hook -H "Content-Type: application/json" -d @- # %s`,
						eventType, baseURL, hookMarker,
					)
				}
			}
		}
	}
}

// Uninstall removes websessions hooks from Claude's settings.
func Uninstall() error {
	settings, err := Load()
	if err != nil {
		return err
	}

	hooks, ok := settings.raw["hooks"].(map[string]interface{})
	if !ok {
		return nil
	}

	for event, eventHooks := range hooks {
		entries, ok := eventHooks.([]interface{})
		if !ok {
			continue
		}
		var filtered []interface{}
		for _, entry := range entries {
			entryMap, ok := entry.(map[string]interface{})
			if !ok {
				filtered = append(filtered, entry)
				continue
			}
			hooksList, ok := entryMap["hooks"].([]interface{})
			if !ok {
				filtered = append(filtered, entry)
				continue
			}
			hasMarker := false
			for _, h := range hooksList {
				hMap, ok := h.(map[string]interface{})
				if !ok {
					continue
				}
				cmd, _ := hMap["command"].(string)
				if containsMarker(cmd) {
					hasMarker = true
					break
				}
			}
			if !hasMarker {
				filtered = append(filtered, entry)
			}
		}
		if len(filtered) == 0 {
			delete(hooks, event)
		} else {
			hooks[event] = filtered
		}
	}

	return settings.Save()
}
