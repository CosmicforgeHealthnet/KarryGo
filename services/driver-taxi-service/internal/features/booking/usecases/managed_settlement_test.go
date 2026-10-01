package bookingusecases

import (
	"testing"
	"time"

	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
)

func ptr[T any](v T) *T { return &v }

func managedCompletedBooking() bookingmodels.Booking {
	completedAt := time.Date(2026, 9, 14, 10, 30, 0, 0, time.UTC)
	fare := int64(450000)
	return bookingmodels.Booking{
		ID:                 "11111111-1111-1111-1111-111111111111",
		ProviderID:         ptr("prov-1"),
		Status:             bookingmodels.StatusCompleted,
		IsCorporateManaged: true,
		OrganizationID:     ptr("org-1"),
		MembershipID:       ptr("mem-1"),
		FleetAssignmentID:  ptr("asg-1"),
		SettlementMode:     bookingmodels.SettlementOrganizationManaged,
		SourceContext:      bookingmodels.SourceContextFleetAssignment,
		SourceVersion:      7,
		FareFinalKobo:      &fare,
		CompletedAt:        &completedAt,
	}
}

// TAX-015A: completed work is reported ONLY at the authoritative completion.
// Every earlier or terminal-but-not-complete state must produce nothing.
func TestCompletedWorkPayloadFor_OnlyAtAuthoritativeCompletion(t *testing.T) {
	for _, status := range []bookingmodels.Status{
		bookingmodels.StatusPendingMatch, bookingmodels.StatusAwaitingAccept,
		bookingmodels.StatusAccepted, bookingmodels.StatusEnRoutePickup,
		bookingmodels.StatusArrivedAtPickup, bookingmodels.StatusInProgress,
		bookingmodels.StatusCancelled, bookingmodels.StatusUnmatched,
	} {
		booking := managedCompletedBooking()
		booking.Status = status
		if _, ok := completedWorkPayloadFor(booking); ok {
			t.Fatalf("status %q must not report completed work", status)
		}
	}
	if _, ok := completedWorkPayloadFor(managedCompletedBooking()); !ok {
		t.Fatal("an authoritative completion must report completed work")
	}
}

// An independent ride never produces a Fleet completed-work record.
func TestCompletedWorkPayloadFor_IndependentProducesNothing(t *testing.T) {
	booking := managedCompletedBooking()
	booking.IsCorporateManaged = false
	if _, ok := completedWorkPayloadFor(booking); ok {
		t.Fatal("an independent ride must not report completed work")
	}
}

// A corporate booking missing its organization or membership cannot be
// reported: an incomplete binding is not a settlement authority.
func TestCompletedWorkPayloadFor_IncompleteBindingProducesNothing(t *testing.T) {
	noOrg := managedCompletedBooking()
	noOrg.OrganizationID = ptr("")
	if _, ok := completedWorkPayloadFor(noOrg); ok {
		t.Fatal("missing organization must not report completed work")
	}
	noMembership := managedCompletedBooking()
	noMembership.MembershipID = nil
	if _, ok := completedWorkPayloadFor(noMembership); ok {
		t.Fatal("missing membership must not report completed work")
	}
	noProvider := managedCompletedBooking()
	noProvider.ProviderID = nil
	if _, ok := completedWorkPayloadFor(noProvider); ok {
		t.Fatal("missing provider must not report completed work")
	}
}

func TestCompletedWorkPayloadFor_CanonicalFields(t *testing.T) {
	payload, ok := completedWorkPayloadFor(managedCompletedBooking())
	if !ok {
		t.Fatal("expected a payload")
	}
	if payload.SourceService != "TAXI" {
		t.Fatalf("sourceService = %q", payload.SourceService)
	}
	if payload.SourceJobID != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("sourceJobId = %q", payload.SourceJobID)
	}
	if payload.SourceAssignmentID != "asg-1" || payload.MembershipID != "mem-1" || payload.OrganizationID != "org-1" {
		t.Fatalf("corporate binding not carried: %+v", payload)
	}
	// The eligible basis is the fare Taxi already settled. Nothing is priced
	// here; both numbers are pricing snapshots taken long before completion.
	if payload.EligibleBasisMinor != 450000 {
		t.Fatalf("eligibleBasisMinor = %d, want the settled fare", payload.EligibleBasisMinor)
	}
	if payload.SourceVersion != 7 {
		t.Fatalf("sourceVersion = %d", payload.SourceVersion)
	}
	if payload.CompletedAt.Location() != time.UTC {
		t.Fatal("completedAt must be UTC")
	}
	if err := payload.Request().Validate(); err != nil {
		t.Fatalf("payload does not satisfy the canonical Fleet contract: %v", err)
	}
}

// TAX-016A: the idempotency key is derived from the booking and its
// completion version, so a redelivery reproduces the SAME identity.
func TestCompletedWorkIdempotencyKeyIsStable(t *testing.T) {
	first, _ := completedWorkPayloadFor(managedCompletedBooking())
	second, _ := completedWorkPayloadFor(managedCompletedBooking())
	if first.IdempotencyKey != second.IdempotencyKey {
		t.Fatalf("idempotency key is not stable: %q vs %q", first.IdempotencyKey, second.IdempotencyKey)
	}
	if first.IdempotencyKey == "" {
		t.Fatal("idempotency key must not be empty")
	}
	other := managedCompletedBooking()
	other.SourceVersion = 8
	later, _ := completedWorkPayloadFor(other)
	if later.IdempotencyKey == first.IdempotencyKey {
		t.Fatal("a different completion version must be a distinct identity")
	}
}

// TAX-020: the settlement route is read from the booking's own persisted
// mode, so a Fleet outage at completion time cannot reroute corporate work
// onto the independent direct-payment path.
func TestSettlementRouteFor(t *testing.T) {
	service := &BookingService{}

	independent := managedCompletedBooking()
	independent.IsCorporateManaged = false
	independent.SettlementMode = bookingmodels.SettlementIndependent
	if got := service.settlementRouteFor(independent); got != RouteIndependent {
		t.Fatalf("independent route = %q", got)
	}

	if got := service.settlementRouteFor(managedCompletedBooking()); got != RouteManaged {
		t.Fatalf("managed route = %q", got)
	}

	// A booking that claims managed settlement but names no organization is
	// not a settlement authority, so it must not be routed away from the
	// independent path on that claim alone.
	halfBound := managedCompletedBooking()
	halfBound.OrganizationID = nil
	if got := service.settlementRouteFor(halfBound); got != RouteIndependent {
		t.Fatalf("incomplete binding route = %q, want INDEPENDENT", got)
	}
}
