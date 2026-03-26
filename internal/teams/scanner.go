package teams

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// claudeDir returns the path to ~/.claude, or empty if HOME is unset.
func claudeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude")
}

// ScanTeams reads all team config files from ~/.claude/teams/*/config.json.
// Returns an empty slice (not an error) if the directory does not exist.
func ScanTeams() ([]TeamConfig, error) {
	base := claudeDir()
	if base == "" {
		return nil, nil
	}
	teamsDir := filepath.Join(base, "teams")
	entries, err := os.ReadDir(teamsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var configs []TeamConfig
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		cfgPath := filepath.Join(teamsDir, e.Name(), "config.json")
		data, err := os.ReadFile(cfgPath)
		if err != nil {
			continue // skip unreadable or missing config
		}
		var cfg TeamConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			continue
		}
		if cfg.Name == "" {
			cfg.Name = e.Name()
		}
		configs = append(configs, cfg)
	}
	return configs, nil
}

// taskFile is the on-disk JSON format for a task in ~/.claude/tasks/{session-id}/*.json.
type taskFile struct {
	ID          string   `json:"id"`
	Subject     string   `json:"subject"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	Owner       string   `json:"owner"`
	Blocks      []string `json:"blocks"`
	BlockedBy   []string `json:"blockedBy"`
}

// ScanTasks reads task JSON files for a given session/team ID from ~/.claude/tasks/{id}/.
// Returns an empty slice if the directory does not exist.
func ScanTasks(id string) ([]Task, error) {
	base := claudeDir()
	if base == "" {
		return nil, nil
	}
	tasksDir := filepath.Join(base, "tasks", id)
	entries, err := os.ReadDir(tasksDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var tasks []Task
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(tasksDir, e.Name()))
		if err != nil {
			continue
		}
		var tf taskFile
		if err := json.Unmarshal(data, &tf); err != nil {
			continue
		}

		info, _ := e.Info()
		var modTime time.Time
		if info != nil {
			modTime = info.ModTime()
		}

		tasks = append(tasks, Task{
			ID:          tf.ID,
			Title:       tf.Subject,
			Description: tf.Description,
			State:       parseTaskState(tf.Status),
			AssignedTo:  tf.Owner,
			DependsOn:   tf.BlockedBy,
			UpdatedAt:   modTime,
		})
	}
	return tasks, nil
}

func parseTaskState(s string) TaskState {
	switch s {
	case "in_progress":
		return TaskInProgress
	case "completed":
		return TaskCompleted
	default:
		return TaskPending
	}
}

// ScanMailbox reads message files from a team's mailbox directory.
// The exact mailbox format depends on Claude Code's implementation;
// this provides a best-effort reader for JSON message files.
func ScanMailbox(teamName string) ([]Message, error) {
	base := claudeDir()
	if base == "" {
		return nil, nil
	}
	// Messages may be stored under the team directory
	mailboxDir := filepath.Join(base, "teams", teamName, "mailbox")
	entries, err := os.ReadDir(mailboxDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	type messageFile struct {
		From      string    `json:"from"`
		To        string    `json:"to"`
		Content   string    `json:"content"`
		Timestamp time.Time `json:"timestamp"`
	}

	var messages []Message
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(mailboxDir, e.Name()))
		if err != nil {
			continue
		}
		var mf messageFile
		if err := json.Unmarshal(data, &mf); err != nil {
			continue
		}
		messages = append(messages, Message{
			From:      mf.From,
			To:        mf.To,
			Content:   mf.Content,
			Timestamp: mf.Timestamp,
		})
	}
	return messages, nil
}
