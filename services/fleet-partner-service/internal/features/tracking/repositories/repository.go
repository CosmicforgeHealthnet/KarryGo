// Package repositories owns every Section 16 tracking-projection SQL
// statement: the authoritative event inbox, projections, version conflicts
// and the freshness/retention sweep. Methods that take a database.Querier
// join the caller's transaction.
package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	dispatchmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/dispatch/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/tracking/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/pagination"
)

// Repository is the PostgreSQL tracking-projection store.
type Repository struct{ pool *pgxpool.Pool }

// NewRepository builds the repository.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

const projectionColumns = `organization_id,source_service,canonical_trip_reference,assignment_id,COALESCE(route_plan_id,''),provider_id,vehicle_or_truck_reference,latitude,longitude,heading,speed,accuracy,operational_status,source_timestamp,received_at,source_version,freshness_status,gap_status,last_reconciled_at`

func scanProjection(row pgx.Row) (models.TrackingProjection, error) {
	var v models.TrackingProjection
	err := row.Scan(&v.OrganizationID, &v.SourceService, &v.CanonicalTripReference, &v.AssignmentID, &v.RoutePlanID, &v.ProviderID, &v.VehicleOrTruckReference, &v.Latitude, &v.Longitude, &v.Heading, &v.Speed, &v.Accuracy, &v.OperationalStatus, &v.SourceTimestamp, &v.ReceivedAt, &v.SourceVersion, &v.FreshnessStatus, &v.GapStatus, &v.LastReconciledAt)
	if errors.Is(err, pgx.ErrNoRows) {
		err = dispatchmodels.ErrNotFound
	}
	return v, err
}

// RecordInbound inserts the event into the inbox; inserted is false when the
// event id was already received.
func (r *Repository) RecordInbound(ctx context.Context, q database.Querier, e models.TrackingEvent, payloadHash string) (bool, error) {
	tag, x := q.Exec(ctx, `INSERT INTO fleet_partner_tracking_event_inbox(event_id,organization_id,source_service,canonical_trip_reference,source_version,payload_hash) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(event_id) DO NOTHING`, e.EventID, e.OrganizationID, e.SourceService, e.CanonicalTripReference, e.SourceVersion, payloadHash)
	if x != nil {
		return false, x
	}
	return tag.RowsAffected() > 0, nil
}

// LockProjection locks the current projection of one trip and returns its
// source version, payload hash and gap status.
func (r *Repository) LockProjection(ctx context.Context, q database.Querier, sourceService, trip string) (int64, string, string, error) {
	var version int64
	var hash, gap string
	x := q.QueryRow(ctx, `SELECT source_version,payload_hash,gap_status FROM fleet_partner_tracking_projections WHERE source_service=$1 AND canonical_trip_reference=$2 FOR UPDATE`, sourceService, trip).Scan(&version, &hash, &gap)
	if errors.Is(x, pgx.ErrNoRows) {
		x = dispatchmodels.ErrNotFound
	}
	return version, hash, gap, x
}

// RecordConflict stores a same-version event with a different payload.
func (r *Repository) RecordConflict(ctx context.Context, q database.Querier, e models.TrackingEvent, existingHash, receivedHash string) error {
	_, x := q.Exec(ctx, `INSERT INTO fleet_partner_tracking_projection_conflicts(id,organization_id,source_service,canonical_trip_reference,source_version,existing_payload_hash,received_payload_hash) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING`, ids.New("tpc"), e.OrganizationID, e.SourceService, e.CanonicalTripReference, e.SourceVersion, existingHash, receivedHash)
	return x
}

