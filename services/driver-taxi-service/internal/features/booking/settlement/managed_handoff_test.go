package settlement_test

import (
	"context"
	"errors"
	"testing"

	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	"cosmicforge/logistics/services/taxi-service/internal/features/booking/settlement"
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

func completedBooking() bookingmodels.Booking {
	return bookingmodels.Booking{ID: "book-1"}
}

func TestSettledJobNeedsNothingFurther(t *testing.T) {
	wallet := &walletStub{outcome: managedsettlement.Outcome{Settled: true}}
	if err := settlement.New(managedsettlement.Client{}).HandleManagedCompletion(context.Background(), completedBooking()); err == nil {
		t.Fatal("an unconfigured handoff reported success")
	}
	handoff := &settlement.ManagedHandoff{Wallet: wallet}
	if err := handoff.HandleManagedCompletion(context.Background(), completedBooking()); err != nil {
		t.Fatalf("a settled job returned an error: %v", err)
	}
}

// Fleet not having instructed yet is normal moments after completion. It is a
// definite answer, so the completion is not retried forever -- but it also
// never triggers an independent payout.
func TestNotYetInstructedIsAcceptedWithoutPayingAnyone(t *testing.T) {
	wallet := &walletStub{outcome: managedsettlement.Outcome{}}
	handoff := &settlement.ManagedHandoff{Wallet: wallet}
	if err := handoff.HandleManagedCompletion(context.Background(), completedBooking()); err != nil {
		t.Fatalf("an uninstructed completion returned an error: %v", err)
	}
	if wallet.callCount != 1 {
		t.Fatalf("wallet consulted %d times, want 1", wallet.callCount)
	}
}

// WAL-011B. A timeout is not a confirmed failure and is not a confirmed
// success. It must surface as an error so the completion is looked at again,
// because the one thing that must never happen is concluding "nothing settled"
// and paying the driver directly for work the organization owes.
func TestUnknownOutcomeSurfacesRatherThanBeingAssumed(t *testing.T) {
	wallet := &walletStub{
		outcome: managedsettlement.Outcome{Unknown: true},
		err:     errors.New("connection reset"),
	}
	handoff := &settlement.ManagedHandoff{Wallet: wallet}
	err := handoff.HandleManagedCompletion(context.Background(), completedBooking())
	if err == nil {
		t.Fatal("an unknown settlement outcome was reported as handled")
	}
}

// WAL-011A. Both paths that can settle a completion must address the job by
// the same name, or the second will ask Wallet about a job Wallet has never
// heard of and conclude nothing was paid.
func TestSettlementIdentityIsDerivedFromTheBookingAlone(t *testing.T) {
	booking := completedBooking()
	first := settlement.Identity(booking)

	// The same booking seen later, after fields that have nothing to do with
	// its financial identity have moved on.
	later := booking
	later.SourceVersion = 7
	second := settlement.Identity(later)

	if first.Key() != second.Key() {
		t.Fatalf("identity drifted between paths: %q vs %q", first.Key(), second.Key())
	}
	if first.SourceService != managedsettlement.SourceTaxi {
		t.Fatalf("identity source service = %q, want %q", first.SourceService, managedsettlement.SourceTaxi)
	}
	if first.SourceJobID != booking.ID {
		t.Fatalf("identity job id = %q, want the booking id %q", first.SourceJobID, booking.ID)
	}
}

func TestHandoffAsksAboutTheJobItWasGiven(t *testing.T) {
	wallet := &walletStub{outcome: managedsettlement.Outcome{Settled: true}}
	handoff := &settlement.ManagedHandoff{Wallet: wallet}
	if err := handoff.HandleManagedCompletion(context.Background(), completedBooking()); err != nil {
		t.Fatal(err)
	}
	if len(wallet.asked) != 1 || wallet.asked[0].Key() != "TAXI:book-1" {
		t.Fatalf("wallet was asked about %+v, want TAXI:book-1", wallet.asked)
	}
}

func TestBookingWithNoIDIsRefusedRatherThanLookedUp(t *testing.T) {
	wallet := &walletStub{}
	handoff := &settlement.ManagedHandoff{Wallet: wallet}
	if err := handoff.HandleManagedCompletion(context.Background(), bookingmodels.Booking{}); err == nil {
		t.Fatal("a booking with no id was accepted")
	}
	if wallet.callCount != 0 {
		t.Fatal("an unusable identity was sent to Wallet anyway")
	}
}
