package taxonomy

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

func (r *Repository) GetMappingByID(
	ctx context.Context,
	id string,
) (TaxonomyMapping, error) {
	var mapping TaxonomyMapping

	err := r.pool.QueryRow(
		ctx,
		`
        SELECT
            id,
            namespace_id,
            version_id,
            node_id,
            source_type,
            source_ref,
            mapping_type,
            status,
            confidence_basis,
            confidence_score,
            business_ref,
            branch_ref,
            metadata,
            revision,
            created_by_actor_ref,
            updated_by_actor_ref,
            created_at,
            updated_at
          FROM canonical_taxonomy_mappings
         WHERE id=$1
        `,
		id,
	).Scan(
		&mapping.ID,
		&mapping.NamespaceID,
		&mapping.VersionID,
		&mapping.NodeID,
		&mapping.SourceType,
		&mapping.SourceRef,
		&mapping.MappingType,
		&mapping.Status,
		&mapping.ConfidenceBasis,
		&mapping.ConfidenceScore,
		&mapping.BusinessRef,
		&mapping.BranchRef,
		&mapping.Metadata,
		&mapping.Revision,
		&mapping.CreatedByActorRef,
		&mapping.UpdatedByActorRef,
		&mapping.CreatedAt,
		&mapping.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return TaxonomyMapping{}, ErrNotFound
	}

	return mapping, err
}

func (r *Repository) ListMappingsForSource(
	ctx context.Context,
	namespaceID string,
	sourceType string,
	sourceRef string,
) ([]TaxonomyMapping, error) {
	rows, err := r.pool.Query(
		ctx,
		`
        SELECT
            id,
            namespace_id,
            version_id,
            node_id,
            source_type,
            source_ref,
            mapping_type,
            status,
            confidence_basis,
            confidence_score,
            business_ref,
            branch_ref,
            metadata,
            revision,
            created_by_actor_ref,
            updated_by_actor_ref,
            created_at,
            updated_at
          FROM canonical_taxonomy_mappings
         WHERE namespace_id=$1
           AND source_type=$2
           AND source_ref=$3
           AND status='ACTIVE'
         ORDER BY
            CASE mapping_type
                WHEN 'PRIMARY' THEN 1
                WHEN 'SECONDARY' THEN 2
                ELSE 3
            END,
            created_at,
            id
        `,
		namespaceID,
		sourceType,
		sourceRef,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []TaxonomyMapping

	for rows.Next() {
		var mapping TaxonomyMapping

		if err = rows.Scan(
			&mapping.ID,
			&mapping.NamespaceID,
			&mapping.VersionID,
			&mapping.NodeID,
			&mapping.SourceType,
			&mapping.SourceRef,
			&mapping.MappingType,
			&mapping.Status,
			&mapping.ConfidenceBasis,
			&mapping.ConfidenceScore,
			&mapping.BusinessRef,
			&mapping.BranchRef,
			&mapping.Metadata,
			&mapping.Revision,
			&mapping.CreatedByActorRef,
			&mapping.UpdatedByActorRef,
			&mapping.CreatedAt,
			&mapping.UpdatedAt,
		); err != nil {
			return nil, err
		}

		result = append(result, mapping)
	}

	return result, rows.Err()
}
