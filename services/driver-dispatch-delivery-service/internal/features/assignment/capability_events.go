package assignment

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/logging"
)

// EventCapabilityMismatch is the Section 8 fact that a mission could not be
// matched because no nearby rider's bike could physically carry it.
//
// It joins Dispatch's EXISTING versioned event vocabulary rather than
// introducing a "network.*" family. dispatch.delivery_recovery.selected.v1,
// dispatch.provider_replacement.started.v1 and their siblings are already live
// with real consumers; a second, differently-shaped namespace for facts of the
// same kind would leave every consumer having to know which dialect a given
// event speaks.
//
// Recovery facts themselves are deliberately NOT re-published here. Dispatch
// already emits them from internal/features/deliveryrecovery, and duplicating
// them would double-count every replacement in any consumer that sums them.
const EventCapabilityMismatch = "dispatch.capability_mismatch.v1"

// mismatchSchemaVersion is the payload schema this event carries.
const mismatchSchemaVersion = 1

// CapabilityEventSink publishes capability facts through the marketplace
// outbox Dispatch already runs.
//
// Reusing that outbox rather than adding a table means the lease, the backoff,
// the dead-letter path and the operator repair tooling all already exist and
// are proven. A second outbox would be a second thing to monitor and a second
// place for a delivery bug to hide.
type CapabilityEventSink struct {
	db      *pgxpool.Pool
	enabled bool
	now     func() time.Time
}

// NewCapabilityEventSink builds the sink. A disabled or unwired sink is a
// silent no-op: an assignment must never fail because an analytics consumer
// was unavailable.
func NewCapabilityEventSink(db *pgxpool.Pool, enabled bool, now func() time.Time) *CapabilityEventSink {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &CapabilityEventSink{db: db, enabled: enabled, now: now}
}

// PublishMismatch records that hard parcel fit emptied a candidate set.
//
// It fires once per mission, not once per rejected candidate. A per-candidate
// event would be both noisy -- fifty events for one unmatched mission -- and
// misleading: the actionable fact is "this load found nobody", not "this
// particular rider was too small", which the match-decision table already
// records in far more detail for an operator who wants it.
func (s *CapabilityEventSink) PublishMismatch(ctx context.Context, missionID string, considered int, reasons string) {
	if s == nil || !s.enabled || s.db == nil || missionID == "" {
		return
	}
	payload, err := json.Marshal(map[string]any{
		"event_id":           uuid.NewString(),
		"schema_version":     mismatchSchemaVersion,
		"source_service":     "driver-dispatch-delivery-service",
		"source_entity_type": "delivery_mission",
		"source_entity_id":   missionID,
		"source_version":     1,
		"occurred_at":        s.now().UTC().Format(time.RFC3339Nano),
		// Stable reason codes from the shared Appendix B catalogue, so a
		// consumer can act on WHY rather than parsing prose.
		"reason_codes":          reasons,
		"candidates_considered": considered,
		"candidates_eligible":   0,
	})
	if err != nil {
		return
	}

	// A detached context: the caller's context is cancelled the moment
	// matching gives up, and losing the explanation for the mission that just
	// failed is exactly the case an operator most wants to read.
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	if _, err := s.db.Exec(writeCtx, `
INSERT INTO dispatch_marketplace_outbox
	(event_type, schema_version, aggregate_type, aggregate_id, aggregate_version,
	 payload, payload_hash, target_service)
VALUES ($1, $2, 'delivery_mission', $3, 1, $4::jsonb,
	encode(digest(($4::jsonb)::text,'sha256'),'hex'), 'analytics-service')
ON CONFLICT DO NOTHING`,
		EventCapabilityMismatch, mismatchSchemaVersion, missionID, payload); err != nil {
		logging.Error("dispatch-capability", "publish capability mismatch for %s: %v", missionID, err)
	}
}
