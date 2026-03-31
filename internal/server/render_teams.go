package server

import (
	"fmt"
	"html"
	"io"
	"strings"

	"github.com/IgorDeo/claude-websessions/internal/teams"
)

// renderTeamDashboardHTML writes the team dashboard as an HTML partial.
func renderTeamDashboardHTML(w io.Writer, team *teams.Team) {
	memberCount := len(team.Members)
	leadName := ""
	for _, m := range team.Members {
		if m.Role == teams.RoleLead {
			leadName = m.Name
			break
		}
	}

	stateClass := "state-active"
	stateLabel := "Active"
	if team.State == teams.TeamInactive {
		stateClass = "state-inactive"
		stateLabel = "Inactive"
	}

	fmt.Fprintf(w, `<div class="team-dashboard" id="team-dashboard-%s">`, html.EscapeString(team.Name))
	fmt.Fprintf(w, `<div class="team-header">`)
	fmt.Fprintf(w, `<h3>Team: %s</h3>`, html.EscapeString(team.Name))
	fmt.Fprintf(w, `<span class="team-lead">Lead: %s</span>`, html.EscapeString(leadName))
	fmt.Fprintf(w, `<span class="team-member-count">%d members</span>`, memberCount)
	fmt.Fprintf(w, `<span class="team-state %s">%s</span>`, stateClass, stateLabel)
	fmt.Fprintf(w, `</div>`)

	// Layout: members panel + task board
	fmt.Fprintf(w, `<div class="team-body">`)

	// Members panel
	fmt.Fprintf(w, `<div class="team-members-panel">`)
	fmt.Fprintf(w, `<h4>Members</h4>`)
	for _, m := range team.Members {
		icon := "&#x1F517;" // link emoji for teammate
		if m.Role == teams.RoleLead {
			icon = "&#x1F451;" // crown for lead
		}
		connClass := "disconnected"
		connDot := "&#x25CB;" // hollow circle
		if m.Connected {
			connClass = "connected"
			connDot = "&#x25CF;" // filled circle
		}
		fmt.Fprintf(w, `<div class="team-member %s"`, connClass)
		if m.SessionID != "" {
			fmt.Fprintf(w, ` data-session-id="%s" onclick="websessions.focusSession('%s')" style="cursor:pointer"`, html.EscapeString(m.SessionID), html.EscapeString(m.SessionID))
		}
		fmt.Fprintf(w, `>`)
		fmt.Fprintf(w, `<span class="member-icon">%s</span>`, icon)
		fmt.Fprintf(w, `<span class="member-name">%s</span>`, html.EscapeString(m.Name))
		fmt.Fprintf(w, `<span class="member-status">%s</span>`, connDot)
		fmt.Fprintf(w, `</div>`)
	}
	fmt.Fprintf(w, `</div>`)

	// Task board
	renderTeamTasksHTML(w, team)

	fmt.Fprintf(w, `</div>`) // .team-body

	// Messages
	renderTeamMessagesHTML(w, team)

	fmt.Fprintf(w, `</div>`) // .team-dashboard
}

// renderTeamTasksHTML writes the Kanban-style task board as HTML.
func renderTeamTasksHTML(w io.Writer, team *teams.Team) {
	var pending, inProgress, completed []teams.Task
	for _, t := range team.Tasks {
		switch t.State {
		case teams.TaskPending:
			pending = append(pending, t)
		case teams.TaskInProgress:
			inProgress = append(inProgress, t)
		case teams.TaskCompleted:
			completed = append(completed, t)
		}
	}

	fmt.Fprintf(w, `<div class="task-board" hx-get="/teams/%s/tasks" hx-trigger="every 10s, teamUpdate" hx-swap="outerHTML">`, html.EscapeString(team.Name))

	renderTaskColumn(w, "Pending", "pending", pending)
	renderTaskColumn(w, "In Progress", "in-progress", inProgress)
	renderTaskColumn(w, "Completed", "completed", completed)

	fmt.Fprintf(w, `</div>`)
}

func renderTaskColumn(w io.Writer, title, className string, tasks []teams.Task) {
	fmt.Fprintf(w, `<div class="task-column task-column-%s">`, className)
	fmt.Fprintf(w, `<h4>%s <span class="task-count">%d</span></h4>`, title, len(tasks))
	for _, t := range tasks {
		fmt.Fprintf(w, `<div class="task-card task-%s">`, className)
		fmt.Fprintf(w, `<div class="task-title">%s</div>`, html.EscapeString(t.Title))
		if t.AssignedTo != "" {
			fmt.Fprintf(w, `<div class="task-assignee">&#x2192; %s</div>`, html.EscapeString(t.AssignedTo))
		}
		fmt.Fprintf(w, `</div>`)
	}
	fmt.Fprintf(w, `</div>`)
}

// renderTeamMessagesHTML writes the message log as HTML.
func renderTeamMessagesHTML(w io.Writer, team *teams.Team) {
	fmt.Fprintf(w, `<div class="message-log" hx-get="/teams/%s/messages" hx-trigger="every 10s, teamUpdate" hx-swap="outerHTML">`, html.EscapeString(team.Name))
	fmt.Fprintf(w, `<h4>Messages</h4>`)

	if len(team.Messages) == 0 {
		fmt.Fprintf(w, `<div class="message-empty">No messages yet</div>`)
	} else {
		// Show last 50 messages
		start := 0
		if len(team.Messages) > 50 {
			start = len(team.Messages) - 50
		}
		for _, msg := range team.Messages[start:] {
			ts := msg.Timestamp.Format("15:04")
			to := msg.To
			if to == "" {
				to = "all"
			}
			fmt.Fprintf(w, `<div class="message-entry">`)
			fmt.Fprintf(w, `<span class="message-time">[%s]</span> `, ts)
			fmt.Fprintf(w, `<span class="message-from">%s</span>`, html.EscapeString(msg.From))
			fmt.Fprintf(w, ` &#x2192; `)
			fmt.Fprintf(w, `<span class="message-to">%s</span>: `, html.EscapeString(to))
			fmt.Fprintf(w, `<span class="message-content">%s</span>`, html.EscapeString(truncate(msg.Content, 200)))
			fmt.Fprintf(w, `</div>`)
		}
	}
	fmt.Fprintf(w, `</div>`)
}

func truncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}
