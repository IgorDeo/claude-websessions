package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/igor-deoalves/websessions/internal/discovery"
	"github.com/igor-deoalves/websessions/internal/notification"
	"github.com/igor-deoalves/websessions/internal/session"
	"github.com/igor-deoalves/websessions/web/templates"
)

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	sessions := s.mgr.List()
	views := make([]templates.SessionView, len(sessions))
	for i, sess := range sessions {
		views[i] = sessionToView(sess)
	}
	data := templates.PageData{Sessions: views, UnreadCount: s.sink.UnreadCount()}
	templates.Index(data).Render(r.Context(), w)
}

func (s *Server) handleSidebar(w http.ResponseWriter, r *http.Request) {
	sessions := s.mgr.List()
	views := make([]templates.SessionView, len(sessions))
	for i, sess := range sessions {
		views[i] = sessionToView(sess)
	}
	templates.Sidebar(views).Render(r.Context(), w)
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	name := r.FormValue("name")
	workDir := r.FormValue("work_dir")
	prompt := r.FormValue("prompt")
	if name == "" || workDir == "" {
		http.Error(w, "name and work_dir required", http.StatusBadRequest)
		return
	}
	args := []string{}
	if prompt != "" {
		args = append(args, "-p", prompt)
	}
	_, err := s.mgr.Create(name, workDir, "claude", args)
	if err != nil {
		slog.Error("failed to create session", "error", err)
		http.Error(w, "failed to create session: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.handleSidebar(w, r)
}

func (s *Server) handleOpenSession(w http.ResponseWriter, r *http.Request, sessionID string) {
	sess, ok := s.mgr.Get(sessionID)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	v := sessionToView(sess)
	templates.Terminal(v.ID, v.Name, v.WorkDir, v.State).Render(r.Context(), w)
}

func (s *Server) handleNewSessionModal(w http.ResponseWriter, r *http.Request) {
	var recentDirs []string
	if s.store != nil {
		recentDirs, _ = s.store.RecentDirs(10)
	}
	// Also add working dirs from currently discovered/running sessions
	for _, sess := range s.mgr.List() {
		found := false
		for _, d := range recentDirs {
			if d == sess.WorkDir {
				found = true
				break
			}
		}
		if !found && sess.WorkDir != "" {
			recentDirs = append(recentDirs, sess.WorkDir)
		}
	}
	templates.NewSessionModal(s.cfg.Sessions.DefaultDir, recentDirs).Render(r.Context(), w)
}

func (s *Server) handleRecentProjects(w http.ResponseWriter, r *http.Request) {
	var dirs []string
	if s.store != nil {
		dirs, _ = s.store.RecentDirs(10)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dirs)
}

func (s *Server) handleNotifications(w http.ResponseWriter, r *http.Request) {
	events := s.sink.Pending()
	views := make([]templates.NotificationView, len(events))
	for i, e := range events {
		views[i] = templates.NotificationView{SessionID: e.SessionID, EventType: string(e.Type)}
	}
	templates.Notifications(views).Render(r.Context(), w)
}

func sessionToView(s *session.Session) templates.SessionView {
	return templates.SessionView{
		ID: s.ID, Name: s.Name, WorkDir: s.WorkDir,
		State: string(s.GetState()), Owned: s.Owned,
	}
}

func (s *Server) handleTakeover(w http.ResponseWriter, r *http.Request, sessionID string) {
	sess, ok := s.mgr.Get(sessionID)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	if sess.GetState() != session.StateDiscovered {
		http.Error(w, "session is not in discovered state", http.StatusBadRequest)
		return
	}
	claudeID := sess.ClaudeID
	workDir := sess.WorkDir
	pid := sess.PID
	if err := discovery.KillProcess(pid, 5*time.Second); err != nil {
		slog.Error("takeover kill failed", "session", sessionID, "error", err)
		http.Error(w, "failed to kill process: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.mgr.Remove(sessionID)
	args := []string{"--resume", claudeID}
	newSess, err := s.mgr.Create(sessionID, workDir, "claude", args)
	if err != nil {
		slog.Error("takeover resume failed", "session", sessionID, "error", err)
		http.Error(w, "failed to resume session: "+err.Error(), http.StatusInternalServerError)
		return
	}
	v := sessionToView(newSess)
	templates.Terminal(v.ID, v.Name, v.WorkDir, v.State).Render(r.Context(), w)
}

// handleListSessions returns a JSON list of all sessions for the split picker.
func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	sessions := s.mgr.List()
	type sessionJSON struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		WorkDir string `json:"work_dir"`
		State   string `json:"state"`
	}
	var result []sessionJSON
	for _, sess := range sessions {
		result = append(result, sessionJSON{
			ID: sess.ID, Name: sess.Name, WorkDir: sess.WorkDir,
			State: string(sess.GetState()),
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// handleRenameSession renames a session.
func (s *Server) handleRenameSession(w http.ResponseWriter, r *http.Request, sessionID string) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	name := r.FormValue("name")
	if name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}
	sess, ok := s.mgr.Get(sessionID)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	sess.Name = name
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleRestartSession(w http.ResponseWriter, r *http.Request, sessionID string) {
	newSess, err := s.mgr.Restart(sessionID)
	if err != nil {
		slog.Error("restart failed", "session", sessionID, "error", err)
		http.Error(w, "failed to restart session: "+err.Error(), http.StatusInternalServerError)
		return
	}
	v := sessionToView(newSess)
	templates.Terminal(v.ID, v.Name, v.WorkDir, v.State).Render(r.Context(), w)
}

func (s *Server) setupNotificationBridge() {
	s.bus.Subscribe(func(e notification.SessionEvent) { s.sink.Send(e) })
}

// handleListDirs returns a JSON list of directories for the file finder autocomplete.
func (s *Server) handleListDirs(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		query = "~"
	}

	// Expand ~
	if strings.HasPrefix(query, "~") {
		home, _ := os.UserHomeDir()
		query = home + query[1:]
	}

	// If query doesn't end with /, list parent dir filtered by prefix
	dir := query
	prefix := ""
	if !strings.HasSuffix(query, "/") {
		dir = filepath.Dir(query)
		prefix = filepath.Base(query)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]string{})
		return
	}

	var dirs []string
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if prefix != "" && !strings.HasPrefix(strings.ToLower(entry.Name()), strings.ToLower(prefix)) {
			continue
		}
		dirs = append(dirs, filepath.Join(dir, entry.Name()))
	}
	sort.Strings(dirs)

	// Limit results
	if len(dirs) > 20 {
		dirs = dirs[:20]
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dirs)
}
