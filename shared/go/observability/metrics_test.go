package observability

import (
	"strings"
	"sync"
	"testing"
)

func TestSnapshotReplacementClearsStaleLabels(t *testing.T) {
	m := NewMetrics()
	m.SetGauge("queue_depth", map[string]string{"status": "queued"}, 4)
	m.ReplaceGauges("queue_depth", []GaugeSample{{Labels: map[string]string{"status": "queued"}, Value: 0}})
	if strings.Contains(m.Render(), " 4") {
		t.Fatal("stale nonzero backlog")
	}
	m.ReplaceGauges("queue_depth", nil)
	if strings.Contains(m.Render(), "queue_depth") {
		t.Fatal("stale series remains")
	}
}

func TestRegistryConcurrentSnapshotAndCounter(t *testing.T) {
	m := NewMetrics()
	var wg sync.WaitGroup
	for n := 0; n < 100; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.IncCounter("attempts", nil, 1)
			m.ReplaceGauges("depth", []GaugeSample{{Value: 0}})
			_ = m.Render()
		}()
	}
	wg.Wait()
	if m.CounterValue("attempts", nil) != 100 {
		t.Fatal("lost counter updates")
	}
}
