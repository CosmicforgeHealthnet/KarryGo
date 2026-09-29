// Package handover provides pure, storage-agnostic primitives for generating
// and verifying short-lived numeric handover codes used to confirm package
// pickup/drop-off between a customer and a driver. It builds directly on the
// existing OTP primitives in shared/go/auth and adds no persistence of its
// own — each service persists CodeRecord fields in its own schema.
package handover

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/auth"
)

// Step identifies which side of a booking/trip a handover code belongs to.
type Step string

const (
	StepPickup           Step = "pickup"
	StepDelivery         Step = "delivery"
	StepReturn           Step = "return"
	StepProviderTransfer Step = "provider_transfer"
)

const (
	// DefaultTTL is how long a generated code remains valid.
	DefaultTTL = 15 * time.Minute
	// MaxVerifyAttempts caps how many incorrect submissions are allowed
	// against a single generated code before it must be regenerated.
	MaxVerifyAttempts = 5
	// ResendCooldown is the minimum time between two resend requests.
	ResendCooldown = 45 * time.Second
	// MaxResends caps how many times a single handover step can be
	// regenerated before requiring operational intervention.
	MaxResends = 5
)

// BuildChallengeID namespaces a handover code so it never collides with
// login OTPs or other services' handover codes, since auth.HashOTP hardcodes
// its own "otp" domain tag and relies on the caller for uniqueness.
func BuildChallengeID(service string, step Step, entityID string) string {
	return fmt.Sprintf("handover:%s:%s:%s", service, step, entityID)
}

// GenerateCode returns a new random numeric handover code.
func GenerateCode() (string, error) {
	return auth.GenerateNumericOTP(auth.DefaultOTPLength)
}

// HashCode hashes a handover code for storage/comparison.
func HashCode(secret []byte, challengeID, code string) string {
	return auth.HashOTP(secret, challengeID, "", code)
}

// VerifyCode reports whether code matches the previously stored hash for
// the given challengeID.
func VerifyCode(secret []byte, challengeID, code, expectedHash string) bool {
	return auth.VerifyOTP(secret, challengeID, "", code, expectedHash)
}

// CodeRecord is the plain-value shape of a single handover code's state.
// Services scan their own columns into/out of this struct; it is not
// persisted by this package.
type CodeRecord struct {
	Code         string
	Hash         string
	QRToken      string
	QRHash       string
	ChallengeRef string
	Version      int64
	Service      string
	ProviderID   string
	OwnerID      string
	BookingID    string
	Stage        Step
	ExpiresAt    time.Time
	Attempts     int
	ResendCount  int
	LastSentAt   *time.Time
	VerifiedAt   *time.Time
}

const SecureHandoverPurpose = "hauling-secure-handover-v1"

// NewSecureCodeRecord issues two independent in-memory credentials for one
// logical handover challenge. Only Hash and QRHash belong in persistence.
func NewSecureCodeRecord(secret []byte, service string, stage Step, challengeRef string, version int64, bookingID, providerID, ownerID string, resendCount int, now time.Time) (CodeRecord, error) {
	record := CodeRecord{
		ChallengeRef: strings.TrimSpace(challengeRef), Version: version,
		Service: strings.TrimSpace(service), Stage: stage, BookingID: strings.TrimSpace(bookingID),
		ProviderID: strings.TrimSpace(providerID), OwnerID: strings.TrimSpace(ownerID),
		// PostgreSQL stores timestamptz at microsecond precision. Normalize before
		// hashing so a persisted-and-reloaded expiry reproduces the same binding.
		ExpiresAt: now.UTC().Truncate(time.Microsecond).Add(DefaultTTL), ResendCount: resendCount,
	}
	if record.ChallengeRef == "" || record.Version < 1 || record.BookingID == "" || record.ProviderID == "" || record.OwnerID == "" {
		return CodeRecord{}, fmt.Errorf("secure handover binding is incomplete")
	}
	record.Code, record.QRToken = deriveSecureCredentials(secret, record)
	record.Hash = secureHash(secret, "OTP", record, record.Code)
	record.QRHash = secureHash(secret, "QR", record, record.QRToken)
	return record, nil
}

