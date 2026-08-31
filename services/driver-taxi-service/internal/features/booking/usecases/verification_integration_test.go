package bookingusecases

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	bookingrepositories "cosmicforge/logistics/services/taxi-service/internal/features/booking/repositories"
	bookingverification "cosmicforge/logistics/services/taxi-service/internal/features/booking/verification"
	availabilitymodels "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/models"
	availabilityrepositories "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
)

// newVerificationService is newLifecycleService plus real pickup-verification
// wiring (Postgres repository + Redis rate limiter) -- a separate
// constructor from newLifecycleService since most other lifecycle tests
// don't need any of this.
func newVerificationService(db *pgxpool.Pool, redisClient *redis.Client) *BookingService {
	bookings := bookingrepositories.NewPostgresBookingRepository(db)
	opState := availabilityrepositories.NewPostgresOperationalStateRepository(db)
	return NewBookingService(Options{
		Bookings: bookings, Markets: newFakeMarkets(), Pricing: newFakePricing(), OperationalState: opState,
		PickupVerifications: bookingverification.NewPostgresRepository(db),
		VerifyRateLimiter:   bookingverification.NewRateLimiter(redisClient),
		Verification: VerificationConfig{
			HandoverSecret: []byte("test-handover-secret"),
			RateWindow:     time.Minute,
			MaxAttempts:    10, // high on purpose: most of these tests are not about rate limiting itself
		},
	})
}

// TestPickupVerification_FullRoundTrip is the composition proof: arrival
// generates a real code, the passenger reads it back, the driver submits it,
// and ONLY THEN does MarkInProgress unlock -- exercising every piece this
// session built (repository, rate limiter, usecase, and the real B3
// lifecycle gate) together against real Postgres and real Redis.
func TestPickupVerification_FullRoundTrip(t *testing.T) {
	db := lifecycleTestDB(t)
	redisClient := matchingTestRedis(t)
	svc := newVerificationService(db, redisClient)
	bookingID, providerID := seedLifecycleFixture(t, db, bookingmodels.StatusEnRoutePickup, availabilitymodels.StateEnRoutePickup)
	customerID := customerIDOf(t, db, bookingID)

	// Before arrival: no code exists yet.
	if _, err := svc.GetPickupCode(context.Background(), bookingID, customerID); err == nil {
		t.Fatal("expected GetPickupCode to fail before the driver has arrived")
	}

	// Driver arrives: a code is generated as part of the SAME transaction.
	if _, err := svc.MarkArrivedAtPickup(context.Background(), bookingID, providerID); err != nil {
		t.Fatalf("MarkArrivedAtPickup: %v", err)
	}

	// Starting now must be refused: not verified yet.
	if _, err := svc.MarkInProgress(context.Background(), bookingID, providerID); err == nil {
		t.Fatal("expected MarkInProgress to be refused before verification")
	}

	// Passenger reads their code.
	credential, err := svc.GetPickupCode(context.Background(), bookingID, customerID)
	if err != nil {
		t.Fatalf("GetPickupCode: %v", err)
	}
	if len(credential.Code) == 0 {
		t.Fatal("expected a non-empty code")
	}

	// Driver submits the WRONG code first.
	if _, err := svc.VerifyPickup(context.Background(), bookingID, providerID, "000000"); err == nil {
		t.Fatal("expected the wrong code to be refused")
	}

	// Driver submits the RIGHT code.
	booking, err := svc.VerifyPickup(context.Background(), bookingID, providerID, credential.Code)
	if err != nil {
		t.Fatalf("VerifyPickup (correct code): %v", err)
	}
	if booking.PickupVerificationStatus != bookingmodels.PickupVerificationVerified {
		t.Errorf("PickupVerificationStatus = %q, want verified", booking.PickupVerificationStatus)
	}

	// Now starting must succeed.
	started, err := svc.MarkInProgress(context.Background(), bookingID, providerID)
	if err != nil {
		t.Fatalf("MarkInProgress after verification: %v", err)
	}
	if started.Status != bookingmodels.StatusInProgress {
		t.Errorf("status = %q, want in_progress", started.Status)
	}
}

func TestVerifyPickup_WrongCodeIncrementsAttemptsAndDoesNotUnlock(t *testing.T) {
	db := lifecycleTestDB(t)
	redisClient := matchingTestRedis(t)
	svc := newVerificationService(db, redisClient)
	bookingID, providerID := seedLifecycleFixture(t, db, bookingmodels.StatusEnRoutePickup, availabilitymodels.StateEnRoutePickup)

	if _, err := svc.MarkArrivedAtPickup(context.Background(), bookingID, providerID); err != nil {
		t.Fatalf("MarkArrivedAtPickup: %v", err)
	}

	_, err := svc.VerifyPickup(context.Background(), bookingID, providerID, "999999")
	if err == nil {
		t.Fatal("expected the wrong code to be refused")
	}

	rec, gerr := bookingverification.NewPostgresRepository(db).Get(context.Background(), bookingID)
	if gerr != nil {
		t.Fatalf("Get: %v", gerr)
	}
	if rec.Attempts != 1 {
		t.Errorf("Attempts = %d, want 1", rec.Attempts)
	}
	if rec.VerifiedAt != nil {
		t.Error("a wrong code must never set VerifiedAt")
	}
}

