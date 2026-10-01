package marketplace

import (
	"context"
	"errors"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/providercompat"
)

type stubFleet struct {
	result   providercompat.SubmitResult
	err      error
	requests []providercompat.CompletedWorkRequest
}

func (s *stubFleet) SubmitCompletedWork(_ context.Context, request providercompat.CompletedWorkRequest) (providercompat.SubmitResult, error) {
	s.requests = append(s.requests, request)
	return s.result, s.err
}

type stubEvents struct {
	configured bool
	calls      int
}

func (s *stubEvents) Configured() bool { return s.configured }
func (s *stubEvents) Deliver(context.Context, Envelope) (DeliveryResult, error) {
	s.calls++
	return DeliveryResult{Outcome: DeliveryAccepted}, nil
}

func completedWorkEnvelope() Envelope {
	return Envelope{
		EventID:   "evt-1",
		EventType: CompletedWorkEventType,
		Payload: map[string]any{
			"sourceService":      "DISPATCH",
			"sourceJobId":        "trip-1",
			"sourceAssignmentId": "asg-1",
			"providerId":         "rider-1",
			"membershipId":       "mem-1",
			"organizationId":     "org-1",
			"completedAt":        time.Date(2026, 9, 14, 8, 0, 0, 0, time.UTC).Format(time.RFC3339),
			"currency":           "NGN",
			"eligibleBasisMinor": float64(250000),
			"sourceVersion":      float64(4),
			"idempotencyKey":     "dispatch-completed-work:trip-1:4",
			"correlationId":      "corr-1",
		},
	}
}

// DSP-008: completed work reaches Fleet's canonical endpoint with the exact
// canonical DTO, not a Dispatch-specific shape.
func TestCompletedWorkDeliverer_SubmitsCanonicalDTO(t *testing.T) {
	fleet := &stubFleet{result: providercompat.SubmitResult{Outcome: providercompat.SubmitAccepted}}
	deliverer := CompletedWorkDeliverer{Events: &stubEvents{configured: true}, Fleet: fleet}

	result, err := deliverer.Deliver(context.Background(), completedWorkEnvelope())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != DeliveryAccepted {
		t.Fatalf("outcome = %q", result.Outcome)
	}
	if len(fleet.requests) != 1 {
		t.Fatalf("submissions = %d", len(fleet.requests))
	}
	got := fleet.requests[0]
	if got.SourceService != providercompat.SourceServiceDispatch {
		t.Fatalf("sourceService = %q", got.SourceService)
	}
	if got.SourceJobID != "trip-1" || got.SourceAssignmentID != "asg-1" ||
		got.ProviderID != "rider-1" || got.MembershipID != "mem-1" || got.OrganizationID != "org-1" {
		t.Fatalf("canonical identifiers not carried: %+v", got)
	}
	if got.EligibleBasisMinor != 250000 || got.SourceVersion != 4 {
		t.Fatalf("basis/version not carried: %+v", got)
	}
	if got.CompletedAt.Location() != time.UTC {
		t.Fatal("completedAt must be submitted as UTC")
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("submitted payload is not a valid canonical request: %v", err)
	}
}

// Ordinary marketplace events keep going to the generic ingress: adding
// completed-work routing must not divert anything else.
func TestCompletedWorkDeliverer_OtherEventsUnaffected(t *testing.T) {
	events := &stubEvents{configured: true}
	fleet := &stubFleet{}
	deliverer := CompletedWorkDeliverer{Events: events, Fleet: fleet}

	if _, err := deliverer.Deliver(context.Background(),
		Envelope{EventID: "evt-2", EventType: "trip.completed.v1"}); err != nil {
		t.Fatal(err)
	}
	if events.calls != 1 {
		t.Fatalf("generic deliverer calls = %d, want 1", events.calls)
	}
	if len(fleet.requests) != 0 {
		t.Fatal("a non-completed-work event must never reach the completed-work endpoint")
	}
}

