package privateexecution

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("hauling private execution not found")
var ErrConflict = errors.New("hauling private execution idempotency conflict")
var ErrIneligible = errors.New("hauling provider or truck is ineligible")
var ErrNotCancellable = errors.New("hauling execution is not cancellable")
var ErrDisabled = errors.New("hauling private execution is disabled")

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db} }

const cols = `fleet_operation_id,status,COALESCE(canonical_booking_id::text,''),
	COALESCE(canonical_booking_id::text,''),source_version,cancellation_status,
	last_error_code,created_at,updated_at,idempotency_key,request_fingerprint,
	organization_id,settlement_organization_id,executing_provider_id::text,
	organization_membership_id,fleet_private_order_id,fleet_shipment_id,
	fleet_assignment_id,fleet_provider_response_id,truck_id::text,correlation_id,
	COALESCE(cancellation_idempotency_key,''),COALESCE(cancellation_fingerprint,'')`

func scan(row pgx.Row) (record, error) {
	var r record
	err := row.Scan(&r.FleetOperationID, &r.Status, &r.CanonicalBookingID, &r.CanonicalTripID, &r.SourceVersion, &r.CancellationStatus, &r.LastErrorCode, &r.CreatedAt, &r.UpdatedAt, &r.IdempotencyKey, &r.RequestFingerprint, &r.OrganizationID, &r.SettlementOrganizationID, &r.ExecutingProviderID, &r.OrganizationMembershipID, &r.PrivateOrderID, &r.ShipmentID, &r.PrivateAssignmentID, &r.ProviderResponseID, &r.TruckID, &r.CorrelationID, &r.CancellationKey, &r.CancellationFingerprint)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}
func (r *Repository) GetOperation(ctx context.Context, id string) (record, error) {
	return scan(r.db.QueryRow(ctx, `SELECT `+cols+` FROM hauling_private_execution_mappings WHERE fleet_operation_id=$1`, id))
}
func (r *Repository) GetKey(ctx context.Context, key string) (record, error) {
	return scan(r.db.QueryRow(ctx, `SELECT `+cols+` FROM hauling_private_execution_mappings WHERE idempotency_key=$1`, key))
}
func (r *Repository) GetTrip(ctx context.Context, id string) (record, error) {
	return scan(r.db.QueryRow(ctx, `SELECT `+cols+` FROM hauling_private_execution_mappings WHERE canonical_booking_id=$1`, id))
}

