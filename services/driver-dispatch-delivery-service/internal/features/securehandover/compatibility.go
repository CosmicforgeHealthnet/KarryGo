package securehandover

import (
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/handover"
)

// NewCompatibilityCodeRecord keeps the legacy Trip API but stores an opaque
// reference, not its redisplayable OTP. It reuses the secure-handover PRF and
// the established legacy verification hash; this is not a second challenge.
func NewCompatibilityCodeRecord(secret []byte, challengeID string, now time.Time) (handover.CodeRecord, error) {
	cipher, err := newCredentialCipher(secret)
	if err != nil {
		return handover.CodeRecord{}, err
	}
	ref, err := randomReference("handover")
	if err != nil {
		return handover.CodeRecord{}, err
	}
	code := cipher.deriveOTP("compatibility:"+challengeID+":"+ref, 1)
	return handover.CodeRecord{Code: ref, Hash: handover.HashCode(secret, challengeID, code), ExpiresAt: now.Add(handover.DefaultTTL)}, nil
}

// CompatibilityCode is called only after existing customer/provider ownership
// checks. The legacy numeric read path allows a staged upgrade before the
// guarded migration removes expired historical plaintext.
func CompatibilityCode(secret []byte, challengeID, stored string) (string, error) {
	if !strings.HasPrefix(stored, "handover_") {
		return stored, nil
	}
	cipher, err := newCredentialCipher(secret)
	if err != nil {
		return "", err
	}
	return cipher.deriveOTP("compatibility:"+challengeID+":"+stored, 1), nil
}
