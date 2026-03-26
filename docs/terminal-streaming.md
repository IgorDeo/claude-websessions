# Terminal Streaming System

## Overview

The terminal streaming system delivers real-time PTY output from Claude Code sessions to one or more browser tabs. Each session runs inside a tmux session; a reader goroutine reads from a PTY attached to that tmux session, stores output in a fixed-size ring buffer, and broadcasts every chunk over WebSocket connections to xterm.js terminals in the browser.

Two WebSocket channels coexist on the same hub:

- **Per-session channels** (`/ws/{sessionID}`) carry binary PTY data and JSON control messages (resize).
- **Global notification channel** (`/ws/notifications`) carries JSON event payloads for the notification system.

---

## Architecture

```
                        SERVER SIDE                                    BROWSER SIDE
  .--------------------------------------------------------------------.  .-----------------------------.
  |                                                                    |  |                             |
  |  tmux session (ws-<id>)                                            |  |  Browser tab / split pane   |
  |       |                                                            |  |                             |
  |       | PTY (xterm-256color, 50x200)                               |  |  .----------------------.  |
  |       v                                                            |  |  | xterm.js Terminal     |  |
  |  tmux attach -t ws-<id> -r                                         |  |  | (scrollback: 50000)   |  |
  |       |                                                            |  |  '----------+-----------'  |
  |       | reader goroutine                                           |  |             |              |
  |       | reads 4096-byte chunks                                     |  |        FitAddon            |
  |       v                                                            |  |        ResizeObserver      |
  |  .-----------.    .--------------------------------------------.   |  |             |              |
  |  | RingBuf   |    |              wsHub                         |   |  '------|------+--------------'
  |  | (64 KB)   |    |                                            |   |         |      |
  |  | .Write()  |    |  clients[sessionID] = {conn1, conn2, ...}  |   |         |      |
  |  '-----------'    |                                            |   |         |      |
  |       |           |  broadcast(sessionID, data)                |   |         |      |
  |       |           |    for each conn: BinaryMessage            |   |         |      |
  |       |           '-------------------+------------------------'   |         |      |
  |       |                               |                            |         |      |
  |       | also: onOutput callback       | WebSocket                  |  WS     |      | WS JSON
  |       +------>  hub.broadcast() ------+----------------------------+-->>-----'      | {type:resize}
  |                                                                    |                |
  |       .------------------------------------------------------------.                |
  |       | New client connects:                                       |                |
  |       |   1. sess.Output().Bytes() -> replay ring buffer           |                |
  |       |   2. hub.add(sessionID, conn)                              |                |
  |       |   3. conn.ReadMessage loop (user input + resize)           |                |
  |       '------------------------------------------------------------'                |
  |                                                                    |                |
  |  sess.Resize(rows, cols) <---------------------------------------------------------'
  |       |                                                            |
  |       +-- pty.Setsize(readerPTY, ...)   (resize the reader PTY)    |
  |       +-- tmuxResizeWindow(name, ...)   (resize tmux window)       |
  |                                                                    |
  '--------------------------------------------------------------------'
```

---

## WebSocket Hub Design

The `wsHub` struct in `internal/server/ws.go` manages two independent connection pools behind a single `sync.RWMutex`:

```
wsHub
  |
  +-- clients       map[string]map[*websocket.Conn]bool
  |     Per-session connections. Key = session ID.
  |     Each session can have multiple viewers (tabs, splits).
  |
  +-- globalClients map[*websocket.Conn]bool
        Global notification channel. One per browser window.
```

### Per-Session Multiplexing

| Operation | Method | Lock | Description |
|-----------|--------|------|-------------|
| Register | `add(sessionID, conn)` | Write | Adds conn to session's client set; creates set if first viewer |
| Unregister | `remove(sessionID, conn)` | Write | Removes conn; deletes session key if last viewer |
| Broadcast | `broadcast(sessionID, data)` | Read | Sends `BinaryMessage` to all conns for that session |

Broadcast copies the connection set under a read lock, then writes outside the lock. Each write has a 10-second deadline. Failed writes close the connection and remove it from the hub.

### Global Notification Channel

| Operation | Method | Lock | Description |
|-----------|--------|------|-------------|
| Register | `addGlobal(conn)` | Write | Adds conn to global set |
| Unregister | `removeGlobal(conn)` | Write | Removes conn from global set |
| Broadcast | `broadcastNotification(event)` | Read | Sends `TextMessage` (JSON) to all global conns |

The notification WebSocket handler (`handleNotificationWS`) keeps the connection alive by reading messages in a loop. When the read returns an error (client disconnect), the connection is cleaned up via `defer`.

