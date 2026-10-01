// Package repositories persists approval requests and decisions.
package repositories

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/pricing-service/internal/database"
	"cosmicforge/logistics/services/pricing-service/internal/features/approvals/models"
)

// Store reads and writes approvals.
type Store struct{ db database.Querier }

// NewStore returns a store over db.
func NewStore(db database.Querier) *Store { return &Store{db: db} }

// WithQuerier returns a store bound to a transaction, so an approval decision and
// the change it authorises commit together.
func (s *Store) WithQuerier(q database.Querier) *Store { return &Store{db: q} }

// Create records a new pending request.
func (s *Store) Create(ctx context.Context, request models.Request) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO pricing_approval_requests
			(id, subject_type, subject_id, subject_hash, requested_by, reason, status)
		VALUES ($1, $2, $3, $4, $5, $6, 'PENDING')`,
		request.ID, request.SubjectType, request.SubjectID, request.SubjectHash,
		request.RequestedBy, request.Reason)
	if err != nil {
		return fmt.Errorf("approvals: create request: %w", err)
	}
	return nil
}

// Get returns one request.
func (s *Store) Get(ctx context.Context, id string) (models.Request, error) {
	var request models.Request
	err := s.db.QueryRow(ctx, `
		SELECT id, subject_type, subject_id, subject_hash, requested_by, reason, status, created_at, decided_at
		FROM pricing_approval_requests WHERE id = $1`, id,
	).Scan(&request.ID, &request.SubjectType, &request.SubjectID, &request.SubjectHash,
		&request.RequestedBy, &request.Reason, &request.Status, &request.CreatedAt, &request.DecidedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Request{}, fmt.Errorf("%w: %s", models.ErrNotFound, id)
	}
	if err != nil {
		return models.Request{}, fmt.Errorf("approvals: get request: %w", err)
	}
	return request, nil
}

// Decide records the decision and closes the request, in one transaction-safe
// pair of statements.
//
// The database also refuses self-approval and a second decision; this method maps
// those refusals onto domain errors so a caller can respond usefully.
func (s *Store) Decide(ctx context.Context, decision models.Decision) error {
	if _, err := s.db.Exec(ctx, `
		INSERT INTO pricing_approval_decisions (id, request_id, decided_by, decision, reason)
		VALUES ($1, $2, $3, $4, $5)`,
		decision.ID, decision.RequestID, decision.DecidedBy, decision.Decision, decision.Reason,
	); err != nil {
		message := err.Error()
		switch {
		case strings.Contains(message, "self-approval is refused"):
			return fmt.Errorf("%w: %s", models.ErrSelfApproval, decision.DecidedBy)
		case strings.Contains(message, "pricing_approval_decisions_request_id_key"):
			return fmt.Errorf("%w: %s", models.ErrAlreadyDecided, decision.RequestID)
		default:
			return fmt.Errorf("approvals: record decision: %w", err)
		}
	}

	tag, err := s.db.Exec(ctx, `
		UPDATE pricing_approval_requests
		SET status = $2, decided_at = now()
		WHERE id = $1 AND status = 'PENDING'`, decision.RequestID, decision.Decision)
	if err != nil {
		return fmt.Errorf("approvals: close request: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%w: %s", models.ErrAlreadyDecided, decision.RequestID)
	}
	return nil
}

// Cancel withdraws a pending request, for example when its draft is discarded.
func (s *Store) Cancel(ctx context.Context, id string) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE pricing_approval_requests SET status = 'CANCELLED'
		WHERE id = $1 AND status = 'PENDING'`, id)
	if err != nil {
		return fmt.Errorf("approvals: cancel: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%w: %s", models.ErrAlreadyDecided, id)
	}
	return nil
}

// ListPending returns requests awaiting a decision, oldest first.
func (s *Store) ListPending(ctx context.Context, limit int) ([]models.Request, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, subject_type, subject_id, subject_hash, requested_by, reason, status, created_at, decided_at
		FROM pricing_approval_requests
		WHERE status = 'PENDING'
		ORDER BY created_at
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("approvals: list pending: %w", err)
	}
	defer rows.Close()

	var requests []models.Request
	for rows.Next() {
		var request models.Request
		if err := rows.Scan(&request.ID, &request.SubjectType, &request.SubjectID, &request.SubjectHash,
			&request.RequestedBy, &request.Reason, &request.Status, &request.CreatedAt, &request.DecidedAt); err != nil {
			return nil, fmt.Errorf("approvals: scan pending: %w", err)
		}
		requests = append(requests, request)
	}
	return requests, rows.Err()
}

// ApprovedFor returns the approved request for a subject whose hash still
// matches, which is how a caller checks "may I act on this now".
func (s *Store) ApprovedFor(ctx context.Context, subjectType, subjectID, subjectHash string) (models.Request, error) {
	var request models.Request
	err := s.db.QueryRow(ctx, `
		SELECT id, subject_type, subject_id, subject_hash, requested_by, reason, status, created_at, decided_at
		FROM pricing_approval_requests
		WHERE subject_type = $1 AND subject_id = $2 AND status = 'APPROVED'
		ORDER BY decided_at DESC
		LIMIT 1`, subjectType, subjectID,
	).Scan(&request.ID, &request.SubjectType, &request.SubjectID, &request.SubjectHash,
		&request.RequestedBy, &request.Reason, &request.Status, &request.CreatedAt, &request.DecidedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Request{}, fmt.Errorf("%w: no approval for %s %s", models.ErrNotFound, subjectType, subjectID)
	}
	if err != nil {
		return models.Request{}, fmt.Errorf("approvals: find approval: %w", err)
	}
	if request.SubjectHash != subjectHash {
		return models.Request{}, fmt.Errorf("%w: approved %s but the subject is now %s",
			models.ErrSubjectChanged, short(request.SubjectHash), short(subjectHash))
	}
	return request, nil
}

func short(hash string) string {
	if len(hash) <= 12 {
		return hash
	}
	return hash[:12]
}

// PendingFor returns the open request for a subject, which is what a decision
// acts on.
func (s *Store) PendingFor(ctx context.Context, subjectType, subjectID string) (models.Request, error) {
	var request models.Request
	err := s.db.QueryRow(ctx, `
		SELECT id, subject_type, subject_id, subject_hash, requested_by, reason, status, created_at, decided_at
		FROM pricing_approval_requests
		WHERE subject_type = $1 AND subject_id = $2 AND status = 'PENDING'
		ORDER BY created_at DESC
		LIMIT 1`, subjectType, subjectID,
	).Scan(&request.ID, &request.SubjectType, &request.SubjectID, &request.SubjectHash,
		&request.RequestedBy, &request.Reason, &request.Status, &request.CreatedAt, &request.DecidedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Request{}, fmt.Errorf("%w: no pending approval for %s %s", models.ErrNotFound, subjectType, subjectID)
	}
	if err != nil {
		return models.Request{}, fmt.Errorf("approvals: find pending: %w", err)
	}
	return request, nil
}

// Now is here so tests can freeze time without the store importing a clock.
var Now = func() time.Time { return time.Now().UTC() }
