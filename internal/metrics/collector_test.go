package metrics

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/IgorDeo/claude-websessions/internal/notification"
	"github.com/IgorDeo/claude-websessions/internal/session"
	"github.com/IgorDeo/claude-websessions/internal/store"
)

func TestCollector_Collect(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close() //nolint:errcheck

	mgr := session.NewManager(1024)
	sink := notification.NewInAppSink(100)

	c := New(st, mgr, sink, time.Minute, 7)
	c.collect()

	now := time.Now()
	all, err := st.QueryAllMetrics(now.Add(-time.Minute), now.Add(time.Minute))
	if err != nil {
		t.Fatalf("QueryAllMetrics: %v", err)
	}

	required := []string{"sessions_active", "uptime_seconds", "goroutines", "memory_alloc_bytes", "notifications_pending"}
	for _, name := range required {
		if _, ok := all[name]; !ok {
			t.Errorf("missing expected metric %q in collected samples", name)
		}
	}
}

func TestCollector_StartStop(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close() //nolint:errcheck

	mgr := session.NewManager(1024)
	sink := notification.NewInAppSink(100)

	c := New(st, mgr, sink, 50*time.Millisecond, 7)
	c.Start()
	time.Sleep(200 * time.Millisecond)
	c.Stop()

	now := time.Now()
	all, err := st.QueryAllMetrics(now.Add(-time.Minute), now.Add(time.Minute))
	if err != nil {
		t.Fatalf("QueryAllMetrics: %v", err)
	}
	if len(all) == 0 {
		t.Error("expected at least one metric after collector ran")
	}
}
