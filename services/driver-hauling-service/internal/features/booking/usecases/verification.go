package bookingusecases

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	bookingmodels "cosmicforge/logistics/services/hauling-service/internal/features/booking/models"
	bookingrepo "cosmicforge/logistics/services/hauling-service/internal/features/booking/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/custodychain"
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
	Code         string     `json:"code,omitempty"`
	QRToken      string     `json:"qr_token,omitempty"`
	ChallengeRef string     `json:"challenge_ref,omitempty"`
	Version      int64      `json:"version,omitempty"`
	Status       string     `json:"status,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	Verified     bool       `json:"verified"`

	// Direction names the credential's purpose, holder and verifier. The
	// driver always holds a Hauling pickup credential; who verifies it
	// depends on whether this is a Commerce mission or a direct booking.
	Direction custodychain.Direction `json:"direction,omitempty"`
}

// VerificationCodeView is the customer-facing view of the delivery
// credential — never exposed to the driver directly (the driver only
// submits an attempt via ConfirmDelivery).
type VerificationCodeView struct {
	QRToken           string     `json:"qr_token,omitempty"`
	ChallengeRef      string     `json:"challenge_ref,omitempty"`
	Version           int64      `json:"version,omitempty"`
	Status            string     `json:"status,omitempty"`
	Code              string     `json:"code,omitempty"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
	AttemptsRemaining int        `json:"attempts_remaining"`

	// Direction names the credential's purpose, holder and verifier. A
	// delivery credential has one direction on every flow: the receiver holds
	// it and the driver verifies it.
	Direction custodychain.Direction `json:"direction,omitempty"`
}

// CommerceHandoverObservation is intentionally credential-free. The version
// combines immutable challenge generation with terminal status so resend and
// consumption are both monotonic to downstream projections.
type CommerceHandoverObservation struct {
	MissionRef    string `json:"mission_ref"`
	HandoverRef   string `json:"handover_ref"`
	Stage         string `json:"stage"`
	Status        string `json:"status"`
	SourceVersion int64  `json:"source_version"`
}

type commerceHandoverBindingReader interface {
	CommerceHandoverBinding(context.Context, string) (bookingrepo.CommerceHandoverBinding, error)
}

func commerceObservation(binding bookingrepo.CommerceHandoverBinding, rec handover.CodeRecord, status string) CommerceHandoverObservation {
	projected := strings.ToUpper(strings.TrimSpace(status))
	rank := int64(1)
	switch projected {
	case "CONSUMED":
		rank = 9
	case "SUPERSEDED", "INVALIDATED":
		projected, rank = "INVALIDATED", 8
	case "EXPIRED", "LOCKED":
		rank = 8
	}
	return CommerceHandoverObservation{MissionRef: binding.MissionRef, HandoverRef: rec.ChallengeRef, Stage: strings.ToUpper(string(rec.Stage)), Status: projected, SourceVersion: rec.Version*10 + rank}
}

// ReadCommerceHandover is the canonical status/read-back path used after an
// unknown network outcome. It never returns Code, QRToken, hashes or attempts.
func (s *BookingService) ReadCommerceHandover(ctx context.Context, missionRef, stage string) (CommerceHandoverObservation, error) {
	reader, ok := s.bookings.(commerceHandoverBindingReader)
	if !ok || s.secureHandover == nil {
		return CommerceHandoverObservation{}, apperrors.Unavailable("Secure handover is unavailable.", nil)
	}
	step := handover.Step(strings.ToLower(strings.TrimSpace(stage)))
	if step != handover.StepPickup && step != handover.StepDelivery {
		return CommerceHandoverObservation{}, apperrors.BadRequest("Invalid handover stage.", nil)
	}
	binding, err := reader.CommerceHandoverBinding(ctx, missionRef)
	if err != nil {
		return CommerceHandoverObservation{}, apperrors.NotFound("Commerce handover was not found.", err)
	}
	rec, status, err := s.secureHandover.GetHandoverChallenge(ctx, binding.BookingID, step)
	if err != nil {
		return CommerceHandoverObservation{}, apperrors.NotFound("Commerce handover was not found.", err)
	}
	return commerceObservation(binding, rec, status), nil
}

