package assignment

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"time"
)

type ReplacementContext struct {
	IncidentID, MissionID, TripID, OldProviderID, OrganizationID string
	Strategy                                                     Strategy
	Ready, LiveOffer                                             bool
}
type replacementStore interface {
	ReadReplacement(context.Context, string) (ReplacementContext, error)
	OfferReplacement(context.Context, ReplacementContext, []Candidate, time.Time, time.Duration) (bool, error)
}

// ReconcileReplacement reuses discovery, ranking, Fleet membership and the
// exclusive-offer inbox. Acceptance reserves a replacement, never a second Trip.
func (s *Service) ReconcileReplacement(ctx context.Context, incidentID string) (bool, error) {
	r, ok := s.store.(replacementStore)
	if !ok {
		return false, errors.New("replacement store unavailable")
	}
	scope, err := r.ReadReplacement(ctx, incidentID)
	if err != nil || scope.Ready {
		return scope.Ready, err
	}
	if scope.LiveOffer {
		return false, nil
	}
	strategy := scope.Strategy
	if strategy == BroadcastFirstAccept {
		strategy = SequentialNearest
	}
	if _, err = s.policy.Select(strategy, scope.OrganizationID, false); err != nil {
		return false, err
	}
	if strategy == AdminAssisted {
		return false, errors.New("replacement requires authorized operations selection")
	}
	if s.finder == nil {
		return false, errors.New("candidate authority unavailable")
	}
	found, err := s.finder.Discover(ctx, scope.MissionID, false)
	if err != nil {
		return false, err
	}
	candidates := make([]Candidate, 0, len(found))
	for _, c := range found {
		if c.ProviderID == scope.OldProviderID {
			continue
		}
		if strategy == OrganizationDispatch {
			if s.organizations == nil {
				return false, errors.New("Fleet authority unavailable")
			}
			if err = s.organizations.ValidateMembership(ctx, scope.OrganizationID, c.ProviderID); err != nil {
				continue
			}
		}
		candidates = append(candidates, c)
	}
	ranked, err := RankCandidates(candidates)
	if err != nil {
		return false, err
	}
	return r.OfferReplacement(ctx, scope, ranked, s.now(), s.policy.OfferWindow)
}

func (r *Repository) ReadReplacement(ctx context.Context, id string) (ReplacementContext, error) {
	var c ReplacementContext
	err := r.db.QueryRow(ctx, `SELECT i.id::text,m.id::text,t.id::text,i.provider_id::text,CASE WHEN COALESCE(t.organization_id,'')<>'' THEN 'ORGANIZATION_DISPATCH' ELSE COALESCE(p.strategy,'SEQUENTIAL_NEAREST') END,COALESCE(NULLIF(t.organization_id,''),p.organization_id::text,''),i.recovery_result='READY',EXISTS(SELECT 1 FROM dispatch_assignment_offers o WHERE o.incident_id=i.id AND o.status='OFFERED' AND o.expires_at>now())
 FROM dispatch_delivery_incidents i JOIN trips t ON t.id=i.trip_id JOIN dispatch_delivery_missions m ON m.trip_id=t.id
 JOIN dispatch_delivery_groups g ON g.id=m.delivery_group_id LEFT JOIN dispatch_assignment_policies p ON p.mission_id=m.id
 WHERE i.id=$1 AND i.state='REASSIGNING' AND m.state='REASSIGNING' AND t.provider_id=i.provider_id
 AND t.status NOT IN ('completed','cancelled','failed') AND g.state NOT IN ('COMPLETED','CANCELLED','FAILED','DELIVERED','RETURNED')`, id).Scan(&c.IncidentID, &c.MissionID, &c.TripID, &c.OldProviderID, &c.Strategy, &c.OrganizationID, &c.Ready, &c.LiveOffer)
	return c, err
}

