package partneraccount

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"cosmicforge/logistics/services/partner-web-service/internal/features/partner/delivery"
)

const signupSuccessMessage = "A verification code has been sent to your email."
const loginSuccessMessage = "A verification code has been sent to your email."

// PartnerEmailSender sends a partner signup OTP. Implementations must not log
// the plaintext OTP in production logs or expose it through API responses.
type PartnerEmailSender interface {
	SendPartnerOTP(ctx context.Context, toEmail string, otpCode string) error
}

func (s *Service) StartSignup(ctx context.Context, in SignupEmailInput) error {
	email, err := s.normalizeSignupEmail(in.BusinessEmail)
	if err != nil {
		return err
	}
	if err := s.ensureEmailAvailable(ctx, email); err != nil {
		return err
	}
	return s.createAndSendOTP(ctx, email)
}

func (s *Service) ResendSignup(ctx context.Context, in SignupEmailInput) error {
	email, err := s.normalizeSignupEmail(in.BusinessEmail)
	if err != nil {
		return err
	}
	if err := s.ensureEmailAvailable(ctx, email); err != nil {
		return err
	}
	if s.otps == nil {
		return delivery.ErrInternal()
	}

	latest, err := s.otps.FindLatestOTPChallenge(ctx, email)
	if err != nil {
		return delivery.ErrInternal()
	}
	if latest != nil {
		remaining := s.otpResendCooldown - s.now().Sub(latest.CreatedAt)
		if remaining > 0 {
			seconds := int((remaining + time.Second - 1) / time.Second)
			return apiErrWithMeta(http.StatusTooManyRequests, "resend_too_soon",
				"Please wait before requesting another code.", map[string]any{"seconds_remaining": seconds})
		}
	}
	return s.createAndSendOTP(ctx, email)
}

func (s *Service) VerifySignup(ctx context.Context, in SignupVerifyInput, meta RequestMeta) (*SessionResult, error) {
	email, err := s.normalizeSignupEmail(in.BusinessEmail)
	if err != nil {
		return nil, err
	}
	code := stringsTrimSpace(in.OTPCode)
	if !validOTPCode(code) {
		return nil, validationErr("otp_code", "OTP code must be 6 digits.")
	}
	if s.otps == nil {
		return nil, delivery.ErrInternal()
	}

	challenge, err := s.otps.FindLatestOTPChallenge(ctx, email)
	if err != nil {
		return nil, delivery.ErrInternal()
	}
	if challenge == nil || challenge.ConsumedAt != nil || !challenge.ExpiresAt.After(s.now()) {
		return nil, apiErr(http.StatusBadRequest, "otp_expired", "This code has expired. Request a new one.")
	}
	if challenge.Attempts >= s.otpMaxAttempts {
		return nil, apiErr(http.StatusTooManyRequests, "otp_max_attempts", "Too many wrong attempts. Request a new code.")
	}

	submittedHash := hashOTPCode(code)
	if subtle.ConstantTimeCompare([]byte(submittedHash), []byte(challenge.OTPCodeHash)) != 1 {
		attempts, err := s.otps.IncrementOTPAttempts(ctx, challenge.ID)
		if err != nil {
			return nil, delivery.ErrInternal()
		}
		remaining := s.otpMaxAttempts - attempts
		if remaining < 0 {
			remaining = 0
		}
		return nil, apiErrWithMeta(http.StatusBadRequest, "otp_invalid",
			fmt.Sprintf("Incorrect code. %d attempts remaining.", remaining), map[string]any{"attempts_remaining": remaining})
	}

	if err := s.otps.ConsumeOTPChallenge(ctx, challenge.ID); err != nil {
		return nil, delivery.ErrInternal()
	}

	account, _, err := s.accounts.FindByEmail(ctx, email)
	if err != nil {
		return nil, delivery.ErrInternal()
	}
	if account == nil {
		created, err := s.accounts.CreateAccount(ctx, CreateAccountParams{
			BusinessEmail: email,
			// Product decision: OTP-created web app accounts become active as soon
			// as email ownership is verified. Profile completion is an onboarding
			// step, so sandbox dashboard access is allowed while fields are null.
			Status: StatusActive,
			Plan:   PlanSandbox,
		})
		if err == ErrEmailTaken {
			account, _, err = s.accounts.FindByEmail(ctx, email)
			if err != nil {
				return nil, delivery.ErrInternal()
			}
		} else if err != nil {
			return nil, delivery.ErrInternal()
		} else {
			account = &created
		}
	}
	if account == nil || account.Status != StatusActive {
		return nil, apiErr(http.StatusConflict, "email_taken", "An account with this email already exists.")
	}

	credentials, err := s.issueSession(ctx, account.ID, meta)
	if err != nil {
		return nil, delivery.ErrInternal()
	}
	credentials.Account = *account
	return credentials, nil
}

// StartLogin sends a login OTP to an existing, active account's email.
func (s *Service) StartLogin(ctx context.Context, in SignupEmailInput) error {
	email, err := s.normalizeSignupEmail(in.BusinessEmail)
	if err != nil {
		return err
	}
	if err := s.ensureEmailExists(ctx, email); err != nil {
		return err
	}
	return s.createAndSendOTP(ctx, email)
}