// UpsertProjection stores the projection for the event.
func (r *Repository) UpsertProjection(ctx context.Context, q database.Querier, e models.TrackingEvent, receivedAt time.Time, payloadHash, freshness, gapStatus string, reconciledAt *time.Time, retentionExpiresAt time.Time) error {
	_, x := q.Exec(ctx, `INSERT INTO fleet_partner_tracking_projections(organization_id,source_service,canonical_trip_reference,assignment_id,route_plan_id,provider_id,vehicle_or_truck_reference,latitude,longitude,heading,speed,accuracy,operational_status,source_timestamp,received_at,source_version,payload_hash,freshness_status,gap_status,last_reconciled_at,retention_expires_at) VALUES($1,$2,$3,$4,NULLIF($5,''),$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21) ON CONFLICT(source_service,canonical_trip_reference) DO UPDATE SET organization_id=EXCLUDED.organization_id,assignment_id=EXCLUDED.assignment_id,route_plan_id=EXCLUDED.route_plan_id,provider_id=EXCLUDED.provider_id,vehicle_or_truck_reference=EXCLUDED.vehicle_or_truck_reference,latitude=EXCLUDED.latitude,longitude=EXCLUDED.longitude,heading=EXCLUDED.heading,speed=EXCLUDED.speed,accuracy=EXCLUDED.accuracy,operational_status=EXCLUDED.operational_status,source_timestamp=EXCLUDED.source_timestamp,received_at=EXCLUDED.received_at,source_version=EXCLUDED.source_version,payload_hash=EXCLUDED.payload_hash,freshness_status=EXCLUDED.freshness_status,gap_status=EXCLUDED.gap_status,last_reconciled_at=COALESCE(EXCLUDED.last_reconciled_at,fleet_partner_tracking_projections.last_reconciled_at),retention_expires_at=EXCLUDED.retention_expires_at`, e.OrganizationID, e.SourceService, e.CanonicalTripReference, e.AssignmentID, e.RoutePlanID, e.ProviderID, e.VehicleOrTruckReference, e.Latitude, e.Longitude, e.Heading, e.Speed, e.Accuracy, e.OperationalStatus, e.SourceTimestamp, receivedAt, e.SourceVersion, payloadHash, freshness, gapStatus, reconciledAt, retentionExpiresAt)
	return x
}

// Get reads the organization's projection of one canonical trip.
func (r *Repository) Get(ctx context.Context, org, trip string) (models.TrackingProjection, error) {
	return scanProjection(r.pool.QueryRow(ctx, `SELECT `+projectionColumns+` FROM fleet_partner_tracking_projections WHERE organization_id=$1 AND canonical_trip_reference=$2`, org, trip))
}

