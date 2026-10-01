package repositories

import (
	"context"
	"time"

	"cosmicforge/logistics/services/analytics-service/internal/features/networkanalytics/models"
)

// ProductSupply aggregates a zone's supply facts, optionally for one product.
//
// If any part of the zone's eligibility or occupancy is unknown, the total is
// unknown: adding a known count to an unknown one produces a number that
// looks precise and is not.
func (r *Repository) ProductSupply(ctx context.Context, authority, zone, product string) (models.ProductSupply, error) {
	var supply models.ProductSupply
	var rows int64
	var observed *time.Time
	err := r.db.QueryRow(ctx, `
		SELECT count(*), COALESCE(SUM(online_count),0),
		       CASE WHEN bool_or(eligible_count IS NULL) THEN NULL ELSE SUM(eligible_count) END,
		       CASE WHEN bool_or(occupied_count IS NULL) THEN NULL ELSE SUM(occupied_count) END,
		       CASE WHEN bool_or(supply_state = 'UNKNOWN') THEN 'UNKNOWN'
		            WHEN bool_or(supply_state = 'AVAILABLE') THEN 'AVAILABLE' ELSE 'LIMITED' END,
		       MAX(observed_at)
		  FROM supply_product_snapshots
		 WHERE authority = $1 AND zone = $2 AND ($3 = '' OR product_code = $3)`,
		authority, zone, product).Scan(&rows, &supply.OnlineCount, &supply.EligibleCount,
		&supply.OccupiedCount, &supply.State, &observed)
	if err != nil {
		return supply, err
	}
	if rows == 0 || observed == nil {
		return supply, ErrNoData
	}
	supply.ObservedAt = observed.UTC()
	return supply, nil
}

// DemandSlots reads the named buckets of one zone and product.
func (r *Repository) DemandSlots(ctx context.Context, authority, zone, product string,
	slots []time.Time) ([]models.DemandSlot, error) {
	rows, err := r.db.Query(ctx, `
		SELECT bucket_start, SUM(demand_count), MAX(observed_at), MAX(source_version)
		  FROM demand_buckets
		 WHERE authority = $1 AND zone = $2 AND ($3 = '' OR product_code = $3) AND bucket_start = ANY($4)
		 GROUP BY bucket_start`, authority, zone, product, slots)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var found []models.DemandSlot
	for rows.Next() {
		var slot models.DemandSlot
		if err = rows.Scan(&slot.BucketStart, &slot.DemandCount, &slot.ObservedAt, &slot.SourceVersion); err != nil {
			return nil, err
		}
		slot.BucketStart, slot.ObservedAt = slot.BucketStart.UTC(), slot.ObservedAt.UTC()
		found = append(found, slot)
	}
	return found, rows.Err()
}

// MissionOutcomes returns daily outcomes by authority and product.
func (r *Repository) MissionOutcomes(ctx context.Context, authority, product string,
	window models.Window) ([]models.MissionOutcomeDay, error) {
	rows, err := r.db.Query(ctx, `
		SELECT authority, product_code, metric_date, committed, assigned, completed, failed, cancelled,
		       recovered, recovery_failed, guarantee_breaches
		  FROM mission_outcomes_daily
		 WHERE ($1 = '' OR authority = $1) AND ($2 = '' OR product_code = $2)
		   AND metric_date BETWEEN $3 AND $4
		 ORDER BY metric_date, authority, product_code`, authority, product, window.Start, window.End)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	days := []models.MissionOutcomeDay{}
	for rows.Next() {
		var day models.MissionOutcomeDay
		if err = rows.Scan(&day.Authority, &day.ProductCode, &day.Date, &day.Committed, &day.Assigned,
			&day.Completed, &day.Failed, &day.Cancelled, &day.Recovered, &day.RecoveryFailed,
			&day.GuaranteeBreaches); err != nil {
			return nil, err
		}
		day.SuccessRate = rate(day.Completed, day.Completed+day.Failed)
		days = append(days, day)
	}
	return days, rows.Err()
}
