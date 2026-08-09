package fieldcrypto

import (
	"bytes"
	"testing"
)

func testKey(fill byte) []byte {
	key := make([]byte, KeySize)
	for i := range key {
		key[i] = fill
	}
	return key
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	provider, err := NewEnvKeyProvider(map[int][]byte{1: testKey(0x01)}, 1)
	if err != nil {
		t.Fatalf("build provider: %v", err)
	}
	plaintext := []byte("A1234567 (a national identification number)")

	sealed, err := Encrypt(provider, plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if sealed.KeyVersion != 1 {
		t.Errorf("keyVersion = %d, want 1", sealed.KeyVersion)
	}
	if bytes.Contains(sealed.Ciphertext, plaintext) {
		t.Fatal("ciphertext must not contain the plaintext verbatim")
	}

	decrypted, err := Decrypt(provider, sealed)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("decrypted = %q, want %q", decrypted, plaintext)
	}
}

func TestEncrypt_NeverReusesNonce(t *testing.T) {
	provider, err := NewEnvKeyProvider(map[int][]byte{1: testKey(0x02)}, 1)
	if err != nil {
		t.Fatalf("build provider: %v", err)
	}
	first, err := Encrypt(provider, []byte("same plaintext"))
	if err != nil {
		t.Fatalf("encrypt 1: %v", err)
	}
	second, err := Encrypt(provider, []byte("same plaintext"))
	if err != nil {
		t.Fatalf("encrypt 2: %v", err)
	}
	if bytes.Equal(first.Nonce, second.Nonce) {
		t.Fatal("two encryptions must never reuse the same nonce")
	}
	if bytes.Equal(first.Ciphertext, second.Ciphertext) {
		t.Fatal("identical plaintext must not produce identical ciphertext (nonce reuse or deterministic encryption)")
	}
}