// List reads up to limit projections, newest received first with the
// projection identity as a stable tie-breaker. It pages by f.Offset, or by
// the keyset position f.After/f.AfterID (a ProjectionKey) when set.
func (r *Repository) List(ctx context.Context, org string, f dispatchmodels.PageFilter, limit int) ([]models.TrackingProjection, error) {
	args := []any{org}
	where := `organization_id=$1`
	if f.ProviderID != "" {
		args = append(args, f.ProviderID)
		where += fmt.Sprintf(` AND provider_id=$%d`, len(args))
	}
	if f.VehicleReference != "" {
		args = append(args, f.VehicleReference)
		where += fmt.Sprintf(` AND vehicle_or_truck_reference=$%d`, len(args))
	}
	offset := pagination.ClampOffset(f.Offset)
	if f.After != nil {
		source, trip, _ := models.SplitProjectionKey(f.AfterID)
		args = append(args, *f.After, source, trip)
		where += fmt.Sprintf(` AND (received_at<$%d OR (received_at=$%d AND (source_service,canonical_trip_reference)>($%d,$%d)))`, len(args)-2, len(args)-2, len(args)-1, len(args))
		offset = 0
	}
	args = append(args, limit, offset)
	rows, e := r.pool.Query(ctx, `SELECT `+projectionColumns+` FROM fleet_partner_tracking_projections WHERE `+where+fmt.Sprintf(` ORDER BY received_at DESC,source_service,canonical_trip_reference LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []models.TrackingProjection{}
	for rows.Next() {
		v, e := scanProjection(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ProjectionGaps reads projections whose authoritative sequence has a gap,
// for the control-tower board.
func (r *Repository) ProjectionGaps(ctx context.Context, org string) ([]map[string]any, error) {
	gapRows, e := r.pool.Query(ctx, `SELECT source_service,canonical_trip_reference,source_version,gap_status,received_at
FROM fleet_partner_tracking_projections WHERE organization_id=$1 AND gap_status<>'CURRENT' ORDER BY received_at DESC LIMIT 200`, org)
	if e != nil {
		return nil, e
	}
	defer gapRows.Close()
	projectionGaps := []map[string]any{}
	for gapRows.Next() {
		var source, trip, status string
		var version int64
		var receivedAt time.Time
		if e = gapRows.Scan(&source, &trip, &version, &status, &receivedAt); e != nil {
			return nil, e
		}
		projectionGaps = append(projectionGaps, map[string]any{"sourceService": source, "canonicalTripReference": trip, "sourceVersion": version, "gapStatus": status, "receivedAt": receivedAt})
	}
	return projectionGaps, gapRows.Err()
}

// GapStatus reads the gap status of one projection.
func (r *Repository) GapStatus(ctx context.Context, org, sourceService, trip string) (string, error) {
	var gapStatus string
	e := r.pool.QueryRow(ctx, `SELECT gap_status FROM fleet_partner_tracking_projections WHERE organization_id=$1 AND source_service=$2 AND canonical_trip_reference=$3`, org, sourceService, trip).Scan(&gapStatus)
	return gapStatus, e
}

// ResolveConflicts closes the open conflicts of a projection that is current
// again.
func (r *Repository) ResolveConflicts(ctx context.Context, org, sourceService, trip string) error {
	_, e := r.pool.Exec(ctx, `UPDATE fleet_partner_tracking_projection_conflicts SET status='RESOLVED',resolved_at=now(),lease_owner=NULL,lease_expires_at=NULL WHERE organization_id=$1 AND source_service=$2 AND canonical_trip_reference=$3 AND status IN('OPEN','RECONCILING')`, org, sourceService, trip)
	return e
}

// MarkConflictsReconciling moves open conflicts to RECONCILING and reports
// how many moved.
func (r *Repository) MarkConflictsReconciling(ctx context.Context, org, sourceService, trip string) (int64, error) {
	tag, e := r.pool.Exec(ctx, `UPDATE fleet_partner_tracking_projection_conflicts SET status='RECONCILING',lease_heartbeat_at=now() WHERE organization_id=$1 AND source_service=$2 AND canonical_trip_reference=$3 AND status='OPEN'`, org, sourceService, trip)
	if e != nil {
		return 0, e
	}
	return tag.RowsAffected(), nil
}

// SweepProjections deletes projections past retention and recomputes every
// stored freshness status.
func (r *Repository) SweepProjections(ctx context.Context, q database.Querier, freshAfter, staleAfter time.Duration) error {
	if _, e := q.Exec(ctx, `DELETE FROM fleet_partner_tracking_projections WHERE retention_expires_at<now()`); e != nil {
		return e
	}
	_, e := q.Exec(ctx, `UPDATE fleet_partner_tracking_projections SET freshness_status=CASE WHEN gap_status<>'CURRENT' THEN 'GAPPED' WHEN source_timestamp<now()-$1::interval THEN 'STALE' WHEN source_timestamp<now()-$2::interval THEN 'AGING' ELSE 'FRESH' END`, durationInterval(staleAfter), durationInterval(freshAfter))
	return e
}

func durationInterval(d time.Duration) string { return fmt.Sprintf("%d seconds", int64(d.Seconds())) }

// StaleProjections reads up to 200 projections marked STALE.
func (r *Repository) StaleProjections(ctx context.Context, q database.Querier) ([]models.StaleProjection, error) {
	rows, e := q.Query(ctx, `SELECT organization_id,source_service,canonical_trip_reference,COALESCE(route_plan_id,''),assignment_id FROM fleet_partner_tracking_projections WHERE freshness_status='STALE' LIMIT 200`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	stale := []models.StaleProjection{}
	for rows.Next() {
		var v models.StaleProjection
		if e = rows.Scan(&v.OrganizationID, &v.SourceService, &v.CanonicalTripReference, &v.RoutePlanID, &v.AssignmentID); e != nil {
			return nil, e
		}
		stale = append(stale, v)
	}
	return stale, rows.Err()
}
