package authusecases

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/google/uuid"

	"cosmicforge/logistics/shared/go/apperrors"
	authmodels "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/models"
)

const refreshTestPhone = "+2348012345678"

type refreshFixture struct {
	service      *AuthUsecase
	identities   *fakeIdentityRepository
	sessions     *fakeSessionRepository
	refreshToken string
	session      authmodels.Session
	now          time.Time
}

func newRefreshFixture(t *testing.T, status string, expiresAt time.Time, revoked bool) refreshFixture {
	t.Helper()

	identityRepo := newFakeIdentityRepository()
	sessionRepo := newFakeSessionRepository()
	service := NewAuthUsecase(Options{
		Identities:         identityRepo,
		Sessions:           sessionRepo,
		AccessTokenSecret:  []byte("access-secret-32-bytes-long-xxxx"),
		RefreshTokenSecret: []byte("refresh-secret-32-bytes-long-xxx"),
		AccessTokenTTL:     15 * time.Minute,
		RefreshTokenTTL:    30 * 24 * time.Hour,
	})

	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	service.sessions.WithClock(func() time.Time { return now })
	service.tokens.WithClock(func() time.Time { return now })

	identity := authmodels.Identity{
		ID:          uuid.NewString(),
		PhoneNumber: refreshTestPhone,
		Status:      status,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	identityRepo.identities[refreshTestPhone] = identity

	refreshToken, err := service.sessions.GenerateSecureToken(32)
	if err != nil {
		t.Fatalf("GenerateSecureToken() error = %v", err)
	}

	session := authmodels.Session{
		ID:               uuid.NewString(),
		DispatchRiderID:  identity.ID,
		PhoneNumber:      identity.PhoneNumber,
		RefreshTokenHash: service.sessions.HashRefreshToken(refreshToken),
		ExpiresAt:        expiresAt,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if revoked {
		revokedAt := now.Add(-time.Minute)
		session.RevokedAt = &revokedAt
	}
	sessionRepo.sessions[session.ID] = session

	return refreshFixture{
		service:      service,
		identities:   identityRepo,
		sessions:     sessionRepo,
		refreshToken: refreshToken,
		session:      session,
		now:          now,
	}
}

func TestRefreshSuccessRotatesRefreshTokenAndIssuesAccessToken(t *testing.T) {
	fx := newRefreshFixture(t, authmodels.StatusActive, time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC), false)

	result, err := fx.service.Refresh(context.Background(), RefreshInput{RefreshToken: fx.refreshToken})
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}

	if result.TokenType != "Bearer" {
		t.Fatalf("TokenType = %q, want Bearer", result.TokenType)
	}
	if result.ExpiresInSeconds != 900 {
		t.Fatalf("ExpiresInSeconds = %d, want 900", result.ExpiresInSeconds)
	}
	if result.AccessToken == "" {
		t.Fatal("AccessToken must not be empty")
	}
	if len(result.RefreshToken) != 64 {
		t.Fatalf("RefreshToken len = %d, want 64", len(result.RefreshToken))
	}
	if result.RefreshToken == fx.refreshToken {
		t.Fatal("RefreshToken must be rotated")
	}

	claims, err := fx.service.tokens.ValidateAccessToken(result.AccessToken)
	if err != nil {
		t.Fatalf("ValidateAccessToken() error = %v", err)
	}
	if claims.DispatchRiderID != fx.session.DispatchRiderID {
		t.Fatalf("DispatchRiderID = %q, want %q", claims.DispatchRiderID, fx.session.DispatchRiderID)
	}
	if claims.PhoneNumber != fx.session.PhoneNumber {
		t.Fatalf("PhoneNumber = %q, want %q", claims.PhoneNumber, fx.session.PhoneNumber)
	}
	if claims.SessionID != fx.session.ID {
		t.Fatalf("SessionID = %q, want %q", claims.SessionID, fx.session.ID)
	}
	if claims.Role != authmodels.RoleDispatchProvider {
		t.Fatalf("Role = %q, want %q", claims.Role, authmodels.RoleDispatchProvider)
	}
	if claims.Service != TokenService {
		t.Fatalf("Service = %q, want %q", claims.Service, TokenService)
	}
	if claims.Subject != fx.session.DispatchRiderID {
		t.Fatalf("Subject = %q, want %q", claims.Subject, fx.session.DispatchRiderID)
	}
	if claims.SID != fx.session.ID {
		t.Fatalf("SID = %q, want %q", claims.SID, fx.session.ID)
	}
	if _, err := uuid.Parse(claims.JWTID); err != nil {
		t.Fatalf("JWTID = %q, want UUID: %v", claims.JWTID, err)
	}

	oldHash := fx.service.sessions.HashRefreshToken(fx.refreshToken)
	if _, ok, _ := fx.sessions.FindByRefreshTokenHash(context.Background(), oldHash); ok {
		t.Fatal("old refresh token hash must not resolve after rotation")
	}

	newHash := fx.service.sessions.HashRefreshToken(result.RefreshToken)
	if _, ok, _ := fx.sessions.FindByRefreshTokenHash(context.Background(), newHash); !ok {
		t.Fatal("new refresh token hash must resolve after rotation")
	}
	if stored := fx.sessions.sessions[fx.session.ID].RefreshTokenHash; stored == result.RefreshToken {
		t.Fatal("plain refresh token must not be stored")
	}
	if stored := fx.sessions.sessions[fx.session.ID].RefreshTokenHash; stored != newHash {
		t.Fatalf("stored hash = %q, want SHA-256 hash %q", stored, newHash)
	}
}

