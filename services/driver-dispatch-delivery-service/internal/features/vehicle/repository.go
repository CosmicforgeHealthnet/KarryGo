package vehicle

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository is the persistence interface for the vehicle feature.
type Repository interface {
	InsertBike(ctx context.Context, providerID string, input RegisterBikeInput, isPrimary bool) (Bike, error)
	GetBikeByID(ctx context.Context, bikeID string, providerID string) (Bike, bool, error)
	GetBikeByIDAdmin(ctx context.Context, bikeID string) (Bike, bool, error)
	ListBikesByProvider(ctx context.Context, providerID string) ([]Bike, error)
	UpdateBike(ctx context.Context, bikeID string, providerID string, input UpdateBikeInput) (Bike, error)
	UpdateBikeStatus(ctx context.Context, bikeID string, status VehicleStatus) (Bike, error)
	HasAnyBike(ctx context.Context, providerID string) (bool, error)
	PlateNumberExists(ctx context.Context, plateNumber string) (bool, error)
	AdminUpdateBikeStatus(ctx context.Context, bikeID string, status VehicleStatus) (Bike, error)
	SuspendAllBikesForProvider(ctx context.Context, providerID string, reason string) error
	InsertAudit(ctx context.Context, input AuditInput) error
	InsertBikeDocument(ctx context.Context, input BikeDocument) (BikeDocument, error)
	ListBikeDocuments(ctx context.Context, bikeID string, providerID string) ([]BikeDocument, error)
}

// PostgresRepository implements Repository using pgx.
type PostgresRepository struct {
	db *pgxpool.Pool
}

func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) InsertBike(ctx context.Context, providerID string, input RegisterBikeInput, isPrimary bool) (Bike, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO bikes (
			provider_id, bike_type, brand, model, year, color, plate_number,
			engine_cc, chassis_number, is_primary
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id::text, provider_id::text, bike_type, brand, model, year, color,
			plate_number, engine_cc, chassis_number, verification_status,
			is_active, is_primary, created_at, updated_at
	`, providerID, input.BikeType, input.Brand, input.Model, input.Year,
		input.Color, input.PlateNumber, input.EngineCc, input.ChassisNumber, isPrimary)
	return scanBike(row)
}

func (r *PostgresRepository) GetBikeByID(ctx context.Context, bikeID string, providerID string) (Bike, bool, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id::text, provider_id::text, bike_type, brand, model, year, color,
			plate_number, engine_cc, chassis_number, verification_status,
			is_active, is_primary, created_at, updated_at
		FROM bikes
		WHERE id = $1 AND provider_id = $2
	`, bikeID, providerID)
	bike, err := scanBike(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Bike{}, false, nil
	}
	return bike, err == nil, err
}

func (r *PostgresRepository) ListBikesByProvider(ctx context.Context, providerID string) ([]Bike, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, provider_id::text, bike_type, brand, model, year, color,
			plate_number, engine_cc, chassis_number, verification_status,
			is_active, is_primary, created_at, updated_at
		FROM bikes
		WHERE provider_id = $1
		ORDER BY is_primary DESC, created_at ASC
	`, providerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bikes []Bike
	for rows.Next() {
		b, err := scanBike(rows)
		if err != nil {
			return nil, err
		}
		bikes = append(bikes, b)
	}
	return bikes, rows.Err()
}

func (r *PostgresRepository) UpdateBike(ctx context.Context, bikeID string, providerID string, input UpdateBikeInput) (Bike, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE bikes
		SET brand          = COALESCE($3, brand),
		    model          = COALESCE($4, model),
		    year           = COALESCE($5, year),
		    color          = COALESCE($6, color),
		    engine_cc      = COALESCE($7, engine_cc),
		    chassis_number = COALESCE($8, chassis_number),
		    updated_at     = now()
		WHERE id = $1 AND provider_id = $2
		RETURNING id::text, provider_id::text, bike_type, brand, model, year, color,
			plate_number, engine_cc, chassis_number, verification_status,
			is_active, is_primary, created_at, updated_at
	`, bikeID, providerID,
		input.Brand, input.Model, input.Year, input.Color,
		input.EngineCc, input.ChassisNumber)
	return scanBike(row)
}

func (r *PostgresRepository) HasAnyBike(ctx context.Context, providerID string) (bool, error) {
	row := r.db.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM bikes WHERE provider_id = $1)
	`, providerID)
	var exists bool
	err := row.Scan(&exists)
	return exists, err
}

func (r *PostgresRepository) PlateNumberExists(ctx context.Context, plateNumber string) (bool, error) {
	row := r.db.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM bikes WHERE plate_number = $1)
	`, plateNumber)
	var exists bool
	err := row.Scan(&exists)
	return exists, err
}

func (r *PostgresRepository) GetBikeByIDAdmin(ctx context.Context, bikeID string) (Bike, bool, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id::text, provider_id::text, bike_type, brand, model, year, color,
			plate_number, engine_cc, chassis_number, verification_status,
			is_active, is_primary, created_at, updated_at
		FROM bikes
		WHERE id = $1
	`, bikeID)
	bike, err := scanBike(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Bike{}, false, nil
	}
	return bike, err == nil, err
}

func (r *PostgresRepository) UpdateBikeStatus(ctx context.Context, bikeID string, status VehicleStatus) (Bike, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE bikes
		SET verification_status = $2,
		    updated_at          = now()
		WHERE id = $1
		RETURNING id::text, provider_id::text, bike_type, brand, model, year, color,
			plate_number, engine_cc, chassis_number, verification_status,
			is_active, is_primary, created_at, updated_at
	`, bikeID, status)
	return scanBike(row)
}

