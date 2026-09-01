package bookingusecases

import (
	"context"
	"errors"
	"testing"

	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	"cosmicforge/logistics/shared/go/walletclient"
)

type fakePaymentClient struct {
	createErr, payErr, completeErr, refundErr error
	nextIntentID                              string

	createCalls   []walletclient.PaymentIntentRequest
	payCalls      []string // intent IDs
	completeCalls []string // "service|ref"
	refundCalls   []walletclient.RefundRequest
}

func (f *fakePaymentClient) CreatePaymentIntent(ctx context.Context, req walletclient.PaymentIntentRequest) (walletclient.PaymentIntent, error) {
	f.createCalls = append(f.createCalls, req)
	if f.createErr != nil {
		return walletclient.PaymentIntent{}, f.createErr
	}
	id := f.nextIntentID
	if id == "" {
		id = "intent-1"
	}
	return walletclient.PaymentIntent{ID: id, Reference: id, AmountKobo: req.AmountKobo, ProviderID: req.ProviderID}, nil
}

func (f *fakePaymentClient) PayFromWallet(ctx context.Context, paymentIntentID, idempotencyKey string) (walletclient.PaymentIntent, error) {
	f.payCalls = append(f.payCalls, paymentIntentID)
	if f.payErr != nil {
		return walletclient.PaymentIntent{}, f.payErr
	}
	return walletclient.PaymentIntent{ID: paymentIntentID, Status: "held"}, nil
}

func (f *fakePaymentClient) CompleteJob(ctx context.Context, sourceService, sourceReference string) (walletclient.PaymentIntent, error) {
	f.completeCalls = append(f.completeCalls, sourceService+"|"+sourceReference)
	if f.completeErr != nil {
		return walletclient.PaymentIntent{}, f.completeErr
	}
	return walletclient.PaymentIntent{Status: "settled"}, nil
}

func (f *fakePaymentClient) RequestRefund(ctx context.Context, req walletclient.RefundRequest) (walletclient.Refund, error) {
	f.refundCalls = append(f.refundCalls, req)
	if f.refundErr != nil {
		return walletclient.Refund{}, f.refundErr
	}
	return walletclient.Refund{Status: "refunded"}, nil
}

func walletBooking(overrides func(*bookingmodels.Booking)) bookingmodels.Booking {
	fare := int64(150000)
	b := bookingmodels.Booking{
		ID: "booking-1", CustomerID: "customer-1", PaymentMethod: "wallet",
		PaymentStatus: bookingmodels.PaymentUnpaid, FareEstimateKobo: &fare,
	}
	if overrides != nil {
		overrides(&b)
	}
	return b
}

// ─── createPaymentIntentForCandidate ───────────────────────────────────────

func TestCreatePaymentIntentForCandidate_SkipsWhenPaymentsNotConfigured(t *testing.T) {
	svc := NewBookingService(Options{}) // Payments left nil
	booking := walletBooking(nil)

	id, err := svc.createPaymentIntentForCandidate(context.Background(), booking, "provider-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != nil {
		t.Errorf("id = %v, want nil when payments is not configured", id)
	}
}

func TestCreatePaymentIntentForCandidate_AttachesProviderAtCreation(t *testing.T) {
	pc := &fakePaymentClient{nextIntentID: "intent-abc"}
	svc := NewBookingService(Options{Payments: pc})
	booking := walletBooking(nil)

	id, err := svc.createPaymentIntentForCandidate(context.Background(), booking, "provider-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id == nil || *id != "intent-abc" {
		t.Fatalf("id = %v, want intent-abc", id)
	}
	if len(pc.createCalls) != 1 {
		t.Fatalf("createCalls = %d, want 1", len(pc.createCalls))
	}
	req := pc.createCalls[0]
	if req.ProviderID != "provider-1" || req.ProviderType != "taxi" {
		t.Errorf("provider not attached at creation: ProviderID=%q ProviderType=%q", req.ProviderID, req.ProviderType)
	}
	if req.AmountKobo != 150000 {
		t.Errorf("AmountKobo = %d, want 150000", req.AmountKobo)
	}
	if req.SourceService != PaymentServiceName || req.SourceReference != booking.ID {
		t.Errorf("source pair = %s/%s, want %s/%s", req.SourceService, req.SourceReference, PaymentServiceName, booking.ID)
	}
}

func TestCreatePaymentIntentForCandidate_PropagatesError(t *testing.T) {
	pc := &fakePaymentClient{createErr: errors.New("wallet unreachable")}
	svc := NewBookingService(Options{Payments: pc})
	booking := walletBooking(nil)

	_, err := svc.createPaymentIntentForCandidate(context.Background(), booking, "provider-1")
	if err == nil {
		t.Fatal("expected the wallet error to propagate")
	}
}

// ─── holdPayment ────────────────────────────────────────────────────────────

func TestHoldPayment_SkipsWhenPaymentsNotConfigured(t *testing.T) {
	svc := NewBookingService(Options{}) // Payments left nil
	intentID := "intent-1"
	booking := walletBooking(func(b *bookingmodels.Booking) { b.PaymentIntentID = &intentID })

	// An intent id is set, so only the nil-client half of the guard can be
	// what stops the charge -- local dev without TAXI_PAYMENT_URL must book
	// without charging anyone rather than failing.
	if err := svc.holdPayment(context.Background(), booking); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHoldPayment_SkipsWhenNoIntentID(t *testing.T) {
	pc := &fakePaymentClient{}
	svc := NewBookingService(Options{Payments: pc})
	booking := walletBooking(nil) // PaymentIntentID left nil

	if err := svc.holdPayment(context.Background(), booking); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pc.payCalls) != 0 {
		t.Error("PayFromWallet should never be called with no intent id")
	}
}

