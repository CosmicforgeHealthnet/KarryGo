package customershare

import (
	"context"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/journeyshare"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type Service struct {
	DB     *pgxpool.Pool
	Points journeyshare.BoundPointStore
}

func (s *Service) Record(ctx context.Context, provider string, lat, lng, heading, accuracy float64) {
	var ref, binding string
	err := s.DB.QueryRow(ctx, `SELECT id::text,updated_at::text FROM haulage_bookings WHERE provider_id=$1 AND status IN ('accepted','en_route_pickup','arrived_at_pickup','picked_up','en_route_delivery') AND 1=(SELECT count(*) FROM haulage_bookings WHERE provider_id=$1 AND status IN ('accepted','en_route_pickup','arrived_at_pickup','picked_up','en_route_delivery')) LIMIT 1`, provider).Scan(&ref, &binding)
	if err == nil {
		_ = s.Points.Put(ctx, ref, provider, binding, journeyshare.Point{Lat: lat, Lng: lng, Heading: heading, Accuracy: accuracy, At: time.Now().UTC()})
	}
}
func (s *Service) Read(ctx context.Context, ref, customer string) (journeyshare.Projection, error) {
	p := journeyshare.Projection{Authority: "HAULING", SourceRef: ref, ProductCode: "TRUCK", Location: journeyshare.Location{State: "UNAVAILABLE"}}
	var provider, binding, first, make, model, plate string
	if _, err := uuid.Parse(ref); err != nil {
		return p, apperrors.NotFound("Journey was not found.", nil)
	}
	pickup := &journeyshare.Place{}
	dropoff := &journeyshare.Place{}
	err := s.DB.QueryRow(ctx, `SELECT b.customer_id::text,b.status,COALESCE(b.provider_id::text,''),b.updated_at::text,b.completed_at,b.pickup_address,b.pickup_lat,b.pickup_lng,b.dropoff_address,b.dropoff_lat,b.dropoff_lng,1+(SELECT count(*) FROM booking_events WHERE booking_id=b.id),COALESCE(p.first_name,''),COALESCE(v.make,''),COALESCE(v.model,''),COALESCE(v.plate_number,'') FROM haulage_bookings b LEFT JOIN truck_providers p ON p.id=b.provider_id LEFT JOIN trucks v ON v.id=b.truck_id AND v.provider_id=b.provider_id AND v.status='active' WHERE b.id=$1 AND b.customer_id=$2`, ref, customer).Scan(&p.CustomerID, &p.Status, &provider, &binding, &p.CompletedAt, &pickup.Label, &pickup.Lat, &pickup.Lng, &dropoff.Label, &dropoff.Lat, &dropoff.Lng, &p.SourceVersion, &first, &make, &model, &plate)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, apperrors.NotFound("Journey was not found.", nil)
	}
	if err != nil {
		return p, apperrors.Unavailable("Journey tracking is unavailable.", nil)
	}
	states := map[string]string{"pending_match": "SEARCHING", "awaiting_acceptance": "SEARCHING", "accepted": "ASSIGNED", "en_route_pickup": "EN_ROUTE_PICKUP", "arrived_at_pickup": "ARRIVED", "picked_up": "IN_PROGRESS", "en_route_delivery": "IN_PROGRESS", "delivered": "DELIVERED", "completed": "COMPLETED", "cancelled": "CANCELLED", "unmatched": "UNAVAILABLE"}
	p.PublicStatus = states[p.Status]
	if p.PublicStatus == "" {
		p.PublicStatus = "UNAVAILABLE"
	}
	p.Terminal = p.Status == "completed" || p.Status == "cancelled" || p.Status == "unmatched"
	p.Eligible = !p.Terminal && p.PublicStatus != "UNAVAILABLE"
	p.Progress = journeyshare.Progress{Stage: p.PublicStatus}
	if first != "" {
		p.Provider = &journeyshare.Provider{DisplayName: first}
		if make != "" || model != "" {
			p.Provider.Vehicle = &journeyshare.Vehicle{Make: make, Model: model, Plate: plate}
		}
	}
	p.Pickup = pickup
	p.Dropoff = dropoff
	if !p.Terminal && provider != "" {
		p.Location, err = s.Points.Read(ctx, ref, provider, binding, 30*time.Second)
		if err != nil {
			return p, apperrors.Unavailable("Journey tracking is unavailable.", nil)
		}
	}
	var same bool
	err = s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM haulage_bookings WHERE id=$1 AND COALESCE(provider_id::text,'')=$2 AND updated_at::text=$3 AND status=$4)`, ref, provider, binding, p.Status).Scan(&same)
	if err != nil || !same {
		return p, apperrors.Unavailable("Journey changed; retry the read.", nil)
	}
	return p, nil
}
