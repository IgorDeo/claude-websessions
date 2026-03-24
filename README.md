# websessions

A web-based command center for managing multiple [Claude Code](https://claude.ai/code) CLI sessions from a single browser window. Launch, discover, monitor, and interact with all your Claude sessions in one place.

## Features

- **Full interactive terminals** — xterm.js-powered web terminals with real-time PTY streaming via WebSocket
- **Session discovery** — automatically finds running Claude Code sessions on your machine and lets you take them over
- **Session takeover** — kills a discovered session and resumes it with `--resume` so the conversation continues seamlessly
- **Split panes** — Terminator-style horizontal/vertical splits, drag tabs to edges to create splits
- **Tab management** — open multiple sessions as tabs, drag to reorder, right-click context menu (close, close & stop, close others)
- **Session history** — SQLite-backed history of all past sessions with one-click restart
- **Notifications** — real-time push notifications via WebSocket when sessions complete, error, or need input (tool approvals)
- **Claude Code hooks** — optional hooks in `~/.claude/settings.json` that notify websessions on permission prompts, completions, and tool use
- **Git diff viewer** — see `git diff` and `git status` for any session's working directory
- **Session rename** — double-click the session name in the pane header to rename
- **Directory autocomplete** — file picker with autocomplete when creating sessions or in settings
- **Resume previous sessions** — when creating a session, see and resume past Claude sessions from `~/.claude/projects/`
- **Dark theme** — Tokyo Night color scheme, monospace fonts
- **Single binary** — all assets (htmx, xterm.js, split.js, CSS) embedded via `go:embed`
- **Settings page** — configure server, notifications, scan interval, and Claude Code hooks from the UI

## Requirements

- **tmux** — required at runtime for session management
- **Claude Code CLI** — installed and available in your PATH (`claude` command)

### Build from source only

- **Go 1.26+**
- **templ** — Go HTML template engine CLI

### Install dependencies

```bash
# Install tmux (required)
# Ubuntu/Debian: sudo apt install tmux
# macOS: brew install tmux

# For building from source:
# Install Go — https://go.dev/dl/
# Install templ CLI
go install github.com/a-h/templ/cmd/templ@latest

# Install golangci-lint (optional, for linting)
# https://golangci-lint.run/usage/install/
```

## Installation

### Quick install (recommended)

```bash
curl -LsSf https://raw.githubusercontent.com/IgorDeo/claude-websessions/main/install.sh | sh
```

This detects your OS/architecture, downloads the latest binary, and installs it to `~/.local/bin` (or `/usr/local/bin` if writable). Override with `WEBSESSIONS_INSTALL_DIR` or pin a version with `WEBSESSIONS_VERSION=v0.5.0`.

### Download binary manually

Download the latest release for your platform from [GitHub Releases](https://github.com/IgorDeo/claude-websessions/releases):

```bash
# Linux (amd64)
curl -L https://github.com/IgorDeo/claude-websessions/releases/latest/download/websessions-linux-amd64 -o websessions
chmod +x websessions
./websessions

# macOS (Apple Silicon)
curl -L https://github.com/IgorDeo/claude-websessions/releases/latest/download/websessions-darwin-arm64 -o websessions
chmod +x websessions
./websessions

# macOS (Intel)
curl -L https://github.com/IgorDeo/claude-websessions/releases/latest/download/websessions-darwin-amd64 -o websessions
chmod +x websessions
./websessions
```

### Build from source

```bash
# Clone the repository
git clone https://github.com/IgorDeo/claude-websessions.git
cd claude-websessions

# Build
make build

# Run
./bin/websessions
```

Open http://localhost:8080 in your browser.

Any running Claude Code sessions on your machine will automatically appear in the sidebar.

## Build & Run

```bash
make build          # Build binary to bin/websessions (runs templ generate)
make run            # Build and run in one step
make test           # Run all tests
make lint           # Run golangci-lint
make clean          # Remove build artifacts
```

### Run with options

```bash
# Custom config file
./bin/websessions --config /path/to/config.yaml

# Debug logging
./bin/websessions --log-level debug
```

### Docker

```bash
# Build image
docker build -t websessions .

# Run
docker run -p 8080:8080 websessions
```

> Note: Docker mode can only manage sessions inside the container. For managing host sessions, run the binary directly.

## Configuration

Config file location: `~/.websessions/config.yaml` (created automatically, or copy from `config.example.yaml`)

```yaml
server:
  port: 8080
  host: 0.0.0.0        # Use 127.0.0.1 to restrict to localhost

sessions:
  scan_interval: 30s    # How often to scan for new Claude processes
  output_buffer_size: 10MB  # Ring buffer size per session
  default_dir: ~/projects   # Default working directory for new sessions

notifications:
  desktop: true         # Enable browser desktop notifications
  events:               # Which events trigger notifications
    - completed
    - errored
    - waiting
```

All settings can also be changed from the **Settings** page in the UI (gear icon in the top bar).

## Usage

### Creating a session

1. Click **+ New Session** in the footer or the **+** tab
2. Pick a **recent project** or type a working directory (autocomplete available)
3. Optionally select a previous Claude session to **resume**
4. Give it a name and an optional initial prompt
5. Click **Create** — the session opens automatically

### Discovering existing sessions

websessions scans for running `claude` processes on startup and periodically (default 30s). Discovered sessions appear in the sidebar with a **Take Over** button.

**Take Over** does:
1. Resolves the Claude session ID from `~/.claude/projects/`
2. Kills the original terminal process
3. Launches `claude --resume <session-id>` in a websessions-owned PTY
4. The conversation continues where it left off

### Tabs and splits

- **Click a session** in the sidebar to open it as a tab
- **Drag tabs** to reorder them
- **Drag a tab to the terminal area edges** (left/right/top/bottom) to create a split
- **Right-click a tab** for context menu: close tab, close & stop session, close others, close all
- **Tab close (x)** just closes the tab — the session keeps running
- Use the **split buttons** in the pane header for horizontal/vertical splits
- Use the **x button** in the pane header to close a split pane (session stays running)

### Killing a session

- **Right-click tab > Close & stop session** — kills the process, moves to history
- **Stop button (&#9632;)** in the pane header — same, with confirmation prompt
- Killed sessions appear in the **History** tab with a **Restart** button

### Renaming a session

Double-click the session name in the terminal pane header. Type the new name and press Enter. The name persists across restarts.

### Git diff viewer

Click the **&#916;** button in the pane header to see `git status` and `git diff` for that session's working directory.

### Notifications

Click the **bell icon** in the top bar to see notifications. Each notification shows:
- Session name and project directory
- Event type with icon (completed, errored, waiting)
- Relative timestamp ("2 min ago")
- Human-readable message

Click a notification to open that session. Dismiss individual notifications or clear all.

#### Claude Code Hooks (recommended)

For more reliable notifications, install hooks in Claude's settings:

1. Go to **Settings** (gear icon) > **Claude Code Hooks**
2. Click **Install Hooks**

This adds entries to `~/.claude/settings.json` that call back to websessions when:
- **Notification** (permission_prompt) — Claude needs tool approval
- **Stop** — Claude finishes its turn
- **PreToolUse** — before each tool execution

Hooks are added alongside your existing hooks and can be uninstalled cleanly.

### Session history

The **History** tab in the sidebar shows all past sessions (completed, errored, killed). Each entry has a **Restart** button that launches a new Claude session in the same directory with `--resume`.

### Settings

Access via the **gear icon** in the top bar:
- **Server** — port, host
- **Sessions** — scan interval, buffer size, default directory
- **Notifications** — desktop notifications on/off, event toggles
- **Claude Code Hooks** — install/uninstall with one click

## Architecture

```
cmd/websessions/main.go       Entry point, wires all components, signal handling
internal/config/               YAML config loading with defaults
internal/store/                SQLite (pure Go) for session history and notifications
internal/session/              Session manager: PTY lifecycle, ring buffer, state machine
internal/discovery/            Process scanner (/proc on Linux, ps on macOS), takeover
internal/notification/         Event bus with NotificationSink interface
internal/hooks/                Claude Code hooks installer for ~/.claude/settings.json
internal/server/               chi router, htmx handlers, WebSocket streaming
web/templates/                 Templ files (.templ) compiled to Go
web/static/                    Vendored JS + CSS, embedded via go:embed
```

### Key dependencies

| Package | Purpose |
|---------|---------|
| [chi](https://github.com/go-chi/chi) | HTTP router |
| [templ](https://github.com/a-h/templ) | Type-safe Go HTML templates |
| [gorilla/websocket](https://github.com/gorilla/websocket) | WebSocket connections |
| [creack/pty](https://github.com/creack/pty) | PTY allocation and management |
| [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) | Pure Go SQLite driver (no CGO) |
| [yaml.v3](https://pkg.go.dev/gopkg.in/yaml.v3) | YAML config parsing |

### Vendored frontend libraries

Embedded in the binary, no npm required:

| Library | Version | Purpose |
|---------|---------|---------|
| [htmx](https://htmx.org/) | 2.0.4 | HTML-driven interactivity |
| [xterm.js](https://xtermjs.org/) | 5.5.0 | Web terminal emulator |
| [xterm-addon-fit](https://www.npmjs.com/package/@xterm/addon-fit) | 0.10.0 | Terminal auto-resize |
| [split.js](https://split.js.org/) | 1.6.5 | Resizable split panes |

## Data storage

| What | Where |
|------|-------|
| Config | `~/.websessions/config.yaml` |
| Database | `~/.websessions/websessions.db` (SQLite) |
| Tab state | Browser localStorage |
| Sidebar order | Browser localStorage |

## Development

```bash
# Run tests
make test

# Run a single test
go test ./internal/session/... -v -run TestManager_CreateSession

# Integration tests
go test ./internal/server/... -v -tags=integration

# Regenerate templ templates (required after editing .templ files)
templ generate

# Build and run with debug logging
make build && ./bin/websessions --log-level debug
```

## License

MIT
