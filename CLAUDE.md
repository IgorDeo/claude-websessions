# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Git Workflow

**All changes to `main` must go through a pull request.** Do not commit directly to main. Create a feature branch, push it, and open a PR.

**Never merge a PR without explicit user approval.** After opening a PR, ask the user to test and approve it before merging. Do not auto-merge.

## Project

websessions is a web-based command center for managing multiple Claude Code CLI sessions. Single Go binary serving an htmx+Templ UI. Full spec at `docs/superpowers/specs/2026-03-23-websessions-design.md`.

## Build & Run

```bash
make build          # builds to bin/websessions (runs templ generate first)
make run            # go run with templ generate
make test           # go test ./... -v
make lint           # golangci-lint
templ generate      # regenerate templ templates (required before build)
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
