// Package fieldcrypto implements Section 28's recoverable-encryption
// primitive: authenticated encryption (AES-256-GCM, a standard, well-
// reviewed Go standard-library construction -- never an invented cipher)
// behind a versioned key-provider interface, for the rare column disposition
// that genuinely needs the plaintext back (ENCRYPTED in
// docs/section28-sensitive-column-inventory.json), as distinct from HASHED
// (equality-only, never recoverable) or a bare reference.
//
// As of this package's introduction, zero columns in
// docs/section28-sensitive-column-inventory.json are actually dispositioned
// ENCRYPTED -- every column this schema currently stores that could need
// it (personal government-identity values: NIN, BVN, passport, driver's
// licence numbers) is not actually stored anywhere in this schema today
// (confirmed by the inventory itself, which found none). This package
// exists as real, tested, ready infrastructure for the day one is added --
// internal/section28contract.ClassifyDisposition already defaults any
// future GOVERNMENT_ID-category column that is not a bare
// "registration_number" to ENCRYPTED, so a real future column is
// mechanically routed here rather than silently defaulting to plaintext.
package fieldcrypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
)

// KeySize is the required AES-256 key length in bytes.
const KeySize = 32

// KeyProvider resolves versioned symmetric keys. Never returns a raw key
// from a database -- see EnvKeyProvider's doc comment for why every real
// implementation in this service reads from process environment /
// configuration, never a table this service itself owns.
type KeyProvider interface {
	// CurrentVersion returns the key version new encryptions should use.
	CurrentVersion() (int, error)
	// Key returns the raw key bytes for a specific version -- needed to
	// decrypt a ciphertext that was encrypted under an older version during
	// key rotation.
	Key(version int) ([]byte, error)
}

// ErrNoKeysConfigured means the provider has no usable key at all --
// callers (specifically internal/app's production readiness checks, once a
// real ENCRYPTED column exists) must fail closed on this, never silently
// fall back to storing plaintext.
var ErrNoKeysConfigured = errors.New("fieldcrypto: no encryption keys configured")

// ErrKeyVersionNotFound means a specific requested key version is not
// available -- decrypting a value encrypted under a version whose key has
// been permanently retired, which is a data-loss configuration error, not
// a transient one.
var ErrKeyVersionNotFound = errors.New("fieldcrypto: requested key version not found")

// EnvKeyProvider resolves versioned keys from an in-memory map built at
// startup from configuration (typically parsed from an env var in the same
// "name:vN=value" style internal/platform/serviceauth.ParseVersionedSecrets
// already established for service credentials -- deliberately the same
// convention, not a second one). Keys are held only in process memory,
// never written to any table this service owns ("no keys in the
// database").
type EnvKeyProvider struct {
	current int
	keys    map[int][]byte
}

// NewEnvKeyProvider builds a provider from a version->key map and the
// version new encryptions should use. Returns an error (fail closed, never
// a provider that silently has no usable key) if keys is empty, if
// currentVersion has no corresponding key, or if any key is not exactly
// KeySize bytes.
func NewEnvKeyProvider(keys map[int][]byte, currentVersion int) (*EnvKeyProvider, error) {
	if len(keys) == 0 {
		return nil, ErrNoKeysConfigured
	}
	if _, ok := keys[currentVersion]; !ok {
		return nil, fmt.Errorf("fieldcrypto: current version %d has no configured key", currentVersion)
	}
	for version, key := range keys {
		if len(key) != KeySize {
			return nil, fmt.Errorf("fieldcrypto: key version %d is %d bytes, want %d (AES-256 requires a 32-byte key)", version, len(key), KeySize)
		}
	}
	return &EnvKeyProvider{current: currentVersion, keys: keys}, nil
}

// CurrentVersion implements KeyProvider.
func (p *EnvKeyProvider) CurrentVersion() (int, error) {
	if p == nil {
		return 0, ErrNoKeysConfigured
	}
	return p.current, nil
}

// Key implements KeyProvider.
func (p *EnvKeyProvider) Key(version int) ([]byte, error) {
	if p == nil {
		return nil, ErrNoKeysConfigured
	}
	key, ok := p.keys[version]
	if !ok {
		return nil, ErrKeyVersionNotFound
	}
	return key, nil
}

// Sealed is one encrypted value: ciphertext, the nonce used to produce it,
// and which key version encrypted it -- all three are required to decrypt,
// and none of the three is sensitive on its own (the nonce and version are
// safe to store alongside the ciphertext in the same column/row; they carry
// no information about the plaintext without the key).
type Sealed struct {
	Ciphertext []byte
	Nonce      []byte
	KeyVersion int
}

// Encrypt authenticated-encrypts plaintext under provider's current key
// version. A fresh random nonce is generated for every call (GCM's security
// depends on never reusing a nonce with the same key -- crypto/rand, not a
// counter or timestamp, removes any possibility of an implementation bug
// reusing one).
func Encrypt(provider KeyProvider, plaintext []byte) (Sealed, error) {
	if provider == nil {
		return Sealed{}, ErrNoKeysConfigured
	}
	version, err := provider.CurrentVersion()
	if err != nil {
		return Sealed{}, err
	}
	key, err := provider.Key(version)
	if err != nil {
		return Sealed{}, err
	}
	gcm, err := newGCM(key)
	if err != nil {
		return Sealed{}, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return Sealed{}, fmt.Errorf("fieldcrypto: generate nonce: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
	return Sealed{Ciphertext: ciphertext, Nonce: nonce, KeyVersion: version}, nil
}

// Decrypt authenticated-decrypts a Sealed value, using whichever key
// version it was encrypted under (not necessarily the provider's current
// one -- this is what makes key rotation possible: an old ciphertext
// remains decryptable as long as its key version has not been removed from
// the provider). Returns an error (never a partial/corrupted plaintext) if
// the ciphertext was tampered with, encrypted under a different key, or the
// requested key version is unavailable -- GCM's authentication tag makes
// tampering cryptographically detectable, not just a checksum-level
// heuristic.
func Decrypt(provider KeyProvider, sealed Sealed) ([]byte, error) {
	if provider == nil {
		return nil, ErrNoKeysConfigured
	}
	key, err := provider.Key(sealed.KeyVersion)
	if err != nil {
		return nil, err
	}
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	if len(sealed.Nonce) != gcm.NonceSize() {
		return nil, fmt.Errorf("fieldcrypto: nonce is %d bytes, want %d", len(sealed.Nonce), gcm.NonceSize())
	}
	plaintext, err := gcm.Open(nil, sealed.Nonce, sealed.Ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("fieldcrypto: decrypt: authentication failed (tampered ciphertext, wrong key, or corrupted data): %w", err)
	}
	return plaintext, nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("fieldcrypto: build cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("fieldcrypto: build GCM: %w", err)
	}
	return gcm, nil
}
