package vendorauthusecases

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"

	vendorauthmodels "cosmicforge/logistics/services/vendor-service/internal/features/auth/models"
)

// StepUpStartInput initiates step-up authentication. Can only be called from
// an already-authenticated session (Section 5 rule 14) — SessionID/StaffID
// come from the caller's own verified access token context, never from
// request body input.
type StepUpStartInput struct {
	SessionID string
	StaffID   string
	VendorID  string
	Purpose   string
}

func (s *AuthService) StepUpStart(ctx context.Context, in StepUpStartInput) (StartResult, error) {
	if strings.TrimSpace(in.Purpose) == "" {
		return StartResult{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "purpose", Message: "A step-up purpose is required."},
		})
	}

	affiliation, err := s.staff.GetByID(ctx, in.StaffID)
	if err != nil || !affiliation.Active || affiliation.VendorID != in.VendorID {
		return StartResult{}, apperrors.Forbidden("Step-up authentication is not available for this session.", nil)
	}
	user, err := s.users.GetByID(ctx, affiliation.UserID)
	if err != nil {
		return StartResult{}, err
	}
	identifierType, identifierValue, _, err := user.ContactIdentifier()
	if err != nil {
		return StartResult{}, err
	}

	otp, err := sharedauth.GenerateNumericOTP(sharedauth.DefaultOTPLength)
	if err != nil {
		return StartResult{}, apperrors.Internal("Verification code could not be generated.", err)
	}

	challengeID := "vsu_" + uuid.NewString()
	sessionID, staffID, vendorID, purpose := in.SessionID, in.StaffID, in.VendorID, in.Purpose
	challenge := vendorauthmodels.OTPChallenge{
		ID: challengeID, Purpose: vendorauthmodels.PurposeStepUp,
		IdentifierType: identifierType, IdentifierValue: identifierValue,
		SessionID: &sessionID, StaffID: &staffID, VendorID: &vendorID, StepUpPurpose: &purpose,
		Generation: 1, ExpiresAt: s.now().Add(s.otpTTL),
	}
	challenge.OTPHash = challenge.HashFor(s.otpSecret, otp, 1)

	// Unlike login, step-up challenges are always freshly created (never
	// reused/resent-in-place at Start time) — GetActiveByIdentifier is keyed
	// on (identifier, purpose) only, and a staff member could step up for two
	// different reasons back to back; each gets its own row. A dedicated
	// StepUpResend below still applies the same generation-bump semantics as
	// login resend.
	if err := s.challenges.Create(ctx, challenge); err != nil {
		return StartResult{}, err
	}
	result := StartResult{ChallengeID: challengeID, ExpiresIn: int64(s.otpTTL.Seconds())}
	if s.otpDebug {
		result.DebugOTP = otp
	}
	return result, s.deliverOTP(ctx, identifierType, identifierValue, vendorauthmodels.PurposeStepUp, otp)
}

type StepUpVerifyInput struct {
	SessionID   string
	StaffID     string
	VendorID    string
	Purpose     string
	ChallengeID string
	OTP         string
}

type StepUpVerifyResult struct {
	GrantID   string
	ExpiresIn int64
}

func (s *AuthService) StepUpVerify(ctx context.Context, in StepUpVerifyInput) (StepUpVerifyResult, error) {
	if in.ChallengeID == "" || strings.TrimSpace(in.OTP) == "" {
		return StepUpVerifyResult{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "otp", Message: "Verification code is required."},
		})
	}
	challenge, err := s.challenges.GetByID(ctx, in.ChallengeID)
	if err != nil {
		return StepUpVerifyResult{}, apperrors.Unauthorized("Invalid or expired verification code.", nil)
	}
	// Every binding dimension must match exactly — a step-up challenge issued
	// for one session/staff/vendor/purpose can never verify for another, even
	// if the OTP code itself happens to collide (astronomically unlikely, but
	// the binding check makes it structurally impossible regardless).
	if challenge.Purpose != vendorauthmodels.PurposeStepUp ||
		challenge.SessionID == nil || *challenge.SessionID != in.SessionID ||
		challenge.StaffID == nil || *challenge.StaffID != in.StaffID ||
		challenge.VendorID == nil || *challenge.VendorID != in.VendorID ||
		challenge.StepUpPurpose == nil || *challenge.StepUpPurpose != in.Purpose {
		return StepUpVerifyResult{}, apperrors.Unauthorized("Invalid or expired verification code.", nil)
	}

	if err := challenge.Verify(s.otpSecret, in.ChallengeID, in.OTP, s.otpMaxAttempts, s.now()); err != nil {
		_ = s.challenges.RecordFailedAttempt(ctx, challenge.ID, s.otpMaxAttempts)
		return StepUpVerifyResult{}, err
	}
	consumed, err := s.challenges.Consume(ctx, challenge.ID)
	if err != nil {
		return StepUpVerifyResult{}, err
	}
	if !consumed {
		return StepUpVerifyResult{}, apperrors.Unauthorized("Invalid or expired verification code.", nil)
	}

	grant := vendorauthmodels.StepUpGrant{
		ID: "vsug_" + uuid.NewString(), SessionID: in.SessionID, StaffID: in.StaffID,
		VendorID: in.VendorID, Purpose: in.Purpose, ExpiresAt: s.now().Add(s.stepUpTTL),
	}
	if err := s.stepUps.Create(ctx, grant); err != nil {
		return StepUpVerifyResult{}, err
	}
	return StepUpVerifyResult{GrantID: grant.ID, ExpiresIn: int64(s.stepUpTTL.Seconds())}, nil
}

// ConsumeGrant implements staffusecases.StepUpConsumer — it lets the staff
// feature spend a step-up grant for a high-risk IAM change without importing
// internal/features/auth/repositories directly (the cross-feature import
// guardrail forbids that); the composition root wires *AuthService itself as
// the adapter, since this method already has the exact right shape.
func (s *AuthService) ConsumeGrant(ctx context.Context, grantID, sessionID, staffID, vendorID, purpose string) error {
	grant, err := s.stepUps.GetByID(ctx, grantID)
	if err != nil {
		return apperrors.Forbidden("Step-up authentication is required for this change.", err)
	}
	if !grant.Usable(sessionID, staffID, vendorID, purpose, s.now()) {
		return apperrors.Forbidden("Step-up authentication is required for this change.", nil)
	}
	consumed, err := s.stepUps.Consume(ctx, grantID)
	if err != nil {
		return err
	}
	if !consumed {
		return apperrors.Forbidden("Step-up authentication is required for this change.", nil)
	}
	return nil
}
