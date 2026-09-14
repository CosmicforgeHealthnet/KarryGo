package tripsettlement_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/tripsettlement"
	"cosmicforge/logistics/shared/go/managedsettlement"
)

type walletStub struct {
	outcome   managedsettlement.Outcome
	err       error
	asked     []managedsettlement.Identity
	callCount int
}

func (s *walletStub) Lookup(_ context.Context, identity managedsettlement.Identity) (managedsettlement.Outcome, error) {
	s.callCount++
	s.asked = append(s.asked, identity)
	return s.outcome, s.err
}

func TestSettledMissionNeedsNothingFurther(t *testing.T) {
	wallet := &walletStub{outcome: managedsettlement.Outcome{Settled: true}}
	handoff := &tripsettlement.ManagedHandoff{Wallet: wallet}
	if err := handoff.HandleManagedCompletion(context.Background(), "trip-1", "book-1", time.Now().UTC()); err != nil {
		t.Fatalf("a settled mission returned an error: %v", err)
	}
}

func TestNotYetInstructedIsAcceptedWithoutPayingAnyone(t *testing.T) {
	wallet := &walletStub{outcome: managedsettlement.Outcome{}}
	handoff := &tripsettlement.ManagedHandoff{Wallet: wallet}
	if err := handoff.HandleManagedCompletion(context.Background(), "trip-1", "book-1", time.Now().UTC()); err != nil {
		t.Fatalf("an uninstructed completion returned an error: %v", err)
	}
}

// A timeout is not a confirmed failure. It must surface so the reconciliation
// worker looks at the mission again, rather than being read as "nothing
// settled" -- which is what would eventually pay the rider twice.
func TestUnknownOutcomeSurfacesRatherThanBeingAssumed(t *testing.T) {
	wallet := &walletStub{outcome: managedsettlement.Outcome{Unknown: true}, err: errors.New("i/o timeout")}
	handoff := &tripsettlement.ManagedHandoff{Wallet: wallet}
	if err := handoff.HandleManagedCompletion(context.Background(), "trip-1", "book-1", time.Now().UTC()); err == nil {
		t.Fatal("an unknown settlement outcome was reported as handled")
	}
}

// WAL-012, the requirement this package exists for: the synchronous completion
// path and the reconciliation worker must address one mission's money under
// one identity. They call the handoff with the arguments each happens to hold,
// so the handoff -- not the caller -- decides what the identity is.
func TestBothSettlementPathsAddressOneIdentity(t *testing.T) {
	wallet := &walletStub{outcome: managedsettlement.Outcome{Settled: true}}
	handoff := &tripsettlement.ManagedHandoff{Wallet: wallet}
	ctx := context.Background()

	// The synchronous path, at completion.
	completedAt := time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)
	if err := handoff.HandleManagedCompletion(ctx, "trip-1", "book-1", completedAt); err != nil {
		t.Fatal(err)
	}
	// The reconciliation worker, later, from a claimed settlement row whose
	// timestamp has been rounded by a database round trip and whose booking id
	// it may not even carry.
	if err := handoff.HandleManagedCompletion(ctx, "trip-1", "", completedAt.Add(90*time.Second)); err != nil {
		t.Fatal(err)
	}

	if len(wallet.asked) != 2 {
		t.Fatalf("wallet consulted %d times, want 2", len(wallet.asked))
	}
	if wallet.asked[0].Key() != wallet.asked[1].Key() {
		t.Fatalf("the two settlement paths addressed different jobs: %q vs %q",
			wallet.asked[0].Key(), wallet.asked[1].Key())
	}
	if wallet.asked[0].Key() != "DISPATCH:trip-1" {
		t.Fatalf("identity = %q, want DISPATCH:trip-1", wallet.asked[0].Key())
	}
}

// The identity is the trip, not the booking. One booking can produce more than
// one trip, so a booking-keyed identity would let a second mission's
// settlement answer for the first.
func TestIdentityIsTheTripNotTheBooking(t *testing.T) {
	wallet := &walletStub{outcome: managedsettlement.Outcome{Settled: true}}
	handoff := &tripsettlement.ManagedHandoff{Wallet: wallet}
	ctx := context.Background()

	if err := handoff.HandleManagedCompletion(ctx, "trip-1", "book-1", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := handoff.HandleManagedCompletion(ctx, "trip-2", "book-1", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if wallet.asked[0].Key() == wallet.asked[1].Key() {
		t.Fatalf("two trips of one booking shared a settlement identity: %q", wallet.asked[0].Key())
	}
}

func TestTripWithNoIDIsRefusedRatherThanLookedUp(t *testing.T) {
	wallet := &walletStub{}
	handoff := &tripsettlement.ManagedHandoff{Wallet: wallet}
	if err := handoff.HandleManagedCompletion(context.Background(), "  ", "book-1", time.Now().UTC()); err == nil {
		t.Fatal("a trip with no id was accepted")
	}
	if wallet.callCount != 0 {
		t.Fatal("an unusable identity was sent to Wallet anyway")
	}
}

func TestUnconfiguredHandoffRefusesRatherThanReportingSuccess(t *testing.T) {
	if err := tripsettlement.New(managedsettlement.Client{}).HandleManagedCompletion(
		context.Background(), "trip-1", "book-1", time.Now().UTC()); err == nil {
		t.Fatal("an unconfigured handoff reported success")
	}
}
