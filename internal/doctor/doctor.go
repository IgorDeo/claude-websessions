package doctor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type Check struct {
	Name    string
	Status  string // "ok", "missing", "warning"
	Version string
	Detail  string
}

func RunChecks() []Check {
	var checks []Check

	checks = append(checks, checkTmux())
	checks = append(checks, checkClaude())
	checks = append(checks, checkDocker())
	checks = append(checks, checkGit())
	checks = append(checks, checkShell())
	checks = append(checks, checkSQLite())
	checks = append(checks, checkOS())
	checks = append(checks, checkConfigDir())
	checks = append(checks, checkHooks())

	return checks
}

func checkTmux() Check {
	c := Check{Name: "tmux"}
	path, err := exec.LookPath("tmux")
	if err != nil {
		c.Status = "missing"
		c.Detail = "Required. Install: sudo apt install tmux (Linux) or brew install tmux (macOS)"
		return c
	}
	out, _ := exec.Command("tmux", "-V").Output()
	c.Status = "ok"
	c.Version = strings.TrimSpace(string(out))
	c.Detail = path
	return c
}

func checkClaude() Check {
	c := Check{Name: "claude"}
	path, err := exec.LookPath("claude")
	if err != nil {
		c.Status = "missing"
		c.Detail = "Required. Install Claude Code CLI: https://claude.ai/code"
		return c
	}
	out, _ := exec.Command("claude", "--version").Output()
	ver := strings.TrimSpace(string(out))
	if ver == "" {
		ver = "installed"
	}
	// Take first line only
	if idx := strings.Index(ver, "\n"); idx > 0 {
		ver = ver[:idx]
	}
	c.Status = "ok"
	c.Version = ver
	c.Detail = path
	return c
}

func checkDocker() Check {
	c := Check{Name: "docker sandbox"}
	path, err := exec.LookPath("docker")
	if err != nil {
		c.Status = "missing"
		c.Detail = "Optional. Required for sandboxed sessions. Install Docker Desktop."
		return c
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "sandbox", "version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		c.Status = "warning"
		c.Detail = "Docker Desktop is required — docker engine alone is not enough"
		return c
	}
	c.Status = "ok"
	c.Version = strings.TrimSpace(string(out))
	c.Detail = path
	return c
}

func checkGit() Check {
	c := Check{Name: "git"}
	path, err := exec.LookPath("git")
	if err != nil {
		c.Status = "warning"
		c.Detail = "Optional. Needed for git diff viewer"
		return c
	}
	out, _ := exec.Command("git", "--version").Output()
	c.Status = "ok"
	c.Version = strings.TrimSpace(string(out))
	c.Detail = path
	return c
}

func checkShell() Check {
	c := Check{Name: "shell"}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "bash"
	}
	path, err := exec.LookPath(filepath.Base(shell))
	if err != nil {
		c.Status = "warning"
		c.Detail = "$SHELL not found: " + shell
		return c
	}
	c.Status = "ok"
	c.Version = filepath.Base(shell)
	c.Detail = path
	return c
}

func checkSQLite() Check {
	c := Check{Name: "sqlite (embedded)"}
	c.Status = "ok"
	c.Version = "modernc.org/sqlite (pure Go)"
	c.Detail = "No external dependency needed"
	return c
}

func checkOS() Check {
	c := Check{Name: "platform"}
	c.Status = "ok"
	c.Version = runtime.GOOS + "/" + runtime.GOARCH
	c.Detail = "Go " + runtime.Version()
	return c
}

func checkConfigDir() Check {
	c := Check{Name: "config directory"}
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".websessions")
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		c.Status = "ok"
		c.Detail = dir

		// Check if config.yaml exists
		configPath := filepath.Join(dir, "config.yaml")
		if _, err := os.Stat(configPath); err == nil {
			c.Version = "config.yaml found"
		} else {
			c.Version = "using defaults"
		}

		// Check if DB exists
		dbPath := filepath.Join(dir, "websessions.db")
		if info, err := os.Stat(dbPath); err == nil {
			sizeMB := info.Size() / 1024 / 1024
			if sizeMB > 0 {
				c.Detail += " (db: " + strings.TrimRight(strings.TrimRight(
					strings.ReplaceAll(string(rune('0'+sizeMB)), "\x00", ""), "0"), ".") + "MB)"
			}
		}
	} else {
		c.Status = "warning"
		c.Detail = dir + " (will be created on first run)"
	}
	return c
}

func checkHooks() Check {
	c := Check{Name: "claude hooks"}
	home, _ := os.UserHomeDir()
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	if _, err := os.Stat(settingsPath); err != nil {
		c.Status = "warning"
		c.Detail = "~/.claude/settings.json not found"
		return c
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		c.Status = "warning"
		c.Detail = "Cannot read settings.json"
		return c
	}

	if strings.Contains(string(data), "websessions-hook") {
		c.Status = "ok"
		c.Version = "installed"
		c.Detail = "Hooks registered in ~/.claude/settings.json"
	} else {
		c.Status = "warning"
		c.Version = "not installed"
		c.Detail = "Install from Settings > Claude Code Hooks"
	}
	return c
}
