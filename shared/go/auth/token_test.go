package auth

import (
	"errors"
	"testing"
	"time"
)

func TestTokenSignerSignAndVerify(t *testing.T) {
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	signer := NewTokenSigner([]byte("secret")).WithClock(func() time.Time { return now })

	token, err := signer.Sign(Claims{
		Subject:   "customer-id",
		Role:      "customer",
		Service:   "customer",
		SessionID: "session-id",
		Type:      TokenTypeAccess,
		ExpiresAt: now.Add(time.Minute).Unix(),
	})
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	claims, err := signer.Verify(token)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if claims.Subject != "customer-id" || claims.Role != "customer" || claims.Service != "customer" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestTokenSignerRejectsExpiredToken(t *testing.T) {
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	signer := NewTokenSigner([]byte("secret")).WithClock(func() time.Time { return now })

	token, err := signer.Sign(Claims{
		Subject:   "customer-id",
		Role:      "customer",
		Service:   "customer",
		SessionID: "session-id",
		Type:      TokenTypeAccess,
		ExpiresAt: now.Add(-time.Minute).Unix(),
	})
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	_, err = signer.Verify(token)
	if !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("expected ErrExpiredToken, got %v", err)
	}
}

func TestHashRefreshToken(t *testing.T) {
	hash := HashRefreshToken([]byte("secret"), "refresh-token")
	if hash == "" {
		t.Fatal("expected refresh token hash")
	}
	if hash == "refresh-token" {
		t.Fatal("refresh token hash should not equal raw token")
	}
}