func TestDecrypt_TamperedCiphertextFailsAuthentication(t *testing.T) {
	provider, err := NewEnvKeyProvider(map[int][]byte{1: testKey(0x03)}, 1)
	if err != nil {
		t.Fatalf("build provider: %v", err)
	}
	sealed, err := Encrypt(provider, []byte("national id AB123456"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	tampered := sealed
	tampered.Ciphertext = append([]byte(nil), sealed.Ciphertext...)
	tampered.Ciphertext[0] ^= 0xFF // flip a bit -- simulates a tampered/corrupted value

	if _, err := Decrypt(provider, tampered); err == nil {
		t.Fatal("expected tampered ciphertext to fail authentication, got nil error")
	}
}

func TestDecrypt_WrongKeyFails(t *testing.T) {
	providerA, err := NewEnvKeyProvider(map[int][]byte{1: testKey(0x04)}, 1)
	if err != nil {
		t.Fatalf("build provider A: %v", err)
	}
	providerB, err := NewEnvKeyProvider(map[int][]byte{1: testKey(0x05)}, 1)
	if err != nil {
		t.Fatalf("build provider B: %v", err)
	}
	sealed, err := Encrypt(providerA, []byte("secret"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := Decrypt(providerB, sealed); err == nil {
		t.Fatal("expected decryption under a different key to fail")
	}
}

// TestKeyRotation_OldCiphertextStillDecryptableUnderRetiredVersion proves
// the versioned-key-provider design actually supports rotation: encrypting
// under v1, then rotating CurrentVersion to v2 (v1's key still present),
// still decrypts the v1 ciphertext correctly, while new encryptions use v2.
func TestKeyRotation_OldCiphertextStillDecryptableUnderRetiredVersion(t *testing.T) {
	v1Provider, err := NewEnvKeyProvider(map[int][]byte{1: testKey(0x06)}, 1)
	if err != nil {
		t.Fatalf("build v1 provider: %v", err)
	}
	oldSealed, err := Encrypt(v1Provider, []byte("encrypted under v1"))
	if err != nil {
		t.Fatalf("encrypt under v1: %v", err)
	}

	rotatedProvider, err := NewEnvKeyProvider(map[int][]byte{1: testKey(0x06), 2: testKey(0x07)}, 2)
	if err != nil {
		t.Fatalf("build rotated provider: %v", err)
	}

	decryptedOld, err := Decrypt(rotatedProvider, oldSealed)
	if err != nil {
		t.Fatalf("decrypt v1 ciphertext under rotated provider: %v", err)
	}
	if string(decryptedOld) != "encrypted under v1" {
		t.Fatalf("decrypted = %q, want original plaintext", decryptedOld)
	}

	newSealed, err := Encrypt(rotatedProvider, []byte("encrypted under v2"))
	if err != nil {
		t.Fatalf("encrypt under rotated provider: %v", err)
	}
	if newSealed.KeyVersion != 2 {
		t.Fatalf("new encryption keyVersion = %d, want 2 (current)", newSealed.KeyVersion)
	}
}

// TestKeyRotation_RetiredVersionRemovedMakesOldCiphertextUnrecoverable
// documents the real, disclosed consequence of actually retiring (not just
// rotating past) a key version: a ciphertext encrypted under a removed
// version becomes permanently undecryptable. This is expected behavior
// (retiring a key is supposed to make old data unrecoverable), pinned as a
// test so it is a deliberate, known property, not a surprise.
func TestKeyRotation_RetiredVersionRemovedMakesOldCiphertextUnrecoverable(t *testing.T) {
	v1Provider, err := NewEnvKeyProvider(map[int][]byte{1: testKey(0x08)}, 1)
	if err != nil {
		t.Fatalf("build v1 provider: %v", err)
	}
	sealed, err := Encrypt(v1Provider, []byte("will become unrecoverable"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	// v1's key is no longer present at all -- fully retired, not just
	// superseded.
	v2OnlyProvider, err := NewEnvKeyProvider(map[int][]byte{2: testKey(0x09)}, 2)
	if err != nil {
		t.Fatalf("build v2-only provider: %v", err)
	}
	if _, err := Decrypt(v2OnlyProvider, sealed); err == nil {
		t.Fatal("expected decryption to fail once the encrypting key version is fully retired")
	}
}

func TestNewEnvKeyProvider_FailsClosedWithNoKeys(t *testing.T) {
	if _, err := NewEnvKeyProvider(map[int][]byte{}, 1); err != ErrNoKeysConfigured {
		t.Fatalf("err = %v, want ErrNoKeysConfigured", err)
	}
}

func TestNewEnvKeyProvider_FailsClosedWhenCurrentVersionHasNoKey(t *testing.T) {
	_, err := NewEnvKeyProvider(map[int][]byte{1: testKey(0x0A)}, 2)
	if err == nil {
		t.Fatal("expected an error when currentVersion has no corresponding key")
	}
}

func TestNewEnvKeyProvider_FailsClosedOnWrongKeySize(t *testing.T) {
	_, err := NewEnvKeyProvider(map[int][]byte{1: []byte("too-short")}, 1)
	if err == nil {
		t.Fatal("expected an error for a key that is not exactly KeySize bytes")
	}
}

// TestNilProvider_FailsClosedNotPanics proves a nil KeyProvider (the
// "readiness failure when required keys are unavailable" case -- a
// deployment that never configured encryption keys at all) is a clean
// error from Encrypt/Decrypt, never a nil-pointer panic that could crash a
// request handler.
func TestNilProvider_FailsClosedNotPanics(t *testing.T) {
	if _, err := Encrypt(nil, []byte("x")); err != ErrNoKeysConfigured {
		t.Fatalf("Encrypt with nil provider: err = %v, want ErrNoKeysConfigured", err)
	}
	if _, err := Decrypt(nil, Sealed{}); err != ErrNoKeysConfigured {
		t.Fatalf("Decrypt with nil provider: err = %v, want ErrNoKeysConfigured", err)
	}
	var typedNil *EnvKeyProvider
	if _, err := typedNil.CurrentVersion(); err != ErrNoKeysConfigured {
		t.Fatalf("typed-nil CurrentVersion: err = %v, want ErrNoKeysConfigured", err)
	}
	if _, err := typedNil.Key(1); err != ErrNoKeysConfigured {
		t.Fatalf("typed-nil Key: err = %v, want ErrNoKeysConfigured", err)
	}
}
