package server

import (
	"bufio"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
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
	args := []string{"--name", name}
	resumeID := r.FormValue("resume_id")
	if resumeID != "" {
		args = append(args, "--resume", resumeID)
	}
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

func (s *Server) handleKillSession(w http.ResponseWriter, r *http.Request, sessionID string) {
	sess, ok := s.mgr.Get(sessionID)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	state := sess.GetState()
	if state == session.StateRunning || state == session.StateWaiting || state == session.StateCreated {
		if err := s.mgr.Kill(sessionID); err != nil {
			slog.Error("kill failed", "session", sessionID, "error", err)
			http.Error(w, "failed to kill session: "+err.Error(), http.StatusInternalServerError)
			return
		}
		// Wait for process to finish
		go s.mgr.Wait(sessionID)
	}
	// For offline/discovered, just remove from the list
	if state == session.StateOffline || state == session.StateDiscovered {
		s.mgr.Remove(sessionID)
	}
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

// handleGitDiff returns git diff and status for a session's working directory.
func (s *Server) handleGitDiff(w http.ResponseWriter, r *http.Request, sessionID string) {
	sess, ok := s.mgr.Get(sessionID)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	workDir := sess.WorkDir

	type gitDiffResponse struct {
		Diff    string   `json:"diff"`
		Status  string   `json:"status"`
		Files   []string `json:"files"`
		WorkDir string   `json:"work_dir"`
	}

	// git status
	statusCmd := exec.Command("git", "status", "--short")
	statusCmd.Dir = workDir
	statusOut, _ := statusCmd.Output()

	// git diff (staged + unstaged)
	diffCmd := exec.Command("git", "diff", "HEAD", "--no-color")
	diffCmd.Dir = workDir
	diffOut, err := diffCmd.Output()
	if err != nil {
		// Maybe no HEAD yet, try just git diff
		diffCmd2 := exec.Command("git", "diff", "--no-color")
		diffCmd2.Dir = workDir
		diffOut, _ = diffCmd2.Output()
	}

	// Also get untracked/new files diff
	statusLines := strings.Split(strings.TrimSpace(string(statusOut)), "\n")
	var files []string
	for _, line := range statusLines {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}

	resp := gitDiffResponse{
		Diff:    string(diffOut),
		Status:  string(statusOut),
		Files:   files,
		WorkDir: workDir,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleClaudeSessions lists claude sessions available for a given project directory.
func (s *Server) handleClaudeSessions(w http.ResponseWriter, r *http.Request) {
	workDir := r.URL.Query().Get("dir")
	if workDir == "" {
		http.Error(w, "dir required", http.StatusBadRequest)
		return
	}
	// Expand ~
	if strings.HasPrefix(workDir, "~") {
		home, _ := os.UserHomeDir()
		workDir = home + workDir[1:]
	}
	// Clean trailing slash
	workDir = strings.TrimSuffix(workDir, "/")

	// Convert path to claude's project folder name: /home/user.name/foo -> -home-user-name-foo
	// Claude replaces both / and . with -
	projectName := strings.ReplaceAll(workDir, "/", "-")
	projectName = strings.ReplaceAll(projectName, ".", "-")

	home, _ := os.UserHomeDir()
	sessionsDir := filepath.Join(home, ".claude", "projects", projectName)

	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]interface{}{})
		return
	}

	type claudeSession struct {
		ID      string `json:"id"`
		Date    string `json:"date"`
		Summary string `json:"summary"`
		SizeKB  int64  `json:"size_kb"`
	}

	var sessions []claudeSession
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		sessionID := strings.TrimSuffix(entry.Name(), ".jsonl")
		info, _ := entry.Info()
		if info == nil {
			continue
		}

		summary := ""
		fpath := filepath.Join(sessionsDir, entry.Name())
		f, err := os.Open(fpath)
		if err == nil {
			scanner := bufio.NewScanner(f)
			scanner.Buffer(make([]byte, 1024*64), 1024*64)
			for scanner.Scan() {
				line := scanner.Text()
				if line == "" {
					continue
				}
				var obj map[string]interface{}
				if json.Unmarshal([]byte(line), &obj) != nil {
					continue
				}
				if obj["type"] == "user" {
					if msg, ok := obj["message"].(map[string]interface{}); ok {
						if content, ok := msg["content"].(string); ok {
							summary = content
							if len(summary) > 100 {
								summary = summary[:100]
							}
							break
						}
						if contentList, ok := msg["content"].([]interface{}); ok {
							for _, c := range contentList {
								if cm, ok := c.(map[string]interface{}); ok {
									if text, ok := cm["text"].(string); ok {
										summary = text
										if len(summary) > 100 {
											summary = summary[:100]
										}
										break
									}
								}
							}
							if summary != "" {
								break
							}
						}
					}
				}
			}
			f.Close()
		}

		sessions = append(sessions, claudeSession{
			ID:      sessionID,
			Date:    info.ModTime().Format("2006-01-02 15:04"),
			Summary: summary,
			SizeKB:  info.Size() / 1024,
		})
	}

	// Sort by date descending (most recent first)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Date > sessions[j].Date
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessions)
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
