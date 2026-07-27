package jobs

import (
	"context"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
)

// Expirer marks overdue records expired and reports how many changed.
type Expirer interface {
	ExpireDue(ctx context.Context, limit int) (int64, error)
}

// MaintenanceWorker runs the Section 8–10 expiry sweeps: OTP challenges,
// corporate sessions, staff invitations and provider invitations.
//
// Every sweep is a bounded, idempotent UPDATE — nothing here completes an
// operation based on a timeout assumption; it only expires records whose
// deadline has genuinely passed.
type MaintenanceWorker struct {
	Metrics  *observability.Metrics
	Interval time.Duration
	Batch    int

	// Sweeps are named so metrics stay low-cardinality and a failing sweep is
	// identifiable without logging record identifiers.
	Sweeps map[string]Expirer
}

// Run sweeps until ctx is cancelled.
func (w *MaintenanceWorker) Run(ctx context.Context) {
	if w.Interval <= 0 {
		w.Interval = time.Minute
	}
	if w.Batch <= 0 {
		w.Batch = 200
	}
	ticker := time.NewTicker(w.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.tick(ctx)
		}
	}
}

func (w *MaintenanceWorker) tick(ctx context.Context) {
	for name, sweep := range w.Sweeps {
		if sweep == nil {
			continue
		}
		expired, err := sweep.ExpireDue(ctx, w.Batch)
		if err != nil {
			w.Metrics.IncCounter("fleet_partner_maintenance_errors_total",
				map[string]string{"sweep": name}, 1)
			continue
		}
		if expired > 0 {
			w.Metrics.IncCounter("fleet_partner_maintenance_expired_total",
				map[string]string{"sweep": name}, float64(expired))
		}
	}
}
