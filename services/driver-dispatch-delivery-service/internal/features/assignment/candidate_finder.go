package assignment

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/availability"
)

type nearbyAuthority interface {
	GetNearby(context.Context, availability.NearbyProvidersRequest) ([]availability.NearbyProvider, error)
}
type CandidateAuthority struct {
	db     *pgxpool.Pool
	live   nearbyAuthority
	radius float64
	limit  int
}

const providerEligibleSQL = `EXISTS(
	SELECT 1 FROM providers p
	WHERE p.id=$1 AND p.is_active AND p.onboarding_complete AND p.verification_status='verified'
	AND NOT EXISTS(SELECT 1 FROM dispatch_authority_restrictions r WHERE r.subject_type='PROVIDER' AND r.subject_reference=p.id::text AND r.state='APPLIED')
	AND NOT EXISTS(SELECT 1 FROM trips t WHERE t.provider_id=p.id AND t.status IN ('assigned','en_route_pickup','arrived_pickup','in_progress','proof_submitted'))
	AND EXISTS(SELECT 1 FROM bikes b WHERE b.provider_id=p.id AND b.is_active AND b.verification_status='verified' AND (
		$2='' OR ($2='MOTORCYCLE' AND b.bike_type IN ('motorcycle','dispatch_bike','scooter','electric_bike')) OR lower(b.bike_type)=lower($2)
	))
)`

const providerEligibleForMissionSQL = `EXISTS(
	SELECT 1 FROM providers p
	WHERE p.id=$1 AND p.is_active AND p.onboarding_complete AND p.verification_status='verified'
	AND NOT EXISTS(SELECT 1 FROM dispatch_authority_restrictions r WHERE r.subject_type='PROVIDER' AND r.subject_reference=p.id::text AND r.state='APPLIED')
	AND NOT EXISTS(SELECT 1 FROM trips t WHERE t.provider_id=p.id AND t.status IN ('assigned','en_route_pickup','arrived_pickup','in_progress','proof_submitted'))
	AND EXISTS(SELECT 1 FROM provider_availability a WHERE a.provider_id=p.id AND a.status='online' AND a.verified_to_go_online)
	AND EXISTS(SELECT 1 FROM bikes b WHERE b.provider_id=p.id AND b.is_active AND b.verification_status='verified' AND (
		m.vehicle_class='' OR (m.vehicle_class='MOTORCYCLE' AND b.bike_type IN ('motorcycle','dispatch_bike','scooter','electric_bike')) OR lower(b.bike_type)=lower(m.vehicle_class)
	))
)`

func NewCandidateAuthority(db *pgxpool.Pool, live nearbyAuthority, radius float64) *CandidateAuthority {
	if radius <= 0 {
		radius = 10
	}
	return &CandidateAuthority{db: db, live: live, radius: radius, limit: 50}
}
func (a *CandidateAuthority) Discover(ctx context.Context, missionID string, expanded bool) ([]Candidate, error) {
	if a == nil || a.db == nil || a.live == nil {
		return nil, errors.New("authoritative candidate discovery is unavailable")
	}
	var raw []byte
	var vehicleClass string
	err := a.db.QueryRow(ctx, `SELECT s.location_snapshot,m.vehicle_class FROM dispatch_mission_stops s JOIN dispatch_delivery_missions m ON m.id=s.mission_id WHERE s.mission_id=$1 AND s.stop_type='PICKUP' ORDER BY s.sequence LIMIT 1`, missionID).Scan(&raw, &vehicleClass)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var point struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	}
	if err = json.Unmarshal(raw, &point); err != nil {
		return nil, err
	}
	radius := a.radius
	if expanded {
		radius *= 2
		if radius > 50 {
			radius = 50
		}
	}
	nearby, err := a.live.GetNearby(ctx, availability.NearbyProvidersRequest{Latitude: point.Latitude, Longitude: point.Longitude, RadiusKM: radius, Limit: a.limit})
	if err != nil {
		return nil, err
	}
	out := make([]Candidate, 0, len(nearby))
	for _, p := range nearby {
		var eligible bool
		err = a.db.QueryRow(ctx, `SELECT `+providerEligibleSQL, p.ProviderID, vehicleClass).Scan(&eligible)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if eligible {
			out = append(out, Candidate{ProviderID: p.ProviderID, DistanceM: int64(p.DistanceKM * 1000), Snapshot: map[string]any{"eligibility": "verified_available", "location_updated_at": p.UpdatedAt}})
		}
	}
	return out, nil
}

func (a *CandidateAuthority) Validate(ctx context.Context, missionID, providerID string) error {
	if a == nil || a.db == nil {
		return errors.New("authoritative candidate validation is unavailable")
	}
	var eligible bool
	if err := a.db.QueryRow(ctx, `SELECT `+providerEligibleForMissionSQL+` FROM dispatch_delivery_missions m WHERE m.id=$2`, providerID, missionID).Scan(&eligible); err != nil {
		return err
	}
	if !eligible {
		return ErrConflict
	}
	return nil
}
