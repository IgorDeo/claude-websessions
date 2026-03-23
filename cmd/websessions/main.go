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

	mgr := session.NewManager(cfg.Sessions.OutputBufferSize)
	bus := notification.NewBus()
	sink := notification.NewInAppSink(100)

	mgr.OnStateChange(func(s *session.Session, from, to session.State) {
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
			ID: s.ID, ClaudeID: s.ClaudeID, WorkDir: s.WorkDir,
			StartTime: s.StartTime, EndTime: s.EndTime,
			ExitCode: s.ExitCode, Status: string(to), PID: s.PID,
		})
		st.SaveNotification(store.NotificationRecord{
			SessionID: s.ID, EventType: string(eventType), Timestamp: time.Now(),
		})
	})

	// Restore previous sessions from SQLite as offline
	prevSessions, err := st.ListSessions(50)
	if err == nil {
		for _, rec := range prevSessions {
			if rec.Status == "running" || rec.Status == "waiting" || rec.Status == "created" {
				mgr.AddOffline(rec.ID, rec.ID, rec.ClaudeID, rec.WorkDir)
				slog.Info("restored offline session", "id", rec.ID, "dir", rec.WorkDir)
			}
		}
	}

	go func() {
		processes, err := discovery.Scan()
		if err != nil {
			slog.Warn("initial discovery scan failed", "error", err)
			return
		}
		for _, p := range processes {
			id := fmt.Sprintf("discovered-%d", p.PID)
			mgr.AddDiscovered(id, p.ClaudeID, p.WorkDir, p.PID, p.StartTime)
			slog.Info("discovered claude session", "pid", p.PID, "dir", p.WorkDir)
		}
	}()

	if cfg.Sessions.ScanInterval > 0 {
		go func() {
			ticker := time.NewTicker(cfg.Sessions.ScanInterval)
			defer ticker.Stop()
			for range ticker.C {
				processes, err := discovery.Scan()
				if err != nil {
					slog.Debug("discovery scan error", "error", err)
					continue
				}
				for _, p := range processes {
					existing := mgr.List()
					found := false
					for _, s := range existing {
						if s.PID == p.PID {
							found = true
							break
						}
					}
					if !found {
						id := fmt.Sprintf("discovered-%d", p.PID)
						mgr.AddDiscovered(id, p.ClaudeID, p.WorkDir, p.PID, p.StartTime)
						slog.Info("discovered new claude session", "pid", p.PID)
					}
				}
			}
		}()
	}

	srv := server.New(cfg, mgr, bus, sink, st)
	httpServer := &http.Server{Addr: srv.Addr(), Handler: srv.Handler()}

	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("websessions starting", "addr", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-done
	slog.Info("shutting down...")

	for _, s := range mgr.List() {
		st.SaveSession(store.SessionRecord{
			ID: s.ID, ClaudeID: s.ClaudeID, WorkDir: s.WorkDir,
			StartTime: s.StartTime, EndTime: s.EndTime,
			ExitCode: s.ExitCode, Status: string(s.GetState()), PID: s.PID,
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	httpServer.Shutdown(ctx)
	slog.Info("websessions stopped")
}
