package commercemission

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/commercemission"
)

type Record struct {
	ID                 string
	MissionRef         string
	OperationRef       string
	ParentOrderRef     string
	VendorOrderRefs    []string
	CustomerID         string
	BookingID          string
	RequestFingerprint string
	AgreedFeeMinor     int64
	Currency           string
	Status             string
	RefusalCode        string
	SourceVersion      int64
	CreatedAt          time.Time
}

func (r Record) Mission() commercemission.Mission {
	mission := commercemission.Mission{
		MissionRef:       r.MissionRef,
		OperationRef:     r.OperationRef,
		ExternalRef:      r.BookingID,
		AcceptedFeeMinor: r.AgreedFeeMinor,
		AcceptedAt:       r.CreatedAt,
		Version:          r.SourceVersion,
	}
	switch r.Status {
	case "refused":
		mission.Status = commercemission.StatusRefused
		mission.RefusalCode = r.RefusalCode
		mission.ExternalRef = ""
	case "cancelled":
		mission.Status = commercemission.StatusCancelled
	default:
		mission.Status = commercemission.StatusAccepted
	}
	return mission
}

type CreateInput struct {
	Request            commercemission.Request
	RequestFingerprint string
	CreatedAt          time.Time
}

type RefusalInput struct {
	Request            commercemission.Request
	RequestFingerprint string
	RefusalCode        string
	CreatedAt          time.Time
}

type CancelInput struct {
	OperationRef string
	ReasonCode   string
	CancelledAt  time.Time
}

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

const missionColumns = `
	id::text, mission_ref, operation_ref, parent_order_ref, vendor_order_refs, customer_id::text,
	booking_id::text, request_fingerprint, agreed_fee_minor, currency, status, refusal_code,
	source_version, created_at`

func (r *Repository) FindByMissionRef(ctx context.Context, missionRef string) (Record, bool, error) {
	if r == nil || r.db == nil {
		return Record{}, false, ErrMissionNotFound
	}
	return r.find(ctx, `mission_ref=$1`, missionRef)
}

func (r *Repository) FindByOperationRef(ctx context.Context, operationRef string) (Record, bool, error) {
	if r == nil || r.db == nil {
		return Record{}, false, ErrMissionNotFound
	}
	return r.find(ctx, `operation_ref=$1`, operationRef)
}

func (r *Repository) find(ctx context.Context, predicate string, arg string) (Record, bool, error) {
	record, err := scanRecord(r.db.QueryRow(ctx,
		`SELECT `+missionColumns+` FROM commerce_missions WHERE `+predicate, arg))
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, false, nil
	}
	if err != nil {
		return Record{}, false, err
	}
	return record, true, nil
}

