package integration_test

import (
	"context"
	"testing"
	"time"

	mktmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	mktrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
	mktusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/usecases"
)

// ingestTripEvent delivers one TRIP-aggregate envelope.
func (f *tripFixture) ingestTripEvent(t *testing.T, eventID string, eventType string,
	version int64, payload map[string]any) mktusecases.IngestResult {
	t.Helper()
	raw, event := tripEvent(t, eventID, eventType, f.tripID, version, payload)
	result, err := f.service.IngestEvent(context.Background(), raw, event)
	if err != nil {
		t.Fatalf("ingest %s: %v", eventType, err)
	}
	return result
}

// outcome reads the projected delivery outcome for the fixture's trip.
func (f *tripFixture) outcome(t *testing.T) *mktrepos.DeliveryOutcome {
	t.Helper()
	trips := mktrepos.NewTripRepository(f.pool)
	projection, err := trips.GetBySourceTrip(context.Background(), f.pool,
		mktmodels.SourceDispatch, f.tripID)
	if err != nil {
		t.Fatalf("load trip projection: %v", err)
	}
	result, err := trips.GetDeliveryOutcome(context.Background(), f.orgID, projection.ID)
	if err != nil {
		t.Fatalf("load delivery outcome: %v", err)
	}
	return result
}

// requestStatus reads the associated request projection's status.
func (f *tripFixture) requestStatus(t *testing.T) string {
	t.Helper()
	projections := mktrepos.NewProjectionRepository(f.pool)
	stored, err := projections.GetByID(context.Background(), f.orgID, f.projection.ID)
	if err != nil {
		t.Fatalf("load request projection: %v", err)
	}
	return stored.ProjectionStatus
}

// TestRecipientConfirmationProjectsWithoutAdvancingLifecycle proves fleet
// records the confirmation and nothing else.
func TestRecipientConfirmationProjectsWithoutAdvancingLifecycle(t *testing.T) {
	f := newTripFixture(t, "Confirm Projection Org")
	f.createTrip(t, nil)

	confirmedAt := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	result := f.ingestTripEvent(t, "evt-confirm-1", mktusecases.EventTripRecipientConfirmed, 2,
		map[string]any{
			"verificationMethod":       "SIGNATURE",
			"recipientReference":       "rcp_opaque_1",
			"recipientMatchedExpected": true,
			"confirmedAt":              confirmedAt,
		})
	if result.Status != mktusecases.IngestAccepted {
		t.Fatalf("status = %s, want ACCEPTED", result.Status)
	}

	outcome := f.outcome(t)
	if outcome.Confirmation == nil {
		t.Fatal("the confirmation must be projected")
	}
	if outcome.Confirmation.Method != "SIGNATURE" {
		t.Fatalf("method = %q", outcome.Confirmation.Method)
	}
	if outcome.Confirmation.RecipientReference != "rcp_opaque_1" {
		t.Fatalf("recipientReference = %q", outcome.Confirmation.RecipientReference)
	}
	if !outcome.Confirmation.MatchedExpected {
		t.Fatal("the match verdict must be projected")
	}
	if outcome.Failure != nil {
		t.Fatal("a confirmation must not project a failure")
	}

	// The request projection is untouched: confirmation is not completion.
	if status := f.requestStatus(t); status == mktmodels.ProjectionCompleted {
		t.Fatal("a recipient confirmation must never mark the request completed")
	}
}

// TestDuplicateRecipientConfirmationIsIgnored proves redelivery is safe.
func TestDuplicateRecipientConfirmationIsIgnored(t *testing.T) {
	f := newTripFixture(t, "Confirm Duplicate Org")
	f.createTrip(t, nil)

	payload := map[string]any{
		"verificationMethod":       "OTP",
		"recipientReference":       "rcp_opaque_2",
		"recipientMatchedExpected": true,
	}
	f.ingestTripEvent(t, "evt-confirm-2", mktusecases.EventTripRecipientConfirmed, 2, payload)
	first := f.outcome(t).Confirmation

	// Same event id: the inbox recognises it.
	replay := f.ingestTripEvent(t, "evt-confirm-2", mktusecases.EventTripRecipientConfirmed, 2, payload)
	if replay.Status != mktusecases.IngestDuplicate {
		t.Fatalf("status = %s, want DUPLICATE", replay.Status)
	}

	// A different event id at the same version with different content must not
	// silently overwrite the confirmation.
	f.ingestTripEvent(t, "evt-confirm-2b", mktusecases.EventTripRecipientConfirmed, 2,
		map[string]any{
			"verificationMethod":       "VERBAL",
			"recipientReference":       "rcp_someone_else",
			"recipientMatchedExpected": false,
		})
	second := f.outcome(t).Confirmation
	if second.RecipientReference != first.RecipientReference {
		t.Fatalf("recipientReference changed from %q to %q; a contradiction must not overwrite",
			first.RecipientReference, second.RecipientReference)
	}
}

