package bookingusecases

import (
	"context"
	"errors"
	"testing"

	bookingmodels "cosmicforge/logistics/services/hauling-service/internal/features/booking/models"
)

// bindingGuardPayments records whether money was released, and answers the
// Section 17 binding guard however the test needs.
type bindingGuardPayments struct {
	unknownPaymentClient
	ensureErr    error
	ensuredFor   string
	settleCalled bool
}

func (p *bindingGuardPayments) EnsureExecutionProvider(_ context.Context, _ string, providerID string) error {
	p.ensuredFor = providerID
	return p.ensureErr
}

func (p *bindingGuardPayments) Settle(context.Context, string, string) error {
	p.settleCalled = true
	return nil
}

func heldBooking(provider string) *fakeBookingRepo {
	reference := "pay_ref"
	return &fakeBookingRepo{booking: bookingmodels.Booking{
		ID: "booking-guard", CustomerID: "customer", ProviderID: &provider,
		PaymentMethod: bookingmodels.PaymentMethodWallet, PaymentStatus: bookingmodels.PaymentStatusHeld,
		PaymentIntentID: &reference,
	}}
}

// The payment is confirmed bound to the provider who did the job BEFORE any
// money is released.
func TestSettlementConfirmsTheBindingBeforeReleasingMoney(t *testing.T) {
	repo := heldBooking("replacement-provider")
	payments := &bindingGuardPayments{}
	service := NewBookingService(Options{Bookings: repo, Payments: payments})

	service.settleOnComplete(context.Background(), repo.booking.ID)

	if payments.ensuredFor != "replacement-provider" {
		t.Fatalf("guard checked %q, want the booking's current provider", payments.ensuredFor)
	}
	if !payments.settleCalled || repo.booking.PaymentStatus != bookingmodels.PaymentStatusPaid {
		t.Fatalf("a confirmed binding must settle: settled=%v status=%s",
			payments.settleCalled, repo.booking.PaymentStatus)
	}
}

// An unconfirmable binding releases nothing and leaves the payment visible
// for reconciliation -- it never pays whoever the payment happens to name.
func TestSettlementRefusesAnUnconfirmableBinding(t *testing.T) {
	repo := heldBooking("replacement-provider")
	payments := &bindingGuardPayments{ensureErr: errors.New("no recovery case explains the mismatch")}
	service := NewBookingService(Options{Bookings: repo, Payments: payments})

	service.settleOnComplete(context.Background(), repo.booking.ID)

	if payments.settleCalled {
		t.Fatal("money was released without a confirmed beneficiary")
	}
	if repo.booking.PaymentStatus != bookingmodels.PaymentStatusUnknown {
		t.Fatalf("status=%s, want unknown so reconciliation sees it", repo.booking.PaymentStatus)
	}
}
