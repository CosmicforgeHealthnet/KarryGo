package messageusecases

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"
)

type RealtimeTokenManager struct {
	secret []byte
	now    func() time.Time
}

func NewRealtimeTokenManager(secret []byte) *RealtimeTokenManager {
	return &RealtimeTokenManager{
		secret: secret,
		now:    time.Now,
	}
}

func (m *RealtimeTokenManager) Sign(recipientType string, recipientID string, ttl time.Duration) (string, error) {
	if len(m.secret) == 0 {
		return "", fmt.Errorf("realtime token secret is required")
	}
	if recipientType == "" || recipientID == "" {
		return "", apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "recipient", Message: "Recipient type and id are required."},
		})
	}
	expiresAt := m.now().Add(ttl).Unix()
	payload := recipientType + "." + recipientID + "." + strconv.FormatInt(expiresAt, 10)
	signature := m.sign(payload)
	return base64.RawURLEncoding.EncodeToString([]byte(payload + "." + signature)), nil
}

func (m *RealtimeTokenManager) Verify(token string) (string, string, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return "", "", apperrors.Unauthorized("Realtime token is invalid.", err)
	}
	parts := strings.Split(string(decoded), ".")
	if len(parts) != 4 {
		return "", "", apperrors.Unauthorized("Realtime token is invalid.", nil)
	}
	payload := strings.Join(parts[:3], ".")
	if !hmac.Equal([]byte(m.sign(payload)), []byte(parts[3])) {
		return "", "", apperrors.Unauthorized("Realtime token is invalid.", nil)
	}
	expiresAt, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || expiresAt <= m.now().Unix() {
		return "", "", apperrors.Unauthorized("Realtime token has expired.", err)
	}
	return parts[0], parts[1], nil
}

func (m *RealtimeTokenManager) sign(payload string) string {
	mac := hmac.New(sha256.New, m.secret)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
