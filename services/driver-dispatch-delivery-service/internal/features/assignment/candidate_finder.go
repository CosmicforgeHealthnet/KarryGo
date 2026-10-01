package assignment

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/availability"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/parcelfit"
	"cosmicforge/logistics/shared/go/logging"
)

type nearbyAuthority interface {
	GetNearby(context.Context, availability.NearbyProvidersRequest) ([]availability.NearbyProvider, error)
}
type CandidateAuthority struct {
	db     *pgxpool.Pool
	live   nearbyAuthority
	radius float64
	limit  int

	// gate applies hard parcel fit after proximity discovery. A nil gate is a
	// pass-through, so a deployment with Section 8 switched off discovers
	// candidates exactly as it did before.
	gate *CapabilityGate
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

// WithCapabilityGate attaches hard parcel fit.
//
// It is a separate call rather than a constructor argument so every existing
// caller keeps compiling unchanged and gets legacy behaviour, which is what
// makes this additive rather than a rewrite of the assignment path.
func (a *CandidateAuthority) WithCapabilityGate(gate *CapabilityGate) *CandidateAuthority {
	if a != nil {
		a.gate = gate
	}
	return a
}

// missionRequirement reads the physical requirement a mission implies from the
// parcel rows it already carries.
//
// commerce_mission_parcels has stored weight, volume, dimensions, package
// count and a handling snapshot since migration 000042. Nothing has ever read
// them for eligibility; this is where that starts. A mission with no parcel
// rows -- a legacy or non-commerce mission -- yields an empty requirement,
// which constrains nothing and therefore cannot change its outcome.
func (a *CandidateAuthority) missionRequirement(ctx context.Context, missionID string) (parcelfit.Requirement, error) {
	rows, err := a.db.Query(ctx, `
SELECT p.weight_grams, p.volume_ml, p.length_mm, p.width_mm, p.height_mm,
       p.package_count, p.handling_snapshot
  FROM commerce_mission_parcels p
  JOIN commerce_missions m ON m.id = p.commerce_mission_id
  JOIN dispatch_delivery_missions d ON d.id = $1
 WHERE m.mission_ref = d.mission_ref`, missionID)
	if err != nil {
		return parcelfit.Requirement{}, err
	}
	defer rows.Close()

	var parcels []ParcelSnapshot
	for rows.Next() {
		var parcel ParcelSnapshot
		if err := rows.Scan(&parcel.WeightGrams, &parcel.VolumeMl,
			&parcel.LengthMm, &parcel.WidthMm, &parcel.HeightMm,
			&parcel.PackageCount, &parcel.Handling); err != nil {
			return parcelfit.Requirement{}, err
		}
		parcels = append(parcels, parcel)
	}
	if err := rows.Err(); err != nil {
		return parcelfit.Requirement{}, err
	}
	return RequirementFromMissionSnapshot(parcels), nil
}

// gateRequirement resolves the requirement for the gate, treating a read
// failure as an empty requirement rather than an error.
//
// That is deliberate and worth stating: failing discovery because the parcel
// rows could not be read would take dispatch down for a data problem, whereas
// an empty requirement simply means the gate can only apply the checks that do
// not depend on parcel facts. The bike's own resolution failure is still
// reported honestly by the gate itself.
func (a *CandidateAuthority) gateRequirement(ctx context.Context, missionID string) parcelfit.Requirement {
	requirement, err := a.missionRequirement(ctx, missionID)
	if err != nil {
		logging.Error("dispatch-capability",
			"parcel requirement unavailable for mission %s: %v", missionID, err)
		return parcelfit.Requirement{}
	}
	return requirement
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

	// Hard parcel fit, after proximity and the legacy structural gates. The
	// SQL above establishes that a rider is verified, free and riding roughly
	// the right kind of machine; it cannot establish that the parcel fits the
	// box, that a chilled order stays chilled, or that 18kg is within payload.
	// This is where those are decided, and it only ever removes from the list.
	if a.gate.Active() {
		out = a.gate.Apply(ctx, missionID, a.gateRequirement(ctx, missionID), out)
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

	// The same gate discovery applies, so an accept or a direct assignment
	// cannot route around a check the candidate search would have made. An
	// assignment path that skipped it would be the obvious way to hand a
	// rider a parcel their bike cannot carry.
	if a.gate.Active() {
		allowed, reason := a.gate.Allows(ctx, missionID,
			a.gateRequirement(ctx, missionID), providerID)
		if !allowed {
			logging.Notice("dispatch-capability",
				"provider %s refused mission %s on hard parcel fit (%s)",
				providerID, missionID, reason)
			return ErrConflict
		}
	}
	return nil
}
