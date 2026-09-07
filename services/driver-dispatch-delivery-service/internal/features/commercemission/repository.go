package commercemission

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/rollout"
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
		// A refused mission executes nothing, so it names no booking.
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
type Repository struct {
	db            *pgxpool.Pool
	events        TargetEventEmitter
	rolloutPolicy rollout.Policy
}

// NewRepository builds the store.
func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) WithTargetEventEmitter(events TargetEventEmitter) *Repository {
	r.events = events
	return r
}

// WithRolloutPolicy supplies the same server-owned cohort policy used by
// direct bookings and delivery planning. Commerce callers provide selectors,
// never the effective protection mode.
func (r *Repository) WithRolloutPolicy(policy rollout.Policy) *Repository {
	r.rolloutPolicy = policy
	return r
}

func (r *Repository) protection(customerID, sourceRef, serviceTier string) rollout.Protection {
	return r.rolloutPolicy.Protect(rollout.Context{
		CustomerID: customerID, SourceType: "VENDOR_COMMERCE",
		SourceRef: sourceRef, ServiceTier: serviceTier,
	})
}

const missionColumns = `
	id::text, mission_ref, parent_order_ref, vendor_order_refs, customer_id::text,
	COALESCE(booking_id::text,''), agreed_fee_minor, currency, status, refusal_code, created_at`

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
// The booking's fare is the agreed fee, copied verbatim. Nothing in this
// statement recomputes a price, which is what makes the commerce total-price
// guarantee structural here rather than a convention somebody must remember.
//
// The booking is created already paid: commerce charged the customer through
// payment-wallet-service before this mission existed, so holding escrow a
// second time would take the money twice.
func (r *Repository) CreateMissionWithBooking(
	ctx context.Context, input CreateInput,
) (Record, error) {
	if r == nil || r.db == nil {
		return Record{}, ErrMissionNotFound
	}
	request := input.Request
	protection := r.protection(request.CustomerRef, request.MissionRef, "standard")

	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Record{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var bookingID string
	if err = tx.QueryRow(ctx, `
		INSERT INTO dispatch_bookings(
			customer_id,status,pickup_address,pickup_lat,pickup_lng,
			dropoff_address,dropoff_lat,dropoff_lng,fare_amount,currency,
			payment_method,payment_status,payment_ref,
			receiver_name,receiver_phone,package_desc,package_size,is_fragile,service_tier,
			rollout_protection_mode,rollout_policy_version,handover_required,
			pickup_evidence_required,delivery_evidence_required,provider_transfer_required)
		VALUES($1::uuid,'searching',$2,$3,$4,$5,$6,$7,$8,$9,'wallet','paid',$10,
		       $11,$12,$13,$14,$15,'standard',$16,$17,$18,$19,$20,$21)
		RETURNING id::text`,
		request.CustomerRef,
		request.Pickup.Address, request.Pickup.Latitude, request.Pickup.Longitude,
		request.Dropoff.Address, request.Dropoff.Latitude, request.Dropoff.Longitude,
		request.AgreedFeeMinor, request.Currency,
		request.ParentOrderRef,
		receiverName(request), request.Dropoff.Phone,
		request.Load.Description, packageSize(request.Load.WeightGrams),
		request.Load.Fragile, protection.Mode, protection.PolicyVersion,
		protection.HandoverRequired, protection.PickupEvidenceRequired,
		protection.DeliveryEvidenceRequired, protection.ProviderTransferRequired).Scan(&bookingID); err != nil {
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

// MarkRefused records a refusal durably.
//
// A refusal carries no booking, because nothing will be executed.
func (r *Repository) MarkRefused(
	ctx context.Context, input RefusalInput,
) (Record, error) {
	if r == nil || r.db == nil {
		return Record{}, ErrMissionNotFound
	}
	request := input.Request
	protection := r.protection(request.CustomerRef, request.MissionRef, "standard")

	// A refused mission still needs a booking column value, and the schema
	// requires a real booking row, so a refusal is stored as a cancelled
	// booking naming the reason. That keeps every mission traceable to a row
	// rather than leaving refusals invisible.
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Record{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var bookingID string
	if err = tx.QueryRow(ctx, `
		INSERT INTO dispatch_bookings(
			customer_id,status,pickup_address,pickup_lat,pickup_lng,
			dropoff_address,dropoff_lat,dropoff_lng,fare_amount,currency,
			payment_method,payment_status,payment_ref,
			receiver_name,receiver_phone,package_desc,package_size,is_fragile,
			service_tier,cancel_reason,cancelled_at,
			rollout_protection_mode,rollout_policy_version,handover_required,
			pickup_evidence_required,delivery_evidence_required,provider_transfer_required)
		VALUES($1::uuid,'cancelled',$2,$3,$4,$5,$6,$7,$8,$9,'wallet','paid',$10,
		       $11,$12,$13,$14,$15,'standard',$16,$17,$18,$19,$20,$21,$22,$23)
		RETURNING id::text`,
		request.CustomerRef,
		request.Pickup.Address, request.Pickup.Latitude, request.Pickup.Longitude,
		request.Dropoff.Address, request.Dropoff.Latitude, request.Dropoff.Longitude,
		request.AgreedFeeMinor, request.Currency,
		request.ParentOrderRef,
		receiverName(request), request.Dropoff.Phone,
		request.Load.Description, packageSize(request.Load.WeightGrams),
		request.Load.Fragile,
		input.RefusalCode, input.CreatedAt, protection.Mode, protection.PolicyVersion,
		protection.HandoverRequired, protection.PickupEvidenceRequired,
		protection.DeliveryEvidenceRequired, protection.ProviderTransferRequired).Scan(&bookingID); err != nil {
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
