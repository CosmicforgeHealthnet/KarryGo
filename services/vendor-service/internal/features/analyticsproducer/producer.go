// Package analyticsproducer is Vendor's owner-side Analytics coverage producer
// (Admin Section 6). Vendor has no own five-scope resource authorization
// contexts, so it attests GLOBAL coverage: the complete set of readiness facts
// it published per window, signed with Vendor's own key. Analytics then admits
// a fact to a scope only through Vendor's existing independently signed
// per-fact allocations, and refuses COMPLETE if any attested fact has none.
// Business/branch identifiers are never treated as scope. Default off.
package analyticsproducer

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/analyticsadmin"
	"cosmicforge/logistics/shared/go/networkanalytics"
)

const (
	Owner    = "vendor-service"
	Prefix   = "VENDOR"
	maxFacts = 200000
)

// Facts lists readiness facts from vendor_outbox, using the delivered
// envelope (the outbox payload IS the envelope the network ingest receives).
type Facts struct{ DB *pgxpool.Pool }

func (f Facts) Facts(ctx context.Context, start, end time.Time) ([]analyticsadmin.ProducerFact, error) {
	// created_at and the envelope's occurredAt are taken a moment apart in one
	// transaction; the margin catches boundary rows, the filter below decides.
	rows, err := f.DB.Query(ctx, `SELECT payload FROM vendor_outbox WHERE event_type=$1 AND created_at>=$2 AND created_at<$3 ORDER BY created_at LIMIT $4`, networkanalytics.EventBranchReadiness, start.Add(-time.Hour), end.Add(time.Hour), maxFacts+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []analyticsadmin.ProducerFact{}
	count := 0
	for rows.Next() {
		count++
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var e struct {
			EventID          string    `json:"eventId"`
			EventType        string    `json:"eventType"`
			AggregateType    string    `json:"aggregateType"`
			AggregateID      string    `json:"aggregateId"`
			AggregateVersion int64     `json:"aggregateVersion"`
			OccurredAt       time.Time `json:"occurredAt"`
		}
		if err := json.Unmarshal(raw, &e); err != nil {
			return nil, err
		}
		at := e.OccurredAt.UTC().Truncate(time.Microsecond)
		if at.Before(start) || !at.Before(end) {
			continue
		}
		family, ok := analyticsadmin.FactFamily(e.EventType, "")
		if !ok {
			continue
		}
		out = append(out, analyticsadmin.ProducerFact{Family: family, AnchorKind: e.AggregateType, AnchorRef: e.AggregateID, Member: analyticsadmin.ManifestMember{EventID: e.EventID, EventName: e.EventType, EntityType: e.AggregateType, EntityID: e.AggregateID, Version: e.AggregateVersion, OccurredAt: at}})
	}
	if count > maxFacts {
		return nil, errors.New("analytics producer window too large")
	}
	return out, rows.Err()
}

// Runtime is nil unless VENDOR_ANALYTICS_PRODUCER_ENABLED is "true".
func Runtime(ctx context.Context, db *pgxpool.Pool) (*analyticsadmin.ProducerRuntime, error) {
	return analyticsadmin.ProducerFromEnvironment(ctx, analyticsadmin.ProducerOptions{Prefix: Prefix, Owner: Owner, DB: db, Source: Facts{DB: db}, Global: true})
}
