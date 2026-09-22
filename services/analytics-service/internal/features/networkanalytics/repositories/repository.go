// Package repositories stores network events and projects them into the
// Section 13 read models.
package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/analytics-service/internal/features/networkanalytics/models"
	"cosmicforge/logistics/shared/go/networkanalytics"
)

// Repository is the network analytics store.
type Repository struct{ db *pgxpool.Pool }

// NewRepository builds the store.
func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

// Ingest stores one event and projects it, in one transaction.
//
// Deduplication is by event id AND by fact identity (entity, event name,
// version), because producers are allowed to re-publish a fact under a new
// event id. A version at or below the newest already seen for that entity and
// event name is stored as STALE and projected nowhere: counting it again
// would double a metric, and applying an old snapshot would move a read model
// backwards.
func (r *Repository) Ingest(ctx context.Context, event models.Event) (string, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Serialize per entity and event name so two deliveries of neighbouring
	// versions cannot both believe they are the newest.
	var last int64
	err = tx.QueryRow(ctx, `
		INSERT INTO analytics_network_entity_versions
			(source_service, source_entity_type, source_entity_id, event_name, last_version)
		VALUES ($1,$2,$3,$4,0)
		ON CONFLICT (source_service, source_entity_type, source_entity_id, event_name)
			DO UPDATE SET updated_at = analytics_network_entity_versions.updated_at
		RETURNING last_version`,
		event.SourceService, event.SourceEntityType, event.SourceEntityID, event.EventName).Scan(&last)
	if err != nil {
		return "", err
	}

	status := models.StatusProcessed
	switch {
	case event.Kind == "":
		status = models.StatusIgnored
	case event.SourceVersion <= last:
		status = models.StatusStale
	}

	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return "", err
	}
	tag, err := tx.Exec(ctx, `
		INSERT INTO analytics_network_events
			(event_id, event_name, kind, authority, source_service, source_entity_type, source_entity_id,
			 source_version, occurred_at, correlation_id, payload, payload_hash, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT DO NOTHING`,
		event.EventID, event.EventName, string(event.Kind), event.Authority, event.SourceService,
		event.SourceEntityType, event.SourceEntityID, event.SourceVersion, event.OccurredAt.UTC(),
		event.CorrelationID, payload, event.PayloadHash, status)
	if err != nil {
		return "", err
	}
	if tag.RowsAffected() == 0 {
		// Either the same event id, or the same fact under a new id.
		return models.StatusDuplicate, nil
	}
	if status != models.StatusProcessed {
		return status, tx.Commit(ctx)
	}

	if _, err = tx.Exec(ctx, `
		UPDATE analytics_network_entity_versions SET last_version = $5, updated_at = now()
		 WHERE source_service=$1 AND source_entity_type=$2 AND source_entity_id=$3 AND event_name=$4`,
		event.SourceService, event.SourceEntityType, event.SourceEntityID, event.EventName,
		event.SourceVersion); err != nil {
		return "", err
	}
	if err = project(ctx, tx, event); err != nil {
		return "", fmt.Errorf("project %s: %w", event.EventName, err)
	}
	return models.StatusProcessed, tx.Commit(ctx)
}

