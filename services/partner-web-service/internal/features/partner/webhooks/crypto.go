package webhooks

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
)

// Cryptor provides authenticated encryption (AES-256-GCM) for webhook signing
// secrets at rest. The key is derived from server-side key material
// (WEBHOOK_SIGNING_SECRET) via SHA-256, so any passphrase length is accepted.
//
// Plaintext signing secrets are NEVER persisted or logged. They are encrypted
// before storage and decrypted only transiently in memory for HMAC signing.
type Cryptor struct {
	key [32]byte
}

// ErrMissingWebhookKey is returned when WEBHOOK_SIGNING_SECRET is not configured.
var ErrMissingWebhookKey = errors.New("WEBHOOK_SIGNING_SECRET is not set; webhook secrets cannot be encrypted securely")

// NewCryptor derives an AES-256 key from keyMaterial. It fails when keyMaterial
// is empty so webhook registration/delivery never runs insecurely.
func NewCryptor(keyMaterial string) (*Cryptor, error) {
	if keyMaterial == "" {
		return nil, ErrMissingWebhookKey
	}
	return &Cryptor{key: sha256.Sum256([]byte(keyMaterial))}, nil
}

// EncryptSecret encrypts plaintext and returns base64(nonce || ciphertext||tag).
// A fresh random nonce means the same plaintext yields different ciphertexts.
func (c *Cryptor) EncryptSecret(plain string) (string, error) {
	gcm, err := c.gcm()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// DecryptSecret reverses EncryptSecret. Tampered or malformed input fails safely.
func (c *Cryptor) DecryptSecret(ciphertext string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", errors.New("invalid webhook secret ciphertext")
	}
	gcm, err := c.gcm()
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("invalid webhook secret ciphertext")
	}
	nonce, body := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, body, nil)
	if err != nil {
		return "", errors.New("invalid webhook secret ciphertext")
	}
	return string(plain), nil
}

func (c *Cryptor) gcm() (cipher.AEAD, error) {
	block, err := aes.NewCipher(c.key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
