# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Git Workflow

**All changes to `main` must go through a pull request.** Do not commit directly to main. Create a feature branch, push it, and open a PR.

**Never merge a PR without explicit user approval.** After opening a PR, ask the user to test and approve it before merging. Do not auto-merge.

## Project

websessions is a web-based command center for managing multiple Claude Code CLI sessions. Single Go binary serving an htmx+Templ UI. Full spec at `docs/superpowers/specs/2026-03-23-websessions-design.md`.

## Build & Run

```bash
mise run build      # builds to bin/websessions (runs templ generate first)
mise run run        # go run with templ generate
mise run test       # go test ./... -v
mise run lint       # golangci-lint
mise run generate   # regenerate templ templates (required before build)
mise run setup      # install dev tools (templ CLI)
```

Run a single test:
```bash
go test ./internal/session/... -v -run TestManager_CreateSession
```

Integration tests (require `integration` build tag):
```bash
go test ./internal/server/... -v -tags=integration
```

## Architecture

```
cmd/websessions/main.go     → entry point, wires all components, signal handling
internal/config/             → YAML config loading with defaults and env overrides
internal/store/              → SQLite (modernc.org/sqlite, pure Go) for history/audit
internal/session/            → Session manager: PTY lifecycle, ring buffer, state machine
internal/discovery/          → Process scanner (/proc on Linux, ps on macOS), kill+resume takeover
internal/notification/       → Event bus with NotificationSink interface (in-app sink for v1)
internal/server/             → chi router, htmx handlers, WebSocket terminal streaming, auth middleware
web/templates/               → Templ files (.templ) — must run `templ generate` before building
web/static/                  → Vendored JS (htmx, xterm.js, split.js) + CSS, embedded via go:embed
```

Key patterns:
- Session state machine in `internal/session/session.go` — all transitions validated
- WebSocket hub in `internal/server/ws.go` — multiplexes PTY output to multiple browser clients
- Ring buffer in `internal/session/ringbuf.go` — replayed to new WS clients on connect
- State changes fire through notification.Bus → sinks + SQLite persistence

## Documentation

When modifying code that changes a documented flow, **update the corresponding doc** in the same PR. Stale docs are worse than no docs.

| Doc | Covers | Update when changing... |
|-----|--------|------------------------|
| `docs/notifications-system.md` | Notification bus, sinks, hooks, WebSocket push, reminder/snooze, UI components | `internal/notification/`, hook callbacks in `handlers.go`, notification WS in `ws.go`, notification JS/templates, reminder logic in `main.go` |
| `docs/session-lifecycle.md` | State machine, session types, creation, kill/restart, tmux, PTY, ring buffer, auto-cleanup, offline recovery | `internal/session/`, session handlers in `handlers.go`, `internal/docker/`, state change wiring in `main.go` |
| `docs/terminal-streaming.md` | WebSocket hub, xterm.js, resize, ring buffer replay, alt screen filtering, theming | `internal/server/ws.go`, terminal JS in `app.js` (connectSession, openWS), `internal/session/ringbuf.go` |
| `docs/tabs-and-splits.md` | Tab bar, split tree, persistence, drag-drop, iframe panes, focus management | Split/tab JS in `app.js`, `web/templates/terminal.templ`, `web/templates/iframe.templ`, `web/templates/tabs.templ`, tab-related handlers |
| `docs/discovery-and-takeover.md` | Process scanning, Claude project discovery, hook session resolution, takeover | `internal/discovery/`, discovery loop in `main.go`, `handleHookCallback`/`handleTakeover`/`handleClaudeSessions` in `handlers.go` |
| `docs/configuration-and-platform.md` | Config, SQLite store, hooks install, service management, self-update, Docker sandbox, doctor checks | `internal/config/`, `internal/store/`, `internal/hooks/`, `internal/service/`, `internal/updater/`, `internal/doctor/`, `install.sh`, settings handlers |
| `docs/agent-teams.md` | Agent teams integration, team discovery, task board, messaging, team hooks, dashboard UI | `internal/teams/`, team handlers in `handlers_teams.go`, `render_teams.go`, team WS in `ws.go`, team JS in `app.js`, team config in `config.go` |

## Browser Testing (Claude-in-Chrome)

When a Chrome browser session is available (claude-in-chrome MCP tools), **always verify UI changes in the browser after implementing them**:

1. **Run the server** — Start the server in the background before testing:
   ```bash
   make build && ./bin/websessions &
   # or for GUI mode:
   make build-gui && ./bin/websessions --gui &
   ```

2. **Restart after changes** — After modifying code, rebuild and restart:
   ```bash
   kill %1 2>/dev/null; sleep 1; make build && ./bin/websessions &
   ```

3. **Navigate and inspect** — Use chrome tools to navigate to `http://localhost:8080`, read the DOM, execute JavaScript, and verify the rendered output matches expectations.

4. **Test the fix** — Don't just build and commit. Verify the change works by:
   - Navigating to the page
   - Reading the relevant DOM elements with `read_page` or `javascript_tool`
   - Checking computed styles, element counts, nesting, event handlers
   - Testing interactive behavior (clicks, toggles, etc.)

5. **Iterate** — If the browser shows the fix didn't work, investigate in the browser (check raw HTML, computed styles, event handlers) before making more code changes.
