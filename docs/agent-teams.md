# Agent Teams Integration

Websessions integrates with Claude Code's experimental **Agent Teams** feature to provide a visual command center for coordinating multi-agent workflows. This feature is opt-in and disabled by default.

## Overview

Agent teams let you coordinate multiple Claude Code instances working together. One session acts as the **team lead**, coordinating work and assigning tasks. **Teammates** work independently, each in their own context window, communicating via a shared task list and mailbox.

Websessions discovers active teams by scanning `~/.claude/teams/` for config files and `~/.claude/tasks/` for shared task lists. Team members are correlated with websessions sessions by matching their `agentID` against session `ClaudeID`.

## Enabling

Add to `~/.websessions/config.yaml`:

```yaml
teams:
  enabled: true
  scan_interval: "5s"  # how often to check for team changes
```

Requirements:
- Claude Code v2.1.32 or later
- The `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS` environment variable must be set to `1` in Claude Code's settings

## Architecture

```
~/.claude/teams/{name}/config.json    <- TeamScanner reads
~/.claude/tasks/{name}/*.json         <- TaskScanner reads
~/.claude/teams/{name}/mailbox/       <- MailboxScanner reads
                |
                v
        internal/teams/
        ├── team.go        Core types (Team, Member, Task, Message)
        ├── scanner.go     Filesystem scanner
        ├── manager.go     Team lifecycle, session correlation
        └── mailbox.go     Message reader
                |
                v
        notification.Bus   session.Manager
        (team events)      (map members -> sessions)
                |
                v
        Server handlers + WebSocket hub
                |
                v
        UI (dashboard, sidebar groups, task board)
```

### Key Components

| Component | File(s) | Purpose |
|---|---|---|
| Team types | `internal/teams/team.go` | Team, Member, Task, Message, state enums |
| FS scanner | `internal/teams/scanner.go` | Reads team configs, tasks, and mailbox from disk |
| Team manager | `internal/teams/manager.go` | Lifecycle management, session correlation, event publishing |
| Team handlers | `internal/server/handlers_teams.go` | REST API endpoints for teams |
| Team rendering | `internal/server/render_teams.go` | HTML rendering for dashboard, task board, messages |
| Team WebSocket | `internal/server/ws.go` | Real-time push for team events |
| Team hooks | `internal/hooks/hooks.go` | Claude Code hook installation for TeammateIdle, TaskCreated, TaskCompleted |

### API Endpoints

| Method | Path | Description |
|---|---|---|
| GET | `/api/teams` | List all discovered teams as JSON |
| GET | `/teams/{name}` | Team dashboard HTML partial |
| GET | `/teams/{name}/tasks` | Task board HTML partial |
| GET | `/teams/{name}/messages` | Message log HTML partial |
| GET | `/teams/new` | Create team modal |
| POST | `/teams` | Create a new team (spawns lead session) |
| POST | `/teams/{name}/messages` | Send message via lead session |
| POST | `/teams/install-hooks` | Install team hooks in Claude settings |
| POST | `/api/hook/team` | Receive team hook callbacks |
| GET | `/ws/teams/{name}` | WebSocket for real-time team updates |

### Notification Events

| Event | Fired when |
|---|---|
| `team_discovered` | A new team config is found on disk |
| `team_member_join` | A team member is correlated with a session |
| `task_created` | A new task appears in the task list |
| `task_updated` | A task changes state |
| `task_completed` | A task transitions to completed |
| `team_message` | A new inter-agent message is detected |

## UI Components

### Sidebar Team Groups

When team members are correlated with sessions, they appear grouped in the sidebar under a collapsible "Team: {name}" header. Each member shows:
- Crown icon for the team lead
- Link icon for teammates
- Connection status indicator

Clicking a member focuses their terminal tab. The team header includes a clipboard icon to open the team dashboard.

### Team Dashboard

The dashboard is opened as a tab panel (same pattern as iframe panes) and contains:

1. **Header** — Team name, lead name, member count, active/inactive state
2. **Members panel** — Roster with role icons and connection status
3. **Task board** — Three-column Kanban (Pending / In Progress / Completed) with task cards
4. **Message log** — Chronological inter-agent messages with timestamps

The task board and message log auto-refresh via htmx polling (every 10s) and are also triggered immediately by WebSocket events.

### Create Team Modal

Available when teams are enabled. Spawns a new Claude Code session as team lead with:
- `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1` environment variable
- The user's prompt describing the team structure and task

## Session Correlation

Team members are matched to websessions sessions by comparing the `agentID` field in `~/.claude/teams/{name}/config.json` against the `ClaudeID` resolved for each session. When a match is found:

1. The session's `TeamName` and `TeamRole` fields are set
2. The member is marked as `Connected`
3. The session appears in the team's sidebar group
4. Clicking the member in the dashboard focuses their terminal

## Graceful Degradation

When `teams.enabled` is `false` (default):
- No filesystem scanning occurs
- No team routes are registered
- No team UI elements appear
- Zero runtime overhead

When teams are enabled but `~/.claude/teams/` doesn't exist:
- Scanner returns empty results without errors
- No teams appear in the UI
- System logs a debug message

When Claude Code is not installed or below v2.1.32:
- A warning is printed at startup
- Scanning still works (existing team files are readable)
- Team creation may fail (no `claude` command available)