---

## Ring Buffer

### Implementation (`internal/session/ringbuf.go`)

`RingBuf` is a thread-safe circular byte buffer protecting all operations with a `sync.Mutex`.

```
Fields:
  buf     []byte   -- fixed-size backing array
  size    int      -- capacity (default: 64 KB from config)
  w       int      -- next write position (wraps around)
  written int64    -- total bytes written (monotonic counter)
```

### Write Behavior

```
Case 1: data fits without wrapping
  buf:  [----XXXXX---------]
             ^w
  write "AB":
  buf:  [----XXXXXAB-------]
                   ^w

Case 2: data wraps around end
  buf:  [-----XXXXXXXXXXXX]
                          ^w (at end)
  write "ABC":
  buf:  [BC---XXXXXXXXXXXXA]
          ^w

Case 3: data larger than buffer
  Only the last `size` bytes are kept.
  write position resets to 0.
```

### Read Behavior (`Bytes()`)

The `isFull()` check uses `written >= size` to determine whether the buffer has wrapped:

- **Not full:** Return `buf[0:w]` (data written so far, in order).
- **Full:** Return `buf[w:] + buf[:w]` (oldest data first, then newest).

Both paths return a copy, so callers can use the slice without holding the lock.

### Replay on New Client Connect

When a new WebSocket client connects to `/ws/{sessionID}`, the handler calls:

```go
if buf := sess.Output().Bytes(); len(buf) > 0 {
    conn.WriteMessage(websocket.BinaryMessage, buf)
}
```

This sends the entire ring buffer contents as a single binary frame, giving the new viewer immediate context of recent output before live data starts flowing.

---

## Terminal Connect Flow

Step-by-step sequence when a user clicks a session in the sidebar:

```
Browser                              Server
  |                                    |
  |  1. Click session in sidebar       |
  |                                    |
  |  2. GET /terminal/{sessionID}      |
  |  --------------------------------> |
  |  <-- HTML (terminal.templ)         |
  |      .terminal-pane                |
  |        .pane-header (name, status) |
  |        .terminal-container#term-X  |
  |                                    |
  |  3. connectSession(id, "term-X")   |
  |     a. new Terminal({...})         |
  |     b. new FitAddon() + fit()      |
  |                                    |
  |  4. openWS()                       |
  |     WebSocket /ws/{sessionID}      |
  |  ================================> |
  |                                    |  5. Upgrade HTTP -> WebSocket
  |                                    |     Check session exists
  |                                    |     Wait if StateStarting (sandbox)
  |                                    |
  |                                    |  6. hub.add(sessionID, conn)
  |                                    |
  |  <-- BinaryMessage: ring buffer    |  7. Replay: sess.Output().Bytes()
  |      (all buffered output)         |
  |                                    |
  |  8. term.write(filtered output)    |
  |     (alt screen sequences stripped)|
  |                                    |
  |  9. Send initial resize            |
  |  -- JSON: {type:resize, rows, cols}|
  |  --------------------------------> |
  |                                    |  10. sess.Resize(rows, cols)
  |                                    |      pty.Setsize + tmuxResizeWindow
  |                                    |
  |  ... live streaming begins ...     |
  |                                    |
  |  <-- BinaryMessage: PTY chunks     |  reader goroutine -> onOutput
  |  <-- BinaryMessage: PTY chunks     |    -> hub.broadcast()
  |                                    |
```

### Sandbox Provisioning Wait

If the session is in `StateStarting` (Docker sandbox being provisioned), the WebSocket handler:

1. Sends a cyan message: "Provisioning Docker sandbox..."
2. Polls every 1 second for up to 2 minutes
3. On `StateRunning`: sends green "Sandbox ready!" and continues
4. On `StateErrored`: sends red error message and closes
5. On timeout: sends red timeout message and closes

---

## Resize Handling

Terminal resize follows a chain from browser DOM changes all the way to the tmux window:

```
Browser                              Server
  |                                    |
  |  ResizeObserver fires              |
  |  (container size changed)          |
  |       |                            |
  |       v                            |
  |  fitAddon.fit()                    |
  |  (recalculates rows/cols           |
  |   based on container px size       |
  |   and font metrics)               |
  |       |                            |
  |       v                            |
  |  JSON: {"type":"resize",           |
  |         "rows":N, "cols":M}        |
  |  --------------------------------> |
  |                                    |  handleWS: TextMessage
  |                                    |    json.Unmarshal -> wsMessage
  |                                    |    msg.Type == "resize"
  |                                    |       |
  |                                    |       v
  |                                    |  sess.Resize(rows, cols)
  |                                    |       |
  |                                    |       +-- pty.Setsize(readerPTY)
  |                                    |       |   (kernel PTY dimensions)
  |                                    |       |
  |                                    |       +-- tmuxResizeWindow(name)
  |                                    |           (tmux server dimensions)
  |                                    |
```

