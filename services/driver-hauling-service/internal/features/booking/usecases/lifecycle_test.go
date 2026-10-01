package bookingusecases

import (
	"context"
	"testing"
	"time"

	bookingclients "cosmicforge/logistics/services/hauling-service/internal/features/booking/clients"
	bookingmodels "cosmicforge/logistics/services/hauling-service/internal/features/booking/models"
	"cosmicforge/logistics/shared/go/handover"
)

// lifecycleService builds a BookingService with no availability/payment wiring —
// enough to exercise the provider trip-progress usecases (ownership + status).
func lifecycleService(repo *trackingRepo) *BookingService {
	return NewBookingService(Options{
		Bookings:       repo,
		Notifier:       bookingclients.NewBookingNotifierWith(&recordingNotifier{}),
		HandoverSecret: testHandoverSecret,
	})
}

// TestTripLifecycle_AdvancesThroughAllStages walks an accepted booking through
// the full provider-driven trip lifecycle, including the mirrored handover
// verification steps: the customer verifies the driver's pickup credential
// before ConfirmPickup is allowed, and the driver verifies the customer's
// delivery code inside ConfirmDelivery.
func TestTripLifecycle_AdvancesThroughAllStages(t *testing.T) {
	const provider = "p1"
	const customer = "c1"
	repo := newTrackingRepo(bookingmodels.Booking{
		ID: "b1", CustomerID: customer, ReceiverCustomerID: customer, ProviderID: ptr(provider),
		Status: bookingmodels.StatusAccepted,
	})
	svc := lifecycleService(repo)
	ctx := context.Background()

	if _, err := svc.MarkEnRoutePickup(ctx, "b1", provider); err != nil {
		t.Fatalf("MarkEnRoutePickup() error = %v", err)
	}
	if got := repo.statusOf("b1"); got != bookingmodels.StatusEnRoutePickup {
		t.Fatalf("after en_route_pickup: status = %s, want %s", got, bookingmodels.StatusEnRoutePickup)
	}

	arrived, err := svc.MarkArrivedAtPickup(ctx, "b1", provider)
	if err != nil {
		t.Fatalf("MarkArrivedAtPickup() error = %v", err)
	}
	if got := repo.statusOf("b1"); got != bookingmodels.StatusArrivedAtPickup {
		t.Fatalf("after arrived: status = %s, want %s", got, bookingmodels.StatusArrivedAtPickup)
	}
	if arrived.PickupCredential.Code == "" {
		t.Fatalf("expected a pickup credential to be generated for the driver")
	}

	// The driver cannot confirm pickup until the customer verifies them.
	if _, err := svc.ConfirmPickup(ctx, "b1", provider); err == nil {
		t.Fatalf("expected ConfirmPickup() to fail before customer verification")
	}

	if _, err := svc.VerifyDriverAtPickup(ctx, customer, "b1", arrived.PickupCredential.Code); err != nil {
		t.Fatalf("VerifyDriverAtPickup() error = %v", err)
	}

	if _, err := svc.ConfirmPickup(ctx, "b1", provider); err != nil {
		t.Fatalf("ConfirmPickup() error = %v", err)
	}
	if got := repo.statusOf("b1"); got != bookingmodels.StatusPickedUp {
		t.Fatalf("after picked_up: status = %s, want %s", got, bookingmodels.StatusPickedUp)
	}

	if _, err := svc.MarkEnRouteDelivery(ctx, "b1", provider); err != nil {
		t.Fatalf("MarkEnRouteDelivery() error = %v", err)
	}
	if got := repo.statusOf("b1"); got != bookingmodels.StatusEnRouteDelivery {
		t.Fatalf("after en_route_delivery: status = %s, want %s", got, bookingmodels.StatusEnRouteDelivery)
	}

	deliveryView, err := svc.GetVerificationCode(ctx, customer, "b1", "delivery")
	if err != nil {
		t.Fatalf("GetVerificationCode() error = %v", err)
	}
	if deliveryView.Code != "" {
		t.Fatalf("generic read must not redisplay the delivery code")
	}
	repo.mu.Lock()
	deliveryCode := repo.bookings["b1"].DeliveryCode // simulates the one-time notification boundary
	repo.mu.Unlock()
	if deliveryCode == "" {
		t.Fatal("expected the issue path to generate a delivery code")
	}

	// The driver cannot confirm delivery with a wrong code.
	if _, err := svc.ConfirmDelivery(ctx, "b1", provider, "000000", "https://media/proof.jpg", "https://media/sig.png"); err == nil {
		t.Fatalf("expected ConfirmDelivery() to fail with an incorrect code")
	}

	if _, err := svc.ConfirmDelivery(ctx, "b1", provider, deliveryCode, "https://media/proof.jpg", "https://media/sig.png"); err != nil {
		t.Fatalf("ConfirmDelivery() error = %v", err)
	}
	if got := repo.statusOf("b1"); got != bookingmodels.StatusDelivered {
		t.Fatalf("after delivered: status = %s, want %s", got, bookingmodels.StatusDelivered)
	}
}

