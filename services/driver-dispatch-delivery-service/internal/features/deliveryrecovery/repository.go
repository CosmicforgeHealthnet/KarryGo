package deliveryrecovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/apperrors"
)

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) Trip(ctx context.Context, tripID, providerID string, lock bool) (TripContext, error) {
	if r == nil || r.db == nil {
		return TripContext{}, errors.New("delivery recovery repository unavailable")
	}
	query := `SELECT t.id::text,t.booking_id::text,t.provider_id::text,t.customer_id::text,t.status,t.source_version,
		t.pickup_address,t.dropoff_address,t.pickup_lat::float8,t.pickup_lng::float8,t.dropoff_lat::float8,t.dropoff_lng::float8,
		b.currency,b.receiver_name,b.receiver_phone FROM trips t JOIN dispatch_bookings b ON b.id=t.booking_id WHERE t.id=$1::uuid`
	if providerID != "" {
		query += ` AND t.provider_id=$2::uuid`
	}
	if lock {
		return TripContext{}, errors.New("locking requires a transaction")
	}
	var item TripContext
	args := []any{tripID}
	if providerID != "" {
		args = append(args, providerID)
	}
	err := r.db.QueryRow(ctx, query, args...).Scan(&item.TripID, &item.BookingID, &item.ProviderID, &item.CustomerID,
		&item.Status, &item.SourceVersion, &item.PickupAddress, &item.DropoffAddress, &item.PickupLat, &item.PickupLng, &item.DropoffLat, &item.DropoffLng,
		&item.Currency, &item.ReceiverName, &item.ReceiverPhone)
	if errors.Is(err, pgx.ErrNoRows) {
		return TripContext{}, apperrors.NotFound("Trip not found.", nil)
	}
	return item, err
}