func TestRefreshMissingAndEmptyTokenReturnValidationFailed(t *testing.T) {
	fx := newRefreshFixture(t, authmodels.StatusActive, time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC), false)

	for _, token := range []string{"", "   "} {
		_, err := fx.service.Refresh(context.Background(), RefreshInput{RefreshToken: token})
		requireRefreshErrorCode(t, err, apperrors.CodeValidationFailed)
	}
}

func TestRefreshInvalidHexReturnsValidationFailed(t *testing.T) {
	fx := newRefreshFixture(t, authmodels.StatusActive, time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC), false)

	_, err := fx.service.Refresh(context.Background(), RefreshInput{RefreshToken: "not-a-64-char-token"})
	requireRefreshErrorCode(t, err, apperrors.CodeValidationFailed)

	_, err = fx.service.Refresh(context.Background(), RefreshInput{RefreshToken: "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"})
	requireRefreshErrorCode(t, err, apperrors.CodeValidationFailed)
}

func TestRefreshUnknownTokenReturnsUnauthorized(t *testing.T) {
	fx := newRefreshFixture(t, authmodels.StatusActive, time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC), false)

	unknown := "1111111111111111111111111111111111111111111111111111111111111111"
	_, err := fx.service.Refresh(context.Background(), RefreshInput{RefreshToken: unknown})
	requireRefreshErrorCode(t, err, apperrors.CodeUnauthorized)
}

func TestRefreshExpiredSessionReturnsUnauthorized(t *testing.T) {
	fx := newRefreshFixture(t, authmodels.StatusActive, time.Date(2026, 6, 2, 11, 59, 0, 0, time.UTC), false)

	_, err := fx.service.Refresh(context.Background(), RefreshInput{RefreshToken: fx.refreshToken})
	requireRefreshErrorCode(t, err, apperrors.CodeUnauthorized)
}

func TestRefreshRevokedSessionReturnsUnauthorized(t *testing.T) {
	fx := newRefreshFixture(t, authmodels.StatusActive, time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC), true)

	_, err := fx.service.Refresh(context.Background(), RefreshInput{RefreshToken: fx.refreshToken})
	requireRefreshErrorCode(t, err, apperrors.CodeUnauthorized)
}

func TestRefreshSuspendedIdentityReturnsForbiddenWithoutRotation(t *testing.T) {
	fx := newRefreshFixture(t, authmodels.StatusSuspended, time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC), false)
	oldHash := fx.sessions.sessions[fx.session.ID].RefreshTokenHash

	_, err := fx.service.Refresh(context.Background(), RefreshInput{RefreshToken: fx.refreshToken})
	requireRefreshErrorCode(t, err, apperrors.CodeForbidden)

	if got := fx.sessions.sessions[fx.session.ID].RefreshTokenHash; got != oldHash {
		t.Fatal("suspended identity must not rotate refresh token")
	}
}

func TestRefreshDeletedIdentityReturnsForbiddenWithoutRotation(t *testing.T) {
	fx := newRefreshFixture(t, authmodels.StatusDeleted, time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC), false)
	oldHash := fx.sessions.sessions[fx.session.ID].RefreshTokenHash

	_, err := fx.service.Refresh(context.Background(), RefreshInput{RefreshToken: fx.refreshToken})
	requireRefreshErrorCode(t, err, apperrors.CodeForbidden)

	if got := fx.sessions.sessions[fx.session.ID].RefreshTokenHash; got != oldHash {
		t.Fatal("deleted identity must not rotate refresh token")
	}
}

func TestHashRefreshTokenUsesSHA256(t *testing.T) {
	sessions := NewSessionUsecase(nil, nil, 30*24*time.Hour)
	token := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	sum := sha256.Sum256([]byte(token))
	want := hex.EncodeToString(sum[:])
	if got := sessions.HashRefreshToken(token); got != want {
		t.Fatalf("HashRefreshToken() = %q, want %q", got, want)
	}
}

func requireRefreshErrorCode(t *testing.T, err error, code apperrors.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error code %q, got nil", code)
	}
	appErr, ok := err.(*apperrors.Error)
	if !ok {
		t.Fatalf("expected *apperrors.Error, got %T", err)
	}
	if appErr.Code != code {
		t.Fatalf("code = %q, want %q", appErr.Code, code)
	}
}
