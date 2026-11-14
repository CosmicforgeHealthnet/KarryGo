package uploadclients

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// EncryptingPrivateStorage encrypts every private object at rest.
//
// It wraps any PrivateStorage backend (filesystem or bucket), so the bytes the
// backend holds are always ciphertext and the plaintext exists only inside this
// process while a validated upload or an authorized content grant is being
// served. Callers keep using the PrivateStorage port unchanged: Put takes
// plaintext, Get and Stat answer in plaintext terms.
//
// Stored object layout (format version 1):
//
//	magic    4 bytes  0x89 'K' 'G' 'M'
//	version  1 byte   0x01
//	idLen    1 byte   length of the key id (1..64)
//	keyID    idLen    ASCII key id naming the key that sealed the object
//	nonce    12 bytes random, fresh per object write
//	sealed   n+16     AES-256-GCM ciphertext followed by its tag
//
// The additional authenticated data binds the full header and the object key,
// so a ciphertext copied to a different key, or a header edited to name a
// different key or version, fails authentication instead of decrypting.
//
// The magic starts with 0x89, which no accepted plaintext type (PDF, JPEG,
// PNG, WebP, CSV, NDJSON) begins with, so a legacy object written before
// encryption is never mistaken for an encrypted one.
type EncryptingPrivateStorage struct {
	inner   PrivateStorage
	active  EncryptionKey
	keys    map[string]cipher.AEAD
	metrics CounterRecorder
}

// CounterRecorder is the narrow metrics port the decorator reports to.
type CounterRecorder interface {
	IncCounter(name string, labels map[string]string, delta float64)
}

// MetricLegacyPlaintextReads counts reads of objects that predate encryption
// at rest. A non-zero rate means legacy plaintext objects remain in storage.
const MetricLegacyPlaintextReads = "karry_media_private_legacy_plaintext_reads_total"

// EncryptionKey is one entry of MEDIA_FILE_PRIVATE_ENCRYPTION_KEYS.
type EncryptionKey struct {
	ID  string
	Key []byte
}

const (
	encryptionFormatVersion = 0x01
	encryptionKeySize       = 32
	encryptionNonceSize     = 12
	encryptionTagSize       = 16
	maxEncryptionKeyIDLen   = 64
)

var (
	encryptionMagic   = []byte{0x89, 'K', 'G', 'M'}
	encryptionKeyIDRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

	// ErrEncryptedObjectUnreadable means an object carries the encryption
	// header but cannot be authenticated: it was tampered with, truncated,
	// moved to another key, or sealed with a key this process does not hold.
	// It is never answered with the raw stored bytes.
	ErrEncryptedObjectUnreadable = errors.New("media storage: encrypted object cannot be decrypted")
)

// ParseEncryptionKeys parses "keyId:base64(32 bytes),keyId:base64,...".
//
// The first entry is the active key used for every new write; every entry is
// accepted for reads, which is what makes rotation possible. An empty value
// yields no keys and no error (encryption off). Any malformed entry is an
// error: a typo must stop the service, not silently disable encryption.
func ParseEncryptionKeys(raw string) ([]EncryptionKey, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	seen := map[string]bool{}
	var keys []EncryptionKey
	for index, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("media encryption keys: entry %d is empty", index+1)
		}
		id, encoded, ok := strings.Cut(part, ":")
		id = strings.TrimSpace(id)
		if !ok || !encryptionKeyIDRe.MatchString(id) {
			return nil, fmt.Errorf("media encryption keys: entry %d must be keyId:base64 with a key id of 1-64 characters [A-Za-z0-9._-]", index+1)
		}
		if seen[id] {
			return nil, fmt.Errorf("media encryption keys: duplicate key id %q", id)
		}
		key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
		if err != nil {
			return nil, fmt.Errorf("media encryption keys: key %q is not valid base64", id)
		}
		if len(key) != encryptionKeySize {
			return nil, fmt.Errorf("media encryption keys: key %q must decode to %d bytes, got %d", id, encryptionKeySize, len(key))
		}
		seen[id] = true
		keys = append(keys, EncryptionKey{ID: id, Key: key})
	}
	return keys, nil
}

// NewEncryptingPrivateStorage wraps inner. keys[0] is the active write key.
// metrics may be nil.
func NewEncryptingPrivateStorage(inner PrivateStorage, keys []EncryptionKey, metrics CounterRecorder) (*EncryptingPrivateStorage, error) {
	if inner == nil {
		return nil, fmt.Errorf("media encryption: inner storage is required")
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("media encryption: at least one key is required")
	}
	aeads := make(map[string]cipher.AEAD, len(keys))
	for _, k := range keys {
		if !encryptionKeyIDRe.MatchString(k.ID) || len(k.Key) != encryptionKeySize {
			return nil, fmt.Errorf("media encryption: invalid key %q", k.ID)
		}
		if _, dup := aeads[k.ID]; dup {
			return nil, fmt.Errorf("media encryption: duplicate key id %q", k.ID)
		}
		block, err := aes.NewCipher(k.Key)
		if err != nil {
			return nil, fmt.Errorf("media encryption: key %q: %w", k.ID, err)
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			return nil, fmt.Errorf("media encryption: key %q: %w", k.ID, err)
		}
		aeads[k.ID] = aead
	}
	return &EncryptingPrivateStorage{inner: inner, active: keys[0], keys: aeads, metrics: metrics}, nil
}

