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

// challengeID namespaces one booking's code for ONE handover step among every
// other handover code in the platform (hauling/dispatch each pass their own
// service name in this same call, per handover.BuildChallengeID's own doc
// comment). Because the step is part of the challenge, a pickup code can
// never verify against a delivery attempt or vice versa -- that isolation is
// the whole reason BuildChallengeID takes a step at all.
func challengeID(step handover.Step, bookingID string) string {
	return handover.BuildChallengeID("taxi", step, bookingID)
}

func pickupChallengeID(bookingID string) string {
	return challengeID(handover.StepPickup, bookingID)
}

// unavailableFor keeps the "not configured" wording truthful per step.
func unavailableFor(step handover.Step) *apperrors.Error {
	if step == handover.StepDelivery {
		return apperrors.Unavailable("Delivery verification is not available right now.", nil)
	}
	return apperrors.Unavailable("Pickup verification is not available right now.", nil)
}

func codeGenFailed(step handover.Step, err error) *apperrors.Error {
	if step == handover.StepDelivery {
		return apperrors.Internal("A delivery code could not be generated.", err)
	}
	return apperrors.Internal("A pickup code could not be generated.", err)
}

// generateHandoverCode writes a fresh code for one step. A nil
// PickupVerifications repository is a deliberate no-op, not an error: several
// existing tests build a BookingService without wiring verification at all
// (see Options' own doc comment), and the transition that generates the code
// must still succeed for them. Any REAL deployment wires this, so a booking
// left without a generated code simply can never pass the gate that depends
// on it -- a safe, honest degrade, not a silent bypass.
func (s *BookingService) generateHandoverCode(ctx context.Context, exec Executor, bookingID string, step handover.Step) (handover.CodeRecord, error) {
	if s.pickupVerifications == nil {
		return handover.CodeRecord{}, nil
	}
	rec, err := handover.NewCodeRecord(s.verification.HandoverSecret, challengeID(step, bookingID), time.Now())
	if err != nil {
		return handover.CodeRecord{}, codeGenFailed(step, err)
	}
	if err := s.pickupVerifications.Upsert(ctx, exec, bookingverification.Record{
		BookingID: bookingID, Step: step, CodeRecord: rec,
	}); err != nil {
		return handover.CodeRecord{}, err
	}
	return rec, nil
}

// generatePickupCode is called from MarkArrivedAtPickup, inside its own
// transaction.
func (s *BookingService) generatePickupCode(ctx context.Context, exec Executor, bookingID string) error {
	_, err := s.generateHandoverCode(ctx, exec, bookingID, handover.StepPickup)
	return err
}

// HandoverCredential is the customer-facing view of a code: a bare digit
// string, rendered as text and (client-side) as a QR code from that same
// string -- see internal/features/booking/verification's own package doc
// comment for why this build has one secret, not a separate QR payload.
type HandoverCredential struct {
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expires_at"`
}

// PickupCredential is the original name for the pickup step's credential.
// Kept as an alias so the existing pickup response shape is provably
// unchanged by the parcel work.
type PickupCredential = HandoverCredential

// getCode returns the customer's current code for one step.
func (s *BookingService) getCode(ctx context.Context, bookingID, customerID string, step handover.Step) (HandoverCredential, error) {
	if s.pickupVerifications == nil {
		return HandoverCredential{}, unavailableFor(step)
	}
	// Ownership check first: a customer must not be able to probe another
	// customer's booking for whether a code exists.
	if _, err := s.bookings.GetByIDForCustomer(ctx, bookingID, customerID); err != nil {
		return HandoverCredential{}, err
	}
	rec, err := s.pickupVerifications.Get(ctx, bookingID, step)
	if err != nil {
		return HandoverCredential{}, err
	}
	return HandoverCredential{Code: rec.Code, ExpiresAt: rec.ExpiresAt}, nil
}

