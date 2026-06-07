package authusecases

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestAccessTokenUsesDispatchRiderClaims(t *testing.T) {
	tokens := NewTokenUsecase([]byte("test-access-secret"), 15*time.Minute, 30*24*time.Hour)

	rawToken, expiresAt, err := tokens.GenerateAccessToken("rider-id", "+15551234567", "session-id")
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}
	if rawToken == "" {
		t.Fatal("GenerateAccessToken() returned empty token")
	}
	if !expiresAt.After(time.Now()) {
		t.Fatalf("expiresAt = %v, want future time", expiresAt)
	}

	claims, err := tokens.ValidateAccessToken(rawToken)
	if err != nil {
		t.Fatalf("ValidateAccessToken() error = %v", err)
	}
	if claims.DispatchRiderID != "rider-id" {
		t.Fatalf("DispatchRiderID = %q", claims.DispatchRiderID)
	}
	if claims.PhoneNumber != "+15551234567" {
		t.Fatalf("PhoneNumber = %q", claims.PhoneNumber)
	}
	if claims.SessionID != "session-id" {
		t.Fatalf("SessionID = %q", claims.SessionID)
	}
	if claims.TokenType != TokenTypeAccess {
		t.Fatalf("TokenType = %q, want %q", claims.TokenType, TokenTypeAccess)
	}
}

func TestGeneratedTokenHeaderUsesHS256(t *testing.T) {
	tokens := NewTokenUsecase([]byte("test-access-secret"), 15*time.Minute, 30*24*time.Hour)

	rawToken, _, err := tokens.GenerateAccessToken("rider-id", "+15551234567", "session-id")
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	headerPart, _, ok := strings.Cut(rawToken, ".")
	if !ok {
		t.Fatal("generated token does not contain JWT separator")
	}
	headerJSON, err := base64.RawURLEncoding.DecodeString(headerPart)
	if err != nil {
		t.Fatalf("decode header: %v", err)
	}
	var header map[string]string
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		t.Fatalf("unmarshal header: %v", err)
	}
	if header["alg"] != "HS256" {
		t.Fatalf("alg = %q, want HS256", header["alg"])
	}
}

func TestRefreshTokenIsRejectedAsAccessToken(t *testing.T) {
	tokens := NewTokenUsecase([]byte("test-access-secret"), 15*time.Minute, 30*24*time.Hour)

	rawToken, _, err := tokens.GenerateRefreshToken("rider-id", "+15551234567", "session-id")
	if err != nil {
		t.Fatalf("GenerateRefreshToken() error = %v", err)
	}

	_, err = tokens.ValidateAccessToken(rawToken)
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("ValidateAccessToken(refresh) error = %v, want %v", err, ErrInvalidToken)
	}
}

func TestHashRefreshTokenDoesNotReturnPlainToken(t *testing.T) {
	sessions := NewSessionUsecase(nil, []byte("test-refresh-secret"), 30*24*time.Hour)

	hash := sessions.HashRefreshToken("refresh-token")
	if hash == "refresh-token" {
		t.Fatal("HashRefreshToken() returned plain token")
	}
	if hash == "" {
		t.Fatal("HashRefreshToken() returned empty hash")
	}
}
