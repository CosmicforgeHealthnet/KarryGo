package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/providercompat"
)

type stubSubmitter struct {
	result  providercompat.SubmitResult
	err     error
	calls   int
	lastKey string
}

func (s *stubSubmitter) SubmitCompletedWork(_ context.Context, request providercompat.CompletedWorkRequest) (providercompat.SubmitResult, error) {
	s.calls++
	s.lastKey = request.IdempotencyKey
	return s.result, s.err
}

func samplePayload() CompletedWorkPayload {
	return CompletedWorkPayload{
		SourceService: "TAXI", SourceJobID: "book-1", SourceAssignmentID: "asg-1",
		ProviderID: "prov-1", MembershipID: "mem-1", OrganizationID: "org-1",
		CompletedAt: time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC),
		Currency:    "NGN", EligibleBasisMinor: 450000, SourceVersion: 3,
		IdempotencyKey: CompletedWorkIdempotencyKey("book-1", 3),
	}
}

func sampleRecord(t *testing.T) Record {
	t.Helper()
	encoded, err := json.Marshal(samplePayload())
	if err != nil {
		t.Fatal(err)
	}
	return Record{ID: "row-1", Target: TargetFleet, EventType: EventFleetCompletedWork, Payload: encoded}
}

// TAX-016B: each Fleet outcome maps to the right delivery classification.
func TestFleetDestination_OutcomeClassification(t *testing.T) {
	cases := []struct {
		name        string
		outcome     providercompat.SubmitOutcome
		wantSuccess bool
		wantKind    string
	}{
		{"accepted", providercompat.SubmitAccepted, true, ""},
		{"duplicate is success", providercompat.SubmitDuplicate, true, ""},
		{"terminal", providercompat.SubmitTerminal, false, "terminal"},
		{"unknown is ambiguous", providercompat.SubmitUnknown, false, "ambiguous"},
		{"retryable", providercompat.SubmitRetryable, false, "retryable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			destination := FleetCompletedWorkDestination{
				Client:  &stubSubmitter{result: providercompat.SubmitResult{Outcome: tc.outcome, Code: "c"}},
				Enabled: true,
			}
			err := destination.Deliver(context.Background(), sampleRecord(t))
			if tc.wantSuccess {
				if err != nil {
					t.Fatalf("expected success, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected a failure")
			}
			var terminal *TerminalDeliveryError
			var ambiguous *AmbiguousDeliveryError
			switch tc.wantKind {
			case "terminal":
				if !errors.As(err, &terminal) {
					t.Fatalf("want TerminalDeliveryError, got %T", err)
				}
			case "ambiguous":
				if !errors.As(err, &ambiguous) {
					t.Fatalf("want AmbiguousDeliveryError, got %T", err)
				}
			default:
				if errors.As(err, &terminal) || errors.As(err, &ambiguous) {
					t.Fatalf("a plain retryable failure must be neither terminal nor ambiguous, got %T", err)
				}
			}
		})
	}
}

// TAX-017A: a redelivery reuses the original idempotency key, so Fleet
// returns the existing record rather than creating a second one.
func TestFleetDestination_RedeliveryReusesIdempotencyKey(t *testing.T) {
	submitter := &stubSubmitter{result: providercompat.SubmitResult{Outcome: providercompat.SubmitUnknown}}
	destination := FleetCompletedWorkDestination{Client: submitter, Enabled: true}
	record := sampleRecord(t)

	_ = destination.Deliver(context.Background(), record)
	first := submitter.lastKey

	submitter.result = providercompat.SubmitResult{Outcome: providercompat.SubmitDuplicate}
	if err := destination.Deliver(context.Background(), record); err != nil {
		t.Fatalf("redelivery should succeed as a duplicate: %v", err)
	}
	if submitter.lastKey != first || first == "" {
		t.Fatalf("redelivery changed the idempotency key: %q then %q", first, submitter.lastKey)
	}
	if submitter.calls != 2 {
		t.Fatalf("calls = %d", submitter.calls)
	}
}

// A disabled destination must leave the row queued, not dead-letter an
// authoritative completion.
func TestFleetDestination_DisabledIsRetryable(t *testing.T) {
	err := FleetCompletedWorkDestination{Enabled: false}.Deliver(context.Background(), sampleRecord(t))
	if err == nil {
		t.Fatal("a disabled destination must not report success")
	}
	var terminal *TerminalDeliveryError
	if errors.As(err, &terminal) {
		t.Fatal("a disabled destination must not dead-letter the row")
	}
}

// An undecodable payload can never become deliverable, so it is terminal.
func TestFleetDestination_UndecodablePayloadIsTerminal(t *testing.T) {
	destination := FleetCompletedWorkDestination{Client: &stubSubmitter{}, Enabled: true}
	err := destination.Deliver(context.Background(),
		Record{ID: "row-1", Target: TargetFleet, Payload: []byte(`{not json`)})
	var terminal *TerminalDeliveryError
	if !errors.As(err, &terminal) {
		t.Fatalf("want TerminalDeliveryError, got %T (%v)", err, err)
	}
}

// The router keeps Vendor and Fleet rows on one outbox without ever
// delivering one to the other's consumer.
func TestRouter_RoutesByTarget(t *testing.T) {
	fleetCalls, vendorCalls := 0, 0
	router := NewRouter(map[string]Destination{
		TargetFleet:  destinationFunc(func(context.Context, Record) error { fleetCalls++; return nil }),
		TargetVendor: destinationFunc(func(context.Context, Record) error { vendorCalls++; return nil }),
	})
	if err := router.Deliver(context.Background(), Record{Target: TargetFleet}); err != nil {
		t.Fatal(err)
	}
	if err := router.Deliver(context.Background(), Record{Target: TargetVendor}); err != nil {
		t.Fatal(err)
	}
	if fleetCalls != 1 || vendorCalls != 1 {
		t.Fatalf("fleet=%d vendor=%d", fleetCalls, vendorCalls)
	}
	// An unroutable row cannot become deliverable by waiting.
	err := router.Deliver(context.Background(), Record{Target: "nowhere"})
	var terminal *TerminalDeliveryError
	if !errors.As(err, &terminal) {
		t.Fatalf("want TerminalDeliveryError, got %T", err)
	}
}

type destinationFunc func(context.Context, Record) error

func (f destinationFunc) Deliver(ctx context.Context, r Record) error { return f(ctx, r) }
