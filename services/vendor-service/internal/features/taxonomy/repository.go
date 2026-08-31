package taxonomy

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Begin(ctx context.Context) (pgx.Tx, error) {
	if r == nil || r.pool == nil {
		return nil, ErrInvalidArgument
	}

	return r.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.ReadCommitted,
	})
}

func (r *Repository) GetNamespaceByCode(
	ctx context.Context,
	code NamespaceCode,
) (Namespace, error) {
	if r == nil || r.pool == nil {
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
        WHERE code = $1
    `, string(code)))
}

func (r *Repository) LockNamespaceByCode(
	ctx context.Context,
	tx pgx.Tx,
	code NamespaceCode,
) (Namespace, error) {
	if tx == nil {
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
        WHERE code = $1
        FOR UPDATE
    `, string(code)))
}

func (r *Repository) GetVersionByID(
	ctx context.Context,
	id string,
) (Version, error) {
	if r == nil || r.pool == nil || id == "" {
		return Version{}, ErrInvalidArgument
	}

	return scanVersion(r.pool.QueryRow(ctx, `
        SELECT
            id,
            namespace_id,
            version_number,
            status,
            source_version_id,
            revision,
            created_by_actor_ref,
            created_at,
            updated_at,
            published_by_actor_ref,
            published_at,
            retired_by_actor_ref,
            retired_at
        FROM canonical_taxonomy_versions
        WHERE id = $1
    `, id))
}

func (r *Repository) GetVersionByIDTx(
	ctx context.Context,
	tx pgx.Tx,
	id string,
	forUpdate bool,
) (Version, error) {
	if tx == nil || id == "" {
		return Version{}, ErrInvalidArgument
	}

	query := `
        SELECT
            id,
            namespace_id,
            version_number,
            status,
            source_version_id,
            revision,
            created_by_actor_ref,
            created_at,
            updated_at,
            published_by_actor_ref,
            published_at,
            retired_by_actor_ref,
            retired_at
        FROM canonical_taxonomy_versions
        WHERE id = $1
    `

	if forUpdate {
		query += ` FOR UPDATE`
	}

	return scanVersion(tx.QueryRow(ctx, query, id))
}

func (r *Repository) GetPublishedVersion(
	ctx context.Context,
	namespaceID string,
) (Version, error) {
	if r == nil || r.pool == nil || namespaceID == "" {
		return Version{}, ErrInvalidArgument
	}

	return scanVersion(r.pool.QueryRow(ctx, `
        SELECT
            id,
            namespace_id,
            version_number,
            status,
            source_version_id,
            revision,
            created_by_actor_ref,
            created_at,
            updated_at,
            published_by_actor_ref,
            published_at,
            retired_by_actor_ref,
            retired_at
        FROM canonical_taxonomy_versions
        WHERE namespace_id = $1
          AND status = 'PUBLISHED'
        LIMIT 1
    `, namespaceID))
}

func (r *Repository) GetPublishedVersionTx(
	ctx context.Context,
	tx pgx.Tx,
	namespaceID string,
	forUpdate bool,
) (Version, error) {
	if tx == nil || namespaceID == "" {
		return Version{}, ErrInvalidArgument
	}

	query := `
        SELECT
            id,
            namespace_id,
            version_number,
            status,
            source_version_id,
            revision,
            created_by_actor_ref,
            created_at,
            updated_at,
            published_by_actor_ref,
            published_at,
            retired_by_actor_ref,
            retired_at
        FROM canonical_taxonomy_versions
        WHERE namespace_id = $1
          AND status = 'PUBLISHED'
        LIMIT 1
    `

	if forUpdate {
		query += ` FOR UPDATE`
	}

	return scanVersion(tx.QueryRow(ctx, query, namespaceID))
}

func (r *Repository) ListVersions(
	ctx context.Context,
	namespaceID string,
) ([]Version, error) {
	if r == nil || r.pool == nil || namespaceID == "" {
		return nil, ErrInvalidArgument
	}

	rows, err := r.pool.Query(ctx, `
        SELECT
            id,
            namespace_id,
            version_number,
            status,
            source_version_id,
            revision,
            created_by_actor_ref,
            created_at,
            updated_at,
            published_by_actor_ref,
            published_at,
            retired_by_actor_ref,
            retired_at
        FROM canonical_taxonomy_versions
        WHERE namespace_id = $1
        ORDER BY version_number DESC
    `, namespaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	versions := make([]Version, 0)

	for rows.Next() {
		version, scanErr := scanVersion(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		versions = append(versions, version)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return versions, nil
}

func (r *Repository) NextVersionNumber(
	ctx context.Context,
	tx pgx.Tx,
	namespaceID string,
) (int64, error) {
	if tx == nil || namespaceID == "" {
		return 0, ErrInvalidArgument
	}

	var next int64

	err := tx.QueryRow(ctx, `
        SELECT COALESCE(MAX(version_number), 0) + 1
        FROM canonical_taxonomy_versions
        WHERE namespace_id = $1
    `, namespaceID).Scan(&next)
	if err != nil {
		return 0, err
	}

	return next, nil
}

func (r *Repository) InsertVersion(
	ctx context.Context,
	tx pgx.Tx,
	version Version,
) (Version, error) {
	if tx == nil ||
		version.ID == "" ||
		version.NamespaceID == "" ||
		version.VersionNumber <= 0 ||
		version.CreatedByActorRef == "" {
		return Version{}, ErrInvalidArgument
	}

	return scanVersion(tx.QueryRow(ctx, `
        INSERT INTO canonical_taxonomy_versions (
            id,
            namespace_id,
            version_number,
            status,
            source_version_id,
            revision,
            created_by_actor_ref
        )
        VALUES ($1, $2, $3, 'DRAFT', $4, 1, $5)
        RETURNING
            id,
            namespace_id,
            version_number,
            status,
            source_version_id,
            revision,
            created_by_actor_ref,
            created_at,
            updated_at,
            published_by_actor_ref,
            published_at,
            retired_by_actor_ref,
            retired_at
    `,
		version.ID,
		version.NamespaceID,
		version.VersionNumber,
		version.SourceVersionID,
		version.CreatedByActorRef,
	))
}

func scanNamespace(row pgx.Row) (Namespace, error) {
	var namespace Namespace
	var code string
	var status string

	err := row.Scan(
		&namespace.ID,
		&code,
		&namespace.DisplayName,
		&status,
		&namespace.Revision,
		&namespace.CreatedAt,
		&namespace.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Namespace{}, ErrNotFound
		}
		return Namespace{}, err
	}

	namespace.Code = NamespaceCode(code)
	namespace.Status = NamespaceStatus(status)

	return namespace, nil
}

func scanVersion(row pgx.Row) (Version, error) {
	var version Version
	var status string

	err := row.Scan(
		&version.ID,
		&version.NamespaceID,
		&version.VersionNumber,
		&status,
		&version.SourceVersionID,
		&version.Revision,
		&version.CreatedByActorRef,
		&version.CreatedAt,
		&version.UpdatedAt,
		&version.PublishedByActorRef,
		&version.PublishedAt,
		&version.RetiredByActorRef,
		&version.RetiredAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Version{}, ErrNotFound
		}
		return Version{}, err
	}

	version.Status = VersionStatus(status)

	return version, nil
}
