package trip

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

type providerMissionProjectionRepository interface {
	GetProviderMissionProjection(context.Context, string, string) (*ProviderMissionProjection, error)
}

// GetProviderMissionProjection joins only Dispatch-owned normalized state and
// independently scopes the read to the assigned provider. Possessing a Trip ID
// alone is therefore insufficient.
func (r *PostgresRepository) GetProviderMissionProjection(ctx context.Context, tripID, providerID string) (*ProviderMissionProjection, error) {
	projection := &ProviderMissionProjection{Stops: []ProviderMissionStop{}, Parcels: []ProviderParcel{}}
	err := r.db.QueryRow(ctx, `
		SELECT m.id::text,m.delivery_group_id::text,m.state,m.version
		FROM trips t
		JOIN dispatch_mission_execution_links link ON link.booking_id=t.booking_id
		JOIN dispatch_delivery_missions m ON m.id=link.mission_id
		WHERE t.id=$1 AND t.provider_id=$2`, tripID, providerID).Scan(
		&projection.MissionID, &projection.DeliveryGroupID, &projection.State, &projection.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	rows, err := r.db.Query(ctx, `SELECT id::text,stop_ref,sequence,stop_type,location_snapshot FROM dispatch_mission_stops WHERE mission_id=$1 ORDER BY sequence`, projection.MissionID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var stop ProviderMissionStop
		var location []byte
		if err := rows.Scan(&stop.ID, &stop.StopRef, &stop.Sequence, &stop.Type, &location); err != nil {
			rows.Close()
			return nil, err
		}
		if err := json.Unmarshal(location, &stop.Location); err != nil {
			rows.Close()
			return nil, err
		}
		projection.Stops = append(projection.Stops, stop)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	var manifestID string
	var manifest ProviderManifest
	err = r.db.QueryRow(ctx, `
		SELECT id::text,manifest_ref,version,status FROM dispatch_parcel_manifests
		WHERE mission_id=$1
		ORDER BY CASE WHEN status IN ('ACTIVE','CUSTODY_ACTIVE') THEN 0 ELSE 1 END,version DESC LIMIT 1`, projection.MissionID).Scan(
		&manifestID, &manifest.Reference, &manifest.Version, &manifest.Status)
	if err == nil {
		projection.Manifest = &manifest
		parcelRows, queryErr := r.db.Query(ctx, `
			SELECT p.karrygo_parcel_ref,p.scannable_code,p.state
			FROM dispatch_manifest_parcels mp JOIN dispatch_parcels p ON p.id=mp.parcel_id
			WHERE mp.manifest_id=$1 ORDER BY mp.ordinal`, manifestID)
		if queryErr != nil {
			return nil, queryErr
		}
		for parcelRows.Next() {
			var parcel ProviderParcel
			if err := parcelRows.Scan(&parcel.Reference, &parcel.ScannableCode, &parcel.State); err != nil {
				parcelRows.Close()
				return nil, err
			}
			projection.Parcels = append(projection.Parcels, parcel)
		}
		if err := parcelRows.Err(); err != nil {
			parcelRows.Close()
			return nil, err
		}
		parcelRows.Close()
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	var handover ProviderHandover
	if err := r.db.QueryRow(ctx, `SELECT stage,status FROM dispatch_handover_challenges WHERE trip_id=$1 ORDER BY created_at DESC LIMIT 1`, tripID).Scan(&handover.Stage, &handover.Status); err == nil {
		projection.Handover = &handover
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	var recovery ProviderRecovery
	if err := r.db.QueryRow(ctx, `
		SELECT a.state,COALESCE((SELECT action FROM dispatch_delivery_recovery_actions WHERE delivery_attempt_id=a.id ORDER BY created_at DESC LIMIT 1),'')
		FROM dispatch_delivery_attempts a WHERE a.trip_id=$1 ORDER BY attempt_number DESC LIMIT 1`, tripID).Scan(&recovery.AttemptState, &recovery.Action); err == nil {
		projection.Recovery = &recovery
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	var route ProviderRoute
	if err := r.db.QueryRow(ctx, `
		SELECT er.version,er.status,count(ra.id) FILTER (WHERE ra.state='OPEN')
		FROM dispatch_expected_routes er LEFT JOIN dispatch_route_alerts ra ON ra.route_id=er.id
		WHERE er.trip_id=$1 AND er.status='CURRENT' GROUP BY er.id LIMIT 1`, tripID).Scan(&route.Version, &route.Status, &route.OpenAlertCount); err == nil {
		projection.Route = &route
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	return projection, nil
}

func providerAllowedActions(status TripStatus) []string {
	switch status {
	case StatusAssigned, StatusEnRoutePickup:
		return []string{"ARRIVE_AT_PICKUP", "REPORT_BREAKDOWN"}
	case StatusArrivedPickup:
		return []string{"READ_PICKUP_CREDENTIAL", "SCAN_PARCELS", "START_DELIVERY", "REPORT_BREAKDOWN"}
	case StatusInProgress:
		return []string{"SCAN_PARCELS", "VERIFY_DELIVERY", "START_DELIVERY_ATTEMPT", "REPORT_BREAKDOWN"}
	case StatusProofSubmitted:
		return []string{"COMPLETE"}
	default:
		return []string{}
	}
}