func TestHoldPayment_ChargesTheStoredIntent(t *testing.T) {
	pc := &fakePaymentClient{}
	svc := NewBookingService(Options{Payments: pc})
	intentID := "intent-xyz"
	booking := walletBooking(func(b *bookingmodels.Booking) { b.PaymentIntentID = &intentID })

	if err := svc.holdPayment(context.Background(), booking); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pc.payCalls) != 1 || pc.payCalls[0] != intentID {
		t.Errorf("payCalls = %v, want exactly [%s]", pc.payCalls, intentID)
	}
}

func TestHoldPayment_PropagatesError(t *testing.T) {
	pc := &fakePaymentClient{payErr: errors.New("insufficient balance")}
	svc := NewBookingService(Options{Payments: pc})
	intentID := "intent-xyz"
	booking := walletBooking(func(b *bookingmodels.Booking) { b.PaymentIntentID = &intentID })

	if err := svc.holdPayment(context.Background(), booking); err == nil {
		t.Fatal("expected the wallet error to propagate -- a hold failure must block accept")
	}
}

// ─── settlePayment ──────────────────────────────────────────────────────────

func TestSettlePayment_UsesTheSourcePairNotTheIntentID(t *testing.T) {
	pc := &fakePaymentClient{}
	svc := NewBookingService(Options{Payments: pc})
	intentID := "intent-xyz"
	booking := walletBooking(func(b *bookingmodels.Booking) { b.PaymentIntentID = &intentID })

	if err := svc.settlePayment(context.Background(), booking); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := PaymentServiceName + "|" + booking.ID
	if len(pc.completeCalls) != 1 || pc.completeCalls[0] != want {
		t.Errorf("completeCalls = %v, want exactly [%s]", pc.completeCalls, want)
	}
}

func TestSettlePayment_SkipsWhenNoIntentID(t *testing.T) {
	pc := &fakePaymentClient{}
	svc := NewBookingService(Options{Payments: pc})
	booking := walletBooking(nil) // no PaymentIntentID: nothing was ever held

	if err := svc.settlePayment(context.Background(), booking); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pc.completeCalls) != 0 {
		t.Error("CompleteJob should never be called without a held intent")
	}
}

// ─── refundIfHeld ───────────────────────────────────────────────────────────

func TestRefundIfHeld_SkipsWhenNeverHeld(t *testing.T) {
	pc := &fakePaymentClient{}
	svc := NewBookingService(Options{Payments: pc})
	booking := walletBooking(nil) // PaymentStatus = unpaid

	if err := svc.refundIfHeld(context.Background(), booking, "cancelled"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pc.refundCalls) != 0 {
		t.Error("RequestRefund should never be called for a booking that was never held")
	}
}

func TestRefundIfHeld_RefundsWhenHeld(t *testing.T) {
	pc := &fakePaymentClient{}
	svc := NewBookingService(Options{Payments: pc})
	booking := walletBooking(func(b *bookingmodels.Booking) { b.PaymentStatus = bookingmodels.PaymentHeld })

	if err := svc.refundIfHeld(context.Background(), booking, "cancelled_by_customer: changed my mind"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pc.refundCalls) != 1 {
		t.Fatalf("refundCalls = %d, want 1", len(pc.refundCalls))
	}
	req := pc.refundCalls[0]
	if req.AmountKobo != 150000 {
		t.Errorf("AmountKobo = %d, want 150000", req.AmountKobo)
	}
	if req.SourceService != PaymentServiceName || req.SourceReference != booking.ID {
		t.Errorf("source pair = %s/%s, want %s/%s", req.SourceService, req.SourceReference, PaymentServiceName, booking.ID)
	}
	// Found live, not by this test: payment-wallet-service's RequestRefund
	// validates PaymentReference as required BEFORE it ever attempts the
	// source-pair fallback, so an empty PaymentReference here fails with
	// "Check your refund request." against every real refund, even though
	// SourceService/SourceReference are both correct. A fake wallet client
	// (as used throughout this file) does not enforce that rule, so this
	// exact field is the only thing standing between "tests green" and
	// "refunds broken in production" -- assert it explicitly.
	if req.PaymentReference != booking.ID {
		t.Errorf("PaymentReference = %q, want %q (required by payment-wallet-service before it tries the source-pair fallback)", req.PaymentReference, booking.ID)
	}
}

func TestRefundIfHeld_SkipsWhenPaymentsNotConfigured(t *testing.T) {
	svc := NewBookingService(Options{}) // Payments left nil
	booking := walletBooking(func(b *bookingmodels.Booking) {
		// Held status, so only the nil-client half of the guard can stop the
		// refund attempt.
		b.PaymentStatus = bookingmodels.PaymentHeld
	})

	if err := svc.refundIfHeld(context.Background(), booking, "cancelled"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ─── fareKobo ───────────────────────────────────────────────────────────────

func TestFareKobo_PrefersFinalOverEstimate(t *testing.T) {
	estimate := int64(100000)
	final := int64(125000)
	b := bookingmodels.Booking{FareEstimateKobo: &estimate, FareFinalKobo: &final}
	if got := fareKobo(b); got != final {
		t.Errorf("fareKobo = %d, want the final fare %d", got, final)
	}
}

func TestFareKobo_FallsBackToEstimate(t *testing.T) {
	estimate := int64(100000)
	b := bookingmodels.Booking{FareEstimateKobo: &estimate}
	if got := fareKobo(b); got != estimate {
		t.Errorf("fareKobo = %d, want the estimate %d", got, estimate)
	}
}

func TestFareKobo_ZeroWhenNeitherSet(t *testing.T) {
	if got := fareKobo(bookingmodels.Booking{}); got != 0 {
		t.Errorf("fareKobo = %d, want 0", got)
	}
}