// VerifyLogin checks a login OTP against an existing account and issues a
// session. Unlike VerifySignup, it never creates an account.
func (s *Service) VerifyLogin(ctx context.Context, in SignupVerifyInput, meta RequestMeta) (*SessionResult, error) {
	email, err := s.normalizeSignupEmail(in.BusinessEmail)
	if err != nil {
		return nil, err
	}
	code := stringsTrimSpace(in.OTPCode)
	if !validOTPCode(code) {
		return nil, validationErr("otp_code", "OTP code must be 6 digits.")
	}
	if s.otps == nil {
		return nil, delivery.ErrInternal()
	}

	challenge, err := s.otps.FindLatestOTPChallenge(ctx, email)
	if err != nil {
		return nil, delivery.ErrInternal()
	}
	if challenge == nil || challenge.ConsumedAt != nil || !challenge.ExpiresAt.After(s.now()) {
		return nil, apiErr(http.StatusBadRequest, "otp_expired", "This code has expired. Request a new one.")
	}
	if challenge.Attempts >= s.otpMaxAttempts {
		return nil, apiErr(http.StatusTooManyRequests, "otp_max_attempts", "Too many wrong attempts. Request a new code.")
	}

	submittedHash := hashOTPCode(code)
	if subtle.ConstantTimeCompare([]byte(submittedHash), []byte(challenge.OTPCodeHash)) != 1 {
		attempts, err := s.otps.IncrementOTPAttempts(ctx, challenge.ID)
		if err != nil {
			return nil, delivery.ErrInternal()
		}
		remaining := s.otpMaxAttempts - attempts
		if remaining < 0 {
			remaining = 0
		}
		return nil, apiErrWithMeta(http.StatusBadRequest, "otp_invalid",
			fmt.Sprintf("Incorrect code. %d attempts remaining.", remaining), map[string]any{"attempts_remaining": remaining})
	}

	if err := s.otps.ConsumeOTPChallenge(ctx, challenge.ID); err != nil {
		return nil, delivery.ErrInternal()
	}

	account, _, err := s.accounts.FindByEmail(ctx, email)
	if err != nil {
		return nil, delivery.ErrInternal()
	}
	if account == nil || account.Status != StatusActive {
		return nil, apiErr(http.StatusUnauthorized, "invalid_credentials", "No account found with this email.")
	}

	credentials, err := s.issueSession(ctx, account.ID, meta)
	if err != nil {
		return nil, delivery.ErrInternal()
	}
	credentials.Account = *account
	return credentials, nil
}

func (s *Service) normalizeSignupEmail(raw string) (string, error) {
	email := normalizeEmail(raw)
	if !validEmail(email) {
		return "", validationErr("business_email", "A valid business email is required.")
	}
	return email, nil
}

func (s *Service) ensureEmailAvailable(ctx context.Context, email string) error {
	account, _, err := s.accounts.FindByEmail(ctx, email)
	if err != nil {
		return delivery.ErrInternal()
	}
	if account != nil {
		return apiErr(http.StatusConflict, "email_taken", "An account with this email already exists.")
	}
	return nil
}

func (s *Service) ensureEmailExists(ctx context.Context, email string) error {
	account, _, err := s.accounts.FindByEmail(ctx, email)
	if err != nil {
		return delivery.ErrInternal()
	}
	if account == nil {
		return apiErr(http.StatusUnauthorized, "invalid_credentials", "No account found with this email.")
	}
	if account.Status != StatusActive {
		return apiErr(http.StatusForbidden, "account_inactive", "This account is not active.")
	}
	return nil
}

func (s *Service) createAndSendOTP(ctx context.Context, email string) error {
	if s.otps == nil {
		return delivery.ErrInternal()
	}
	if s.emailSender == nil {
		return apiErr(http.StatusInternalServerError, "email_not_configured", "Email delivery is not configured.")
	}
	code, err := s.otpGenerator()
	if err != nil {
		return delivery.ErrInternal()
	}
	challenge, err := s.otps.CreateOTPChallenge(ctx, CreateOTPChallengeParams{
		BusinessEmail: email,
		OTPCodeHash:   hashOTPCode(code),
		ExpiresAt:     s.now().Add(s.otpTTL),
	})
	if err != nil {
		return delivery.ErrInternal()
	}
	if err := s.emailSender.SendPartnerOTP(ctx, email, code); err != nil {
		_ = s.otps.DeleteOTPChallenge(ctx, challenge.ID)
		if errors.Is(err, ErrPartnerEmailNotConfigured) {
			return apiErr(http.StatusInternalServerError, "email_not_configured", "Email delivery is not configured.")
		}
		return apiErr(http.StatusBadGateway, "email_delivery_failed", "Could not send verification code. Please try again.")
	}
	return nil
}

func stringsTrimSpace(value string) string { return strings.TrimSpace(value) }
