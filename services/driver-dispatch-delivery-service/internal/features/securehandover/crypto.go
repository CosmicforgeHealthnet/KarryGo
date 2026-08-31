package securehandover

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

type credentialCipher struct{ secret []byte }

// A public SHA-256 of a six-digit credential request is an offline guessing
// oracle. Idempotency for secret-bearing commands must also be keyed.
func (c *credentialCipher) requestFingerprint(value any) string {
	raw, _ := json.Marshal(value)
	return hex.EncodeToString(c.mac("request-fingerprint", "", 1, string(raw)))
}

func newCredentialCipher(secret []byte) (*credentialCipher, error) {
	if len(secret) < 16 {
		return nil, errors.New("handover secret must contain at least 16 bytes")
	}
	return &credentialCipher{secret: append([]byte(nil), secret...)}, nil
}

func randomReference(prefix string) (string, error) {
	var raw [18]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return prefix + "_" + base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

// deriveOTP is a cryptographic PRF over a randomly generated challenge
// reference. It can be re-derived for the assigned rider or secret transport
// without storing plaintext, while remaining unpredictable without the key.
func (c *credentialCipher) deriveOTP(challengeRef string, version int64) string {
	digest := c.mac("otp-value", challengeRef, version, "")
	value := binary.BigEndian.Uint64(digest[:8]) % 1_000_000
	return fmt.Sprintf("%06d", value)
}

func (c *credentialCipher) deriveQR(challengeRef string, version int64) string {
	signature := c.mac("qr-token", challengeRef, version, "")
	return challengeRef + "." + integer(version) + "." + base64.RawURLEncoding.EncodeToString(signature[:24])
}

func (c *credentialCipher) hash(method Method, challengeRef string, version int64, value string) string {
	return hex.EncodeToString(c.mac("credential-hash:"+string(method), challengeRef, version, value))
}

func (c *credentialCipher) verify(method Method, challengeRef string, version int64, value, expected string) bool {
	actual := c.hash(method, challengeRef, version, value)
	return hmac.Equal([]byte(actual), []byte(expected))
}

func (c *credentialCipher) recipientRef(value string) string {
	return "receiver:" + hex.EncodeToString(c.mac("recipient", value, 0, ""))
}

func (c *credentialCipher) mac(domain, reference string, version int64, value string) []byte {
	mac := hmac.New(sha256.New, c.secret)
	mac.Write([]byte("dispatch-secure-handover-v1\x00"))
	mac.Write([]byte(domain))
	mac.Write([]byte{'\x00'})
	mac.Write([]byte(reference))
	mac.Write([]byte{'\x00'})
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], uint64(version))
	mac.Write(encoded[:])
	mac.Write([]byte{'\x00'})
	mac.Write([]byte(value))
	return mac.Sum(nil)
}
