package privateexecution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("dispatch private execution not found")
var ErrConflict = errors.New("dispatch private execution idempotency conflict")

type repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *repository { return &repository{db: db} }

const mappingColumns = `fleet_operation_id,status,COALESCE(canonical_request_id::text,''),
	COALESCE(canonical_reservation_id::text,''),COALESCE(canonical_trip_id::text,''),
	source_version,cancellation_status,last_error_code,created_at,updated_at,
	id::text,idempotency_key,request_fingerprint,organization_id,settlement_organization_id,
	executing_provider_id::text,organization_membership_id,fleet_private_order_id,
	fleet_shipment_id,fleet_assignment_id,fleet_provider_response_id,
	COALESCE(vehicle_id::text,''),service_category,contract_reference,
	quotation_reference,correlation_id,COALESCE(cancellation_idempotency_key,''),
	COALESCE(cancellation_fingerprint,'')`

func scanRecord(row pgx.Row) (record, error) {
	var r record
	err := row.Scan(&r.FleetOperationID, &r.Status, &r.CanonicalRequestID,
		&r.CanonicalReservationID, &r.CanonicalTripID, &r.SourceVersion,
		&r.CancellationStatus, &r.LastErrorCode, &r.CreatedAt, &r.UpdatedAt,
		&r.ID, &r.IdempotencyKey, &r.RequestFingerprint, &r.OrganizationID,
		&r.SettlementOrganizationID, &r.ExecutingProviderID,
		&r.OrganizationMembershipID, &r.PrivateOrderID, &r.ShipmentID,
		&r.PrivateAssignmentID, &r.ProviderResponseID, &r.VehicleID,
		&r.ServiceCategory, &r.ContractReference, &r.QuotationReference,
		&r.CorrelationID, &r.CancellationKey, &r.CancellationFingerprint)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}

func (r *repository) getByOperation(ctx context.Context, operationID string) (record, error) {
	return scanRecord(r.db.QueryRow(ctx, `SELECT `+mappingColumns+`
		FROM dispatch_private_execution_mappings WHERE fleet_operation_id=$1`, operationID))
}

func (r *repository) getByKey(ctx context.Context, key string) (record, error) {
	return scanRecord(r.db.QueryRow(ctx, `SELECT `+mappingColumns+`
		FROM dispatch_private_execution_mappings WHERE idempotency_key=$1`, key))
}

func (r *repository) getByTrip(ctx context.Context, tripID string) (record, error) {
	return scanRecord(r.db.QueryRow(ctx, `SELECT `+mappingColumns+`
		FROM dispatch_private_execution_mappings WHERE canonical_trip_id=$1`, tripID))
}

func (r *repository) prepare(ctx context.Context, in CreateRequest, fingerprint string) (record, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return record{}, err
	}
	defer tx.Rollback(ctx)

	requestID, bookingID := uuid.NewString(), uuid.NewString()
	payload, err := json.Marshal(map[string]any{
		"customerId": uuid.NewString(), "pickupAddress": in.Route.PickupAddress,
		"pickupLat": in.Route.PickupLatitude, "pickupLng": in.Route.PickupLongitude,
		"dropoffAddress": in.Route.DropoffAddress, "dropoffLat": in.Route.DropoffLatitude,
		"dropoffLng": in.Route.DropoffLongitude, "distanceKm": 0, "fareAmount": 0,
		"currency": "NGN", "receiverName": "", "receiverPhone": "",
	})
	if err != nil {
		return record{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO request_broadcasts
		(id,booking_id,service_type,broadcast_radius_km,status,expires_at,booking_payload,source_version)
		VALUES($1,$2,'dispatch',1,'broadcasting',now()+interval '30 minutes',$3,1)`,
		requestID, bookingID, payload)
	if err != nil {
		return record{}, fmt.Errorf("prepare canonical request: %w", err)
	}

	var out record
	err = tx.QueryRow(ctx, `INSERT INTO dispatch_private_execution_mappings(
		fleet_operation_id,idempotency_key,request_fingerprint,organization_id,
		settlement_organization_id,executing_provider_id,organization_membership_id,
		fleet_private_order_id,fleet_shipment_id,fleet_assignment_id,
		fleet_provider_response_id,vehicle_id,service_category,contract_reference,
		quotation_reference,correlation_id,canonical_request_id,status)
		VALUES($1,$2,$3,$4,$5,$6::uuid,$7,$8,$9,$10,$11,NULLIF($12,'')::uuid,
		$13,$14,$15,$16,$17,'READY') RETURNING `+mappingColumns,
		in.FleetOperationID, in.IdempotencyKey, fingerprint, in.OrganizationID,
		in.SettlementOrganizationID, in.ExecutingProviderID,
		in.OrganizationMembershipID, in.PrivateOrderID, in.ShipmentID,
		in.PrivateAssignmentID, in.ProviderResponseID, in.VehicleID,
		in.ServiceCategory, in.ContractReference, in.QuotationReference,
		in.CorrelationID, requestID).Scan(
		&out.FleetOperationID, &out.Status, &out.CanonicalRequestID,
		&out.CanonicalReservationID, &out.CanonicalTripID, &out.SourceVersion,
		&out.CancellationStatus, &out.LastErrorCode, &out.CreatedAt, &out.UpdatedAt,
		&out.ID, &out.IdempotencyKey, &out.RequestFingerprint, &out.OrganizationID,
		&out.SettlementOrganizationID, &out.ExecutingProviderID,
		&out.OrganizationMembershipID, &out.PrivateOrderID, &out.ShipmentID,
		&out.PrivateAssignmentID, &out.ProviderResponseID, &out.VehicleID,
		&out.ServiceCategory, &out.ContractReference, &out.QuotationReference,
		&out.CorrelationID, &out.CancellationKey, &out.CancellationFingerprint)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return record{}, ErrConflict
		}
		return record{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return record{}, err
	}
	return out, nil
}

func (r *repository) setReservation(ctx context.Context, operationID, reservationID string) error {
	_, err := r.db.Exec(ctx, `UPDATE dispatch_private_execution_mappings
		SET canonical_reservation_id=$2::uuid,updated_at=now() WHERE fleet_operation_id=$1`, operationID, reservationID)
	return err
}

func (r *repository) confirm(ctx context.Context, operationID, tripID string, version int64) error {
	_, err := r.db.Exec(ctx, `UPDATE dispatch_private_execution_mappings
		SET canonical_trip_id=$2::uuid,status='CONFIRMED',source_version=$3,last_error_code='',updated_at=now()
		WHERE fleet_operation_id=$1`, operationID, tripID, version)
	return err
}

func (r *repository) fail(ctx context.Context, operationID, status, code string) error {
	_, err := r.db.Exec(ctx, `UPDATE dispatch_private_execution_mappings
		SET status=$2,last_error_code=$3,updated_at=now() WHERE fleet_operation_id=$1`, operationID, status, code)
	return err
}

func (r *repository) setCancellation(ctx context.Context, operationID, key, fp, status string, version int64) error {
	_, err := r.db.Exec(ctx, `UPDATE dispatch_private_execution_mappings SET
		cancellation_idempotency_key=COALESCE(cancellation_idempotency_key,$2),
		cancellation_fingerprint=COALESCE(cancellation_fingerprint,$3),
		cancellation_status=$4,status=CASE WHEN $4='CONFIRMED' THEN 'CANCELLED' ELSE status END,
		source_version=GREATEST(source_version,$5),updated_at=now()
		WHERE fleet_operation_id=$1`, operationID, key, fp, status, version)
	return err
}
