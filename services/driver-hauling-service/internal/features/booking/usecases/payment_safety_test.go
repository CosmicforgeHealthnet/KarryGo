package bookingusecases

import (
	"context"
	"errors"
	"testing"

	bookingmodels "cosmicforge/logistics/services/hauling-service/internal/features/booking/models"
)

type unknownPaymentClient struct{}

func (unknownPaymentClient) HoldFromWallet(context.Context, PaymentHoldInput) (string, error) {
	return "payment-reference", ErrPaymentOutcomeUnknown
}
func (unknownPaymentClient) CreateCardIntent(context.Context, PaymentHoldInput, string) (string, string, error) {
	return "", "", errors.New("unused")
}
func (unknownPaymentClient) Settle(context.Context, string, string) error {
	return ErrPaymentOutcomeUnknown
}
func (unknownPaymentClient) Refund(context.Context, string, int64, string) error {
	return ErrPaymentOutcomeUnknown
}

func TestWalletHoldUnknownOutcomeRemainsUnknown(t *testing.T) {
	fare := int64(5000)
	repo := &fakeBookingRepo{booking: bookingmodels.Booking{
		ID: "booking-payment-unknown", CustomerID: "customer", PaymentMethod: bookingmodels.PaymentMethodWallet,
		PaymentStatus: bookingmodels.PaymentStatusUnpaid, FareEstimateKobo: &fare,
	}}
	service := NewBookingService(Options{Bookings: repo, Payments: unknownPaymentClient{}})
	err := service.ensurePaymentSecured(context.Background(), repo.booking)
	if err == nil {
		t.Fatal("unknown Wallet outcome must fail the acceptance attempt")
	}
	if repo.booking.PaymentStatus != bookingmodels.PaymentStatusUnknown {
		t.Fatalf("payment status=%s want=%s", repo.booking.PaymentStatus, bookingmodels.PaymentStatusUnknown)
	}
	if repo.booking.PaymentIntentID == nil || *repo.booking.PaymentIntentID != "payment-reference" {
		t.Fatal("authoritative payment reference must be retained for reconciliation")
	}
}

func TestMissingWalletClientFailsClosed(t *testing.T) {
	fare := int64(5000)
	repo := &fakeBookingRepo{booking: bookingmodels.Booking{
		ID: "booking-no-wallet", CustomerID: "customer", PaymentMethod: bookingmodels.PaymentMethodWallet,
		PaymentStatus: bookingmodels.PaymentStatusUnpaid, FareEstimateKobo: &fare,
	}}
	service := NewBookingService(Options{Bookings: repo})
	if err := service.ensurePaymentSecured(context.Background(), repo.booking); err == nil {
		t.Fatal("missing Wallet client must fail closed")
	}
	if repo.booking.PaymentStatus == bookingmodels.PaymentStatusPaid || repo.booking.PaymentStatus == bookingmodels.PaymentStatusHeld {
		t.Fatalf("missing Wallet fabricated payment status %s", repo.booking.PaymentStatus)
	}
}

func TestCashBookingsAreDisabledByDefault(t *testing.T) {
	service := NewBookingService(Options{Bookings: &fakeBookingRepo{}})
	_, err := service.CreateBooking(context.Background(), CreateBookingInput{
		CustomerID: "customer", PickupAddress: "A", DropoffAddress: "B",
		PickupLat: 6.5, PickupLng: 3.3, DropoffLat: 6.6, DropoffLng: 3.4,
		CargoWeightKg: 100, PaymentMethod: bookingmodels.PaymentMethodCash,
	})
	if err == nil {
		t.Fatal("cash payments must be rejected unless explicitly enabled")
	}
}
