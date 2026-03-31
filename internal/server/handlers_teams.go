package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/IgorDeo/claude-websessions/internal/hooks"
	"github.com/IgorDeo/claude-websessions/web/templates"
)

// handleListTeams returns all discovered agent teams as JSON.
func (s *Server) handleListTeams(w http.ResponseWriter, r *http.Request) {
	if s.teamMgr == nil {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
		return
	}

	type memberJSON struct {
		Name      string `json:"name"`
		AgentID   string `json:"agentId"`
		Role      string `json:"role"`
		SessionID string `json:"sessionId,omitempty"`
		Connected bool   `json:"connected"`
	}
	type taskJSON struct {
		ID         string `json:"id"`
		Title      string `json:"title"`
		State      string `json:"state"`
		AssignedTo string `json:"assignedTo,omitempty"`
	}
	type teamJSON struct {
		Name    string       `json:"name"`
		State   string       `json:"state"`
		Members []memberJSON `json:"members"`
		Tasks   []taskJSON   `json:"tasks"`
	}

	teams := s.teamMgr.List()
	result := make([]teamJSON, 0, len(teams))
	for _, t := range teams {
		tj := teamJSON{
			Name:    t.Name,
			State:   string(t.State),
			Members: make([]memberJSON, 0, len(t.Members)),
			Tasks:   make([]taskJSON, 0, len(t.Tasks)),
		}
		for _, m := range t.Members {
			tj.Members = append(tj.Members, memberJSON{
				Name:      m.Name,
				AgentID:   m.AgentID,
				Role:      string(m.Role),
				SessionID: m.SessionID,
				Connected: m.Connected,
			})
		}
		for _, task := range t.Tasks {
			tj.Tasks = append(tj.Tasks, taskJSON{
				ID:         task.ID,
				Title:      task.Title,
				State:      string(task.State),
				AssignedTo: task.AssignedTo,
			})
		}
		result = append(result, tj)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

// handleTeamDashboard renders the team dashboard as an HTML partial for htmx.
func (s *Server) handleTeamDashboard(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if s.teamMgr == nil {
		http.Error(w, "teams feature is disabled", http.StatusNotFound)
		return
	}

	team, ok := s.teamMgr.Get(name)
	if !ok {
		http.Error(w, "team not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	renderTeamDashboardHTML(w, team)
}

// handleTeamTasks returns the task board as an HTML partial.
func (s *Server) handleTeamTasks(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if s.teamMgr == nil {
		http.Error(w, "teams feature is disabled", http.StatusNotFound)
		return
	}

	team, ok := s.teamMgr.Get(name)
	if !ok {
		http.Error(w, "team not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	renderTeamTasksHTML(w, team)
}

// handleTeamMessages returns the message log as an HTML partial.
func (s *Server) handleTeamMessages(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if s.teamMgr == nil {
		http.Error(w, "teams feature is disabled", http.StatusNotFound)
		return
	}

	team, ok := s.teamMgr.Get(name)
	if !ok {
		http.Error(w, "team not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	renderTeamMessagesHTML(w, team)
}

// handleNewTeamModal renders the "Create Team" modal.
func (s *Server) handleNewTeamModal(w http.ResponseWriter, r *http.Request) {
	var recentDirs []string
	if s.store != nil {
		recentDirs, _ = s.store.RecentDirs(10)
	}
	if err := templates.NewTeamModal(s.cfg.Sessions.DefaultDir, recentDirs).Render(r.Context(), w); err != nil {
		slog.Error("failed to render new team modal", "error", err)
	}
}

// handleCreateTeam spawns a new Claude Code session as a team lead.
func (s *Server) handleCreateTeam(w http.ResponseWriter, r *http.Request) {
	if s.teamMgr == nil {
		http.Error(w, "teams feature is disabled", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	workDir := strings.TrimSpace(r.FormValue("work_dir"))
	prompt := strings.TrimSpace(r.FormValue("prompt"))
	model := strings.TrimSpace(r.FormValue("model"))

	if name == "" || workDir == "" || prompt == "" {
		http.Error(w, "name, work_dir, and prompt are required", http.StatusBadRequest)
		return
	}

	// Build the claude CLI args with agent teams env
	id := "team-lead-" + name
	args := []string{"--name", name}
	if model != "" {
		args = append(args, "--model", model)
	}

	// Ensure the agent teams env var is set in ~/.claude/settings.json
	// so all Claude Code sessions pick it up automatically.
	_ = hooks.SetAgentTeams(true)

	// Create the session
	sess, err := s.mgr.Create(id, workDir, "claude", args)
	if err != nil {
		slog.Error("failed to create team lead session", "error", err)
		http.Error(w, fmt.Sprintf("failed to create session: %v", err), http.StatusInternalServerError)
		return
	}
	sess.Name = name + " (lead)"
	sess.TeamName = name
	sess.TeamRole = "lead"

	// Send the team creation prompt to the session
	teamPrompt := prompt + "\n"
	_ = s.mgr.WriteInput(id, []byte(teamPrompt))

	// Return htmx redirect to open the session
	w.Header().Set("HX-Trigger", `{"closeModal": true, "refreshSidebar": true}`)
	w.WriteHeader(http.StatusOK)
}

// handleSendTeamMessage sends a message to a teammate via the lead session's input.
func (s *Server) handleSendTeamMessage(w http.ResponseWriter, r *http.Request) {
	teamName := chi.URLParam(r, "name")
	if s.teamMgr == nil {
		http.Error(w, "teams feature is disabled", http.StatusBadRequest)
		return
	}

	team, ok := s.teamMgr.Get(teamName)
	if !ok {
		http.Error(w, "team not found", http.StatusNotFound)
		return
	}

	var payload struct {
		To      string `json:"to"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if payload.Content == "" {
		http.Error(w, "content is required", http.StatusBadRequest)
		return
	}

	// Find the lead session to send the message through
	var leadSessionID string
	for _, m := range team.Members {
		if m.Role == "lead" && m.SessionID != "" {
			leadSessionID = m.SessionID
			break
		}
	}
	if leadSessionID == "" {
		http.Error(w, "lead session not connected", http.StatusBadRequest)
		return
	}

	// Verify the lead session exists
	if _, ok := s.mgr.Get(leadSessionID); !ok {
		http.Error(w, "lead session not found", http.StatusBadRequest)
		return
	}

	// Send the message command to the lead session
	var msgCmd string
	if payload.To == "" || payload.To == "all" {
		msgCmd = fmt.Sprintf("Broadcast to all teammates: %s\n", payload.Content)
	} else {
		msgCmd = fmt.Sprintf("Send message to %s: %s\n", payload.To, payload.Content)
	}
	if err := s.mgr.WriteInput(leadSessionID, []byte(msgCmd)); err != nil {
		http.Error(w, fmt.Sprintf("failed to send message: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "sent"})
}

// handleTeamHookCallback receives webhook callbacks from Claude Code's team hooks
// (TeammateIdle, TaskCreated, TaskCompleted).
func (s *Server) handleTeamHookCallback(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Event     string `json:"event"`
		SessionID string `json:"session_id"`
		TeamName  string `json:"team_name"`
		TaskID    string `json:"task_id"`
		AgentID   string `json:"agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	slog.Info("team hook callback", "event", payload.Event, "team", payload.TeamName, "task", payload.TaskID)

	// Trigger a team rescan to pick up the latest state
	if s.teamMgr != nil {
		_ = s.teamMgr.Scan()
	}

	w.WriteHeader(http.StatusOK)
}

// handleInstallTeamHooks installs agent team hooks into Claude's settings.json.
// Returns an HTML partial for htmx to swap into #teams-hooks-status.
func (s *Server) handleInstallTeamHooks(w http.ResponseWriter, r *http.Request) {
	baseURL := fmt.Sprintf("http://localhost:%d", s.cfg.Server.Port)
	if err := hooks.InstallTeamHooks(baseURL); err != nil {
		slog.Error("failed to install team hooks", "error", err)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `<span class="settings-sublabel">Team Hooks</span><span class="hooks-badge hooks-inactive">Error: %s</span>`, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<span class="settings-sublabel">Team Hooks</span><span class="hooks-badge hooks-active">Installed</span>`)
}
