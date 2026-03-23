# websessions — Design Spec

A web-based command center for managing multiple Claude Code CLI sessions from a single browser interface. Provides full visibility, interaction, and passive monitoring of all running sessions.

## Problem

Running multiple Claude Code sessions in terminal splits (Terminator, tmux) quickly becomes overwhelming. There's no unified way to see what each session is doing, get notified when one needs attention, or interact with many sessions without context-switching between terminal panes.

## Goals

- Launch new Claude Code sessions from a web UI
- Discover already-running Claude Code sessions and take them over via kill + `--resume`
- Full interactive terminal access to each session from the browser
- Notifications when sessions complete, error, or need input
- Split-pane view to monitor multiple sessions simultaneously
- Single binary deployment, minimal dependencies

## Non-Goals

- Full user management / RBAC (lightweight shared-token auth only)
- Cloud deployment / multi-machine orchestration
- Replacing Claude Code's own UI — this is a session manager, not a fork

## Tech Stack

| Layer | Choice | Rationale |
|-------|--------|-----------|
| Backend | Go | Excellent PTY/process management, concurrency, single binary output |
| Templates | Templ | Type-safe HTML templates that compile to Go. No JS toolchain. |
| Interactivity | htmx (~14KB) | HTML-attribute-driven interactivity, native WebSocket support |
| Terminal | xterm.js (~500KB) | Industry standard web terminal (used by VS Code). Only JS lib needed. |
| Database | SQLite (modernc.org/sqlite) | Pure Go, no CGO. Single file for history/audit. |
| Config | YAML | Human-editable, simple |
| Deployment | Single binary + Dockerfile | `go build` or `docker compose up` |

All static assets (htmx, xterm.js, CSS) are vendored and embedded via `go:embed`. No npm, no frontend build pipeline.

## Architecture

```
Browser (htmx + xterm.js)
    ↕ WebSocket + HTTP
Session Manager (Go)
    ↕ PTY
Claude Code processes
```

### Session Manager

The core component. Responsibilities:
- Maintains a registry of all sessions (map of session ID → session state)
- Allocates PTYs and spawns `claude` processes
- Scans for existing `claude` processes on startup and periodically re-discovers
- Multiplexes PTY output to connected browser clients via WebSocket
- Keeps a ring buffer per session (default ~10MB) for instant context when switching tabs

### Web Server

Built on `net/http` with a lightweight router (chi or similar):
- Serves Templ-rendered HTML pages
- Handles htmx partial page updates (tab switches, session list refresh, split pane management)
- Upgrades to WebSocket for terminal streaming and real-time notifications

### WebSocket Concurrency

- Multiple browser tabs/clients can connect to the same session simultaneously
- All viewers receive the same PTY output stream
- All viewers can send input (last-writer-wins — collaborative, not locked)
- On connect, the ring buffer contents are replayed to the new client so they see recent history
- Slow clients: if a WebSocket write exceeds a deadline (e.g., 10s), the connection is dropped. The client can reconnect and get the buffer replayed.

## Session Lifecycle

```
                                    ┌─────────┐
                                    │completed│
                                    └────▲────┘
                                         │ exit 0
discovered → takeover → running ────────┤
                ↑                        │ exit non-zero
             created ──→ running ───────►┌───────┐
                              │          │errored│
                              ▼          └───────┘
                          waiting
                         (needs input)
                              │
                              ▼
                           running
```

### States

- **discovered** — found via process scan, not yet taken over
- **takeover** — killing original process and resuming via `claude --resume`
- **created** — freshly launched from the UI
- **running** — actively producing output, fully interactive
- **waiting** — Claude is asking for input (detected heuristically, see below)
- **completed** — process exited 0
- **errored** — process exited non-zero or crashed

### Creating a new session

1. User clicks "New Session," provides a working directory and optional initial prompt
2. Session Manager allocates a PTY, spawns `claude` with given args
3. Session enters `running` state, output streams to the UI

### Discovering and taking over existing sessions

