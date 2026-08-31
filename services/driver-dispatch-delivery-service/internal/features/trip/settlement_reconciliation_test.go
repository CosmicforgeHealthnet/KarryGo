package trip

import (
	"context"
	"errors"
	"testing"
	"time"
)

type settlementMemoryStore struct {
	Repository
	item      PendingSettlement
	confirmed bool
	owners    []string
	attempts  int
}

func (r *settlementMemoryStore) ClaimSettlements(_ context.Context, _ time.Time, owner string, _ SettlementRetryPolicy) ([]PendingSettlement, error) {
	if r.confirmed {
		return nil, nil
	}
	r.owners = append(r.owners, owner)
	return []PendingSettlement{r.item}, nil
}
func (r *settlementMemoryStore) FinishSettlement(_ context.Context, tripID, owner string, confirmed bool, _ time.Time, _ time.Duration, _ int) error {
	if tripID != r.item.TripID || owner != r.owners[len(r.owners)-1] {
		return errors.New("lease or trip mismatch")
	}
	r.confirmed = confirmed
	r.attempts++
	return nil
}

type settlementReply struct {
	fail     bool
	bookings []string
}

func (r *settlementReply) HandleTripCompleted(_ context.Context, id string, _ time.Time) error {
	r.bookings = append(r.bookings, id)
	if r.fail {
		return errors.New("remote acknowledgement lost")
	}
	return nil
}
func TestSettlementWorkerRecoversTimeoutWithoutNewIdentity(t *testing.T) {
	store := &settlementMemoryStore{item: PendingSettlement{TripID: "trip", BookingID: "booking", CompletedAt: time.Now()}}
	remote := &settlementReply{fail: true}
	s := NewService(store, nil).WithSettlementReconciler(remote, SettlementRetryPolicy{})
	s.reconcileSettlements(context.Background())
	if store.confirmed || store.attempts != 1 {
		t.Fatal("timeout was marked confirmed")
	}
	remote.fail = false
	s.reconcileSettlements(context.Background())
	s.reconcileSettlements(context.Background())
	if !store.confirmed || len(remote.bookings) != 2 || remote.bookings[0] != remote.bookings[1] {
		t.Fatalf("reconciliation failed: %+v", remote)
	}
	if store.owners[0] == store.owners[1] {
		t.Fatal("claim epochs reused fencing identity")
	}
}
