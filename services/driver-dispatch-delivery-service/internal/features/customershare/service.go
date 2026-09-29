package customershare

import (
	"context"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/availability"
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

func (s *Service) Record(ctx context.Context, provider string, l availability.Location) {
	// Existing provider ingestion resolved to the only current canonical trip.
	var ref, binding string
	err := s.DB.QueryRow(ctx, `SELECT id::text,updated_at::text FROM trips WHERE provider_id=$1 AND status IN ('assigned','en_route_pickup','arrived_pickup','in_progress') AND 1=(SELECT count(*) FROM trips WHERE provider_id=$1 AND status IN ('assigned','en_route_pickup','arrived_pickup','in_progress')) LIMIT 1`, provider).Scan(&ref, &binding)
	if err == nil {
		_ = s.Points.Put(ctx, ref, provider, binding, journeyshare.Point{Lat: l.Lat, Lng: l.Lng, Heading: l.Heading, Accuracy: l.Accuracy, At: l.UpdatedAt})
	}
}
func (s *Service) Read(ctx context.Context, ref, customer string) (journeyshare.Projection, error) {
	p := journeyshare.Projection{Authority: "DISPATCH", SourceRef: ref, ProductCode: "SEND", Location: journeyshare.Location{State: "UNAVAILABLE"}}
	var provider, binding, first string
	if _, err := uuid.Parse(ref); err != nil {
		return p, apperrors.NotFound("Journey was not found.", nil)
	}
	pickup := &journeyshare.Place{}
	dropoff := &journeyshare.Place{}
	err := s.DB.QueryRow(ctx, `SELECT t.customer_id::text,t.status,t.provider_id::text,t.updated_at::text,t.completed_at,t.pickup_address,t.pickup_lat,t.pickup_lng,t.dropoff_address,t.dropoff_lat,t.dropoff_lng,1+(SELECT count(*) FROM trip_state_log WHERE trip_id=t.id),COALESCE(split_part(p.full_name,' ',1),'') FROM trips t LEFT JOIN providers p ON p.id=t.provider_id WHERE t.id=$1 AND t.customer_id=$2`, ref, customer).Scan(&p.CustomerID, &p.Status, &provider, &binding, &p.CompletedAt, &pickup.Label, &pickup.Lat, &pickup.Lng, &dropoff.Label, &dropoff.Lat, &dropoff.Lng, &p.SourceVersion, &first)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, apperrors.NotFound("Journey was not found.", nil)
	}
	if err != nil {
		return p, apperrors.Unavailable("Journey tracking is unavailable.", nil)
	}
	states := map[string]string{"assigned": "ASSIGNED", "en_route_pickup": "EN_ROUTE_PICKUP", "arrived_pickup": "ARRIVED", "in_progress": "IN_PROGRESS", "proof_submitted": "DELIVERED", "completed": "COMPLETED", "cancelled": "CANCELLED", "failed": "UNAVAILABLE"}
	p.PublicStatus = states[p.Status]
	if p.PublicStatus == "" {
		p.PublicStatus = "UNAVAILABLE"
	}
	p.Terminal = p.Status == "completed" || p.Status == "cancelled" || p.Status == "failed"
	p.Eligible = !p.Terminal && p.PublicStatus != "UNAVAILABLE"
	p.Progress = journeyshare.Progress{Stage: p.PublicStatus}
	if first != "" {
		p.Provider = &journeyshare.Provider{DisplayName: first}
	}
	p.Pickup = pickup
	p.Dropoff = dropoff
	if !p.Terminal {
		p.Location, err = s.Points.Read(ctx, ref, provider, binding, 30*time.Second)
		if err != nil {
			return p, apperrors.Unavailable("Journey tracking is unavailable.", nil)
		}
	}
	// Verify assignment did not change during Redis read. No old rider position survives reassignment.
	var same bool
	err = s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM trips WHERE id=$1 AND provider_id::text=$2 AND updated_at::text=$3 AND status=$4)`, ref, provider, binding, p.Status).Scan(&same)
	if err != nil || !same {
		return p, apperrors.Unavailable("Journey changed; retry the read.", nil)
	}
	return p, nil
}