// TestTripLifecycle_RejectsWrongProvider ensures a provider cannot advance a
// booking they don't own.
func TestTripLifecycle_RejectsWrongProvider(t *testing.T) {
	repo := newTrackingRepo(bookingmodels.Booking{
		ID: "b1", CustomerID: "c1", ProviderID: ptr("owner"),
		Status: bookingmodels.StatusAccepted,
	})
	svc := lifecycleService(repo)

	if _, err := svc.MarkEnRoutePickup(context.Background(), "b1", "intruder"); err == nil {
		t.Fatal("expected a non-owner provider to be rejected")
	}
	if got := repo.statusOf("b1"); got != bookingmodels.StatusAccepted {
		t.Fatalf("status should be unchanged, got %s", got)
	}
}

// TestVerifyDriverAtPickup_RejectsWrongCode ensures an incorrect customer
// submission is rejected and counted as a failed attempt, without unblocking
// the driver's ConfirmPickup.
func TestVerifyDriverAtPickup_RejectsWrongCode(t *testing.T) {
	const provider = "p1"
	const customer = "c1"
	repo := newTrackingRepo(bookingmodels.Booking{
		ID: "b1", CustomerID: customer, ProviderID: ptr(provider),
		Status: bookingmodels.StatusEnRoutePickup,
	})
	svc := lifecycleService(repo)
	ctx := context.Background()

	if _, err := svc.MarkArrivedAtPickup(ctx, "b1", provider); err != nil {
		t.Fatalf("MarkArrivedAtPickup() error = %v", err)
	}

	if _, err := svc.VerifyDriverAtPickup(ctx, customer, "b1", "000000"); err == nil {
		t.Fatalf("expected VerifyDriverAtPickup() to fail with an incorrect code")
	}
	if repo.bookings["b1"].PickupCodeAttempts != 1 {
		t.Fatalf("expected a failed attempt to be recorded, got %d", repo.bookings["b1"].PickupCodeAttempts)
	}
	if _, err := svc.ConfirmPickup(ctx, "b1", provider); err == nil {
		t.Fatalf("expected ConfirmPickup() to remain blocked after a failed verification")
	}
}

// TestVerifyDriverAtPickup_RejectsExpiredCode ensures a stale pickup
// credential cannot be verified even with the correct digits.
func TestVerifyDriverAtPickup_RejectsExpiredCode(t *testing.T) {
	const provider = "p1"
	const customer = "c1"
	expired := time.Now().Add(-time.Minute)
	rec, err := handover.NewCodeRecord(testHandoverSecret, handover.BuildChallengeID("hauling", handover.StepPickup, "b1"), time.Now())
	if err != nil {
		t.Fatalf("NewCodeRecord() error = %v", err)
	}
	repo := newTrackingRepo(bookingmodels.Booking{
		ID: "b1", CustomerID: customer, ProviderID: ptr(provider),
		Status:              bookingmodels.StatusArrivedAtPickup,
		PickupCode:          rec.Code,
		PickupCodeHash:      rec.Hash,
		PickupCodeExpiresAt: &expired,
	})
	svc := lifecycleService(repo)

	if _, err := svc.VerifyDriverAtPickup(context.Background(), customer, "b1", rec.Code); err == nil {
		t.Fatalf("expected VerifyDriverAtPickup() to fail with an expired code")
	}
}

