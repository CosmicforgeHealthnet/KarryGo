package movementusecases

import (
	"context"
	"strings"
	"testing"

	"cosmicforge/logistics/shared/go/events"
	"cosmicforge/logistics/shared/go/observability"
)

// Customer V4 §27.2: ingestion outcomes and lag are counted per authority
// with bounded labels only.
func TestIngestRecordsBoundedMovementMetrics(t *testing.T) {
	repo := newMemoryRepo()
	m := observability.NewMetrics()
	svc := NewService(Options{Repository: repo, Callers: callers, ConsumerEnabled: true, Metrics: m})
	ctx := context.Background()
	first := envelope(t, "m1", "driver-taxi-service", "taxi.booking.created", 2, taxi(events.MovementStateSearching, 0))
	if _, err := svc.Ingest(ctx, "driver-taxi-service", first); err != nil {
		t.Fatal(err)
	}
	_, _ = svc.Ingest(ctx, "driver-taxi-service", envelope(t, "m0", "driver-taxi-service", "taxi.booking.created", 1, taxi(events.MovementStateSearching, 0)))
	_, _ = svc.Ingest(ctx, "vendor-service", first)
	for labels, want := range map[[2]string]float64{
		{"TAXI", "applied"}: 1, {"TAXI", "stale"}: 1, {"TAXI", "authorization_denied"}: 1,
	} {
		if got := m.CounterValue("karry_movement_ingest_total", map[string]string{"authority": labels[0], "outcome": labels[1]}); got != want {
			t.Errorf("%v = %v, want %v", labels, got, want)
		}
	}
	out := m.Render()
	if !strings.Contains(out, `karry_movement_ingest_lag_seconds_count{authority="TAXI"}`) {
		t.Fatalf("lag not observed:\n%s", out)
	}
	for _, forbidden := range []string{customerA, "m1", "source_ref", "customer"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("metric output carries %q:\n%s", forbidden, out)
		}
	}
}
