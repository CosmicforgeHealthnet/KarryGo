package trip

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Regression coverage for RunAutoCompleteWorker: dispatch previously had no
// path from 'proof_submitted' back to 'completed' if the rider's app never
// called complete (killed, crashed, lost network after submitting proof).
// That left the customer's escrowed fare held forever and the rider unpaid.

func seedProofSubmittedTripAt(t *testing.T, repo *fakeRepository, providerID string, updatedAt time.Time) Trip {
	t.Helper()
	trip := Trip{
		ID:         uuid.NewString(),
		BookingID:  uuid.NewString(),
		ProviderID: providerID,
		Status:     StatusProofSubmitted,
		FareAmount: 150000,
		Currency:   "NGN",
		UpdatedAt:  updatedAt,
	}
	repo.trips = append(repo.trips, trip)
	repo.proofs = append(repo.proofs, DeliveryProof{
		ID:     uuid.NewString(),
		TripID: trip.ID,
	})
	return trip
}

func findTrip(repo *fakeRepository, id string) *Trip {
	for i := range repo.trips {
		if repo.trips[i].ID == id {
			return &repo.trips[i]
		}
	}
	return nil
}

func TestAutoCompleteWorker_SettlesStaleProofSubmittedTrip(t *testing.T) {
	repo, wallet, svc := setup8DEnv(t)
	fixedNow := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixedNow }

	providerID := uuid.NewString()
	stale := seedProofSubmittedTripAt(t, repo, providerID, fixedNow.Add(-45*time.Minute))

	svc.autoCompleteProofSubmitted(context.Background())

	got := findTrip(repo, stale.ID)
	if got == nil || got.Status != StatusCompleted {
		t.Fatalf("stale trip not settled: %+v", got)
	}
	calls := wallet.completeJobCallsCopy()
	if len(calls) != 1 || calls[0].sourceReference != stale.BookingID {
		t.Fatalf("expected exactly one CompleteJob call for %s, got %v", stale.BookingID, calls)
	}
}

func TestAutoCompleteWorker_LeavesFreshTripAlone(t *testing.T) {
	repo, wallet, svc := setup8DEnv(t)
	fixedNow := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixedNow }

	providerID := uuid.NewString()
	fresh := seedProofSubmittedTripAt(t, repo, providerID, fixedNow.Add(-5*time.Minute))

	svc.autoCompleteProofSubmitted(context.Background())

	got := findTrip(repo, fresh.ID)
	if got == nil || got.Status != StatusProofSubmitted {
		t.Fatalf("fresh trip should be untouched, got %+v", got)
	}
	if len(wallet.completeJobCallsCopy()) != 0 {
		t.Fatal("wallet must not be touched for a trip still inside its grace period")
	}
}

func TestAutoCompleteWorker_AlreadyCompletedTripDoesNotDoubleSettle(t *testing.T) {
	repo, wallet, svc := setup8DEnv(t)
	fixedNow := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixedNow }

	providerID := uuid.NewString()
	trip := seedProofSubmittedTripAt(t, repo, providerID, fixedNow.Add(-45*time.Minute))

	// Rider completes it first, through the normal path.
	if _, err := svc.CompleteTrip(context.Background(), trip.ID, providerID); err != nil {
		t.Fatalf("CompleteTrip: %v", err)
	}
	if len(wallet.completeJobCallsCopy()) != 1 {
		t.Fatal("expected the rider's own completion to settle once")
	}

	// The worker's list query only selects 'proof_submitted' rows, so an
	// already-completed trip is never even handed to CompleteTrip again —
	// this asserts that end-to-end via the real ListStaleProofSubmittedTrips
	// filter on the fake repository.
	svc.autoCompleteProofSubmitted(context.Background())

	if len(wallet.completeJobCallsCopy()) != 1 {
		t.Fatal("worker must not re-settle a trip that already completed")
	}
}
