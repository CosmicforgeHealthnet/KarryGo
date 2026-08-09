package trip

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"cosmicforge/logistics/shared/go/apperrors"
)

// Canonical events added by WP6.3 and WP6.4.
//
// Both are versioned in the type itself, following the established catalogue
// (trip.completed.v1, proof.completed.v1). Neither reuses an existing name:
// trip.status_changed.v1 could not carry the confirmation or failure detail,
// and overloading trip.completed.v1 for a failure would be actively wrong.
const (
	eventRecipientConfirmed = "trip.recipient_confirmed.v1"
	eventDeliveryFailed     = "trip.delivery_failed.v1"
)

// ErrConfirmationFingerprintMismatch means a stored idempotency key was reused
// with different inputs.
var ErrConfirmationFingerprintMismatch = errors.New("trip: idempotency key reused with a different request")

// ConfirmRecipientTx records the recipient confirmation, denormalises it onto
// the trip, appends the state log and publishes the canonical event — all in
// one transaction.
//
// Returns (nil, nil) when the trip was not in a confirmable state or the
// expected source version did not match, which the service turns into the
// right conflict for the caller. The distinction is made in SQL rather than by
// a prior read so a concurrent transition cannot slip in between.
func (r *PostgresRepository) ConfirmRecipientTx(ctx context.Context,
	input ConfirmRecipientDBInput) (*RecipientConfirmation, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Lock the trip and re-assert every precondition inside the lock.
	var status string
	var sourceVersion int64
	var alreadyConfirmed pgtype.Timestamptz
	err = tx.QueryRow(ctx, `
		SELECT status, source_version, recipient_confirmed_at
		FROM trips WHERE id = $1 AND provider_id = $2 FOR UPDATE`,
		input.TripID, input.ProviderID).Scan(&status, &sourceVersion, &alreadyConfirmed)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !CanConfirmRecipient(TripStatus(status)) || alreadyConfirmed.Valid {
		return nil, nil
	}
	// Zero means "no opinion": a client with no cached copy may confirm
	// without guessing a version.
	if input.ExpectedSourceVersion != 0 && input.ExpectedSourceVersion != sourceVersion {
		return nil, apperrors.Conflict("This trip changed. Reload and try again.", nil)
	}

	row := tx.QueryRow(ctx, `
		INSERT INTO recipient_confirmations
			(trip_id, verification_method, recipient_reference, recipient_matched_expected,
			 proof_id, from_status, confirmed_at, confirmed_by_actor_type, confirmed_by_actor_id,
			 expected_source_version, idempotency_key, request_fingerprint,
			 correlation_id, causation_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		RETURNING id::text, trip_id::text, verification_method, recipient_reference,
			recipient_matched_expected, proof_id::text, from_status, confirmed_at,
			confirmed_by_actor_type, confirmed_by_actor_id, expected_source_version,
			idempotency_key, request_fingerprint, correlation_id, causation_id, created_at`,
		input.TripID, input.VerificationMethod, input.RecipientReference,
		input.RecipientMatchedExpected, input.ProofID, string(input.FromStatus), input.Now,
		input.ActorType, input.ActorID, input.ExpectedSourceVersion, input.IdempotencyKey,
		input.RequestFingerprint, input.CorrelationID, input.CausationID)
	confirmation, err := scanRecipientConfirmation(row)
	if isUniqueViolation(err) {
		return nil, apperrors.Conflict("This delivery has already been confirmed.", err)
	}
	if err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE trips SET recipient_confirmed_at = $2, recipient_confirmation_method = $3,
			updated_at = $2
		WHERE id = $1`, input.TripID, input.Now, input.VerificationMethod); err != nil {
		return nil, err
	}

	// The state log records the confirmation against the status it happened
	// in. From and to are equal because the trip did not move — which is the
	// point, and an operator reconstructing the delivery needs to see it.
	if _, err := tx.Exec(ctx, `
		INSERT INTO trip_state_log (trip_id, from_status, to_status, changed_at, changed_by, notes)
		VALUES ($1,$2,$2,$3,'provider','recipient_confirmed')`,
		input.TripID, string(input.FromStatus), input.Now); err != nil {
		return nil, err
	}

	// The event is written in the SAME transaction as the state change. A
	// consumer that saw the confirmation but not the event, or the reverse,
	// could never reconcile the two.
	version, err := r.emitRecipientConfirmed(ctx, tx, input, confirmation)
	if err != nil {
		return nil, err
	}
	confirmation.SourceVersion = version

	return confirmation, tx.Commit(ctx)
}

// emitRecipientConfirmed publishes trip.recipient_confirmed.v1.
//
// The payload carries the verification method, the opaque recipient surrogate
// and the server's match verdict. It carries no name, no phone, no address and
// no proof artefact reference: a managing organization needs to know the
// delivery was confirmed and how, not who answered the door.
func (r *PostgresRepository) emitRecipientConfirmed(ctx context.Context, tx pgx.Tx,
	input ConfirmRecipientDBInput, confirmation *RecipientConfirmation) (int64, error) {
	if !r.emitting() {
		return 0, nil
	}
	version, err := bumpTripVersion(ctx, tx, input.TripID)
	if err != nil {
		return 0, err
	}
	payload := map[string]any{
		"sourceService":            sourceServiceName,
		"tripId":                   input.TripID,
		"sourceVersion":            version,
		"status":                   string(input.FromStatus),
		"confirmationId":           confirmation.ID,
		"verificationMethod":       confirmation.VerificationMethod,
		"recipientReference":       confirmation.RecipientReference,
		"recipientMatchedExpected": confirmation.RecipientMatchedExpected,
		"hasProof":                 confirmation.ProofID != nil,
		"confirmedAt":              confirmation.ConfirmedAt,
	}
	return version, r.emitter.Emit(ctx, tx, eventRecipientConfirmed, "TRIP", input.TripID,
		version, input.CorrelationID, input.CausationID, payload)
}

// FailDeliveryTx moves the trip to `failed`, records the failure, appends the
// state log and publishes the canonical event in one transaction.
//
// Returns (nil, nil) when the trip was not in a failable state, which the
// service turns into the right conflict.
func (r *PostgresRepository) FailDeliveryTx(ctx context.Context,
	input FailDeliveryDBInput) (*DeliveryFailure, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var status string
	var sourceVersion int64
	err = tx.QueryRow(ctx, `
		SELECT status, source_version FROM trips
		WHERE id = $1 AND provider_id = $2 FOR UPDATE`,
		input.TripID, input.ProviderID).Scan(&status, &sourceVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !CanFailDelivery(TripStatus(status)) {
		return nil, nil
	}
	if input.ExpectedSourceVersion != 0 && input.ExpectedSourceVersion != sourceVersion {
		return nil, apperrors.Conflict("This trip changed. Reload and try again.", nil)
	}
	// The status observed under the lock wins over whatever the caller read
	// earlier, so the recorded from_status is always the real one.
	fromStatus := TripStatus(status)

	if _, err := tx.Exec(ctx, `
		UPDATE trips SET status = 'failed', failed_at = $2, updated_at = $2
		WHERE id = $1`, input.TripID, input.Now); err != nil {
		return nil, err
	}

	row := tx.QueryRow(ctx, `
		INSERT INTO delivery_failures
			(trip_id, reason_code, reason_category, operator_notes, from_status, failed_at,
			 responsible_actor_type, responsible_actor_id, retry_eligible, recipient_unavailable,
			 attempt_reference, incident_reference, expected_source_version, idempotency_key,
			 request_fingerprint, correlation_id, causation_id)
		VALUES ($1,$2,$3,NULLIF($4,''),$5,$6,$7,$8,$9,$10,NULLIF($11,''),NULLIF($12,''),
		        $13,$14,$15,$16,$17)
		RETURNING id::text, trip_id::text, reason_code, reason_category, operator_notes,
			from_status, failed_at, responsible_actor_type, responsible_actor_id,
			retry_eligible, recipient_unavailable, attempt_reference, incident_reference,
			expected_source_version, idempotency_key, request_fingerprint,
			correlation_id, causation_id, created_at`,
		input.TripID, input.ReasonCode, input.ReasonCategory, input.OperatorNotes,
		string(fromStatus), input.Now, input.ActorType, input.ActorID, input.RetryEligible,
		input.RecipientUnavailable, input.AttemptReference, input.IncidentReference,
		input.ExpectedSourceVersion, input.IdempotencyKey, input.RequestFingerprint,
		input.CorrelationID, input.CausationID)
	failure, err := scanDeliveryFailure(row)
	if isUniqueViolation(err) {
		return nil, apperrors.Conflict("This delivery has already been recorded as failed.", err)
	}
	if err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO trip_state_log (trip_id, from_status, to_status, changed_at, changed_by, notes)
		VALUES ($1,$2,'failed',$3,$4,$5)`,
		input.TripID, string(fromStatus), input.Now, input.ActorType, input.ReasonCode); err != nil {
		return nil, err
	}

	version, err := r.emitDeliveryFailed(ctx, tx, input, failure, fromStatus)
	if err != nil {
		return nil, err
	}
	failure.SourceVersion = version

	return failure, tx.Commit(ctx)
}