func (r *Repository) OfferReplacement(ctx context.Context, c ReplacementContext, candidates []Candidate, now time.Time, window time.Duration) (bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var valid, ready bool
	err = tx.QueryRow(ctx, `SELECT m.state='REASSIGNING' AND t.status NOT IN ('completed','cancelled','failed') AND t.provider_id=i.provider_id AND g.state NOT IN ('COMPLETED','CANCELLED','FAILED','DELIVERED','RETURNED') AND i.recovery_deadline>$2 AND i.recovery_result IN ('PENDING','UNKNOWN','READY'),i.recovery_result='READY'
 FROM dispatch_delivery_incidents i JOIN trips t ON t.id=i.trip_id JOIN dispatch_delivery_missions m ON m.trip_id=t.id JOIN dispatch_delivery_groups g ON g.id=m.delivery_group_id
 WHERE i.id=$1 AND m.id=$3 FOR UPDATE OF i,m,t,g`, c.IncidentID, now, c.MissionID).Scan(&valid, &ready)
	if err != nil || !valid {
		return false, ErrConflict
	}
	if ready {
		return true, tx.Commit(ctx)
	}
	// Read-back precedes creation: lost responses and two sweep instances reuse
	// the same persisted live/accepted offer.
	var existing string
	err = tx.QueryRow(ctx, `SELECT status FROM dispatch_assignment_offers WHERE incident_id=$1 AND (status='ACCEPTED' OR (status='OFFERED' AND expires_at>$2)) ORDER BY created_at DESC LIMIT 1`, c.IncidentID, now).Scan(&existing)
	if err == nil {
		return existing == "ACCEPTED", tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	if _, err = tx.Exec(ctx, `UPDATE dispatch_assignment_offers SET status='EXPIRED',responded_at=$2,version=version+1 WHERE incident_id=$1 AND status='OFFERED' AND expires_at<=$2`, c.IncidentID, now); err != nil {
		return false, err
	}
	for _, candidate := range candidates {
		var eligible, offered bool
		if err = tx.QueryRow(ctx, `SELECT `+providerEligibleForMissionSQL+`,EXISTS(SELECT 1 FROM dispatch_assignment_offers WHERE incident_id=$3 AND provider_id=$1) FROM dispatch_delivery_missions m WHERE m.id=$2`, candidate.ProviderID, c.MissionID, c.IncidentID).Scan(&eligible, &offered); err != nil {
			return false, err
		}
		if !eligible || offered {
			continue
		}
		raw, _ := json.Marshal(candidate.Snapshot)
		var candidateID string
		err = tx.QueryRow(ctx, `INSERT INTO dispatch_assignment_candidates(mission_id,provider_id,rank,distance_meters,candidate_snapshot) SELECT $1,$2,COALESCE(MAX(rank),0)+1,$3,$4 FROM dispatch_assignment_candidates WHERE mission_id=$1 ON CONFLICT(mission_id,provider_id) DO UPDATE SET provider_id=EXCLUDED.provider_id RETURNING id::text`, c.MissionID, candidate.ProviderID, candidate.DistanceM, raw).Scan(&candidateID)
		if err != nil {
			return false, err
		}
		_, err = tx.Exec(ctx, `INSERT INTO dispatch_assignment_offers(mission_id,candidate_id,provider_id,offered_at,expires_at,incident_id) VALUES($1,$2,$3,$4,$5,$6)`, c.MissionID, candidateID, candidate.ProviderID, now, now.Add(window), c.IncidentID)
		if err != nil {
			return false, err
		}
		break
	}
	return false, tx.Commit(ctx)
}

func (r *Repository) replacementOffer(ctx context.Context, id string) (bool, error) {
	var v bool
	err := r.db.QueryRow(ctx, `SELECT incident_id IS NOT NULL FROM dispatch_assignment_offers WHERE id=$1`, id).Scan(&v)
	return v, err
}

func (r *Repository) declineReplacement(ctx context.Context, id, provider string, d Decision, now time.Time) (Assignment, error) {
	var mission string
	err := r.db.QueryRow(ctx, `UPDATE dispatch_assignment_offers SET status='DECLINED',responded_at=$4,decline_reason=$5,idempotency_key=$6,request_fingerprint=$7,version=CASE WHEN status='DECLINED' THEN version ELSE version+1 END WHERE id=$1 AND provider_id=$2 AND incident_id IS NOT NULL AND ((status='OFFERED' AND version=$3 AND expires_at>$4) OR (status='DECLINED' AND idempotency_key=$6 AND request_fingerprint=$7)) RETURNING mission_id::text`, id, provider, d.ExpectedVersion, now, d.Reason, d.IdempotencyKey, fingerprint(d)).Scan(&mission)
	return Assignment{MissionID: mission, Status: "REASSIGNING"}, err
}

func (r *Repository) acceptReplacement(ctx context.Context, id, provider string, d Decision, now time.Time) (Assignment, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Assignment{}, err
	}
	defer tx.Rollback(ctx)
	var mission, tripID, incident, status, key, fp, originalProvider string
	var version int64
	var expires time.Time
	var valid bool
	err = tx.QueryRow(ctx, `SELECT o.mission_id::text,t.id::text,i.id::text,o.status,COALESCE(o.idempotency_key,''),COALESCE(o.request_fingerprint,''),o.provider_id::text,o.version,o.expires_at,
 m.state='REASSIGNING' AND i.state='REASSIGNING' AND i.recovery_result IN ('PENDING','UNKNOWN','READY') AND i.recovery_deadline>$3 AND t.provider_id=i.provider_id AND t.status NOT IN ('completed','cancelled','failed') AND g.state NOT IN ('COMPLETED','CANCELLED','FAILED','DELIVERED','RETURNED')
 FROM dispatch_assignment_offers o JOIN dispatch_delivery_incidents i ON i.id=o.incident_id JOIN dispatch_delivery_missions m ON m.id=o.mission_id JOIN trips t ON t.id=i.trip_id JOIN dispatch_delivery_groups g ON g.id=m.delivery_group_id
 WHERE o.id=$1 AND o.provider_id=$2 FOR UPDATE OF o,i,m,t,g`, id, provider, now).Scan(&mission, &tripID, &incident, &status, &key, &fp, &originalProvider, &version, &expires, &valid)
	if err != nil {
		return Assignment{}, err
	}
	want := fingerprint(struct {
		Offer, Provider string
		Version         int64
	}{id, provider, d.ExpectedVersion})
	if status == "ACCEPTED" && key == d.IdempotencyKey && fp == want {
		return Assignment{MissionID: mission, TripID: tripID, Status: "REPLACEMENT_READY", Replayed: true}, tx.Commit(ctx)
	}
	if !valid || status != "OFFERED" || !expires.After(now) || version != d.ExpectedVersion {
		return Assignment{}, ErrConflict
	}
	var eligible bool
	if err = tx.QueryRow(ctx, `SELECT `+providerEligibleForMissionSQL+` FROM dispatch_delivery_missions m WHERE id=$2`, provider, mission).Scan(&eligible); err != nil || !eligible {
		return Assignment{}, ErrConflict
	}
	if _, err = tx.Exec(ctx, `UPDATE dispatch_assignment_offers SET status='ACCEPTED',responded_at=$2,idempotency_key=$3,request_fingerprint=$4,version=version+1 WHERE id=$1`, id, now, d.IdempotencyKey, want); err != nil {
		return Assignment{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE dispatch_delivery_incidents SET replacement_provider_id=$2,recovery_result='READY',recovery_error_code='' WHERE id=$1`, incident, provider); err != nil {
		return Assignment{}, err
	}
	return Assignment{MissionID: mission, TripID: tripID, Status: "REPLACEMENT_READY"}, tx.Commit(ctx)
}
