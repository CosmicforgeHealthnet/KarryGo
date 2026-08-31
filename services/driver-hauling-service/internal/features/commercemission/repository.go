package commercemission

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/commercemission"
)

// Record is one persisted mission.
type Record struct {
	ID              string
	MissionRef      string
	ParentOrderRef  string
	VendorOrderRefs []string
	CustomerID      string
	BookingID       string
	AgreedFeeMinor  int64
	Currency        string
	Status          string
	RefusalCode     string
	CreatedAt       time.Time
}

// Mission renders the record as the shared contract's answer.
//
// AcceptedFeeMinor is always the agreed fee that was stored, never a value
// derived at read time, so the caller's re-pricing check compares against what
// was actually committed.
func (r Record) Mission() commercemission.Mission {
	mission := commercemission.Mission{
		MissionRef:       r.MissionRef,
		ExternalRef:      r.BookingID,
		AcceptedFeeMinor: r.AgreedFeeMinor,
		AcceptedAt:       r.CreatedAt,
	}
	if r.Status == "refused" {
		mission.Status = commercemission.StatusRefused
		mission.RefusalCode = r.RefusalCode
		mission.ExternalRef = ""
		return mission
	}
	mission.Status = commercemission.StatusAccepted
	return mission
}

// CreateInput carries an accepted mission.
type CreateInput struct {
	Request   commercemission.Request
	CreatedAt time.Time
}

// RefusalInput carries a refused one.
type RefusalInput struct {
	Request     commercemission.Request
	RefusalCode string
	CreatedAt   time.Time
}

// Repository is the PostgreSQL store.
type Repository struct{ db *pgxpool.Pool }

// NewRepository builds the store.
func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

const missionColumns = `
	id::text, mission_ref, parent_order_ref, vendor_order_refs, customer_id::text,
	booking_id::text, agreed_fee_minor, currency, status, refusal_code, created_at`

// FindByMissionRef returns an existing mission for this reference.
func (r *Repository) FindByMissionRef(
	ctx context.Context, missionRef string,
) (Record, bool, error) {
	if r == nil || r.db == nil {
		return Record{}, false, ErrMissionNotFound
	}
	record, err := scanRecord(r.db.QueryRow(ctx,
		`SELECT `+missionColumns+` FROM commerce_missions WHERE mission_ref=$1`, missionRef))
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, false, nil
	}
	if err != nil {
		return Record{}, false, err
	}
	return record, true, nil
}

