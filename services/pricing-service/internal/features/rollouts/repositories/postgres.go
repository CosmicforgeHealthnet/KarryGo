// Package repositories persists rollout switches and market freezes.
package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/pricing-service/internal/database"
	"cosmicforge/logistics/services/pricing-service/internal/features/rollouts/models"
)

// Store reads and writes rollout state.
type Store struct{ db database.Querier }

// NewStore returns a store over db.
func NewStore(db database.Querier) *Store { return &Store{db: db} }

// WithQuerier binds the store to a transaction.
func (s *Store) WithQuerier(q database.Querier) *Store { return &Store{db: q} }

// RecordRollout appends a switch event.
func (s *Store) RecordRollout(ctx context.Context, rollout models.Rollout) error {
	var approval any
	if rollout.ApprovalRequestID != "" {
		approval = rollout.ApprovalRequestID
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO pricing_rollouts (id, scope, capability, market_code, mode, performed_by, reason, approval_request_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		rollout.ID, rollout.Scope, rollout.Capability, rollout.MarketCode, rollout.Mode,
		rollout.PerformedBy, rollout.Reason, approval)
	if err != nil {
		return fmt.Errorf("rollouts: record rollout: %w", err)
	}
	return nil
}

// ModeFor returns the current mode for a scope, capability and market. A market
// nobody has switched is LEGACY, which is the safe default: pricing calculates
// nothing authoritative until somebody deliberately says so.
func (s *Store) ModeFor(ctx context.Context, scope, capability, marketCode string) (string, error) {
	var mode string
	err := s.db.QueryRow(ctx, `
		SELECT mode FROM pricing_rollouts
		WHERE scope = $1 AND capability = $2 AND market_code = $3
		ORDER BY created_at DESC
		LIMIT 1`, scope, capability, marketCode).Scan(&mode)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.ModeLegacy, nil
	}
	if err != nil {
		return "", fmt.Errorf("rollouts: mode lookup: %w", err)
	}
	return mode, nil
}

// RecordFreeze appends a freeze or unfreeze event.
func (s *Store) RecordFreeze(ctx context.Context, freeze models.Freeze) error {
	var approval any
	if freeze.ApprovalRequestID != "" {
		approval = freeze.ApprovalRequestID
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO pricing_market_freezes (id, market_code, action, performed_by, reason, approval_request_id)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		freeze.ID, freeze.MarketCode, freeze.Action, freeze.PerformedBy, freeze.Reason, approval)
	if err != nil {
		return fmt.Errorf("rollouts: record freeze: %w", err)
	}
	return nil
}

// FreezeStateFor returns whether a market is frozen and since when.
func (s *Store) FreezeStateFor(ctx context.Context, marketCode string) (models.FreezeState, error) {
	var action, reason string
	var createdAt time.Time
	err := s.db.QueryRow(ctx, `
		SELECT action, reason, created_at FROM pricing_market_freezes
		WHERE market_code = $1
		ORDER BY created_at DESC
		LIMIT 1`, marketCode).Scan(&action, &reason, &createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.FreezeState{}, nil
	}
	if err != nil {
		return models.FreezeState{}, fmt.Errorf("rollouts: freeze lookup: %w", err)
	}
	if action != models.ActionFreeze {
		return models.FreezeState{}, nil
	}
	return models.FreezeState{Frozen: true, Since: createdAt, Reason: reason}, nil
}

// ListRollouts returns the markets currently switched away from LEGACY.
func (s *Store) ListRollouts(ctx context.Context) ([]models.Rollout, error) {
	rows, err := s.db.Query(ctx, `
		SELECT DISTINCT ON (scope, capability, market_code)
		       id, scope, capability, market_code, mode, performed_by, reason,
		       COALESCE(approval_request_id, ''), created_at
		FROM pricing_rollouts
		ORDER BY scope, capability, market_code, created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("rollouts: list: %w", err)
	}
	defer rows.Close()

	var rollouts []models.Rollout
	for rows.Next() {
		var rollout models.Rollout
		if err := rows.Scan(&rollout.ID, &rollout.Scope, &rollout.Capability, &rollout.MarketCode,
			&rollout.Mode, &rollout.PerformedBy, &rollout.Reason,
			&rollout.ApprovalRequestID, &rollout.CreatedAt); err != nil {
			return nil, fmt.Errorf("rollouts: scan: %w", err)
		}
		rollouts = append(rollouts, rollout)
	}
	return rollouts, rows.Err()
}
