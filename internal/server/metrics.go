package server

import (
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/IgorDeo/claude-websessions/internal/session"
)

var startTime = time.Now()

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	sessions := s.mgr.List()

	// Count sessions per state
	stateCounts := make(map[session.State]int)
	for _, sess := range sessions {
		stateCounts[sess.GetState()]++
	}

	// websessions_sessions_total
	_, _ = fmt.Fprintf(w, "# HELP websessions_sessions_total Number of sessions per state\n")
	_, _ = fmt.Fprintf(w, "# TYPE websessions_sessions_total gauge\n")
	if len(stateCounts) == 0 {
		_, _ = fmt.Fprintf(w, "websessions_sessions_total{state=\"none\"} 0\n")
	} else {
		for state, count := range stateCounts {
			_, _ = fmt.Fprintf(w, "websessions_sessions_total{state=%q} %d\n", state, count)
		}
	}

	// websessions_sessions_active
	activeCount := 0
	for _, sess := range sessions {
		st := sess.GetState()
		if st == session.StateRunning || st == session.StateWaiting {
			activeCount++
		}
	}
	_, _ = fmt.Fprintf(w, "# HELP websessions_sessions_active Number of active (running or waiting) sessions\n")
	_, _ = fmt.Fprintf(w, "# TYPE websessions_sessions_active gauge\n")
	_, _ = fmt.Fprintf(w, "websessions_sessions_active %d\n", activeCount)

	// websessions_uptime_seconds
	uptime := time.Since(startTime).Seconds()
	_, _ = fmt.Fprintf(w, "# HELP websessions_uptime_seconds Seconds since server start\n")
	_, _ = fmt.Fprintf(w, "# TYPE websessions_uptime_seconds gauge\n")
	_, _ = fmt.Fprintf(w, "websessions_uptime_seconds %g\n", uptime)

	// websessions_goroutines
	_, _ = fmt.Fprintf(w, "# HELP websessions_goroutines Number of goroutines\n")
	_, _ = fmt.Fprintf(w, "# TYPE websessions_goroutines gauge\n")
	_, _ = fmt.Fprintf(w, "websessions_goroutines %d\n", runtime.NumGoroutine())

	// websessions_memory_alloc_bytes
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	_, _ = fmt.Fprintf(w, "# HELP websessions_memory_alloc_bytes Bytes of allocated heap objects\n")
	_, _ = fmt.Fprintf(w, "# TYPE websessions_memory_alloc_bytes gauge\n")
	_, _ = fmt.Fprintf(w, "websessions_memory_alloc_bytes %d\n", m.Alloc)

	// websessions_notifications_pending
	_, _ = fmt.Fprintf(w, "# HELP websessions_notifications_pending Number of unread notifications\n")
	_, _ = fmt.Fprintf(w, "# TYPE websessions_notifications_pending gauge\n")
	pendingCount := 0
	if s.sink != nil {
		pendingCount = s.sink.UnreadCount()
	}
	_, _ = fmt.Fprintf(w, "websessions_notifications_pending %d\n", pendingCount)
}
