package trip

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"cosmicforge/logistics/shared/go/handover"
)

// advanceToInProgress walks a fresh trip to the state where a delivery is
// under way, which is the only place both new commands are meaningful.
func (f *emissionFixture) advanceToInProgress(t *testing.T, ctx context.Context) *Trip {
	t.Helper()
	trip := f.createTrip(t, ctx)
	now := time.Now().UTC()
	pickup, err := handover.NewCodeRecord(fghHandoverSecret, handover.BuildChallengeID("dispatch", handover.StepPickup, trip.ID), now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.MarkArrived(ctx, trip.ID, f.providerID, StatusAssigned, now, pickup); err != nil {
		t.Fatal(err)
	}
	delivery, err := handover.NewCodeRecord(fghHandoverSecret, handover.BuildChallengeID("dispatch", handover.StepDelivery, trip.ID), now)
	if err != nil {
		t.Fatal(err)
	}
	started, err := f.repo.MarkTripStarted(ctx, trip.ID, f.providerID, now, delivery)
	if err != nil {
		t.Fatal(err)
	}
	return started
}

// confirmInput builds a well-formed confirmation command.
func confirmInput(tripID, providerID, key string) ConfirmRecipientDBInput {
	return ConfirmRecipientDBInput{
		TripID:                   tripID,
		ProviderID:               providerID,
		FromStatus:               StatusInProgress,
		VerificationMethod:       VerificationSignature,
		RecipientReference:       "rcp_" + uuid.NewString(),
		RecipientMatchedExpected: true,
		IdempotencyKey:           key,
		RequestFingerprint:       "fp-" + key,
		CorrelationID:            "corr-" + key,
		ActorType:                "provider",
		ActorID:                  providerID,
		Now:                      time.Now().UTC(),
	}
}

// failInput builds a well-formed failure command.
func failInput(tripID, providerID, key, reason, category string) FailDeliveryDBInput {
	return FailDeliveryDBInput{
		TripID:                tripID,
		ProviderID:            providerID,
		FromStatus:            StatusInProgress,
		ReasonCode:            reason,
		ReasonCategory:        category,
		RetryEligible:         true,
		RecipientUnavailable:  true,
		IdempotencyKey:        key,
		RequestFingerprint:    "fp-" + key,
		CorrelationID:         "corr-" + key,
		ActorType:             "provider",
		ActorID:               providerID,
		Now:                   time.Now().UTC(),
		ExpectedSourceVersion: 0,
	}
}

// findEvent returns the first event of a type, or fails.
func findEvent(t *testing.T, events []outboxEvent, eventType string) outboxEvent {
	t.Helper()
	for _, event := range events {
		if event.EventType == eventType {
			return event
		}
	}
	t.Fatalf("event %s was not published; got %v", eventType, eventTypesOf(events))
	return outboxEvent{}
}

func eventTypesOf(events []outboxEvent) []string {
	types := make([]string, 0, len(events))
	for _, event := range events {
		types = append(types, event.EventType)
	}
	return types
}

// TestRecipientConfirmationIsAuthoritativeAndVersioned proves the confirmation
// commits with its event, advances the trip's source version, and does not
// move the trip's status.
func TestRecipientConfirmationIsAuthoritativeAndVersioned(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	fixture := newEmissionFixture(t, ctx)
	trip := fixture.advanceToInProgress(t, ctx)

	before, err := fixture.repo.TripSourceVersion(ctx, trip.ID)
	if err != nil {
		t.Fatal(err)
	}

	confirmation, err := fixture.repo.ConfirmRecipientTx(ctx,
		confirmInput(trip.ID, fixture.providerID, "confirm-key-1"))
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if confirmation == nil {
		t.Fatal("confirmation must be recorded from in_progress")
	}

	after, err := fixture.repo.TripSourceVersion(ctx, trip.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after != before+1 {
		t.Fatalf("source version %d -> %d; an event must advance it by exactly one", before, after)
	}

	reloaded, err := fixture.repo.GetTripByID(ctx, trip.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Status != StatusInProgress {
		t.Fatalf("status = %s; confirmation must not move the trip", reloaded.Status)
	}

	stored, err := fixture.repo.GetRecipientConfirmation(ctx, trip.ID)
	if err != nil || stored == nil {
		t.Fatalf("stored confirmation = %v, %v", stored, err)
	}
	if !stored.RecipientMatchedExpected {
		t.Fatal("the match verdict must be persisted")
	}
}

// TestRecipientConfirmedEventCarriesNoRecipientIdentity is the data-protection
// boundary: the event stream must let a managing organization see that the
// delivery was confirmed without seeing who confirmed it.
func TestRecipientConfirmedEventCarriesNoRecipientIdentity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	fixture := newEmissionFixture(t, ctx)
	trip := fixture.advanceToInProgress(t, ctx)

	if _, err := fixture.repo.ConfirmRecipientTx(ctx,
		confirmInput(trip.ID, fixture.providerID, "confirm-key-2")); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	event := findEvent(t, fixture.events(t, ctx), eventRecipientConfirmed)
	for _, forbidden := range []string{
		"receiverName", "receiverPhone", "recipientName", "recipientPhone",
		"recipientAddress", "fareAmount", "fare", "customerPhone",
	} {
		if _, present := event.Payload[forbidden]; present {
			t.Fatalf("event payload leaked %q: %v", forbidden, event.Payload)
		}
	}
	for _, expected := range []string{
		"verificationMethod", "recipientReference", "recipientMatchedExpected", "sourceVersion",
	} {
		if _, present := event.Payload[expected]; !present {
			t.Fatalf("event payload missing %q: %v", expected, event.Payload)
		}
	}
	// The surrogate must not be the receiver's phone number in disguise.
	if reference, _ := event.Payload["recipientReference"].(string); reference == "+2348011223344" {
		t.Fatal("recipientReference must not be the recipient's contact detail")
	}
}

// TestRecipientConfirmationRefusesSecondConfirmation proves one confirmation
// per trip.
func TestRecipientConfirmationRefusesSecondConfirmation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	fixture := newEmissionFixture(t, ctx)
	trip := fixture.advanceToInProgress(t, ctx)

	if _, err := fixture.repo.ConfirmRecipientTx(ctx,
		confirmInput(trip.ID, fixture.providerID, "confirm-key-3a")); err != nil {
		t.Fatalf("first confirm: %v", err)
	}
	second, err := fixture.repo.ConfirmRecipientTx(ctx,
		confirmInput(trip.ID, fixture.providerID, "confirm-key-3b"))
	if err == nil && second != nil {
		t.Fatal("a second confirmation must not be recorded")
	}

	// Exactly one event was published for the confirmation.
	confirmations := 0
	for _, event := range fixture.events(t, ctx) {
		if event.EventType == eventRecipientConfirmed {
			confirmations++
		}
	}
	if confirmations != 1 {
		t.Fatalf("confirmation events = %d, want 1", confirmations)
	}
}

// TestRecipientConfirmationRejectsStaleExpectedVersion proves the versioned
// command is enforced.
func TestRecipientConfirmationRejectsStaleExpectedVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	fixture := newEmissionFixture(t, ctx)
	trip := fixture.advanceToInProgress(t, ctx)

	input := confirmInput(trip.ID, fixture.providerID, "confirm-key-4")
	input.ExpectedSourceVersion = 9999
	if _, err := fixture.repo.ConfirmRecipientTx(ctx, input); err == nil {
		t.Fatal("a stale expected version must be rejected")
	}

	stored, err := fixture.repo.GetRecipientConfirmation(ctx, trip.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored != nil {
		t.Fatal("a rejected command must leave no record")
	}
}

// TestRecipientConfirmationRefusesForeignProvider is the ownership boundary.
func TestRecipientConfirmationRefusesForeignProvider(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	fixture := newEmissionFixture(t, ctx)
	trip := fixture.advanceToInProgress(t, ctx)

	confirmation, err := fixture.repo.ConfirmRecipientTx(ctx,
		confirmInput(trip.ID, uuid.NewString(), "confirm-key-5"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if confirmation != nil {
		t.Fatal("another provider must not confirm this delivery")
	}
}

// TestDeliveryFailureIsAuthoritativeAndTerminal proves the transition the
// state machine always allowed but nothing implemented.
func TestDeliveryFailureIsAuthoritativeAndTerminal(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	fixture := newEmissionFixture(t, ctx)
	trip := fixture.advanceToInProgress(t, ctx)

	before, err := fixture.repo.TripSourceVersion(ctx, trip.ID)
	if err != nil {
		t.Fatal(err)
	}

	failure, err := fixture.repo.FailDeliveryTx(ctx, failInput(trip.ID, fixture.providerID,
		"fail-key-1", FailureRecipientUnavailable, FailureCategoryRecipient))
	if err != nil {
		t.Fatalf("fail: %v", err)
	}
	if failure == nil {
		t.Fatal("a delivery in progress must be failable")
	}

	reloaded, err := fixture.repo.GetTripByID(ctx, trip.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Status != StatusFailed {
		t.Fatalf("status = %s, want failed", reloaded.Status)
	}
	if reloaded.CompletedAt != nil {
		t.Fatal("a failed delivery must not carry a completion time")
	}
	if reloaded.CancelledAt != nil {
		t.Fatal("a failed delivery is not a cancellation")
	}

	after, err := fixture.repo.TripSourceVersion(ctx, trip.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after != before+1 {
		t.Fatalf("source version %d -> %d; want exactly one advance", before, after)
	}

	// `failed` is terminal: a second attempt finds no failable state.
	second, err := fixture.repo.FailDeliveryTx(ctx, failInput(trip.ID, fixture.providerID,
		"fail-key-1b", FailureRecipientRefused, FailureCategoryRecipient))
	if err == nil && second != nil {
		t.Fatal("a failed trip must not be failed twice")
	}
}

// TestDeliveryFailedEventStatesItIsNeitherCompletionNorCancellation is the
// contract downstream settlement depends on.
func TestDeliveryFailedEventStatesItIsNeitherCompletionNorCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	fixture := newEmissionFixture(t, ctx)
	trip := fixture.advanceToInProgress(t, ctx)

	if _, err := fixture.repo.FailDeliveryTx(ctx, failInput(trip.ID, fixture.providerID,
		"fail-key-2", FailureRecipientUnavailable, FailureCategoryRecipient)); err != nil {
		t.Fatalf("fail: %v", err)
	}

	events := fixture.events(t, ctx)
	event := findEvent(t, events, eventDeliveryFailed)

	if completion, _ := event.Payload["isCompletion"].(bool); completion {
		t.Fatal("a failure must not report itself as a completion")
	}
	if cancellation, _ := event.Payload["isCancellation"].(bool); cancellation {
		t.Fatal("a failure must not report itself as a cancellation")
	}
	for _, forbidden := range []string{
		"fareAmount", "fare", "refundAmount", "penaltyAmount", "settlementAmount",
		"operatorNotes", "receiverPhone", "receiverName",
	} {
		if _, present := event.Payload[forbidden]; present {
			t.Fatalf("failure event leaked %q: %v", forbidden, event.Payload)
		}
	}
	for _, expected := range []string{
		"reasonCode", "reasonCategory", "retryEligible", "recipientUnavailable", "fromStatus",
	} {
		if _, present := event.Payload[expected]; !present {
			t.Fatalf("failure event missing %q: %v", expected, event.Payload)
		}
	}

	// No completion or cancellation event accompanied the failure.
	for _, published := range events {
		if published.EventType == eventTripCompleted || published.EventType == eventTripCancelled {
			t.Fatalf("a failure must not also publish %s", published.EventType)
		}
	}
}

// TestDeliveryFailureRecordsOperatorNotesWithoutPublishingThem proves free
// text stays behind.
func TestDeliveryFailureRecordsOperatorNotesWithoutPublishingThem(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	fixture := newEmissionFixture(t, ctx)
	trip := fixture.advanceToInProgress(t, ctx)

	input := failInput(trip.ID, fixture.providerID, "fail-key-3",
		FailurePackageDamaged, FailureCategoryPackage)
	input.OperatorNotes = "box was crushed; customer named in handover dispute"
	input.IncidentReference = "cmp_incident_123"
	if _, err := fixture.repo.FailDeliveryTx(ctx, input); err != nil {
		t.Fatalf("fail: %v", err)
	}

	stored, err := fixture.repo.GetDeliveryFailure(ctx, trip.ID)
	if err != nil || stored == nil {
		t.Fatalf("stored failure = %v, %v", stored, err)
	}
	if stored.OperatorNotes == nil || *stored.OperatorNotes == "" {
		t.Fatal("operator notes must be persisted for the operator")
	}
	if stored.IncidentReference == nil || *stored.IncidentReference != "cmp_incident_123" {
		t.Fatalf("incident reference = %v", stored.IncidentReference)
	}

	event := findEvent(t, fixture.events(t, ctx), eventDeliveryFailed)
	for _, value := range event.Payload {
		if text, ok := value.(string); ok && text == input.OperatorNotes {
			t.Fatalf("operator notes must not be published: %v", event.Payload)
		}
	}
	// The opaque case pointer, by contrast, is safe and useful.
	if reference, _ := event.Payload["incidentReference"].(string); reference != "cmp_incident_123" {
		t.Fatalf("incidentReference = %q; the opaque case pointer should travel", reference)
	}
}

// TestDeliveryFailureRejectsStaleExpectedVersion proves the versioned command
// is enforced, and that a rejection changes nothing.
func TestDeliveryFailureRejectsStaleExpectedVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	fixture := newEmissionFixture(t, ctx)
	trip := fixture.advanceToInProgress(t, ctx)

	input := failInput(trip.ID, fixture.providerID, "fail-key-4",
		FailureWrongAddress, FailureCategoryAddress)
	input.ExpectedSourceVersion = 4242
	if _, err := fixture.repo.FailDeliveryTx(ctx, input); err == nil {
		t.Fatal("a stale expected version must be rejected")
	}

	reloaded, err := fixture.repo.GetTripByID(ctx, trip.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Status != StatusInProgress {
		t.Fatalf("status = %s; a rejected command must not move the trip", reloaded.Status)
	}
	stored, err := fixture.repo.GetDeliveryFailure(ctx, trip.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored != nil {
		t.Fatal("a rejected command must leave no record")
	}
}

// TestCompletedTripCannotBeFailed proves failure never overwrites a settled
// completion.
func TestCompletedTripCannotBeFailed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	fixture := newEmissionFixture(t, ctx)
	trip := fixture.advanceToInProgress(t, ctx)
	now := time.Now().UTC()

	if _, err := fixture.repo.SubmitProofTx(ctx, SubmitProofDBInput{
		TripID: trip.ID, ProviderID: fixture.providerID,
		PhotoRef: "local-private://p", SignatureRef: "local-private://s",
		ReceiverName: "Receiver", ReceiverPhone: "+2348011223344", Now: now,
	}); err != nil {
		t.Fatal(err)
	}
	completed, _, err := fixture.repo.CompleteTripTx(ctx, CompleteTripInput{
		TripID: trip.ID, ProviderID: fixture.providerID, Now: now})
	if err != nil || completed == nil {
		t.Fatalf("complete: %v, %v", completed, err)
	}

	failure, err := fixture.repo.FailDeliveryTx(ctx, failInput(trip.ID, fixture.providerID,
		"fail-key-5", FailureRecipientRefused, FailureCategoryRecipient))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if failure != nil {
		t.Fatal("a completed trip must not be failable")
	}

	reloaded, err := fixture.repo.GetTripByID(ctx, trip.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Status != StatusCompleted {
		t.Fatalf("status = %s; completion must survive", reloaded.Status)
	}
}

// TestFailureClassificationIsServerOwned proves a caller cannot decide its own
// consequences.
func TestFailureClassificationIsServerOwned(t *testing.T) {
	cases := []struct {
		reason        string
		category      string
		retryEligible bool
	}{
		{FailureRecipientUnavailable, FailureCategoryRecipient, true},
		{FailurePackageLost, FailureCategoryPackage, false},
		{FailureSecurityRisk, FailureCategoryExternal, false},
		{FailureVehicleBreakdown, FailureCategoryProvider, true},
	}
	for _, testCase := range cases {
		classification, ok := ClassifyFailure(testCase.reason)
		if !ok {
			t.Fatalf("%s must be a recognised reason", testCase.reason)
		}
		if classification.Category != testCase.category {
			t.Fatalf("%s category = %s, want %s", testCase.reason,
				classification.Category, testCase.category)
		}
		if classification.RetryEligible != testCase.retryEligible {
			t.Fatalf("%s retryEligible = %v, want %v", testCase.reason,
				classification.RetryEligible, testCase.retryEligible)
		}
	}
	if _, ok := ClassifyFailure("MADE_UP_REASON"); ok {
		t.Fatal("an unrecognised reason must not classify")
	}
	// A lost package must never be re-attemptable, whatever a caller asks for.
	lost, _ := ClassifyFailure(FailurePackageLost)
	if lost.RetryEligible {
		t.Fatal("a lost package must never be marked re-attemptable")
	}
}
