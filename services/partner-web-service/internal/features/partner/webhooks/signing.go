package webhooks

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
)

// Webhook HTTP headers.
const (
	HeaderSignature = "X-KarryGo-Signature"
	HeaderEvent     = "X-KarryGo-Event"
	HeaderDelivery  = "X-KarryGo-Delivery"
	HeaderTimestamp = "X-KarryGo-Timestamp"

	signingSecretPrefix = "whsec_"
)

// SignPayload computes the webhook signature header value for a body.
// The signed message is: <timestamp> + "." + <raw JSON body>.
// The returned value is "sha256=" + hex(HMAC-SHA256(message, secret)).
func SignPayload(secret string, timestamp int64, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(timestamp, 10)))
	mac.Write([]byte("."))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// generateSigningSecret returns a fresh plaintext secret like whsec_<32 hex chars>.
func generateSigningSecret() (string, error) {
	buf := make([]byte, 16) // 16 bytes -> 32 hex chars
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return signingSecretPrefix + hex.EncodeToString(buf), nil
}
