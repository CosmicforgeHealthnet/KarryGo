package privateexecution

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository owns the mapping table and the one transaction that creates a
// corporate trip.
type Repository struct{ db *pgxpool.Pool }

// NewRepository builds the repository.
func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

const columns = `fleet_operation_id, status,
	COALESCE(canonical_booking_id::text,''), COALESCE(canonical_booking_id::text,''),
	source_version, cancellation_status, last_error_code, created_at, updated_at,
	idempotency_key, request_fingerprint, organization_id, settlement_organization_id,
	executing_provider_id::text, organization_membership_id, fleet_private_order_id,
	fleet_shipment_id, fleet_assignment_id, fleet_provider_response_id, vehicle_id::text,
	correlation_id, COALESCE(cancellation_idempotency_key,''), COALESCE(cancellation_fingerprint,'')`

func scan(row pgx.Row) (record, error) {
	var r record
	err := row.Scan(&r.FleetOperationID, &r.Status, &r.CanonicalBookingID, &r.CanonicalTripID,
		&r.SourceVersion, &r.CancellationStatus, &r.LastErrorCode, &r.CreatedAt, &r.UpdatedAt,
		&r.IdempotencyKey, &r.RequestFingerprint, &r.OrganizationID, &r.SettlementOrganizationID,
		&r.ExecutingProviderID, &r.OrganizationMembershipID, &r.PrivateOrderID, &r.ShipmentID,
		&r.PrivateAssignmentID, &r.ProviderResponseID, &r.VehicleID, &r.CorrelationID,
		&r.CancellationKey, &r.CancellationFingerprint)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}

// GetOperation reads a mapping by Fleet's operation id. This is the read-back
// Fleet performs after an ambiguous create.
func (r *Repository) GetOperation(ctx context.Context, id string) (record, error) {
	return scan(r.db.QueryRow(ctx,
		`SELECT `+columns+` FROM taxi_private_execution_mappings WHERE fleet_operation_id=$1`, id))
}

// GetKey reads a mapping by idempotency key.
func (r *Repository) GetKey(ctx context.Context, key string) (record, error) {
	return scan(r.db.QueryRow(ctx,
		`SELECT `+columns+` FROM taxi_private_execution_mappings WHERE idempotency_key=$1`, key))
}

// GetBooking reads a mapping by the taxi booking it created.
func (r *Repository) GetBooking(ctx context.Context, id string) (record, error) {
	return scan(r.db.QueryRow(ctx,
		`SELECT `+columns+` FROM taxi_private_execution_mappings WHERE canonical_booking_id::text=$1`, id))
}

