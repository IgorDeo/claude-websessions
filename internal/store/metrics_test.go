package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestMetricsSaveAndQuery(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close() //nolint:errcheck

	now := time.Now().Truncate(time.Second)
	samples := map[string]float64{
		"sessions_active":      3,
		"goroutines":           42,
		"memory_alloc_bytes":   1024000,
		"notifications_pending": 2,
	}

	if err := st.SaveMetricSamples(samples, now); err != nil {
		t.Fatalf("SaveMetricSamples: %v", err)
	}

	got, err := st.QueryMetrics("goroutines", now.Add(-time.Minute), now.Add(time.Minute))
	if err != nil {
		t.Fatalf("QueryMetrics: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 sample, got %d", len(got))
	}
	if got[0].Value != 42 {
		t.Errorf("expected value 42, got %g", got[0].Value)
	}
}

func TestMetricsQueryAllMetrics(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close() //nolint:errcheck

	now := time.Now().Truncate(time.Second)
	_ = st.SaveMetricSamples(map[string]float64{"a": 1, "b": 2}, now)
	_ = st.SaveMetricSamples(map[string]float64{"a": 10, "b": 20}, now.Add(time.Minute))

	all, err := st.QueryAllMetrics(now.Add(-time.Minute), now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("QueryAllMetrics: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 metrics, got %d", len(all))
	}
	if len(all["a"]) != 2 {
		t.Errorf("expected 2 samples for 'a', got %d", len(all["a"]))
	}
	if all["a"][0].Value != 1 || all["a"][1].Value != 10 {
		t.Errorf("unexpected values for 'a': %v", all["a"])
	}
}

func TestMetricsPrune(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close() //nolint:errcheck

	old := time.Now().Add(-48 * time.Hour).Truncate(time.Second)
	recent := time.Now().Truncate(time.Second)

	_ = st.SaveMetricSamples(map[string]float64{"x": 1}, old)
	_ = st.SaveMetricSamples(map[string]float64{"x": 2}, recent)

	deleted, err := st.PruneMetrics(time.Now().Add(-24 * time.Hour))
	if err != nil {
		t.Fatalf("PruneMetrics: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", deleted)
	}

	remaining, _ := st.QueryMetrics("x", old, recent.Add(time.Minute))
	if len(remaining) != 1 {
		t.Errorf("expected 1 remaining, got %d", len(remaining))
	}
}

func TestMetricsQueryEmpty(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close() //nolint:errcheck

	got, err := st.QueryMetrics("nonexistent", time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Fatalf("QueryMetrics: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 samples, got %d", len(got))
	}
}
