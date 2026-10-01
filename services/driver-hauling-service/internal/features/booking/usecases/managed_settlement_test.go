package bookingusecases

import (
	"context"
	"errors"
	"testing"

	bookingrepo "cosmicforge/logistics/services/hauling-service/internal/features/booking/repositories"
)

// routeRepo is a booking repository that only answers the managed-route
// question. Every other method is unused by these tests.
type routeRepo struct {
	bookingrepo.BookingRepository
	managed bool
	err     error
	calls   int
}

func (r *routeRepo) IsOrganizationManaged(context.Context, string) (bool, error) {
	r.calls++
	return r.managed, r.err
}

// HLG-025: with isolation on, corporate work never takes the legacy
// direct-to-provider path when no Section 12 handoff exists.
func TestRouteCompletedSettlement_ManagedWithoutHandoffWithholds(t *testing.T) {
	service := &BookingService{
		bookings:          &routeRepo{managed: true},
		managedSettlement: ManagedSettlementConfig{Enabled: true},
	}
	if service.routeCompletedSettlement(context.Background(), "book-1") {
		t.Fatal("corporate work must not reach the legacy direct-to-provider settlement")
	}
}

// With a Section 12 handoff wired, corporate work goes there and still does
// not take the legacy path.
func TestRouteCompletedSettlement_ManagedUsesHandoff(t *testing.T) {
	handoff := &recordingHandoff{}
	service := &BookingService{
		bookings:          &routeRepo{managed: true},
		managedSettlement: ManagedSettlementConfig{Enabled: true},
		managedHandoff:    handoff,
	}
	if service.routeCompletedSettlement(context.Background(), "book-1") {
		t.Fatal("a managed completion must not also take the legacy path")
	}
	if handoff.calls != 1 || handoff.lastID != "book-1" {
		t.Fatalf("handoff calls=%d id=%q", handoff.calls, handoff.lastID)
	}
}

// An independent job keeps the existing behaviour exactly.
func TestRouteCompletedSettlement_IndependentTakesLegacyPath(t *testing.T) {
	service := &BookingService{
		bookings:          &routeRepo{managed: false},
		managedSettlement: ManagedSettlementConfig{Enabled: true},
	}
	if !service.routeCompletedSettlement(context.Background(), "book-1") {
		t.Fatal("an independent job must settle through the existing path")
	}
}

// With isolation off nothing changes at all: no route lookup, legacy path.
func TestRouteCompletedSettlement_IsolationOffPreservesBehaviour(t *testing.T) {
	repo := &routeRepo{managed: true}
	service := &BookingService{bookings: repo}
	if !service.routeCompletedSettlement(context.Background(), "book-1") {
		t.Fatal("isolation off must preserve the legacy path")
	}
	if repo.calls != 0 {
		t.Fatal("isolation off must not even consult the route")
	}
}

// An unresolvable route resolves to MANAGED: withholding a payout is
// recoverable, paying the wrong party for corporate work is not.
func TestSettlementRouteFor_UnknownResolvesToManaged(t *testing.T) {
	service := &BookingService{bookings: &routeRepo{err: errors.New("db down")}}
	if got := service.settlementRouteFor(context.Background(), "book-1"); got != RouteManaged {
		t.Fatalf("route = %q, want MANAGED when the route cannot be read", got)
	}
}

// A repository that cannot answer the question at all falls back to the
// legacy path, which is the pre-Section-10 behaviour for a service wired
// without the corporate repository.
func TestSettlementRouteFor_NoReaderIsIndependent(t *testing.T) {
	service := &BookingService{bookings: nil}
	if got := service.settlementRouteFor(context.Background(), "book-1"); got != RouteIndependent {
		t.Fatalf("route = %q, want INDEPENDENT without a route reader", got)
	}
}

type recordingHandoff struct {
	calls  int
	lastID string
}

func (h *recordingHandoff) HandleManagedCompletion(_ context.Context, bookingID string) error {
	h.calls++
	h.lastID = bookingID
	return nil
}