func TestVerifyPickup_WrongProviderRefused(t *testing.T) {
	db := lifecycleTestDB(t)
	redisClient := matchingTestRedis(t)
	svc := newVerificationService(db, redisClient)
	bookingID, providerID := seedLifecycleFixture(t, db, bookingmodels.StatusEnRoutePickup, availabilitymodels.StateEnRoutePickup)

	if _, err := svc.MarkArrivedAtPickup(context.Background(), bookingID, providerID); err != nil {
		t.Fatalf("MarkArrivedAtPickup: %v", err)
	}
	rec, err := bookingverification.NewPostgresRepository(db).Get(context.Background(), bookingID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	imposterID := uuid.NewString() // a real UUID, just not the assigned provider
	_, verr := svc.VerifyPickup(context.Background(), bookingID, imposterID, rec.Code)
	if verr == nil {
		t.Fatal("expected verification to be refused for a provider who is not assigned to this booking")
	}
	if appErr := apperrors.From(verr); appErr == nil || appErr.Code != apperrors.CodeNotFound {
		t.Fatalf("expected NotFound (ownership check), got %v", verr)
	}
}

func TestVerifyPickup_AlreadyVerifiedIsIdempotent(t *testing.T) {
	db := lifecycleTestDB(t)
	redisClient := matchingTestRedis(t)
	svc := newVerificationService(db, redisClient)
	bookingID, providerID := seedLifecycleFixture(t, db, bookingmodels.StatusEnRoutePickup, availabilitymodels.StateEnRoutePickup)

	if _, err := svc.MarkArrivedAtPickup(context.Background(), bookingID, providerID); err != nil {
		t.Fatalf("MarkArrivedAtPickup: %v", err)
	}
	credential, err := svc.GetPickupCode(context.Background(), bookingID, customerIDOf(t, db, bookingID))
	if err != nil {
		t.Fatalf("GetPickupCode: %v", err)
	}
	if _, err := svc.VerifyPickup(context.Background(), bookingID, providerID, credential.Code); err != nil {
		t.Fatalf("first VerifyPickup: %v", err)
	}

	// A duplicate submit of the same correct code (e.g. a retried request)
	// must succeed as a no-op, not error as if the code were now invalid.
	booking, err := svc.VerifyPickup(context.Background(), bookingID, providerID, credential.Code)
	if err != nil {
		t.Fatalf("second VerifyPickup (duplicate): %v", err)
	}
	if booking.PickupVerificationStatus != bookingmodels.PickupVerificationVerified {
		t.Errorf("PickupVerificationStatus = %q, want verified", booking.PickupVerificationStatus)
	}
}

func TestResendPickupCode_IssuesANewCodeThatSupersedesTheOld(t *testing.T) {
	db := lifecycleTestDB(t)
	redisClient := matchingTestRedis(t)
	svc := newVerificationService(db, redisClient)
	bookingID, providerID := seedLifecycleFixture(t, db, bookingmodels.StatusEnRoutePickup, availabilitymodels.StateEnRoutePickup)
	customerID := customerIDOf(t, db, bookingID)

	if _, err := svc.MarkArrivedAtPickup(context.Background(), bookingID, providerID); err != nil {
		t.Fatalf("MarkArrivedAtPickup: %v", err)
	}
	original, err := svc.GetPickupCode(context.Background(), bookingID, customerID)
	if err != nil {
		t.Fatalf("GetPickupCode: %v", err)
	}

	fresh, err := svc.ResendPickupCode(context.Background(), bookingID, customerID)
	if err != nil {
		t.Fatalf("ResendPickupCode: %v", err)
	}

	// The old code must no longer verify -- only the fresh one should.
	if _, err := svc.VerifyPickup(context.Background(), bookingID, providerID, original.Code); err == nil && original.Code != fresh.Code {
		t.Error("the original code should no longer verify after a resend")
	}
	if _, err := svc.VerifyPickup(context.Background(), bookingID, providerID, fresh.Code); err != nil {
		t.Fatalf("the freshly resent code should verify: %v", err)
	}
}

func TestGetPickupCode_WrongCustomerCannotReadAnothersCode(t *testing.T) {
	db := lifecycleTestDB(t)
	redisClient := matchingTestRedis(t)
	svc := newVerificationService(db, redisClient)
	bookingID, providerID := seedLifecycleFixture(t, db, bookingmodels.StatusEnRoutePickup, availabilitymodels.StateEnRoutePickup)

	if _, err := svc.MarkArrivedAtPickup(context.Background(), bookingID, providerID); err != nil {
		t.Fatalf("MarkArrivedAtPickup: %v", err)
	}

	_, err := svc.GetPickupCode(context.Background(), bookingID, uuid.NewString())
	if err == nil {
		t.Fatal("expected GetPickupCode to be refused for a customer who does not own this booking")
	}
}
