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

	// Hook into permission prompts (needs immediate attention) and stop events
	// (session finished — triggers completion notification).
	addHook(hooks, "Notification", "permission_prompt", baseURL, "waiting")
	addHook(hooks, "Stop", "", baseURL, "stop")

	return settings.Save()
}

func addHook(hooks map[string]interface{}, event, matcher, baseURL, eventType string) {
	// Claude Code hooks receive JSON on stdin with session_id and cwd.
	// We read stdin, extract fields with python, and POST to websessions.
	cmd := fmt.Sprintf(
		`python3 -c "import sys,json; d=json.load(sys.stdin); print(json.dumps({'event':'%s','session_id':d.get('session_id',''),'project':d.get('cwd',d.get('project','')),'stop_hook_active':d.get('stop_hook_active',False)}))" | curl -s -X POST %s/api/hook -H "Content-Type: application/json" -d @- # %s`,
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
						eventType = "stop"
					case "PreToolUse":
						eventType = "tool_use"
					}
					hMap["command"] = fmt.Sprintf(
						`python3 -c "import sys,json; d=json.load(sys.stdin); print(json.dumps({'event':'%s','session_id':d.get('session_id',''),'project':d.get('cwd',d.get('project','')),'stop_hook_active':d.get('stop_hook_active',False)}))" | curl -s -X POST %s/api/hook -H "Content-Type: application/json" -d @- # %s`,
						eventType, baseURL, hookMarker,
					)
				}
			}
		}
	}
}

// IsPlannotatorIntegrated checks if PLANNOTATOR_BROWSER is set to ws-open-url.
func (s *ClaudeSettings) IsPlannotatorIntegrated() bool {
	env, ok := s.raw["env"].(map[string]interface{})
	if !ok {
		return false
	}
	val, _ := env["PLANNOTATOR_BROWSER"].(string)
	return contains(val, "ws-open-url")
}

const wsOpenURLScript = `#!/bin/sh
# ws-open-url: Opens a URL in a websessions iframe pane.
# Used as PLANNOTATOR_BROWSER to embed plannotator plans in websessions.
# Auto-generated by websessions — do not edit.
URL="$1"
WS_HOST="${WEBSESSIONS_HOST:-localhost:8080}"
if [ -z "$URL" ]; then
  echo "Usage: ws-open-url <url>" >&2
  exit 1
fi
curl -s -X POST "http://${WS_HOST}/api/panes/iframe" \
  -H "Content-Type: application/json" \
  -d "{\"url\":\"$URL\",\"title\":\"Plan Review\"}" \
  -o /dev/null -w "" 2>/dev/null || true
`

// wsOpenURLPath returns a stable path for the ws-open-url script.
func wsOpenURLPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "bin", "ws-open-url")
}

// SetPlannotatorIntegration enables or disables the PLANNOTATOR_BROWSER env var.
// On enable, it writes the ws-open-url script to ~/.local/bin/ so the path is
// stable regardless of how/where websessions was built or installed.
func SetPlannotatorIntegration(enable bool) error {
	scriptPath := wsOpenURLPath()

	if enable {
		dir := filepath.Dir(scriptPath)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
		if err := os.WriteFile(scriptPath, []byte(wsOpenURLScript), 0o755); err != nil {
			return fmt.Errorf("write %s: %w", scriptPath, err)
		}
	} else {
		_ = os.Remove(scriptPath) // best-effort cleanup
	}

	settings, err := Load()
	if err != nil {
		return err
	}

	env, ok := settings.raw["env"].(map[string]interface{})
	if !ok {
		env = make(map[string]interface{})
		settings.raw["env"] = env
	}

	if enable {
		env["PLANNOTATOR_BROWSER"] = scriptPath
	} else {
		delete(env, "PLANNOTATOR_BROWSER")
		if len(env) == 0 {
			delete(settings.raw, "env")
		}
	}

	return settings.Save()
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
