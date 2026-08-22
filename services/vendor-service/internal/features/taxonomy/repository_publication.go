package taxonomy

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

func (r *Repository) LockNamespaceByIDTx(
	ctx context.Context,
	tx pgx.Tx,
	id string,
) (Namespace, error) {
	if tx == nil || id == "" {
		return Namespace{}, ErrInvalidArgument
	}

	return scanNamespace(tx.QueryRow(ctx, `
        SELECT
            id,
            code,
            display_name,
            status,
            revision,
            created_at,
            updated_at
        FROM canonical_taxonomy_namespaces
        WHERE id = $1
        FOR UPDATE
    `, id))
}

func (r *Repository) ListPublicationHistory(
	ctx context.Context,
	namespaceID string,
) ([]PublicationHistoryRecord, error) {
	if r == nil || r.pool == nil || namespaceID == "" {
		return nil, ErrInvalidArgument
	}

	rows, err := r.pool.Query(ctx, `
        SELECT
            id,
            namespace_id,
            version_id,
            previous_version_id,
            event_type,
            actor_ref,
            command_ref,
            correlation_id,
            causation_id,
            metadata,
            occurred_at
        FROM canonical_taxonomy_publication_history
        WHERE namespace_id = $1
        ORDER BY occurred_at DESC, id DESC
    `, namespaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]PublicationHistoryRecord, 0)

	for rows.Next() {
		var item PublicationHistoryRecord

		if err = rows.Scan(
			&item.ID,
			&item.NamespaceID,
			&item.VersionID,
			&item.PreviousVersionID,
			&item.EventType,
			&item.ActorRef,
			&item.CommandRef,
			&item.CorrelationID,
			&item.CausationID,
			&item.Metadata,
			&item.OccurredAt,
		); err != nil {
			return nil, err
		}

		result = append(result, item)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func (r *Repository) PublishedVersionExistsTx(
	ctx context.Context,
	tx pgx.Tx,
	namespaceID string,
) (bool, error) {
	if tx == nil || namespaceID == "" {
		return false, ErrInvalidArgument
	}

	var exists bool

	err := tx.QueryRow(ctx, `
        SELECT EXISTS(
            SELECT 1
            FROM canonical_taxonomy_versions
            WHERE namespace_id = $1
              AND status = 'PUBLISHED'
        )
    `, namespaceID).Scan(&exists)

	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}

	return exists, err
}
