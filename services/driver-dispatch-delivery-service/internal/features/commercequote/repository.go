package commercequote

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository persists issued quotes in Postgres.
type Repository struct {
	db *pgxpool.Pool
}

// NewRepository builds the store.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

const quoteColumns = `id, caller_service, quote_reference, revision,
	request_fingerprint, vehicle_class,
	pickup_latitude, pickup_longitude, dropoff_latitude, dropoff_longitude,
	package_count, total_weight_grams,
	serviceable, reason_code, distance_km, estimated_minutes,
	fee_minor, currency, base_minor, distance_minor, surcharge_minor,
	available_providers, expires_at, created_at`

// Upsert stores a quote idempotently on (caller_service, quote_reference).
//
// The whole decision is one statement on purpose. A read-then-write would let
// two concurrent asks for the same reference each believe they were first and
// issue two different quotes for it; the ON CONFLICT below makes the winner
// deterministic and the loser read back the winner's row.
//
// The WHERE clause on the update is what implements idempotency: an identical
// fingerprint leaves the stored row completely untouched — same id, same
// revision, same expiry — so a retry returns the same quote rather than a
// freshly-dated copy of it.
func (r *Repository) Upsert(
	ctx context.Context,
	record Record,
) (Record, error) {
	if r == nil || r.db == nil {
		return Record{}, ErrUnavailable
	}

	row := r.db.QueryRow(ctx, `
		INSERT INTO commerce_delivery_quotes (`+quoteColumns+`)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,
		        $19,$20,$21,$22,$23, now())
		ON CONFLICT (caller_service, quote_reference) DO UPDATE
		   SET revision = commerce_delivery_quotes.revision + 1,
		       request_fingerprint = EXCLUDED.request_fingerprint,
		       vehicle_class = EXCLUDED.vehicle_class,
		       pickup_latitude = EXCLUDED.pickup_latitude,
		       pickup_longitude = EXCLUDED.pickup_longitude,
		       dropoff_latitude = EXCLUDED.dropoff_latitude,
		       dropoff_longitude = EXCLUDED.dropoff_longitude,
		       package_count = EXCLUDED.package_count,
		       total_weight_grams = EXCLUDED.total_weight_grams,
		       serviceable = EXCLUDED.serviceable,
		       reason_code = EXCLUDED.reason_code,
		       distance_km = EXCLUDED.distance_km,
		       estimated_minutes = EXCLUDED.estimated_minutes,
		       fee_minor = EXCLUDED.fee_minor,
		       currency = EXCLUDED.currency,
		       base_minor = EXCLUDED.base_minor,
		       distance_minor = EXCLUDED.distance_minor,
		       surcharge_minor = EXCLUDED.surcharge_minor,
		       available_providers = EXCLUDED.available_providers,
		       expires_at = EXCLUDED.expires_at,
		       updated_at = now()
		 WHERE commerce_delivery_quotes.request_fingerprint
		       IS DISTINCT FROM EXCLUDED.request_fingerprint
		RETURNING `+quoteColumns,
		record.ID, record.CallerService, record.QuoteReference, record.Revision,
		record.RequestFingerprint, record.VehicleClass,
		record.PickupLatitude, record.PickupLongitude,
		record.DropoffLatitude, record.DropoffLongitude,
		record.PackageCount, record.TotalWeightGrams,
		record.Serviceable, record.ReasonCode,
		record.DistanceKm, record.EstimatedMinutes,
		record.FeeMinor, record.Currency,
		record.BaseMinor, record.DistanceMinor, record.SurchargeMinor,
		record.AvailableProviders, record.ExpiresAt,
	)

	stored, err := scanQuote(row)

	if err == nil {
		return stored, nil
	}

	// No row came back, which means the conflict target matched and the update
	// predicate refused: the caller re-asked with an identical fingerprint.
	// The already-stored answer is the correct one to return.
	existing, readErr := r.get(
		ctx, record.CallerService, record.QuoteReference,
	)
	if readErr != nil {
		return Record{}, err
	}

	return existing, nil
}

func (r *Repository) get(
	ctx context.Context,
	caller, reference string,
) (Record, error) {
	return scanQuote(r.db.QueryRow(ctx, `
		SELECT `+quoteColumns+`
		  FROM commerce_delivery_quotes
		 WHERE caller_service = $1 AND quote_reference = $2`,
		caller, reference,
	))
}

type scanner interface {
	Scan(dest ...any) error
}

func scanQuote(row scanner) (Record, error) {
	var (
		record    Record
		expiresAt time.Time
		createdAt time.Time
	)

	err := row.Scan(
		&record.ID, &record.CallerService, &record.QuoteReference,
		&record.Revision, &record.RequestFingerprint, &record.VehicleClass,
		&record.PickupLatitude, &record.PickupLongitude,
		&record.DropoffLatitude, &record.DropoffLongitude,
		&record.PackageCount, &record.TotalWeightGrams,
		&record.Serviceable, &record.ReasonCode,
		&record.DistanceKm, &record.EstimatedMinutes,
		&record.FeeMinor, &record.Currency,
		&record.BaseMinor, &record.DistanceMinor, &record.SurchargeMinor,
		&record.AvailableProviders, &expiresAt, &createdAt,
	)
	if err != nil {
		return Record{}, err
	}

	record.ExpiresAt = expiresAt
	record.CreatedAt = createdAt

	return record, nil
}
