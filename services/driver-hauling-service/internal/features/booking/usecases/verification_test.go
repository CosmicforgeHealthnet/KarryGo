package bookingusecases

import (
	"context"
	"testing"
	"time"

	bookingmodels "cosmicforge/logistics/services/hauling-service/internal/features/booking/models"
	"cosmicforge/logistics/shared/go/handover"
)

func TestResendPickupCredential_EnforcesCooldownAndOwnership(t *testing.T) {
	const provider = "p1"
	const customer = "c1"
	future := time.Now().Add(time.Hour)
	rec, err := handover.NewCodeRecord(testHandoverSecret, handover.BuildChallengeID("hauling", handover.StepPickup, "b1"), time.Now())
	if err != nil {
		t.Fatalf("NewCodeRecord() error = %v", err)
	}
	sentJustNow := time.Now()
	repo := newTrackingRepo(bookingmodels.Booking{
		ID: "b1", CustomerID: customer, ReceiverCustomerID: customer, ProviderID: ptr(provider),
		Status:               bookingmodels.StatusArrivedAtPickup,
		PickupCode:           rec.Code,
		PickupCodeHash:       rec.Hash,
		PickupCodeExpiresAt:  &future,
		PickupCodeLastSentAt: &sentJustNow,
	})
	svc := lifecycleService(repo)
	ctx := context.Background()

	if _, err := svc.ResendPickupCredential(ctx, provider, "b1"); err == nil {
		t.Fatalf("expected ResendPickupCredential() to be rate-limited immediately after generation")
	}

	if _, err := svc.ResendPickupCredential(ctx, "intruder", "b1"); err == nil {
		t.Fatalf("expected ResendPickupCredential() to reject a non-owning provider")
	}

	// Simulate the cooldown having elapsed.
	past := time.Now().Add(-handover.ResendCooldown - time.Second)
	b := repo.bookings["b1"]
	b.PickupCodeLastSentAt = &past
	repo.bookings["b1"] = b

	view, err := svc.ResendPickupCredential(ctx, provider, "b1")
	if err != nil {
		t.Fatalf("ResendPickupCredential() error = %v", err)
	}
	if view.Code == "" || view.Code == rec.Code {
		t.Fatalf("expected a freshly generated, different code, got %q (was %q)", view.Code, rec.Code)
	}
}

func TestResendVerificationCode_DeliveryOnlyAndCooldown(t *testing.T) {
	const provider = "p1"
	const customer = "c1"
	future := time.Now().Add(time.Hour)
	rec, err := handover.NewCodeRecord(testHandoverSecret, handover.BuildChallengeID("hauling", handover.StepDelivery, "b1"), time.Now())
	if err != nil {
		t.Fatalf("NewCodeRecord() error = %v", err)
	}
	sentJustNow := time.Now()
	repo := newTrackingRepo(bookingmodels.Booking{
		ID: "b1", CustomerID: customer, ReceiverCustomerID: customer, ProviderID: ptr(provider),
		Status:                 bookingmodels.StatusEnRouteDelivery,
		DeliveryCode:           rec.Code,
		DeliveryCodeHash:       rec.Hash,
		DeliveryCodeExpiresAt:  &future,
		DeliveryCodeLastSentAt: &sentJustNow,
	})
	svc := lifecycleService(repo)
	ctx := context.Background()

	// A customer must never be able to fetch or resend the driver's pickup
	// credential.
	if _, err := svc.GetVerificationCode(ctx, customer, "b1", "pickup"); err == nil {
		t.Fatalf("expected GetVerificationCode(step=pickup) to be rejected for a customer")
	}
	if _, err := svc.ResendVerificationCode(ctx, customer, "b1", "pickup"); err == nil {
		t.Fatalf("expected ResendVerificationCode(step=pickup) to be rejected for a customer")
	}

	if _, err := svc.ResendVerificationCode(ctx, customer, "b1", "delivery"); err == nil {
		t.Fatalf("expected ResendVerificationCode(delivery) to be rate-limited immediately after generation")
	}

	past := time.Now().Add(-handover.ResendCooldown - time.Second)
	b := repo.bookings["b1"]
	b.DeliveryCodeLastSentAt = &past
	repo.bookings["b1"] = b

	view, err := svc.ResendVerificationCode(ctx, customer, "b1", "delivery")
	if err != nil {
		t.Fatalf("ResendVerificationCode() error = %v", err)
	}
	if view.Code == "" || view.Code == rec.Code {
		t.Fatalf("expected a freshly generated, different code, got %q (was %q)", view.Code, rec.Code)
	}
}
