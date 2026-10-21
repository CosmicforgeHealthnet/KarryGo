package webhooks

import "testing"

func TestNewCryptorRequiresKey(t *testing.T) {
	if _, err := NewCryptor(""); err == nil {
		t.Fatal("NewCryptor(\"\") = nil error, want ErrMissingWebhookKey")
	}
}

func TestEncryptDecryptRoundtrip(t *testing.T) {
	c, err := NewCryptor("server-side-key-material")
	if err != nil {
		t.Fatalf("NewCryptor() error = %v", err)
	}
	plain := "whsec_deadbeefdeadbeefdeadbeefdeadbeef"
	ct, err := c.EncryptSecret(plain)
	if err != nil {
		t.Fatalf("EncryptSecret() error = %v", err)
	}
	got, err := c.DecryptSecret(ct)
	if err != nil {
		t.Fatalf("DecryptSecret() error = %v", err)
	}
	if got != plain {
		t.Fatalf("roundtrip = %q, want %q", got, plain)
	}
}

func TestEncryptedDiffersFromPlaintextAndIsRandomized(t *testing.T) {
	c, _ := NewCryptor("key")
	plain := "whsec_secretvalue"
	ct1, _ := c.EncryptSecret(plain)
	ct2, _ := c.EncryptSecret(plain)
	if ct1 == plain {
		t.Fatal("ciphertext equals plaintext")
	}
	if ct1 == ct2 {
		t.Fatal("same plaintext produced identical ciphertexts (nonce not random)")
	}
}

func TestDecryptInvalidCiphertextFailsSafely(t *testing.T) {
	c, _ := NewCryptor("key")
	if _, err := c.DecryptSecret("!!!not-base64!!!"); err == nil {
		t.Fatal("DecryptSecret(non-base64) = nil error, want error")
	}
	if _, err := c.DecryptSecret("YWJjZA=="); err == nil { // valid base64, too short/garbage
		t.Fatal("DecryptSecret(garbage) = nil error, want error")
	}
	// Tampered ciphertext from a different key must not decrypt.
	other, _ := NewCryptor("different-key")
	ct, _ := other.EncryptSecret("whsec_x")
	if _, err := c.DecryptSecret(ct); err == nil {
		t.Fatal("DecryptSecret(wrong key) = nil error, want error")
	}
}
