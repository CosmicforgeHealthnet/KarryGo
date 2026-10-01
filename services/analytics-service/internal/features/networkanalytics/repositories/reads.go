package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/analytics-service/internal/features/networkanalytics/models"
)

// ErrNoData means the read model holds nothing for the subject.
var ErrNoData = errors.New("no analytics data")

// ProviderTotals sums a provider's counts over a window.
func (r *Repository) ProviderTotals(ctx context.Context, authority, providerID string,
	window models.Window) (models.ProviderTotals, error) {
	var totals models.ProviderTotals
	var asOf *time.Time
	err := r.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(completed),0), COALESCE(SUM(failed),0), COALESCE(SUM(cancelled),0),
		       COALESCE(SUM(arrivals),0), COALESCE(SUM(declines),0), COALESCE(SUM(safety_declines),0),
		       COALESCE(SUM(recoveries_as_original),0), COALESCE(SUM(recoveries_as_replacement),0),
		       MAX(updated_at)
		  FROM provider_quality_snapshots
		 WHERE authority=$1 AND provider_id=$2 AND metric_date BETWEEN $3 AND $4`,
		authority, providerID, window.Start, window.End).Scan(&totals.Completed, &totals.Failed,
		&totals.Cancelled, &totals.Arrivals, &totals.Declines, &totals.SafetyDeclines, &totals.RecoveriesAsOriginal,
		&totals.RecoveriesAsReplacement, &asOf)
	if err != nil {
		return totals, err
	}
	if asOf == nil {
		return totals, ErrNoData
	}
	totals.DataAsOf = asOf.UTC()
	return totals, nil
}

// ReadinessTotals sums a branch's readiness counts over a window.
func (r *Repository) ReadinessTotals(ctx context.Context, branchID string,
	window models.Window) (models.ReadinessTotals, error) {
	var totals models.ReadinessTotals
	var asOf *time.Time
	err := r.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(ready_count),0), COALESCE(SUM(on_time_count),0), COALESCE(SUM(late_count),0),
		       COALESCE(SUM(lateness_seconds_total),0), COALESCE(SUM(prep_seconds_total),0),
		       COALESCE(SUM(prep_count),0), MAX(updated_at)
		  FROM vendor_branch_readiness_stats
		 WHERE branch_id=$1 AND metric_date BETWEEN $2 AND $3`,
		branchID, window.Start, window.End).Scan(&totals.ReadyCount, &totals.OnTimeCount, &totals.LateCount,
		&totals.LatenessSecondsTotal, &totals.PrepSecondsTotal, &totals.PrepCount, &asOf)
	if err != nil {
		return totals, err
	}
	if asOf == nil {
		return totals, ErrNoData
	}
	totals.DataAsOf = asOf.UTC()
	return totals, nil
}

// LatestSupply reads the newest supply report for a zone.
func (r *Repository) LatestSupply(ctx context.Context, authority, zone string) (models.Supply, error) {
	var supply models.Supply
	err := r.db.QueryRow(ctx, `SELECT available_providers, busy_providers, observed_at
		FROM zone_supply_snapshots WHERE authority=$1 AND zone=$2`, authority, zone).
		Scan(&supply.AvailableProviders, &supply.BusyProviders, &supply.ObservedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return supply, ErrNoData
	}
	return supply, err
}

// MissionSuccess returns daily mission outcomes, optionally for one authority.
func (r *Repository) MissionSuccess(ctx context.Context, authority string, window models.Window) ([]models.MissionSuccessDay, error) {
	rows, err := r.db.Query(ctx, `
		SELECT authority, metric_date, completed, failed, cancelled, guarantee_breaches
		  FROM mission_success_daily
		 WHERE ($1 = '' OR authority = $1) AND metric_date BETWEEN $2 AND $3
		 ORDER BY metric_date, authority`, authority, window.Start, window.End)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	days := []models.MissionSuccessDay{}
	for rows.Next() {
		var day models.MissionSuccessDay
		if err = rows.Scan(&day.Authority, &day.Date, &day.Completed, &day.Failed, &day.Cancelled,
			&day.GuaranteeBreaches); err != nil {
			return nil, err
		}
		day.SuccessRate = rate(day.Completed, day.Completed+day.Failed)
		days = append(days, day)
	}
	return days, rows.Err()
}