// ActiveKeyID names the key new writes are sealed with.
func (s *EncryptingPrivateStorage) ActiveKeyID() string { return s.active.ID }

func encryptionHeader(keyID string) []byte {
	header := make([]byte, 0, len(encryptionMagic)+2+len(keyID))
	header = append(header, encryptionMagic...)
	header = append(header, encryptionFormatVersion, byte(len(keyID)))
	return append(header, keyID...)
}

// additionalData binds the header and the object key. The key is length
// prefixed so no (header, key) pair can be confused with another.
func additionalData(header []byte, objectKey string) []byte {
	aad := make([]byte, 0, len(header)+8+len(objectKey))
	aad = append(aad, header...)
	aad = binary.BigEndian.AppendUint64(aad, uint64(len(objectKey)))
	return append(aad, objectKey...)
}

// Put seals content with the active key and stores the ciphertext.
func (s *EncryptingPrivateStorage) Put(ctx context.Context, key string, contentType string, content []byte) error {
	header := encryptionHeader(s.active.ID)
	nonce := make([]byte, encryptionNonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("media encryption: nonce: %w", err)
	}
	aead := s.keys[s.active.ID]
	stored := make([]byte, 0, len(header)+len(nonce)+len(content)+encryptionTagSize)
	stored = append(stored, header...)
	stored = append(stored, nonce...)
	stored = aead.Seal(stored, nonce, content, additionalData(header, key))
	return s.inner.Put(ctx, key, contentType, stored)
}

// Get returns the plaintext.
func (s *EncryptingPrivateStorage) Get(ctx context.Context, key string) ([]byte, error) {
	stored, err := s.inner.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	return s.open(key, stored)
}

// Stat returns the plaintext size.
//
// Callers compare it with the uploaded content length to prove the write was
// complete, so it must answer in plaintext terms. It authenticates the object
// rather than subtracting a fixed overhead: a truncated or damaged ciphertext
// then fails Stat, which is exactly the incomplete write the check exists to
// catch.
func (s *EncryptingPrivateStorage) Stat(ctx context.Context, key string) (int64, error) {
	content, err := s.Get(ctx, key)
	if err != nil {
		return 0, err
	}
	return int64(len(content)), nil
}

// Delete passes through.
func (s *EncryptingPrivateStorage) Delete(ctx context.Context, key string) error {
	return s.inner.Delete(ctx, key)
}

// Check passes through.
func (s *EncryptingPrivateStorage) Check(ctx context.Context) error { return s.inner.Check(ctx) }

func (s *EncryptingPrivateStorage) open(key string, stored []byte) ([]byte, error) {
	if !bytes.HasPrefix(stored, encryptionMagic) {
		// Written before encryption at rest was enabled. Served as stored so
		// existing objects stay readable; counted so the backlog is visible.
		if s.metrics != nil {
			s.metrics.IncCounter(MetricLegacyPlaintextReads, nil, 1)
		}
		return stored, nil
	}
	rest := stored[len(encryptionMagic):]
	if len(rest) < 2 || rest[0] != encryptionFormatVersion {
		return nil, fmt.Errorf("%w: unsupported format", ErrEncryptedObjectUnreadable)
	}
	idLen := int(rest[1])
	if idLen == 0 || idLen > maxEncryptionKeyIDLen || len(rest) < 2+idLen+encryptionNonceSize+encryptionTagSize {
		return nil, fmt.Errorf("%w: malformed header", ErrEncryptedObjectUnreadable)
	}
	keyID := string(rest[2 : 2+idLen])
	aead, ok := s.keys[keyID]
	if !ok {
		return nil, fmt.Errorf("%w: unknown key id", ErrEncryptedObjectUnreadable)
	}
	headerLen := len(encryptionMagic) + 2 + idLen
	header := stored[:headerLen]
	nonce := stored[headerLen : headerLen+encryptionNonceSize]
	sealed := stored[headerLen+encryptionNonceSize:]
	plaintext, err := aead.Open(nil, nonce, sealed, additionalData(header, key))
	if err != nil {
		return nil, fmt.Errorf("%w: authentication failed", ErrEncryptedObjectUnreadable)
	}
	return plaintext, nil
}
