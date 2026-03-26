# Notifications System

## Overview

The notification system alerts users when their Claude Code sessions need attention. It supports four delivery channels that fire simultaneously: in-app panel, desktop notifications, server-side audio, and browser audio/toasts.

---

## Architecture

```
                         TRIGGER SOURCES
          .------------------------------------------.
          |                                          |
   Session State Change                     Claude Code Hook
   (managed sessions)                    (external/discovered)
          |                                          |
          |  OnStateChange callback                  |  POST /api/hook
          |  cmd/websessions/main.go                 |  handleHookCallback()
          |                                          |
          '-------------.   .------------------------'
                        |   |
                        v   v
               .--------------------.
               | notification.Bus   |
               | Publish(event)     |
               '--------+-----------'
                        |
          .-------------+--------------.
          |             |              |
          v             v              v
   .----------.  .------------.  .-----------.
   | InAppSink|  |DesktopSink |  | SoundSink |
   | (memory) |  |(notify-send|  | (paplay/  |
   |          |  | / osascript|  |  aplay)   |
   '----------'  '------------'  '-----------'
          |
          |  also:
          v
   .-----------------------------.
   | hub.broadcastNotification() |
   | WebSocket JSON push         |
   '-------------+---------------'
                 |
                 v
   .-----------------------------.
   | Browser (app.js)            |
   | - Badge count update        |
   | - Client-side audio (WebAudio)
   | - Desktop Notification API  |
   | - Toast fallback            |
   | - Sidebar refresh           |
   '-----------------------------'
```

---

## Event Types

| Event | Trigger | Icon | Color |
|-------|---------|------|-------|
| `waiting` | Permission prompt / tool approval needed | Warning | Yellow |
| `completed` | Session exited successfully (exit code 0) | Check | Green |
| `errored` | Session exited with error (non-zero exit) | X | Red |

---

## Two Entry Points

Notifications enter the system through two paths depending on whether the session is managed by websessions or external:

### Path 1: Managed Sessions (State Machine)

For sessions created through the websessions UI:

```
Session state transition
        |
        v
OnStateChange(session, from, to)      # cmd/websessions/main.go
        |
        |-- to == StateCompleted  -->  EventCompleted
        |-- to == StateErrored    -->  EventErrored
        |-- to == StateWaiting    -->  EventWaiting
        |-- (killed + errored)    -->  (skip, no notification)
        |
        v
bus.Publish(SessionEvent{...})
```

The state machine enforces valid transitions:

```
  Created --> Starting --> Running --> Completed
                             |
                             +--> Waiting --> Running (resumes)
                             |
                             +--> Errored
```

### Path 2: Claude Code Hooks (External Sessions)

For sessions running outside websessions (standalone Claude CLI):

```
Claude Code hits permission prompt
        |
        v
Hook fires (from ~/.claude/settings.json)
        |
        v
python3 extracts {session_id, cwd} from stdin
        |
        v
curl POST http://localhost:8080/api/hook
     {"event":"waiting", "session_id":"...", "project":"..."}
        |
        v
handleHookCallback()
        |
        |-- Match to active session by workDir
        |-- OR auto-discover via process scan
        |-- OR create external-{basename} placeholder
        |
        v
bus.Publish(SessionEvent{...})
```

The hook command installed in `~/.claude/settings.json`:

```json
{
  "hooks": {
    "Notification": [{
      "matcher": "permission_prompt",
      "hooks": [{
        "type": "command",
        "command": "python3 -c \"...\" | curl -s -X POST .../api/hook -d @-"
      }]
    }]
  }
}
```

---

## Delivery Channels

Once `bus.Publish(event)` fires, the `setupNotificationBridge()` subscriber distributes to all channels simultaneously:

```
bus.Publish(event)
       |
       |  setupNotificationBridge subscriber
       |
       +---> InAppSink.Send()        Store in memory ring buffer
       |                              (used by GET /notifications panel)
       |
       +---> DesktopSink.Send()       Linux:  notify-send --urgency=... --icon=...
       |                              macOS:  osascript "display notification"
       |
       +---> SoundSink.Send()         Linux:  paplay ~/.websessions/sounds/{event}.wav
       |                              macOS:  afplay
       |
       +---> store.SaveNotification() Persist to SQLite
       |
       +---> hub.broadcastNotification(JSON)
                    |
                    v
              All connected WebSocket clients
              /ws/notifications
```

### Channel: WebSocket + Browser

The browser maintains a persistent WebSocket to `/ws/notifications`:

```
Server                                          Browser
  |                                               |
  |  --- ws: {"type":"notification",              |
  |           "event":"waiting",                  |
  |           "sessionID":"sess-1",               |
  |           "message":"my-project: prompt",     |
  |           "timestamp":"14:32:05"} ----------> |
  |                                               |
  |                                    +----------+----------+
  |                                    |          |          |
  |                                    v          v          v
  |                               Update      Play       Desktop
  |                               bell       audio     Notification
  |                               badge      tones      or Toast
  |                                    |
  |                                    v
  |                              Refresh sidebar
  |                              (session state dot)
```

**Auto-reconnect:** If the WebSocket drops, the client reconnects after 3 seconds.

### Channel: Client-Side Audio

The browser plays tones via the Web Audio API (separate from server-side audio):

| Event | Tone | Frequencies | Waveform |
|-------|------|-------------|----------|
| `completed` | Ascending two-note chime | 523 Hz + 784 Hz (C5 to G5) | Sine |
| `waiting` | Three pings | 660 Hz + 660 Hz + 880 Hz | Sine |
| `errored` | Descending two-note | 440 Hz + 330 Hz (A4 to E4) | Triangle |