// Recovery returns daily recovery performance.
func (r *Repository) Recovery(ctx context.Context, authority string, window models.Window) ([]models.RecoveryDay, error) {
	rows, err := r.db.Query(ctx, `
		SELECT authority, metric_date, started, replaced, resolved, failed,
		       resolution_seconds_total, resolution_count
		  FROM recovery_performance
		 WHERE ($1 = '' OR authority = $1) AND metric_date BETWEEN $2 AND $3
		 ORDER BY metric_date, authority`, authority, window.Start, window.End)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	days := []models.RecoveryDay{}
	for rows.Next() {
		var day models.RecoveryDay
		var seconds, count int64
		if err = rows.Scan(&day.Authority, &day.Date, &day.Started, &day.Replaced, &day.Resolved,
			&day.Failed, &seconds, &count); err != nil {
			return nil, err
		}
		if count > 0 {
			day.AverageResolutionSeconds = float64(seconds) / float64(count)
		}
		days = append(days, day)
	}
	return days, rows.Err()
}

// Mismatches returns daily capability mismatches by reason.
func (r *Repository) Mismatches(ctx context.Context, authority string, window models.Window) ([]models.MismatchDay, error) {
	rows, err := r.db.Query(ctx, `
		SELECT authority, metric_date, reason_code, mismatches
		  FROM capability_mismatch_metrics
		 WHERE ($1 = '' OR authority = $1) AND metric_date BETWEEN $2 AND $3
		 ORDER BY metric_date, authority, reason_code`, authority, window.Start, window.End)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	days := []models.MismatchDay{}
	for rows.Next() {
		var day models.MismatchDay
		if err = rows.Scan(&day.Authority, &day.Date, &day.ReasonCode, &day.Mismatches); err != nil {
			return nil, err
		}
		days = append(days, day)
	}
	return days, rows.Err()
}

// Demand returns daily demand by zone.
func (r *Repository) Demand(ctx context.Context, authority string, window models.Window) ([]models.DemandDay, error) {
	rows, err := r.db.Query(ctx, `
		SELECT authority, zone, metric_date, requests, unmatched
		  FROM zone_demand_daily
		 WHERE ($1 = '' OR authority = $1) AND metric_date BETWEEN $2 AND $3
		 ORDER BY metric_date, authority, zone`, authority, window.Start, window.End)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	days := []models.DemandDay{}
	for rows.Next() {
		var day models.DemandDay
		if err = rows.Scan(&day.Authority, &day.Zone, &day.Date, &day.Requests, &day.Unmatched); err != nil {
			return nil, err
		}
		days = append(days, day)
	}
	return days, rows.Err()
}

// Declines returns daily provider declines by reason.
func (r *Repository) Declines(ctx context.Context, authority string, window models.Window) ([]models.DeclineDay, error) {
	rows, err := r.db.Query(ctx, `
		SELECT authority, metric_date, reason_code, safety, declines
		  FROM decline_reason_metrics
		 WHERE ($1 = '' OR authority = $1) AND metric_date BETWEEN $2 AND $3
		 ORDER BY metric_date, authority, reason_code`, authority, window.Start, window.End)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	days := []models.DeclineDay{}
	for rows.Next() {
		var day models.DeclineDay
		if err = rows.Scan(&day.Authority, &day.Date, &day.ReasonCode, &day.Safety, &day.Declines); err != nil {
			return nil, err
		}
		days = append(days, day)
	}
	return days, rows.Err()
}

func rate(numerator, denominator int64) float64 {
	if denominator <= 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}
