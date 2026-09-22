// Package networkobserver publishes Haulage's supply and demand to analytics
// (Section 13).
//
// It observes the same online set the matcher dispatches from and the
// bookings Haulage has committed, and reduces both to counts per zone. It
// never matches, reserves or locks a provider. Provider ids, truck ids and
// positions stay in this process; only counts reach the analytics outbox, once
// per bucket.
package networkobserver

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	availabilityrepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_availability/repositories"
	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/networkanalytics"
)

// PolicyVersion names what "eligible" means here: online on an active truck.
const PolicyVersion = "hauling_active_truck_v1"

// activeBookingStatuses are the statuses in which a provider holds a job.
var activeBookingStatuses = []string{"awaiting_acceptance", "accepted", "en_route_pickup", "arrived_at_pickup",
	"picked_up", "en_route_delivery", "delivered"}

// OnlineSet is the matcher's own online set.
type OnlineSet interface {
	GetOnlineProviders(ctx context.Context) ([]availabilityrepositories.ProviderStatus, error)
}

// entityNamespace makes a stable UUID for each fact entity, because
// hauling_domain_outbox keys aggregates by UUID.
var entityNamespace = uuid.MustParse("6f1c2a52-8f3e-4c1b-9a8e-2f1d7c9e4b10")

// Observer publishes supply and demand.
type Observer struct {
	DB     *pgxpool.Pool
	Online OnlineSet
	Now    func() time.Time

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
			logging.Error("hauling-network", "observe: %v", err)
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

func (o *Observer) supply(ctx context.Context, now time.Time) ([]networkanalytics.SupplyFact, error) {
	if o.Online == nil {
		return nil, nil
	}
	online, err := o.Online.GetOnlineProviders(ctx)
	if err != nil {
		return nil, err
	}
	if len(online) == 0 {
		return nil, nil
	}
	providers := make([]string, 0, len(online))
	trucks := make([]string, 0, len(online))
	for _, status := range online {
		providers = append(providers, status.ProviderID)
		if status.TruckID != "" {
			trucks = append(trucks, status.TruckID)
		}
	}
	busy := map[string]bool{}
	rows, err := o.DB.Query(ctx, `SELECT DISTINCT provider_id::text FROM haulage_bookings
		WHERE provider_id::text = ANY($1) AND status = ANY($2)`, providers, activeBookingStatuses)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		busy[id] = true
	}
	rows.Close()
	active := map[string]bool{}
	rows, err = o.DB.Query(ctx, `SELECT id::text FROM trucks WHERE id::text = ANY($1) AND status = 'active'`, trucks)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		active[id] = true
	}
	rows.Close()

	observations := make([]networkanalytics.SupplyObservation, 0, len(online))
	for _, status := range online {
		// A provider online without a truck on record has unknown
		// eligibility, not ineligibility: the heartbeat simply did not say.
		var eligible *bool
		if status.TruckID != "" {
			eligible = networkanalytics.Bool(active[status.TruckID])
		}
		observations = append(observations, networkanalytics.SupplyObservation{
			Zone: networkanalytics.ZoneFor(status.Lat, status.Lng), ProductCode: networkanalytics.ProductHaulage,
			Online: true, Occupied: networkanalytics.Bool(busy[status.ProviderID]), Eligible: eligible,
		})
	}
	return networkanalytics.BuildSupplyFacts(networkanalytics.AuthorityHauling, "", observations, now, PolicyVersion), nil
}

func (o *Observer) demand(ctx context.Context, now time.Time) ([]networkanalytics.DemandBucket, error) {
	// No upper bound: the database clock and this process's clock are not
	// the same clock, and a booking committed a moment "after" now is still
	// in the open bucket.
	rows, err := o.DB.Query(ctx, `SELECT pickup_lat, pickup_lng, status, created_at FROM haulage_bookings
		WHERE created_at >= $1`, networkanalytics.DemandWindowStart(now))
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
			Zone: networkanalytics.ZoneFor(lat, lng), ProductCode: networkanalytics.ProductHaulage,
			CommittedAt: created, Unmatched: status == "unmatched",
		})
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return networkanalytics.BuildDemandBuckets(networkanalytics.AuthorityHauling, observations, now), nil
}

// publish writes the facts to the analytics outbox, where networkevents
// delivers them. The outbox's (aggregate, version, event) uniqueness makes a
// second replica's identical observation a no-op.
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
	insert := func(event, aggregateType, entity string, version int64, observedAt time.Time, payload any) error {
		raw, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			return marshalErr
		}
		_, execErr := tx.Exec(ctx, `INSERT INTO hauling_domain_outbox
			(aggregate_type, aggregate_id, event_type, event_version, payload, occurred_at, target)
			VALUES ($1, $2, $3, $4, $5, $6, 'analytics-service')
			ON CONFLICT (aggregate_type, aggregate_id, event_version, event_type) DO NOTHING`,
			aggregateType, uuid.NewSHA1(entityNamespace, []byte(entity)), event, version, raw, observedAt)
		return execErr
	}
	for _, fact := range facts {
		if err = insert(networkanalytics.EventSupplySnapshot, "network_supply", fact.EntityID(), fact.Version(),
			fact.ObservedAt, fact); err != nil {
			return err
		}
	}
	for _, bucket := range buckets {
		if err = insert(networkanalytics.EventDemandBucket, "network_demand", bucket.EntityID(), bucket.Version(),
			bucket.ObservedAt, bucket); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
