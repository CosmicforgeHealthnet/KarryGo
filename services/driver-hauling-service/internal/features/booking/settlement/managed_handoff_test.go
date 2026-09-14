package settlement_test

import (
	"context"
	"errors"
	"testing"

	"cosmicforge/logistics/services/hauling-service/internal/features/booking/settlement"
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

func TestSettledJobNeedsNothingFurther(t *testing.T) {
	wallet := &walletStub{outcome: managedsettlement.Outcome{Settled: true}}
	handoff := &settlement.ManagedHandoff{Wallet: wallet}
	if err := handoff.HandleManagedCompletion(context.Background(), "book-1"); err != nil {
		t.Fatalf("a settled job returned an error: %v", err)
	}
	if len(wallet.asked) != 1 || wallet.asked[0].Key() != "HAULING:book-1" {
		t.Fatalf("wallet asked about %+v, want HAULING:book-1", wallet.asked)
	}
}

func TestNotYetInstructedIsAcceptedWithoutPayingAnyone(t *testing.T) {
	wallet := &walletStub{outcome: managedsettlement.Outcome{Pending: true}}
	handoff := &settlement.ManagedHandoff{Wallet: wallet}
	if err := handoff.HandleManagedCompletion(context.Background(), "book-1"); err != nil {
		t.Fatalf("a pending settlement returned an error: %v", err)
	}
}

// A timeout is not a confirmed failure. It surfaces, so the unresolved job is
// visible rather than quietly treated as unpaid -- which is the reading that
// would eventually pay the provider directly for work the organization owes.
func TestUnknownOutcomeSurfacesRatherThanBeingAssumed(t *testing.T) {
	wallet := &walletStub{outcome: managedsettlement.Outcome{Unknown: true}, err: errors.New("i/o timeout")}
	handoff := &settlement.ManagedHandoff{Wallet: wallet}
	if err := handoff.HandleManagedCompletion(context.Background(), "book-1"); err == nil {
		t.Fatal("an unknown settlement outcome was reported as handled")
	}
}

// The identity must be the booking id, because that is exactly what Hauling
// reports to Fleet as SourceJobID -- any other derivation would ask Wallet
// about a job it has never heard of.
func TestIdentityMatchesTheJobIdReportedToFleet(t *testing.T) {
	identity := settlement.Identity("book-1")
	if identity.SourceService != managedsettlement.SourceHauling || identity.SourceJobID != "book-1" {
		t.Fatalf("identity = %+v, want HAULING/book-1", identity)
	}
	if settlement.Identity("  book-1  ").Key() != identity.Key() {
		t.Fatal("identity is not stable under whitespace")
	}
}

func TestBookingWithNoIDIsRefusedRatherThanLookedUp(t *testing.T) {
	wallet := &walletStub{}
	handoff := &settlement.ManagedHandoff{Wallet: wallet}
	if err := handoff.HandleManagedCompletion(context.Background(), ""); err == nil {
		t.Fatal("a booking with no id was accepted")
	}
	if wallet.callCount != 0 {
		t.Fatal("an unusable identity was sent to Wallet anyway")
	}
}

func TestUnconfiguredHandoffRefusesRatherThanReportingSuccess(t *testing.T) {
	if err := settlement.New(managedsettlement.Client{}).HandleManagedCompletion(
		context.Background(), "book-1"); err == nil {
		t.Fatal("an unconfigured handoff reported success")
	}
}
