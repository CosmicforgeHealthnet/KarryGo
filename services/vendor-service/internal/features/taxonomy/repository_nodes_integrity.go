package taxonomy

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// HasActiveChildrenTx protects discovery hierarchy integrity when archiving
// a parent node. A parent cannot become inactive while active direct children
// still depend on it.
func (r *Repository) HasActiveChildrenTx(
	ctx context.Context,
	tx pgx.Tx,
	nodeID string,
) (bool, error) {
	if tx == nil || nodeID == "" {
		return false, ErrInvalidArgument
	}

	var exists bool

	err := tx.QueryRow(ctx, `
        SELECT EXISTS(
            SELECT 1
            FROM canonical_taxonomy_nodes
            WHERE parent_id = $1
              AND status = 'ACTIVE'
        )
    `, nodeID).Scan(&exists)
	if err != nil {
		return false, err
	}

	return exists, nil
}
