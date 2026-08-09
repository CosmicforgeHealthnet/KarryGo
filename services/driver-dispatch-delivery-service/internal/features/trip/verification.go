package trip

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/handover"
)

// verifyDeliveryCode validates a rider-submitted delivery code against the
// trip's stored delivery credential, incrementing the attempt counter on any
// failure.
func (s *Service) verifyDeliveryCode(ctx context.Context, trip *Trip, code string) error {
	rec := trip.DeliveryCodeRecord()
	if rec.Hash == "" {
		return apperrors.Conflict("A delivery verification code has not been generated yet.", nil)
	}
	if rec.IsExpired(s.now()) {
		return apperrors.Unauthorized("This code has expired. Ask the customer for a new one.", nil)
	}
	if rec.AttemptsExceeded() {
		return apperrors.Unauthorized("Too many incorrect attempts. Ask the customer to resend the code.", nil)
	}
	challengeID := handover.BuildChallengeID("dispatch", handover.StepDelivery, trip.ID)
	if !handover.VerifyCode(s.handoverSecret, challengeID, strings.TrimSpace(code), rec.Hash) {
		_ = s.repository.IncrementDeliveryCodeAttempt(ctx, trip.ID)
		return apperrors.Unauthorized("Incorrect code. Please try again.", nil)
	}
	return nil
}

// VerifyPickupCodeForCustomer lets the customer confirm the rider's pickup
// credential (looked up by booking id, since the customer-facing surface
// only knows about bookings, not trip ids). On success, StartTrip becomes
// unblocked for the rider.
func (s *Service) VerifyPickupCodeForCustomer(ctx context.Context, bookingID, code string) error {
	trip, err := s.repository.GetTripByBookingID(ctx, bookingID)
	if err != nil {
		return err
	}
	if trip == nil {
		return apperrors.NotFound("Trip not found.", nil)
	}
	if trip.PickupCodeVerifiedAt != nil {
		return nil // already verified — idempotent success
	}

	rec := trip.PickupCodeRecord()
	if rec.Hash == "" {
		return apperrors.Conflict("Your rider's pickup credential has not been generated yet.", nil)
	}
	if rec.IsExpired(s.now()) {
		return apperrors.Unauthorized("This code has expired. Ask your rider to generate a new one.", nil)
	}
	if rec.AttemptsExceeded() {
		return apperrors.Unauthorized("Too many incorrect attempts. Ask your rider to generate a new code.", nil)
	}

	challengeID := handover.BuildChallengeID("dispatch", handover.StepPickup, trip.ID)
	if !handover.VerifyCode(s.handoverSecret, challengeID, strings.TrimSpace(code), rec.Hash) {
		_ = s.repository.IncrementPickupCodeAttempt(ctx, trip.ID)
		return apperrors.Unauthorized("Incorrect code. Please try again.", nil)
	}

	return s.repository.MarkPickupVerified(ctx, trip.ID)
}

// ResendDeliveryCodeForCustomer regenerates the customer's delivery
// credential (looked up by booking id).
func (s *Service) ResendDeliveryCodeForCustomer(ctx context.Context, bookingID string) (handover.CodeRecord, error) {
	trip, err := s.repository.GetTripByBookingID(ctx, bookingID)
	if err != nil {
		return handover.CodeRecord{}, err
	}
	if trip == nil {
		return handover.CodeRecord{}, apperrors.NotFound("Trip not found.", nil)
	}

	now := s.now()
	if ok, wait := trip.DeliveryCodeRecord().CanResend(now); !ok {
		return handover.CodeRecord{}, resendWaitError(wait)
	}

	rec, err := handover.NewCodeRecord(s.handoverSecret, handover.BuildChallengeID("dispatch", handover.StepDelivery, trip.ID), now)
	if err != nil {
		return handover.CodeRecord{}, apperrors.Internal("Could not generate a delivery code.", err)
	}
	if err := s.repository.ResendDeliveryCode(ctx, trip.ID, rec); err != nil {
		return handover.CodeRecord{}, err
	}
	return rec, nil
}

// GetPickupCredential returns the rider's own pickup credential for
// redisplay (e.g. after navigating away from the arrived-at-pickup screen).
func (s *Service) GetPickupCredential(ctx context.Context, providerID, tripID string) (PickupCredentialView, error) {
	trip, err := s.repository.GetProviderTripByID(ctx, tripID, providerID)
	if err != nil {
		return PickupCredentialView{}, err
	}
	if trip == nil {
		return PickupCredentialView{}, apperrors.NotFound("Trip not found.", nil)
	}
	if trip.PickupCodeHash == "" {
		return PickupCredentialView{}, apperrors.NotFound("No pickup credential has been generated yet.", nil)
	}
	return PickupCredentialViewFrom(*trip), nil
}

// ResendPickupCredential regenerates the rider's pickup credential (e.g.
// after it expired before the customer could verify it).
func (s *Service) ResendPickupCredential(ctx context.Context, providerID, tripID string) (PickupCredentialView, error) {
	trip, err := s.repository.GetProviderTripByID(ctx, tripID, providerID)
	if err != nil {
		return PickupCredentialView{}, err
	}
	if trip == nil {
		return PickupCredentialView{}, apperrors.NotFound("Trip not found.", nil)
	}

	now := s.now()
	if ok, wait := trip.PickupCodeRecord().CanResend(now); !ok {
		return PickupCredentialView{}, resendWaitError(wait)
	}

	rec, err := handover.NewCodeRecord(s.handoverSecret, handover.BuildChallengeID("dispatch", handover.StepPickup, tripID), now)
	if err != nil {
		return PickupCredentialView{}, apperrors.Internal("Could not generate a pickup code.", err)
	}
	if err := s.repository.ResendPickupCode(ctx, tripID, rec); err != nil {
		return PickupCredentialView{}, err
	}
	expires := rec.ExpiresAt
	return PickupCredentialView{Code: rec.Code, ExpiresAt: &expires, Verified: false}, nil
}

func resendWaitError(wait time.Duration) error {
	return apperrors.RateLimited(fmt.Sprintf("Please wait %d seconds before requesting a new code.", int(wait.Round(time.Second).Seconds())), nil)
}
