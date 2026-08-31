package taxonomy

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

// -----------------------------------------------------------------------------
// Facets
// -----------------------------------------------------------------------------

func (r *Repository) GetFacetByID(
	ctx context.Context,
	id string,
) (Facet, error) {
	if r == nil || r.pool == nil || id == "" {
		return Facet{}, ErrInvalidArgument
	}

	return scanFacet(r.pool.QueryRow(ctx, facetSelect+`
        WHERE id = $1
    `, id))
}

func (r *Repository) GetFacetByIDTx(
	ctx context.Context,
	tx pgx.Tx,
	id string,
	forUpdate bool,
) (Facet, error) {
	if tx == nil || id == "" {
		return Facet{}, ErrInvalidArgument
	}

	query := facetSelect + ` WHERE id = $1`

	if forUpdate {
		query += ` FOR UPDATE`
	}

	return scanFacet(tx.QueryRow(ctx, query, id))
}

func (r *Repository) ListFacets(
	ctx context.Context,
	versionID string,
) ([]Facet, error) {
	if r == nil || r.pool == nil || versionID == "" {
		return nil, ErrInvalidArgument
	}

	rows, err := r.pool.Query(ctx, facetSelect+`
        WHERE version_id = $1
        ORDER BY position ASC, code ASC, id ASC
    `, versionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return collectFacets(rows)
}

func (r *Repository) ListFacetsTx(
	ctx context.Context,
	tx pgx.Tx,
	versionID string,
) ([]Facet, error) {
	if tx == nil || versionID == "" {
		return nil, ErrInvalidArgument
	}

	rows, err := tx.Query(ctx, facetSelect+`
        WHERE version_id = $1
        ORDER BY position ASC, code ASC, id ASC
    `, versionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return collectFacets(rows)
}

func (r *Repository) InsertFacet(
	ctx context.Context,
	tx pgx.Tx,
	facet Facet,
) (Facet, error) {
	if tx == nil ||
		facet.ID == "" ||
		facet.NamespaceID == "" ||
		facet.VersionID == "" ||
		facet.Code == "" ||
		facet.DisplayName == "" {
		return Facet{}, ErrInvalidArgument
	}

	metadata, err := normalizeMetadata(facet.Metadata)
	if err != nil {
		return Facet{}, err
	}

	structureType := facet.StructureType
	if structureType == "" {
		structureType = FacetStructureFlat
	}

	status := facet.Status
	if status == "" {
		status = RecordStatusActive
	}

	return scanFacet(tx.QueryRow(ctx, `
        INSERT INTO canonical_taxonomy_facets (
            id,
            namespace_id,
            version_id,
            code,
            display_name,
            description,
            structure_type,
            position,
            status,
            metadata,
            revision,
            archived_at
        )
        VALUES (
            $1, $2, $3, $4, $5, $6, $7, $8, $9,
            $10::jsonb,
            1,
            CASE WHEN $9 = 'ARCHIVED' THEN NOW() ELSE NULL END
        )
        RETURNING
            id,
            namespace_id,
            version_id,
            code,
            display_name,
            description,
            structure_type,
            position,
            status,
            metadata,
            revision,
            created_at,
            updated_at,
            archived_at
    `,
		facet.ID,
		facet.NamespaceID,
		facet.VersionID,
		facet.Code,
		facet.DisplayName,
		facet.Description,
		string(structureType),
		facet.Position,
		string(status),
		metadata,
	))
}

func (r *Repository) UpdateFacet(
	ctx context.Context,
	tx pgx.Tx,
	facet Facet,
	expectedRevision int64,
) (Facet, error) {
	if tx == nil ||
		facet.ID == "" ||
		facet.Code == "" ||
		facet.DisplayName == "" ||
		expectedRevision <= 0 {
		return Facet{}, ErrInvalidArgument
	}

	metadata, err := normalizeMetadata(facet.Metadata)
	if err != nil {
		return Facet{}, err
	}

	updated, err := scanFacet(tx.QueryRow(ctx, `
        UPDATE canonical_taxonomy_facets
        SET
            code = $2,
            display_name = $3,
            description = $4,
            structure_type = $5,
            position = $6,
            status = $7,
            metadata = $8::jsonb,
            revision = revision + 1,
            updated_at = NOW(),
            archived_at = CASE
                WHEN $7 = 'ARCHIVED'
                    THEN COALESCE(archived_at, NOW())
                ELSE NULL
            END
        WHERE id = $1
          AND revision = $9
        RETURNING
            id,
            namespace_id,
            version_id,
            code,
            display_name,
            description,
            structure_type,
            position,
            status,
            metadata,
            revision,
            created_at,
            updated_at,
            archived_at
    `,
		facet.ID,
		facet.Code,
		facet.DisplayName,
		facet.Description,
		string(facet.StructureType),
		facet.Position,
		string(facet.Status),
		metadata,
		expectedRevision,
	))
	if !errors.Is(err, ErrNotFound) {
		return updated, err
	}

	return Facet{}, r.classifyFacetWriteFailure(ctx, tx, facet.ID)
}

// -----------------------------------------------------------------------------
// Nodes
// -----------------------------------------------------------------------------

func (r *Repository) GetNodeByID(
	ctx context.Context,
	id string,
) (Node, error) {
	if r == nil || r.pool == nil || id == "" {
		return Node{}, ErrInvalidArgument
	}

	return scanNode(r.pool.QueryRow(ctx, nodeSelect+`
        WHERE id = $1
    `, id))
}

func (r *Repository) GetNodeByIDTx(
	ctx context.Context,
	tx pgx.Tx,
	id string,
	forUpdate bool,
) (Node, error) {
	if tx == nil || id == "" {
		return Node{}, ErrInvalidArgument
	}

	query := nodeSelect + ` WHERE id = $1`

	if forUpdate {
		query += ` FOR UPDATE`
	}

	return scanNode(tx.QueryRow(ctx, query, id))
}

func (r *Repository) ListNodes(
	ctx context.Context,
	versionID string,
) ([]Node, error) {
	if r == nil || r.pool == nil || versionID == "" {
		return nil, ErrInvalidArgument
	}

	rows, err := r.pool.Query(ctx, nodeSelect+`
        WHERE version_id = $1
        ORDER BY facet_id ASC, position ASC, canonical_code ASC, id ASC
    `, versionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return collectNodes(rows)
}

func (r *Repository) ListNodesTx(
	ctx context.Context,
	tx pgx.Tx,
	versionID string,
) ([]Node, error) {
	if tx == nil || versionID == "" {
		return nil, ErrInvalidArgument
	}

	rows, err := tx.Query(ctx, nodeSelect+`
        WHERE version_id = $1
        ORDER BY facet_id ASC, position ASC, canonical_code ASC, id ASC
    `, versionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return collectNodes(rows)
}

func (r *Repository) ListNodesByFacet(
	ctx context.Context,
	versionID string,
	facetID string,
) ([]Node, error) {
	if r == nil ||
		r.pool == nil ||
		versionID == "" ||
		facetID == "" {
		return nil, ErrInvalidArgument
	}

	rows, err := r.pool.Query(ctx, nodeSelect+`
        WHERE version_id = $1
          AND facet_id = $2
        ORDER BY position ASC, canonical_code ASC, id ASC
    `, versionID, facetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return collectNodes(rows)
}

func (r *Repository) InsertNode(
	ctx context.Context,
	tx pgx.Tx,
	node Node,
) (Node, error) {
	if tx == nil ||
		node.ID == "" ||
		node.NamespaceID == "" ||
		node.VersionID == "" ||
		node.FacetID == "" ||
		node.CanonicalCode == "" ||
		node.DisplayName == "" {
		return Node{}, ErrInvalidArgument
	}

	metadata, err := normalizeMetadata(node.Metadata)
	if err != nil {
		return Node{}, err
	}

	status := node.Status
	if status == "" {
		status = RecordStatusActive
	}

	return scanNode(tx.QueryRow(ctx, `
        INSERT INTO canonical_taxonomy_nodes (
            id,
            namespace_id,
            version_id,
            facet_id,
            canonical_code,
            display_name,
            description,
            parent_id,
            position,
            status,
            metadata,
            revision,
            archived_at
        )
        VALUES (
            $1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
            $11::jsonb,
            1,
            CASE WHEN $10 = 'ARCHIVED' THEN NOW() ELSE NULL END
        )
        RETURNING
            id,
            namespace_id,
            version_id,
            facet_id,
            canonical_code,
            display_name,
            description,
            parent_id,
            position,
            status,
            metadata,
            revision,
            created_at,
            updated_at,
            archived_at
    `,
		node.ID,
		node.NamespaceID,
		node.VersionID,
		node.FacetID,
		node.CanonicalCode,
		node.DisplayName,
		node.Description,
		node.ParentID,
		node.Position,
		string(status),
		metadata,
	))
}

func (r *Repository) UpdateNode(
	ctx context.Context,
	tx pgx.Tx,
	node Node,
	expectedRevision int64,
) (Node, error) {
	if tx == nil ||
		node.ID == "" ||
		node.CanonicalCode == "" ||
		node.DisplayName == "" ||
		expectedRevision <= 0 {
		return Node{}, ErrInvalidArgument
	}

	metadata, err := normalizeMetadata(node.Metadata)
	if err != nil {
		return Node{}, err
	}

	updated, err := scanNode(tx.QueryRow(ctx, `
        UPDATE canonical_taxonomy_nodes
        SET
            canonical_code = $2,
            display_name = $3,
            description = $4,
            parent_id = $5,
            position = $6,
            status = $7,
            metadata = $8::jsonb,
            revision = revision + 1,
            updated_at = NOW(),
            archived_at = CASE
                WHEN $7 = 'ARCHIVED'
                    THEN COALESCE(archived_at, NOW())
                ELSE NULL
            END
        WHERE id = $1
          AND revision = $9
        RETURNING
            id,
            namespace_id,
            version_id,
            facet_id,
            canonical_code,
            display_name,
            description,
            parent_id,
            position,
            status,
            metadata,
            revision,
            created_at,
            updated_at,
            archived_at
    `,
		node.ID,
		node.CanonicalCode,
		node.DisplayName,
		node.Description,
		node.ParentID,
		node.Position,
		string(node.Status),
		metadata,
		expectedRevision,
	))
	if !errors.Is(err, ErrNotFound) {
		return updated, err
	}

	return Node{}, r.classifyNodeWriteFailure(ctx, tx, node.ID)
}

// -----------------------------------------------------------------------------
// Scanning / optimistic concurrency helpers
// -----------------------------------------------------------------------------

const facetSelect = `
    SELECT
        id,
        namespace_id,
        version_id,
        code,
        display_name,
        description,
        structure_type,
        position,
        status,
        metadata,
        revision,
        created_at,
        updated_at,
        archived_at
    FROM canonical_taxonomy_facets
`

const nodeSelect = `
    SELECT
        id,
        namespace_id,
        version_id,
        facet_id,
        canonical_code,
        display_name,
        description,
        parent_id,
        position,
        status,
        metadata,
        revision,
        created_at,
        updated_at,
        archived_at
    FROM canonical_taxonomy_nodes
`

func scanFacet(row pgx.Row) (Facet, error) {
	var facet Facet
	var structureType string
	var status string

	err := row.Scan(
		&facet.ID,
		&facet.NamespaceID,
		&facet.VersionID,
		&facet.Code,
		&facet.DisplayName,
		&facet.Description,
		&structureType,
		&facet.Position,
		&status,
		&facet.Metadata,
		&facet.Revision,
		&facet.CreatedAt,
		&facet.UpdatedAt,
		&facet.ArchivedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Facet{}, ErrNotFound
		}
		return Facet{}, err
	}

	facet.StructureType = FacetStructureType(structureType)
	facet.Status = RecordStatus(status)

	return facet, nil
}

func scanNode(row pgx.Row) (Node, error) {
	var node Node
	var status string

	err := row.Scan(
		&node.ID,
		&node.NamespaceID,
		&node.VersionID,
		&node.FacetID,
		&node.CanonicalCode,
		&node.DisplayName,
		&node.Description,
		&node.ParentID,
		&node.Position,
		&status,
		&node.Metadata,
		&node.Revision,
		&node.CreatedAt,
		&node.UpdatedAt,
		&node.ArchivedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Node{}, ErrNotFound
		}
		return Node{}, err
	}

	node.Status = RecordStatus(status)

	return node, nil
}

func collectFacets(rows pgx.Rows) ([]Facet, error) {
	facets := make([]Facet, 0)

	for rows.Next() {
		facet, err := scanFacet(rows)
		if err != nil {
			return nil, err
		}

		facets = append(facets, facet)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return facets, nil
}

func collectNodes(rows pgx.Rows) ([]Node, error) {
	nodes := make([]Node, 0)

	for rows.Next() {
		node, err := scanNode(rows)
		if err != nil {
			return nil, err
		}

		nodes = append(nodes, node)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return nodes, nil
}

func normalizeMetadata(metadata json.RawMessage) (string, error) {
	if len(metadata) == 0 {
		return `{}`, nil
	}

	if !json.Valid(metadata) {
		return "", ErrInvalidArgument
	}

	var object map[string]any
	if err := json.Unmarshal(metadata, &object); err != nil || object == nil {
		return "", ErrInvalidArgument
	}

	return string(metadata), nil
}

func (r *Repository) classifyFacetWriteFailure(
	ctx context.Context,
	tx pgx.Tx,
	id string,
) error {
	var exists bool

	if err := tx.QueryRow(ctx, `
        SELECT EXISTS(
            SELECT 1
            FROM canonical_taxonomy_facets
            WHERE id = $1
        )
    `, id).Scan(&exists); err != nil {
		return err
	}

	if !exists {
		return ErrNotFound
	}

	return ErrVersionMismatch
}

func (r *Repository) classifyNodeWriteFailure(
	ctx context.Context,
	tx pgx.Tx,
	id string,
) error {
	var exists bool

	if err := tx.QueryRow(ctx, `
        SELECT EXISTS(
            SELECT 1
            FROM canonical_taxonomy_nodes
            WHERE id = $1
        )
    `, id).Scan(&exists); err != nil {
		return err
	}

	if !exists {
		return ErrNotFound
	}

	return ErrVersionMismatch
}