The initial resize is also sent on `ws.onopen`, so the server knows the client's dimensions from the start.

Resize triggers include:
- Browser window resize
- Split pane drag (gutter resize)
- Tab switch (pane becomes visible)
- Font size change (Ctrl+= / Ctrl+-)

---

## Alt Screen Filtering

### The Problem

Claude Code (and tools it invokes like editors, pagers, `less`, etc.) uses ANSI alternate screen sequences to switch to a secondary buffer. In a standard terminal this is fine, but in a web-based xterm.js terminal it causes problems:

- The alternate screen has no scrollback, so output history is lost when switching back.
- Users cannot scroll up to review earlier output.
- Multiple enter/exit cycles create confusing visual artifacts.

### The Solution

The browser-side JavaScript strips alternate screen escape sequences before writing to xterm.js:

```javascript
var altScreenRe = /\x1b\[\?(1049|1047|47)[hl]/g;
var filtered = text.replace(altScreenRe, '');
```

### Sequences Filtered

| Sequence | Meaning | Direction |
|----------|---------|-----------|
| `ESC[?1049h` | Enable alternate screen buffer (xterm) | Enter alt screen |
| `ESC[?1049l` | Disable alternate screen buffer (xterm) | Leave alt screen |
| `ESC[?1047h` | Enable alternate screen buffer (DEC) | Enter alt screen |
| `ESC[?1047l` | Disable alternate screen buffer (DEC) | Leave alt screen |
| `ESC[?47h` | Enable alternate screen buffer (legacy) | Enter alt screen |
| `ESC[?47l` | Disable alternate screen buffer (legacy) | Leave alt screen |

By stripping these, all output stays in the normal scrollable buffer. The `h` suffix means "set" (enter) and `l` means "reset" (leave). All three mode numbers (47, 1047, 1049) are variants of the same feature across different terminal emulator implementations.

---

## Terminal Theming

### xterm.js Theme Objects

Two theme objects are defined in `app.js` and applied based on the current `data-theme` attribute:

| Property | Dark Theme | Light Theme |
|----------|-----------|-------------|
| `background` | `#13141c` | `#f5f6fa` |
| `foreground` | `#d0d4f0` | `#1a1c2b` |
| `cursor` | `#6c8cff` | `#4a6de5` |
| `selectionBackground` | `rgba(108, 140, 255, 0.2)` | `rgba(74, 109, 229, 0.15)` |

Theme switching updates all open terminal instances:

```javascript
Object.keys(terminals).forEach(function(id) {
    if (terminals[id]) terminals[id].options.theme = theme;
});
```

### CSS Variables

The terminal pane uses the global CSS custom properties for its chrome (header, borders, actions):

| CSS Variable | Purpose |
|-------------|---------|
| `--bg-tertiary` | Pane header background |
| `--border` | Pane header bottom border, gutter |
| `--text-primary` | Pane title color |
| `--text-muted` | Status text, directory path, action buttons |
| `--accent` | Focused pane header border, gutter hover |
| `--font-mono` | Pane title font, xterm.js font |

### Font Size

Terminal font size is persisted in `localStorage['ws-term-font-size']` (default: 14). Users can adjust with keyboard shortcuts. Changes call `fitAddon.fit()` on all terminals and send resize messages.

---

## Disconnect and Cleanup Flow

### Client Disconnect (browser tab closed, network drop)

```
Browser                              Server
  |                                    |
  |  tab closed / network lost         |
  |  ~~~~ WebSocket closes ~~~~        |
  |                                    |
  |                                    |  handleWS: ReadMessage returns error
  |                                    |    -> break out of read loop
  |                                    |    -> defer hub.remove(sessionID, conn)
  |                                    |    -> defer conn.Close()
  |                                    |
  |                                    |  (session + tmux + reader unaffected)
  |                                    |  (other viewers of same session unaffected)
```

### Client-Side Disconnect (`disconnectSession`)

Called when a tab is closed by the user in the UI:

```javascript
function disconnectSession(sessionID) {
    t.closed = true;       // prevent reconnect attempts
    t.ws.close();          // close WebSocket
    t.resizeObserver.disconnect();  // stop observing container
    t.term.dispose();      // destroy xterm.js instance
    delete terminals[sessionID];
}
```