// TestVerifyDriverAtPickup_RejectsAfterMaxAttempts ensures the attempt
// ceiling locks out further verification of the same code.
func TestVerifyDriverAtPickup_RejectsAfterMaxAttempts(t *testing.T) {
	const provider = "p1"
	const customer = "c1"
	future := time.Now().Add(time.Hour)
	rec, err := handover.NewCodeRecord(testHandoverSecret, handover.BuildChallengeID("hauling", handover.StepPickup, "b1"), time.Now())
	if err != nil {
		t.Fatalf("NewCodeRecord() error = %v", err)
	}
	repo := newTrackingRepo(bookingmodels.Booking{
		ID: "b1", CustomerID: customer, ProviderID: ptr(provider),
		Status:              bookingmodels.StatusArrivedAtPickup,
		PickupCode:          rec.Code,
		PickupCodeHash:      rec.Hash,
		PickupCodeExpiresAt: &future,
		PickupCodeAttempts:  handover.MaxVerifyAttempts,
	})
	svc := lifecycleService(repo)

	if _, err := svc.VerifyDriverAtPickup(context.Background(), customer, "b1", rec.Code); err == nil {
		t.Fatalf("expected VerifyDriverAtPickup() to fail once attempts are exceeded, even with the correct code")
	}
}

// TestConfirmDelivery_RejectsExpiredCode ensures a stale delivery code cannot
// be used to complete delivery even with the correct digits.
func TestConfirmDelivery_RejectsExpiredCode(t *testing.T) {
	const provider = "p1"
	const customer = "c1"
	expired := time.Now().Add(-time.Minute)
	rec, err := handover.NewCodeRecord(testHandoverSecret, handover.BuildChallengeID("hauling", handover.StepDelivery, "b1"), time.Now())
	if err != nil {
		t.Fatalf("NewCodeRecord() error = %v", err)
	}
	repo := newTrackingRepo(bookingmodels.Booking{
		ID: "b1", CustomerID: customer, ProviderID: ptr(provider),
		Status:                bookingmodels.StatusEnRouteDelivery,
		DeliveryCode:          rec.Code,
		DeliveryCodeHash:      rec.Hash,
		DeliveryCodeExpiresAt: &expired,
	})
	svc := lifecycleService(repo)

	if _, err := svc.ConfirmDelivery(context.Background(), "b1", provider, rec.Code, "", ""); err == nil {
		t.Fatalf("expected ConfirmDelivery() to fail with an expired code")
	}
}

// TestConfirmDelivery_RejectsAfterMaxAttempts ensures the attempt ceiling
// locks out further delivery confirmation attempts for the same code.
func TestConfirmDelivery_RejectsAfterMaxAttempts(t *testing.T) {
	const provider = "p1"
	const customer = "c1"
	future := time.Now().Add(time.Hour)
	rec, err := handover.NewCodeRecord(testHandoverSecret, handover.BuildChallengeID("hauling", handover.StepDelivery, "b1"), time.Now())
	if err != nil {
		t.Fatalf("NewCodeRecord() error = %v", err)
	}
	repo := newTrackingRepo(bookingmodels.Booking{
		ID: "b1", CustomerID: customer, ProviderID: ptr(provider),
		Status:                bookingmodels.StatusEnRouteDelivery,
		DeliveryCode:          rec.Code,
		DeliveryCodeHash:      rec.Hash,
		DeliveryCodeExpiresAt: &future,
		DeliveryCodeAttempts:  handover.MaxVerifyAttempts,
	})
	svc := lifecycleService(repo)

	if _, err := svc.ConfirmDelivery(context.Background(), "b1", provider, rec.Code, "", ""); err == nil {
		t.Fatalf("expected ConfirmDelivery() to fail once attempts are exceeded, even with the correct code")
	}
}

func ptr(s string) *string { return &s }
