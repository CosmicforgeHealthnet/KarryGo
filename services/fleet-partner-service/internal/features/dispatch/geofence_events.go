package dispatch

import (
	"context"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// GeofenceEvent is one derived geofence transition (spec 16.12, 45.2). Events
// are written only by the geofence worker from FRESH authoritative tracking
// projections; this read surface never derives or edits them. DataFreshness
// reports how live the triggering location fix is at read time, so an old
// transition is never mistaken for a live position.
type GeofenceEvent struct {
	ID                     string    `json:"id"`
	OrganizationID         string    `json:"organizationId"`
	GeofenceID             string    `json:"geofenceId"`
	GeofenceName           string    `json:"geofenceName"`
	BranchID               string    `json:"branchId,omitempty"`
	CanonicalTripReference string    `json:"canonicalTripReference"`
	EventRule              string    `json:"eventRule"`
	SourceVersion          int64     `json:"sourceVersion"`
	Confidence             string    `json:"confidence"`
	OccurredAt             time.Time `json:"occurredAt"`
	RecordedAt             time.Time `json:"recordedAt"`
	DataFreshness          string    `json:"dataFreshness"`
	AgeSeconds             int64     `json:"ageSeconds"`
}

type GeofenceEventFilter struct {
	GeofenceID, CanonicalTripReference, EventRule, BranchID string
	From, To                                                *time.Time
	Limit, Offset                                           int
}

func (s *Service) ListGeofenceEvents(ctx context.Context, t tenant.Context, f GeofenceEventFilter) ([]GeofenceEvent, error) {
	if x := s.gate(s.o.Flags.Geofences, false, "geofences_disabled"); x != nil {
		return nil, x
	}
	if f.BranchID != "" && !t.BranchPermitted(f.BranchID) {
		return []GeofenceEvent{}, nil
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT e.id,e.organization_id,e.geofence_id,g.name,COALESCE(g.branch_id,''),e.canonical_trip_reference,e.event_rule,e.source_version,e.confidence,e.occurred_at,e.created_at
FROM fleet_partner_geofence_events e JOIN fleet_partner_geofences g ON g.organization_id=e.organization_id AND g.id=e.geofence_id
WHERE e.organization_id=$1 AND ($2='' OR e.geofence_id=$2) AND ($3='' OR e.canonical_trip_reference=$3) AND ($4='' OR e.event_rule=$4) AND ($5='' OR g.branch_id=$5)
AND ($6::timestamptz IS NULL OR e.occurred_at>=$6) AND ($7::timestamptz IS NULL OR e.occurred_at<$7)
ORDER BY e.occurred_at DESC,e.id LIMIT $8 OFFSET $9`, t.OrganizationID, f.GeofenceID, f.CanonicalTripReference, f.EventRule, f.BranchID, f.From, f.To, bounded(f.Limit), max(f.Offset, 0))
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	now := s.o.Now()
	out := []GeofenceEvent{}
	for rows.Next() {
		var v GeofenceEvent
		if e = rows.Scan(&v.ID, &v.OrganizationID, &v.GeofenceID, &v.GeofenceName, &v.BranchID, &v.CanonicalTripReference, &v.EventRule, &v.SourceVersion, &v.Confidence, &v.OccurredAt, &v.RecordedAt); e != nil {
			return nil, e
		}
		if !t.BranchPermitted(v.BranchID) {
			continue
		}
		v.DataFreshness = Freshness(v.OccurredAt, now, s.o.TrackingFreshAfter, s.o.TrackingStaleAfter, false)
		v.AgeSeconds = int64(now.Sub(v.OccurredAt).Seconds())
		out = append(out, v)
	}
	return out, rows.Err()
}
