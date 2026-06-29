package authusecases

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	sharedauth "cosmicforge/logistics/shared/go/auth"
)

func TestAccessTokenUsesDispatchRiderClaims(t *testing.T) {
	tokens := NewTokenUsecase([]byte("test-access-secret"), 15*time.Minute, 30*24*time.Hour)

	riderID := uuid.NewString()
	sessionID := uuid.NewString()
	rawToken, expiresAt, err := tokens.GenerateAccessToken(riderID, "+15551234567", sessionID)
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
	if claims.DispatchRiderID != riderID {
		t.Fatalf("DispatchRiderID = %q, want %q", claims.DispatchRiderID, riderID)
	}
	if claims.PhoneNumber != "+15551234567" {
		t.Fatalf("PhoneNumber = %q", claims.PhoneNumber)
	}
	if claims.SessionID != sessionID {
		t.Fatalf("SessionID = %q, want %q", claims.SessionID, sessionID)
	}
	if claims.Subject != riderID {
		t.Fatalf("Subject = %q, want %q", claims.Subject, riderID)
	}
	if claims.SID != sessionID {
		t.Fatalf("SID = %q, want %q", claims.SID, sessionID)
	}
	if _, err := uuid.Parse(claims.JWTID); err != nil {
		t.Fatalf("JWTID = %q, want UUID: %v", claims.JWTID, err)
	}
	if claims.Service != TokenService {
		t.Fatalf("Service = %q, want %q", claims.Service, TokenService)
	}
	if claims.Role != "dispatch_provider" {
		t.Fatalf("Role = %q, want dispatch_provider", claims.Role)
	}
	if claims.TokenType != TokenTypeAccess {
		t.Fatalf("TokenType = %q, want %q", claims.TokenType, TokenTypeAccess)
	}
	if claims.Type != TokenTypeAccess {
		t.Fatalf("Type = %q, want %q", claims.Type, TokenTypeAccess)
	}

	sharedClaims, err := sharedauth.NewTokenSigner([]byte("test-access-secret")).Verify(rawToken)
	if err != nil {
		t.Fatalf("shared auth Verify() error = %v", err)
	}
	if sharedClaims.Subject != riderID || sharedClaims.Service != TokenService || sharedClaims.Role != "dispatch_provider" {
		t.Fatalf("shared auth claims = %+v", sharedClaims)
	}
}

func TestGeneratedTokenHeaderUsesHS256(t *testing.T) {
	tokens := NewTokenUsecase([]byte("test-access-secret"), 15*time.Minute, 30*24*time.Hour)

	rawToken, _, err := tokens.GenerateAccessToken(uuid.NewString(), "+15551234567", uuid.NewString())
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

	rawToken, _, err := tokens.GenerateRefreshToken(uuid.NewString(), "+15551234567", uuid.NewString())
	if err != nil {
		t.Fatalf("GenerateRefreshToken() error = %v", err)
	}

	_, err = tokens.ValidateAccessToken(rawToken)
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("ValidateAccessToken(refresh) error = %v, want %v", err, ErrInvalidToken)
	}
}

func TestNonUUIDDispatchRiderIDIsRejected(t *testing.T) {
	tokens := NewTokenUsecase([]byte("test-access-secret"), 15*time.Minute, 30*24*time.Hour)

	// Generate a token with a non-UUID rider ID (bypasses validation at generation time).
	rawToken, _, err := tokens.GenerateAccessToken("not-a-uuid", "+15551234567", uuid.NewString())
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	_, err = tokens.ValidateAccessToken(rawToken)
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("ValidateAccessToken(non-uuid rider) error = %v, want ErrInvalidToken", err)
	}
}

func TestNonUUIDSessionIDIsRejected(t *testing.T) {
	tokens := NewTokenUsecase([]byte("test-access-secret"), 15*time.Minute, 30*24*time.Hour)

	// Generate a token with a non-UUID session ID.
	rawToken, _, err := tokens.GenerateAccessToken(uuid.NewString(), "+15551234567", "bad-session")
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	_, err = tokens.ValidateAccessToken(rawToken)
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("ValidateAccessToken(non-uuid session) error = %v, want ErrInvalidToken", err)
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
