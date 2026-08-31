// Package adminaudit persists taxi_admin_audit_events (phase B10): an
// immutable record of every operator-initiated action against a booking or
// provider outside the normal customer/driver-facing lifecycle. Every ops
// action that mutates state (cancel, force-recover, pause-provider,
// release-hold) writes exactly one row here, always with a reason -- a
// missing reason is rejected at the usecase layer before this package is
// ever called, not silently defaulted.
package adminaudit

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Action is one recorded operator action.
type Action struct {
	OperatorID string
	Action     string
	EntityType string // "booking" | "provider"
	EntityID   string
	Reason     string
	Before     []byte // JSON, optional
	After      []byte // JSON, optional
}

type Repository interface {
	Record(ctx context.Context, a Action) error
}

type PostgresRepository struct {
	db *pgxpool.Pool
}

func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Record(ctx context.Context, a Action) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO taxi_admin_audit_events (operator_id, action, entity_type, entity_id, reason, before, after)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, a.OperatorID, a.Action, a.EntityType, a.EntityID, a.Reason, a.Before, a.After)
	return err
}
