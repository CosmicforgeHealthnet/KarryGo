// Package analyticsproducer is Haulage's owner-side Analytics coverage
// producer (Admin Section 6). It attests, with Haulage's own signing key, the
// complete set of individual facts Haulage published to Analytics per window
// and scope, and records an individually verifiable supply roster from the
// same availability records the matcher dispatches from. Scope comes only
// from Haulage's own signed resource authorization contexts that carry
// analyticsadmin.CoverageExportCapability. Everything is default off.
package analyticsproducer

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/hauling-service/internal/features/adminresources"
	availabilityrepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_availability/repositories"
	"cosmicforge/logistics/shared/go/analyticsadmin"
	sharedevents "cosmicforge/logistics/shared/go/events"
	"cosmicforge/logistics/shared/go/networkanalytics"
)

const (
	Owner         = "driver-hauling-service"
	Prefix        = "HAULING"
	PolicyVersion = "hauling_active_truck_v1"
	// maxFacts bounds one producer read; the lookback is far smaller in practice.
	maxFacts = 200000
)

// anchors maps an analytics outbox aggregate to the Haulage admin resource
// whose signed authorization context assigns its scope.
var anchors = map[string]string{
	"haulage_booking":                       "missions",
	"recovery_case":                         "recoveries",
	networkanalytics.SupplyRosterEntityType: "providers",
}

// AnchorKinds are the resource kinds that can carry the export capability.
func AnchorKinds() []string { return []string{"missions", "recoveries", "providers"} }

// Facts reads hauling_domain_outbox rows addressed to Analytics, rebuilding
// each fact's identity exactly as networkevents delivers it.
type Facts struct{ DB *pgxpool.Pool }

