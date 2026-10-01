// Package networkobserver publishes Taxi's supply and demand to analytics
// (Section 13).
//
// Supply is observed from the online set the matcher dispatches from and the
// operational state it already maintains; demand from the bookings and goods
// missions Taxi has committed, in Taxi's own markets and operating zones. It
// never matches or reserves a driver. Driver ids, vehicle ids and positions
// stay in this process; only counts reach the analytics outbox, once per
// bucket.
package networkobserver

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	availabilityrepositories "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/repositories"
	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/networkanalytics"
)

// PolicyVersion names what "eligible" means here: online and in a state the
// matcher offers work in.
const PolicyVersion = "taxi_operational_state_v1"

// occupiedStates hold a job; the driver is online but not free.
var occupiedStates = map[string]bool{"OFFERED": true, "RESERVED": true, "EN_ROUTE_PICKUP": true,
	"AT_PICKUP": true, "ON_TRIP": true}

// ineligibleStates are online but may not be offered work.
var ineligibleStates = map[string]bool{"PAUSED": true, "SAFETY_HOLD": true, "COMPLIANCE_HOLD": true}

// OnlineSet is the matcher's own online set.
type OnlineSet interface {
	GetOnlineProviders(ctx context.Context) ([]availabilityrepositories.ProviderStatus, error)
}

var entityNamespace = uuid.MustParse("0b3d6a5e-9c7f-4e21-8a4d-5c1e2f3a7b90")

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
			logging.Error("taxi-network", "observe: %v", err)
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

// ProductFor is the supply product for a vehicle class. Passenger and goods
// work are told apart on the demand side, where the booking says which it is.
func ProductFor(vehicleClass string) string {
	class := strings.ToUpper(strings.TrimSpace(vehicleClass))
	if class == "" {
		class = "UNSPECIFIED"
	}
	return "TAXI_" + class
}

func (o *Observer) supply(ctx context.Context, now time.Time) ([]networkanalytics.SupplyFact, error) {
	if o.Online == nil {
		return nil, nil
	}
	online, err := o.Online.GetOnlineProviders(ctx)
	if err != nil || len(online) == 0 {
		return nil, err
	}
	ids := make([]string, 0, len(online))
	for _, status := range online {
		ids = append(ids, status.ProviderID)
	}
	states := map[string]string{}
	rows, err := o.DB.Query(ctx, `SELECT provider_id::text, state FROM provider_operational_state
		WHERE provider_id::text = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, state string
		if err = rows.Scan(&id, &state); err != nil {
			rows.Close()
			return nil, err
		}
		states[id] = strings.ToUpper(state)
	}
	rows.Close()

	byMarket := map[string][]networkanalytics.SupplyObservation{}
	for _, status := range online {
		state, known := states[status.ProviderID]
		if known && state == "OFFLINE" {
			// The online set lags a go-offline by at most its TTL; the
			// operational state is the authority.
			continue
		}
		observation := networkanalytics.SupplyObservation{
			Zone: networkanalytics.ZoneFor(status.Lat, status.Lng), ProductCode: ProductFor(status.VehicleClass),
			Online: true,
		}
		// No operational-state row means Taxi has not recorded one: both
		// counts are unknown, not zero.
		if known {
			observation.Occupied = networkanalytics.Bool(occupiedStates[state])
			observation.Eligible = networkanalytics.Bool(!ineligibleStates[state])
		}
		byMarket[status.MarketID] = append(byMarket[status.MarketID], observation)
	}
	var facts []networkanalytics.SupplyFact
	for market, observations := range byMarket {
		facts = append(facts, networkanalytics.BuildSupplyFacts(networkanalytics.AuthorityTaxi, market,
			observations, now, PolicyVersion)...)
	}
	return facts, nil
}

// demand counts committed passenger bookings in Taxi's own operating zones,
// and committed goods missions in their pickup cell.
func (o *Observer) demand(ctx context.Context, now time.Time) ([]networkanalytics.DemandBucket, error) {
	since := networkanalytics.DemandWindowStart(now)
	var observations []networkanalytics.DemandObservation
	rows, err := o.DB.Query(ctx, `SELECT market_id::text, operating_zone_id::text, status, created_at
		FROM taxi_bookings WHERE created_at >= $1`, since)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var market, zone, status string
		var created time.Time
		if err = rows.Scan(&market, &zone, &status, &created); err != nil {
			rows.Close()
			return nil, err
		}
		observations = append(observations, networkanalytics.DemandObservation{MarketID: market,
			Zone: "tz:" + zone, ProductCode: networkanalytics.ProductTaxiPassenger, CommittedAt: created,
			Unmatched: status == "unmatched"})
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	rows, err = o.DB.Query(ctx, `SELECT COALESCE(q.market_id::text, ''),
		COALESCE((m.pickup->>'latitude')::float8, 0), COALESCE((m.pickup->>'longitude')::float8, 0),
		m.status, m.created_at
		FROM taxi_goods_missions m LEFT JOIN taxi_goods_quotes q ON q.id = m.quote_id
		WHERE m.created_at >= $1`, since)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var market, status string
		var lat, lng float64
		var created time.Time
		if err = rows.Scan(&market, &lat, &lng, &status, &created); err != nil {
			rows.Close()
			return nil, err
		}
		observations = append(observations, networkanalytics.DemandObservation{MarketID: market,
			Zone: networkanalytics.ZoneFor(lat, lng), ProductCode: networkanalytics.ProductTaxiGoods,
			CommittedAt: created, Unmatched: status == "UNMATCHED"})
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return networkanalytics.BuildDemandBuckets(networkanalytics.AuthorityTaxi, observations, now), nil
}

// publish writes the facts to taxi_outbox for the analytics destination.
// occurred_at is the observation time, which that destination uses as the
// version; the idempotency key collapses a second replica's identical fact.
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
	insert := func(event, aggregateType, entity, key string, occurred time.Time, payload any) error {
		raw, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			return marshalErr
		}
		_, execErr := tx.Exec(ctx, `INSERT INTO taxi_outbox
			(aggregate_type, aggregate_id, event_type, payload, occurred_at, target, idempotency_key)
			VALUES ($1, $2, $3, $4, $5, 'analytics-service', $6)
			ON CONFLICT DO NOTHING`,
			aggregateType, uuid.NewSHA1(entityNamespace, []byte(entity)), event, raw, occurred, key)
		return execErr
	}
	for _, fact := range facts {
		// A supply fact's version is its bucket, so its occurred_at is the
		// bucket start: every replica writes the same row.
		if err = insert(networkanalytics.EventSupplySnapshot, "network_supply", fact.EntityID(), fact.EventID(),
			fact.BucketStart, fact); err != nil {
			return err
		}
	}
	for _, bucket := range buckets {
		if err = insert(networkanalytics.EventDemandBucket, "network_demand", bucket.EntityID(), bucket.EventID(),
			bucket.ObservedAt, bucket); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