// DSP-010: outcome classification. DUPLICATE is a success because it proves
// Fleet holds exactly one record; UNKNOWN stays UNKNOWN so the worker
// redelivers the same event id rather than minting a new completion.
func TestCompletedWorkDeliverer_OutcomeClassification(t *testing.T) {
	cases := []struct {
		submit providercompat.SubmitOutcome
		want   DeliveryOutcome
	}{
		{providercompat.SubmitAccepted, DeliveryAccepted},
		{providercompat.SubmitDuplicate, DeliveryDuplicate},
		{providercompat.SubmitTerminal, DeliveryRejected},
		{providercompat.SubmitUnknown, DeliveryUnknown},
		{providercompat.SubmitRetryable, DeliveryRetryable},
	}
	for _, tc := range cases {
		t.Run(string(tc.submit), func(t *testing.T) {
			deliverer := CompletedWorkDeliverer{
				Events: &stubEvents{configured: true},
				Fleet:  &stubFleet{result: providercompat.SubmitResult{Outcome: tc.submit, Code: "c"}},
			}
			result, _ := deliverer.Deliver(context.Background(), completedWorkEnvelope())
			if result.Outcome != tc.want {
				t.Fatalf("%q -> %q, want %q", tc.submit, result.Outcome, tc.want)
			}
		})
	}
}

// A redelivery submits the SAME idempotency key, so Fleet returns the
// existing record instead of creating a second one.
func TestCompletedWorkDeliverer_RedeliveryReusesIdempotencyKey(t *testing.T) {
	fleet := &stubFleet{result: providercompat.SubmitResult{Outcome: providercompat.SubmitUnknown}}
	deliverer := CompletedWorkDeliverer{Events: &stubEvents{configured: true}, Fleet: fleet}
	envelope := completedWorkEnvelope()

	_, _ = deliverer.Deliver(context.Background(), envelope)
	fleet.result = providercompat.SubmitResult{Outcome: providercompat.SubmitDuplicate}
	result, _ := deliverer.Deliver(context.Background(), envelope)

	if result.Outcome != DeliveryDuplicate {
		t.Fatalf("redelivery outcome = %q, want DUPLICATE", result.Outcome)
	}
	if len(fleet.requests) != 2 {
		t.Fatalf("submissions = %d", len(fleet.requests))
	}
	if fleet.requests[0].IdempotencyKey != fleet.requests[1].IdempotencyKey {
		t.Fatalf("redelivery changed the idempotency key: %q then %q",
			fleet.requests[0].IdempotencyKey, fleet.requests[1].IdempotencyKey)
	}
}

// A switched-off completed-work target must leave the row queued rather than
// dead-lettering an authoritative completion.
func TestCompletedWorkDeliverer_UnconfiguredFleetIsRetryable(t *testing.T) {
	deliverer := CompletedWorkDeliverer{Events: &stubEvents{configured: true}}
	result, err := deliverer.Deliver(context.Background(), completedWorkEnvelope())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Outcome != DeliveryRetryable {
		t.Fatalf("outcome = %q, want RETRYABLE", result.Outcome)
	}
}

// A payload that cannot be decoded will never become deliverable.
func TestCompletedWorkDeliverer_UndecodablePayloadIsRejected(t *testing.T) {
	deliverer := CompletedWorkDeliverer{
		Events: &stubEvents{configured: true},
		Fleet:  &stubFleet{result: providercompat.SubmitResult{Outcome: providercompat.SubmitAccepted}},
	}
	envelope := completedWorkEnvelope()
	envelope.Payload = map[string]any{"completedAt": "not-a-timestamp"}
	result, _ := deliverer.Deliver(context.Background(), envelope)
	if result.Outcome != DeliveryRejected {
		t.Fatalf("outcome = %q, want REJECTED", result.Outcome)
	}
}

// The submit error is propagated so the worker records a real cause rather
// than an empty one.
func TestCompletedWorkDeliverer_PropagatesSubmitError(t *testing.T) {
	cause := errors.New("dial tcp: connection refused")
	deliverer := CompletedWorkDeliverer{
		Events: &stubEvents{configured: true},
		Fleet:  &stubFleet{result: providercompat.SubmitResult{Outcome: providercompat.SubmitUnknown}, err: cause},
	}
	_, err := deliverer.Deliver(context.Background(), completedWorkEnvelope())
	if !errors.Is(err, cause) {
		t.Fatalf("error = %v, want the submit cause", err)
	}
}
