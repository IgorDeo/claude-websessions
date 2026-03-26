package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
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
