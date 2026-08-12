package audit

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresRecorder persists audit entries to vendor_audit_log
// (migrations/000004_vendor_auth_iam.up.sql).
type PostgresRecorder struct {
	db *pgxpool.Pool
}

func NewPostgresRecorder(db *pgxpool.Pool) *PostgresRecorder {
	return &PostgresRecorder{db: db}
}

func (r *PostgresRecorder) Record(ctx context.Context, entry Entry) error {
	before, err := marshalOrNil(entry.Before)
	if err != nil {
		return err
	}
	after, err := marshalOrNil(entry.After)
	if err != nil {
		return err
	}

	_, err = r.db.Exec(ctx, `
		INSERT INTO vendor_audit_log
			(id, vendor_id, actor_type, actor_id, action, aggregate_type, aggregate_id,
			 branch_id, before_data, after_data, reason)
		VALUES ($1, NULLIF($2,''), $3, NULLIF($4,''), $5, $6, $7, NULLIF($8,''), $9, $10, NULLIF($11,''))`,
		"val_"+uuid.NewString(), entry.VendorID, entry.ActorType, entry.ActorID, entry.Action,
		entry.AggregateType, entry.AggregateID, entry.BranchID, before, after, entry.Reason,
	)
	return err
}

func marshalOrNil(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}