// TestStaleRecipientConfirmationIsIgnored proves an out-of-order delivery does
// not move the projection backwards.
func TestStaleRecipientConfirmationIsIgnored(t *testing.T) {
	f := newTripFixture(t, "Confirm Stale Org")
	f.createTrip(t, nil)
	f.ingestTripEvent(t, "evt-confirm-3", mktusecases.EventTripRecipientConfirmed, 5,
		map[string]any{
			"verificationMethod": "PHOTO",
			"recipientReference": "rcp_current",
		})

	stale := f.ingestTripEvent(t, "evt-confirm-3-old", mktusecases.EventTripRecipientConfirmed, 2,
		map[string]any{
			"verificationMethod": "VERBAL",
			"recipientReference": "rcp_stale",
		})
	if stale.Status != mktusecases.IngestIgnored {
		t.Fatalf("status = %s, want IGNORED", stale.Status)
	}
	if reference := f.outcome(t).Confirmation.RecipientReference; reference != "rcp_current" {
		t.Fatalf("recipientReference = %q; a stale event must change nothing", reference)
	}
}

// TestDeliveryFailureProjectsWithoutReleasingCapacityOrSettling is the load-
// bearing test of WP6.4 on the fleet side.
func TestDeliveryFailureProjectsWithoutReleasingCapacityOrSettling(t *testing.T) {
	f := newTripFixture(t, "Failure Projection Org")
	f.createTrip(t, nil)

	f.ingestTripEvent(t, "evt-fail-1", mktusecases.EventTripDeliveryFailed, 2, map[string]any{
		"reasonCode":           "RECIPIENT_UNAVAILABLE",
		"reasonCategory":       "RECIPIENT",
		"retryEligible":        true,
		"recipientUnavailable": true,
		"fromStatus":           "in_progress",
		"status":               "failed",
		"isCancellation":       false,
		"isCompletion":         false,
	})

	outcome := f.outcome(t)
	if outcome.Failure == nil {
		t.Fatal("the failure must be projected")
	}
	if outcome.Failure.ReasonCategory != "RECIPIENT" || !outcome.Failure.RetryEligible {
		t.Fatalf("failure projection = %+v", outcome.Failure)
	}
	if !outcome.Failure.RecipientUnavailable {
		t.Fatal("the recipient-unavailable classification must be projected")
	}

	status := f.requestStatus(t)
	if status == mktmodels.ProjectionCompleted {
		t.Fatal("a failed delivery must never be projected as completed")
	}
	if status == mktmodels.ProjectionCancelled {
		t.Fatal("a failed delivery must never be projected as cancelled")
	}
	if status != mktmodels.ProjectionDeliveryFailed {
		t.Fatalf("request status = %s, want DELIVERY_FAILED", status)
	}

	// Capacity was not released: the offer, if any, is untouched. A failed
	// delivery may be re-attempted, and standing the offer down would free a
	// provider still holding the package.
	var cancelledOffers int
	if err := f.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM fleet_partner_organization_request_offers
		WHERE request_projection_id = $1 AND status = 'CANCELLED'`,
		f.projection.ID).Scan(&cancelledOffers); err != nil {
		t.Fatalf("count offers: %v", err)
	}
	if cancelledOffers != 0 {
		t.Fatalf("cancelled offers = %d; a failure must not release capacity", cancelledOffers)
	}
}

// TestDeliveryFailureDoesNotOverwriteCompletion proves a late or contradictory
// failure cannot displace a settled completion.
func TestDeliveryFailureDoesNotOverwriteCompletion(t *testing.T) {
	f := newTripFixture(t, "Failure After Completion Org")
	f.createTrip(t, nil)
	f.ingestTripEvent(t, "evt-complete-1", mktusecases.EventTripCompleted, 2, map[string]any{
		"status":      "completed",
		"completedAt": time.Now().UTC().Format(time.RFC3339),
	})
	if status := f.requestStatus(t); status != mktmodels.ProjectionCompleted {
		t.Fatalf("request status = %s, want COMPLETED before the failure arrives", status)
	}

	f.ingestTripEvent(t, "evt-fail-2", mktusecases.EventTripDeliveryFailed, 3, map[string]any{
		"reasonCode":     "RECIPIENT_REFUSED",
		"reasonCategory": "RECIPIENT",
		"retryEligible":  false,
	})

	if status := f.requestStatus(t); status != mktmodels.ProjectionCompleted {
		t.Fatalf("request status = %s; a failure must not displace a completion", status)
	}

	// The contradiction is surfaced as a reconciliation case rather than
	// resolved silently in favour of whichever event arrived last.
	var cases int
	if err := f.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM fleet_partner_projection_reconciliation_cases
		WHERE aggregate_source_id = $1 AND status IN ('OPEN', 'IN_PROGRESS')`,
		f.tripID).Scan(&cases); err != nil {
		t.Fatalf("count reconciliation cases: %v", err)
	}
	if cases == 0 {
		t.Fatal("a failure contradicting a completion must open a reconciliation case")
	}
}