// RecoverSecureCredentials reconstructs the current challenge after its holder
// proves identity. Older random challenges fail hash verification and require
// a resend; no plaintext credential is persisted for either format.
func RecoverSecureCredentials(secret []byte, record CodeRecord) (CodeRecord, bool) {
	record.Code, record.QRToken = deriveSecureCredentials(secret, record)
	if !VerifySecureCredential(secret, "OTP", record, record.Code, record.Hash) ||
		!VerifySecureCredential(secret, "QR", record, record.QRToken, record.QRHash) {
		return CodeRecord{}, false
	}
	return record, true
}

func deriveSecureCredentials(secret []byte, record CodeRecord) (string, string) {
	derive := func(label string) []byte {
		mac := hmac.New(sha256.New, secret)
		for _, part := range []string{SecureHandoverPurpose, "receiver-read-v1", label, record.Service,
			string(record.Stage), record.ChallengeRef, strconv.FormatInt(record.Version, 10),
			record.BookingID, record.ProviderID, record.OwnerID, record.ExpiresAt.UTC().Format(time.RFC3339Nano)} {
			_, _ = mac.Write([]byte(part))
			_, _ = mac.Write([]byte{0})
		}
		return mac.Sum(nil)
	}
	otpDigest := derive("OTP")
	otp := binary.BigEndian.Uint64(otpDigest[:8]) % 1_000_000
	return fmt.Sprintf("%06d", otp), base64.RawURLEncoding.EncodeToString(derive("QR"))
}

// VerifySecureCredential compares the keyed digest in constant time. OTP and
// QR use separate purpose labels while sharing the same authoritative binding.
func VerifySecureCredential(secret []byte, method string, record CodeRecord, credential, expectedHash string) bool {
	actual := secureHash(secret, strings.ToUpper(strings.TrimSpace(method)), record, strings.TrimSpace(credential))
	return hmac.Equal([]byte(actual), []byte(expectedHash))
}

func secureHash(secret []byte, method string, record CodeRecord, credential string) string {
	mac := hmac.New(sha256.New, secret)
	parts := []string{
		SecureHandoverPurpose, strings.TrimSpace(record.Service), string(record.Stage),
		strings.TrimSpace(record.ChallengeRef), strconv.FormatInt(record.Version, 10),
		strings.TrimSpace(record.BookingID), strings.TrimSpace(record.ProviderID),
		strings.TrimSpace(record.OwnerID), record.ExpiresAt.UTC().Format(time.RFC3339Nano),
		strings.ToUpper(strings.TrimSpace(method)), credential,
	}
	for i, part := range parts {
		if i > 0 {
			_, _ = mac.Write([]byte{0})
		}
		_, _ = mac.Write([]byte(part))
	}
	return hex.EncodeToString(mac.Sum(nil))
}

// NewCodeRecord generates a fresh code and returns the record to persist.
// Attempts and ResendCount are reset to zero.
func NewCodeRecord(secret []byte, challengeID string, now time.Time) (CodeRecord, error) {
	code, err := GenerateCode()
	if err != nil {
		return CodeRecord{}, err
	}

	return CodeRecord{
		Code:      code,
		Hash:      HashCode(secret, challengeID, code),
		ExpiresAt: now.Add(DefaultTTL),
	}, nil
}

// IsExpired reports whether the record's code is no longer valid at now.
func (r CodeRecord) IsExpired(now time.Time) bool {
	if r.ExpiresAt.IsZero() {
		return true
	}
	return now.After(r.ExpiresAt)
}

// AttemptsExceeded reports whether the record has hit the verification
// attempt ceiling and must be regenerated before it can be tried again.
func (r CodeRecord) AttemptsExceeded() bool {
	return r.Attempts >= MaxVerifyAttempts
}

// CanResend reports whether a new code may be generated right now. When it
// cannot, the returned duration is how long the caller must still wait.
func (r CodeRecord) CanResend(now time.Time) (bool, time.Duration) {
	if r.ResendCount >= MaxResends {
		return false, 0
	}

	if r.LastSentAt == nil {
		return true, 0
	}

	elapsed := now.Sub(*r.LastSentAt)
	if elapsed >= ResendCooldown {
		return true, 0
	}

	return false, ResendCooldown - elapsed
}