func (r *Repository) StartAttempt(ctx context.Context, command StartAttemptCommand, grace time.Duration, now time.Time, fp string) (Attempt, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Attempt{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if existing, operation, stored, found, err := findAttemptByKey(ctx, tx, command.IdempotencyKey); err != nil {
		return Attempt{}, err
	} else if found {
		if operation != "START" || stored != fp || existing.TripID != command.TripID {
			return Attempt{}, apperrors.Conflict("Idempotency key was reused with different delivery-attempt data.", nil)
		}
		existing.Replayed = true
		return existing, tx.Commit(ctx)
	}
	trip, err := lockTrip(ctx, tx, command.TripID, command.ProviderID)
	if err != nil {
		return Attempt{}, err
	}
	if trip.Status != "in_progress" && trip.Status != "proof_submitted" {
		return Attempt{}, apperrors.Conflict("A delivery attempt requires an in-progress trip.", nil)
	}
	var number int
	if err = tx.QueryRow(ctx, `SELECT COALESCE(max(attempt_number),0)+1 FROM dispatch_delivery_attempts WHERE trip_id=$1::uuid`, trip.TripID).Scan(&number); err != nil {
		return Attempt{}, err
	}
	item := Attempt{ID: uuid.NewString(), TripID: trip.TripID, BookingID: trip.BookingID, ProviderID: trip.ProviderID,
		CustomerID: trip.CustomerID, AttemptNumber: number, State: AttemptGracePeriod, ArrivedAt: now,
		GraceExpiresAt: now.Add(grace), PolicyVersion: PolicyVersion, SourceVersion: trip.SourceVersion}
	_, err = tx.Exec(ctx, `INSERT INTO dispatch_delivery_attempts(id,trip_id,provider_id,attempt_number,state,arrived_at,grace_expires_at,policy_version,source_version,idempotency_key,request_fingerprint)
		VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5,$6,$7,$8,$9,$10,$11)`, item.ID, item.TripID, item.ProviderID, item.AttemptNumber,
		item.State, item.ArrivedAt, item.GraceExpiresAt, item.PolicyVersion, item.SourceVersion, command.IdempotencyKey, fp)
	if err != nil {
		return Attempt{}, operationConflict(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO dispatch_delivery_attempt_operations(delivery_attempt_id,operation_type,idempotency_key,request_fingerprint)
		VALUES($1::uuid,'START',$2,$3)`, item.ID, command.IdempotencyKey, fp); err != nil {
		return Attempt{}, operationConflict(err)
	}
	if err = emit(ctx, tx, "dispatch.delivery_attempt.started.v1", "trip", item.TripID, item.SourceVersion, item); err != nil {
		return Attempt{}, err
	}
	return item, tx.Commit(ctx)
}

func (r *Repository) AppendContact(ctx context.Context, command ContactCommand, now time.Time, fp string) (ContactAttempt, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ContactAttempt{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var item ContactAttempt
	err = tx.QueryRow(ctx, `SELECT id::text,delivery_attempt_id::text,method,outcome,attempted_at FROM dispatch_delivery_contact_attempts WHERE idempotency_key=$1`, command.IdempotencyKey).
		Scan(&item.ID, &item.AttemptID, &item.Method, &item.Outcome, &item.AttemptedAt)
	if err == nil {
		var stored string
		if err = tx.QueryRow(ctx, `SELECT request_fingerprint FROM dispatch_delivery_contact_attempts WHERE id=$1::uuid`, item.ID).Scan(&stored); err != nil {
			return ContactAttempt{}, err
		}
		if stored != fp {
			return ContactAttempt{}, apperrors.Conflict("Idempotency key was reused with different contact data.", nil)
		}
		item.Replayed = true
		return item, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ContactAttempt{}, err
	}
	var state, provider string
	err = tx.QueryRow(ctx, `SELECT state,provider_id::text FROM dispatch_delivery_attempts WHERE id=$1::uuid FOR UPDATE`, command.AttemptID).Scan(&state, &provider)
	if errors.Is(err, pgx.ErrNoRows) {
		return ContactAttempt{}, apperrors.NotFound("Delivery attempt not found.", nil)
	}
	if err != nil {
		return ContactAttempt{}, err
	}
	if provider != command.ProviderID {
		return ContactAttempt{}, apperrors.Forbidden("This delivery attempt belongs to another provider.", nil)
	}
	if state != string(AttemptGracePeriod) {
		return ContactAttempt{}, apperrors.Conflict("Contact attempts are only accepted during the grace period.", nil)
	}
	item = ContactAttempt{ID: uuid.NewString(), AttemptID: command.AttemptID, Method: command.Method, Outcome: command.Outcome, AttemptedAt: now}
	_, err = tx.Exec(ctx, `INSERT INTO dispatch_delivery_contact_attempts(id,delivery_attempt_id,provider_id,method,outcome,attempted_at,idempotency_key,request_fingerprint)
		VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5,$6,$7,$8)`, item.ID, item.AttemptID, command.ProviderID, item.Method, item.Outcome, now, command.IdempotencyKey, fp)
	if err != nil {
		return ContactAttempt{}, operationConflict(err)
	}
	return item, tx.Commit(ctx)
}

func (r *Repository) FailAttempt(ctx context.Context, command FailAttemptCommand, actions []RecoveryAction, now time.Time, fp string) (Attempt, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Attempt{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if existing, operation, stored, found, err := findAttemptByKey(ctx, tx, command.IdempotencyKey); err != nil {
		return Attempt{}, err
	} else if found {
		if operation != "FAIL" || stored != fp || existing.ID != command.AttemptID {
			return Attempt{}, apperrors.Conflict("Idempotency key was reused with different attempt-failure data.", nil)
		}
		existing.Replayed = true
		return existing, tx.Commit(ctx)
	}
	var item Attempt
	var storedKey, storedFP string
	err = tx.QueryRow(ctx, `SELECT a.id::text,a.trip_id::text,t.booking_id::text,a.provider_id::text,t.customer_id::text,a.attempt_number,a.state,a.arrived_at,a.grace_expires_at,a.failure_reason,a.allowed_actions,a.policy_version,a.source_version,a.idempotency_key,a.request_fingerprint
		FROM dispatch_delivery_attempts a JOIN trips t ON t.id=a.trip_id WHERE a.id=$1::uuid FOR UPDATE`, command.AttemptID).
		Scan(&item.ID, &item.TripID, &item.BookingID, &item.ProviderID, &item.CustomerID, &item.AttemptNumber, &item.State, &item.ArrivedAt,
			&item.GraceExpiresAt, &item.FailureReason, &item.AllowedActions, &item.PolicyVersion, &item.SourceVersion, &storedKey, &storedFP)
	if errors.Is(err, pgx.ErrNoRows) {
		return Attempt{}, apperrors.NotFound("Delivery attempt not found.", nil)
	}
	if err != nil {
		return Attempt{}, err
	}
	if item.ProviderID != command.ProviderID {
		return Attempt{}, apperrors.Forbidden("This delivery attempt belongs to another provider.", nil)
	}
	if item.State != AttemptGracePeriod {
		return Attempt{}, apperrors.Conflict("Delivery attempt cannot be failed from its current state.", nil)
	}
	if now.Before(item.GraceExpiresAt) {
		return Attempt{}, apperrors.New(409, "RECEIVER_GRACE_PERIOD_ACTIVE", "The receiver grace period has not elapsed.", nil)
	}
	var contactCount int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM dispatch_delivery_contact_attempts WHERE delivery_attempt_id=$1::uuid`, item.ID).Scan(&contactCount); err != nil {
		return Attempt{}, err
	}
	if contactCount == 0 {
		return Attempt{}, apperrors.New(409, "CONTACT_ATTEMPT_REQUIRED", "At least one contact attempt is required.", nil)
	}
	raw, _ := json.Marshal(actions)
	item.State, item.FailureReason, item.AllowedActions, item.SourceVersion = AttemptReceiverUnavailable, command.ReasonCode, actions, item.SourceVersion+1
	_, err = tx.Exec(ctx, `UPDATE dispatch_delivery_attempts SET state=$2,failure_reason=$3,allowed_actions=$4,source_version=$5,
		updated_at=$6 WHERE id=$1::uuid`, item.ID, item.State, item.FailureReason, raw, item.SourceVersion, now)
	if err != nil {
		return Attempt{}, operationConflict(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO dispatch_delivery_attempt_operations(delivery_attempt_id,operation_type,idempotency_key,request_fingerprint)
		VALUES($1::uuid,'FAIL',$2,$3)`, item.ID, command.IdempotencyKey, fp); err != nil {
		return Attempt{}, operationConflict(err)
	}
	if err = emit(ctx, tx, "dispatch.receiver_unavailable.recorded.v1", "trip", item.TripID, item.SourceVersion, item); err != nil {
		return Attempt{}, err
	}
	return item, tx.Commit(ctx)
}

func (r *Repository) LatestUnavailableForBooking(ctx context.Context, bookingID, customerID string) (Attempt, error) {
	var item Attempt
	err := r.db.QueryRow(ctx, `SELECT a.id::text,a.trip_id::text,t.booking_id::text,a.provider_id::text,t.customer_id::text,a.attempt_number,a.state,a.arrived_at,a.grace_expires_at,a.failure_reason,a.allowed_actions,a.policy_version,a.source_version
		FROM dispatch_delivery_attempts a JOIN trips t ON t.id=a.trip_id WHERE t.booking_id=$1::uuid AND t.customer_id=$2::uuid AND a.state='RECEIVER_UNAVAILABLE' ORDER BY a.attempt_number DESC LIMIT 1`, bookingID, customerID).
		Scan(&item.ID, &item.TripID, &item.BookingID, &item.ProviderID, &item.CustomerID, &item.AttemptNumber, &item.State, &item.ArrivedAt,
			&item.GraceExpiresAt, &item.FailureReason, &item.AllowedActions, &item.PolicyVersion, &item.SourceVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return Attempt{}, apperrors.NotFound("No receiver-unavailable attempt was found for this booking.", nil)
	}
	return item, err
}

func (r *Repository) FindRecovery(ctx context.Context, key string) (RecoveryResult, string, bool, error) {
	var item RecoveryResult
	var fp string
	err := r.db.QueryRow(ctx, `SELECT id::text,delivery_attempt_id::text,trip_id::text,booking_id::text,action,state,scheduled_for,
		alternative_receiver_ref,hub_ref,financial_operation_ref,financial_state,policy_version,request_fingerprint FROM dispatch_delivery_recovery_actions WHERE idempotency_key=$1`, key).
		Scan(&item.ID, &item.AttemptID, &item.TripID, &item.BookingID, &item.Action, &item.State, &item.ScheduledFor,
			&item.AlternativeReceiverRef, &item.HubRef, &item.FinancialOperationRef, &item.FinancialState, &item.PolicyVersion, &fp)
	if errors.Is(err, pgx.ErrNoRows) {
		return RecoveryResult{}, "", false, nil
	}
	if err == nil {
		_ = r.db.QueryRow(ctx, `SELECT COALESCE(return_mission_id::text,''),COALESCE(return_booking_id::text,'') FROM dispatch_return_workflows WHERE recovery_action_id=$1::uuid`, item.ID).Scan(&item.ReturnMissionID, &item.ReturnBookingID)
	}
	return item, fp, err == nil, err
}

func (r *Repository) SelectRecovery(ctx context.Context, attempt Attempt, command SelectRecoveryCommand, receiverRef string, route *RouteDecision, now time.Time, fp string) (RecoveryResult, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return RecoveryResult{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	trip, err := lockTrip(ctx, tx, attempt.TripID, "")
	if err != nil {
		return RecoveryResult{}, err
	}
	if trip.CustomerID != command.CustomerID || trip.BookingID != command.BookingID {
		return RecoveryResult{}, apperrors.NotFound("Booking was not found.", nil)
	}
	if command.ExpectedTripVersion > 0 && command.ExpectedTripVersion != trip.SourceVersion {
		return RecoveryResult{}, apperrors.Conflict("Trip changed. Reload and try again.", nil)
	}
	var state string
	if err = tx.QueryRow(ctx, `SELECT state FROM dispatch_delivery_attempts WHERE id=$1::uuid FOR UPDATE`, attempt.ID).Scan(&state); err != nil {
		return RecoveryResult{}, err
	}
	if state != string(AttemptReceiverUnavailable) {
		return RecoveryResult{}, apperrors.Conflict("A recovery action has already been selected.", nil)
	}
	recoveryState := "READY"
	if command.Action == Reschedule || command.Action == RetrySameDay {
		recoveryState = "SCHEDULED"
	}
	if command.Action == ReturnToSender {
		recoveryState = "READY"
	}
	if command.Action == HubHold {
		recoveryState = "READY"
	}
	item := RecoveryResult{ID: uuid.NewString(), AttemptID: attempt.ID, TripID: trip.TripID, BookingID: trip.BookingID,
		Action: command.Action, State: recoveryState, ScheduledFor: command.ScheduledFor, AlternativeReceiverRef: receiverRef,
		HubRef: strings.TrimSpace(command.HubRef), FinancialState: "NOT_REQUIRED", FinancialOperationRef: strings.TrimSpace(command.FinancialAuthorizationRef), PolicyVersion: PolicyVersion}
	if command.AdditionalFeeKobo > 0 {
		item.FinancialState = "APPROVED"
	}
	_, err = tx.Exec(ctx, `INSERT INTO dispatch_delivery_recovery_actions(id,delivery_attempt_id,trip_id,booking_id,customer_id,action,state,scheduled_for,
		alternative_receiver_ref,hub_ref,financial_operation_ref,financial_state,policy_version,expected_trip_version,idempotency_key,request_fingerprint)
		VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`, item.ID, item.AttemptID,
		item.TripID, item.BookingID, command.CustomerID, item.Action, item.State, item.ScheduledFor, receiverRef, item.HubRef,
		strings.TrimSpace(command.FinancialAuthorizationRef), item.FinancialState, item.PolicyVersion, trip.SourceVersion, command.IdempotencyKey, fp)
	if err != nil {
		return RecoveryResult{}, operationConflict(err)
	}
	if command.Action == AlternativeReceiver {
		if _, err = tx.Exec(ctx, `UPDATE dispatch_bookings SET receiver_phone=$2,updated_at=$3 WHERE id=$1::uuid`, trip.BookingID, command.AlternativeReceiverContact, now); err != nil {
			return RecoveryResult{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE trips SET receiver_phone=$2,source_version=source_version+1,updated_at=$3 WHERE id=$1::uuid`, trip.TripID, command.AlternativeReceiverContact, now); err != nil {
			return RecoveryResult{}, err
		}
	}
	if command.Action == ReturnToSender {
		if route == nil {
			return RecoveryResult{}, apperrors.Unavailable("Return route authority is unavailable.", nil)
		}
		returnID, returnMission, returnBooking, err := createReturnWorkflow(ctx, tx, item, trip, command, *route, now, fp)
		if err != nil {
			return RecoveryResult{}, err
		}
		_ = returnID
		item.ReturnMissionID = returnMission
		item.ReturnBookingID = returnBooking
	}
	if command.Action == HubHold {
		if strings.TrimSpace(command.HubRef) == "" || strings.TrimSpace(command.HubAuthorityRef) == "" {
			return RecoveryResult{}, apperrors.New(409, "HUB_AUTHORITY_REQUIRED", "An approved hub authority is required.", nil)
		}
		_, err = tx.Exec(ctx, `INSERT INTO dispatch_hub_hold_operations(recovery_action_id,trip_id,hub_ref,hub_authority_ref,state,expires_at)
			VALUES($1::uuid,$2::uuid,$3,$4,'APPROVED',$5)`, item.ID, trip.TripID, command.HubRef, command.HubAuthorityRef, now.Add(24*time.Hour))
		if err != nil {
			return RecoveryResult{}, err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE dispatch_delivery_attempts SET state='RECOVERY_SELECTED',source_version=source_version+1,updated_at=$2 WHERE id=$1::uuid`, attempt.ID, now); err != nil {
		return RecoveryResult{}, err
	}
	if err = emit(ctx, tx, "dispatch.delivery_recovery.selected.v1", "trip", trip.TripID, trip.SourceVersion+1, item); err != nil {
		return RecoveryResult{}, err
	}
	eventType := map[RecoveryAction]string{
		RetrySameDay:        "dispatch.delivery.retry_scheduled.v1",
		Reschedule:          "dispatch.delivery.rescheduled.v1",
		AlternativeReceiver: "dispatch.delivery.alternative_receiver_selected.v1",
		ReturnToSender:      "dispatch.return.started.v1",
		HubHold:             "dispatch.hub_hold.started.v1",
		OperationsReview:    "dispatch.delivery.operations_review_requested.v1",
	}[command.Action]
	if eventType != "" {
		if err = emit(ctx, tx, eventType, "trip", trip.TripID, trip.SourceVersion+1, item); err != nil {
			return RecoveryResult{}, err
		}
	}
	return item, tx.Commit(ctx)
}

func (r *Repository) ReportIncident(ctx context.Context, command IncidentCommand, now time.Time, fp string) (Incident, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Incident{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var item Incident
	var stored string
	err = tx.QueryRow(ctx, `SELECT id::text,trip_id::text,reason_code,state,replacement_strategy,source_version,created_at,request_fingerprint
		FROM dispatch_delivery_incidents WHERE idempotency_key=$1`, command.IdempotencyKey).Scan(&item.ID, &item.TripID, &item.ReasonCode,
		&item.State, &item.ReplacementStrategy, &item.SourceVersion, &item.CreatedAt, &stored)
	if err == nil {
		if stored != fp {
			return Incident{}, apperrors.Conflict("Idempotency key was reused with different incident data.", nil)
		}
		item.Replayed = true
		_ = tx.QueryRow(ctx, `SELECT id::text FROM dispatch_delivery_missions WHERE trip_id=$1::uuid AND state='REASSIGNING' ORDER BY updated_at DESC LIMIT 1`, item.TripID).Scan(&item.MissionID)
		return item, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Incident{}, err
	}
	trip, err := lockTrip(ctx, tx, command.TripID, command.ProviderID)
	if err != nil {
		return Incident{}, err
	}
	if trip.Status == "completed" || trip.Status == "cancelled" || trip.Status == "failed" {
		return Incident{}, apperrors.Conflict("A terminal trip cannot enter replacement.", nil)
	}
	item = Incident{ID: uuid.NewString(), TripID: trip.TripID, ReasonCode: command.ReasonCode, State: "REASSIGNING",
		ReplacementStrategy: command.PreferredStrategy, SourceVersion: trip.SourceVersion + 1, CreatedAt: now}
	_, err = tx.Exec(ctx, `INSERT INTO dispatch_delivery_incidents(id,trip_id,provider_id,reason_code,state,replacement_strategy,safe_note,source_version,idempotency_key,request_fingerprint)
		VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5,$6,$7,$8,$9,$10)`, item.ID, item.TripID, command.ProviderID, item.ReasonCode,
		item.State, item.ReplacementStrategy, command.SafeNote, item.SourceVersion, command.IdempotencyKey, fp)
	if err != nil {
		return Incident{}, operationConflict(err)
	}
	_ = tx.QueryRow(ctx, `UPDATE dispatch_delivery_missions SET state='REASSIGNING',version=version+1,updated_at=$2 WHERE trip_id=$1::uuid AND state NOT IN ('DELIVERED','COMPLETED','CANCELLED','RETURNED') RETURNING id::text`, trip.TripID, now).Scan(&item.MissionID)
	_, _ = tx.Exec(ctx, `UPDATE commerce_missions SET runtime_state='REPLACEMENT_REQUIRED',replacement_state='REQUIRED',replacement_reason=$2,
		source_version=source_version+1,updated_at=$3 WHERE trip_id=$1::uuid AND runtime_state NOT IN ('COMPLETED','CANCELLED')`, trip.TripID, item.ReasonCode, now)
	if err = emit(ctx, tx, "dispatch.delivery_incident.reported.v1", "trip", trip.TripID, item.SourceVersion, item); err != nil {
		return Incident{}, err
	}
	if err = emit(ctx, tx, "dispatch.provider_replacement.started.v1", "trip", trip.TripID, item.SourceVersion, item); err != nil {
		return Incident{}, err
	}
	return item, tx.Commit(ctx)
}

func (r *Repository) HubCheckpointContext(ctx context.Context, command HubCheckpointCommand) (HubCheckpointContext, error) {
	var item HubCheckpointContext
	err := r.db.QueryRow(ctx, `SELECT h.id::text,h.hub_ref,h.state,
		COALESCE(dm.id::text,'trip:'||t.id::text),COALESCE(pm.manifest_ref,'')
		FROM dispatch_hub_hold_operations h
		JOIN trips t ON t.id=h.trip_id
		LEFT JOIN dispatch_delivery_missions dm ON dm.trip_id=t.id
		LEFT JOIN dispatch_parcel_manifests pm ON pm.mission_id=dm.id AND pm.status IN ('ACTIVE','CUSTODY_ACTIVE')
		WHERE h.trip_id=$1::uuid AND t.provider_id=$2::uuid
		ORDER BY pm.version DESC LIMIT 1`, command.TripID, command.ProviderID).
		Scan(&item.OperationID, &item.HubRef, &item.State, &item.MissionRef, &item.ManifestRef)
	if errors.Is(err, pgx.ErrNoRows) {
		return item, apperrors.NotFound("Approved hub hold was not found.", nil)
	}
	if err != nil {
		return item, err
	}
	if item.ManifestRef == "" {
		return item, apperrors.New(409, "MANIFEST_REQUIRED", "Hub custody requires an active parcel manifest.", nil)
	}
	rows, err := r.db.Query(ctx, `SELECT p.parcel_ref,p.scannable_code FROM dispatch_parcel_manifests pm
		JOIN dispatch_manifest_parcels mp ON mp.manifest_id=pm.id JOIN dispatch_parcels p ON p.id=mp.parcel_id
		WHERE pm.manifest_ref=$1 AND pm.status IN ('ACTIVE','CUSTODY_ACTIVE') ORDER BY mp.ordinal`, item.ManifestRef)
	if err != nil {
		return item, err
	}
	defer rows.Close()
	for rows.Next() {
		var ref, code string
		if err = rows.Scan(&ref, &code); err != nil {
			return item, err
		}
		item.ParcelRefs = append(item.ParcelRefs, ref)
		item.ParcelCodes = append(item.ParcelCodes, code)
	}
	if err = rows.Err(); err != nil {
		return item, err
	}
	if len(item.ParcelCodes) == 0 {
		return item, apperrors.New(409, "PARCEL_MANIFEST_EMPTY", "Hub custody requires manifest parcels.", nil)
	}
	return item, nil
}

func (r *Repository) RecordHubCheckpoint(ctx context.Context, command HubCheckpointCommand, context HubCheckpointContext, now time.Time, fp string) (HubCheckpoint, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return HubCheckpoint{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var existing HubCheckpoint
	var stored string
	err = tx.QueryRow(ctx, `SELECT c.id::text,c.trip_id::text,h.hub_ref,c.stage,h.state,c.media_ref,c.recorded_at,c.request_fingerprint
		FROM dispatch_hub_hold_checkpoints c JOIN dispatch_hub_hold_operations h ON h.id=c.hub_hold_operation_id
		WHERE c.idempotency_key=$1`, command.IdempotencyKey).
		Scan(&existing.OperationID, &existing.TripID, &existing.HubRef, &existing.Stage, &existing.State, &existing.EvidenceRef, &existing.RecordedAt, &stored)
	if err == nil {
		if stored != fp {
			return HubCheckpoint{}, apperrors.Conflict("Idempotency key was reused with different hub evidence.", nil)
		}
		existing.Replayed = true
		return existing, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return HubCheckpoint{}, err
	}
	var state string
	if err = tx.QueryRow(ctx, `SELECT state FROM dispatch_hub_hold_operations WHERE id=$1::uuid FOR UPDATE`, context.OperationID).Scan(&state); err != nil {
		return HubCheckpoint{}, err
	}
	next := ""
	if command.Stage == "ARRIVAL" && (state == "APPROVED" || state == "EN_ROUTE" || state == "CUSTODY_PENDING") {
		next = "HELD"
	}
	if command.Stage == "RELEASE" && (state == "HELD" || state == "PICKUP_PENDING") {
		next = "RELEASED"
	}
	if next == "" {
		return HubCheckpoint{}, apperrors.Conflict("Hub custody stage conflicts with current state.", nil)
	}
	item := HubCheckpoint{OperationID: uuid.NewString(), TripID: command.TripID, HubRef: context.HubRef, Stage: command.Stage,
		State: next, EvidenceRef: command.MediaRef, RecordedAt: now}
	_, err = tx.Exec(ctx, `INSERT INTO dispatch_hub_hold_checkpoints(id,hub_hold_operation_id,trip_id,provider_id,stage,manifest_ref,parcel_codes,media_ref,idempotency_key,request_fingerprint,recorded_at)
		VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5,$6,$7,$8,$9,$10,$11)`, item.OperationID, context.OperationID, command.TripID, command.ProviderID,
		command.Stage, context.ManifestRef, command.ParcelCodes, command.MediaRef, command.IdempotencyKey, fp, now)
	if err != nil {
		return HubCheckpoint{}, operationConflict(err)
	}
	_, err = tx.Exec(ctx, `UPDATE dispatch_hub_hold_operations SET state=$2,custody_operation_ref=$3,evidence_ref=$4,updated_at=$5 WHERE id=$1::uuid`,
		context.OperationID, next, command.IdempotencyKey, command.MediaRef, now)
	if err != nil {
		return HubCheckpoint{}, err
	}
	if err = emit(ctx, tx, "dispatch.hub_hold.checkpoint_recorded.v1", "trip", command.TripID, now.UnixNano(), item); err != nil {
		return HubCheckpoint{}, err
	}
	if command.Stage == "RELEASE" {
		if err = emit(ctx, tx, "dispatch.hub_hold.released.v1", "trip", command.TripID, now.UnixNano(), item); err != nil {
			return HubCheckpoint{}, err
		}
	}
	return item, tx.Commit(ctx)
}

func (r *Repository) RecordEmergencyRecovery(ctx context.Context, command EmergencyRecoveryCommand, now time.Time, fp string) (EmergencyRecovery, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return EmergencyRecovery{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var item EmergencyRecovery
	var stored string
	err = tx.QueryRow(ctx, `SELECT id::text,trip_id::text,incident_id::text,actor_ref,reason_code,state,created_at,request_fingerprint
		FROM dispatch_emergency_recovery_actions WHERE idempotency_key=$1`, command.IdempotencyKey).
		Scan(&item.ID, &item.TripID, &item.IncidentID, &item.ActorRef, &item.ReasonCode, &item.State, &item.CreatedAt, &stored)
	if err == nil {
		if stored != fp {
			return item, apperrors.Conflict("Idempotency key was reused with different emergency recovery data.", nil)
		}
		item.Replayed = true
		return item, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return item, err
	}
	var incidentID string
	var tripVersion int64
	err = tx.QueryRow(ctx, `SELECT i.id::text,t.source_version FROM dispatch_delivery_incidents i JOIN trips t ON t.id=i.trip_id
		WHERE i.trip_id=$1::uuid AND i.state NOT IN ('RESOLVED','CANCELLED','EMERGENCY_RECOVERY') AND t.status NOT IN ('completed','cancelled','failed')
		ORDER BY i.created_at DESC LIMIT 1 FOR UPDATE OF i,t`, command.TripID).Scan(&incidentID, &tripVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return item, apperrors.NotFound("An active delivery incident was not found.", nil)
	}
	if err != nil {
		return item, err
	}
	if tripVersion != command.ExpectedTripVersion {
		return item, apperrors.Conflict("Trip version has changed.", nil)
	}
	item = EmergencyRecovery{ID: uuid.NewString(), TripID: command.TripID, IncidentID: incidentID, ActorRef: command.ActorRef,
		ReasonCode: command.ReasonCode, State: "OPERATIONS_CONTROLLED", CreatedAt: now}
	_, err = tx.Exec(ctx, `INSERT INTO dispatch_emergency_recovery_actions(id,incident_id,trip_id,actor_ref,reason_code,safe_note,state,idempotency_key,request_fingerprint,created_at)
		VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5,$6,$7,$8,$9,$10)`, item.ID, item.IncidentID, item.TripID, item.ActorRef,
		item.ReasonCode, command.SafeNote, item.State, command.IdempotencyKey, fp, now)
	if err != nil {
		return item, operationConflict(err)
	}
	if _, err = tx.Exec(ctx, `UPDATE dispatch_delivery_incidents SET state='EMERGENCY_RECOVERY',updated_at=$2 WHERE id=$1::uuid`, incidentID, now); err != nil {
		return item, err
	}
	if err = emit(ctx, tx, "dispatch.emergency_recovery.controlled.v1", "trip", command.TripID, now.UnixNano(), item); err != nil {
		return item, err
	}
	return item, tx.Commit(ctx)
}

func lockTrip(ctx context.Context, tx pgx.Tx, tripID, providerID string) (TripContext, error) {
	query := `SELECT t.id::text,t.booking_id::text,t.provider_id::text,t.customer_id::text,t.status,t.source_version,
		t.pickup_address,t.dropoff_address,t.pickup_lat::float8,t.pickup_lng::float8,t.dropoff_lat::float8,t.dropoff_lng::float8,
		b.currency,b.receiver_name,b.receiver_phone FROM trips t JOIN dispatch_bookings b ON b.id=t.booking_id WHERE t.id=$1::uuid`
	args := []any{tripID}
	if providerID != "" {
		query += ` AND t.provider_id=$2::uuid`
		args = append(args, providerID)
	}
	query += ` FOR UPDATE`
	var item TripContext
	err := tx.QueryRow(ctx, query, args...).Scan(&item.TripID, &item.BookingID, &item.ProviderID, &item.CustomerID, &item.Status,
		&item.SourceVersion, &item.PickupAddress, &item.DropoffAddress, &item.PickupLat, &item.PickupLng, &item.DropoffLat, &item.DropoffLng,
		&item.Currency, &item.ReceiverName, &item.ReceiverPhone)
	if errors.Is(err, pgx.ErrNoRows) {
		return TripContext{}, apperrors.NotFound("Trip not found.", nil)
	}
	return item, err
}

func findAttemptByKey(ctx context.Context, tx pgx.Tx, key string) (Attempt, string, string, bool, error) {
	var item Attempt
	var operation, fingerprint string
	err := tx.QueryRow(ctx, `SELECT a.id::text,a.trip_id::text,t.booking_id::text,a.provider_id::text,t.customer_id::text,a.attempt_number,a.state,a.arrived_at,a.grace_expires_at,a.failure_reason,a.allowed_actions,a.policy_version,a.source_version,o.operation_type,o.request_fingerprint
		FROM dispatch_delivery_attempt_operations o JOIN dispatch_delivery_attempts a ON a.id=o.delivery_attempt_id JOIN trips t ON t.id=a.trip_id WHERE o.idempotency_key=$1`, key).Scan(&item.ID, &item.TripID,
		&item.BookingID, &item.ProviderID, &item.CustomerID, &item.AttemptNumber, &item.State, &item.ArrivedAt, &item.GraceExpiresAt,
		&item.FailureReason, &item.AllowedActions, &item.PolicyVersion, &item.SourceVersion, &operation, &fingerprint)
	if errors.Is(err, pgx.ErrNoRows) {
		return Attempt{}, "", "", false, nil
	}
	return item, operation, fingerprint, err == nil, err
}

func createReturnWorkflow(ctx context.Context, tx pgx.Tx, recovery RecoveryResult, trip TripContext, command SelectRecoveryCommand, route RouteDecision, now time.Time, fp string) (string, string, string, error) {
	var groupID, originalMission, sourceType string
	err := tx.QueryRow(ctx, `SELECT dm.delivery_group_id::text,dm.id::text,dm.source_type FROM dispatch_delivery_missions dm
		LEFT JOIN dispatch_mission_execution_links link ON link.mission_id=dm.id WHERE dm.trip_id=$1::uuid OR link.trip_id=$1::uuid LIMIT 1`, trip.TripID).
		Scan(&groupID, &originalMission, &sourceType)
	if errors.Is(err, pgx.ErrNoRows) {
		groupID, sourceType = uuid.NewString(), "CUSTOMER"
		_, err = tx.Exec(ctx, `INSERT INTO dispatch_delivery_groups(id,customer_id,source_type,source_ref,source_version,booking_id,trip_id,state,version,created_at,updated_at)
			VALUES($1::uuid,$2::uuid,'CUSTOMER',$3,1,$4::uuid,$5::uuid,'RETURNING',1,$6,$6)`, groupID, trip.CustomerID, "return:"+trip.BookingID, trip.BookingID, trip.TripID, now)
		if err != nil {
			return "", "", "", err
		}
		parcelID := uuid.NewString()
		_, err = tx.Exec(ctx, `INSERT INTO dispatch_parcels(id,delivery_group_id,parcel_ref,declaration,package_policy_version,state,karrygo_parcel_ref,scannable_code)
			VALUES($1::uuid,$2::uuid,$3,'{}','dispatch_legacy_return_v1','RETURNING',$4,$5)`, parcelID, groupID, "booking:"+trip.BookingID,
			"KGP-"+strings.ToUpper(strings.ReplaceAll(parcelID, "-", "")), "kgp_scan-"+strings.ReplaceAll(parcelID, "-", ""))
		if err != nil {
			return "", "", "", err
		}
	} else if err != nil {
		return "", "", "", err
	}
	returnMission := uuid.NewString()
	logical := "return:" + trip.TripID
	_, err = tx.Exec(ctx, `INSERT INTO dispatch_delivery_missions(id,delivery_group_id,source_type,source_ref,source_version,state,version,logical_mission_ref,created_at,updated_at)
		VALUES($1::uuid,$2::uuid,$3,$4,1,'PLANNED',1,$5,$6,$6)`, returnMission, groupID, sourceType, logical, logical, now)
	if err != nil {
		return "", "", "", err
	}
	currentLocation, _ := json.Marshal(map[string]any{"address": trip.DropoffAddress, "latitude": trip.DropoffLat, "longitude": trip.DropoffLng})
	returnLocation, _ := json.Marshal(map[string]any{"address": trip.PickupAddress, "latitude": trip.PickupLat, "longitude": trip.PickupLng})
	_, err = tx.Exec(ctx, `INSERT INTO dispatch_mission_stops(mission_id,sequence,stop_type,location_snapshot,version,stop_ref)
		VALUES($1::uuid,1,'PICKUP',$2,1,$3),($1::uuid,2,'RETURN',$4,1,$5)`, returnMission, currentLocation,
		"return-origin:"+trip.TripID, returnLocation, "return-destination:"+trip.TripID)
	if err != nil {
		return "", "", "", err
	}
	manifestID, manifestRef := uuid.NewString(), "return-manifest:"+trip.TripID
	_, err = tx.Exec(ctx, `INSERT INTO dispatch_parcel_manifests(id,mission_id,manifest_ref,version,status,policy_version)
		VALUES($1::uuid,$2::uuid,$3,1,'ACTIVE','dispatch_return_manifest_v1')`, manifestID, returnMission, manifestRef)
	if err != nil {
		return "", "", "", err
	}
	rows, err := tx.Query(ctx, `SELECT id::text FROM dispatch_parcels WHERE delivery_group_id=$1::uuid ORDER BY created_at,id`, groupID)
	if err != nil {
		return "", "", "", err
	}
	defer rows.Close()
	parcelIDs := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return "", "", "", err
		}
		parcelIDs = append(parcelIDs, id)
	}
	if err = rows.Err(); err != nil {
		return "", "", "", err
	}
	for i, id := range parcelIDs {
		if _, err = tx.Exec(ctx, `INSERT INTO dispatch_manifest_parcels(manifest_id,parcel_id,ordinal) VALUES($1::uuid,$2::uuid,$3)`, manifestID, id, i+1); err != nil {
			return "", "", "", err
		}
	}
	var returnStopID string
	if err = tx.QueryRow(ctx, `SELECT id::text FROM dispatch_mission_stops WHERE mission_id=$1::uuid AND stop_type='RETURN'`, returnMission).Scan(&returnStopID); err != nil {
		return "", "", "", err
	}
	for _, id := range parcelIDs {
		if _, err = tx.Exec(ctx, `INSERT INTO dispatch_stop_expected_parcels(mission_stop_id,parcel_id,manifest_id) VALUES($1::uuid,$2::uuid,$3::uuid)`, returnStopID, id, manifestID); err != nil {
			return "", "", "", err
		}
	}
	// Physical custody does not disappear when the receiver is unavailable.
	// Move the active projection to the linked return manifest atomically; a
	// parcel already held by an unrelated manifest causes the transaction to
	// fail through the parcel primary key.
	if originalMission != "" {
		if _, err = tx.Exec(ctx, `UPDATE dispatch_active_custody_manifest_parcels SET manifest_id=$2::uuid,mission_id=$3::uuid
			WHERE parcel_id=ANY($1::uuid[]) AND mission_id=$4::uuid`, parcelIDs, manifestID, returnMission, originalMission); err != nil {
			return "", "", "", err
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO dispatch_active_custody_manifest_parcels(parcel_id,manifest_id,mission_id,activated_at)
		SELECT unnest($1::uuid[]),$2::uuid,$3::uuid,$4 ON CONFLICT(parcel_id) DO NOTHING`, parcelIDs, manifestID, returnMission, now); err != nil {
		return "", "", "", err
	}
	var active int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM dispatch_active_custody_manifest_parcels WHERE manifest_id=$1::uuid AND mission_id=$2::uuid`, manifestID, returnMission).Scan(&active); err != nil {
		return "", "", "", err
	}
	if active != len(parcelIDs) {
		return "", "", "", apperrors.Conflict("Parcel custody belongs to another active mission.", nil)
	}
	if _, err = tx.Exec(ctx, `UPDATE dispatch_parcel_manifests SET status='CUSTODY_ACTIVE' WHERE id=$1::uuid`, manifestID); err != nil {
		return "", "", "", err
	}
	returnBooking := uuid.NewString()
	paymentState := "paid"
	if command.AdditionalFeeKobo > 0 {
		paymentState = "held"
	}
	distanceKM := float64(route.DistanceMeters) / 1000
	insertBooking, err := tx.Exec(ctx, `INSERT INTO dispatch_bookings(id,customer_id,status,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,
		distance_km,fare_amount,currency,payment_method,payment_status,payment_ref,receiver_name,receiver_phone,package_desc,package_type,package_size,service_tier,
		rollout_protection_mode,rollout_policy_version,handover_required,pickup_evidence_required,delivery_evidence_required,provider_transfer_required)
		SELECT $1::uuid,$2::uuid,'searching',$3,$4,$5,$6,$7,$8,$9,$10,$11,'wallet',$12,$13,'Original sender','',$14,'return_manifest','large','standard',
			b.rollout_protection_mode,b.rollout_policy_version,b.handover_required,b.pickup_evidence_required,b.delivery_evidence_required,b.provider_transfer_required
		FROM dispatch_bookings b WHERE b.id=$15::uuid`,
		returnBooking, trip.CustomerID, trip.DropoffAddress, trip.DropoffLat, trip.DropoffLng, trip.PickupAddress, trip.PickupLat, trip.PickupLng,
		distanceKM, command.AdditionalFeeKobo, trip.Currency, paymentState, command.FinancialAuthorizationRef, "Return parcels "+strings.Join(parcelIDs, ","), trip.BookingID)
	if err != nil {
		return "", "", "", err
	}
	if insertBooking.RowsAffected() != 1 {
		return "", "", "", errors.New("original booking protection snapshot unavailable")
	}
	payload, _ := json.Marshal(map[string]any{"booking_id": returnBooking, "customer_id": trip.CustomerID, "pickup_lat": trip.DropoffLat,
		"pickup_lng": trip.DropoffLng, "dropoff_lat": trip.PickupLat, "dropoff_lng": trip.PickupLng, "pickup_address": trip.DropoffAddress,
		"dropoff_address": trip.PickupAddress, "fare_amount": command.AdditionalFeeKobo, "service_type": "dispatch", "payment_method": "wallet",
		"currency": trip.Currency, "receiver_name": "Original sender", "receiver_phone": "", "package_desc": "Return parcels " + strings.Join(parcelIDs, ","),
		"package_type": "return_manifest", "package_size": "large", "is_fragile": false, "service_tier": "standard", "occurred_at": now})
	if _, err = tx.Exec(ctx, `INSERT INTO request_broadcasts(booking_id,service_type,broadcast_radius_km,status,broadcast_at,expires_at,booking_payload)
		VALUES($1::uuid,'dispatch',1,'cancelled',$2,$2+interval '5 minutes',$3)`, returnBooking, now, payload); err != nil {
		return "", "", "", err
	}
	operation := command.IdempotencyKey + ":return-execution"
	if _, err = tx.Exec(ctx, `INSERT INTO dispatch_mission_execution_links(mission_id,booking_id,source_type,source_ref,source_version,operation_ref,request_fingerprint)
		VALUES($1::uuid,$2::uuid,$3,$4,1,$5,$6)`, returnMission, returnBooking, sourceType, logical, operation, fp); err != nil {
		return "", "", "", err
	}
	if _, err = tx.Exec(ctx, `UPDATE dispatch_delivery_missions SET booking_id=$2::uuid WHERE id=$1::uuid`, returnMission, returnBooking); err != nil {
		return "", "", "", err
	}
	routeRaw, _ := json.Marshal(route.Snapshot)
	returnID := uuid.NewString()
	_, err = tx.Exec(ctx, `INSERT INTO dispatch_return_workflows(id,recovery_action_id,original_trip_id,delivery_group_id,original_mission_id,return_mission_id,return_booking_id,parcel_ids,route_reference,route_snapshot,wallet_operation_ref,wallet_state,state,policy_version)
		VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,NULLIF($5,'')::uuid,$6::uuid,$7::uuid,$8::uuid[],$9,$10,$11,$12,'ROUTED',$13)`, returnID, recovery.ID,
		trip.TripID, groupID, originalMission, returnMission, returnBooking, parcelIDs, route.Reference, routeRaw, recovery.FinancialOperationRef, recovery.FinancialState, PolicyVersion)
	if err != nil {
		return "", "", "", err
	}
	_, _ = tx.Exec(ctx, `UPDATE dispatch_delivery_groups SET state='RETURNING',version=version+1,updated_at=$2 WHERE id=$1::uuid`, groupID, now)
	if originalMission != "" {
		_, _ = tx.Exec(ctx, `UPDATE dispatch_delivery_missions SET state='RETURNING',version=version+1,updated_at=$2 WHERE id=$1::uuid`, originalMission, now)
	}
	return returnID, returnMission, returnBooking, nil
}

// ActivateScheduled advances due retry/reschedule actions exactly once under
// a row lock. The transition and its durable event share the transaction.
func (r *Repository) ActivateScheduled(ctx context.Context, now time.Time, limit int) (int, error) {
	if limit <= 0 {
		limit = 100
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	rows, err := tx.Query(ctx, `SELECT a.id::text,a.trip_id::text,a.action,t.status,t.source_version
		FROM dispatch_delivery_recovery_actions a JOIN trips t ON t.id=a.trip_id
		WHERE a.state='SCHEDULED' AND a.scheduled_for<=$1
		ORDER BY a.scheduled_for,a.created_at FOR UPDATE OF t,a SKIP LOCKED LIMIT $2`, now, limit)
	if err != nil {
		return 0, err
	}
	type due struct {
		id, tripID string
		action     RecoveryAction
		tripStatus string
		version    int64
	}
	var items []due
	for rows.Next() {
		var x due
		if err = rows.Scan(&x.id, &x.tripID, &x.action, &x.tripStatus, &x.version); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, x)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return 0, err
	}
	for _, item := range items {
		if item.tripStatus == "completed" || item.tripStatus == "cancelled" {
			if _, err = tx.Exec(ctx, `UPDATE dispatch_delivery_recovery_actions SET state='CANCELLED',updated_at=$2 WHERE id=$1::uuid AND state='SCHEDULED'`, item.id, now); err != nil {
				return 0, err
			}
			continue
		}
		tag, updateErr := tx.Exec(ctx, `UPDATE dispatch_delivery_recovery_actions SET state='READY',updated_at=$2 WHERE id=$1::uuid AND state='SCHEDULED'`, item.id, now)
		if updateErr != nil {
			return 0, updateErr
		}
		if tag.RowsAffected() == 1 {
			// Close the previous attempt without fabricating a new physical arrival.
			// The assigned provider starts the next numbered attempt on arrival.
			if _, err = tx.Exec(ctx, `UPDATE dispatch_delivery_attempts SET state='RESOLVED',updated_at=$2
				WHERE id=(SELECT delivery_attempt_id FROM dispatch_delivery_recovery_actions WHERE id=$1::uuid)
				AND state='RECOVERY_SELECTED'`, item.id, now); err != nil {
				return 0, err
			}
			if err = emit(ctx, tx, "dispatch.delivery_recovery.activated.v1", "recovery_action", item.id, 1, map[string]any{"trip_id": item.tripID, "recovery_action_id": item.id, "action": item.action, "state": "READY", "activated_at": now}); err != nil {
				return 0, err
			}
		}
	}
	return len(items), tx.Commit(ctx)
}

func (r *Repository) ClaimReturnAssignments(ctx context.Context, now time.Time, owner string, lease time.Duration, maxAttempts, limit int) ([]PendingReturnAssignment, error) {
	if limit <= 0 {
		limit = 100
	}
	if maxAttempts <= 0 {
		maxAttempts = 10
	}
	rows, err := r.db.Query(ctx, `WITH due AS (
		SELECT w.id FROM dispatch_return_workflows w JOIN dispatch_delivery_missions m ON m.id=w.return_mission_id
		WHERE w.assignment_state IN ('PENDING','UNKNOWN','DELIVERING') AND w.assignment_attempt_count<$4
		  AND w.assignment_next_attempt_at<=$1 AND (w.assignment_lease_expires_at IS NULL OR w.assignment_lease_expires_at<=$1)
		  AND w.wallet_state IN ('NOT_REQUIRED','APPROVED','ACKNOWLEDGED') AND m.trip_id IS NULL
		  AND m.state IN ('PLANNED','OFFERING','QUEUED') AND w.state IN ('REQUIRED','ROUTED')
		ORDER BY w.assignment_next_attempt_at,w.created_at FOR UPDATE OF w SKIP LOCKED LIMIT $5
	)
	UPDATE dispatch_return_workflows w SET assignment_state='DELIVERING',assignment_lease_owner=$2,
		assignment_lease_expires_at=$1::timestamptz+$3::interval,updated_at=$1::timestamptz FROM due WHERE w.id=due.id
	RETURNING w.id::text,w.return_mission_id::text,(SELECT a.idempotency_key||':return-assignment' FROM dispatch_delivery_recovery_actions a WHERE a.id=w.recovery_action_id),w.assignment_attempt_count`, now, owner, lease.String(), maxAttempts, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []PendingReturnAssignment
	for rows.Next() {
		var item PendingReturnAssignment
		if err = rows.Scan(&item.WorkflowID, &item.MissionID, &item.OperationRef, &item.AttemptCount); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *Repository) CompleteReturnAssignment(ctx context.Context, workflowID string, acknowledged bool, code string, now time.Time, backoff time.Duration, maxAttempts int, owner string) error {
	if maxAttempts <= 0 {
		maxAttempts = 10
	}
	_, err := r.db.Exec(ctx, `UPDATE dispatch_return_workflows SET
		assignment_attempt_count=assignment_attempt_count+1,
		assignment_state=CASE WHEN $2 THEN 'ACKNOWLEDGED' WHEN assignment_attempt_count+1 >= $6 THEN 'DEAD_LETTER' ELSE 'UNKNOWN' END,
		assignment_next_attempt_at=$4::timestamptz+$5::interval,assignment_last_error_code=$3,
		assignment_lease_owner=NULL,assignment_lease_expires_at=NULL,
		state=CASE WHEN $2 THEN 'RETURNING' WHEN assignment_attempt_count+1 >= $6 THEN 'SUPPORT_REQUIRED' ELSE state END,updated_at=$4::timestamptz
		WHERE id=$1::uuid AND assignment_state='DELIVERING' AND assignment_lease_owner=$7`, workflowID, acknowledged, code, now, backoff.String(), maxAttempts, owner)
	return err
}

func (r *Repository) MarkReturnAssignmentAcknowledged(ctx context.Context, missionID string, now time.Time) error {
	_, err := r.db.Exec(ctx, `UPDATE dispatch_return_workflows SET assignment_state='ACKNOWLEDGED',assignment_lease_owner=NULL,
		assignment_lease_expires_at=NULL,state='RETURNING',updated_at=$2 WHERE return_mission_id=$1::uuid AND assignment_state<>'ACKNOWLEDGED'`, missionID, now)
	return err
}

func emit(ctx context.Context, tx pgx.Tx, eventType, aggregateType, aggregateID string, version int64, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO dispatch_marketplace_outbox(event_type,aggregate_type,aggregate_id,aggregate_version,payload,payload_hash,target_service)
		VALUES($1,$2,$3,$4,$5::jsonb,encode(digest(($5::jsonb)::text,'sha256'),'hex'),'notification-service') ON CONFLICT DO NOTHING`, eventType, aggregateType, aggregateID, version, raw)
	return err
}

func operationConflict(err error) error {
	if err == nil {
		return nil
	}
	return apperrors.Conflict("This recovery operation conflicts with current state.", err)
}

func (r *Repository) String() string { return fmt.Sprintf("deliveryrecovery.Repository(%p)", r) }
