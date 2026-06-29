package trip

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"cosmicforge/logistics/shared/go/walletclient"
)

// â”€â”€ fake wallet client â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€

type completeJobCall struct {
	sourceService   string
	sourceReference string
}

type fakeWalletClient struct {
	mu                 sync.Mutex
	completeJobCalls   []completeJobCall
	requestRefundCalls []walletclient.RefundRequest
	completeJobErr     error
	requestRefundErr   error
	// refundDone is signalled each time RequestRefund returns. Tests that need to
	// observe goroutine completion should set this to a buffered channel before
	// calling CancelTrip, then receive from it with a timeout.
	refundDone chan struct{}
}

func (f *fakeWalletClient) CompleteJob(ctx context.Context, sourceService, sourceReference string) (walletclient.PaymentIntent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.completeJobCalls = append(f.completeJobCalls, completeJobCall{sourceService, sourceReference})
	return walletclient.PaymentIntent{}, f.completeJobErr
}

func (f *fakeWalletClient) RequestRefund(ctx context.Context, request walletclient.RefundRequest) (walletclient.Refund, error) {
	f.mu.Lock()
	f.requestRefundCalls = append(f.requestRefundCalls, request)
	err := f.requestRefundErr
	done := f.refundDone
	f.mu.Unlock()
	if done != nil {
		done <- struct{}{}
	}
	return walletclient.Refund{}, err
}

func (f *fakeWalletClient) completeJobCallsCopy() []completeJobCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]completeJobCall, len(f.completeJobCalls))
	copy(out, f.completeJobCalls)
	return out
}

func (f *fakeWalletClient) requestRefundCallsCopy() []walletclient.RefundRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]walletclient.RefundRequest, len(f.requestRefundCalls))
	copy(out, f.requestRefundCalls)
	return out
}

// â”€â”€ Phase 8D helpers â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€

func setup8DEnv(t *testing.T) (*fakeRepository, *fakeWalletClient, *Service) {
	t.Helper()
	repo := newFakeRepository()
	wallet := &fakeWalletClient{}
	svc := NewService(repo, nil).WithWalletClient(wallet)
	return repo, wallet, svc
}

func seedProofSubmittedTrip(t *testing.T, repo *fakeRepository, providerID string) Trip {
	t.Helper()
	trip := Trip{
		ID:         uuid.NewString(),
		BookingID:  uuid.NewString(),
		ProviderID: providerID,
		Status:     StatusProofSubmitted,
		FareAmount: 150000,
		Currency:   "NGN",
	}
	repo.trips = append(repo.trips, trip)
	repo.proofs = append(repo.proofs, DeliveryProof{
		ID:     uuid.NewString(),
		TripID: trip.ID,
	})
	return trip
}

// â”€â”€ Phase 8D tests â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€

func TestPhase8D_CompleteJobCalledAfterTripCompletion(t *testing.T) {
	repo, wallet, svc := setup8DEnv(t)
	providerID := uuid.NewString()
	trip := seedProofSubmittedTrip(t, repo, providerID)

	if _, err := svc.CompleteTrip(context.Background(), trip.ID, providerID); err != nil {
		t.Fatalf("CompleteTrip: %v", err)
	}

	calls := wallet.completeJobCallsCopy()
	if len(calls) != 1 {
		t.Fatalf("CompleteJob call count=%d want 1", len(calls))
	}
	if calls[0].sourceService != "dispatch-delivery-service" {
		t.Fatalf("sourceService=%q want dispatch-delivery-service", calls[0].sourceService)
	}
	if calls[0].sourceReference != trip.BookingID {
		t.Fatalf("sourceReference=%q want %q", calls[0].sourceReference, trip.BookingID)
	}
}

func TestPhase8D_WalletFailureDoesNotRollBackCompletion(t *testing.T) {
	repo, wallet, svc := setup8DEnv(t)
	wallet.completeJobErr = errorf("wallet unavailable")
	providerID := uuid.NewString()
	trip := seedProofSubmittedTrip(t, repo, providerID)

	resp, err := svc.CompleteTrip(context.Background(), trip.ID, providerID)
	if err != nil {
		t.Fatalf("CompleteTrip must succeed even when wallet fails: %v", err)
	}
	if resp.Status != StatusCompleted {
		t.Fatalf("status=%s want completed", resp.Status)
	}
	// trip must be completed in the DB
	var found *Trip
	for i := range repo.trips {
		if repo.trips[i].ID == trip.ID {
			found = &repo.trips[i]
		}
	}
	if found == nil || found.Status != StatusCompleted {
		t.Fatalf("trip not marked completed in DB after wallet failure")
	}
}

func TestPhase8D_CompleteJobNotCalledWhenNoWalletClient(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo, nil) // no wallet
	providerID := uuid.NewString()
	trip := seedProofSubmittedTrip(t, repo, providerID)

	if _, err := svc.CompleteTrip(context.Background(), trip.ID, providerID); err != nil {
		t.Fatalf("CompleteTrip: %v", err)
	}
	// nothing to assert â€” just must not panic
}