func (r *Repository) CreateMissionWithBooking(ctx context.Context, input CreateInput) (Record, error) {
	if r == nil || r.db == nil {
		return Record{}, ErrMissionNotFound
	}
	request := input.Request
	pickup := primaryPickup(request)

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
			fare_estimate_kobo,fare_final_kobo,payment_intent_id,status,
			preferred_truck_type,scheduled_at,receiver_name,receiver_phone)
		VALUES($1::uuid,$2,$3,$4,$5,$6,$7,'general',$8,$9,$10,$10,$11,'pending_match',
		       $12,$13,$14,$15)
		RETURNING id::text`,
		request.CustomerRef,
		pickup.Address, pickup.Latitude, pickup.Longitude,
		request.Dropoff.Address, request.Dropoff.Latitude, request.Dropoff.Longitude,
		cargoWeightKg(request.Load.WeightGrams), cargoDescription(request),
		request.AgreedFeeMinor, request.ParentOrderRef, preferredTruckType(request),
		scheduledAt(request), receiverName(request), request.Dropoff.Phone).Scan(&bookingID); err != nil {
		return Record{}, err
	}

	record, err := insertMission(ctx, tx, request, input.RequestFingerprint, bookingID, "accepted", "", input.CreatedAt)
	if err != nil {
		return Record{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Record{}, err
	}
	return record, nil
}

func (r *Repository) MarkRefused(ctx context.Context, input RefusalInput) (Record, error) {
	if r == nil || r.db == nil {
		return Record{}, ErrMissionNotFound
	}
	request := input.Request
	pickup := primaryPickup(request)

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
			fare_estimate_kobo,fare_final_kobo,payment_intent_id,status,
			preferred_truck_type,scheduled_at,receiver_name,receiver_phone,
			cancel_reason,cancelled_by,cancelled_at)
		VALUES($1::uuid,$2,$3,$4,$5,$6,$7,'general',$8,$9,$10,$10,$11,'cancelled',
		       $12,$13,$14,$15,$16,'platform',$17)
		RETURNING id::text`,
		request.CustomerRef,
		pickup.Address, pickup.Latitude, pickup.Longitude,
		request.Dropoff.Address, request.Dropoff.Latitude, request.Dropoff.Longitude,
		cargoWeightKg(request.Load.WeightGrams), cargoDescription(request),
		request.AgreedFeeMinor, request.ParentOrderRef, preferredTruckType(request),
		scheduledAt(request), receiverName(request), request.Dropoff.Phone,
		input.RefusalCode, input.CreatedAt).Scan(&bookingID); err != nil {
		return Record{}, err
	}

	record, err := insertMission(ctx, tx, request, input.RequestFingerprint, bookingID, "refused", input.RefusalCode, input.CreatedAt)
	if err != nil {
		return Record{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Record{}, err
	}
	return record, nil
}