// emitDeliveryFailed publishes trip.delivery_failed.v1.
//
// What the payload deliberately does NOT contain:
//
//   - operator notes, which are free text written for an operator and may name
//     a person or describe an incident;
//   - any fare, penalty or refund field. Whether money moves is the wallet
//     service's to say, and a failure event that carried a monetary hint would
//     invite a consumer to act on it.
//
// What it does contain is the classification consumers must branch on, stated
// explicitly rather than inferred: the category, whether a re-attempt is
// permitted, and whether this was a recipient-unavailable failure.
func (r *PostgresRepository) emitDeliveryFailed(ctx context.Context, tx pgx.Tx,
	input FailDeliveryDBInput, failure *DeliveryFailure, fromStatus TripStatus) (int64, error) {
	if !r.emitting() {
		return 0, nil
	}
	version, err := bumpTripVersion(ctx, tx, input.TripID)
	if err != nil {
		return 0, err
	}
	payload := map[string]any{
		"sourceService":        sourceServiceName,
		"tripId":               input.TripID,
		"sourceVersion":        version,
		"status":               string(StatusFailed),
		"fromStatus":           string(fromStatus),
		"toStatus":             string(StatusFailed),
		"failureId":            failure.ID,
		"reasonCode":           failure.ReasonCode,
		"reasonCategory":       failure.ReasonCategory,
		"retryEligible":        failure.RetryEligible,
		"recipientUnavailable": failure.RecipientUnavailable,
		"responsibleActorType": failure.ResponsibleActorType,
		"failedAt":             failure.FailedAt,
		// Stated so no consumer has to guess: a failed delivery is not a
		// cancellation, and settlement must not treat it as completion.
		"isCancellation": false,
		"isCompletion":   false,
	}
	if failure.IncidentReference != nil {
		payload["incidentReference"] = *failure.IncidentReference
	}
	return version, r.emitter.Emit(ctx, tx, eventDeliveryFailed, "TRIP", input.TripID,
		version, input.CorrelationID, input.CausationID, payload)
}

