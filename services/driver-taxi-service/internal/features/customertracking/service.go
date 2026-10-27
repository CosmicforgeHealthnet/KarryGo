package customertracking

import (
	"context"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/journeyshare"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"math"
	"time"
)

type Reader interface {
	Read(context.Context, string, string) (journeyshare.Projection, error)
}
type Postgres struct {
	DB       *pgxpool.Pool
	FreshFor time.Duration
	Now      func() time.Time
	// ETA estimates pickup and drop-off times; nil leaves them unset.
	ETA journeyshare.Estimator
}

// One SQL snapshot binds the point, assigned provider and vehicle to the same
// booking row. Raw telemetry never leaves this owning service.
func (r *Postgres) Read(ctx context.Context, id, customer string) (journeyshare.Projection, error) {
	start := time.Now()
	defer func() { journeyshare.Observe("taxi_safe_location_read", time.Since(start)) }()
	p := journeyshare.Projection{Authority: "TAXI", SourceRef: id, ProductCode: "RIDE", Location: journeyshare.Location{State: "UNAVAILABLE"}}
	if _, err := uuid.Parse(id); err != nil {
		return p, apperrors.NotFound("Ride was not found.", nil)
	}
	var first, make, model, plate string
	var pickup, dropoff *journeyshare.Place
	var heading, accuracy, lat, lng *float64
	var at *time.Time
	var pickupKnown, dropoffKnown bool
	pickup = &journeyshare.Place{}
	dropoff = &journeyshare.Place{}
	err := r.DB.QueryRow(ctx, `SELECT b.customer_id::text,b.status,GREATEST(b.source_version,b.movement_version,1),b.completed_at,
 COALESCE(p.first_name,''),COALESCE(v.make,''),COALESCE(v.model,''),COALESCE(v.plate_number,''),
 COALESCE(a.address,''),COALESCE(a.lat,0),COALESCE(a.lng,0),(a.lat IS NOT NULL AND a.lng IS NOT NULL),COALESCE(z.address,''),COALESCE(z.lat,0),COALESCE(z.lng,0),(z.lat IS NOT NULL AND z.lng IS NOT NULL),
 point.lat,point.lng,point.heading_deg,point.accuracy_m,point.recorded_at
 FROM taxi_bookings b
 LEFT JOIN taxi_providers p ON p.id=b.provider_id
 LEFT JOIN vehicles v ON v.id=b.vehicle_id AND v.provider_id=b.provider_id AND v.status='active'
 LEFT JOIN LATERAL(SELECT address,lat,lng FROM taxi_booking_stops WHERE booking_id=b.id AND type='PICKUP' ORDER BY sequence LIMIT 1)a ON true
 LEFT JOIN LATERAL(SELECT address,lat,lng FROM taxi_booking_stops WHERE booking_id=b.id AND type='DROPOFF' ORDER BY sequence DESC LIMIT 1)z ON true
 LEFT JOIN LATERAL(SELECT lat,lng,heading_deg,accuracy_m,recorded_at FROM taxi_trip_location_points
 WHERE booking_id=b.id AND provider_id=b.provider_id AND integrity_flags='{}'
 AND recorded_at>=COALESCE(b.accepted_at,b.created_at) AND recorded_at<=now()
 ORDER BY sequence DESC LIMIT 1)point ON b.status IN ('accepted','en_route_pickup','arrived_at_pickup','in_progress')
 WHERE b.id=$1 AND b.customer_id=$2 AND b.ride_purpose='passenger'`, id, customer).Scan(&p.CustomerID, &p.Status, &p.SourceVersion, &p.CompletedAt, &first, &make, &model, &plate, &pickup.Label, &pickup.Lat, &pickup.Lng, &pickupKnown, &dropoff.Label, &dropoff.Lat, &dropoff.Lng, &dropoffKnown, &lat, &lng, &heading, &accuracy, &at)
	if errors.Is(err, pgx.ErrNoRows) {
		journeyshare.Observe("taxi_authorization_or_missing", 0)
		return p, apperrors.NotFound("Ride was not found.", nil)
	}
	if err != nil {
		return p, apperrors.Unavailable("Ride tracking is unavailable.", nil)
	}
	if pickupKnown {
		p.Pickup = pickup
	}
	if dropoffKnown {
		p.Dropoff = dropoff
	}
	if first != "" {
		p.Provider = &journeyshare.Provider{DisplayName: first}
		if make != "" || model != "" || plate != "" {
			p.Provider.Vehicle = &journeyshare.Vehicle{Make: make, Model: model, Plate: plate}
		}
	}
	now := time.Now().UTC()
	if r.Now != nil {
		now = r.Now().UTC()
	}
	fresh := r.FreshFor
	if fresh <= 0 {
		fresh = 30 * time.Second
	}
	p = Safe(p, lat, lng, heading, accuracy, at, now, fresh)
	p = journeyshare.WithETAs(ctx, p, r.ETA, now)
	journeyshare.Observe("taxi_location_"+p.Location.State, 0)
	if at != nil && !now.Before(*at) {
		// Customer V4 §27.2 ingest lag: provider fix to customer read.
		journeyshare.Observe("taxi_location_ingest_lag", now.Sub(*at))
	}
	return p, nil
}
func Safe(p journeyshare.Projection, lat, lng, heading, accuracy *float64, at *time.Time, now time.Time, fresh time.Duration) journeyshare.Projection {
	states := map[string]string{"scheduled": "SCHEDULED", "pending_match": "SEARCHING", "awaiting_acceptance": "SEARCHING", "accepted": "ASSIGNED", "en_route_pickup": "EN_ROUTE_PICKUP", "arrived_at_pickup": "ARRIVED", "in_progress": "IN_PROGRESS", "completed": "COMPLETED", "cancelled": "CANCELLED", "unmatched": "UNAVAILABLE"}
	p.PublicStatus = states[p.Status]
	if p.PublicStatus == "" {
		p.PublicStatus = "UNAVAILABLE"
	}
	p.Terminal = p.Status == "completed" || p.Status == "cancelled" || p.Status == "unmatched"
	p.Eligible = p.PublicStatus != "UNAVAILABLE" && !p.Terminal
	p.Progress = journeyshare.Progress{Stage: p.PublicStatus}
	p.ETA = nil
	p.Location = journeyshare.Location{State: "UNAVAILABLE"}
	active := p.Status == "accepted" || p.Status == "en_route_pickup" || p.Status == "arrived_at_pickup" || p.Status == "in_progress"
	if active && lat != nil && lng != nil && at != nil && !at.After(now) && *lat >= -90 && *lat <= 90 && *lng >= -180 && *lng <= 180 {
		if heading != nil && (math.IsNaN(*heading) || math.IsInf(*heading, 0) || *heading < 0 || *heading > 360) {
			heading = nil
		}
		if accuracy != nil && (math.IsNaN(*accuracy) || math.IsInf(*accuracy, 0) || *accuracy < 0) {
			accuracy = nil
		}
		state := "STALE"
		if now.Sub(*at) <= fresh {
			state = "FRESH"
		}
		p.Location = journeyshare.Location{Available: true, State: state, Lat: lat, Lng: lng, Heading: heading, AccuracyM: accuracy, UpdatedAt: at}
	}
	return p
}