1. On startup and periodically (default 30s), scan for `claude` processes
   - **Linux:** read `/proc/*/cmdline`
   - **macOS:** fall back to `ps aux | grep claude` (discovery is cross-platform, `/proc` is not)
2. Extract metadata: PID, working directory, start time, command args
3. Extract Claude Code session ID from process args or `~/.claude/` session files
4. Skip already-tracked processes
5. Session appears in sidebar as `discovered` with metadata
6. User can click "Take Over" which:
   a. Sends SIGTERM to the original process (graceful shutdown)
   b. Waits for exit (with timeout, then SIGKILL)
   c. Spawns `claude --resume <session-id>` in a new PTY owned by websessions
   d. Session transitions to `running` with full interactive control
7. If takeover fails (e.g., can't find session ID, `--resume` fails due to stale/corrupted session), the session transitions to `errored` with a user-visible message explaining the failure

### Waiting state detection

Detecting when Claude needs input is heuristic-based and best-effort in v1. The session manager watches PTY output for known patterns:
- Tool approval prompts (e.g., "Allow", "Deny" patterns)
- Question prompts waiting for user response

This is fragile and Claude-version-dependent. False positives show an unnecessary notification; false negatives mean you don't get notified. Both are acceptable for v1.

### Process ownership

- **Owned sessions** (launched or taken over by websessions): full lifecycle control — can kill, restart
- **Discovered sessions** (not yet taken over): metadata-only in sidebar. User must explicitly take over to get interactive access.

### Cleanup

- Process exit detected via PTY EOF or `os.Process.Wait()`
- Session transitions to `completed` or `errored`
- PTY resources freed; output buffer retained until user dismisses
- Health checks periodically verify tracked PIDs are still alive

## Server Lifecycle

### Startup

1. Load config from `~/.websessions/config.yaml` (or path from `--config` flag)
2. Open/migrate SQLite database
3. Start HTTP server
4. Run initial process discovery scan
5. Restore any previously-owned sessions that are still running (matched by PID from SQLite, validated by comparing process command name and start time to prevent PID reuse collisions)

### Graceful shutdown (SIGTERM / SIGINT)

1. Stop accepting new HTTP connections
2. Close all WebSocket connections (clients see a "server shutting down" message)
3. Persist active session metadata to SQLite (PID, session ID, state)
4. **Leave spawned Claude processes running** — they are independent processes
5. Exit cleanly

On next startup, the discovery scan finds these still-running processes and offers them for takeover again. This means server restarts don't kill your Claude sessions.

## Web UI Layout

```
┌─────────────────────────────────────────────────┐
│  websessions                    🔔 (3)   ⚙️     │  ← Top bar
├────────┬────────────────────────────────────────┤
│        │  Tab1 │ Tab2 │ Tab3 │  [+]            │  ← Session tabs
│ myapp  ├────────────────────────────────────────┤
│ api    │                                        │
│ tests  │  ┌──────────────────────────────────┐  │
│ ● run  │  │                                  │  │
│ ✓ done │  │  xterm.js terminal               │  │
│ ✗ err  │  │                                  │  │
│ ⏳wait │  │                                  │  │
│        │  └──────────────────────────────────┘  │
│        │  [Status: running] [Dir: ~/myapp]      │  ← Session info bar
├────────┴────────────────────────────────────────┤
│  + New Session                                  │
└─────────────────────────────────────────────────┘
```

### Sidebar

Session list with color-coded status indicators. Active sessions on top, completed/errored below. Click to open in a tab.

### Split panes

The session area supports Terminator-style splits:

```
┌──────────────────┬─────────────────┐
│                  │                 │
│  session: myapp  │  session: api   │
│  (xterm.js)      │  (xterm.js)     │
│                  │                 │
├──────────────────┴─────────────────┤
│                                    │
│  session: tests                    │
│  (xterm.js)                        │
│                                    │
└────────────────────────────────────┘
```

- Right-click or keyboard shortcut to split horizontally/vertically
- Each pane is an independent xterm.js instance
- Drag dividers to resize (using vendored `split.js` ~2KB for reliable resize handling)
- Splits are nestable
- Double-click pane header to maximize (escape to restore)
- Each tab can have its own split layout
- Split state persisted in localStorage

### Responsive

Sidebar collapses on narrow viewports. Terminal takes full width.

## Notification System

### Event bus

Session state changes emit events internally. Notification sinks consume them.

### Events

- Session completed (exit 0)
- Session errored (exit non-zero)
- Session waiting for input (tool approval, question)

### Delivery (phase 1)

- **In-app** — badge counter on notification bell, event list in dropdown
- **Desktop** — browser Notification API. Click focuses the session.

### Extensibility

```go
type NotificationSink interface {
    Send(event SessionEvent) error
}
```

Slack, webhooks, and future integrations implement this interface. No rewrite needed.

### Preferences

Per-session and global notification settings. Stored in config file.

## Data & Configuration

### In-memory

- Active session registry (PTYs, state, output ring buffers)
- Notification queue for connected clients

### SQLite (`~/.websessions/websessions.db`)

- Session history (start time, end time, directory, exit status, duration)
- Notification history (event type, timestamp, read/unread)
- Audit log (action taken, timestamp, client IP)

### YAML (`~/.websessions/config.yaml`)

```yaml
server:
  port: 8080
  host: 0.0.0.0
sessions:
  scan_interval: 30s
  output_buffer_size: 10MB
  default_dir: ~/projects
notifications:
  desktop: true
  events: [completed, errored, waiting]
auth:
  enabled: false
  token: ""
```

### Client-side (localStorage)

- Split layouts per tab
- UI preferences

## Auth

Lightweight shared-token gate. When enabled, all requests require a bearer token. No user management. Token set via config or `WEBSESSIONS_AUTH_TOKEN` env var.

**TLS:** For non-localhost use, deploy behind a reverse proxy with TLS (nginx, Caddy). The token is sent in the clear over HTTP, so plaintext exposure on a network is a security risk.

## Error Handling

All startup-critical errors (port in use, `claude` binary not in PATH, SQLite DB inaccessible) cause the process to exit with a clear error message.

Runtime errors are handled gracefully:
- PTY allocation failure → session creation fails with error shown in UI
- SQLite write failure → logged, operation retried; if persistent, surfaced in UI
- WebSocket errors → connection closed, client reconnects automatically
- Process scan failures → logged, next scan continues normally

## Logging

Uses Go's `slog` (structured logging). Output goes to stderr by default. Log level configurable via config or `--log-level` flag (debug, info, warn, error). Default: info.

## Project Structure

```
websessions/
├── cmd/
│   └── websessions/
│       └── main.go              # Entry point, wires everything together
├── internal/
│   ├── server/                  # HTTP server, routes, WebSocket handlers
│   ├── session/                 # Session manager, PTY allocation, lifecycle
│   ├── discovery/               # Process scanner, session takeover
│   ├── notification/            # Event bus, sinks (in-app, desktop)
│   ├── store/                   # SQLite repository (history, audit)
│   └── config/                  # YAML config loading
├── web/
│   ├── templates/               # Templ files (.templ)
│   ├── static/                  # htmx.min.js, xterm.js, CSS
│   └── embed.go                 # go:embed directives
├── docs/
├── config.example.yaml
├── Dockerfile
├── Makefile
└── go.mod
```

## Key Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/creack/pty` | PTY allocation and management |
| `github.com/gorilla/websocket` | WebSocket connections |
| `github.com/a-h/templ` | Type-safe Go HTML templates |
| `modernc.org/sqlite` | Pure Go SQLite driver |
| `gopkg.in/yaml.v3` | Config parsing |
| `github.com/go-chi/chi/v5` | HTTP router |

Vendored JS (embedded, no npm): htmx.min.js, xterm.js, split.js

## Future Considerations (not in scope for v1)

- Slack / webhook notification sinks
- Session grouping / workspaces
- Session templates (predefined commands/directories)
- Session sharing between users (real-time collaboration)
- Keyboard shortcut customization
- `reptyr`-based PTY re-attach as alternative to kill+resume takeover
