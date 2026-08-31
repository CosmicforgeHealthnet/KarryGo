package commercequote

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
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

const multiStopColumns = `id,caller_service,quote_reference,revision,
	request_fingerprint,request_snapshot,route_snapshot,serviceable,reason_code,
	supply_state,mode,service_class,capability_version,route_provider,
	route_evidence_version,distance_meters,estimated_seconds,fee_minor,currency,
	available_providers,expires_at,created_at`

// UpsertMultiStop stores the versioned route-aware answer in the same quote
// authority table as legacy single-stop quotes. Identical retries return the
// exact committed row; changed semantics advance the revision atomically.
func (r *Repository) UpsertMultiStop(ctx context.Context, record MultiStopRecord) (MultiStopRecord, error) {
	if r == nil || r.db == nil {
		return MultiStopRecord{}, ErrUnavailable
	}
	var request struct {
		PickupStops []struct {
			Location struct {
				Latitude  float64 `json:"latitude"`
				Longitude float64 `json:"longitude"`
			} `json:"location"`
		} `json:"pickup_stops"`
		Receiver struct {
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		} `json:"receiver"`
		Parcels []struct {
			WeightGrams int64 `json:"weight_grams"`
		} `json:"parcels"`
	}
	_ = json.Unmarshal(record.RequestSnapshot, &request)
	var packageCount, weight int64
	for _, parcel := range request.Parcels {
		packageCount++
		weight += parcel.WeightGrams
	}
	if packageCount == 0 {
		packageCount = 1
	}
	var pickupLat, pickupLng float64
	if len(request.PickupStops) > 0 {
		pickupLat = request.PickupStops[0].Location.Latitude
		pickupLng = request.PickupStops[0].Location.Longitude
	}
	row := r.db.QueryRow(ctx, `
		INSERT INTO commerce_delivery_quotes(
			id,caller_service,quote_reference,revision,request_fingerprint,vehicle_class,
			pickup_latitude,pickup_longitude,dropoff_latitude,dropoff_longitude,
			package_count,total_weight_grams,serviceable,reason_code,distance_km,
			estimated_minutes,fee_minor,currency,base_minor,distance_minor,surcharge_minor,
			available_providers,expires_at,contract_version,request_snapshot,route_snapshot,
			supply_state,mode,service_class,capability_version,route_provider,
			route_evidence_version,distance_meters,estimated_seconds)
		VALUES($1,$2,$3,$4,$5,'MOTORCYCLE',$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,
			$16,$17,0,0,0,$18,$19,1,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29)
		ON CONFLICT(caller_service,quote_reference) DO UPDATE SET
			revision=commerce_delivery_quotes.revision+1,
			request_fingerprint=EXCLUDED.request_fingerprint,
			pickup_latitude=EXCLUDED.pickup_latitude,pickup_longitude=EXCLUDED.pickup_longitude,
			dropoff_latitude=EXCLUDED.dropoff_latitude,dropoff_longitude=EXCLUDED.dropoff_longitude,
			package_count=EXCLUDED.package_count,total_weight_grams=EXCLUDED.total_weight_grams,
			serviceable=EXCLUDED.serviceable,reason_code=EXCLUDED.reason_code,
			distance_km=EXCLUDED.distance_km,estimated_minutes=EXCLUDED.estimated_minutes,
			fee_minor=EXCLUDED.fee_minor,currency=EXCLUDED.currency,
			available_providers=EXCLUDED.available_providers,expires_at=EXCLUDED.expires_at,
			contract_version=1,request_snapshot=EXCLUDED.request_snapshot,
			route_snapshot=EXCLUDED.route_snapshot,supply_state=EXCLUDED.supply_state,
			mode=EXCLUDED.mode,service_class=EXCLUDED.service_class,
			capability_version=EXCLUDED.capability_version,route_provider=EXCLUDED.route_provider,
			route_evidence_version=EXCLUDED.route_evidence_version,
			distance_meters=EXCLUDED.distance_meters,estimated_seconds=EXCLUDED.estimated_seconds,
			updated_at=now()
		WHERE commerce_delivery_quotes.request_fingerprint IS DISTINCT FROM EXCLUDED.request_fingerprint
		RETURNING `+multiStopColumns,
		record.ID, record.CallerService, record.QuoteReference, record.Revision, record.RequestFingerprint,
		pickupLat, pickupLng, request.Receiver.Latitude, request.Receiver.Longitude, packageCount, weight,
		record.Serviceable, record.ReasonCode, float64(record.DistanceMeters)/1000,
		int((record.EstimatedSeconds+59)/60), record.FeeMinor, record.Currency,
		record.AvailableProviders, record.ExpiresAt, record.RequestSnapshot, record.RouteSnapshot,
		record.SupplyState, record.Mode, record.ServiceClass, record.CapabilityVersion,
		record.RouteProvider, record.RouteEvidenceVersion, record.DistanceMeters, record.EstimatedSeconds)
	stored, err := scanMultiStop(row)
	if err == nil {
		return stored, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return MultiStopRecord{}, err
	}
	stored, found, readErr := r.FindMultiStopByReference(ctx, record.CallerService, record.QuoteReference)
	if readErr != nil || !found {
		return MultiStopRecord{}, err
	}
	return stored, nil
}

func (r *Repository) FindMultiStopByReference(ctx context.Context, caller, reference string) (MultiStopRecord, bool, error) {
	if r == nil || r.db == nil {
		return MultiStopRecord{}, false, ErrUnavailable
	}
	record, err := scanMultiStop(r.db.QueryRow(ctx, `SELECT `+multiStopColumns+` FROM commerce_delivery_quotes WHERE caller_service=$1 AND quote_reference=$2 AND contract_version=1`, caller, reference))
	if errors.Is(err, pgx.ErrNoRows) {
		return MultiStopRecord{}, false, nil
	}
	return record, err == nil, err
}

func (r *Repository) FindMultiStopByID(ctx context.Context, caller, quoteID string) (MultiStopRecord, bool, error) {
	if r == nil || r.db == nil {
		return MultiStopRecord{}, false, ErrUnavailable
	}
	record, err := scanMultiStop(r.db.QueryRow(ctx, `SELECT `+multiStopColumns+` FROM commerce_delivery_quotes WHERE caller_service=$1 AND id=$2 AND contract_version=1`, caller, quoteID))
	if errors.Is(err, pgx.ErrNoRows) {
		return MultiStopRecord{}, false, nil
	}
	return record, err == nil, err
}

func scanMultiStop(row scanner) (MultiStopRecord, error) {
	var record MultiStopRecord
	err := row.Scan(&record.ID, &record.CallerService, &record.QuoteReference, &record.Revision,
		&record.RequestFingerprint, &record.RequestSnapshot, &record.RouteSnapshot, &record.Serviceable,
		&record.ReasonCode, &record.SupplyState, &record.Mode, &record.ServiceClass,
		&record.CapabilityVersion, &record.RouteProvider, &record.RouteEvidenceVersion,
		&record.DistanceMeters, &record.EstimatedSeconds, &record.FeeMinor, &record.Currency,
		&record.AvailableProviders, &record.ExpiresAt, &record.CreatedAt)
	return record, err
}