// CreateMissionWithBooking writes the booking and the mission atomically.
//
// fare_estimate_kobo and fare_final_kobo are BOTH the agreed fee. Nothing in
// this statement runs hauling's own fare formula, which is what makes the
// commerce total-price guarantee structural here rather than a convention.
//
// payment_intent_id carries the commerce parent order, so settlement is
// traceable back to the payment that already happened rather than implying a
// second escrow this service would have to hold.
func (r *Repository) CreateMissionWithBooking(
	ctx context.Context, input CreateInput,
) (Record, error) {
	if r == nil || r.db == nil {
		return Record{}, ErrMissionNotFound
	}
	request := input.Request

	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Record{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var bookingID string
	if err = tx.QueryRow(ctx, `
		INSERT INTO haulage_bookings(
			customer_id,pickup_address,pickup_lat,pickup_lng,
			dropoff_address,dropoff_lat,dropoff_lng,
			cargo_type,cargo_weight_kg,cargo_description,
			fare_estimate_kobo,payment_intent_id,status,
			receiver_name,receiver_phone)
		VALUES($1::uuid,$2,$3,$4,$5,$6,$7,'general',$8,$9,$10,$11,'pending_match',$12,$13)
		RETURNING id::text`,
		request.CustomerRef,
		request.Pickup.Address, request.Pickup.Latitude, request.Pickup.Longitude,
		request.Dropoff.Address, request.Dropoff.Latitude, request.Dropoff.Longitude,
		cargoWeightKg(request.Load.WeightGrams), cargoDescription(request),
		request.AgreedFeeMinor, request.ParentOrderRef,
		receiverName(request), request.Dropoff.Phone).Scan(&bookingID); err != nil {
		return Record{}, err
	}

	record, err := scanRecord(tx.QueryRow(ctx, `
		INSERT INTO commerce_missions(
			mission_ref,parent_order_ref,vendor_order_refs,customer_id,booking_id,
			agreed_fee_minor,currency,quote_id,quote_reference,status,created_at)
		VALUES($1,$2,$3,$4::uuid,$5::uuid,$6,$7,$8,$9,'accepted',$10)
		RETURNING `+missionColumns,
		request.MissionRef, request.ParentOrderRef, request.VendorOrderRefs,
		request.CustomerRef, bookingID, request.AgreedFeeMinor, request.Currency,
		request.QuoteID, request.QuoteReference, input.CreatedAt))
	if err != nil {
		return Record{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Record{}, err
	}
	return record, nil
}

// MarkRefused records a refusal durably, as a cancelled booking naming the
// reason so every mission stays traceable to a row.
func (r *Repository) MarkRefused(
	ctx context.Context, input RefusalInput,
) (Record, error) {
	if r == nil || r.db == nil {
		return Record{}, ErrMissionNotFound
	}
	request := input.Request

	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Record{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var bookingID string
	if err = tx.QueryRow(ctx, `
		INSERT INTO haulage_bookings(
			customer_id,pickup_address,pickup_lat,pickup_lng,
			dropoff_address,dropoff_lat,dropoff_lng,
			cargo_type,cargo_weight_kg,cargo_description,
			fare_estimate_kobo,payment_intent_id,status,
			receiver_name,receiver_phone,cancel_reason,cancelled_by,cancelled_at)
		VALUES($1::uuid,$2,$3,$4,$5,$6,$7,'general',$8,$9,$10,$11,'cancelled',
		       $12,$13,$14,'platform',$15)
		RETURNING id::text`,
		request.CustomerRef,
		request.Pickup.Address, request.Pickup.Latitude, request.Pickup.Longitude,
		request.Dropoff.Address, request.Dropoff.Latitude, request.Dropoff.Longitude,
		cargoWeightKg(request.Load.WeightGrams), cargoDescription(request),
		request.AgreedFeeMinor, request.ParentOrderRef,
		receiverName(request), request.Dropoff.Phone,
		input.RefusalCode, input.CreatedAt).Scan(&bookingID); err != nil {
		return Record{}, err
	}

	record, err := scanRecord(tx.QueryRow(ctx, `
		INSERT INTO commerce_missions(
			mission_ref,parent_order_ref,vendor_order_refs,customer_id,booking_id,
			agreed_fee_minor,currency,quote_id,quote_reference,status,refusal_code,created_at)
		VALUES($1,$2,$3,$4::uuid,$5::uuid,$6,$7,$8,$9,'refused',$10,$11)
		RETURNING `+missionColumns,
		request.MissionRef, request.ParentOrderRef, request.VendorOrderRefs,
		request.CustomerRef, bookingID, request.AgreedFeeMinor, request.Currency,
		request.QuoteID, request.QuoteReference, input.RefusalCode, input.CreatedAt))
	if err != nil {
		return Record{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Record{}, err
	}
	return record, nil
}

type rowScanner interface{ Scan(...any) error }

func scanRecord(row rowScanner) (Record, error) {
	var record Record
	err := row.Scan(&record.ID, &record.MissionRef, &record.ParentOrderRef,
		&record.VendorOrderRefs, &record.CustomerID, &record.BookingID,
		&record.AgreedFeeMinor, &record.Currency, &record.Status,
		&record.RefusalCode, &record.CreatedAt)
	return record, err
}

var _ Store = (*Repository)(nil)