// resendCode regenerates a step's code (a fresh random value, same challenge
// ID so the old one stops verifying), subject to handover's own CanResend
// cooldown/max-resends decision.
func (s *BookingService) resendCode(ctx context.Context, bookingID, customerID string, step handover.Step) (HandoverCredential, error) {
	if s.pickupVerifications == nil {
		return HandoverCredential{}, unavailableFor(step)
	}
	if _, err := s.bookings.GetByIDForCustomer(ctx, bookingID, customerID); err != nil {
		return HandoverCredential{}, err
	}

	existing, err := s.pickupVerifications.Get(ctx, bookingID, step)
	if err != nil {
		return HandoverCredential{}, err
	}
	now := time.Now()
	if ok, wait := existing.CanResend(now); !ok {
		rateErr := apperrors.RateLimited("Please wait before requesting a new code.", nil)
		rateErr.Details = map[string]interface{}{"retry_after_seconds": int(wait.Seconds())}
		return HandoverCredential{}, rateErr
	}

	fresh, err := handover.NewCodeRecord(s.verification.HandoverSecret, challengeID(step, bookingID), now)
	if err != nil {
		return HandoverCredential{}, codeGenFailed(step, err)
	}
	fresh.ResendCount = existing.ResendCount + 1
	fresh.LastSentAt = &now

	if err := s.pickupVerifications.Upsert(ctx, nil, bookingverification.Record{
		BookingID: bookingID, Step: step, CodeRecord: fresh,
	}); err != nil {
		return HandoverCredential{}, err
	}
	return HandoverCredential{Code: fresh.Code, ExpiresAt: fresh.ExpiresAt}, nil
}

// verifyCode is the driver-initiated check for one step: the customer has
// told them (or they scanned) a code, and this either unlocks the next
// transition or refuses with a clear, specific reason. Rate-limited on top of
// handover's own durable per-code attempt cap -- see VerificationConfig's own
// doc comment for why both exist.
//
// markVerified performs the booking-level transition this code unlocks; it is
// passed in so the pickup and delivery paths share every check above it while
// still writing to their own column.
func (s *BookingService) verifyCode(
	ctx context.Context,
	bookingID, providerID, code string,
	step handover.Step,
	markVerified func(context.Context, string, string) (bookingmodels.Booking, error),
) (bookingmodels.Booking, error) {
	if s.pickupVerifications == nil {
		return bookingmodels.Booking{}, unavailableFor(step)
	}
	// Ownership: only the assigned driver may attempt verification.
	if _, err := s.bookings.GetByIDForProvider(ctx, bookingID, providerID); err != nil {
		return bookingmodels.Booking{}, err
	}

	if s.verifyRateLimiter != nil {
		allowed, err := s.verifyRateLimiter.Allow(ctx, bookingID, step, s.verification.RateWindow, s.verification.MaxAttempts)
		if err != nil {
			return bookingmodels.Booking{}, apperrors.Unavailable("The code could not be checked. Try again.", err)
		}
		if !allowed {
			return bookingmodels.Booking{}, apperrors.RateLimited("Too many attempts. Wait a moment and try again.", nil)
		}
	}

	rec, err := s.pickupVerifications.Get(ctx, bookingID, step)
	if err != nil {
		return bookingmodels.Booking{}, err
	}
	if rec.VerifiedAt != nil {
		// Already verified (e.g. a duplicate submit) -- report success rather
		// than a confusing "expired/wrong code" error for a no-op retry.
		return s.bookings.GetByIDForProvider(ctx, bookingID, providerID)
	}
	if rec.IsExpired(time.Now()) {
		return bookingmodels.Booking{}, apperrors.Conflict("This code has expired. Ask for a new one.", nil)
	}
	if rec.AttemptsExceeded() {
		return bookingmodels.Booking{}, apperrors.Conflict("Too many incorrect attempts. Ask for a new code.", nil)
	}

	if !handover.VerifyCode(s.verification.HandoverSecret, challengeID(step, bookingID), code, rec.Hash) {
		_ = s.pickupVerifications.IncrementAttempts(ctx, bookingID, step)
		return bookingmodels.Booking{}, apperrors.BadRequest("That code is incorrect.", nil)
	}

	if err := s.pickupVerifications.MarkVerified(ctx, nil, bookingID, step, time.Now()); err != nil {
		return bookingmodels.Booking{}, apperrors.Unavailable("Verification could not be recorded. Try again.", err)
	}
	return markVerified(ctx, bookingID, providerID)
}

// ─── Pickup (passenger or parcel sender shows, driver verifies) ─────────────

// GetPickupCode returns the customer's current pickup code. NotFound if the
// driver has not arrived yet (no code generated).
func (s *BookingService) GetPickupCode(ctx context.Context, bookingID, customerID string) (PickupCredential, error) {
	return s.getCode(ctx, strings.TrimSpace(bookingID), strings.TrimSpace(customerID), handover.StepPickup)
}

// ResendPickupCode regenerates the pickup code.
func (s *BookingService) ResendPickupCode(ctx context.Context, bookingID, customerID string) (PickupCredential, error) {
	return s.resendCode(ctx, strings.TrimSpace(bookingID), strings.TrimSpace(customerID), handover.StepPickup)
}

