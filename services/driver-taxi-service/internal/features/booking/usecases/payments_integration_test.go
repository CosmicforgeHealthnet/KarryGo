package bookingusecases

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	bookingrepositories "cosmicforge/logistics/services/taxi-service/internal/features/booking/repositories"
	availabilitymodels "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/models"
	availabilityrepositories "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/walletclient"
)

var (
	errFakeInsufficientBalance = errors.New("insufficient wallet balance")
	errFakeWalletUnreachable   = errors.New("payment-wallet-service unreachable")
)

// newPaymentTestService is newLifecycleService plus a fake PaymentClient --
// real Postgres for every DB transition, a fake wallet so these tests are
// deterministic and need no running payment-wallet-service.
func newPaymentTestService(db *pgxpool.Pool, pc PaymentClient) *BookingService {
	bookings := bookingrepositories.NewPostgresBookingRepository(db)
	opState := availabilityrepositories.NewPostgresOperationalStateRepository(db)
	return NewBookingService(Options{
		Bookings: bookings, Markets: newFakeMarkets(), Pricing: newFakePricing(), OperationalState: opState,
		Payments: pc,
	})
}

// seedAwaitingAcceptanceWithPayment mirrors seedLifecycleFixture but also
// sets an unpaid wallet payment intent, the state MarkMatched would have
// left the booking in.
func seedAwaitingAcceptanceWithPayment(t *testing.T, db *pgxpool.Pool, intentID string) (bookingID, providerID string) {
	t.Helper()
	bookingID, providerID = seedLifecycleFixture(t, db, bookingmodels.StatusAwaitingAccept, availabilitymodels.StateOffered)
	mustExec(t, db, `UPDATE taxi_bookings SET payment_method = 'wallet', payment_intent_id = $1 WHERE id = $2`, intentID, bookingID)
	return bookingID, providerID
}

func TestAcceptBooking_HoldsPaymentAndMarksHeld(t *testing.T) {
	db := lifecycleTestDB(t)
	pc := &fakePaymentClient{}
	svc := newPaymentTestService(db, pc)
	bookingID, providerID := seedAwaitingAcceptanceWithPayment(t, db, "intent-held-test")

	booking, err := svc.AcceptBooking(context.Background(), bookingID, providerID)
	if err != nil {
		t.Fatalf("AcceptBooking: %v", err)
	}
	if booking.Status != bookingmodels.StatusAccepted {
		t.Errorf("status = %q, want accepted", booking.Status)
	}
	if booking.PaymentStatus != bookingmodels.PaymentHeld {
		t.Errorf("payment_status = %q, want held", booking.PaymentStatus)
	}
	if len(pc.payCalls) != 1 || pc.payCalls[0] != "intent-held-test" {
		t.Errorf("payCalls = %v, want exactly [intent-held-test]", pc.payCalls)
	}

	// Confirm it actually persisted, not just returned in-memory.
	final, ferr := svc.GetByIDForProvider(context.Background(), bookingID, providerID)
	if ferr != nil {
		t.Fatalf("GetByIDForProvider: %v", ferr)
	}
	if final.PaymentStatus != bookingmodels.PaymentHeld {
		t.Errorf("persisted payment_status = %q, want held", final.PaymentStatus)
	}
}

func TestAcceptBooking_BlocksOnHoldFailureAndNeverTransitions(t *testing.T) {
	db := lifecycleTestDB(t)
	pc := &fakePaymentClient{payErr: errFakeInsufficientBalance}
	svc := newPaymentTestService(db, pc)
	bookingID, providerID := seedAwaitingAcceptanceWithPayment(t, db, "intent-fail-test")

	_, err := svc.AcceptBooking(context.Background(), bookingID, providerID)
	if err == nil {
		t.Fatal("expected the accept to be refused when the wallet hold fails")
	}

	final, ferr := svc.GetByIDForProvider(context.Background(), bookingID, providerID)
	if ferr != nil {
		t.Fatalf("GetByIDForProvider: %v", ferr)
	}
	if final.Status != bookingmodels.StatusAwaitingAccept {
		t.Errorf("status = %q, want unchanged awaiting_acceptance", final.Status)
	}
	if final.PaymentStatus != bookingmodels.PaymentUnpaid {
		t.Errorf("payment_status = %q, want unchanged unpaid", final.PaymentStatus)
	}
}

