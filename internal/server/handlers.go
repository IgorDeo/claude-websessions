package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/igor-deoalves/websessions/internal/discovery"
	"github.com/igor-deoalves/websessions/internal/hooks"
	"github.com/igor-deoalves/websessions/internal/notification"
	"github.com/igor-deoalves/websessions/internal/session"
	"github.com/igor-deoalves/websessions/internal/store"
	"github.com/igor-deoalves/websessions/web/templates"
)

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	sessions := s.mgr.List()
	views := make([]templates.SessionView, len(sessions))
	for i, sess := range sessions {
		views[i] = sessionToView(sess)
	}
	data := templates.PageData{Sessions: views, UnreadCount: s.sink.UnreadCount()}
	// History is loaded by sidebar via htmx, not needed for initial render
	templates.Index(data).Render(r.Context(), w)
}

func (s *Server) handleSidebar(w http.ResponseWriter, r *http.Request) {
	sessions := s.mgr.List()
	views := make([]templates.SessionView, len(sessions))
	for i, sess := range sessions {
		views[i] = sessionToView(sess)
	}

	// Load history from SQLite (completed/errored sessions not in active list)
	var history []templates.SessionView
	if s.store != nil {
		records, _ := s.store.ListSessions(20)
		activeIDs := make(map[string]bool)
		for _, v := range views {
			activeIDs[v.ID] = true
		}
		for _, rec := range records {
			if activeIDs[rec.ID] {
				continue
			}
			// Only show finished sessions in history
			if rec.Status != "completed" && rec.Status != "errored" && rec.Status != "killed" {
				continue
			}
			name := rec.Name
			if name == "" {
				name = rec.ID
			}
			history = append(history, templates.SessionView{
				ID:      rec.ID,
				Name:    name,
				WorkDir: rec.WorkDir,
				State:   rec.Status,
			})
		}
	}

	templates.Sidebar(views, history).Render(r.Context(), w)
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
	sess, err := s.mgr.Create(name, workDir, "claude", args)
	if err != nil {
		slog.Error("failed to create session", "error", err)
		http.Error(w, "failed to create session: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// Persist to SQLite immediately so it survives restarts
	if s.store != nil {
		s.store.SaveSession(store.SessionRecord{
			ID: sess.ID, Name: sess.Name, ClaudeID: sess.ClaudeID, WorkDir: sess.WorkDir,
			StartTime: sess.StartTime, Status: "running", PID: sess.PID,
		})
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
	var views []templates.NotificationView
	if s.store != nil {
		records, _ := s.store.ListNotifications(20, false)
		for _, rec := range records {
			views = append(views, templates.NotificationView{
				ID:        rec.ID,
				SessionID: rec.SessionID,
				EventType: rec.EventType,
			})
		}
	} else {
		events := s.sink.Pending()
		for _, e := range events {
			views = append(views, templates.NotificationView{
				SessionID: e.SessionID,
				EventType: string(e.Type),
			})
		}
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
	name := sess.Name

	// Try to resolve session ID if not already known
	if claudeID == "" {
		claudeID = discovery.ResolveClaudeSessionID(workDir)
	}

	if err := discovery.KillProcess(pid, 5*time.Second); err != nil {
		slog.Error("takeover kill failed", "session", sessionID, "error", err)
		http.Error(w, "failed to kill process: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.mgr.Remove(sessionID)

	args := []string{"--name", name}
	if claudeID != "" {
		args = append(args, "--resume", claudeID)
		slog.Info("takeover resuming session", "session", sessionID, "claude_id", claudeID)
	}
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
	// Persist rename to SQLite
	if s.store != nil {
		s.store.SaveSession(store.SessionRecord{
			ID: sess.ID, Name: name, ClaudeID: sess.ClaudeID, WorkDir: sess.WorkDir,
			StartTime: sess.StartTime, EndTime: sess.EndTime,
			ExitCode: sess.ExitCode, Status: string(sess.GetState()), PID: sess.PID,
		})
	}
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
	// Save final state to SQLite before removing
	if s.store != nil {
		s.store.SaveSession(store.SessionRecord{
			ID: sess.ID, Name: sess.Name, ClaudeID: sess.ClaudeID, WorkDir: sess.WorkDir,
			StartTime: sess.StartTime, EndTime: time.Now(),
			ExitCode: -1, Status: "killed", PID: sess.PID,
		})
	}
	// For offline/discovered, remove from active list
	if state == session.StateOffline || state == session.StateDiscovered {
		s.mgr.Remove(sessionID)
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleRestartSession(w http.ResponseWriter, r *http.Request, sessionID string) {
	// Try in-memory first (for offline sessions)
	newSess, err := s.mgr.Restart(sessionID)
	if err != nil {
		// Not in memory — try loading from SQLite history
		if s.store != nil {
			records, _ := s.store.ListSessions(100)
			for _, rec := range records {
				if rec.ID == sessionID {
					// Create directly from history record
					name := rec.Name
					if name == "" {
						name = sessionID
					}
					args := []string{"--name", name}
					if rec.ClaudeID != "" {
						args = append(args, "--resume", rec.ClaudeID)
					}
					newSess, err = s.mgr.Create(sessionID, rec.WorkDir, "claude", args)
					if err == nil {
						newSess.Name = name
					}
					break
				}
			}
		}
		if err != nil || newSess == nil {
			slog.Error("restart failed", "session", sessionID, "error", err)
			http.Error(w, "failed to restart session: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	// Save to DB
	if s.store != nil {
		s.store.SaveSession(store.SessionRecord{
			ID: newSess.ID, Name: newSess.Name, ClaudeID: newSess.ClaudeID, WorkDir: newSess.WorkDir,
			StartTime: newSess.StartTime, Status: "running", PID: newSess.PID,
		})
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

// handleHookCallback receives notifications from Claude Code hooks.
func (s *Server) handleHookCallback(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Event     string `json:"event"`
		SessionID string `json:"session_id"`
		Project   string `json:"project"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	var eventType notification.EventType
	switch payload.Event {
	case "waiting":
		eventType = notification.EventWaiting
	case "completed":
		eventType = notification.EventCompleted
	case "tool_use":
		// tool_use is informational, not a notification event
		w.WriteHeader(http.StatusOK)
		return
	default:
		eventType = notification.EventType(payload.Event)
	}

	event := notification.SessionEvent{
		SessionID: payload.SessionID,
		Type:      eventType,
		Timestamp: time.Now(),
		Message:   "Hook: " + payload.Event + " in " + filepath.Base(payload.Project),
	}
	s.bus.Publish(event)

	// Also persist to store
	if s.store != nil {
		s.store.SaveNotification(store.NotificationRecord{
			SessionID: payload.SessionID,
			EventType: string(eventType),
			Timestamp: time.Now(),
		})
	}

	w.WriteHeader(http.StatusOK)
}

// handleInstallHooks installs/uninstalls websessions hooks in Claude settings.
func (s *Server) handleInstallHooks(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	baseURL := fmt.Sprintf("http://%s:%d", s.cfg.Server.Host, s.cfg.Server.Port)
	if s.cfg.Server.Host == "0.0.0.0" {
		baseURL = fmt.Sprintf("http://localhost:%d", s.cfg.Server.Port)
	}

	var err error
	switch payload.Action {
	case "install":
		err = hooks.Install(baseURL)
	case "uninstall":
		err = hooks.Uninstall()
	default:
		http.Error(w, "action must be 'install' or 'uninstall'", http.StatusBadRequest)
		return
	}

	if err != nil {
		slog.Error("hook action failed", "action", payload.Action, "error", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	// Check new status
	installed := false
	claudeSettings, loadErr := hooks.Load()
	if loadErr == nil {
		installed = claudeSettings.IsInstalled()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":        true,
		"action":    payload.Action,
		"installed": installed,
	})
}

func (s *Server) setupNotificationBridge() {
	s.bus.Subscribe(func(e notification.SessionEvent) {
		s.sink.Send(e)
		// Push to all connected notification WebSocket clients
		msg, _ := json.Marshal(map[string]string{
			"type":      "notification",
			"event":     string(e.Type),
			"sessionID": e.SessionID,
			"message":   e.Message,
			"timestamp": e.Timestamp.Format("15:04:05"),
		})
		s.hub.broadcastNotification(msg)
	})
}

// handleListDirs returns a JSON list of directories for the file finder autocomplete.
func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	data := templates.SettingsData{
		Port:             s.cfg.Server.Port,
		Host:             s.cfg.Server.Host,
		ScanInterval:     s.cfg.Sessions.ScanIntervalRaw,
		OutputBufferSize: s.cfg.Sessions.OutputBufferRaw,
		DefaultDir:       s.cfg.Sessions.DefaultDir,
		DesktopNotifs:    s.cfg.Notifications.Desktop,
	}
	// Check which events are enabled
	for _, e := range s.cfg.Notifications.Events {
		switch e {
		case "completed":
			data.NotifCompleted = true
		case "errored":
			data.NotifErrored = true
		case "waiting":
			data.NotifWaiting = true
		}
	}
	// Check if hooks are installed
	claudeSettings, err := hooks.Load()
	if err == nil {
		data.HooksInstalled = claudeSettings.IsInstalled()
	}
	templates.Settings(data).Render(r.Context(), w)
}

func (s *Server) handleSaveSettings(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	oldPort := s.cfg.Server.Port
	oldHost := s.cfg.Server.Host

	s.cfg.Server.Port, _ = strconv.Atoi(r.FormValue("port"))
	s.cfg.Server.Host = r.FormValue("host")
	s.cfg.Sessions.ScanIntervalRaw = r.FormValue("scan_interval")
	s.cfg.Sessions.OutputBufferRaw = r.FormValue("output_buffer_size")
	s.cfg.Sessions.DefaultDir = r.FormValue("default_dir")
	s.cfg.Notifications.Desktop = r.FormValue("desktop_notifs") == "on"

	var events []string
	if r.FormValue("notif_completed") == "on" {
		events = append(events, "completed")
	}
	if r.FormValue("notif_errored") == "on" {
		events = append(events, "errored")
	}
	if r.FormValue("notif_waiting") == "on" {
		events = append(events, "waiting")
	}
	s.cfg.Notifications.Events = events

	// If port or host changed, uninstall hooks (they contain the old URL)
	if s.cfg.Server.Port != oldPort || s.cfg.Server.Host != oldHost {
		claudeSettings, err := hooks.Load()
		if err == nil && claudeSettings.IsInstalled() {
			hooks.Uninstall()
			slog.Info("hooks uninstalled due to server address change — reinstall from settings")
		}
	}

	// Save config to disk
	if err := s.cfg.Save(); err != nil {
		slog.Error("failed to save config", "error", err)
		http.Error(w, "failed to save: "+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/settings?saved=1", http.StatusSeeOther)
}

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