Controlled by `localStorage['ws-notif-sounds']`. Toggle in Settings.

### Channel: Server-Side Audio

Pre-generated WAV files in `~/.websessions/sounds/`:

| File | Description | Frequencies |
|------|-------------|-------------|
| `completed.wav` | Ascending chime | 523 Hz, 784 Hz |
| `errored.wav` | Descending tone | 392 Hz, 262 Hz |
| `waiting.wav` | Gentle ping | 659 Hz |

Playback via `paplay` (PulseAudio/PipeWire) with fallback to `aplay`. Configurable audio device in Settings.

---

## Reminder System

Sessions stuck in `waiting` state get periodic re-notifications:

```
       Ticker (every 30 seconds)
              |
              v
   For each session in StateWaiting:
              |
              +-- Is snoozed? ----yes----> skip
              |
              no
              |
              +-- Interval elapsed? --no--> skip
              |     (default: 5 min)
              yes
              |
              v
       Publish reminder event
       "session-name is still waiting for your input"
              |
              v
       Update snooze-until = now + interval
```

### Snooze Flow

```
User clicks snooze (15 min)
        |
        v
POST /notifications/snooze
{"session_id": "...", "minutes": 15}
        |
        v
snoozedSessions[id] = now + 15 min
        |
        v
Notification dismissed from UI
        |
        ..... 15 minutes later .....
        |
        v
Reminder ticker fires again
(snooze expired, re-notifies)
```

Snooze is automatically cleared when a session leaves the `waiting` state.

---

## UI Components

### Notification Bell (top bar)

```
 .-------------------------.
 | websessions             |      .---.
 |                         |      | B | <-- Bell icon
 |                         |      | 3 | <-- Red badge (unread count)
 '-------------------------'      '---'
```

Clicking the bell opens the notification dropdown panel.

### Notification Panel

```
 .--------------------------------------.
 | Notifications              (3)  Clear|
 |--------------------------------------|
 | ! my-project              2 min ago  |
 |   permission prompt                  |
 |   ~/code/my-project                  |
 |                     [Snooze] [Dismiss]|
 |--------------------------------------|
 | x api-service              5 min ago |
 |   session errored                    |
 |   ~/code/api-service                 |
 |                             [Dismiss]|
 |--------------------------------------|
 | . data-pipeline           12 min ago |
 |   session completed                  |
 |   ~/code/data-pipeline               |
 |                             [Dismiss]|
 '--------------------------------------'
```

- Click an item to focus that session's tab
- Snooze button only appears for `waiting` events (snoozes 15 min)
- Clear All marks everything as read

### Toast Notifications (fallback)

When desktop notifications aren't available (e.g., GUI/webview mode):

```
                              .--------------------------------.
                              | websessions: waiting           |
                              | Session my-project:            |
                              |   permission prompt            |
                              '--------------------------------'
                                       (bottom-right, auto-fades 5s)
```

### Favicon Badge

The page title updates to show unread count:

```
Tab: (3) websessions        <-- with unread
Tab: websessions             <-- no unread
```

---

## Configuration

Settings page controls (persisted in `~/.websessions/config.yaml`):

```yaml
notifications:
  desktop: true             # Native OS notifications (notify-send)
  sound: true               # Server-side audio playback
  audio_device: ""          # Specific PulseAudio sink (or system default)
  events:                   # Which events trigger notifications
    - completed
    - errored
    - waiting
  reminder_minutes: 5       # Re-notify interval for waiting sessions (0 = disabled)
```

Browser-side sound toggle is stored separately in `localStorage['ws-notif-sounds']`.

---

## Database Schema

Notifications are persisted in SQLite for the panel history:

```
notifications table
+----+------------+-----------+---------------------+------+
| id | session_id | event_type| timestamp           | read |
+----+------------+-----------+---------------------+------+
| 1  | sess-abc   | waiting   | 2026-03-26 14:32:05 | 0    |
| 2  | sess-def   | completed | 2026-03-26 14:35:12 | 1    |
| 3  | sess-abc   | waiting   | 2026-03-26 14:37:05 | 0    |
+----+------------+-----------+---------------------+------+
```

The `GET /notifications` endpoint fetches up to 30 unread records and enriches them with live session data (name, workDir) before rendering.

---

## Complete Example Flow

```
1. User starts Claude in ~/myproject
   Claude runs, hits a tool approval prompt

2. Claude Code hook fires:
   stdin: {"session_id":"abc-123","cwd":"/home/user/myproject"}
   hook:  python3 ... | curl POST /api/hook

3. handleHookCallback():
   - Finds session "myproject-1" by matching workDir
   - Creates SessionEvent{Type: EventWaiting}
   - bus.Publish(event)

4. setupNotificationBridge subscriber fires:
   - InAppSink:    stores event in memory
   - DesktopSink:  notify-send "websessions" "myproject-1: permission prompt"
   - SoundSink:    paplay ~/.websessions/sounds/waiting.wav
   - WebSocket:    broadcasts JSON to all browser clients
   - SQLite:       saves notification record

5. Browser receives WebSocket message:
   - Bell badge: 0 -> 1
   - Client audio: three pings (660, 660, 880 Hz)
   - Desktop notification popup (or toast if no permission)
   - Sidebar: session dot turns yellow (waiting)

6. User ignores for 5 minutes...
   - Reminder ticker: re-publishes waiting event
   - Same flow as step 4-5 (user gets pinged again)

7. User clicks snooze (15 min):
   - POST /notifications/snooze
   - No more reminders for 15 minutes

8. User approves the tool in Claude:
   - Session transitions to Running
   - Snooze cleared automatically
   - Session eventually completes -> EventCompleted fires
   - Bell badge updates, completed chime plays
```
