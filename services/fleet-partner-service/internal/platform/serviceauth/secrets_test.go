package serviceauth

import (
	"bytes"
	"testing"
)

func TestParseVersionedSecrets_BareNameIsVersion1(t *testing.T) {
	secrets := ParseVersionedSecrets("driver-hauling-service=secretA,media-file-service=secretB")
	if !bytes.Equal(secrets.Get("driver-hauling-service", 1), []byte("secretA")) {
		t.Errorf("driver-hauling-service v1 = %q, want secretA", secrets.Get("driver-hauling-service", 1))
	}
	if !bytes.Equal(secrets.Get("media-file-service", 1), []byte("secretB")) {
		t.Errorf("media-file-service v1 = %q, want secretB", secrets.Get("media-file-service", 1))
	}
}

func TestParseVersionedSecrets_ExplicitVersionSuffix(t *testing.T) {
	secrets := ParseVersionedSecrets("payment-wallet-service=secretA,payment-wallet-service:v2=secretB")
	if !bytes.Equal(secrets.Get("payment-wallet-service", 1), []byte("secretA")) {
		t.Errorf("v1 = %q, want secretA", secrets.Get("payment-wallet-service", 1))
	}
	if !bytes.Equal(secrets.Get("payment-wallet-service", 2), []byte("secretB")) {
		t.Errorf("v2 = %q, want secretB", secrets.Get("payment-wallet-service", 2))
	}
	if secrets.Get("payment-wallet-service", 3) != nil {
		t.Error("expected no v3 secret configured")
	}
}

func TestParseVersionedSecrets_MalformedVersionSuffixFallsBackToLiteralName(t *testing.T) {
	// "service:vX" (non-numeric) is not a valid version suffix -- the whole
	// string is treated as a literal (version 1) service name rather than
	// silently dropped, matching ParseSecrets' own permissive-but-safe
	// posture for malformed entries elsewhere in this codebase.
	secrets := ParseVersionedSecrets("weird-service:vX=secretA")
	if !bytes.Equal(secrets.Get("weird-service:vX", 1), []byte("secretA")) {
		t.Errorf("expected literal-name fallback, got %q", secrets.Get("weird-service:vX", 1))
	}
}

func TestParseVersionedSecrets_EmptyAndMalformedEntriesIgnored(t *testing.T) {
	secrets := ParseVersionedSecrets(" , name-only , =no-name , real=value ,")
	if len(secrets) != 1 {
		t.Fatalf("expected exactly one parsed entry, got %d: %+v", len(secrets), secrets)
	}
	if !bytes.Equal(secrets.Get("real", 1), []byte("value")) {
		t.Errorf("real v1 = %q, want value", secrets.Get("real", 1))
	}
}

func TestVersionedSecrets_GetUnconfiguredReturnsNil(t *testing.T) {
	secrets := ParseVersionedSecrets("known=secret")
	if secrets.Get("unknown", 1) != nil {
		t.Error("expected nil for an unconfigured service name")
	}
	if secrets.Get("known", 99) != nil {
		t.Error("expected nil for an unconfigured version")
	}
}