// Create starts a corporate taxi execution.
//
// One transaction covers the eligibility check, the booking, its stops, the
// mapping and the lifecycle event. The failure this prevents is a booking that
// exists with no mapping: Fleet's create would have timed out, its read-back
// would find nothing, it would legitimately retry, and a second driver would
// be dispatched for the same corporate order.
//
// The booking is created already ACCEPTED and bound to the provider. That is
// the difference between private execution and the marketplace path: there is
// no offer to make, because Fleet has already obtained the provider's
// acceptance and this service has just verified it against Fleet.
func (r *Repository) Create(ctx context.Context, in CreateRequest, fingerprint string,
	zone Zone) (record, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return record{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Taxi's own eligibility, asked of Taxi's own tables. Fleet has already
	// said the provider may work for this organization; this asks the separate
	// question of whether they can drive right now, which Fleet has no way to
	// know and must never be allowed to assert.
	var eligible bool
	var vehicleClass string
	err = tx.QueryRow(ctx, `
		SELECT TRUE, v.vehicle_class
		  FROM taxi_providers p
		  JOIN vehicles v ON v.provider_id = p.id
		  LEFT JOIN provider_operational_state s ON s.provider_id = p.id
		 WHERE p.id = $1::uuid
		   AND v.id = $2::uuid
		   AND p.account_status = 'active'
		   AND p.onboarding_status = 'complete'
		   AND v.status = 'active'
		   AND COALESCE(s.state, 'OFFLINE') NOT IN ('OFFLINE', 'SUSPENDED')`,
		in.ExecutingProviderID, in.VehicleID).Scan(&eligible, &vehicleClass)
	if errors.Is(err, pgx.ErrNoRows) {
		return record{}, ErrIneligible
	}
	if err != nil {
		return record{}, err
	}

	// A corporate booking still belongs to a passenger record. Fleet's private
	// orders have no platform customer, so one is minted per execution rather
	// than borrowed: attaching corporate trips to a shared synthetic customer
	// would mix unrelated organizations' history into one rider profile.
	bookingID := uuid.NewString()
	customerID := uuid.NewString()
	passengers := in.Ride.PassengerCount
	if passengers < 1 {
		passengers = 1
	}
	if in.Ride.VehicleClass != "" {
		vehicleClass = in.Ride.VehicleClass
	}

	if _, err = tx.Exec(ctx, `
		INSERT INTO taxi_bookings(
			id, customer_id, provider_id, vehicle_id, market_id, operating_zone_id,
			vehicle_class, passenger_count, ride_notes, status, accepted_at, matched_at,
			is_corporate_managed, organization_id, membership_id, fleet_assignment_id,
			source_context, settlement_mode, source_version, fleet_correlation_id,
			payment_method, payment_status)
		VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,$6::uuid,
			$7,$8,$9,'accepted',now(),now(),
			TRUE,$10,$11,$12,$13,$14,1,$15,
			'corporate','deferred')`,
		bookingID, customerID, in.ExecutingProviderID, in.VehicleID, zone.MarketID, zone.ID,
		vehicleClass, passengers, in.Ride.Notes,
		in.OrganizationID, in.OrganizationMembershipID, in.PrivateAssignmentID,
		SourceContextPrivateExecution, SettlementModeOrganizationManaged, in.CorrelationID); err != nil {
		if isUniqueViolation(err) {
			// Either this provider already has a live booking, or this Fleet
			// assignment already produced one. Both are definite refusals, not
			// transient faults: retrying would never succeed.
			return record{}, ErrIneligible
		}
		return record{}, err
	}

	if _, err = tx.Exec(ctx, `
		INSERT INTO taxi_booking_stops(booking_id, sequence, type, address, lat, lng)
		VALUES($1::uuid,1,'PICKUP',$2,$3,$4),($1::uuid,2,'DROPOFF',$5,$6,$7)`,
		bookingID, in.Route.PickupAddress, in.Route.PickupLatitude, in.Route.PickupLongitude,
		in.Route.DropoffAddress, in.Route.DropoffLatitude, in.Route.DropoffLongitude); err != nil {
		return record{}, err
	}

	stored, err := scan(tx.QueryRow(ctx, `
		INSERT INTO taxi_private_execution_mappings(
			fleet_operation_id, idempotency_key, request_fingerprint, organization_id,
			settlement_organization_id, organization_membership_id, executing_provider_id,
			vehicle_id, fleet_private_order_id, fleet_shipment_id, fleet_assignment_id,
			fleet_provider_response_id, service_category, contract_reference,
			quotation_reference, correlation_id, canonical_booking_id, status, source_version)
		VALUES($1,$2,$3,$4,$5,$6,$7::uuid,$8::uuid,$9,$10,$11,$12,'TAXI',$13,$14,$15,$16::uuid,'CONFIRMED',1)
		RETURNING `+columns,
		in.FleetOperationID, in.IdempotencyKey, fingerprint, in.OrganizationID,
		in.SettlementOrganizationID, in.OrganizationMembershipID, in.ExecutingProviderID,
		in.VehicleID, in.PrivateOrderID, in.ShipmentID, in.PrivateAssignmentID,
		in.ProviderResponseID, in.ContractReference, in.QuotationReference,
		in.CorrelationID, bookingID))
	if err != nil {
		if isUniqueViolation(err) {
			return record{}, ErrConflict
		}
		return record{}, err
	}

	if err = recordEvent(ctx, tx, bookingID, "booking.corporate_execution_created", map[string]any{
		"bookingId": bookingID, "organizationId": in.OrganizationID,
		"settlementOrganizationId":  in.SettlementOrganizationID,
		"executingProviderId":       in.ExecutingProviderID,
		"fleetExecutionOperationId": in.FleetOperationID,
		"privateOrderId":            in.PrivateOrderID,
		"shipmentId":                in.ShipmentID,
		"privateAssignmentId":       in.PrivateAssignmentID,
	}); err != nil {
		return record{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return record{}, err
	}
	return stored, nil
}

// Cancel withdraws a corporate trip, idempotently.
//
// A trip that has already started is refused rather than force-cancelled: the
// passenger is in the car, and a corporate planning system withdrawing a
// journey in progress is not a cancellation, it is a stranding.
func (r *Repository) Cancel(ctx context.Context, operationID, key, fingerprint, reason string) (CancelView, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return CancelView{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var bookingID, bookingStatus, cancellationStatus, storedKey, storedFingerprint string
	var version int64
	err = tx.QueryRow(ctx, `
		SELECT m.canonical_booking_id::text, b.status, m.source_version, m.cancellation_status,
		       COALESCE(m.cancellation_idempotency_key,''), COALESCE(m.cancellation_fingerprint,'')
		  FROM taxi_private_execution_mappings m
		  JOIN taxi_bookings b ON b.id = m.canonical_booking_id
		 WHERE m.fleet_operation_id = $1
		 FOR UPDATE OF m`, operationID).
		Scan(&bookingID, &bookingStatus, &version, &cancellationStatus, &storedKey, &storedFingerprint)
	if errors.Is(err, pgx.ErrNoRows) {
		return CancelView{}, ErrNotFound
	}
	if err != nil {
		return CancelView{}, err
	}

	if storedKey != "" {
		if storedKey != key || storedFingerprint != fingerprint {
			return CancelView{}, ErrConflict
		}
		if cancellationStatus == CancellationConfirmed {
			return CancelView{Status: StatusCancelled, SourceVersion: version}, nil
		}
	}

	switch bookingStatus {
	case "cancelled":
		// Already cancelled by some other path; record the confirmation rather
		// than refusing, so Fleet's retry converges instead of looping.
	case "completed", "in_progress", "arrived_at_pickup", "pickup_verification_pending":
		return CancelView{}, ErrNotCancellable
	default:
		if err = tx.QueryRow(ctx, `
			UPDATE taxi_bookings
			   SET status='cancelled', cancelled_by='fleet-partner-service',
			       cancel_reason=$2, cancelled_at=now(),
			       source_version=source_version+1, updated_at=now()
			 WHERE id=$1::uuid
			RETURNING source_version`, bookingID, reason).Scan(&version); err != nil {
			return CancelView{}, err
		}
		if err = recordEvent(ctx, tx, bookingID, "booking.corporate_execution_cancelled", map[string]any{
			"bookingId": bookingID, "reason": reason,
			"fleetExecutionOperationId": operationID,
		}); err != nil {
			return CancelView{}, err
		}
	}

	if _, err = tx.Exec(ctx, `
		UPDATE taxi_private_execution_mappings
		   SET cancellation_idempotency_key=$2, cancellation_fingerprint=$3,
		       cancellation_status='CONFIRMED', status='CANCELLED',
		       source_version=$4, updated_at=now()
		 WHERE fleet_operation_id=$1`, operationID, key, fingerprint, version); err != nil {
		return CancelView{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return CancelView{}, err
	}
	return CancelView{Status: StatusCancelled, SourceVersion: version}, nil
}

// recordEvent appends to the booking's own audit trail. Corporate execution is
// not a separate history: an auditor reading one booking should see the whole
// story of that booking, including who asked for it.
func recordEvent(ctx context.Context, tx pgx.Tx, bookingID, eventType string, metadata map[string]any) error {
	raw, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO booking_events(booking_id, event_type, actor_type, actor_id, metadata)
		VALUES($1::uuid,$2,'service','fleet-partner-service',$3)`, bookingID, eventType, raw)
	return err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