// TestAcceptBooking_WalletRefusalReachesTheDriverAsPaymentFailed is the QA
// 2026-10-15 finding 1 regression: Wallet answers an unfunded hold with a 409
// RequestError, and the driver must see a definite 409 payment_failed, not a
// retryable 503 "Something went wrong".
func TestAcceptBooking_WalletRefusalReachesTheDriverAsPaymentFailed(t *testing.T) {
	db := lifecycleTestDB(t)
	pc := &fakePaymentClient{payErr: &walletclient.RequestError{Status: http.StatusConflict,
		Code: "conflict", Message: "Wallet balance is not enough for this payment."}}
	svc := newPaymentTestService(db, pc)
	bookingID, providerID := seedAwaitingAcceptanceWithPayment(t, db, "intent-refused-test")

	_, err := svc.AcceptBooking(context.Background(), bookingID, providerID)
	appErr := apperrors.From(err)
	if appErr.Status != http.StatusConflict || appErr.Code != CodePaymentFailed {
		t.Fatalf("got %d %q (%v), want 409 %q", appErr.Status, appErr.Code, err, CodePaymentFailed)
	}
	final, ferr := svc.GetByIDForProvider(context.Background(), bookingID, providerID)
	if ferr != nil {
		t.Fatalf("GetByIDForProvider: %v", ferr)
	}
	if final.Status != bookingmodels.StatusAwaitingAccept || final.PaymentStatus != bookingmodels.PaymentUnpaid {
		t.Errorf("booking = %q/%q, want unchanged awaiting_acceptance/unpaid", final.Status, final.PaymentStatus)
	}
	if len(pc.refundCalls) != 0 {
		t.Errorf("a refused hold moved no money, so nothing may be refunded; got %d refunds", len(pc.refundCalls))
	}
}

// TestAcceptBooking_CompensatingRefundWhenDBTransitionLoses proves the
// narrow race AcceptBooking's own doc comment describes: the wallet hold
// succeeds, but the booking has ALREADY been reset (e.g. an internal
// timeout won in the same instant) by the time MarkAccepted's guarded CAS
// runs. The customer must not stay charged for an offer that no longer
// stands.
func TestAcceptBooking_CompensatingRefundWhenDBTransitionLoses(t *testing.T) {
	db := lifecycleTestDB(t)
	pc := &fakePaymentClient{}
	svc := newPaymentTestService(db, pc)
	bookingID, providerID := seedAwaitingAcceptanceWithPayment(t, db, "intent-race-test")

	// Simulate the timeout winning first: reset straight to pending_match
	// via raw SQL (bypassing the usecase, since we want the booking to be in
	// exactly this state the instant AcceptBooking's GetByIDForProvider
	// still succeeds -- GetByIDForProvider does not filter by status, only
	// MarkAccepted's guard does).
	mustExec(t, db, `UPDATE taxi_bookings SET status = 'pending_match', provider_id = NULL WHERE id = $1`, bookingID)
	// GetByIDForProvider filters on provider_id, which is now NULL -- so to
	// keep the read succeeding (matching the real race window where the
	// read happens BEFORE the reset), seed a variant that keeps provider_id
	// intact but status already moved on. This is the accurate reproduction
	// of "the read still sees the old provider assignment, but the guarded
	// write a moment later does not."
	mustExec(t, db, `UPDATE taxi_bookings SET provider_id = $1 WHERE id = $2`, providerID, bookingID)

	_, err := svc.AcceptBooking(context.Background(), bookingID, providerID)
	if err == nil {
		t.Fatal("expected MarkAccepted's guard to refuse a booking that already moved back to pending_match")
	}

	if len(pc.payCalls) != 1 {
		t.Fatalf("payCalls = %d, want exactly 1 (the hold that then needed unwinding)", len(pc.payCalls))
	}
	if len(pc.refundCalls) != 1 {
		t.Fatalf("refundCalls = %d, want exactly 1 (the compensating refund)", len(pc.refundCalls))
	}
	if pc.refundCalls[0].SourceReference != bookingID {
		t.Errorf("refund source_reference = %q, want %q", pc.refundCalls[0].SourceReference, bookingID)
	}
	// Same PaymentReference requirement as TestRefundIfHeld_RefundsWhenHeld
	// in payments_test.go -- this call site had the identical missing-field
	// bug (found live) since it builds its own walletclient.RefundRequest
	// independently of refundIfHeld rather than calling it.
	if pc.refundCalls[0].PaymentReference != bookingID {
		t.Errorf("refund PaymentReference = %q, want %q", pc.refundCalls[0].PaymentReference, bookingID)
	}
}

