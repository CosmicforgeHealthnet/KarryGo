// Package networkobserver publishes Dispatch's supply and demand to analytics
// (Section 13).
//
// It OBSERVES. It reads the availability rows and the live store the matcher
// already maintains, and the bookings Dispatch has already committed, and
// reduces them to counts per zone and product. It never runs assignment,
// never reserves a rider, and never changes a row it reads. The rider ids and
// positions it reads stay in this process; only the counts are written, to the
// existing analytics outbox, once per bucket.
package networkobserver

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/availability"
	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/networkanalytics"
)

// SourceService is Dispatch's publishing identity.
const SourceService = "driver-dispatch-delivery-service"

// PolicyVersion names what "eligible" means in this producer: the rider has
// passed Dispatch's own go-online gate.
const PolicyVersion = "dispatch_online_gate_v1"

// Locator reads a rider's live position from the matcher's own live store.
type Locator interface {
	GetLocation(ctx context.Context, providerID string) (availability.Location, bool, error)
}

// Observer publishes supply and demand.
type Observer struct {
	DB      *pgxpool.Pool
	Locator Locator
	Now     func() time.Time

	mu       sync.Mutex
	previous []networkanalytics.SupplyFact
}

func (o *Observer) now() time.Time {
	if o.Now != nil {
		return o.Now().UTC()
	}
	return time.Now().UTC()
}

// Run observes once per supply bucket until the context ends.
func (o *Observer) Run(ctx context.Context) {
	ticker := time.NewTicker(networkanalytics.SupplyBucket)
	defer ticker.Stop()
	for {
		if err := o.RunOnce(ctx); err != nil && ctx.Err() == nil {
			logging.Error("dispatch-network", "observe: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// RunOnce publishes one supply observation and the open demand window.
func (o *Observer) RunOnce(ctx context.Context) error {
	if o == nil || o.DB == nil {
		return nil
	}
	now := o.now()
	facts, err := o.supply(ctx, now)
	if err != nil {
		return err
	}
	o.mu.Lock()
	facts = networkanalytics.WithVanishedZones(o.previous, facts, now)
	o.mu.Unlock()
	buckets, err := o.demand(ctx, now)
	if err != nil {
		return err
	}
	if err = o.publish(ctx, facts, buckets); err != nil {
		return err
	}
	o.mu.Lock()
	o.previous = facts
	o.mu.Unlock()
	return nil
}

// supply reduces the riders Dispatch considers online to counts.
func (o *Observer) supply(ctx context.Context, now time.Time) ([]networkanalytics.SupplyFact, error) {
	rows, err := o.DB.Query(ctx, `
		SELECT provider_id::text, status, verified_to_go_online
		  FROM provider_availability WHERE status IN ('online','busy')`)
	if err != nil {
		return nil, err
	}
	type rider struct {
		id, status string
		verified   bool
	}
	var riders []rider
	for rows.Next() {
		var r rider
		if err = rows.Scan(&r.id, &r.status, &r.verified); err != nil {
			rows.Close()
			return nil, err
		}
		riders = append(riders, r)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	observations := make([]networkanalytics.SupplyObservation, 0, len(riders))
	for _, r := range riders {
		zone := networkanalytics.UnlocatedZone
		if o.Locator != nil {
			if location, ok, locateErr := o.Locator.GetLocation(ctx, r.id); locateErr == nil && ok {
				zone = networkanalytics.ZoneFor(location.Lat, location.Lng)
			}
		}
		observations = append(observations, networkanalytics.SupplyObservation{
			Zone: zone, ProductCode: networkanalytics.ProductDispatchParcel, Online: true,
			Occupied: networkanalytics.Bool(r.status == "busy"), Eligible: networkanalytics.Bool(r.verified),
		})
	}
	return networkanalytics.BuildSupplyFacts(networkanalytics.AuthorityDispatch, "", observations, now, PolicyVersion), nil
}

// demand counts the bookings Dispatch committed in the open and just-closed
// buckets. The pickup is reduced to its zone here; it is never published.
func (o *Observer) demand(ctx context.Context, now time.Time) ([]networkanalytics.DemandBucket, error) {
	rows, err := o.DB.Query(ctx, `
		SELECT pickup_lat::float8, pickup_lng::float8, status, created_at
		  		  FROM dispatch_bookings WHERE created_at >= $1`,
		// No upper bound: the database clock and this process's clock are
		// not the same clock.
		networkanalytics.DemandWindowStart(now))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var observations []networkanalytics.DemandObservation
	for rows.Next() {
		var lat, lng float64
		var status string
		var created time.Time
		if err = rows.Scan(&lat, &lng, &status, &created); err != nil {
			return nil, err
		}
		observations = append(observations, networkanalytics.DemandObservation{
			Zone: networkanalytics.ZoneFor(lat, lng), ProductCode: networkanalytics.ProductDispatchParcel,
			CommittedAt: created, Unmatched: status == "unmatched",
		})
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return networkanalytics.BuildDemandBuckets(networkanalytics.AuthorityDispatch, observations, now), nil
}

// publish writes the facts to the analytics outbox in one transaction. The
// outbox's (aggregate, version, event) uniqueness makes a second replica's
// identical observation a no-op.
func (o *Observer) publish(ctx context.Context, facts []networkanalytics.SupplyFact,
	buckets []networkanalytics.DemandBucket) error {
	if len(facts) == 0 && len(buckets) == 0 {
		return nil
	}
	tx, err := o.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	insert := func(event, aggregateType, aggregateID string, version int64, payload any) error {
		raw, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			return marshalErr
		}
		_, execErr := tx.Exec(ctx, `INSERT INTO dispatch_marketplace_outbox
			(event_type, schema_version, aggregate_type, aggregate_id, aggregate_version, payload, payload_hash, target_service)
			VALUES ($1, 1, $2, $3, $4, $5::jsonb, encode(digest(($5::jsonb)::text,'sha256'),'hex'), 'analytics-service')
			ON CONFLICT DO NOTHING`, event, aggregateType, aggregateID, version, raw)
		return execErr
	}
	for _, fact := range facts {
		if err = insert(networkanalytics.EventSupplySnapshot, "network_supply", fact.EntityID(), fact.Version(), fact); err != nil {
			return err
		}
	}
	for _, bucket := range buckets {
		if err = insert(networkanalytics.EventDemandBucket, "network_demand", bucket.EntityID(), bucket.Version(), bucket); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
