# TODO

Tracked gaps and improvements for websessions. Pick what matters most to you.

---

## Critical

These affect security, reliability, or core usability.

- [ ] **Add authentication** — Basic auth or token-based auth for when bound to `0.0.0.0`. Config-defined password/token. Without this, anyone on the network can control all sessions.
- [ ] **WebSocket reconnection for terminals** — Terminal WebSocket doesn't reconnect on disconnect (notification WS does). A network blip kills the terminal feed permanently. Add auto-reconnect with backoff.
- [ ] **Keyboard shortcuts** — No Ctrl+Tab (switch tabs), Ctrl+W (close tab), Ctrl+N (new session), Ctrl+T (new terminal). Power users expect these.
- [ ] **Tab overflow/scrolling** — With many sessions open, tabs shrink infinitely instead of scrolling or collapsing. Add horizontal scroll or overflow menu.

## High

Noticeably missing features or reliability issues.

- [ ] **Persist terminal output** — Ring buffer is in-memory only. Server restart loses all output. Persist to disk or SQLite so users can scroll back after restart.
- [ ] **Session search/filter** — Sidebar has no search. With 10+ sessions it's unusable. Add a search box that filters by name or working directory.
- [ ] **Loading states** — No spinner or feedback when creating sessions, taking over, or restarting. UI feels frozen during these operations.
- [ ] **Run tests in CI** — Release workflow only builds, never runs `make test`. A broken test could ship unnoticed.
- [ ] **Health endpoint** — Add `/health` or `/ready` endpoint for monitoring, load balancers, and systemd health checks.
- [ ] **Safe self-update** — Current self-update overwrites the running binary with no rollback. Download to temp file, verify checksum, then atomic rename.
- [ ] **Pin templ version in CI** — `go install templ@latest` could break the build. Pin to a specific version.

## Medium

Quality of life and hardening.

- [ ] **Terminal font size control** — Add Ctrl+/Ctrl- zoom for terminal font size, or a UI control in the pane header.
- [ ] **SQLite WAL mode** — Enable `PRAGMA journal_mode=WAL` to reduce contention on concurrent reads/writes.
- [ ] **CSRF protection** — POST endpoints accept requests from any origin. Add CSRF tokens or same-origin checks.
- [ ] **Favicon badge** — Show unread notification count in the browser tab favicon so background tabs show activity.
- [ ] **Fix discovery duplicates** — A managed session and its discovered process can appear as separate sidebar entries. Deduplicate by PID or working directory.
- [ ] **First-run onboarding** — Empty state shows "Select a session" but no guidance for new users. Add a getting-started message with key actions.
- [ ] **Cache update check response** — Update checker hits GitHub API every 30 minutes unauthenticated (60 req/hr limit). Cache the response for the interval period.
- [ ] **Clean up sound temp files** — WAV files in /tmp leak if the process is killed without graceful shutdown. Add signal handler or use a fixed path.
- [ ] **Auto-cleanup stale sessions** — Completed/errored sessions sit in the active list. Auto-archive to history after a configurable timeout.

## Low

Polish and nice-to-haves.

- [ ] **Session color coding** — Let users assign colors to sessions for visual identification.
- [ ] **Export session output** — Copy or download a session's full terminal output.
- [ ] **Session resource usage** — Show CPU/memory usage per session in the sidebar or pane header.
- [ ] **Responsive mobile layout** — Sidebar hides on small screens but there's no mobile-optimized layout.
- [ ] **Structured metrics** — Add a Prometheus `/metrics` endpoint for observability.
- [ ] **Improve Docker mode** — Current Docker mode can't manage host sessions. Explore bind-mounting tmux socket or a remote agent approach.
- [ ] **Auto-save settings** — Settings page requires manual save. Auto-save on change with debounce.
- [ ] **Path breadcrumbs** — Working directory paths are truncated. Make them clickable breadcrumbs.
- [ ] **Session notes/annotations** — Let users add notes to sessions like "waiting for PR review".
- [ ] **Multi-select sidebar actions** — Select multiple sessions to kill/restart at once.
- [ ] **Drag folder to create session** — Drag a directory from file manager to the UI to create a session in that directory.
