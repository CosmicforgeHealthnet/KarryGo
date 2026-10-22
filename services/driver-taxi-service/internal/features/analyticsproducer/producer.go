// Package analyticsproducer is Taxi's owner-side Analytics coverage producer
// (Admin Section 6): owner-signed manifests of exactly which individual facts
// Taxi delivered to Analytics per window and scope, and an individually
// verifiable supply roster recorded from the matcher's own online set and
// operational state. Scope comes only from Taxi's own signed resource
// contexts carrying analyticsadmin.CoverageExportCapability. Default off.
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

	"cosmicforge/logistics/services/taxi-service/internal/features/adminresources"
	availabilityrepositories "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/repositories"
	"cosmicforge/logistics/services/taxi-service/internal/outbox"
	"cosmicforge/logistics/shared/go/analyticsadmin"
	sharedevents "cosmicforge/logistics/shared/go/events"
	"cosmicforge/logistics/shared/go/networkanalytics"
)

const (
	Owner         = "driver-taxi-service"
	Prefix        = "TAXI"
	PolicyVersion = "taxi_operational_state_v1"
	maxFacts      = 200000
)

// anchors maps an analytics outbox aggregate to the Taxi admin resource whose
// signed context assigns scope. Goods missions have no enrolled Taxi admin
// resource kind, so their facts belong to no scope (never guessed).
var anchors = map[string]string{
	"booking":                               "independent-rides",
	networkanalytics.SupplyRosterEntityType: "providers",
}

func AnchorKinds() []string { return []string{"independent-rides", "providers"} }

// Facts rebuilds each delivered fact through outbox.AnalyticsEnvelope, the
// same function the delivery path uses, so identities match exactly.
type Facts struct{ DB *pgxpool.Pool }

func (f Facts) Facts(ctx context.Context, start, end time.Time) ([]analyticsadmin.ProducerFact, error) {
	rows, err := f.DB.Query(ctx, `SELECT id::text,aggregate_type,aggregate_id::text,event_type,payload,occurred_at FROM taxi_outbox WHERE target=$1 AND occurred_at>=$2 AND occurred_at<$3 ORDER BY occurred_at LIMIT $4`, outbox.TargetAnalytics, start, end, maxFacts+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []analyticsadmin.ProducerFact{}
	for rows.Next() {
		var record outbox.Record
		if err := rows.Scan(&record.ID, &record.AggregateType, &record.AggregateID, &record.EventType, &record.Payload, &record.OccurredAt); err != nil {
			return nil, err
		}
		envelope := outbox.AnalyticsEnvelope(record)
		var hint struct {
			Authority string `json:"authority"`
		}
		_ = json.Unmarshal(record.Payload, &hint)
		family, ok := analyticsadmin.FactFamily(envelope.EventName, hint.Authority)
		kind := anchors[envelope.SourceEntityType]
		if !ok || kind == "" {
			continue
		}
		out = append(out, analyticsadmin.ProducerFact{Family: family, AnchorKind: kind, AnchorRef: envelope.SourceEntityID, Member: analyticsadmin.ManifestMember{EventID: envelope.EventID, EventName: envelope.EventName, EntityType: envelope.SourceEntityType, EntityID: envelope.SourceEntityID, Version: envelope.SourceVersion, OccurredAt: envelope.OccurredAt.Truncate(time.Microsecond)}})
	}
	if len(out) > maxFacts {
		return nil, errors.New("analytics producer window too large")
	}
	return out, rows.Err()
}

type OnlineSet interface {
	GetOnlineProviders(ctx context.Context) ([]availabilityrepositories.ProviderStatus, error)
}

// The same operational-state interpretation the existing supply observer uses.
var occupiedStates = map[string]bool{"OFFERED": true, "RESERVED": true, "EN_ROUTE_PICKUP": true, "AT_PICKUP": true, "ON_TRIP": true}
var ineligibleStates = map[string]bool{"PAUSED": true, "SAFETY_HOLD": true, "COMPLIANCE_HOLD": true}

// Roster records each online passenger provider individually. A provider with
// no recorded operational state has UNKNOWN eligibility and occupancy.
type Roster struct {
	DB     *pgxpool.Pool
	Online OnlineSet
}

func (r Roster) Observe(ctx context.Context, bucket time.Time) ([]networkanalytics.SupplyRosterMember, error) {
	if r.Online == nil {
		return nil, errors.New("taxi online set unavailable")
	}
	online, err := r.Online.GetOnlineProviders(ctx)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for _, s := range online {
		ids = append(ids, s.ProviderID)
	}
	states := map[string]string{}
	if len(ids) > 0 {
		rows, err := r.DB.Query(ctx, `SELECT provider_id::text,state FROM provider_operational_state WHERE provider_id::text=ANY($1)`, ids)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id, state string
			if err := rows.Scan(&id, &state); err != nil {
				rows.Close()
				return nil, err
			}
			states[id] = strings.ToUpper(state)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	out := []networkanalytics.SupplyRosterMember{}
	seen := map[string]bool{}
	for _, s := range online {
		id, err := uuid.Parse(s.ProviderID)
		if err != nil || id == uuid.Nil {
			return nil, errors.New("non-canonical provider in online set")
		}
		state, known := states[s.ProviderID]
		if known && state == "OFFLINE" || seen[id.String()] {
			continue
		}
		seen[id.String()] = true
		m := networkanalytics.SupplyRosterMember{Authority: networkanalytics.AuthorityTaxi, ProductCode: networkanalytics.ProductTaxiPassenger, ProviderID: id.String(), BucketStart: bucket, PolicyVersion: PolicyVersion}
		if known {
			m.Occupied = networkanalytics.Bool(occupiedStates[state])
			m.Eligible = networkanalytics.Bool(!ineligibleStates[state])
		}
		out = append(out, m)
	}
	return out, nil
}

// Outbox writes roster facts addressed to Analytics with the deterministic
// roster event identity as the row id, so delivery reuses that identity.
type Outbox struct{}

func (Outbox) Enqueue(ctx context.Context, tx pgx.Tx, e sharedevents.Envelope) error {
	_, err := tx.Exec(ctx, `INSERT INTO taxi_outbox(id,aggregate_type,aggregate_id,event_type,payload,occurred_at,target,idempotency_key) VALUES($1::uuid,$2,$3::uuid,$4,$5,$6,$7,$8)
 ON CONFLICT DO NOTHING`, e.EventID, e.SourceEntityType, e.SourceEntityID, e.EventName, []byte(e.Payload), e.OccurredAt, outbox.TargetAnalytics, "roster:"+e.EventID)
	return err
}

// Runtime is nil unless TAXI_ANALYTICS_PRODUCER_ENABLED is "true".
func Runtime(ctx context.Context, db *pgxpool.Pool, online OnlineSet) (*analyticsadmin.ProducerRuntime, error) {
	return analyticsadmin.ProducerFromEnvironment(ctx, analyticsadmin.ProducerOptions{Prefix: Prefix, Owner: Owner, DB: db, Source: Facts{DB: db}, Binding: adminresources.Source(), AnchorKinds: AnchorKinds(), Roster: Roster{DB: db, Online: online}, RosterOutbox: Outbox{}, PolicyVersion: PolicyVersion})
}
