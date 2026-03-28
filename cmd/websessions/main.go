package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/IgorDeo/claude-websessions/internal/config"
	"github.com/IgorDeo/claude-websessions/internal/discovery"
	"github.com/IgorDeo/claude-websessions/internal/docker"
	"github.com/IgorDeo/claude-websessions/internal/metrics"
	"github.com/IgorDeo/claude-websessions/internal/notification"
	"github.com/IgorDeo/claude-websessions/internal/server"
	"github.com/IgorDeo/claude-websessions/internal/session"
	"github.com/IgorDeo/claude-websessions/internal/store"
	"github.com/IgorDeo/claude-websessions/internal/teams"
)


var version = "dev"

const (
	colorReset  = "\033[0m"
	colorDim    = "\033[2m"
	colorBold   = "\033[1m"
	colorBlue   = "\033[38;5;111m"
	colorGreen  = "\033[38;5;114m"
	colorYellow = "\033[38;5;179m"
	colorCyan   = "\033[38;5;116m"
	colorRed    = "\033[38;5;174m"
)

func printBanner(_ string, host string, port int) {
	url := fmt.Sprintf("http://localhost:%d", port)
	if host != "0.0.0.0" && host != "" {
		url = fmt.Sprintf("http://%s:%d", host, port)
	}

	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "%s%s", colorBlue, colorBold)
	fmt.Fprintf(os.Stderr, "  ██╗    ██╗███████╗██████╗ ███████╗███████╗███████╗███████╗██╗ ██████╗ ███╗   ██╗███████╗\n")
	fmt.Fprintf(os.Stderr, "  ██║    ██║██╔════╝██╔══██╗██╔════╝██╔════╝██╔════╝██╔════╝██║██╔═══██╗████╗  ██║██╔════╝\n")
	fmt.Fprintf(os.Stderr, "  ██║ █╗ ██║█████╗  ██████╔╝███████╗█████╗  ███████╗███████╗██║██║   ██║██╔██╗ ██║███████╗\n")
	fmt.Fprintf(os.Stderr, "  ██║███╗██║██╔══╝  ██╔══██╗╚════██║██╔══╝  ╚════██║╚════██║██║██║   ██║██║╚██╗██║╚════██║\n")
	fmt.Fprintf(os.Stderr, "  ╚███╔███╔╝███████╗██████╔╝███████║███████╗███████║███████║██║╚██████╔╝██║ ╚████║███████║\n")
	fmt.Fprintf(os.Stderr, "   ╚══╝╚══╝ ╚══════╝╚═════╝ ╚══════╝╚══════╝╚══════╝╚══════╝╚═╝ ╚═════╝ ╚═╝  ╚═══╝╚══════╝\n")
	fmt.Fprintf(os.Stderr, "%s", colorReset)
	fmt.Fprintf(os.Stderr, "%s  Claude Code Session Manager%s\n", colorDim, colorReset)
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  %s%s➜  Local:%s   %s%s%s\n", colorBold, colorGreen, colorReset, colorBold, url, colorReset)
	if host == "0.0.0.0" {
		fmt.Fprintf(os.Stderr, "  %s%s➜  Network:%s %shttp://%s:%d%s\n", colorBold, colorCyan, colorReset, colorDim, host, port, colorReset)
	}
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  %sversion %s  •  press %sCtrl+C%s%s to stop%s\n", colorDim, version, colorReset+colorYellow, colorReset+colorDim, "", colorReset)
	fmt.Fprintf(os.Stderr, "\n")
}

func printDiscovery(count int) {
	if count == 0 {
		fmt.Fprintf(os.Stderr, "  %s○ No running Claude sessions found%s\n\n", colorDim, colorReset)
	} else {
		fmt.Fprintf(os.Stderr, "  %s%s● Discovered %d Claude session(s)%s\n\n", colorGreen, colorBold, count, colorReset)
	}
}

func printOffline(count int) {
	if count > 0 {
		fmt.Fprintf(os.Stderr, "  %s◐ Restored %d offline session(s) from history%s\n", colorYellow, count, colorReset)
	}
}

func printShutdown() {
	fmt.Fprintf(os.Stderr, "\n  %s%s⏻ Shutting down gracefully...%s\n", colorYellow, colorBold, colorReset)
}

func printStopped() {
	fmt.Fprintf(os.Stderr, "  %s%s✓ All sessions saved. Goodbye!%s\n\n", colorGreen, colorBold, colorReset)
}

func isProcessAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

