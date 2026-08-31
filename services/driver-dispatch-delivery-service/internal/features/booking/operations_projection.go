package booking

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// GetBookingOperations composes a customer-safe operational summary from the
// normalized Sections 4-12 authorities. It intentionally returns lifecycle
// state and opaque references only: no credential/hash, internal audit or
// unrestricted provider data is selected.
func (r *PostgresRepository) GetBookingOperations(ctx context.Context, bookingID string) (*BookingOperationsProjection, error) {
	projection := &BookingOperationsProjection{}
	found := false

	var group DeliveryGroupSummary
	var mission MissionSummary
	err := r.db.QueryRow(ctx, `
		SELECT g.id::text,g.state,g.version,m.id::text,m.state,m.version
		FROM dispatch_delivery_groups g
		JOIN dispatch_delivery_missions m ON m.delivery_group_id=g.id
		LEFT JOIN dispatch_mission_execution_links link ON link.mission_id=m.id
		WHERE g.booking_id=$1 OR m.booking_id=$1 OR link.booking_id=$1
		ORDER BY m.created_at DESC LIMIT 1`, bookingID).Scan(
		&group.ID, &group.State, &group.Version, &mission.ID, &mission.State, &mission.Version)
	if err == nil {
		found = true
		projection.DeliveryGroup = &group
		projection.Mission = &mission
		if err := r.db.QueryRow(ctx, `
			SELECT pm.manifest_ref,pm.version,pm.status,count(mp.parcel_id)
			FROM dispatch_parcel_manifests pm
			LEFT JOIN dispatch_manifest_parcels mp ON mp.manifest_id=pm.id
			WHERE pm.mission_id=$1
			GROUP BY pm.id
			ORDER BY CASE WHEN pm.status IN ('ACTIVE','CUSTODY_ACTIVE') THEN 0 ELSE 1 END,pm.version DESC
			LIMIT 1`, mission.ID).Scan(&mission.ManifestRef, &mission.ManifestVersion, &mission.ManifestStatus, &mission.ParcelCount); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		var assignment AssignmentSummary
		_ = r.db.QueryRow(ctx, `SELECT COALESCE((SELECT status FROM dispatch_assignment_offers WHERE mission_id=$1 ORDER BY created_at DESC LIMIT 1),''),COALESCE((SELECT status FROM dispatch_queue_entries WHERE mission_id=$1 ORDER BY created_at DESC LIMIT 1),'')`, mission.ID).Scan(&assignment.OfferStatus, &assignment.QueueStatus)
		if assignment.OfferStatus != "" || assignment.QueueStatus != "" {
			projection.Assignment = &assignment
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	var serviceability ServiceabilitySummary
	err = r.db.QueryRow(ctx, `
		SELECT q.id,q.decision,q.policy_version,q.version,q.expires_at
		FROM dispatch_bookings b JOIN dispatch_serviceability_quotes q ON q.id=b.serviceability_quote_id
		WHERE b.id=$1`, bookingID).Scan(&serviceability.Reference, &serviceability.Decision, &serviceability.PolicyVersion, &serviceability.Version, &serviceability.ExpiresAt)
	if err == nil {
		found = true
		projection.Serviceability = &serviceability
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	var tripID string
	_ = r.db.QueryRow(ctx, `SELECT COALESCE((SELECT id::text FROM trips WHERE booking_id=$1),'')`, bookingID).Scan(&tripID)
	if tripID != "" {
		var handover HandoverSummary
		if err := r.db.QueryRow(ctx, `SELECT stage,status FROM dispatch_handover_challenges WHERE trip_id=$1 ORDER BY created_at DESC LIMIT 1`, tripID).Scan(&handover.Stage, &handover.Status); err == nil {
			found = true
			projection.Handover = &handover
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}

		var evidence EvidenceSummary
		if err := r.db.QueryRow(ctx, `SELECT count(*) FROM dispatch_evidence_refs WHERE trip_id=$1`, tripID).Scan(&evidence.Count); err != nil {
			return nil, err
		}
		if evidence.Count > 0 {
			found = true
			projection.Evidence = &evidence
		}

		var recovery RecoverySummary
		if err := r.db.QueryRow(ctx, `
			SELECT COALESCE(a.state,''),COALESCE((SELECT r.action FROM dispatch_delivery_recovery_actions r WHERE r.delivery_attempt_id=a.id ORDER BY r.created_at DESC LIMIT 1),''),COALESCE((SELECT w.state FROM dispatch_return_workflows w WHERE w.original_trip_id=$1 ORDER BY w.created_at DESC LIMIT 1),'')
			FROM dispatch_delivery_attempts a WHERE a.trip_id=$1 ORDER BY a.attempt_number DESC LIMIT 1`, tripID).Scan(&recovery.AttemptState, &recovery.Action, &recovery.ReturnState); err == nil {
			found = true
			projection.Recovery = &recovery
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}

		var route RouteSummary
		if err := r.db.QueryRow(ctx, `
			SELECT er.version,er.status,count(ra.id) FILTER (WHERE ra.state='OPEN')
			FROM dispatch_expected_routes er
			LEFT JOIN dispatch_route_alerts ra ON ra.route_id=er.id
			WHERE er.trip_id=$1 AND er.status='CURRENT'
			GROUP BY er.id LIMIT 1`, tripID).Scan(&route.Version, &route.Status, &route.OpenAlertCount); err == nil {
			found = true
			projection.Route = &route
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
	}

	if !found {
		return nil, nil
	}
	return projection, nil
}