func TestPhase8D_CompleteJobIdempotencyViaSameArguments(t *testing.T) {
	repo, wallet, svc := setup8DEnv(t)
	providerID := uuid.NewString()
	trip := seedProofSubmittedTrip(t, repo, providerID)

	if _, err := svc.CompleteTrip(context.Background(), trip.ID, providerID); err != nil {
		t.Fatalf("first CompleteTrip: %v", err)
	}
	// A second call hits the already-completed trip â€” service returns conflict; wallet is not called again.
	_, _ = svc.CompleteTrip(context.Background(), trip.ID, providerID)

	calls := wallet.completeJobCallsCopy()
	if len(calls) != 1 {
		t.Fatalf("CompleteJob should be called exactly once; got %d", len(calls))
	}
}

// â”€â”€ Phase 8E helpers â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€

func setup8EEnv(t *testing.T) (*fakeRepository, *fakeWalletClient, *Service) {
	t.Helper()
	repo := newFakeRepository()
	wallet := &fakeWalletClient{refundDone: make(chan struct{}, 1)}
	svc := NewService(repo, nil).WithWalletClient(wallet)
	return repo, wallet, svc
}

func seedCancellableTrip(t *testing.T, repo *fakeRepository, providerID string, status TripStatus) Trip {
	t.Helper()
	trip := Trip{
		ID:         uuid.NewString(),
		BookingID:  uuid.NewString(),
		ProviderID: providerID,
		Status:     status,
		FareAmount: 200000,
		Currency:   "NGN",
	}
	repo.trips = append(repo.trips, trip)
	return trip
}

func cancelTrip8E(t *testing.T, svc *Service, trip Trip, providerID string) CancelResponse {
	t.Helper()
	resp, err := svc.CancelTrip(context.Background(), trip.ID, providerID, CancelRequest{
		ReasonCode: "customer_unreachable",
	})
	if err != nil {
		t.Fatalf("CancelTrip: %v", err)
	}
	return resp
}

func waitRefund(t *testing.T, done chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("RequestRefund goroutine did not complete within 3s")
	}
}

// â”€â”€ Phase 8E tests â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€

func TestPhase8E_RefundCalledForAssignedCancellation(t *testing.T) {
	repo, wallet, svc := setup8EEnv(t)
	providerID := uuid.NewString()
	trip := seedCancellableTrip(t, repo, providerID, StatusAssigned)

	cancelTrip8E(t, svc, trip, providerID)
	waitRefund(t, wallet.refundDone)

	calls := wallet.requestRefundCallsCopy()
	if len(calls) != 1 {
		t.Fatalf("RequestRefund call count=%d want 1", len(calls))
	}
	r := calls[0]
	if r.PaymentReference != trip.BookingID {
		t.Fatalf("PaymentReference=%q want %q", r.PaymentReference, trip.BookingID)
	}
	if r.SourceService != "dispatch-delivery-service" {
		t.Fatalf("SourceService=%q want dispatch-delivery-service", r.SourceService)
	}
	if r.SourceReference != trip.BookingID {
		t.Fatalf("SourceReference=%q want %q", r.SourceReference, trip.BookingID)
	}
	if r.AmountKobo != trip.FareAmount {
		t.Fatalf("AmountKobo=%d want %d", r.AmountKobo, trip.FareAmount)
	}
	if r.Currency != trip.Currency {
		t.Fatalf("Currency=%q want %q", r.Currency, trip.Currency)
	}
	wantKey := "refund-" + trip.ID
	if r.IdempotencyKey != wantKey {
		t.Fatalf("IdempotencyKey=%q want %q", r.IdempotencyKey, wantKey)
	}
	if r.Reason == "" {
		t.Fatal("Reason must not be empty")
	}
}

func TestPhase8E_RefundCalledForEnRoutePickupCancellation(t *testing.T) {
	repo, wallet, svc := setup8EEnv(t)
	providerID := uuid.NewString()
	trip := seedCancellableTrip(t, repo, providerID, StatusEnRoutePickup)

	cancelTrip8E(t, svc, trip, providerID)
	waitRefund(t, wallet.refundDone)

	if len(wallet.requestRefundCallsCopy()) != 1 {
		t.Fatal("RequestRefund not called for en_route_pickup cancellation")
	}
}

func TestPhase8E_RefundCalledForArrivedPickupCancellation(t *testing.T) {
	repo, wallet, svc := setup8EEnv(t)
	providerID := uuid.NewString()
	trip := seedCancellableTrip(t, repo, providerID, StatusArrivedPickup)

	cancelTrip8E(t, svc, trip, providerID)
	waitRefund(t, wallet.refundDone)

	if len(wallet.requestRefundCallsCopy()) != 1 {
		t.Fatal("RequestRefund not called for arrived_pickup cancellation")
	}
}