// GetRecipientConfirmation loads a trip's confirmation.
func (r *PostgresRepository) GetRecipientConfirmation(ctx context.Context,
	tripID string) (*RecipientConfirmation, error) {
	confirmation, err := scanRecipientConfirmation(r.db.QueryRow(ctx, `
		SELECT id::text, trip_id::text, verification_method, recipient_reference,
			recipient_matched_expected, proof_id::text, from_status, confirmed_at,
			confirmed_by_actor_type, confirmed_by_actor_id, expected_source_version,
			idempotency_key, request_fingerprint, correlation_id, causation_id, created_at
		FROM recipient_confirmations WHERE trip_id = $1`, tripID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return confirmation, err
}

// FindRecipientConfirmationByKey resolves a retried confirmation.
func (r *PostgresRepository) FindRecipientConfirmationByKey(ctx context.Context,
	idempotencyKey string) (*RecipientConfirmation, error) {
	confirmation, err := scanRecipientConfirmation(r.db.QueryRow(ctx, `
		SELECT id::text, trip_id::text, verification_method, recipient_reference,
			recipient_matched_expected, proof_id::text, from_status, confirmed_at,
			confirmed_by_actor_type, confirmed_by_actor_id, expected_source_version,
			idempotency_key, request_fingerprint, correlation_id, causation_id, created_at
		FROM recipient_confirmations WHERE idempotency_key = $1`, idempotencyKey))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return confirmation, err
}

// GetDeliveryFailure loads a trip's failure record.
func (r *PostgresRepository) GetDeliveryFailure(ctx context.Context,
	tripID string) (*DeliveryFailure, error) {
	failure, err := scanDeliveryFailure(r.db.QueryRow(ctx, `
		SELECT id::text, trip_id::text, reason_code, reason_category, operator_notes,
			from_status, failed_at, responsible_actor_type, responsible_actor_id,
			retry_eligible, recipient_unavailable, attempt_reference, incident_reference,
			expected_source_version, idempotency_key, request_fingerprint,
			correlation_id, causation_id, created_at
		FROM delivery_failures WHERE trip_id = $1`, tripID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return failure, err
}

// FindDeliveryFailureByKey resolves a retried failure command.
func (r *PostgresRepository) FindDeliveryFailureByKey(ctx context.Context,
	idempotencyKey string) (*DeliveryFailure, error) {
	failure, err := scanDeliveryFailure(r.db.QueryRow(ctx, `
		SELECT id::text, trip_id::text, reason_code, reason_category, operator_notes,
			from_status, failed_at, responsible_actor_type, responsible_actor_id,
			retry_eligible, recipient_unavailable, attempt_reference, incident_reference,
			expected_source_version, idempotency_key, request_fingerprint,
			correlation_id, causation_id, created_at
		FROM delivery_failures WHERE idempotency_key = $1`, idempotencyKey))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return failure, err
}

// TripSourceVersion reads the current source version of a trip.
func (r *PostgresRepository) TripSourceVersion(ctx context.Context, tripID string) (int64, error) {
	var version int64
	err := r.db.QueryRow(ctx, `SELECT source_version FROM trips WHERE id = $1`, tripID).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("trip: not found")
	}
	return version, err
}

func scanRecipientConfirmation(row rowScanner) (*RecipientConfirmation, error) {
	var confirmation RecipientConfirmation
	var proofID pgtype.Text
	if err := row.Scan(&confirmation.ID, &confirmation.TripID, &confirmation.VerificationMethod,
		&confirmation.RecipientReference, &confirmation.RecipientMatchedExpected, &proofID,
		&confirmation.FromStatus, &confirmation.ConfirmedAt, &confirmation.ConfirmedByActorType,
		&confirmation.ConfirmedByActorID, &confirmation.ExpectedSourceVersion,
		&confirmation.IdempotencyKey, &confirmation.RequestFingerprint,
		&confirmation.CorrelationID, &confirmation.CausationID, &confirmation.CreatedAt); err != nil {
		return nil, err
	}
	if proofID.Valid && proofID.String != "" {
		confirmation.ProofID = &proofID.String
	}
	return &confirmation, nil
}

func scanDeliveryFailure(row rowScanner) (*DeliveryFailure, error) {
	var failure DeliveryFailure
	var notes, attempt, incident pgtype.Text
	if err := row.Scan(&failure.ID, &failure.TripID, &failure.ReasonCode, &failure.ReasonCategory,
		&notes, &failure.FromStatus, &failure.FailedAt, &failure.ResponsibleActorType,
		&failure.ResponsibleActorID, &failure.RetryEligible, &failure.RecipientUnavailable,
		&attempt, &incident, &failure.ExpectedSourceVersion, &failure.IdempotencyKey,
		&failure.RequestFingerprint, &failure.CorrelationID, &failure.CausationID,
		&failure.CreatedAt); err != nil {
		return nil, err
	}
	if notes.Valid && notes.String != "" {
		failure.OperatorNotes = &notes.String
	}
	if attempt.Valid && attempt.String != "" {
		failure.AttemptReference = &attempt.String
	}
	if incident.Valid && incident.String != "" {
		failure.IncidentReference = &incident.String
	}
	return &failure, nil
}
