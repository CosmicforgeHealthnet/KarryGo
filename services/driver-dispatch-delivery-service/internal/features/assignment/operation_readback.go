package assignment

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// ReadStart resolves an acknowledged operation before fresh candidate discovery.
// A timeout retry must not fail because the winning provider is now busy on
// this very mission, nor create a second assignment after the first committed.
func (r *Repository) ReadStart(ctx context.Context, in StartRequest, strategy Strategy) (Assignment, bool, error) {
	var result Assignment
	var key, fp string
	err := r.db.QueryRow(ctx, `SELECT p.strategy,p.idempotency_key,p.request_fingerprint,m.state,COALESCE(m.trip_id::text,'')
		FROM dispatch_assignment_policies p JOIN dispatch_delivery_missions m ON m.id=p.mission_id
		WHERE p.mission_id=$1::uuid`, in.MissionID).Scan(&result.Strategy, &key, &fp, &result.Status, &result.TripID)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, false, nil
	}
	if err != nil {
		return result, false, err
	}
	if key != in.IdempotencyKey || fp != fingerprint(in) || result.Strategy != strategy {
		return Assignment{}, true, ErrConflict
	}
	result.MissionID, result.Replayed = in.MissionID, true
	var offer Offer
	err = r.db.QueryRow(ctx, `SELECT o.id::text,o.mission_id::text,o.provider_id::text,o.status,c.rank,o.offered_at,o.expires_at,o.version
		FROM dispatch_assignment_offers o JOIN dispatch_assignment_candidates c ON c.id=o.candidate_id
		WHERE o.mission_id=$1::uuid AND o.incident_id IS NULL AND o.status IN ('OFFERED','ACCEPTED') ORDER BY o.created_at DESC LIMIT 1`, in.MissionID).
		Scan(&offer.ID, &offer.MissionID, &offer.ProviderID, &offer.Status, &offer.Rank, &offer.OfferedAt, &offer.ExpiresAt, &offer.Version)
	if err == nil {
		result.Offer = &offer
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Assignment{}, true, err
	}
	return result, true, nil
}
