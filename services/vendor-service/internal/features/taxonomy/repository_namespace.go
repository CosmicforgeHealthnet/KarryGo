package taxonomy

import "context"

func (r *Repository) GetNamespaceByID(
	ctx context.Context,
	id string,
) (Namespace, error) {
	if r == nil || r.pool == nil || id == "" {
		return Namespace{}, ErrInvalidArgument
	}

	return scanNamespace(r.pool.QueryRow(ctx, `
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
    `, id))
}