func TestMarkCompleted_SettlesPaymentAndMarksPaid(t *testing.T) {
	db := lifecycleTestDB(t)
	pc := &fakePaymentClient{}
	svc := newPaymentTestService(db, pc)
	bookingID, providerID := seedLifecycleFixture(t, db, bookingmodels.StatusInProgress, availabilitymodels.StateOnTrip)
	mustExec(t, db, `UPDATE taxi_bookings SET payment_method = 'wallet', payment_status = 'held', payment_intent_id = 'intent-settle-test' WHERE id = $1`, bookingID)

	booking, err := svc.MarkCompleted(context.Background(), CompleteBookingInput{
		BookingID: bookingID, ProviderID: providerID, ActualDistanceKm: 10, ActualDurationMin: 20,
	})
	if err != nil {
		t.Fatalf("MarkCompleted: %v", err)
	}
	if booking.Status != bookingmodels.StatusCompleted {
		t.Errorf("status = %q, want completed", booking.Status)
	}
	if booking.PaymentStatus != bookingmodels.PaymentPaid {
		t.Errorf("payment_status = %q, want paid", booking.PaymentStatus)
	}
	want := PaymentServiceName + "|" + bookingID
	if len(pc.completeCalls) != 1 || pc.completeCalls[0] != want {
		t.Errorf("completeCalls = %v, want exactly [%s]", pc.completeCalls, want)
	}
}

func TestMarkCompleted_SettlementFailureStillCompletesTheRide(t *testing.T) {
	db := lifecycleTestDB(t)
	pc := &fakePaymentClient{completeErr: errFakeWalletUnreachable}
	svc := newPaymentTestService(db, pc)
	bookingID, providerID := seedLifecycleFixture(t, db, bookingmodels.StatusInProgress, availabilitymodels.StateOnTrip)
	mustExec(t, db, `UPDATE taxi_bookings SET payment_method = 'wallet', payment_status = 'held', payment_intent_id = 'intent-settle-fail-test' WHERE id = $1`, bookingID)

	// Settlement is best-effort (see MarkCompleted's own doc comment) --
	// completion must still succeed for the passenger/driver even if the
	// wallet is unreachable.
	booking, err := svc.MarkCompleted(context.Background(), CompleteBookingInput{
		BookingID: bookingID, ProviderID: providerID, ActualDistanceKm: 10, ActualDurationMin: 20,
	})
	if err != nil {
		t.Fatalf("MarkCompleted should not fail when settlement fails: %v", err)
	}
	if booking.Status != bookingmodels.StatusCompleted {
		t.Errorf("status = %q, want completed even though settlement failed", booking.Status)
	}
	if booking.PaymentStatus != bookingmodels.PaymentHeld {
		t.Errorf("payment_status = %q, want unchanged held (settlement never succeeded)", booking.PaymentStatus)
	}
}

func TestCancelByCustomer_RefundsWhenPaymentWasHeld(t *testing.T) {
	db := lifecycleTestDB(t)
	pc := &fakePaymentClient{}
	svc := newPaymentTestService(db, pc)
	bookingID, providerID := seedLifecycleFixture(t, db, bookingmodels.StatusAccepted, availabilitymodels.StateOffered)
	_ = providerID
	mustExec(t, db, `UPDATE taxi_bookings SET payment_method = 'wallet', payment_status = 'held', payment_intent_id = 'intent-cancel-test' WHERE id = $1`, bookingID)
	customerID := customerIDOf(t, db, bookingID)

	booking, err := svc.CancelByCustomer(context.Background(), bookingID, customerID, "changed my mind")
	if err != nil {
		t.Fatalf("CancelByCustomer: %v", err)
	}
	if booking.Status != bookingmodels.StatusCancelled {
		t.Errorf("status = %q, want cancelled", booking.Status)
	}
	if booking.PaymentStatus != bookingmodels.PaymentRefunded {
		t.Errorf("payment_status = %q, want refunded", booking.PaymentStatus)
	}
	if len(pc.refundCalls) != 1 {
		t.Fatalf("refundCalls = %d, want exactly 1", len(pc.refundCalls))
	}
}

func TestCancelByCustomer_NoRefundWhenNeverHeld(t *testing.T) {
	db := lifecycleTestDB(t)
	pc := &fakePaymentClient{}
	svc := newPaymentTestService(db, pc)
	bookingID, _ := seedLifecycleFixture(t, db, bookingmodels.StatusPendingMatch, "")
	mustExec(t, db, `UPDATE taxi_bookings SET provider_id = NULL WHERE id = $1`, bookingID)
	customerID := customerIDOf(t, db, bookingID)

	if _, err := svc.CancelByCustomer(context.Background(), bookingID, customerID, "changed my mind"); err != nil {
		t.Fatalf("CancelByCustomer: %v", err)
	}
	if len(pc.refundCalls) != 0 {
		t.Error("a pre-accept cancel must never trigger a refund -- nothing was ever charged")
	}
}
