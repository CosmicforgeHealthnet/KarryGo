package referralrepositories

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/referral-service/internal/database"
	referralmodels "cosmicforge/logistics/services/referral-service/internal/features/referrals/models"
)

// AdminAction is one recorded operator decision.
type AdminAction struct {
	Action      string
	SubjectKind string
	SubjectID   string
	PerformedBy string
	Caller      string
	Reason      string
	Details     map[string]any
}

// RecordAdminAction appends an operator decision to the audit trail. Call it
// in the transaction that applies the decision.
func (s *Store) RecordAdminAction(ctx context.Context, q database.Querier, a AdminAction) error {
	details := a.Details
	if details == nil {
		details = map[string]any{}
	}
	raw, err := json.Marshal(details)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `
		INSERT INTO referral_admin_actions (id, action, subject_kind, subject_id, performed_by, caller, reason, details)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		"aa_"+uuid.NewString(), a.Action, a.SubjectKind, a.SubjectID, a.PerformedBy, a.Caller, a.Reason, raw)
	if err != nil {
		return fmt.Errorf("record admin action: %w", err)
	}
	return nil
}

const flagColumns = `id, COALESCE(person_id, ''), COALESCE(referral_id, ''), flag_type, severity, evidence, status,
	created_at, resolved_at, resolved_by, resolution_note`

func scanFlag(row pgx.Row) (referralmodels.Flag, error) {
	var f referralmodels.Flag
	var evidence []byte
	err := row.Scan(&f.ID, &f.PersonID, &f.ReferralID, &f.FlagType, &f.Severity, &evidence, &f.Status,
		&f.CreatedAt, &f.ResolvedAt, &f.ResolvedBy, &f.ResolutionNote)
	if err != nil {
		return f, notFound(err)
	}
	f.Evidence = map[string]any{}
	if len(evidence) > 0 {
		if err := json.Unmarshal(evidence, &f.Evidence); err != nil {
			return f, err
		}
	}
	return f, nil
}

// ListFlags lists flags in a status (all statuses when empty), newest first.
func (s *Store) ListFlags(ctx context.Context, q database.Querier, status string, limit int) ([]referralmodels.Flag, error) {
	rows, err := q.Query(ctx, `SELECT `+flagColumns+` FROM fraud_flags
		WHERE ($1 = '' OR status = $1) ORDER BY created_at DESC LIMIT $2`, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var flags []referralmodels.Flag
	for rows.Next() {
		f, err := scanFlag(rows)
		if err != nil {
			return nil, err
		}
		flags = append(flags, f)
	}
	return flags, rows.Err()
}

// FlagsForPerson lists the flags raised about a person or their referrals.
func (s *Store) FlagsForPerson(ctx context.Context, q database.Querier, personID string) ([]referralmodels.Flag, error) {
	rows, err := q.Query(ctx, `SELECT `+flagColumns+` FROM fraud_flags
		WHERE person_id = $1 OR referral_id IN (
			SELECT id FROM referrals WHERE referrer_person_id = $1 OR referred_person_id = $1)
		ORDER BY created_at DESC LIMIT 200`, personID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var flags []referralmodels.Flag
	for rows.Next() {
		f, err := scanFlag(rows)
		if err != nil {
			return nil, err
		}
		flags = append(flags, f)
	}
	return flags, rows.Err()
}

// GetFlag loads (and optionally locks) one flag.
func (s *Store) GetFlag(ctx context.Context, q database.Querier, id string, lock bool) (referralmodels.Flag, error) {
	query := `SELECT ` + flagColumns + ` FROM fraud_flags WHERE id = $1`
	if lock {
		query += ` FOR UPDATE`
	}
	return scanFlag(q.QueryRow(ctx, query, id))
}

// ResolveFlag records a reviewer's outcome for a flag.
func (s *Store) ResolveFlag(ctx context.Context, q database.Querier, id, status, resolvedBy, note string, at time.Time) error {
	_, err := q.Exec(ctx, `
		UPDATE fraud_flags SET status = $2, resolved_by = $3, resolution_note = $4, resolved_at = $5
		WHERE id = $1`, id, status, resolvedBy, note, at)
	return err
}

// HasOpenFlag reports whether a referral already has an open flag of a type,
// so repeated evidence does not bury reviewers in duplicates.
func (s *Store) HasOpenFlag(ctx context.Context, q database.Querier, referralID, flagType string) (bool, error) {
	var exists bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM fraud_flags WHERE referral_id = $1 AND flag_type = $2 AND status = 'open')`,
		referralID, flagType).Scan(&exists)
	return exists, err
}

// ReferralsByReferrer lists the referrals a person made, newest first.
func (s *Store) ReferralsByReferrer(ctx context.Context, q database.Querier, referrerPersonID string, limit int) ([]referralmodels.Referral, error) {
	rows, err := q.Query(ctx, `SELECT `+referralColumns+` FROM referrals
		WHERE referrer_person_id = $1 ORDER BY attributed_at DESC LIMIT $2`, referrerPersonID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var referrals []referralmodels.Referral
	for rows.Next() {
		r, err := scanReferral(rows)
		if err != nil {
			return nil, err
		}
		referrals = append(referrals, r)
	}
	return referrals, rows.Err()
}

// RewardTotals sums a person's rewards (as beneficiary) by status and balance.
type RewardTotals struct {
	Status     string `json:"status"`
	Balance    string `json:"balance"`
	AmountKobo int64  `json:"amount_kobo"`
	Count      int    `json:"count"`
}

// RewardTotalsForPerson summarizes a person's rewards.
func (s *Store) RewardTotalsForPerson(ctx context.Context, q database.Querier, personID string) ([]RewardTotals, error) {
	rows, err := q.Query(ctx, `
		SELECT status, balance, COALESCE(sum(amount_kobo), 0), count(*) FROM reward_ledger
		WHERE person_id = $1 GROUP BY status, balance ORDER BY status, balance`, personID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var totals []RewardTotals
	for rows.Next() {
		var t RewardTotals
		if err := rows.Scan(&t.Status, &t.Balance, &t.AmountKobo, &t.Count); err != nil {
			return nil, err
		}
		totals = append(totals, t)
	}
	return totals, rows.Err()
}
