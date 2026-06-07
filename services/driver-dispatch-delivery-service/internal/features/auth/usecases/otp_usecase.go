package authusecases

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	authmodels "karrygo/services/driver-dispatch-delivery-service/internal/features/auth/models"
	authrepositories "karrygo/services/driver-dispatch-delivery-service/internal/features/auth/repositories"
	"karrygo/shared/go/apperrors"
)

const (
	OTPLength          = 6
	OTPRateLimitPrefix = "dispatch_rider_auth:otp_rate:"
)

type OTPUsecase struct {
	repository  authrepositories.OTPRepository
	redisClient *redis.Client
	secret      []byte
	ttl         time.Duration
	rateWindow  time.Duration
	maxRequests int
	maxAttempts int
	lockoutTTL  time.Duration
	now         func() time.Time
	checkLimit  func(ctx context.Context, phone string) error
}

type OTPOptions struct {
	Repository  authrepositories.OTPRepository
	Redis       *redis.Client
	Secret      []byte
	TTL         time.Duration
	RateWindow  time.Duration
	MaxRequests int
	MaxAttempts int
	LockoutTTL  time.Duration
}

func NewOTPUsecase(opts OTPOptions) *OTPUsecase {
	u := &OTPUsecase{
		repository:  opts.Repository,
		redisClient: opts.Redis,
		secret:      opts.Secret,
		ttl:         opts.TTL,
		rateWindow:  opts.RateWindow,
		maxRequests: opts.MaxRequests,
		maxAttempts: opts.MaxAttempts,
		lockoutTTL:  opts.LockoutTTL,
		now:         time.Now,
	}
	u.checkLimit = u.EnforceRateLimit
	return u
}

func (u *OTPUsecase) WithClock(now func() time.Time) *OTPUsecase {
	u.now = now
	return u
}

// WithRateLimiter replaces the rate-limiter for testing.
// Consistent with the WithClock pattern already in use.
func (u *OTPUsecase) WithRateLimiter(fn func(ctx context.Context, phone string) error) *OTPUsecase {
	u.checkLimit = fn
	return u
}

func (u *OTPUsecase) GenerateCode() (string, error) {
	code := make([]byte, OTPLength)
	for i := range code {
		value, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", err
		}
		code[i] = byte('0' + value.Int64())
	}
	return string(code), nil
}

func (u *OTPUsecase) HashCode(otpID string, phoneNumber string, code string) string {
	mac := hmac.New(sha256.New, u.secret)
	_, _ = mac.Write([]byte("dispatch_rider_otp"))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(otpID))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(phoneNumber))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(code))
	return hex.EncodeToString(mac.Sum(nil))
}

func (u *OTPUsecase) CompareCode(otp authmodels.OTP, code string) bool {
	actual := u.HashCode(otp.ID, otp.PhoneNumber, code)
	return hmac.Equal([]byte(actual), []byte(otp.OTPCodeHash))
}

func (u *OTPUsecase) EnforceRateLimit(ctx context.Context, phoneNumber string) error {
	if u.redisClient == nil {
		return nil
	}
	key := OTPRateLimitPrefix + phoneNumber
	count, err := u.redisClient.Incr(ctx, key).Result()
	if err != nil {
		return apperrors.Unavailable("OTP rate limit is temporarily unavailable.", err)
	}
	if count == 1 {
		if err := u.redisClient.Expire(ctx, key, u.rateWindow).Err(); err != nil {
			return apperrors.Unavailable("OTP rate limit is temporarily unavailable.", err)
		}
	}
	if count > int64(u.maxRequests) {
		return apperrors.RateLimited("Too many verification code requests. Try again later.", nil)
	}
	return nil
}

