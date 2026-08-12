package vendorauthrepositories

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	vendorauthmodels "cosmicforge/logistics/services/vendor-service/internal/features/auth/models"
)

type StepUpRepository struct {
	db *pgxpool.Pool
}

func NewStepUpRepository(db *pgxpool.Pool) *StepUpRepository {
	return &StepUpRepository{db: db}
}

const stepUpColumns = `id, session_id, staff_id, vendor_id, purpose, status, expires_at, consumed_at, created_at`

func (r *StepUpRepository) Create(ctx context.Context, g vendorauthmodels.StepUpGrant) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO vendor_step_up_grants (id, session_id, staff_id, vendor_id, purpose, status, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		g.ID, g.SessionID, g.StaffID, g.VendorID, g.Purpose, vendorauthmodels.StepUpStatusActive, g.ExpiresAt)
	return err
}

func (r *StepUpRepository) GetByID(ctx context.Context, id string) (vendorauthmodels.StepUpGrant, error) {
	row := r.db.QueryRow(ctx, `SELECT `+stepUpColumns+` FROM vendor_step_up_grants WHERE id = $1`, id)
	g, err := scanStepUp(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return vendorauthmodels.StepUpGrant{}, fmt.Errorf("step-up grant %s: %w", id, ErrNotFound)
	}
	return g, err
}

// Consume atomically transitions an active grant to consumed. RowsAffected()
// == 0 means it was already consumed/expired/revoked by someone else —
// callers must treat that as "not usable", never retry-consume.
func (r *StepUpRepository) Consume(ctx context.Context, id string) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE vendor_step_up_grants SET status = $1, consumed_at = now()
		WHERE id = $2 AND status = $3`,
		vendorauthmodels.StepUpStatusConsumed, id, vendorauthmodels.StepUpStatusActive)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// RevokeForSession revokes every active step-up grant tied to a session —
// called when a session is revoked, so a stale grant from a now-dead session
// can never be consumed.
func (r *StepUpRepository) RevokeForSession(ctx context.Context, sessionID string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE vendor_step_up_grants SET status = $1
		WHERE session_id = $2 AND status = $3`,
		vendorauthmodels.StepUpStatusRevoked, sessionID, vendorauthmodels.StepUpStatusActive)
	return err
}

func scanStepUp(row pgx.Row) (vendorauthmodels.StepUpGrant, error) {
	var g vendorauthmodels.StepUpGrant
	err := row.Scan(&g.ID, &g.SessionID, &g.StaffID, &g.VendorID, &g.Purpose, &g.Status, &g.ExpiresAt, &g.ConsumedAt, &g.CreatedAt)
	return g, err
}