// project applies one processed event to its read model.
func project(ctx context.Context, tx pgx.Tx, e models.Event) error {
	day := e.OccurredAt.UTC().Truncate(24 * time.Hour)
	p := e.Payload
	switch e.Kind {
	case networkanalytics.KindRecoveryStarted:
		if err := bumpRecovery(ctx, tx, e.Authority, day, "started"); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO analytics_network_open_recoveries (authority, case_ref, started_at)
			VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, e.Authority, caseRef(e), e.OccurredAt.UTC()); err != nil {
			return err
		}
		return bumpProvider(ctx, tx, e.Authority, str(p, "original_provider_id", "failed_provider_id", "provider_id"),
			day, "recoveries_as_original")
	case networkanalytics.KindRecoveryReplaced:
		if err := bumpRecovery(ctx, tx, e.Authority, day, "replaced"); err != nil {
			return err
		}
		return bumpProvider(ctx, tx, e.Authority, str(p, "replacement_provider_id", "new_provider_id"),
			day, "recoveries_as_replacement")
	case networkanalytics.KindRecoveryResolved, networkanalytics.KindRecoveryFailed:
		column := "resolved"
		if e.Kind == networkanalytics.KindRecoveryFailed {
			column = "failed"
		}
		var started time.Time
		err := tx.QueryRow(ctx, `DELETE FROM analytics_network_open_recoveries WHERE authority=$1 AND case_ref=$2
			RETURNING started_at`, e.Authority, caseRef(e)).Scan(&started)
		seconds, timed := int64(0), int64(0)
		if err == nil && !e.OccurredAt.Before(started) {
			seconds, timed = int64(e.OccurredAt.Sub(started).Seconds()), 1
		} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO recovery_performance (authority, metric_date, `+column+`, resolution_seconds_total, resolution_count)
			VALUES ($1,$2,1,$3,$4)
			ON CONFLICT (authority, metric_date) DO UPDATE SET
				`+column+` = recovery_performance.`+column+` + 1,
				resolution_seconds_total = recovery_performance.resolution_seconds_total + EXCLUDED.resolution_seconds_total,
				resolution_count = recovery_performance.resolution_count + EXCLUDED.resolution_count,
				updated_at = now()`, e.Authority, day, seconds, timed)
		return err
	case networkanalytics.KindGuaranteeBreached:
		return bumpMission(ctx, tx, e.Authority, day, "guarantee_breaches")
	case networkanalytics.KindMissionCompleted, networkanalytics.KindMissionFailed, networkanalytics.KindMissionCancelled:
		column := map[networkanalytics.Kind]string{networkanalytics.KindMissionCompleted: "completed",
			networkanalytics.KindMissionFailed: "failed", networkanalytics.KindMissionCancelled: "cancelled"}[e.Kind]
		if err := bumpMission(ctx, tx, e.Authority, day, column); err != nil {
			return err
		}
		return bumpProvider(ctx, tx, e.Authority, str(p, "provider_id", "rider_id", "driver_id"), day, column)
	case networkanalytics.KindCapabilityMismatch:
		for _, reason := range reasons(p) {
			if _, err := tx.Exec(ctx, `
				INSERT INTO capability_mismatch_metrics (authority, metric_date, reason_code, mismatches)
				VALUES ($1,$2,$3,1)
				ON CONFLICT (authority, metric_date, reason_code) DO UPDATE SET
					mismatches = capability_mismatch_metrics.mismatches + 1, updated_at = now()`,
				e.Authority, day, reason); err != nil {
				return err
			}
		}
		return nil
	case networkanalytics.KindProviderArrived:
		return bumpProvider(ctx, tx, e.Authority, str(p, "provider_id", "rider_id", "driver_id"), day, "arrivals")
	case networkanalytics.KindProviderDeclined:
		reason := strings.ToUpper(str(p, "reason_code", "decline_reason", "reason"))
		if reason == "" {
			reason = "UNSPECIFIED"
		}
		safety := networkanalytics.IsSafetyDecline(reason)
		if _, err := tx.Exec(ctx, `
			INSERT INTO decline_reason_metrics (authority, metric_date, reason_code, safety, declines)
			VALUES ($1,$2,$3,$4,1)
			ON CONFLICT (authority, metric_date, reason_code) DO UPDATE SET
				declines = decline_reason_metrics.declines + 1, updated_at = now()`,
			e.Authority, day, reason, safety); err != nil {
			return err
		}
		column := "declines"
		if safety {
			column = "safety_declines"
		}
		return bumpProvider(ctx, tx, e.Authority, str(p, "provider_id", "rider_id", "driver_id"), day, column)
	case networkanalytics.KindSupplySnapshot:
		zone := str(p, "zone")
		if zone == "" {
			return nil
		}
		observed := e.OccurredAt.UTC()
		_, err := tx.Exec(ctx, `
			INSERT INTO zone_supply_snapshots (authority, zone, available_providers, busy_providers, observed_at, source_version)
			VALUES ($1,$2,$3,$4,$5,$6)
			ON CONFLICT (authority, zone) DO UPDATE SET
				available_providers = EXCLUDED.available_providers, busy_providers = EXCLUDED.busy_providers,
				observed_at = EXCLUDED.observed_at, source_version = EXCLUDED.source_version, updated_at = now()
			WHERE zone_supply_snapshots.observed_at < EXCLUDED.observed_at`,
			e.Authority, zone, nonNegative(num(p, "available_providers")), nonNegative(num(p, "busy_providers")),
			observed, e.SourceVersion)
		return err
	case networkanalytics.KindDemandObserved:
		zone := str(p, "zone")
		if zone == "" {
			return nil
		}
		requests := num(p, "requests")
		if requests <= 0 {
			requests = 1
		}
		unmatched := num(p, "unmatched")
		if flag(p, "unmatched") {
			unmatched = 1
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO zone_demand_daily (authority, zone, metric_date, requests, unmatched)
			VALUES ($1,$2,$3,$4,$5)
			ON CONFLICT (authority, zone, metric_date) DO UPDATE SET
				requests = zone_demand_daily.requests + EXCLUDED.requests,
				unmatched = zone_demand_daily.unmatched + EXCLUDED.unmatched, updated_at = now()`,
			e.Authority, zone, day, requests, nonNegative(unmatched))
		return err
	case networkanalytics.KindBranchReadiness:
		branch := str(p, "branch_id")
		if branch == "" {
			return nil
		}
		onTime, late := int64(0), int64(0)
		if flag(p, "on_time") {
			onTime = 1
		} else {
			late = 1
		}
		prep := nonNegative(num(p, "prep_seconds"))
		prepCount := int64(0)
		if prep > 0 {
			prepCount = 1
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO vendor_branch_readiness_stats
				(business_id, branch_id, metric_date, ready_count, on_time_count, late_count,
				 lateness_seconds_total, prep_seconds_total, prep_count)
			VALUES ($1,$2,$3,1,$4,$5,$6,$7,$8)
			ON CONFLICT (branch_id, metric_date) DO UPDATE SET
				ready_count = vendor_branch_readiness_stats.ready_count + 1,
				on_time_count = vendor_branch_readiness_stats.on_time_count + EXCLUDED.on_time_count,
				late_count = vendor_branch_readiness_stats.late_count + EXCLUDED.late_count,
				lateness_seconds_total = vendor_branch_readiness_stats.lateness_seconds_total + EXCLUDED.lateness_seconds_total,
				prep_seconds_total = vendor_branch_readiness_stats.prep_seconds_total + EXCLUDED.prep_seconds_total,
				prep_count = vendor_branch_readiness_stats.prep_count + EXCLUDED.prep_count,
				updated_at = now()`,
			str(p, "business_id"), branch, day, onTime, late, nonNegative(num(p, "lateness_seconds")), prep, prepCount)
		return err
	}
	return nil
}

func bumpRecovery(ctx context.Context, tx pgx.Tx, authority string, day time.Time, column string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO recovery_performance (authority, metric_date, `+column+`) VALUES ($1,$2,1)
		ON CONFLICT (authority, metric_date) DO UPDATE SET
			`+column+` = recovery_performance.`+column+` + 1, updated_at = now()`, authority, day)
	return err
}