func TestPhase8E_RefundNotCalledForInProgressCancellation(t *testing.T) {
	repo, wallet, svc := setup8EEnv(t)
	providerID := uuid.NewString()
	trip := seedCancellableTrip(t, repo, providerID, StatusInProgress)

	cancelTrip8E(t, svc, trip, providerID)
	// give any spurious goroutine time to fire
	time.Sleep(100 * time.Millisecond)

	if len(wallet.requestRefundCallsCopy()) != 0 {
		t.Fatal("RequestRefund must not be called for in_progress cancellation (requiresAdminInvestigation)")
	}
}

func TestPhase8E_RefundNotCalledWhenNoWalletClient(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo, nil) // no wallet
	providerID := uuid.NewString()
	trip := seedCancellableTrip(t, repo, providerID, StatusAssigned)

	cancelTrip8E(t, svc, trip, providerID)
	time.Sleep(100 * time.Millisecond)
	// must not panic; nothing to assert
}

func TestPhase8E_CancelTripSucceedsEvenWhenRefundFails(t *testing.T) {
	repo, wallet, svc := setup8EEnv(t)
	wallet.requestRefundErr = errorf("wallet refund failed")
	providerID := uuid.NewString()
	trip := seedCancellableTrip(t, repo, providerID, StatusAssigned)

	resp := cancelTrip8E(t, svc, trip, providerID)
	waitRefund(t, wallet.refundDone)

	if resp.Status != StatusCancelled {
		t.Fatalf("status=%s want cancelled", resp.Status)
	}
	// trip must be cancelled in DB regardless of refund outcome
	var found *Trip
	for i := range repo.trips {
		if repo.trips[i].ID == trip.ID {
			found = &repo.trips[i]
		}
	}
	if found == nil || found.Status != StatusCancelled {
		t.Fatal("trip not marked cancelled in DB after refund failure")
	}
}

func TestPhase8E_RefundNotCalledForProofSubmittedStatus(t *testing.T) {
	repo, wallet, svc := setup8EEnv(t)
	providerID := uuid.NewString()
	trip := seedCancellableTrip(t, repo, providerID, StatusProofSubmitted)

	_, err := svc.CancelTrip(context.Background(), trip.ID, providerID, CancelRequest{
		ReasonCode: "customer_unreachable",
	})
	if err == nil {
		t.Fatal("CancelTrip must return an error for proof_submitted status")
	}
	time.Sleep(50 * time.Millisecond)
	if len(wallet.requestRefundCallsCopy()) != 0 {
		t.Fatal("RequestRefund must not be called for proof_submitted status")
	}
}

func TestPhase8E_RefundNotCalledForCompletedStatus(t *testing.T) {
	repo, wallet, svc := setup8EEnv(t)
	providerID := uuid.NewString()
	trip := seedCancellableTrip(t, repo, providerID, StatusCompleted)

	_, err := svc.CancelTrip(context.Background(), trip.ID, providerID, CancelRequest{
		ReasonCode: "customer_unreachable",
	})
	if err == nil {
		t.Fatal("CancelTrip must return an error for completed status")
	}
	time.Sleep(50 * time.Millisecond)
	if len(wallet.requestRefundCallsCopy()) != 0 {
		t.Fatal("RequestRefund must not be called for completed status")
	}
}

func TestPhase8E_RefundNotCalledForNonCancellableStatus(t *testing.T) {
	repo, wallet, svc := setup8EEnv(t)
	providerID := uuid.NewString()
	trip := seedCancellableTrip(t, repo, providerID, StatusFailed)

	_, err := svc.CancelTrip(context.Background(), trip.ID, providerID, CancelRequest{
		ReasonCode: "customer_unreachable",
	})
	if err == nil {
		t.Fatal("CancelTrip must return an error for failed/unknown status")
	}
	time.Sleep(50 * time.Millisecond)
	if len(wallet.requestRefundCallsCopy()) != 0 {
		t.Fatal("RequestRefund must not be called for non-cancellable status")
	}
}

func TestPhase8E_RefundIdempotencyKeyIsRefundDashTripID(t *testing.T) {
	repo, wallet, svc := setup8EEnv(t)
	providerID := uuid.NewString()
	trip := seedCancellableTrip(t, repo, providerID, StatusAssigned)

	cancelTrip8E(t, svc, trip, providerID)
	waitRefund(t, wallet.refundDone)

	calls := wallet.requestRefundCallsCopy()
	if len(calls) != 1 {
		t.Fatalf("call count=%d", len(calls))
	}
	want := "refund-" + trip.ID
	if calls[0].IdempotencyKey != want {
		t.Fatalf("IdempotencyKey=%q want %q", calls[0].IdempotencyKey, want)
	}
}

// errorf is a helper to create a plain error value.
func errorf(msg string) error {
	return &plainError{msg}
}

type plainError struct{ s string }

func (e *plainError) Error() string { return e.s }