func main() {
	configPath := ""
	logLevel := "info"
	guiMode := false
	for i, arg := range os.Args[1:] {
		switch arg {
		case "--config":
			if i+1 < len(os.Args)-1 {
				configPath = os.Args[i+2]
			}
		case "--log-level":
			if i+1 < len(os.Args)-1 {
				logLevel = os.Args[i+2]
			}
		case "--gui":
			guiMode = true
		}
	}

	var level slog.Level
	switch logLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	if configPath == "" {
		homeDir, _ := os.UserHomeDir()
		defaultPath := homeDir + "/.websessions/config.yaml"
		if _, err := os.Stat(defaultPath); err == nil {
			configPath = defaultPath
		}
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	homeDir, _ := os.UserHomeDir()
	dbDir := homeDir + "/.websessions"
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		slog.Error("failed to create data directory", "path", dbDir, "error", err)
		os.Exit(1)
	}
	dbPath := dbDir + "/websessions.db"

	st, err := store.Open(dbPath)
	if err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer st.Close() //nolint:errcheck

	// Print banner early
	printBanner("", cfg.Server.Host, cfg.Server.Port)

	mgr := session.NewManager(cfg.Sessions.OutputBufferSize)
	bus := notification.NewBus()
	sink := notification.NewInAppSink(100)

	mgr.OnStateChange(func(s *session.Session, from, to session.State) {
		// Resolve claude session ID if not known yet (for future --resume)
		if s.ClaudeID == "" && s.WorkDir != "" {
			s.ClaudeID = discovery.ResolveClaudeSessionID(s.WorkDir)
		}

		// Skip notification for intentionally killed sessions
		if s.IsKilled() && to == session.StateErrored {
			// Still save to DB but don't notify
			_ = st.SaveSession(store.SessionRecord{
				ID: s.ID, Name: s.Name, ClaudeID: s.ClaudeID, WorkDir: s.WorkDir,
				StartTime: s.StartTime, EndTime: s.EndTime,
				ExitCode: s.ExitCode, Status: "killed", PID: s.PID,
				Sandboxed: s.Sandboxed, SandboxName: s.SandboxName,
				TeamName: s.TeamName, TeamRole: s.TeamRole,
			})
			if buf := s.Output().Bytes(); len(buf) > 0 {
				_ = st.SaveOutput(s.ID, buf)
			}
			return
		}

		var eventType notification.EventType
		switch to {
		case session.StateCompleted:
			eventType = notification.EventCompleted
		case session.StateErrored:
			eventType = notification.EventErrored
		case session.StateWaiting:
			eventType = notification.EventWaiting
		default:
			return
		}
		event := notification.SessionEvent{SessionID: s.ID, Type: eventType, Timestamp: time.Now()}
		bus.Publish(event)
		_ = st.SaveSession(store.SessionRecord{
			ID: s.ID, Name: s.Name, ClaudeID: s.ClaudeID, WorkDir: s.WorkDir,
			StartTime: s.StartTime, EndTime: s.EndTime,
			ExitCode: s.ExitCode, Status: string(to), PID: s.PID,
			Sandboxed: s.Sandboxed, SandboxName: s.SandboxName,
			TeamName: s.TeamName, TeamRole: s.TeamRole,
		})
		_ = st.SaveNotification(store.NotificationRecord{
			SessionID: s.ID, EventType: string(eventType), Timestamp: time.Now(),
		})
		if to == session.StateCompleted || to == session.StateErrored {
			if buf := s.Output().Bytes(); len(buf) > 0 {
				_ = st.SaveOutput(s.ID, buf)
			}
		}
	})

	// Check tmux availability
	if !session.TmuxIsAvailable() {
		fmt.Fprintf(os.Stderr, "  %s%s✗ tmux is not installed. Please install tmux to use websessions.%s\n", colorRed, colorBold, colorReset)
		fmt.Fprintf(os.Stderr, "  %s  Ubuntu/Debian: sudo apt install tmux%s\n", colorDim, colorReset)
		fmt.Fprintf(os.Stderr, "  %s  macOS: brew install tmux%s\n\n", colorDim, colorReset)
		os.Exit(1)
	}

	// Recover existing tmux sessions from a previous server run
	recoveredCount := mgr.RecoverTmuxSessions()
	if recoveredCount > 0 {
		fmt.Fprintf(os.Stderr, "  %s%s● Reattached to %d tmux session(s)%s\n", colorGreen, colorBold, recoveredCount, colorReset)
		// Preload persisted output for reattached sessions
		for _, s := range mgr.List() {
			if s.GetState() == session.StateRunning {
				if data, err := st.LoadOutput(s.ID); err == nil && len(data) > 0 {
					s.PreloadOutput(data)
				}
			}
		}
	}

	// Restore previous sessions from SQLite as offline (only those not already recovered)
	offlineCount := 0
	prevSessions, err := st.ListSessions(50)
	if err == nil {
		for _, rec := range prevSessions {
			// Only restore owned sessions that were running — skip
			// discovered (never owned), killed, completed, errored
			if rec.Status != "running" && rec.Status != "waiting" && rec.Status != "created" {
				continue
			}
			if strings.HasPrefix(rec.ID, "discovered-") {
				continue
			}
			// Skip if already recovered from tmux
			if _, ok := mgr.Get(rec.ID); ok {
				continue
			}
			name := rec.Name
			if name == "" {
				name = rec.ID
			}
			offlineSess := mgr.AddOffline(rec.ID, name, rec.ClaudeID, rec.WorkDir, rec.TeamName, rec.TeamRole)
			if data, err := st.LoadOutput(rec.ID); err == nil && len(data) > 0 {
				offlineSess.PreloadOutput(data)
			}
			offlineCount++
		}
	}
	printOffline(offlineCount)

	// Initial discovery scan (synchronous so banner shows correct count)
	discoveredCount := 0
	processes, scanErr := discovery.Scan()
	if scanErr == nil {
		// Build dedup sets from already-recovered sessions
		existingPIDs := make(map[int]bool)
		existingDirs := make(map[string]bool)
		for _, s := range mgr.List() {
			if s.PID > 0 {
				existingPIDs[s.PID] = true
			}
			if s.WorkDir != "" && s.Owned {
				existingDirs[s.WorkDir] = true
			}
		}
		for _, p := range processes {
			if existingPIDs[p.PID] {
				continue
			}
			if p.WorkDir != "" && existingDirs[p.WorkDir] {
				continue
			}
			id := fmt.Sprintf("discovered-%d", p.PID)
			s := mgr.AddDiscovered(id, p.ClaudeID, p.WorkDir, p.PID, p.StartTime)
			_ = st.SaveSession(store.SessionRecord{
				ID: id, Name: s.Name, ClaudeID: p.ClaudeID, WorkDir: p.WorkDir,
				StartTime: p.StartTime, Status: "discovered", PID: p.PID,
			})
			discoveredCount++
		}
	}
	printDiscovery(discoveredCount)

	if cfg.Sessions.ScanInterval > 0 {
		go func() {
			ticker := time.NewTicker(cfg.Sessions.ScanInterval)
			defer ticker.Stop()
			for range ticker.C {
				// Health check: remove discovered sessions whose process died
				for _, s := range mgr.List() {
					if s.GetState() != session.StateDiscovered {
						continue
					}
					if s.PID > 0 && !isProcessAlive(s.PID) {
						slog.Info("discovered session process died, removing", "id", s.ID, "pid", s.PID)
						_ = st.SaveSession(store.SessionRecord{
							ID: s.ID, Name: s.Name, ClaudeID: s.ClaudeID, WorkDir: s.WorkDir,
							StartTime: s.StartTime, EndTime: time.Now(),
							Status: "completed", PID: s.PID,
						})
						mgr.Remove(s.ID)
					}
				}

				// Discover new claude processes
				processes, err := discovery.Scan()
				if err != nil {
					slog.Debug("discovery scan error", "error", err)
					continue
				}

				// Build sets of PIDs and WorkDirs already tracked
				existingPIDs := make(map[int]bool)
				existingDirs := make(map[string]bool)
				for _, s := range mgr.List() {
					if s.PID > 0 {
						existingPIDs[s.PID] = true
					}
					if s.WorkDir != "" && s.Owned {
						existingDirs[s.WorkDir] = true
					}
				}

				for _, p := range processes {
					if existingPIDs[p.PID] {
						continue
					}
					// Skip if an owned session already manages this directory
					if p.WorkDir != "" && existingDirs[p.WorkDir] {
						continue
					}
					// Skip if this PID was recently killed by the user
					if mgr.WasKilled(p.PID) {
						continue
					}
					id := fmt.Sprintf("discovered-%d", p.PID)
					s := mgr.AddDiscovered(id, p.ClaudeID, p.WorkDir, p.PID, p.StartTime)
					_ = st.SaveSession(store.SessionRecord{
						ID: id, Name: s.Name, ClaudeID: p.ClaudeID, WorkDir: p.WorkDir,
						StartTime: p.StartTime, Status: "discovered", PID: p.PID,
					})
					slog.Info("discovered new claude session", "pid", p.PID)
				}
			}
		}()
	}

	// Auto-cleanup: remove completed/errored sessions from active list after 5 minutes
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			for _, s := range mgr.List() {
				state := s.GetState()
				if (state == session.StateCompleted || state == session.StateErrored) && !s.EndTime.IsZero() {
					if time.Since(s.EndTime) > 5*time.Minute {
						mgr.Remove(s.ID)
						slog.Debug("auto-archived stale session", "id", s.ID, "state", state)
					}
				}
			}
		}
	}()

	// Waiting session reminder — re-notifies if a session stays in waiting state
	snoozedSessions := make(map[string]time.Time) // session ID -> snooze until
	if cfg.Notifications.ReminderMinutes > 0 {
		reminderInterval := time.Duration(cfg.Notifications.ReminderMinutes) * time.Minute
		go func() {
			ticker := time.NewTicker(30 * time.Second) // check every 30s
			defer ticker.Stop()
			for range ticker.C {
				for _, s := range mgr.List() {
					if s.GetState() != session.StateWaiting {
						// Clear snooze when session is no longer waiting
						delete(snoozedSessions, s.ID)
						continue
					}
					// Check if snoozed
					if until, ok := snoozedSessions[s.ID]; ok && time.Now().Before(until) {
						continue
					}
					// Check if waiting long enough
					// Use last state change time approximation: if session is waiting
					// and we haven't reminded recently, fire a reminder
					snoozedSessions[s.ID] = time.Now().Add(reminderInterval)
					event := notification.SessionEvent{
						SessionID: s.ID,
						Type:      notification.EventWaiting,
						Timestamp: time.Now(),
						Message:   s.Name + " is still waiting for your input",
					}
					bus.Publish(event)
					if st != nil {
						_ = st.SaveNotification(store.NotificationRecord{
							SessionID: s.ID,
							EventType: "waiting",
							Timestamp: time.Now(),
						})
					}
				}
			}
		}()
	}

	// Agent Teams integration (opt-in via config)
	var teamMgr *teams.Manager
	if cfg.Teams.Enabled {
		// Check Claude Code version for agent teams support
		ver, supported := teams.CheckClaudeVersion()
		if ver != "" {
			fmt.Fprintf(os.Stderr, "  %s● Agent teams enabled (Claude Code %s)%s\n", colorGreen, ver, colorReset)
			if !supported {
				fmt.Fprintf(os.Stderr, "  %s⚠ Agent teams require Claude Code v2.1.32+%s\n", colorYellow, colorReset)
			}
		} else {
			fmt.Fprintf(os.Stderr, "  %s⚠ Agent teams enabled but Claude Code not found in PATH%s\n", colorYellow, colorReset)
		}

		teamMgr = teams.NewManager(mgr, bus)
		go func() {
			ticker := time.NewTicker(cfg.Teams.ScanInterval)
			defer ticker.Stop()
			for range ticker.C {
				if err := teamMgr.Scan(); err != nil {
					slog.Debug("team scan error", "error", err)
				}
			}
		}()
	}

	srv := server.New(cfg, mgr, bus, sink, st, teamMgr)
	srv.SetVersion(version)

	// Expose snooze function to the server for the snooze API
	srv.SetSnoozeFunc(func(sessionID string, minutes int) {
		snoozedSessions[sessionID] = time.Now().Add(time.Duration(minutes) * time.Minute)
	})

	// Metrics collector
	mc := metrics.New(st, mgr, sink, cfg.Metrics.SampleInterval, cfg.Metrics.RetentionDays)
	mc.Start()
	defer mc.Stop()

	httpServer := &http.Server{Addr: srv.Addr(), Handler: srv.Handler()}

	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "\n  %s%s✗ Failed to start: %s%s\n\n", colorRed, colorBold, err, colorReset)
			os.Exit(1)
		}
	}()

	if guiMode {
		go func() {
			url := fmt.Sprintf("http://localhost:%d", cfg.Server.Port)
			if err := openGUI(url); err != nil {
				fmt.Fprintf(os.Stderr, "\n  %s%s✗ GUI error: %s%s\n\n", colorRed, colorBold, err, colorReset)
			}
			// Window closed — trigger shutdown
			done <- syscall.SIGTERM
		}()
	}

	<-done
	printShutdown()

	for _, s := range mgr.List() {
		// Stop sandboxed sessions (don't remove — user might restart server)
		if s.Sandboxed && s.SandboxName != "" {
			if err := docker.SandboxStop(s.SandboxName); err != nil {
				slog.Warn("failed to stop sandbox on shutdown", "name", s.SandboxName, "error", err)
			}
		}
		_ = st.SaveSession(store.SessionRecord{
			ID: s.ID, Name: s.Name, ClaudeID: s.ClaudeID, WorkDir: s.WorkDir,
			StartTime: s.StartTime, EndTime: s.EndTime,
			ExitCode: s.ExitCode, Status: string(s.GetState()), PID: s.PID,
			Sandboxed: s.Sandboxed, SandboxName: s.SandboxName,
			TeamName: s.TeamName, TeamRole: s.TeamRole,
		})
		if buf := s.Output().Bytes(); len(buf) > 0 {
			_ = st.SaveOutput(s.ID, buf)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		slog.Error("HTTP server shutdown error", "error", err)
	}
	printStopped()
}
