package completedwork

import (
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/providercompat"
)

func managedBinding() Binding {
	return Binding{
		ProviderID: "prov-1", OrganizationID: "org-1", MembershipID: "mem-1",
		FleetAssignmentID: "asg-1", ExecutionMode: ExecutionModeManaged,
		CompletionVersion: 4, CorrelationID: "corr-1",
		FareMinor: 750000, Currency: "NGN",
	}
}

// HLG-017: only a COMPLETE corporate binding is a settlement authority.
func TestBindingManaged(t *testing.T) {
	if !managedBinding().Managed() {
		t.Fatal("a complete corporate binding must be managed")
	}
	for name, drop := range map[string]func(*Binding){
		"no organization": func(b *Binding) { b.OrganizationID = "" },
		"no membership":   func(b *Binding) { b.MembershipID = "" },
		"no provider":     func(b *Binding) { b.ProviderID = "" },
		"independent":     func(b *Binding) { b.ExecutionMode = "INDEPENDENT" },
	} {
		t.Run(name, func(t *testing.T) {
			partial := managedBinding()
			drop(&partial)
			if partial.Managed() {
				t.Fatalf("an incomplete binding must not be managed: %+v", partial)
			}
		})
	}
}

// HLG-017: the canonical DTO carries every field Fleet's ingestion requires,
// with HAULING as the source service.
func TestRequestForCanonicalFields(t *testing.T) {
	completedAt := time.Date(2026, 9, 14, 11, 0, 0, 0, time.UTC)
	request := RequestFor("book-1", managedBinding(), completedAt)

	if request.SourceService != providercompat.SourceServiceHauling {
		t.Fatalf("sourceService = %q, want HAULING", request.SourceService)
	}
	if request.SourceJobID != "book-1" || request.SourceAssignmentID != "asg-1" {
		t.Fatalf("job/assignment not carried: %+v", request)
	}
	if request.OrganizationID != "org-1" || request.MembershipID != "mem-1" || request.ProviderID != "prov-1" {
		t.Fatalf("corporate binding not carried: %+v", request)
	}
	// The eligible basis is the fare Hauling already settled. Nothing is
	// priced here; the amount is a snapshot carried through unchanged.
	if request.EligibleBasisMinor != 750000 {
		t.Fatalf("eligibleBasisMinor = %d, want the settled fare", request.EligibleBasisMinor)
	}
	if request.SourceVersion != 4 {
		t.Fatalf("sourceVersion = %d", request.SourceVersion)
	}
	if request.CompletedAt.Location() != time.UTC {
		t.Fatal("completedAt must be UTC")
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("the built request is not a valid canonical payload: %v", err)
	}
}

// A completion with no currency still produces a submittable payload: the
// platform's currency is NGN and an empty column must not make an otherwise
// valid completion permanently unsubmittable.
func TestRequestForDefaultsCurrency(t *testing.T) {
	binding := managedBinding()
	binding.Currency = ""
	request := RequestFor("book-1", binding, time.Now().UTC())
	if request.Currency != "NGN" {
		t.Fatalf("currency = %q", request.Currency)
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("payload invalid: %v", err)
	}
}

// HLG-019: the idempotency identity is stable across redeliveries of the same
// completion, and distinct for a later completion version or another job.
func TestIdempotencyKeyIsStable(t *testing.T) {
	first := IdempotencyKey("book-1", 4)
	if first != IdempotencyKey("book-1", 4) {
		t.Fatal("the key must be stable for the same completion")
	}
	if first == IdempotencyKey("book-1", 5) {
		t.Fatal("a later completion version must be a distinct identity")
	}
	if first == IdempotencyKey("book-2", 4) {
		t.Fatal("a different job must be a distinct identity")
	}
	if first == "" {
		t.Fatal("the key must not be empty")
	}
	// The same identity must appear on the request the worker submits.
	if RequestFor("book-1", managedBinding(), time.Now().UTC()).IdempotencyKey != first {
		t.Fatal("the request must carry the derived idempotency key")
	}
}

// HLG-018: bounded exponential backoff, capped.
func TestWorkerBackoffIsBoundedAndMonotonic(t *testing.T) {
	worker := &Worker{BaseBackoff: 5 * time.Second, MaxBackoff: time.Minute}
	previous := time.Duration(0)
	for attempt := 0; attempt < 12; attempt++ {
		delay := worker.backoffFor(attempt)
		if delay < previous {
			t.Fatalf("backoff went backwards at attempt %d: %s after %s", attempt, delay, previous)
		}
		if delay > worker.MaxBackoff {
			t.Fatalf("backoff %s exceeded the cap %s", delay, worker.MaxBackoff)
		}
		previous = delay
	}
	if previous != time.Minute {
		t.Fatalf("backoff should reach the cap, got %s", previous)
	}
}

// The reconciliation vocabulary is exactly the five states HLG-020 names, and
// the database CHECK constraint in migration 018 accepts precisely these.
func TestReconciliationStates(t *testing.T) {
	want := map[string]bool{
		"PENDING": true, "DELIVERED": true, "RETRY_SCHEDULED": true,
		"FAILED_TERMINAL": true, "RECONCILIATION_REQUIRED": true,
	}
	for _, state := range []string{ReconPending, ReconDelivered, ReconRetryScheduled,
		ReconFailedTerminal, ReconReconciliationRequired} {
		if !want[state] {
			t.Fatalf("unexpected reconciliation state %q", state)
		}
		delete(want, state)
	}
	if len(want) != 0 {
		t.Fatalf("missing reconciliation states: %v", want)
	}
}