### Client-Side Auto-Reconnect

If the WebSocket closes unexpectedly (and `session.closed` is not set), the client retries with exponential backoff:

```
Attempt 1:   1 second delay
Attempt 2:   2 second delay
Attempt 3:   4 second delay
Attempt 4:   8 second delay
Attempt 5:  15 second delay (capped)
Attempt 6:  give up, display "[Session unavailable]", auto-close tab after 2s
```

### Session Kill

```
User clicks kill button
        |
        v
Manager.Kill(id)
        |
        +-- stopReader(id)         close(stop channel) -> reader goroutine exits
        +-- tmuxKillSession(name)  kills the tmux session
        +-- (sandbox cleanup if applicable)
        +-- State -> StateErrored
        +-- onStateChange fires
        +-- Remove(id) from manager
        |
        v
hub still has connections, but no more data will be broadcast.
Next ReadMessage from browser returns error -> cleanup.
```

### Session Completion (Natural Exit)

```
tmux session exits naturally
        |
        v
Reader goroutine: ptmx.Read() returns error
        |
        +-- tmuxSessionExists() returns false
        +-- State -> StateCompleted
        +-- onStateChange fires
        +-- Remove(id) from manager
```

---

## Message Formats

### WebSocket: Server -> Browser

| Type | Format | Content |
|------|--------|---------|
| PTY output | `BinaryMessage` | Raw bytes from the PTY reader (up to 4096 bytes per chunk) |
| Ring buffer replay | `BinaryMessage` | Full ring buffer contents on connect (single frame) |
| Provisioning status | `BinaryMessage` | ANSI-colored status strings during sandbox startup |

### WebSocket: Browser -> Server

| Type | Format | Content |
|------|--------|---------|
| User keystrokes | `TextMessage` or `BinaryMessage` | Raw terminal input from `term.onData()` |
| Resize | `TextMessage` | `{"type":"resize","rows":N,"cols":M}` |

### Control Message Schema

```json
{
  "type": "resize",
  "rows": 24,
  "cols": 80
}
```

The `wsMessage` struct on the server side:

```go
type wsMessage struct {
    Type string `json:"type"`
    Rows int    `json:"rows,omitempty"`
    Cols int    `json:"cols,omitempty"`
}
```

### Terminal Response Filtering

The server filters out terminal capability responses from xterm.js before forwarding input to tmux. This prevents xterm.js auto-replies (DA1, DA2, DSR) from being interpreted as user keystrokes:

| Pattern | Description |
|---------|-------------|
| `ESC[?...c` | DA1 (Device Attributes primary) response |
| `ESC[>...c` | DA2 (Device Attributes secondary) response |
| `>0;...c` | Raw DA response (no ESC prefix) |
| `ESC[N;MR` | DSR (Cursor Position Report) response |

---

## Multiple Viewers

The same session can be viewed simultaneously in multiple browser contexts:

```
                          wsHub.clients["session-1"]
                                   |
                    +--------------+--------------+
                    |              |              |
                 conn-A         conn-B         conn-C
                    |              |              |
               Tab "main"    Split pane     Second browser
               (original)    (same tab)       window
```

### How It Works

1. Each call to `connectSession()` creates an independent xterm.js instance and WebSocket connection.
2. `hub.add(sessionID, conn)` registers each connection in the same session bucket.
3. `hub.broadcast(sessionID, data)` sends every PTY output chunk to all connections in that bucket.
4. Each viewer gets its own ring buffer replay on connect.
5. Resize messages from any viewer affect the underlying tmux session (last resize wins).

### Split Pane Architecture

The UI supports horizontal and vertical splits within a single tab. Each split pane is a separate `.terminal-pane` element containing its own xterm.js instance and WebSocket connection. The split layout is managed by Split.js with draggable gutters.

```
.terminal-area
  |
  +-- Split.js (horizontal)
       |
       +-- .split-pane
       |     +-- .terminal-pane [session-A]
       |           +-- .pane-header
       |           +-- .terminal-container#term-A
       |                 +-- xterm.js instance
       |
       +-- .gutter (draggable)
       |
       +-- .split-pane
             +-- .terminal-pane [session-B]
                   +-- .pane-header
                   +-- .terminal-container#term-B
                         +-- xterm.js instance
```

### Viewer Independence

- Closing one viewer (tab, split pane) does not affect other viewers of the same session.
- Each viewer can have different dimensions; the tmux session resizes to match the last resize message received.
- The session and its tmux process continue running even if all viewers disconnect.
