package trip

import (
	"context"
	"errors"
	"testing"
	"time"
)

type routeReaderRepo struct {
	Repository
	managed bool
	err     error
	calls   int
}

func (r *routeReaderRepo) IsOrganizationManaged(context.Context, string) (bool, error) {
	r.calls++
	return r.managed, r.err
}

type recordingSettlement struct{ calls int }

func (s *recordingSettlement) HandleTripCompleted(context.Context, string, time.Time) error {
	s.calls++
	return nil
}

type recordingHandoff struct {
	calls  int
	lastID string
}

func (h *recordingHandoff) HandleManagedCompletion(_ context.Context, tripID, _ string, _ time.Time) error {
	h.calls++
	h.lastID = tripID
	return nil
}

// DSP-014: a corporate mission must never be paid directly to the rider once
// isolation is on and no Section 12 handoff exists.
func TestSettleCompletedTrip_ManagedWithoutHandoffSkipsLegacyPayout(t *testing.T) {
	settlement := &recordingSettlement{}
	service := &Service{
		repository:        &routeReaderRepo{managed: true},
		settlement:        settlement,
		managedSettlement: ManagedSettlementConfig{Enabled: true},
	}
	service.settleCompletedTrip(context.Background(), "trip-1", "book-1", time.Now().UTC())
	if settlement.calls != 0 {
		t.Fatal("corporate work must not reach the legacy direct-to-rider settlement")
	}
}

// With a Section 12 handoff wired, corporate work goes there instead.
func TestSettleCompletedTrip_ManagedUsesHandoff(t *testing.T) {
	settlement := &recordingSettlement{}
	handoff := &recordingHandoff{}
	service := &Service{
		repository:        &routeReaderRepo{managed: true},
		settlement:        settlement,
		managedSettlement: ManagedSettlementConfig{Enabled: true},
		managedHandoff:    handoff,
	}
	service.settleCompletedTrip(context.Background(), "trip-1", "book-1", time.Now().UTC())
	if handoff.calls != 1 || handoff.lastID != "trip-1" {
		t.Fatalf("handoff calls = %d id = %q", handoff.calls, handoff.lastID)
	}
	if settlement.calls != 0 {
		t.Fatal("corporate work must not also take the legacy path")
	}
}

// An independent mission keeps the existing behaviour exactly.
func TestSettleCompletedTrip_IndependentUsesLegacyPath(t *testing.T) {
	settlement := &recordingSettlement{}
	service := &Service{
		repository:        &routeReaderRepo{managed: false},
		settlement:        settlement,
		managedSettlement: ManagedSettlementConfig{Enabled: true},
	}
	service.settleCompletedTrip(context.Background(), "trip-1", "book-1", time.Now().UTC())
	if settlement.calls != 1 {
		t.Fatalf("independent settlement calls = %d, want 1", settlement.calls)
	}
}

// With isolation off, nothing changes at all: no route lookup, legacy path.
func TestSettleCompletedTrip_IsolationOffPreservesPreviousBehaviour(t *testing.T) {
	repo := &routeReaderRepo{managed: true}
	settlement := &recordingSettlement{}
	service := &Service{repository: repo, settlement: settlement}
	service.settleCompletedTrip(context.Background(), "trip-1", "book-1", time.Now().UTC())
	if repo.calls != 0 {
		t.Fatal("isolation off must not even consult the route")
	}
	if settlement.calls != 1 {
		t.Fatalf("settlement calls = %d, want 1", settlement.calls)
	}
}

// An unresolvable route resolves to MANAGED: withholding a payout is
// recoverable, paying the wrong party is not.
func TestSettlementRouteFor_UnknownResolvesToManaged(t *testing.T) {
	service := &Service{repository: &routeReaderRepo{err: errors.New("db down")}}
	if got := service.settlementRouteFor(context.Background(), "trip-1"); got != RouteManaged {
		t.Fatalf("route = %q, want MANAGED when the route cannot be read", got)
	}
}

// DSP-014: the reconciliation worker is a second path into the same legacy
// settlement and must honour the same boundary.
func TestReconcileOne_ManagedSkipsLegacySettlement(t *testing.T) {
	settlement := &recordingSettlement{}
	service := &Service{
		repository:        &routeReaderRepo{managed: true},
		settlement:        settlement,
		managedSettlement: ManagedSettlementConfig{Enabled: true},
	}
	if err := service.reconcileOne(context.Background(),
		PendingSettlement{TripID: "trip-1", BookingID: "book-1"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if settlement.calls != 0 {
		t.Fatal("reconciliation must not pay corporate work through the legacy path")
	}
}

func TestReconcileOne_IndependentUsesLegacySettlement(t *testing.T) {
	settlement := &recordingSettlement{}
	service := &Service{
		repository:        &routeReaderRepo{managed: false},
		settlement:        settlement,
		managedSettlement: ManagedSettlementConfig{Enabled: true},
	}
	if err := service.reconcileOne(context.Background(),
		PendingSettlement{TripID: "trip-1", BookingID: "book-1"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if settlement.calls != 1 {
		t.Fatalf("settlement calls = %d, want 1", settlement.calls)
	}
}

// DSP-008: only a complete corporate binding is a settlement authority.
func TestCorporateBindingManaged(t *testing.T) {
	complete := corporateBinding{
		ProviderID: "rider-1", OrganizationID: "org-1", MembershipID: "mem-1",
		ExecutionMode: executionModeOrganizationManaged,
	}
	if !complete.Managed() {
		t.Fatal("a complete corporate binding must be managed")
	}
	for _, drop := range []func(*corporateBinding){
		func(b *corporateBinding) { b.OrganizationID = "" },
		func(b *corporateBinding) { b.MembershipID = "" },
		func(b *corporateBinding) { b.ProviderID = "" },
		func(b *corporateBinding) { b.ExecutionMode = "INDEPENDENT_PROVIDER" },
	} {
		partial := complete
		drop(&partial)
		if partial.Managed() {
			t.Fatalf("an incomplete binding must not be treated as managed: %+v", partial)
		}
	}
}

// DSP-010: the idempotency identity is stable across redeliveries of the same
// completion, and distinct for a later completion version.
func TestCompletedWorkIdempotencyKey(t *testing.T) {
	first := CompletedWorkIdempotencyKey("trip-1", 4)
	if first != CompletedWorkIdempotencyKey("trip-1", 4) {
		t.Fatal("the key must be stable for the same completion")
	}
	if first == CompletedWorkIdempotencyKey("trip-1", 5) {
		t.Fatal("a later completion version must be a distinct identity")
	}
	if first == CompletedWorkIdempotencyKey("trip-2", 4) {
		t.Fatal("a different trip must be a distinct identity")
	}
}
