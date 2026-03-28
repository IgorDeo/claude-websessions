package metrics

import (
	"log/slog"
	"runtime"
	"time"

	"github.com/IgorDeo/claude-websessions/internal/notification"
	"github.com/IgorDeo/claude-websessions/internal/session"
	"github.com/IgorDeo/claude-websessions/internal/store"
)

var startTime = time.Now()

// Collector periodically samples metrics and stores them in SQLite.
type Collector struct {
	store         *store.Store
	mgr           *session.Manager
	sink          *notification.InAppSink
	interval      time.Duration
	retentionDays int
	done          chan struct{}
}

// New creates a metrics collector.
func New(st *store.Store, mgr *session.Manager, sink *notification.InAppSink, interval time.Duration, retentionDays int) *Collector {
	return &Collector{
		store:         st,
		mgr:           mgr,
		sink:          sink,
		interval:      interval,
		retentionDays: retentionDays,
		done:          make(chan struct{}),
	}
}

// Start begins the collection and pruning loops.
func (c *Collector) Start() {
	// Prune on startup
	cutoff := time.Now().AddDate(0, 0, -c.retentionDays)
	if deleted, err := c.store.PruneMetrics(cutoff); err != nil {
		slog.Error("metrics startup prune failed", "error", err)
	} else if deleted > 0 {
		slog.Debug("pruned old metrics on startup", "deleted", deleted)
	}

	go func() {
		collectTicker := time.NewTicker(c.interval)
		pruneTicker := time.NewTicker(1 * time.Hour)
		defer collectTicker.Stop()
		defer pruneTicker.Stop()

		for {
			select {
			case <-collectTicker.C:
				c.collect()
			case <-pruneTicker.C:
				cutoff := time.Now().AddDate(0, 0, -c.retentionDays)
				if deleted, err := c.store.PruneMetrics(cutoff); err != nil {
					slog.Error("metrics prune failed", "error", err)
				} else if deleted > 0 {
					slog.Debug("pruned old metrics", "deleted", deleted)
				}
			case <-c.done:
				return
			}
		}
	}()
}

// Stop signals the collector goroutine to exit.
func (c *Collector) Stop() {
	close(c.done)
}

func (c *Collector) collect() {
	sessions := c.mgr.List()

	activeCount := 0
	stateCounts := make(map[session.State]int)
	for _, sess := range sessions {
		st := sess.GetState()
		stateCounts[st]++
		if st == session.StateRunning || st == session.StateWaiting {
			activeCount++
		}
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	samples := map[string]float64{
		"sessions_active":       float64(activeCount),
		"uptime_seconds":        time.Since(startTime).Seconds(),
		"goroutines":            float64(runtime.NumGoroutine()),
		"memory_alloc_bytes":    float64(m.Alloc),
		"notifications_pending": float64(c.sink.UnreadCount()),
	}

	// Add per-state session counts
	for state, count := range stateCounts {
		samples["sessions_"+string(state)] = float64(count)
	}

	if err := c.store.SaveMetricSamples(samples, time.Now()); err != nil {
		slog.Error("failed to save metric samples", "error", err)
	}
}
