package repositories

// Section 13 closure: supply by product, demand buckets, and mission outcomes
// by product.

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/analytics-service/internal/features/networkanalytics/models"
	"cosmicforge/logistics/shared/go/networkanalytics"
)

// outcomeColumns maps each lifecycle kind to its mission_outcomes_daily column.
var outcomeColumns = map[networkanalytics.Kind]string{
	networkanalytics.KindMissionCommitted:  "committed",
	networkanalytics.KindMissionAssigned:   "assigned",
	networkanalytics.KindMissionCompleted:  "completed",
	networkanalytics.KindMissionFailed:     "failed",
	networkanalytics.KindMissionCancelled:  "cancelled",
	networkanalytics.KindRecoveryResolved:  "recovered",
	networkanalytics.KindRecoveryFailed:    "recovery_failed",
	networkanalytics.KindGuaranteeBreached: "guarantee_breaches",
}

// projectOutcome counts a lifecycle fact against its authority AND product,
// so Taxi passenger and Taxi Goods -- both TAXI -- stay apart.
func projectOutcome(ctx context.Context, tx pgx.Tx, e models.Event, day time.Time) error {
	column, ok := outcomeColumns[e.Kind]
	if !ok {
		return nil
	}
	product := networkanalytics.ProductFor(e.EventName, e.SourceEntityType, e.Payload)
	_, err := tx.Exec(ctx, `
		INSERT INTO mission_outcomes_daily (authority, product_code, metric_date, `+column+`) VALUES ($1,$2,$3,1)
		ON CONFLICT (authority, product_code, metric_date) DO UPDATE SET
			`+column+` = mission_outcomes_daily.`+column+` + 1, updated_at = now()`,
		e.Authority, product, day)
	return err
}

// projectProductSupply stores a supply fact, replacing the zone's previous one
// only when strictly newer. Counts the authority could not determine stay
// NULL.
func projectProductSupply(ctx context.Context, tx pgx.Tx, e models.Event) error {
	p := e.Payload
	zone := str(p, "zone")
	if zone == "" {
		return nil
	}
	state := str(p, "state")
	switch state {
	case networkanalytics.SupplyAvailable, networkanalytics.SupplyLimited, networkanalytics.SupplyUnknown:
	default:
		state = networkanalytics.SupplyUnknown
	}
	bucket := parseTime(p, "bucket_start", e.OccurredAt)
	online := nonNegative(num(p, "online_count"))
	eligible, occupied := nullableCount(p, "eligible_count"), nullableCount(p, "occupied_count")
	// An inconsistent count is not a count. Refusing the row would make the
	// producer retry it forever; recording it as UNKNOWN is the truth.
	if eligible != nil && *eligible > online {
		eligible, state = nil, networkanalytics.SupplyUnknown
	}
	if occupied != nil && *occupied > online {
		occupied, state = nil, networkanalytics.SupplyUnknown
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO supply_product_snapshots (authority, market_id, zone, product_code, online_count,
			eligible_count, occupied_count, supply_state, policy_version, observed_at, bucket_start, source_version)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (authority, market_id, zone, product_code) DO UPDATE SET
			online_count = EXCLUDED.online_count, eligible_count = EXCLUDED.eligible_count,
			occupied_count = EXCLUDED.occupied_count, supply_state = EXCLUDED.supply_state,
			policy_version = EXCLUDED.policy_version, observed_at = EXCLUDED.observed_at,
			bucket_start = EXCLUDED.bucket_start, source_version = EXCLUDED.source_version, updated_at = now()
		WHERE supply_product_snapshots.source_version < EXCLUDED.source_version`,
		e.Authority, str(p, "market_id"), zone, upperOr(str(p, "product_code"), "UNSPECIFIED"),
		online, eligible, occupied, state, str(p, "policy_version"), e.OccurredAt.UTC(), bucket, e.SourceVersion)
	return err
}

// projectDemandBucket stores a demand bucket, replacing it only with a
// strictly newer observation of the same bucket.
func projectDemandBucket(ctx context.Context, tx pgx.Tx, e models.Event) error {
	p := e.Payload
	zone := str(p, "zone")
	bucket := parseTime(p, "bucket_start", time.Time{})
	if zone == "" || bucket.IsZero() {
		return nil
	}
	minutes := num(p, "bucket_minutes")
	if minutes <= 0 {
		minutes = int64(networkanalytics.DemandBucketSize / time.Minute)
	}
	demand := nonNegative(num(p, "demand_count"))
	unmatched := nonNegative(num(p, "unmatched_count"))
	if unmatched > demand {
		unmatched = demand
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO demand_buckets (authority, market_id, zone, product_code, bucket_start, bucket_minutes,
			demand_count, unmatched_count, observed_at, source_version)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (authority, market_id, zone, product_code, bucket_start) DO UPDATE SET
			demand_count = EXCLUDED.demand_count, unmatched_count = EXCLUDED.unmatched_count,
			observed_at = EXCLUDED.observed_at, source_version = EXCLUDED.source_version, updated_at = now()
		WHERE demand_buckets.source_version < EXCLUDED.source_version`,
		e.Authority, str(p, "market_id"), zone, upperOr(str(p, "product_code"), "UNSPECIFIED"),
		bucket.UTC(), minutes, demand, unmatched, e.OccurredAt.UTC(), e.SourceVersion)
	return err
}

// nullableCount reads a count that may be JSON null. Absent or null is
// UNKNOWN and stays NULL; a negative is refused as unknown too.
func nullableCount(p map[string]any, key string) *int64 {
	value, present := p[key]
	if !present || value == nil {
		return nil
	}
	var parsed int64
	switch typed := value.(type) {
	case float64:
		parsed = int64(typed)
	case json.Number:
		n, err := typed.Int64()
		if err != nil {
			return nil
		}
		parsed = n
	case string:
		n, err := strconv.ParseInt(typed, 10, 64)
		if err != nil {
			return nil
		}
		parsed = n
	default:
		return nil
	}
	if parsed < 0 {
		return nil
	}
	return &parsed
}

func parseTime(p map[string]any, key string, fallback time.Time) time.Time {
	if raw, ok := p[key].(string); ok {
		if parsed, err := time.Parse(time.RFC3339Nano, raw); err == nil {
			return parsed.UTC()
		}
	}
	return fallback
}

func upperOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	out := []rune(value)
	for i, r := range out {
		if r >= 'a' && r <= 'z' {
			out[i] = r - 32
		}
	}
	return string(out)
}
