package referralrepositories

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"cosmicforge/logistics/services/referral-service/internal/database"
)

// PostedTotals is what referral-service has sent one account: postings the
// wallet confirmed, and postings queued or in flight (which may or may not
// have reached the wallet yet).
type PostedTotals struct {
	ActorType            string `json:"actor_type"`
	ActorID              string `json:"actor_id"`
	PostedCreditKobo     int64  `json:"posted_credit_kobo"`
	PostedReversalKobo   int64  `json:"posted_reversal_kobo"`
	InFlightCreditKobo   int64  `json:"in_flight_credit_kobo"`
	InFlightReversalKobo int64  `json:"in_flight_reversal_kobo"`
}

// PostedTotalsPage lists, in account order after the given one, what has been
// sent to each account that has had any posting.
func (s *Store) PostedTotalsPage(ctx context.Context, q database.Querier, afterType, afterID string, limit int) ([]PostedTotals, error) {
	rows, err := q.Query(ctx, `
		SELECT beneficiary_actor_type, beneficiary_actor_id,
			COALESCE(sum(amount_kobo) FILTER (WHERE amount_kobo > 0 AND wallet_state = 'posted'), 0),
			COALESCE(-sum(amount_kobo) FILTER (WHERE amount_kobo < 0 AND wallet_state = 'posted'), 0),
			COALESCE(sum(amount_kobo) FILTER (WHERE amount_kobo > 0 AND wallet_state = 'pending'), 0),
			COALESCE(-sum(amount_kobo) FILTER (WHERE amount_kobo < 0 AND wallet_state = 'pending'), 0)
		FROM reward_ledger
		WHERE wallet_state <> 'not_sent' AND (beneficiary_actor_type, beneficiary_actor_id) > ($1, $2)
		GROUP BY beneficiary_actor_type, beneficiary_actor_id
		ORDER BY beneficiary_actor_type, beneficiary_actor_id
		LIMIT $3`, afterType, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var page []PostedTotals
	for rows.Next() {
		var t PostedTotals
		if err := rows.Scan(&t.ActorType, &t.ActorID, &t.PostedCreditKobo, &t.PostedReversalKobo, &t.InFlightCreditKobo, &t.InFlightReversalKobo); err != nil {
			return nil, err
		}
		page = append(page, t)
	}
	return page, rows.Err()
}

// RecordMismatch opens (or refreshes) an account's reconciliation mismatch.
func (s *Store) RecordMismatch(ctx context.Context, q database.Querier, t PostedTotals, walletCredit, walletReversal int64, at time.Time) error {
	_, err := q.Exec(ctx, `
		INSERT INTO reconciliation_mismatches (id, actor_type, actor_id, posted_credit_kobo, posted_reversal_kobo,
			in_flight_credit_kobo, in_flight_reversal_kobo, wallet_credit_kobo, wallet_reversal_kobo, first_seen_at, last_checked_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10)
		ON CONFLICT (actor_type, actor_id) WHERE resolved_at IS NULL DO UPDATE SET
			posted_credit_kobo = EXCLUDED.posted_credit_kobo, posted_reversal_kobo = EXCLUDED.posted_reversal_kobo,
			in_flight_credit_kobo = EXCLUDED.in_flight_credit_kobo, in_flight_reversal_kobo = EXCLUDED.in_flight_reversal_kobo,
			wallet_credit_kobo = EXCLUDED.wallet_credit_kobo, wallet_reversal_kobo = EXCLUDED.wallet_reversal_kobo,
			last_checked_at = EXCLUDED.last_checked_at`,
		"rm_"+uuid.NewString(), t.ActorType, t.ActorID, t.PostedCreditKobo, t.PostedReversalKobo,
		t.InFlightCreditKobo, t.InFlightReversalKobo, walletCredit, walletReversal, at)
	if err != nil {
		return fmt.Errorf("record reconciliation mismatch: %w", err)
	}
	return nil
}

// ResolveMismatch closes an account's open mismatch once the two agree.
func (s *Store) ResolveMismatch(ctx context.Context, q database.Querier, actorType, actorID string, at time.Time) error {
	_, err := q.Exec(ctx, `
		UPDATE reconciliation_mismatches SET resolved_at = $3, last_checked_at = $3
		WHERE actor_type = $1 AND actor_id = $2 AND resolved_at IS NULL`, actorType, actorID, at)
	return err
}

// Mismatch is an open reconciliation mismatch.
type Mismatch struct {
	PostedTotals
	WalletCreditKobo   int64     `json:"wallet_credit_kobo"`
	WalletReversalKobo int64     `json:"wallet_reversal_kobo"`
	FirstSeenAt        time.Time `json:"first_seen_at"`
	LastCheckedAt      time.Time `json:"last_checked_at"`
}

// OpenMismatches lists unresolved mismatches, oldest first.
func (s *Store) OpenMismatches(ctx context.Context, q database.Querier, limit int) ([]Mismatch, error) {
	rows, err := q.Query(ctx, `
		SELECT actor_type, actor_id, posted_credit_kobo, posted_reversal_kobo, in_flight_credit_kobo, in_flight_reversal_kobo,
			wallet_credit_kobo, wallet_reversal_kobo, first_seen_at, last_checked_at
		FROM reconciliation_mismatches WHERE resolved_at IS NULL ORDER BY first_seen_at LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Mismatch
	for rows.Next() {
		var m Mismatch
		if err := rows.Scan(&m.ActorType, &m.ActorID, &m.PostedCreditKobo, &m.PostedReversalKobo, &m.InFlightCreditKobo,
			&m.InFlightReversalKobo, &m.WalletCreditKobo, &m.WalletReversalKobo, &m.FirstSeenAt, &m.LastCheckedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// HasOpenPersonFlag reports an open flag of a type about a person (any type
// when flagType is empty), counting only flags that need a reviewer.
func (s *Store) HasOpenPersonFlag(ctx context.Context, q database.Querier, personID, flagType string) (bool, error) {
	var exists bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM fraud_flags
			WHERE person_id = $1 AND status = 'open' AND severity = 'review' AND ($2 = '' OR flag_type = $2))`,
		personID, flagType).Scan(&exists)
	return exists, err
}