// ConfirmCommercePickup relays a transient credential into the existing
// serialized booking authority. A duplicate/unknown request reconciles from
// canonical state and never performs a second custody transition.
func (s *BookingService) ConfirmCommercePickup(ctx context.Context, missionRef, challengeRef, method, credential, operationRef string) (CommerceHandoverObservation, error) {
	if strings.TrimSpace(operationRef) == "" || strings.TrimSpace(challengeRef) == "" || strings.TrimSpace(credential) == "" || (method != "OTP" && method != "QR") {
		return CommerceHandoverObservation{}, apperrors.BadRequest("Handover confirmation is invalid.", nil)
	}
	reader, ok := s.bookings.(commerceHandoverBindingReader)
	if !ok || s.secureHandover == nil {
		return CommerceHandoverObservation{}, apperrors.Unavailable("Secure handover is unavailable.", nil)
	}
	binding, err := reader.CommerceHandoverBinding(ctx, missionRef)
	if err != nil {
		return CommerceHandoverObservation{}, apperrors.NotFound("Commerce handover was not found.", err)
	}
	current, status, err := s.secureHandover.GetHandoverChallenge(ctx, binding.BookingID, handover.StepPickup)
	if err != nil || current.ChallengeRef != strings.TrimSpace(challengeRef) {
		return CommerceHandoverObservation{}, apperrors.NotFound("Commerce handover was not found.", err)
	}
	if status == "CONSUMED" {
		return commerceObservation(binding, current, status), nil
	}
	_, _, consumeErr := s.secureHandover.ConsumeHandoverChallenge(ctx, binding.BookingID, handover.StepPickup, binding.CustomerID, strings.TrimSpace(credential), s.handoverSecret, "", "")
	latest, latestStatus, readErr := s.secureHandover.GetHandoverChallenge(ctx, binding.BookingID, handover.StepPickup)
	if readErr == nil && latest.ChallengeRef == current.ChallengeRef && latestStatus == "CONSUMED" {
		return commerceObservation(binding, latest, latestStatus), nil
	}
	if consumeErr != nil {
		return CommerceHandoverObservation{}, consumeErr
	}
	return CommerceHandoverObservation{}, readErr
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
		QRToken:           rec.QRToken,
		ChallengeRef:      rec.ChallengeRef,
		Version:           rec.Version,
		AttemptsRemaining: remaining,
	}
	if !rec.ExpiresAt.IsZero() {
		expires := rec.ExpiresAt
		view.ExpiresAt = &expires
	}
	return view
}

func securePickupCredentialView(rec handover.CodeRecord, status string) PickupCredentialView {
	expires := rec.ExpiresAt
	return PickupCredentialView{
		Code: rec.Code, QRToken: rec.QRToken, ChallengeRef: rec.ChallengeRef,
		Version: rec.Version, Status: status, ExpiresAt: &expires, Verified: status == "CONSUMED",
	}
}

