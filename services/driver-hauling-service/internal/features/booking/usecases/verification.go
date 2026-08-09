package bookingusecases

import (
	"context"
	"fmt"
	"strings"
	"time"

	bookingmodels "cosmicforge/logistics/services/hauling-service/internal/features/booking/models"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/handover"
)

// ArrivedAtPickupResult is returned to the driver when they mark arrived at
// pickup: the updated booking plus the freshly generated pickup credential
// that only the driver's own app should display.
type ArrivedAtPickupResult struct {
	Booking          bookingmodels.PublicBooking `json:"booking"`
	PickupCredential PickupCredentialView        `json:"pickup_credential"`
}

// PickupCredentialView is the provider-facing view of the driver's own
// pickup credential — never exposed to the customer.
type PickupCredentialView struct {
	Code      string     `json:"code,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	Verified  bool       `json:"verified"`
}

// VerificationCodeView is the customer-facing view of the delivery
// credential — never exposed to the driver directly (the driver only
// submits an attempt via ConfirmDelivery).
type VerificationCodeView struct {
	Code              string     `json:"code,omitempty"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
	AttemptsRemaining int        `json:"attempts_remaining"`
}

func pickupCredentialViewFrom(b bookingmodels.Booking) PickupCredentialView {
	view := PickupCredentialView{
		Code:     b.PickupCode,
		Verified: b.PickupCodeVerifiedAt != nil,
	}
	if b.PickupCodeExpiresAt != nil {
		view.ExpiresAt = b.PickupCodeExpiresAt
	}
	return view
}

func verificationCodeViewFrom(rec handover.CodeRecord) VerificationCodeView {
	remaining := handover.MaxVerifyAttempts - rec.Attempts
	if remaining < 0 {
		remaining = 0
	}
	view := VerificationCodeView{
		Code:              rec.Code,
		AttemptsRemaining: remaining,
	}
	if !rec.ExpiresAt.IsZero() {
		expires := rec.ExpiresAt
		view.ExpiresAt = &expires
	}
	return view
}

func resendWaitError(wait time.Duration) error {
	return apperrors.RateLimited(fmt.Sprintf("Please wait %d seconds before requesting a new code.", int(wait.Round(time.Second).Seconds())), nil)
}

// verifyDeliveryCode validates a driver-submitted delivery code against the
// booking's stored delivery credential, incrementing the attempt counter on
// any failure. b must already be the freshly-loaded booking for id.
func (s *BookingService) verifyDeliveryCode(ctx context.Context, id string, b bookingmodels.Booking, code string) error {
	rec := b.DeliveryCodeRecord()
	if rec.Hash == "" {
		return apperrors.Conflict("A delivery verification code has not been generated yet.", nil)
	}
	if rec.IsExpired(time.Now()) {
		return apperrors.Unauthorized("This code has expired. Ask the customer for a new one.", nil)
	}
	if rec.AttemptsExceeded() {
		return apperrors.Unauthorized("Too many incorrect attempts. Ask the customer to resend the code.", nil)
	}
	challengeID := handover.BuildChallengeID("hauling", handover.StepDelivery, id)
	if !handover.VerifyCode(s.handoverSecret, challengeID, strings.TrimSpace(code), rec.Hash) {
		_ = s.bookings.IncrementDeliveryCodeAttempt(ctx, id)
		return apperrors.Unauthorized("Incorrect code. Please try again.", nil)
	}
	return nil
}

// VerifyDriverAtPickup lets the customer confirm the driver's identity using
// the pickup credential displayed on the driver's app (entered manually or
// scanned as a QR). On success, ConfirmPickup becomes unblocked for the
// driver.
func (s *BookingService) VerifyDriverAtPickup(ctx context.Context, customerID, id, code string) (bookingmodels.PublicBooking, error) {
	b, err := s.bookings.GetByIDForCustomer(ctx, id, customerID)
	if err != nil {
		return bookingmodels.PublicBooking{}, err
	}

	if b.PickupCodeVerifiedAt != nil {
		// Already verified — idempotent success rather than an error.
		return b.Public(), nil
	}

	rec := b.PickupCodeRecord()
	if rec.Hash == "" {
		return bookingmodels.PublicBooking{}, apperrors.Conflict("Your driver's pickup credential has not been generated yet.", nil)
	}
	if rec.IsExpired(time.Now()) {
		return bookingmodels.PublicBooking{}, apperrors.Unauthorized("This code has expired. Ask your driver to generate a new one.", nil)
	}
	if rec.AttemptsExceeded() {
		return bookingmodels.PublicBooking{}, apperrors.Unauthorized("Too many incorrect attempts. Ask your driver to generate a new code.", nil)
	}

	challengeID := handover.BuildChallengeID("hauling", handover.StepPickup, id)
	if !handover.VerifyCode(s.handoverSecret, challengeID, strings.TrimSpace(code), rec.Hash) {
		_ = s.bookings.IncrementPickupCodeAttempt(ctx, id)
		return bookingmodels.PublicBooking{}, apperrors.Unauthorized("Incorrect code. Please try again.", nil)
	}

	if err := s.bookings.MarkPickupVerified(ctx, id); err != nil {
		return bookingmodels.PublicBooking{}, err
	}
	_ = s.addEvent(ctx, id, "pickup_verified_by_customer", bookingmodels.ActorCustomer, customerID)

	updated, err := s.bookings.GetByIDForCustomer(ctx, id, customerID)
	if err != nil {
		return bookingmodels.PublicBooking{}, err
	}
	if b.ProviderID != nil {
		s.notifier.NotifyProviderPickupVerified(ctx, *b.ProviderID, id, map[string]interface{}{"booking_id": id})
	}
	return updated.Public(), nil
}

