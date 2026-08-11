package vendors

import (
	"strings"
	"testing"
)

func TestSection7MetricsUseOnlyBoundedLabels(t *testing.T) {
	m := NewSection7Metrics()
	m.Inc("verification_status_transition_total", "status", "VERIFIED", "dimension", "BUSINESS")
	got := m.Snapshot()
	if got["verification_status_transition_total{dimension=BUSINESS,status=VERIFIED}"] != 1 {
		t.Fatalf("snapshot=%v", got)
	}
	for key := range got {
		if strings.Contains(key, "vendor_") || strings.Contains(key, "account") || strings.Contains(key, "@") {
			t.Fatalf("sensitive or unbounded metric label: %s", key)
		}
	}
}