func secureVerificationCodeView(rec handover.CodeRecord, status string) VerificationCodeView {
	view := verificationCodeViewFrom(rec)
	view.Status = status
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

	if s.secureHandover != nil {
		updated, consumed, consumeErr := s.secureHandover.ConsumeHandoverChallenge(ctx, id, handover.StepPickup, customerID, strings.TrimSpace(code), s.handoverSecret, "", "")
		if consumeErr != nil && !errors.Is(consumeErr, bookingrepo.ErrSecureHandoverChallengeNotFound) {
			return bookingmodels.PublicBooking{}, consumeErr
		}
		if consumeErr == nil && consumed {
			_ = s.addEvent(ctx, id, "pickup_verified_by_customer", bookingmodels.ActorCustomer, customerID)
			if b.ProviderID != nil {
				s.notifier.NotifyProviderPickupVerified(ctx, *b.ProviderID, id, map[string]interface{}{"booking_id": id})
			}
		}
		if consumeErr == nil {
			return updated.Public(), nil
		}
	}
	if b.PickupCodeVerifiedAt != nil {
		// Legacy version-zero credentials retain their former idempotency during
		// the bounded drain window. Versioned credentials reject replay above.
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

// GetPickupCredential returns status only. Plain credentials are available
// once at issue/resend and are never persisted for redisplay.
func (s *BookingService) GetPickupCredential(ctx context.Context, providerID, id string) (PickupCredentialView, error) {
	b, err := s.bookings.GetByID(ctx, id)
	if err != nil {
		return PickupCredentialView{}, err
	}
	if b.ProviderID == nil || *b.ProviderID != providerID {
		return PickupCredentialView{}, apperrors.NotFound("Booking could not be found.", nil)
	}
	if s.secureHandover != nil {
		rec, status, challengeErr := s.secureHandover.GetHandoverChallenge(ctx, id, handover.StepPickup)
		if challengeErr != nil && !errors.Is(challengeErr, bookingrepo.ErrSecureHandoverChallengeNotFound) {
			return PickupCredentialView{}, challengeErr
		}
		if challengeErr == nil {
			view := securePickupCredentialView(rec, status)
			view.Code, view.QRToken = "", ""
			return s.withPickupDirection(ctx, id, view), nil
		}
	}
	if b.PickupCodeHash == "" {
		return PickupCredentialView{}, apperrors.NotFound("No pickup credential has been generated yet.", nil)
	}
	view := pickupCredentialViewFrom(b)
	view.Code = ""
	return s.withPickupDirection(ctx, id, view), nil
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
	if s.secureHandover != nil {
		current, status, challengeErr := s.secureHandover.GetHandoverChallenge(ctx, id, handover.StepPickup)
		if challengeErr != nil && !errors.Is(challengeErr, bookingrepo.ErrSecureHandoverChallengeNotFound) {
			return PickupCredentialView{}, challengeErr
		}
		if challengeErr == nil && status == "CONSUMED" {
			return PickupCredentialView{}, apperrors.Conflict("This handover is already verified.", nil)
		}
		if challengeErr != nil {
			current = b.PickupCodeRecord()
		}
		if ok, wait := current.CanResend(now); !ok {
			return PickupCredentialView{}, resendWaitError(wait)
		}
		nextVersion := current.Version + 1
		if nextVersion < 1 {
			nextVersion = 1
		}
		rec, issueErr := handover.NewSecureCodeRecord(s.handoverSecret, "hauling", handover.StepPickup, uuid.NewString(), nextVersion, id, providerID, b.CustomerID, current.ResendCount+1, now)
		if issueErr != nil {
			return PickupCredentialView{}, apperrors.Internal("Could not generate a pickup credential.", issueErr)
		}
		if issueErr = s.bookings.ResendPickupCode(ctx, id, rec); issueErr != nil {
			return PickupCredentialView{}, issueErr
		}
		return s.withPickupDirection(ctx, id, securePickupCredentialView(rec, "ACTIVE")), nil
	}
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
	return s.withPickupDirection(ctx, id,
		PickupCredentialView{Code: rec.Code, ExpiresAt: &expires, Verified: false}), nil
}

// GetVerificationCode returns delivery challenge status only. It cannot
// redisplay an OTP or QR bearer token.
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
	if s.secureHandover != nil {
		rec, status, challengeErr := s.secureHandover.GetHandoverChallenge(ctx, id, handover.StepDelivery)
		if challengeErr != nil && !errors.Is(challengeErr, bookingrepo.ErrSecureHandoverChallengeNotFound) {
			return VerificationCodeView{}, challengeErr
		}
		if challengeErr == nil {
			view := secureVerificationCodeView(rec, status)
			view.Code, view.QRToken = "", ""
			return withDeliveryDirection(id, view), nil
		}
	}
	rec := b.DeliveryCodeRecord()
	if rec.Hash == "" {
		return VerificationCodeView{}, apperrors.NotFound("No delivery code has been generated yet.", nil)
	}
	view := verificationCodeViewFrom(rec)
	view.Code, view.QRToken = "", ""
	return withDeliveryDirection(id, view), nil
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
	if s.secureHandover != nil {
		current, status, challengeErr := s.secureHandover.GetHandoverChallenge(ctx, id, handover.StepDelivery)
		if challengeErr != nil && !errors.Is(challengeErr, bookingrepo.ErrSecureHandoverChallengeNotFound) {
			return VerificationCodeView{}, challengeErr
		}
		if challengeErr == nil && status == "CONSUMED" {
			return VerificationCodeView{}, apperrors.Conflict("This handover is already verified.", nil)
		}
		if challengeErr != nil {
			current = b.DeliveryCodeRecord()
		}
		if ok, wait := current.CanResend(now); !ok {
			return VerificationCodeView{}, resendWaitError(wait)
		}
		if b.ProviderID == nil {
			return VerificationCodeView{}, apperrors.Conflict("No provider is assigned to this booking.", nil)
		}
		nextVersion := current.Version + 1
		if nextVersion < 1 {
			nextVersion = 1
		}
		rec, issueErr := handover.NewSecureCodeRecord(s.handoverSecret, "hauling", handover.StepDelivery, uuid.NewString(), nextVersion, id, *b.ProviderID, customerID, current.ResendCount+1, now)
		if issueErr != nil {
			return VerificationCodeView{}, apperrors.Internal("Could not generate a delivery credential.", issueErr)
		}
		if issueErr = s.bookings.ResendDeliveryCode(ctx, id, rec); issueErr != nil {
			return VerificationCodeView{}, issueErr
		}
		s.notifier.NotifyCustomerDeliveryCodeReady(ctx, customerID, id, map[string]interface{}{"booking_id": id, "code": rec.Code, "qr_token": rec.QRToken, "challenge_ref": rec.ChallengeRef, "version": rec.Version})
		return withDeliveryDirection(id, secureVerificationCodeView(rec, "ACTIVE")), nil
	}
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
	return withDeliveryDirection(id, verificationCodeViewFrom(rec)), nil
}