// TestDeliveryFailureOnGappedProjectionOpensReconciliation proves fleet fails
// closed when its own view is untrustworthy.
func TestDeliveryFailureOnGappedProjectionOpensReconciliation(t *testing.T) {
	f := newTripFixture(t, "Failure Gapped Org")
	f.createTrip(t, nil)

	// A version jump marks the projection GAP_DETECTED; the failure arrives on
	// top of a view fleet knows is incomplete.
	f.ingestTripEvent(t, "evt-fail-gap", mktusecases.EventTripDeliveryFailed, 9, map[string]any{
		"reasonCode":     "WRONG_ADDRESS",
		"reasonCategory": "ADDRESS",
		"retryEligible":  true,
	})

	if status := f.requestStatus(t); status == mktmodels.ProjectionDeliveryFailed {
		t.Fatal("a failure on an untrusted projection must not be recorded as authoritative")
	}
	var cases int
	if err := f.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM fleet_partner_projection_reconciliation_cases
		WHERE aggregate_source_id = $1 AND status IN ('OPEN', 'IN_PROGRESS')`,
		f.tripID).Scan(&cases); err != nil {
		t.Fatalf("count reconciliation cases: %v", err)
	}
	if cases == 0 {
		t.Fatal("a gapped failure must open a reconciliation case")
	}

	// The failure detail is still projected — an operator needs to see what
	// dispatch said — but it did not drive the lifecycle status.
	if f.outcome(t).Failure == nil {
		t.Fatal("the failure detail must still be visible to an operator")
	}
}

// TestDeliveryEventsCarryNoRecipientOrMonetaryFields proves the projection
// columns cannot hold what the boundary refuses to carry.
func TestDeliveryEventsCarryNoRecipientOrMonetaryFields(t *testing.T) {
	f := newTripFixture(t, "Delivery Boundary Org")
	f.createTrip(t, nil)

	// A hostile or careless producer includes fields fleet must not store.
	f.ingestTripEvent(t, "evt-confirm-boundary", mktusecases.EventTripRecipientConfirmed, 2,
		map[string]any{
			"verificationMethod": "SIGNATURE",
			"recipientReference": "rcp_boundary",
			"receiverName":       "Ada Obi",
			"receiverPhone":      "+2348011223344",
			"fareAmount":         150000,
		})

	// The projection table has no column for any of them, so nothing landed.
	var columns int
	if err := f.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM information_schema.columns
		WHERE table_name = 'fleet_partner_trip_projections'
		  AND column_name IN ('receiver_name', 'receiver_phone', 'recipient_name',
		                      'recipient_phone', 'fare_amount', 'refund_amount')`).
		Scan(&columns); err != nil {
		t.Fatalf("inspect columns: %v", err)
	}
	if columns != 0 {
		t.Fatalf("trip projection has %d columns it must not have", columns)
	}

	outcome := f.outcome(t)
	if outcome.Confirmation == nil || outcome.Confirmation.RecipientReference != "rcp_boundary" {
		t.Fatalf("confirmation = %+v", outcome.Confirmation)
	}
}

// TestUnknownDeliveryEventTypeIsRejectedPermanently proves the accepted set
// stays closed.
func TestUnknownDeliveryEventTypeIsRejectedPermanently(t *testing.T) {
	f := newTripFixture(t, "Delivery Unknown Org")
	f.createTrip(t, nil)

	raw, event := tripEvent(t, "evt-unknown-1", "trip.delivery_failed.v2", f.tripID, 2, nil)
	if _, err := f.service.IngestEvent(context.Background(), raw, event); err == nil {
		t.Fatal("an unrecognised event type must be rejected")
	}
}
