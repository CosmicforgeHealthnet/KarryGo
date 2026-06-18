package filemetadatarepositories

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	filemetadatamodels "cosmicforge/logistics/services/media-file-service/internal/features/file_metadata/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

const defaultListLimit = 100

type MediaAssetRepository interface {
	Create(ctx context.Context, input filemetadatamodels.CreateMediaAssetInput) (filemetadatamodels.MediaAsset, error)
	GetByID(ctx context.Context, id string) (filemetadatamodels.MediaAsset, error)
	List(ctx context.Context, filter filemetadatamodels.ListMediaAssetsFilter) ([]filemetadatamodels.MediaAsset, error)
	MarkDeleted(ctx context.Context, id string) error
}

type PostgresMediaAssetRepository struct {
	db *pgxpool.Pool
}

func NewPostgresMediaAssetRepository(db *pgxpool.Pool) *PostgresMediaAssetRepository {
	return &PostgresMediaAssetRepository{db: db}
}

func (r *PostgresMediaAssetRepository) Create(ctx context.Context, input filemetadatamodels.CreateMediaAssetInput) (filemetadatamodels.MediaAsset, error) {
	metadata, err := json.Marshal(input.Metadata)
	if err != nil {
		return filemetadatamodels.MediaAsset{}, apperrors.BadRequest("File metadata is invalid.", err)
	}
	if len(metadata) == 0 || string(metadata) == "null" {
		metadata = []byte("{}")
	}

	row := r.db.QueryRow(ctx, `
		INSERT INTO media_assets (
			id,
			owner_service,
			owner_id,
			purpose,
			original_filename,
			content_type,
			size_bytes,
			checksum_sha256,
			storage_bucket,
			storage_path,
			public_url,
			metadata,
			uploaded_by_service
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING
			id::text,
			owner_service,
			owner_id,
			purpose,
			original_filename,
			content_type,
			size_bytes,
			checksum_sha256,
			storage_bucket,
			storage_path,
			public_url,
			status,
			metadata,
			uploaded_by_service,
			created_at,
			updated_at,
			deleted_at
	`, input.ID, input.OwnerService, input.OwnerID, input.Purpose, input.OriginalFilename, input.ContentType, input.SizeBytes, input.ChecksumSHA256, input.StorageBucket, input.StoragePath, input.PublicURL, metadata, input.UploadedByService)

	return scanMediaAsset(row)
}

func (r *PostgresMediaAssetRepository) GetByID(ctx context.Context, id string) (filemetadatamodels.MediaAsset, error) {
	row := r.db.QueryRow(ctx, `
		SELECT
			id::text,
			owner_service,
			owner_id,
			purpose,
			original_filename,
			content_type,
			size_bytes,
			checksum_sha256,
			storage_bucket,
			storage_path,
			public_url,
			status,
			metadata,
			uploaded_by_service,
			created_at,
			updated_at,
			deleted_at
		FROM media_assets
		WHERE id = $1 AND deleted_at IS NULL
	`, id)

	asset, err := scanMediaAsset(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return filemetadatamodels.MediaAsset{}, apperrors.NotFound("Media file could not be found.", err)
	}
	return asset, err
}

func (r *PostgresMediaAssetRepository) List(ctx context.Context, filter filemetadatamodels.ListMediaAssetsFilter) ([]filemetadatamodels.MediaAsset, error) {
	limit := filter.Limit
	if limit <= 0 || limit > defaultListLimit {
		limit = defaultListLimit
	}

	rows, err := r.db.Query(ctx, `
		SELECT
			id::text,
			owner_service,
			owner_id,
			purpose,
			original_filename,
			content_type,
			size_bytes,
			checksum_sha256,
			storage_bucket,
			storage_path,
			public_url,
			status,
			metadata,
			uploaded_by_service,
			created_at,
			updated_at,
			deleted_at
		FROM media_assets
		WHERE deleted_at IS NULL
			AND ($1 = '' OR owner_service = $1)
			AND ($2 = '' OR owner_id = $2)
			AND ($3 = '' OR purpose = $3)
		ORDER BY created_at DESC
		LIMIT $4
	`, filter.OwnerService, filter.OwnerID, filter.Purpose, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	assets := []filemetadatamodels.MediaAsset{}
	for rows.Next() {
		asset, err := scanMediaAsset(rows)
		if err != nil {
			return nil, err
		}
		assets = append(assets, asset)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return assets, nil
}

func (r *PostgresMediaAssetRepository) MarkDeleted(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE media_assets
		SET status = $2,
			deleted_at = COALESCE(deleted_at, now()),
			updated_at = now()
		WHERE id = $1
	`, id, filemetadatamodels.StatusDeleted)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound("Media file could not be found.", nil)
	}

	return nil
}

type mediaAssetRow interface {
	Scan(dest ...interface{}) error
}

func scanMediaAsset(row mediaAssetRow) (filemetadatamodels.MediaAsset, error) {
	var asset filemetadatamodels.MediaAsset
	var metadata []byte
	err := row.Scan(
		&asset.ID,
		&asset.OwnerService,
		&asset.OwnerID,
		&asset.Purpose,
		&asset.OriginalFilename,
		&asset.ContentType,
		&asset.SizeBytes,
		&asset.ChecksumSHA256,
		&asset.StorageBucket,
		&asset.StoragePath,
		&asset.PublicURL,
		&asset.Status,
		&metadata,
		&asset.UploadedByService,
		&asset.CreatedAt,
		&asset.UpdatedAt,
		&asset.DeletedAt,
	)
	if err != nil {
		return filemetadatamodels.MediaAsset{}, err
	}
	if len(metadata) > 0 {
		if err := json.Unmarshal(metadata, &asset.Metadata); err != nil {
			return filemetadatamodels.MediaAsset{}, apperrors.Internal("File metadata could not be read.", err)
		}
	}
	if asset.Metadata == nil {
		asset.Metadata = map[string]interface{}{}
	}

	return asset, nil
}