// GetPickupCredential returns the driver's own pickup credential for
// redisplay (e.g. after navigating away from the arrived-at-pickup screen).
func (s *BookingService) GetPickupCredential(ctx context.Context, providerID, id string) (PickupCredentialView, error) {
	b, err := s.bookings.GetByID(ctx, id)
	if err != nil {
		return PickupCredentialView{}, err
	}
	if b.ProviderID == nil || *b.ProviderID != providerID {
		return PickupCredentialView{}, apperrors.NotFound("Booking could not be found.", nil)
	}
	if b.PickupCodeHash == "" {
		return PickupCredentialView{}, apperrors.NotFound("No pickup credential has been generated yet.", nil)
	}
	return pickupCredentialViewFrom(b), nil
}

// ResendPickupCredential regenerates the driver's pickup credential (e.g.
// after it expired before the customer could verify it).
func (s *BookingService) ResendPickupCredential(ctx context.Context, providerID, id string) (PickupCredentialView, error) {
	b, err := s.bookings.GetByID(ctx, id)
	if err != nil {
		return PickupCredentialView{}, err
	}
	if b.ProviderID == nil || *b.ProviderID != providerID {
		return PickupCredentialView{}, apperrors.NotFound("Booking could not be found.", nil)
	}

	now := time.Now()
	if ok, wait := b.PickupCodeRecord().CanResend(now); !ok {
		return PickupCredentialView{}, resendWaitError(wait)
	}

	rec, err := handover.NewCodeRecord(s.handoverSecret, handover.BuildChallengeID("hauling", handover.StepPickup, id), now)
	if err != nil {
		return PickupCredentialView{}, apperrors.Internal("Could not generate a pickup code.", err)
	}
	if err := s.bookings.ResendPickupCode(ctx, id, rec); err != nil {
		return PickupCredentialView{}, err
	}
	expires := rec.ExpiresAt
	return PickupCredentialView{Code: rec.Code, ExpiresAt: &expires, Verified: false}, nil
}

// GetVerificationCode returns the customer's own delivery credential for
// redisplay. Only the delivery step is readable by the customer — the
// pickup credential belongs to the driver and is never exposed here.
func (s *BookingService) GetVerificationCode(ctx context.Context, customerID, id, step string) (VerificationCodeView, error) {
	if step == string(handover.StepPickup) {
		return VerificationCodeView{}, apperrors.Forbidden("You cannot view this code.", nil)
	}
	if step != string(handover.StepDelivery) {
		return VerificationCodeView{}, apperrors.BadRequest("Invalid verification step.", nil)
	}

	b, err := s.bookings.GetByIDForCustomer(ctx, id, customerID)
	if err != nil {
		return VerificationCodeView{}, err
	}
	rec := b.DeliveryCodeRecord()
	if rec.Hash == "" {
		return VerificationCodeView{}, apperrors.NotFound("No delivery code has been generated yet.", nil)
	}
	return verificationCodeViewFrom(rec), nil
}

// ResendVerificationCode regenerates the customer's delivery credential.
// Only the delivery step may be resent by the customer.
func (s *BookingService) ResendVerificationCode(ctx context.Context, customerID, id, step string) (VerificationCodeView, error) {
	if step == string(handover.StepPickup) {
		return VerificationCodeView{}, apperrors.Forbidden("You cannot resend this code.", nil)
	}
	if step != string(handover.StepDelivery) {
		return VerificationCodeView{}, apperrors.BadRequest("Invalid verification step.", nil)
	}

	b, err := s.bookings.GetByIDForCustomer(ctx, id, customerID)
	if err != nil {
		return VerificationCodeView{}, err
	}

	now := time.Now()
	if ok, wait := b.DeliveryCodeRecord().CanResend(now); !ok {
		return VerificationCodeView{}, resendWaitError(wait)
	}

	rec, err := handover.NewCodeRecord(s.handoverSecret, handover.BuildChallengeID("hauling", handover.StepDelivery, id), now)
	if err != nil {
		return VerificationCodeView{}, apperrors.Internal("Could not generate a delivery code.", err)
	}
	if err := s.bookings.ResendDeliveryCode(ctx, id, rec); err != nil {
		return VerificationCodeView{}, err
	}
	return verificationCodeViewFrom(rec), nil
}