func (r *Repository) Create(ctx context.Context, in CreateRequest, fp string) (record, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return record{}, err
	}
	defer tx.Rollback(ctx)
	var eligible bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM truck_providers p
		JOIN trucks t ON t.provider_id=p.id
		JOIN hauling_organization_provider_links pl ON pl.provider_id=p.id
		JOIN hauling_organization_truck_links tl ON tl.truck_id=t.id AND tl.organization_id=pl.organization_id
		LEFT JOIN hauling_provider_availability a ON a.provider_id=p.id
		WHERE p.id=$1 AND t.id=$2 AND pl.organization_id=$3 AND pl.membership_id=$4
		AND pl.state='ACTIVE' AND tl.state='ACTIVE' AND p.status='active'
		AND p.onboarding_status='complete' AND COALESCE(p.service_type,'hauling')='hauling'
		AND t.status='active' AND a.state='AVAILABLE' AND a.shift_open)`, in.ExecutingProviderID, in.TruckID, in.OrganizationID, in.OrganizationMembershipID).Scan(&eligible)
	if err != nil || !eligible {
		return record{}, ErrIneligible
	}
	bookingID := uuid.NewString()
	cargoType := strings.ToLower(strings.TrimSpace(in.Cargo.CargoType))
	if cargoType == "" {
		cargoType = "general"
	}
	weight := in.Cargo.WeightKG
	if weight < 1 {
		weight = 1
	}
	_, err = tx.Exec(ctx, `INSERT INTO haulage_bookings(id,customer_id,provider_id,truck_id,
		pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,
		cargo_type,cargo_weight_kg,cargo_description,status,accepted_at,source_version)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,'accepted',now(),1)`, bookingID, uuid.NewString(), in.ExecutingProviderID, in.TruckID, in.Route.PickupAddress, in.Route.PickupLatitude, in.Route.PickupLongitude, in.Route.DropoffAddress, in.Route.DropoffLatitude, in.Route.DropoffLongitude, cargoType, weight, in.Cargo.Description)
	if err != nil {
		return record{}, err
	}
	var out record
	err = tx.QueryRow(ctx, `INSERT INTO hauling_private_execution_mappings(
		fleet_operation_id,idempotency_key,request_fingerprint,organization_id,
		settlement_organization_id,executing_provider_id,organization_membership_id,
		fleet_private_order_id,fleet_shipment_id,fleet_assignment_id,
		fleet_provider_response_id,truck_id,service_category,contract_reference,
		quotation_reference,correlation_id,canonical_booking_id,status,source_version)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,'HAULING',$13,$14,$15,$16,'CONFIRMED',1)
		RETURNING `+cols, in.FleetOperationID, in.IdempotencyKey, fp, in.OrganizationID, in.SettlementOrganizationID, in.ExecutingProviderID, in.OrganizationMembershipID, in.PrivateOrderID, in.ShipmentID, in.PrivateAssignmentID, in.ProviderResponseID, in.TruckID, in.ContractReference, in.QuotationReference, in.CorrelationID, bookingID).Scan(&out.FleetOperationID, &out.Status, &out.CanonicalBookingID, &out.CanonicalTripID, &out.SourceVersion, &out.CancellationStatus, &out.LastErrorCode, &out.CreatedAt, &out.UpdatedAt, &out.IdempotencyKey, &out.RequestFingerprint, &out.OrganizationID, &out.SettlementOrganizationID, &out.ExecutingProviderID, &out.OrganizationMembershipID, &out.PrivateOrderID, &out.ShipmentID, &out.PrivateAssignmentID, &out.ProviderResponseID, &out.TruckID, &out.CorrelationID, &out.CancellationKey, &out.CancellationFingerprint)
	if err != nil {
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.Code == "23505" {
			return record{}, ErrConflict
		}
		return record{}, err
	}
	payload := map[string]any{"tripId": bookingID, "requestId": bookingID, "status": "ACCEPTED", "sourceVersion": int64(1), "organizationId": in.OrganizationID, "settlementOrganizationId": in.SettlementOrganizationID, "executingProviderId": in.ExecutingProviderID, "fleetExecutionOperationId": in.FleetOperationID, "privateOrderId": in.PrivateOrderID, "shipmentId": in.ShipmentID, "privateAssignmentId": in.PrivateAssignmentID}
	raw, _ := json.Marshal(payload)
	_, err = tx.Exec(ctx, `INSERT INTO hauling_domain_outbox(aggregate_type,aggregate_id,event_type,event_version,payload,correlation_id) VALUES('TRIP',$1,'trip.created.v1',1,$2,$3)`, bookingID, raw, in.CorrelationID)
	if err != nil {
		return record{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return record{}, err
	}
	return out, nil
}

func (r *Repository) Cancel(ctx context.Context, op, key, fp, reason string) (CancelView, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return CancelView{}, err
	}
	defer tx.Rollback(ctx)
	var booking, status, cancelStatus, oldKey, oldFP, org, settlement, provider, order, shipment, assignment string
	var version int64
	err = tx.QueryRow(ctx, `SELECT m.canonical_booking_id::text,b.status,m.source_version,m.cancellation_status,
		COALESCE(m.cancellation_idempotency_key,''),COALESCE(m.cancellation_fingerprint,''),m.organization_id,m.settlement_organization_id,m.executing_provider_id::text,m.fleet_private_order_id,m.fleet_shipment_id,m.fleet_assignment_id
		FROM hauling_private_execution_mappings m JOIN haulage_bookings b ON b.id=m.canonical_booking_id WHERE m.fleet_operation_id=$1 FOR UPDATE`, op).Scan(&booking, &status, &version, &cancelStatus, &oldKey, &oldFP, &org, &settlement, &provider, &order, &shipment, &assignment)
	if errors.Is(err, pgx.ErrNoRows) {
		return CancelView{}, ErrNotFound
	}
	if err != nil {
		return CancelView{}, err
	}
	if oldKey != "" {
		if oldKey != key || oldFP != fp {
			return CancelView{}, ErrConflict
		}
		if cancelStatus == "CONFIRMED" {
			return CancelView{"CANCELLED", version}, nil
		}
	}
	if status == "cancelled" {
		cancelStatus = "CONFIRMED"
	} else if status == "completed" || status == "failed" {
		return CancelView{}, ErrNotCancellable
	} else {
		err = tx.QueryRow(ctx, `UPDATE haulage_bookings SET status='cancelled',cancelled_by='fleet-partner-service',cancel_reason=$2,cancelled_at=now(),source_version=source_version+1,updated_at=now() WHERE id=$1 RETURNING source_version`, booking, reason).Scan(&version)
		if err != nil {
			return CancelView{}, err
		}
		cancelStatus = "CONFIRMED"
		payload := map[string]any{"tripId": booking, "status": "CANCELLED", "sourceVersion": version, "organizationId": org, "settlementOrganizationId": settlement, "executingProviderId": provider, "fleetExecutionOperationId": op, "privateOrderId": order, "shipmentId": shipment, "privateAssignmentId": assignment}
		raw, _ := json.Marshal(payload)
		_, err = tx.Exec(ctx, `INSERT INTO hauling_domain_outbox(aggregate_type,aggregate_id,event_type,event_version,payload,correlation_id) SELECT 'TRIP',$1,'trip.cancelled.v1',$2,$3,correlation_id FROM hauling_private_execution_mappings WHERE fleet_operation_id=$4`, booking, version, raw, op)
		if err != nil {
			return CancelView{}, err
		}
	}
	_, err = tx.Exec(ctx, `UPDATE hauling_private_execution_mappings SET cancellation_idempotency_key=$2,cancellation_fingerprint=$3,cancellation_status=$4,status='CANCELLED',source_version=$5,updated_at=now() WHERE fleet_operation_id=$1`, op, key, fp, cancelStatus, version)
	if err != nil {
		return CancelView{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return CancelView{}, err
	}
	return CancelView{"CANCELLED", version}, nil
}
