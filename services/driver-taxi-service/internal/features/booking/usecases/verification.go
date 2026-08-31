package bookingusecases

import (
	"context"
	"strings"
	"time"

	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	bookingverification "cosmicforge/logistics/services/taxi-service/internal/features/booking/verification"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/handover"
)

// pickupChallengeID namespaces this booking's pickup code among every other
// handover code in the platform (hauling/dispatch each have their own
// service name in this same call, per handover.BuildChallengeID's own doc
// comment) and among taxi's own future handover uses.
func pickupChallengeID(bookingID string) string {
	return handover.BuildChallengeID("taxi", handover.StepPickup, bookingID)
}

// generatePickupCode is called from MarkArrivedAtPickup, inside its own
// transaction. A nil PickupVerifications repository is a deliberate no-op,
// not an error: several existing tests build a BookingService without
// wiring verification at all (see Options' own doc comment), and arrival
// itself must still succeed for them. Any REAL deployment wires this, so a
// booking left without a generated code simply can never pass
// MarkInProgress's gate -- a safe, honest degrade, not a silent bypass.
func (s *BookingService) generatePickupCode(ctx context.Context, exec Executor, bookingID string) error {
	if s.pickupVerifications == nil {
		return nil
	}
	rec, err := handover.NewCodeRecord(s.verification.HandoverSecret, pickupChallengeID(bookingID), time.Now())
	if err != nil {
		return apperrors.Internal("A pickup code could not be generated.", err)
	}
	return s.pickupVerifications.Upsert(ctx, exec, bookingverification.Record{BookingID: bookingID, CodeRecord: rec})
}

// PickupCredential is the passenger-facing view of the code: a bare digit
// string, rendered as text and (client-side) as a QR code from that same
// string -- see internal/features/booking/verification's own package doc
// comment for why this build has one secret, not a separate QR payload.
type PickupCredential struct {
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expires_at"`
}

// GetPickupCode returns the passenger's current pickup code. NotFound if
// the driver has not arrived yet (no code generated).
func (s *BookingService) GetPickupCode(ctx context.Context, bookingID, customerID string) (PickupCredential, error) {
	bookingID, customerID = strings.TrimSpace(bookingID), strings.TrimSpace(customerID)
	if s.pickupVerifications == nil {
		return PickupCredential{}, apperrors.Unavailable("Pickup verification is not available right now.", nil)
	}
	// Ownership check first: a customer must not be able to probe another
	// customer's booking for whether a code exists.
	if _, err := s.bookings.GetByIDForCustomer(ctx, bookingID, customerID); err != nil {
		return PickupCredential{}, err
	}
	rec, err := s.pickupVerifications.Get(ctx, bookingID)
	if err != nil {
		return PickupCredential{}, err
	}
	return PickupCredential{Code: rec.Code, ExpiresAt: rec.ExpiresAt}, nil
}

// ResendPickupCode regenerates the code (a fresh random value, same
// challenge ID so the old one stops verifying), subject to handover's own
// CanResend cooldown/max-resends decision.
func (s *BookingService) ResendPickupCode(ctx context.Context, bookingID, customerID string) (PickupCredential, error) {
	bookingID, customerID = strings.TrimSpace(bookingID), strings.TrimSpace(customerID)
	if s.pickupVerifications == nil {
		return PickupCredential{}, apperrors.Unavailable("Pickup verification is not available right now.", nil)
	}
	if _, err := s.bookings.GetByIDForCustomer(ctx, bookingID, customerID); err != nil {
		return PickupCredential{}, err
	}

	existing, err := s.pickupVerifications.Get(ctx, bookingID)
	if err != nil {
		return PickupCredential{}, err
	}
	now := time.Now()
	if ok, wait := existing.CanResend(now); !ok {
		rateErr := apperrors.RateLimited("Please wait before requesting a new code.", nil)
		rateErr.Details = map[string]interface{}{"retry_after_seconds": int(wait.Seconds())}
		return PickupCredential{}, rateErr
	}

	fresh, err := handover.NewCodeRecord(s.verification.HandoverSecret, pickupChallengeID(bookingID), now)
	if err != nil {
		return PickupCredential{}, apperrors.Internal("A pickup code could not be generated.", err)
	}
	fresh.ResendCount = existing.ResendCount + 1
	fresh.LastSentAt = &now

	if err := s.pickupVerifications.Upsert(ctx, nil, bookingverification.Record{BookingID: bookingID, CodeRecord: fresh}); err != nil {
		return PickupCredential{}, err
	}
	return PickupCredential{Code: fresh.Code, ExpiresAt: fresh.ExpiresAt}, nil
}

// VerifyPickup is the driver-initiated check: the passenger has told them
// (or they scanned) a code, and this either unlocks MarkInProgress or
// refuses with a clear, specific reason. Rate-limited on top of handover's
// own durable per-code attempt cap -- see VerificationConfig's own doc
// comment for why both exist.
func (s *BookingService) VerifyPickup(ctx context.Context, bookingID, providerID, code string) (bookingmodels.Booking, error) {
	bookingID, providerID, code = strings.TrimSpace(bookingID), strings.TrimSpace(providerID), strings.TrimSpace(code)
	if s.pickupVerifications == nil {
		return bookingmodels.Booking{}, apperrors.Unavailable("Pickup verification is not available right now.", nil)
	}
	// Ownership: only the assigned driver may attempt verification.
	if _, err := s.bookings.GetByIDForProvider(ctx, bookingID, providerID); err != nil {
		return bookingmodels.Booking{}, err
	}

	if s.verifyRateLimiter != nil {
		allowed, err := s.verifyRateLimiter.Allow(ctx, bookingID, s.verification.RateWindow, s.verification.MaxAttempts)
		if err != nil {
			return bookingmodels.Booking{}, apperrors.Unavailable("The pickup code could not be checked. Try again.", err)
		}
		if !allowed {
			return bookingmodels.Booking{}, apperrors.RateLimited("Too many attempts. Wait a moment and try again.", nil)
		}
	}

	rec, err := s.pickupVerifications.Get(ctx, bookingID)
	if err != nil {
		return bookingmodels.Booking{}, err
	}
	if rec.VerifiedAt != nil {
		// Already verified (e.g. a duplicate submit) -- report success rather
		// than a confusing "expired/wrong code" error for a no-op retry.
		return s.bookings.GetByIDForProvider(ctx, bookingID, providerID)
	}
	if rec.IsExpired(time.Now()) {
		return bookingmodels.Booking{}, apperrors.Conflict("This code has expired. Ask the passenger to resend it.", nil)
	}
	if rec.AttemptsExceeded() {
		return bookingmodels.Booking{}, apperrors.Conflict("Too many incorrect attempts. Ask the passenger to resend the code.", nil)
	}

	if !handover.VerifyCode(s.verification.HandoverSecret, pickupChallengeID(bookingID), code, rec.Hash) {
		_ = s.pickupVerifications.IncrementAttempts(ctx, bookingID)
		return bookingmodels.Booking{}, apperrors.BadRequest("That code is incorrect.", nil)
	}

	if err := s.pickupVerifications.MarkVerified(ctx, nil, bookingID, time.Now()); err != nil {
		return bookingmodels.Booking{}, apperrors.Unavailable("Verification could not be recorded. Try again.", err)
	}
	return s.bookings.MarkPickupVerified(ctx, nil, bookingID, providerID)
}
