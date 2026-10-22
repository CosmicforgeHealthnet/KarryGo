// Package analyticsproducer is Dispatch's owner-side Analytics coverage
// producer (Admin Section 6): owner-signed manifests of exactly which
// individual facts Dispatch delivered to Analytics per window and scope, and
// an individually verifiable rider supply roster recorded from the same
// provider_availability rows the online gate and matcher use. Scope comes only
// from Dispatch's own signed resource contexts carrying
// analyticsadmin.CoverageExportCapability. Default off.
package analyticsproducer

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/adminresources"
	"cosmicforge/logistics/shared/go/analyticsadmin"
	sharedevents "cosmicforge/logistics/shared/go/events"
	"cosmicforge/logistics/shared/go/networkanalytics"
)

const (
	Owner         = "driver-dispatch-delivery-service"
	Prefix        = "DISPATCH"
	PolicyVersion = "dispatch_online_gate_v1"
	// analyticsCopyPrefix mirrors routeintegrity: analytics copies carry the
	// original aggregate type behind this prefix; delivery strips it.
	analyticsCopyPrefix = "analytics:"
	maxFacts            = 200000
)

// anchors maps the delivered aggregate type to the Dispatch admin resource
// whose signed context assigns scope. Customer bookings and commerce missions
// have no enrolled Dispatch admin resource kind; they belong to no scope.
var anchors = map[string]string{
	"trip":                                  "missions",
	"delivery_recovery_action":              "delivery-recoveries",
	networkanalytics.SupplyRosterEntityType: "riders",
}

func AnchorKinds() []string { return []string{"missions", "delivery-recoveries", "riders"} }

// Facts rebuilds each fact's identity exactly as the analytics outbox worker
// delivers it (row id, stripped aggregate type, aggregate version, created_at).
type Facts struct{ DB *pgxpool.Pool }

func (f Facts) Facts(ctx context.Context, start, end time.Time) ([]analyticsadmin.ProducerFact, error) {
	rows, err := f.DB.Query(ctx, `SELECT id::text,event_type,aggregate_type,aggregate_id,aggregate_version,payload,created_at FROM dispatch_marketplace_outbox
 WHERE target_service='analytics-service' AND created_at>=$1 AND created_at<$2 ORDER BY created_at LIMIT $3`, start, end, maxFacts+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []analyticsadmin.ProducerFact{}
	for rows.Next() {
		var id, name, aggregateType, aggregateID string
		var version int64
		var payload []byte
		var at time.Time
		if err := rows.Scan(&id, &name, &aggregateType, &aggregateID, &version, &payload, &at); err != nil {
			return nil, err
		}
		var hint struct {
			Authority string `json:"authority"`
		}
		_ = json.Unmarshal(payload, &hint)
		family, ok := analyticsadmin.FactFamily(name, hint.Authority)
		entityType := strings.TrimPrefix(aggregateType, analyticsCopyPrefix)
		kind := anchors[entityType]
		if !ok || kind == "" {
			continue
		}
		out = append(out, analyticsadmin.ProducerFact{Family: family, AnchorKind: kind, AnchorRef: aggregateID, Member: analyticsadmin.ManifestMember{EventID: id, EventName: name, EntityType: entityType, EntityID: aggregateID, Version: version, OccurredAt: at.UTC().Truncate(time.Microsecond)}})
	}
	if len(out) > maxFacts {
		return nil, errors.New("analytics producer window too large")
	}
	return out, rows.Err()
}

// Roster records each online or busy rider individually: eligible is the
// owner's verified_to_go_online gate, occupied is status busy (the same
// interpretation as the existing supply observer).
type Roster struct{ DB *pgxpool.Pool }

func (r Roster) Observe(ctx context.Context, bucket time.Time) ([]networkanalytics.SupplyRosterMember, error) {
	rows, err := r.DB.Query(ctx, `SELECT provider_id::text,status,verified_to_go_online FROM provider_availability WHERE status IN ('online','busy') ORDER BY provider_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []networkanalytics.SupplyRosterMember{}
	for rows.Next() {
		var id, status string
		var verified bool
		if err := rows.Scan(&id, &status, &verified); err != nil {
			return nil, err
		}
		parsed, err := uuid.Parse(id)
		if err != nil || parsed == uuid.Nil {
			return nil, errors.New("non-canonical rider in availability")
		}
		out = append(out, networkanalytics.SupplyRosterMember{Authority: networkanalytics.AuthorityDispatch, ProductCode: networkanalytics.ProductDispatchParcel, ProviderID: parsed.String(), Eligible: networkanalytics.Bool(verified), Occupied: networkanalytics.Bool(status == "busy"), BucketStart: bucket, PolicyVersion: PolicyVersion})
	}
	return out, rows.Err()
}

// Outbox writes roster facts addressed to Analytics with the deterministic
// roster event id as the row id (the delivered event id).
type Outbox struct{}

func (Outbox) Enqueue(ctx context.Context, tx pgx.Tx, e sharedevents.Envelope) error {
	_, err := tx.Exec(ctx, `INSERT INTO dispatch_marketplace_outbox(id,event_type,schema_version,aggregate_type,aggregate_id,aggregate_version,payload,payload_hash,target_service,created_at)
 VALUES($1::uuid,$2,1,$3,$4,$5,$6::jsonb,encode(digest(($6::jsonb)::text,'sha256'),'hex'),'analytics-service',$7) ON CONFLICT DO NOTHING`, e.EventID, e.EventName, e.SourceEntityType, e.SourceEntityID, e.SourceVersion, string(e.Payload), e.OccurredAt)
	return err
}

// Runtime is nil unless DISPATCH_ANALYTICS_PRODUCER_ENABLED is "true".
func Runtime(ctx context.Context, db *pgxpool.Pool) (*analyticsadmin.ProducerRuntime, error) {
	return analyticsadmin.ProducerFromEnvironment(ctx, analyticsadmin.ProducerOptions{Prefix: Prefix, Owner: Owner, DB: db, Source: Facts{DB: db}, Binding: adminresources.Source(), AnchorKinds: AnchorKinds(), Roster: Roster{DB: db}, RosterOutbox: Outbox{}, PolicyVersion: PolicyVersion})
}
