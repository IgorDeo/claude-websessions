package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/igor-deoalves/websessions/internal/config"
	"github.com/igor-deoalves/websessions/internal/discovery"
	"github.com/igor-deoalves/websessions/internal/notification"
	"github.com/igor-deoalves/websessions/internal/server"
	"github.com/igor-deoalves/websessions/internal/session"
	"github.com/igor-deoalves/websessions/internal/store"
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
	os.MkdirAll(dbDir, 0755)
	dbPath := dbDir + "/websessions.db"

	st, err := store.Open(dbPath)
	if err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer st.Close()

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
		if s.Killed && to == session.StateErrored {
			// Still save to DB but don't notify
			st.SaveSession(store.SessionRecord{
				ID: s.ID, Name: s.Name, ClaudeID: s.ClaudeID, WorkDir: s.WorkDir,
				StartTime: s.StartTime, EndTime: s.EndTime,
				ExitCode: s.ExitCode, Status: "killed", PID: s.PID,
			})
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
		st.SaveSession(store.SessionRecord{
			ID: s.ID, Name: s.Name, ClaudeID: s.ClaudeID, WorkDir: s.WorkDir,
			StartTime: s.StartTime, EndTime: s.EndTime,
			ExitCode: s.ExitCode, Status: string(to), PID: s.PID,
		})
		st.SaveNotification(store.NotificationRecord{
			SessionID: s.ID, EventType: string(eventType), Timestamp: time.Now(),
		})
	})

	// Restore previous sessions from SQLite as offline
	offlineCount := 0
	prevSessions, err := st.ListSessions(50)
	if err == nil {
		for _, rec := range prevSessions {
			if rec.Status == "running" || rec.Status == "waiting" || rec.Status == "created" || rec.Status == "discovered" {
				name := rec.Name
				if name == "" {
					name = rec.ID
				}
				mgr.AddOffline(rec.ID, name, rec.ClaudeID, rec.WorkDir)
				offlineCount++
			}
		}
	}
	printOffline(offlineCount)

	// Initial discovery scan (synchronous so banner shows correct count)
	discoveredCount := 0
	processes, scanErr := discovery.Scan()
	if scanErr == nil {
		for _, p := range processes {
			id := fmt.Sprintf("discovered-%d", p.PID)
			s := mgr.AddDiscovered(id, p.ClaudeID, p.WorkDir, p.PID, p.StartTime)
			st.SaveSession(store.SessionRecord{
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
						st.SaveSession(store.SessionRecord{
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

				// Build set of PIDs already tracked
				existingPIDs := make(map[int]bool)
				for _, s := range mgr.List() {
					if s.PID > 0 {
						existingPIDs[s.PID] = true
					}
				}

				for _, p := range processes {
					if existingPIDs[p.PID] {
						continue
					}
					id := fmt.Sprintf("discovered-%d", p.PID)
					s := mgr.AddDiscovered(id, p.ClaudeID, p.WorkDir, p.PID, p.StartTime)
					st.SaveSession(store.SessionRecord{
						ID: id, Name: s.Name, ClaudeID: p.ClaudeID, WorkDir: p.WorkDir,
						StartTime: p.StartTime, Status: "discovered", PID: p.PID,
					})
					slog.Info("discovered new claude session", "pid", p.PID)
				}
			}
		}()
	}

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
						st.SaveNotification(store.NotificationRecord{
							SessionID: s.ID,
							EventType: "waiting",
							Timestamp: time.Now(),
						})
					}
				}
			}
		}()
	}

	srv := server.New(cfg, mgr, bus, sink, st)
	srv.SetVersion(version)

	// Expose snooze function to the server for the snooze API
	srv.SetSnoozeFunc(func(sessionID string, minutes int) {
		snoozedSessions[sessionID] = time.Now().Add(time.Duration(minutes) * time.Minute)
	})

	httpServer := &http.Server{Addr: srv.Addr(), Handler: srv.Handler()}

	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "\n  %s%s✗ Failed to start: %s%s\n\n", colorRed, colorBold, err, colorReset)
			os.Exit(1)
		}
	}()

	<-done
	printShutdown()

	for _, s := range mgr.List() {
		st.SaveSession(store.SessionRecord{
			ID: s.ID, Name: s.Name, ClaudeID: s.ClaudeID, WorkDir: s.WorkDir,
			StartTime: s.StartTime, EndTime: s.EndTime,
			ExitCode: s.ExitCode, Status: string(s.GetState()), PID: s.PID,
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	httpServer.Shutdown(ctx)
	printStopped()
}
