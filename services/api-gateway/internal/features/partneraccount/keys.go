package partneraccount

import (
	"crypto/rand"
	"encoding/hex"
)

// apiKeyPrefixLen matches partnerauth's lookup prefix length (first 12 chars).
const apiKeyPrefixLen = 12

// generateRawAPIKey returns a fresh raw API key and its public lookup prefix.
// Format: sk_test_<hex> for sandbox, sk_live_<hex> for live. Uses crypto/rand.
func generateRawAPIKey(environment string) (raw, prefix string, err error) {
	buf := make([]byte, 24) // 48 hex chars of entropy
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	scheme := "sk_test_"
	if environment == EnvironmentLive {
		scheme = "sk_live_"
	}
	raw = scheme + hex.EncodeToString(buf)
	return raw, raw[:apiKeyPrefixLen], nil
}
