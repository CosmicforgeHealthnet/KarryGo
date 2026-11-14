package models

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrSignatureInvalid means a webhook signature does not verify.
var ErrSignatureInvalid = errors.New("integrations: webhook signature is invalid")

func RandomToken(bytes int) (string, error) {
	raw := make([]byte, bytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("integrations: generate credential: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func GenerateClientID() (string, error) {
	token, err := RandomToken(18)
	if err != nil {
		return "", err
	}
	return "kgo_" + token, nil
}

func HashSecret(key []byte, secret string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(secret))
	return mac.Sum(nil)
}

func SecretMatches(key []byte, secret string, expected []byte) bool {
	actual := HashSecret(key, secret)
	return len(expected) == len(actual) && subtle.ConstantTimeCompare(actual, expected) == 1
}

func EncryptSecret(key []byte, plaintext string) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("integrations: configure webhook encryption: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("integrations: configure webhook encryption: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("integrations: encrypt webhook secret: %w", err)
	}
	return gcm.Seal(nonce, nonce, []byte(plaintext), []byte("karrygo:webhook:v1")), nil
}

func DecryptSecret(key, ciphertext []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("integrations: configure webhook decryption: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("integrations: configure webhook decryption: %w", err)
	}
	if len(ciphertext) < gcm.NonceSize() {
		return "", errors.New("integrations: webhook ciphertext is invalid")
	}
	plain, err := gcm.Open(nil, ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():], []byte("karrygo:webhook:v1"))
	if err != nil {
		return "", errors.New("integrations: webhook ciphertext could not be decrypted")
	}
	return string(plain), nil
}

func signatureCanonical(subscriptionID, deliveryID, eventID string, timestamp int64, body []byte) string {
	sum := sha256.Sum256(body)
	return fmt.Sprintf("v1\n%s\n%s\n%s\n%d\n%s", subscriptionID, deliveryID, eventID, timestamp, hex.EncodeToString(sum[:]))
}

type SigningSecret struct {
	Version int
	Secret  string
}

func SignWebhook(subscriptionID, deliveryID, eventID string, timestamp int64, body []byte, secrets []SigningSecret) string {
	canonical := signatureCanonical(subscriptionID, deliveryID, eventID, timestamp, body)
	parts := make([]string, 0, len(secrets))
	for _, item := range secrets {
		mac := hmac.New(sha256.New, []byte(item.Secret))
		_, _ = mac.Write([]byte(canonical))
		parts = append(parts, fmt.Sprintf("v1.k%d=%s", item.Version, hex.EncodeToString(mac.Sum(nil))))
	}
	return strings.Join(parts, ",")
}

// VerifyWebhookSignature is the documented consumer-side verification helper.
// It performs a constant-time MAC comparison and rejects timestamps outside
// the configured replay window before accepting the body.
func VerifyWebhookSignature(secret string, secretVersion int, subscriptionID, deliveryID, eventID string,
	timestamp int64, body []byte, header string, now time.Time, replayWindow time.Duration) error {
	if replayWindow <= 0 || now.Sub(time.Unix(timestamp, 0)) > replayWindow || time.Unix(timestamp, 0).Sub(now) > replayWindow {
		return ErrSignatureInvalid
	}
	want := SignWebhook(subscriptionID, deliveryID, eventID, timestamp, body,
		[]SigningSecret{{Version: secretVersion, Secret: secret}})
	wantValue := strings.SplitN(want, "=", 2)
	for _, candidate := range strings.Split(header, ",") {
		parts := strings.SplitN(strings.TrimSpace(candidate), "=", 2)
		if len(parts) != 2 || parts[0] != wantValue[0] {
			continue
		}
		actual, errA := hex.DecodeString(parts[1])
		expected, errE := hex.DecodeString(wantValue[1])
		if errA == nil && errE == nil && len(actual) == len(expected) && subtle.ConstantTimeCompare(actual, expected) == 1 {
			return nil
		}
	}
	return ErrSignatureInvalid
}

var prohibitedPayloadKeys = map[string]bool{
	"password": true, "accesstoken": true, "refreshtoken": true, "secret": true,
	"authorization": true, "credential": true, "bankdetails": true, "identitydocument": true,
	"signedurl": true, "internalexception": true, "debug": true, "auditmetadata": true,
}

func ValidateWebhookPayload(body []byte) error {
	var value any
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("integrations: webhook payload is invalid: %w", err)
	}
	return walkPayload(value)
}

func walkPayload(value any) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "_", ""), "-", ""))
			if prohibitedPayloadKeys[normalized] {
				return fmt.Errorf("integrations: webhook payload contains prohibited field %q", key)
			}
			if err := walkPayload(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := walkPayload(child); err != nil {
				return err
			}
		}
	}
	return nil
}