func bumpMission(ctx context.Context, tx pgx.Tx, authority string, day time.Time, column string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO mission_success_daily (authority, metric_date, `+column+`) VALUES ($1,$2,1)
		ON CONFLICT (authority, metric_date) DO UPDATE SET
			`+column+` = mission_success_daily.`+column+` + 1, updated_at = now()`, authority, day)
	return err
}

// bumpProvider counts one fact against a provider. A fact that names no
// provider still counts in the authority-level tables; it just has no
// provider row to land in.
func bumpProvider(ctx context.Context, tx pgx.Tx, authority, providerID string, day time.Time, column string) error {
	if providerID == "" {
		return nil
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO provider_quality_snapshots (authority, provider_id, metric_date, `+column+`) VALUES ($1,$2,$3,1)
		ON CONFLICT (authority, provider_id, metric_date) DO UPDATE SET
			`+column+` = provider_quality_snapshots.`+column+` + 1, updated_at = now()`,
		authority, providerID, day)
	return err
}

// caseRef identifies a recovery case across its events. Producers name it
// in the payload; failing that, the event's own entity is the case.
func caseRef(e models.Event) string {
	if ref := str(e.Payload, "recovery_case_id", "recovery_case_ref", "incident_id"); ref != "" {
		return ref
	}
	return e.SourceEntityType + ":" + e.SourceEntityID
}

// reasons reads a mismatch's reason codes, whichever shape the producer used:
// a list, a comma-separated string, or a single code.
func reasons(p map[string]any) []string {
	var raw []string
	switch value := p["reason_codes"].(type) {
	case []any:
		for _, item := range value {
			if text, ok := item.(string); ok {
				raw = append(raw, text)
			}
		}
	case string:
		raw = strings.Split(value, ",")
	}
	if code := str(p, "reason_code"); code != "" {
		raw = append(raw, code)
	}
	seen := map[string]bool{}
	out := []string{}
	for _, code := range raw {
		code = strings.ToUpper(strings.TrimSpace(code))
		if code != "" && !seen[code] {
			seen[code] = true
			out = append(out, code)
		}
	}
	if len(out) == 0 {
		return []string{"UNSPECIFIED"}
	}
	return out
}

func str(p map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := p[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func num(p map[string]any, key string) int64 {
	switch value := p[key].(type) {
	case float64:
		return int64(value)
	case int64:
		return value
	case int:
		return int64(value)
	case json.Number:
		parsed, _ := value.Int64()
		return parsed
	case string:
		parsed, _ := strconv.ParseInt(value, 10, 64)
		return parsed
	}
	return 0
}

func flag(p map[string]any, key string) bool {
	value, ok := p[key].(bool)
	return ok && value
}

func nonNegative(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}