func (r *Repository) Cancel(ctx context.Context, input CancelInput) (Record, error) {
	if r == nil || r.db == nil {
		return Record{}, ErrMissionNotFound
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Record{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	record, err := scanRecord(tx.QueryRow(ctx,
		`SELECT `+missionColumns+` FROM commerce_missions WHERE operation_ref=$1 FOR UPDATE`,
		strings.TrimSpace(input.OperationRef)))
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, ErrMissionNotFound
	}
	if err != nil {
		return Record{}, err
	}
	if record.Status == "refused" || record.Status == "cancelled" {
		_ = tx.Commit(ctx)
		return record, nil
	}

	var bookingStatus string
	if err = tx.QueryRow(ctx, `SELECT status FROM haulage_bookings WHERE id=$1::uuid FOR UPDATE`,
		record.BookingID).Scan(&bookingStatus); err != nil {
		return Record{}, err
	}
	switch strings.ToLower(strings.TrimSpace(bookingStatus)) {
	case "picked_up", "en_route_delivery", "delivered", "completed":
		return Record{}, apperrors.Conflict("Mission is already in custody and cannot be cancelled here.", nil)
	}

	if _, err = tx.Exec(ctx, `
		UPDATE haulage_bookings
		   SET status='cancelled', cancel_reason=$2, cancelled_by='vendor-service',
		       cancelled_at=$3, updated_at=$3, source_version=source_version+1
		 WHERE id=$1::uuid`,
		record.BookingID, strings.TrimSpace(input.ReasonCode), input.CancelledAt); err != nil {
		return Record{}, err
	}
	if err = tx.QueryRow(ctx, `
		UPDATE commerce_missions
		   SET status='cancelled', updated_at=$2, source_version=source_version+1
		 WHERE operation_ref=$1
		 RETURNING `+missionColumns,
		strings.TrimSpace(input.OperationRef), input.CancelledAt).Scan(
		&record.ID, &record.MissionRef, &record.OperationRef, &record.ParentOrderRef,
		&record.VendorOrderRefs, &record.CustomerID, &record.BookingID,
		&record.RequestFingerprint, &record.AgreedFeeMinor, &record.Currency,
		&record.Status, &record.RefusalCode, &record.SourceVersion, &record.CreatedAt); err != nil {
		return Record{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Record{}, err
	}
	return record, nil
}

type txQueryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func insertMission(
	ctx context.Context,
	tx txQueryer,
	request commercemission.Request,
	fingerprint, bookingID, status, refusalCode string,
	createdAt time.Time,
) (Record, error) {
	pickups, _ := json.Marshal(request.PickupStops)
	parcels, _ := json.Marshal(request.Parcels)
	handling, _ := json.Marshal(request.Handling)
	plan := request.Plan
	if plan == nil {
		plan = &commercemission.PlanBinding{}
	}
	window := firstReadyWindow(request)

	return scanRecord(tx.QueryRow(ctx, `
		INSERT INTO commerce_missions(
			mission_ref,operation_ref,parent_order_ref,vendor_order_refs,customer_id,booking_id,
			request_fingerprint,agreed_fee_minor,currency,quote_id,quote_reference,status,refusal_code,
			plan_ref,plan_version,plan_fingerprint,mission_slot_ref,vehicle_class,pickup_stops,parcels,
			manifest_ref,manifest_version,handling_requirements,pickup_window_start,pickup_window_end,
			pickup_timezone,created_at,updated_at)
		VALUES($1,$2,$3,$4,$5::uuid,$6::uuid,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,
		       $21,$22,$23,$24,$25,$26,$27,$27)
		RETURNING `+missionColumns,
		request.MissionRef, request.OperationReference(), request.ParentOrderRef,
		request.VendorOrderRefs, request.CustomerRef, bookingID, fingerprint,
		request.AgreedFeeMinor, strings.ToUpper(strings.TrimSpace(request.Currency)),
		request.QuoteID, request.QuoteReference, status, refusalCode,
		plan.PlanRef, plan.PlanVersion, plan.PlanFingerprint, plan.MissionSlotRef,
		vehicleClass(request), pickups, parcels, manifestRef(request), manifestVersion(request),
		handling, nullTime(window.From), nullTime(window.To), window.Timezone, createdAt))
}

type rowScanner interface{ Scan(...any) error }

func scanRecord(row rowScanner) (Record, error) {
	var record Record
	err := row.Scan(&record.ID, &record.MissionRef, &record.OperationRef,
		&record.ParentOrderRef, &record.VendorOrderRefs, &record.CustomerID,
		&record.BookingID, &record.RequestFingerprint, &record.AgreedFeeMinor,
		&record.Currency, &record.Status, &record.RefusalCode,
		&record.SourceVersion, &record.CreatedAt)
	return record, err
}

func primaryPickup(request commercemission.Request) commercemission.Point {
	if len(request.PickupStops) > 0 {
		return request.PickupStops[0].Location
	}
	return request.Pickup
}

func firstReadyWindow(request commercemission.Request) commercemission.ReadyWindow {
	if len(request.PickupStops) > 0 {
		return request.PickupStops[0].ReadyWindow
	}
	return commercemission.ReadyWindow{}
}

func scheduledAt(request commercemission.Request) *time.Time {
	window := firstReadyWindow(request)
	if window.From.IsZero() {
		return nil
	}
	value := window.From.UTC()
	return &value
}

func vehicleClass(request commercemission.Request) string {
	if request.Vehicle.Class != "" {
		return string(request.Vehicle.Class)
	}
	return string(request.VehicleClass)
}

func preferredTruckType(request commercemission.Request) string {
	switch vehicleClass(request) {
	case string(commercemission.VehicleVan):
		return "van"
	default:
		return "truck"
	}
}

func manifestRef(request commercemission.Request) string {
	if len(request.PickupStops) > 0 && strings.TrimSpace(request.PickupStops[0].ManifestRef) != "" {
		return strings.TrimSpace(request.PickupStops[0].ManifestRef)
	}
	if len(request.Parcels) > 0 {
		return strings.TrimSpace(request.Parcels[0].ManifestRef)
	}
	return ""
}

func manifestVersion(request commercemission.Request) int64 {
	if len(request.PickupStops) > 0 && request.PickupStops[0].ManifestVersion > 0 {
		return request.PickupStops[0].ManifestVersion
	}
	if len(request.Parcels) > 0 {
		return request.Parcels[0].ManifestVersion
	}
	return 0
}

func nullTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC()
}

var _ Store = (*Repository)(nil)
