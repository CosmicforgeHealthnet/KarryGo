package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
)

// RecipientConfirmationProjection is the fleet-side view of dispatch's
// authoritative recipient confirmation.
//
// RecipientReference is the opaque surrogate dispatch minted; it is not an
// identity and cannot be resolved to one from here. There is deliberately no
// name, phone or address field: what an organization needs to know is that the
// delivery was confirmed and by what method.
type RecipientConfirmationProjection struct {
	ConfirmedAt        time.Time
	Method             string
	RecipientReference string
	MatchedExpected    bool
}

// DeliveryFailureProjection is the fleet-side view of dispatch's authoritative
// delivery failure.
//
// No monetary field, by construction. Whether a failed delivery moves money is
// the wallet service's to state; a field here would be an invitation to decide
// it locally from an operational event.
type DeliveryFailureProjection struct {
	FailedAt             time.Time
	ReasonCode           string
	ReasonCategory       string
	RetryEligible        bool
	RecipientUnavailable bool
	IncidentReference    string
}

// DeliveryOutcome is whatever fleet currently holds about a trip's delivery
// outcome. Either half may be absent.
type DeliveryOutcome struct {
	Confirmation *RecipientConfirmationProjection
	Failure      *DeliveryFailureProjection
}

// ApplyRecipientConfirmation records dispatch's confirmation on a projection.
//
// Written as its own statement rather than by widening the general trip
// advance: the version, conflict and gap rules that govern ApplyTripSourceEvent
// are subtle and already correct, and threading four more columns through them
// would risk that correctness to save a query. The caller runs this inside the
// ingestion transaction, after the version rules have already decided the event
// should be applied.
//
// COALESCE on confirmed_at makes it idempotent: a redelivery that slips past
// the version check cannot move the confirmation time.
func (r *TripRepository) ApplyRecipientConfirmation(ctx context.Context, q database.Querier,
	tripProjectionID string, confirmation RecipientConfirmationProjection) error {
	_, err := q.Exec(ctx, `
		UPDATE fleet_partner_trip_projections SET
			recipient_confirmed_at = COALESCE(recipient_confirmed_at, $2),
			recipient_confirmation_method = COALESCE(recipient_confirmation_method, NULLIF($3, '')),
			recipient_reference = COALESCE(recipient_reference, NULLIF($4, '')),
			recipient_matched_expected = COALESCE(recipient_matched_expected, $5),
			updated_at = now()
		WHERE id = $1`,
		tripProjectionID, confirmation.ConfirmedAt, confirmation.Method,
		confirmation.RecipientReference, confirmation.MatchedExpected)
	if err != nil {
		return fmt.Errorf("marketplace: apply recipient confirmation: %w", err)
	}
	return nil
}

// ApplyDeliveryFailure records dispatch's failure verdict on a projection.
//
// It writes only the failure columns. It does NOT touch completed_at or
// cancelled_at, and that is the point: a failure is neither a completion nor a
// cancellation, and a projection that already carries one of those must keep
// carrying it so an operator can see the contradiction rather than have it
// quietly resolved in favour of whichever event arrived last.
func (r *TripRepository) ApplyDeliveryFailure(ctx context.Context, q database.Querier,
	tripProjectionID string, failure DeliveryFailureProjection) error {
	_, err := q.Exec(ctx, `
		UPDATE fleet_partner_trip_projections SET
			failed_at = COALESCE(failed_at, $2),
			failure_reason_code = COALESCE(failure_reason_code, NULLIF($3, '')),
			failure_reason_category = COALESCE(failure_reason_category, NULLIF($4, '')),
			failure_retry_eligible = COALESCE(failure_retry_eligible, $5),
			failure_recipient_unavailable = COALESCE(failure_recipient_unavailable, $6),
			failure_incident_reference = COALESCE(failure_incident_reference, NULLIF($7, '')),
			updated_at = now()
		WHERE id = $1`,
		tripProjectionID, failure.FailedAt, failure.ReasonCode, failure.ReasonCategory,
		failure.RetryEligible, failure.RecipientUnavailable, failure.IncidentReference)
	if err != nil {
		return fmt.Errorf("marketplace: apply delivery failure: %w", err)
	}
	return nil
}

// GetDeliveryOutcome reads the projected delivery outcome for one trip.
//
// Tenant-scoped: the organization must be the projection's owner.
func (r *TripRepository) GetDeliveryOutcome(ctx context.Context, organizationID string,
	tripProjectionID string) (*DeliveryOutcome, error) {
	var confirmedAt, failedAt *time.Time
	var method, reference, reasonCode, reasonCategory, incident *string
	var matched, retryEligible, recipientUnavailable *bool

	err := r.db.QueryRow(ctx, `
		SELECT recipient_confirmed_at, recipient_confirmation_method, recipient_reference,
			recipient_matched_expected, failed_at, failure_reason_code, failure_reason_category,
			failure_retry_eligible, failure_recipient_unavailable, failure_incident_reference
		FROM fleet_partner_trip_projections
		WHERE organization_id = $1 AND id = $2`, organizationID, tripProjectionID).
		Scan(&confirmedAt, &method, &reference, &matched, &failedAt, &reasonCode,
			&reasonCategory, &retryEligible, &recipientUnavailable, &incident)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("marketplace: read delivery outcome: %w", err)
	}

	outcome := &DeliveryOutcome{}
	if confirmedAt != nil {
		outcome.Confirmation = &RecipientConfirmationProjection{
			ConfirmedAt:        *confirmedAt,
			Method:             stringValue(method),
			RecipientReference: stringValue(reference),
			MatchedExpected:    boolValue(matched),
		}
	}
	if failedAt != nil {
		outcome.Failure = &DeliveryFailureProjection{
			FailedAt:             *failedAt,
			ReasonCode:           stringValue(reasonCode),
			ReasonCategory:       stringValue(reasonCategory),
			RetryEligible:        boolValue(retryEligible),
			RecipientUnavailable: boolValue(recipientUnavailable),
			IncidentReference:    stringValue(incident),
		}
	}
	return outcome, nil
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func boolValue(value *bool) bool {
	return value != nil && *value
}
