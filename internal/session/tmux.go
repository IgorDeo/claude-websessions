package session

import (
	"fmt"
	"os/exec"
	"strings"
)

const tmuxPrefix = "ws-"

// TmuxSessionName returns the tmux session name for a websessions session ID.
func TmuxSessionName(id string) string {
	// Sanitize: tmux session names can't contain dots or colons
	name := tmuxPrefix + id
	name = strings.ReplaceAll(name, ".", "_")
	name = strings.ReplaceAll(name, ":", "_")
	name = strings.ReplaceAll(name, " ", "_")
	return name
}

// tmuxRun runs a tmux command and returns its output.
func tmuxRun(args ...string) (string, error) {
	cmd := exec.Command("tmux", args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// tmuxSessionExists checks if a tmux session exists.
func tmuxSessionExists(name string) bool {
	_, err := tmuxRun("has-session", "-t", name)
	return err == nil
}

// tmuxCreateSession creates a new tmux session running the given command.
func tmuxCreateSession(name, workDir, command string, args []string) error {
	tmuxArgs := []string{
		"new-session",
		"-d",           // detached
		"-s", name,     // session name
		"-x", "200",    // initial width
		"-y", "50",     // initial height
	}
	if workDir != "" {
		tmuxArgs = append(tmuxArgs, "-c", workDir)
	}
	// The command to run inside tmux
	fullCmd := command
	if len(args) > 0 {
		fullCmd += " " + shellJoin(args)
	}
	tmuxArgs = append(tmuxArgs, fullCmd)

	_, err := tmuxRun(tmuxArgs...)
	if err != nil {
		return err
	}

	tmuxConfigureSession(name)
	return nil
}

// tmuxConfigureSession applies standard configuration to a tmux session
// for clean embedded use within websessions. This is called both when
// creating new sessions and when reattaching to existing ones.
func tmuxConfigureSession(name string) {
	_, _ = tmuxRun("set-option", "-t", name, "status", "off")
	_, _ = tmuxRun("set-option", "-t", name, "default-terminal", "xterm-256color")
	// Use the largest client size (not smallest) so the window expands to fill
	_, _ = tmuxRun("set-window-option", "-t", name, "aggressive-resize", "on")
	// Signal wait-for channel when pane dies for fast exit detection
	_, _ = tmuxRun("set-hook", "-t", name, "pane-died", "run-shell 'tmux wait-for -S "+name+"-done'")

	// Enable mouse so clicks can select panes. This is required for Agent
	// Teams mode where Claude Code creates multiple tmux panes for subagents.
	// Without mouse support, users cannot click to select the main pane
	// where permission prompts appear.
	_, _ = tmuxRun("set-option", "-t", name, "mouse", "on")
	// Disable prefix key to prevent tmux from intercepting input
	_, _ = tmuxRun("set-option", "-t", name, "prefix", "None")
	_, _ = tmuxRun("set-option", "-t", name, "prefix2", "None")
	// Disable mouse-wheel scrollback in tmux (xterm.js manages its own scrollback)
	_, _ = tmuxRun("unbind-key", "-T", "root", "WheelUpPane")
	_, _ = tmuxRun("unbind-key", "-T", "root", "WheelDownPane")
}

// tmuxKillSession kills a tmux session.
func tmuxKillSession(name string) error {
	_, err := tmuxRun("kill-session", "-t", name)
	return err
}

// tmuxSendKeys sends input to a tmux session.
func tmuxSendKeys(name string, keys string) error {
	// Use send-keys -l for literal text (no key name interpretation)
	_, err := tmuxRun("send-keys", "-t", name, "-l", keys)
	return err
}

// tmuxResizeWindow resizes a tmux session's window.
func tmuxResizeWindow(name string, cols, rows int) error {
	_, err := tmuxRun("resize-window", "-t", name, "-x", fmt.Sprintf("%d", cols), "-y", fmt.Sprintf("%d", rows))
	return err
}

// tmuxListSessions lists all tmux sessions with the ws- prefix.
func tmuxListSessions() ([]string, error) {
	out, err := tmuxRun("ls", "-F", "#{session_name}")
	if err != nil {
		// tmux ls fails if no sessions exist
		return nil, nil
	}
	var sessions []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, tmuxPrefix) {
			sessions = append(sessions, line)
		}
	}
	return sessions, nil
}

// tmuxIsAvailable checks if tmux is installed.
func TmuxIsAvailable() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

// shellJoin joins args with proper quoting for shell execution.
func shellJoin(args []string) string {
	var parts []string
	for _, arg := range args {
		if strings.ContainsAny(arg, " \t\n\"'\\$`!#&|;(){}") {
			// Quote the argument
			escaped := strings.ReplaceAll(arg, "'", "'\\''")
			parts = append(parts, "'"+escaped+"'")
		} else {
			parts = append(parts, arg)
		}
	}
	return strings.Join(parts, " ")
}