func (r *PostgresRepository) AdminUpdateBikeStatus(ctx context.Context, bikeID string, status VehicleStatus) (Bike, error) {
	// is_active = false only when suspending; true otherwise.
	isActive := status != VehicleSuspended
	row := r.db.QueryRow(ctx, `
		UPDATE bikes
		SET verification_status = $2,
		    is_active           = $3,
		    updated_at          = now()
		WHERE id = $1
		RETURNING id::text, provider_id::text, bike_type, brand, model, year, color,
			plate_number, engine_cc, chassis_number, verification_status,
			is_active, is_primary, created_at, updated_at
	`, bikeID, status, isActive)
	return scanBike(row)
}

// SuspendAllBikesForProvider sets is_active=false and verification_status=suspended
// for every bike owned by the given provider. It also inserts a bike_audit row per bike.
func (r *PostgresRepository) SuspendAllBikesForProvider(ctx context.Context, providerID string, reason string) error {
	// Fetch all bikes so we can create audit rows per-bike.
	rows, err := r.db.Query(ctx, `
		SELECT id::text, verification_status
		FROM bikes
		WHERE provider_id = $1 AND verification_status != 'suspended'
	`, providerID)
	if err != nil {
		return err
	}
	type bikeRow struct {
		id     string
		status VehicleStatus
	}
	var bikes []bikeRow
	for rows.Next() {
		var br bikeRow
		if err := rows.Scan(&br.id, &br.status); err != nil {
			rows.Close()
			return err
		}
		bikes = append(bikes, br)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	if len(bikes) == 0 {
		return nil
	}

	// Bulk update.
	_, err = r.db.Exec(ctx, `
		UPDATE bikes
		SET verification_status = 'suspended',
		    is_active           = false,
		    updated_at          = now()
		WHERE provider_id = $1 AND verification_status != 'suspended'
	`, providerID)
	if err != nil {
		return err
	}

	// Insert audit rows.
	notePtr := &reason
	if reason == "" {
		notePtr = nil
	}
	for _, br := range bikes {
		_, _ = r.db.Exec(ctx, `
			INSERT INTO bike_audit (
				bike_id, provider_id, action, from_status, to_status, performed_by, notes
			) VALUES ($1, $2, $3, $4, $5, NULL, $6)
		`, br.id, providerID, AuditSuspended, br.status, VehicleSuspended, notePtr)
	}
	return nil
}

func (r *PostgresRepository) InsertAudit(ctx context.Context, input AuditInput) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO bike_audit (
			bike_id, provider_id, action, from_status, to_status, performed_by, notes
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, input.BikeID, input.ProviderID, input.Action,
		input.FromStatus, input.ToStatus, input.PerformedBy, input.Notes)
	return err
}

func (r *PostgresRepository) InsertBikeDocument(ctx context.Context, doc BikeDocument) (BikeDocument, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO bike_documents (
			bike_id, provider_id, document_type, file_url, file_size, mime_type, expiry_date
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id::text, bike_id::text, provider_id::text, document_type,
			file_url, file_size, mime_type,
			to_char(expiry_date, 'YYYY-MM-DD'), uploaded_at
	`, doc.BikeID, doc.ProviderID, doc.DocumentType,
		doc.FileURL, doc.FileSize, doc.MimeType, doc.ExpiryDate)
	return scanDocument(row)
}

func (r *PostgresRepository) ListBikeDocuments(ctx context.Context, bikeID string, providerID string) ([]BikeDocument, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, bike_id::text, provider_id::text, document_type,
			file_url, file_size, mime_type,
			to_char(expiry_date, 'YYYY-MM-DD'), uploaded_at
		FROM bike_documents
		WHERE bike_id = $1 AND provider_id = $2
		ORDER BY uploaded_at DESC
	`, bikeID, providerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docs []BikeDocument
	for rows.Next() {
		d, err := scanDocument(rows)
		if err != nil {
			return nil, err
		}
		docs = append(docs, d)
	}
	return docs, rows.Err()
}

// ── Scanners ──────────────────────────────────────────────────────────────────

type scanner interface {
	Scan(dest ...any) error
}

func scanBike(row scanner) (Bike, error) {
	var b Bike
	var engineCc pgtype.Int4
	var chassisNumber pgtype.Text

	err := row.Scan(
		&b.ID, &b.ProviderID, &b.BikeType, &b.Brand, &b.Model,
		&b.Year, &b.Color, &b.PlateNumber,
		&engineCc, &chassisNumber,
		&b.VerificationStatus, &b.IsActive, &b.IsPrimary,
		&b.CreatedAt, &b.UpdatedAt,
	)
	if err != nil {
		return Bike{}, err
	}
	if engineCc.Valid {
		v := int(engineCc.Int32)
		b.EngineCc = &v
	}
	if chassisNumber.Valid {
		b.ChassisNumber = &chassisNumber.String
	}
	return b, nil
}

func scanDocument(row scanner) (BikeDocument, error) {
	var d BikeDocument
	var fileSize pgtype.Int4
	var mimeType pgtype.Text
	var expiryDate pgtype.Text

	err := row.Scan(
		&d.ID, &d.BikeID, &d.ProviderID, &d.DocumentType,
		&d.FileURL, &fileSize, &mimeType, &expiryDate, &d.UploadedAt,
	)
	if err != nil {
		return BikeDocument{}, err
	}
	if fileSize.Valid {
		v := int(fileSize.Int32)
		d.FileSize = &v
	}
	if mimeType.Valid {
		d.MimeType = &mimeType.String
	}
	if expiryDate.Valid {
		d.ExpiryDate = &expiryDate.String
	}
	return d, nil
}