func (u *OTPUsecase) Create(ctx context.Context, phoneNumber string) (authmodels.OTP, string, error) {
	if err := u.checkLimit(ctx, phoneNumber); err != nil {
		return authmodels.OTP{}, "", err
	}
	code, err := u.GenerateCode()
	if err != nil {
		return authmodels.OTP{}, "", apperrors.Internal("Verification code could not be generated.", err)
	}
	otpID := uuid.NewString()
	otp := authmodels.OTP{
		ID:          otpID,
		PhoneNumber: phoneNumber,
		OTPCodeHash: u.HashCode(otpID, phoneNumber, code),
		MaxAttempts: u.maxAttempts,
		ExpiresAt:   u.now().Add(u.ttl),
	}
	saved, err := u.repository.Create(ctx, otp)
	if err != nil {
		return authmodels.OTP{}, "", err
	}
	return saved, code, nil
}

func (u *OTPUsecase) VerifyLatest(ctx context.Context, phoneNumber string, code string) (authmodels.OTP, error) {
	if err := authmodels.ValidateOTPCode(code); err != nil {
		return authmodels.OTP{}, err
	}
	otp, ok, err := u.repository.LatestByPhone(ctx, phoneNumber)
	if err != nil {
		return authmodels.OTP{}, err
	}
	if !ok {
		return authmodels.OTP{}, otpInvalid("Invalid verification code.")
	}
	now := u.now()
	if otp.IsVerified() {
		return authmodels.OTP{}, otpInvalid("Invalid verification code.")
	}
	if otp.IsLocked(now) {
		return authmodels.OTP{}, otpLocked("Verification code is locked. Try again later.")
	}
	if otp.IsExpired(now) {
		return authmodels.OTP{}, otpExpired("Verification code has expired.")
	}
	if !u.CompareCode(otp, code) {
		attempts := otp.Attempts + 1
		var lockedUntil *time.Time
		if attempts >= otp.MaxAttempts {
			lockTime := now.Add(u.lockoutTTL)
			lockedUntil = &lockTime
		}
		if err := u.repository.RecordFailedAttempt(ctx, otp.ID, attempts, lockedUntil); err != nil {
			return authmodels.OTP{}, err
		}
		if lockedUntil != nil {
			return authmodels.OTP{}, otpLocked("Verification code is locked. Try again later.")
		}
		return authmodels.OTP{}, otpInvalid("Invalid verification code.")
	}
	if err := u.repository.MarkVerified(ctx, otp.ID); err != nil {
		return authmodels.OTP{}, err
	}
	return otp, nil
}

func (u *OTPUsecase) TTLSeconds() int64 {
	return int64(u.ttl.Seconds())
}

// ValidatePhoneNumber validates a normalised phone number.
// Enforces E.164: must start with '+', followed by 7–15 digits.
func ValidatePhoneNumber(phoneNumber string) error {
	if phoneNumber == "" {
		return apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "phone_number", Message: "Phone number is required."},
		})
	}
	if phoneNumber[0] != '+' {
		return apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "phone_number", Message: "Phone number must be a valid E.164 number."},
		})
	}
	if len(phoneNumber) > 16 {
		return apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "phone_number", Message: "Phone number is too long."},
		})
	}
	digits := 0
	for _, c := range phoneNumber[1:] {
		if c < '0' || c > '9' {
			return apperrors.Validation("Check your details.", []apperrors.FieldViolation{
				{Field: "phone_number", Message: "Phone number must be a valid E.164 number."},
			})
		}
		digits++
	}
	if digits < 7 {
		return apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "phone_number", Message: "Phone number must be a valid E.164 number."},
		})
	}
	return nil
}

// NormalizePhoneNumber strips formatting characters then validates E.164.
func NormalizePhoneNumber(raw string) (string, error) {
	var b strings.Builder
	for index, char := range raw {
		switch {
		case char == '+' && index == 0:
			b.WriteRune(char)
		case char >= '0' && char <= '9':
			b.WriteRune(char)
		case char == ' ' || char == '-' || char == '(' || char == ')':
			// strip
		default:
			return "", apperrors.Validation("Check your details.", []apperrors.FieldViolation{
				{Field: "phone_number", Message: "Phone number must be a valid E.164 number."},
			})
		}
	}
	phoneNumber := b.String()
	if err := ValidatePhoneNumber(phoneNumber); err != nil {
		return "", err
	}
	return phoneNumber, nil
}