func (f Facts) Facts(ctx context.Context, start, end time.Time) ([]analyticsadmin.ProducerFact, error) {
	rows, err := f.DB.Query(ctx, `SELECT id::text,aggregate_type,aggregate_id::text,event_type,event_version,payload,occurred_at FROM hauling_domain_outbox
 WHERE target='analytics-service' AND occurred_at>=$1 AND occurred_at<$2 ORDER BY occurred_at LIMIT $3`, start, end, maxFacts+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []analyticsadmin.ProducerFact{}
	for rows.Next() {
		var id, aggregateType, aggregateID, name string
		var version int64
		var payload []byte
		var at time.Time
		if err := rows.Scan(&id, &aggregateType, &aggregateID, &name, &version, &payload, &at); err != nil {
			return nil, err
		}
		var stamped struct {
			EventID   string `json:"event_id"`
			Authority string `json:"authority"`
		}
		_ = json.Unmarshal(payload, &stamped)
		family, ok := analyticsadmin.FactFamily(name, stamped.Authority)
		kind := anchors[aggregateType]
		if !ok || kind == "" {
			continue
		}
		eventID := stamped.EventID
		if eventID == "" {
			eventID = id
		}
		if version < 1 {
			version = 1
		}
		out = append(out, analyticsadmin.ProducerFact{Family: family, AnchorKind: kind, AnchorRef: aggregateID, Member: analyticsadmin.ManifestMember{EventID: eventID, EventName: name, EntityType: aggregateType, EntityID: aggregateID, Version: version, OccurredAt: at.UTC().Truncate(time.Microsecond)}})
	}
	if len(out) > maxFacts {
		return nil, errors.New("analytics producer window too large")
	}
	return out, rows.Err()
}

// OnlineSet is the matcher's own online set.
type OnlineSet interface {
	GetOnlineProviders(ctx context.Context) ([]availabilityrepositories.ProviderStatus, error)
}

// activeBookingStatuses are the statuses in which a provider holds a job
// (the same definition the existing supply observer uses).
var activeBookingStatuses = []string{"awaiting_acceptance", "accepted", "en_route_pickup", "arrived_at_pickup", "picked_up", "en_route_delivery", "delivered"}

// Roster observes each online provider individually. Eligibility is "online
// on an active truck"; a provider online without a truck on record has
// UNKNOWN eligibility, not false.
type Roster struct {
	DB     *pgxpool.Pool
	Online OnlineSet
}

func (r Roster) Observe(ctx context.Context, bucket time.Time) ([]networkanalytics.SupplyRosterMember, error) {
	if r.Online == nil {
		return nil, errors.New("hauling online set unavailable")
	}
	online, err := r.Online.GetOnlineProviders(ctx)
	if err != nil {
		return nil, err
	}
	providers, trucks := []string{}, []string{}
	for _, s := range online {
		providers = append(providers, s.ProviderID)
		if s.TruckID != "" {
			trucks = append(trucks, s.TruckID)
		}
	}
	busy, err := idSet(ctx, r.DB, `SELECT DISTINCT provider_id::text FROM haulage_bookings WHERE provider_id::text=ANY($1) AND status=ANY($2)`, providers, activeBookingStatuses)
	if err != nil {
		return nil, err
	}
	active, err := idSet(ctx, r.DB, `SELECT id::text FROM trucks WHERE id::text=ANY($1) AND status='active'`, trucks, nil)
	if err != nil {
		return nil, err
	}
	out := []networkanalytics.SupplyRosterMember{}
	seen := map[string]bool{}
	for _, s := range online {
		id, err := uuid.Parse(s.ProviderID)
		if err != nil || id == uuid.Nil {
			return nil, errors.New("non-canonical provider in online set")
		}
		if seen[id.String()] {
			continue
		}
		seen[id.String()] = true
		var eligible *bool
		if s.TruckID != "" {
			eligible = networkanalytics.Bool(active[s.TruckID])
		}
		out = append(out, networkanalytics.SupplyRosterMember{Authority: networkanalytics.AuthorityHauling, ProductCode: networkanalytics.ProductHaulage, ProviderID: id.String(), Eligible: eligible, Occupied: networkanalytics.Bool(busy[s.ProviderID]), BucketStart: bucket, PolicyVersion: PolicyVersion})
	}
	return out, nil
}

func idSet(ctx context.Context, db *pgxpool.Pool, query string, ids []string, extra []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(ids) == 0 {
		return out, nil
	}
	args := []any{ids}
	if extra != nil {
		args = append(args, extra)
	}
	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// Outbox enqueues roster facts on the existing analytics-addressed outbox,
// stamped with the deterministic roster event identity and bucket time.
type Outbox struct{}

func (Outbox) Enqueue(ctx context.Context, tx pgx.Tx, e sharedevents.Envelope) error {
	var payload map[string]any
	if err := json.Unmarshal(e.Payload, &payload); err != nil {
		return err
	}
	payload["event_id"] = e.EventID
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO hauling_domain_outbox(aggregate_type,aggregate_id,event_type,event_version,payload,occurred_at,target)
 VALUES($1,$2::uuid,$3,$4,$5,$6,'analytics-service') ON CONFLICT (aggregate_type,aggregate_id,event_version,event_type) DO NOTHING`, e.SourceEntityType, e.SourceEntityID, e.EventName, e.SourceVersion, raw, e.OccurredAt)
	return err
}

// Runtime is nil when HAULING_ANALYTICS_PRODUCER_ENABLED is not "true".
func Runtime(ctx context.Context, db *pgxpool.Pool, online OnlineSet) (*analyticsadmin.ProducerRuntime, error) {
	return analyticsadmin.ProducerFromEnvironment(ctx, analyticsadmin.ProducerOptions{Prefix: Prefix, Owner: Owner, DB: db, Source: Facts{DB: db}, Binding: adminresources.Source(), AnchorKinds: AnchorKinds(), Roster: Roster{DB: db, Online: online}, RosterOutbox: Outbox{}, PolicyVersion: PolicyVersion})
}