// VerifyPickup unlocks MarkInProgress once the driver submits the code the
// customer showed them.
func (s *BookingService) VerifyPickup(ctx context.Context, bookingID, providerID, code string) (bookingmodels.Booking, error) {
	bookingID, providerID, code = strings.TrimSpace(bookingID), strings.TrimSpace(providerID), strings.TrimSpace(code)
	return s.verifyCode(ctx, bookingID, providerID, code, handover.StepPickup,
		func(ctx context.Context, bookingID, providerID string) (bookingmodels.Booking, error) {
			return s.bookings.MarkPickupVerified(ctx, nil, bookingID, providerID)
		})
}

// ─── Delivery (parcel only: receiver shows, driver verifies) ────────────────

// requireParcelInTransit is the shared guard for every delivery-code path. A
// passenger ride returns NotFound rather than a "not a parcel" error: the
// concept must not be discoverable on a booking it does not apply to.
func (s *BookingService) requireParcelInTransit(booking bookingmodels.Booking) error {
	if !booking.IsParcel() {
		return apperrors.NotFound("No delivery code for this ride.", nil)
	}
	if booking.Status != bookingmodels.StatusInProgress {
		return apperrors.Conflict("A delivery code is only available once the package is on its way.", nil)
	}
	return nil
}

// GetDeliveryCode returns the code the receiver shows the driver at drop-off,
// generating it on FIRST read rather than at trip start.
//
// This is deliberate: handover.DefaultTTL is 15 minutes and shared across
// services, so a code minted when the trip starts would already be long
// expired by the time a cross-town parcel actually arrives, leaving the
// driver blocked at the door and burning through the resend budget before
// anyone understood why. Generating on first read starts that clock when the
// customer actually opens the screen, which is near drop-off. What happens at
// trip start is only the delivery_verification_status flip to 'pending',
// which is what gates completion.
func (s *BookingService) GetDeliveryCode(ctx context.Context, bookingID, customerID string) (HandoverCredential, error) {
	bookingID, customerID = strings.TrimSpace(bookingID), strings.TrimSpace(customerID)
	if s.pickupVerifications == nil {
		return HandoverCredential{}, unavailableFor(handover.StepDelivery)
	}
	booking, err := s.bookings.GetByIDForCustomer(ctx, bookingID, customerID)
	if err != nil {
		return HandoverCredential{}, err
	}
	if err := s.requireParcelInTransit(booking); err != nil {
		return HandoverCredential{}, err
	}

	rec, err := s.pickupVerifications.Get(ctx, bookingID, handover.StepDelivery)
	if err == nil {
		return HandoverCredential{Code: rec.Code, ExpiresAt: rec.ExpiresAt}, nil
	}
	// Only an absent record means "not generated yet" -- any other failure is
	// real and must surface rather than silently minting a second code.
	if apperrors.From(err).Code != apperrors.CodeNotFound {
		return HandoverCredential{}, err
	}

	fresh, genErr := s.generateHandoverCode(ctx, nil, bookingID, handover.StepDelivery)
	if genErr != nil {
		return HandoverCredential{}, genErr
	}
	return HandoverCredential{Code: fresh.Code, ExpiresAt: fresh.ExpiresAt}, nil
}

// ResendDeliveryCode regenerates the drop-off code.
func (s *BookingService) ResendDeliveryCode(ctx context.Context, bookingID, customerID string) (HandoverCredential, error) {
	bookingID, customerID = strings.TrimSpace(bookingID), strings.TrimSpace(customerID)
	if s.pickupVerifications == nil {
		return HandoverCredential{}, unavailableFor(handover.StepDelivery)
	}
	booking, err := s.bookings.GetByIDForCustomer(ctx, bookingID, customerID)
	if err != nil {
		return HandoverCredential{}, err
	}
	if err := s.requireParcelInTransit(booking); err != nil {
		return HandoverCredential{}, err
	}
	return s.resendCode(ctx, bookingID, customerID, handover.StepDelivery)
}

// VerifyDelivery unlocks completion once the driver submits the code the
// receiver showed them at drop-off.
func (s *BookingService) VerifyDelivery(ctx context.Context, bookingID, providerID, code string) (bookingmodels.Booking, error) {
	bookingID, providerID, code = strings.TrimSpace(bookingID), strings.TrimSpace(providerID), strings.TrimSpace(code)
	booking, err := s.bookings.GetByIDForProvider(ctx, bookingID, providerID)
	if err != nil {
		return bookingmodels.Booking{}, err
	}
	if err := s.requireParcelInTransit(booking); err != nil {
		return bookingmodels.Booking{}, err
	}
	return s.verifyCode(ctx, bookingID, providerID, code, handover.StepDelivery,
		func(ctx context.Context, bookingID, providerID string) (bookingmodels.Booking, error) {
			return s.bookings.MarkDeliveryVerified(ctx, nil, bookingID, providerID)
		})
}
