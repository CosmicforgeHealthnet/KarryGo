package authusecases

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	authclients "karrygo/services/driver-dispatch-delivery-service/internal/features/auth/clients"
	authmodels "karrygo/services/driver-dispatch-delivery-service/internal/features/auth/models"
	"karrygo/shared/go/apperrors"
)

const logoutTestPhone = "+2348012345678"

type logoutFixture struct {
	service      *AuthUsecase
	sessions     *fakeSessionRepository
	refreshToken string
	session      authmodels.Session
	now          time.Time
}

func newLogoutFixture(t *testing.T, publisher authclients.EventPublisher) logoutFixture {
	t.Helper()

	identityRepo := newFakeIdentityRepository()
	sessionRepo := newFakeSessionRepository()
	service := NewAuthUsecase(Options{
		Identities:         identityRepo,
		Sessions:           sessionRepo,
		Publisher:          publisher,
		AccessTokenSecret:  []byte("access-secret-32-bytes-long-xxxx"),
		RefreshTokenSecret: []byte("refresh-secret-32-bytes-long-xxx"),
		AccessTokenTTL:     15 * time.Minute,
		RefreshTokenTTL:    30 * 24 * time.Hour,
	})

	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	service.sessions.WithClock(func() time.Time { return now })
	service.tokens.WithClock(func() time.Time { return now })

	identity := authmodels.Identity{
		ID:          "rider-logout-1",
		PhoneNumber: logoutTestPhone,
		Status:      authmodels.StatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	identityRepo.identities[logoutTestPhone] = identity

	refreshToken, err := service.sessions.GenerateSecureToken(32)
	if err != nil {
		t.Fatalf("GenerateSecureToken() error = %v", err)
	}

	session := authmodels.Session{
		ID:               "session-logout-1",
		DispatchRiderID:  identity.ID,
		PhoneNumber:      identity.PhoneNumber,
		RefreshTokenHash: service.sessions.HashRefreshToken(refreshToken),
		ExpiresAt:        now.Add(30 * 24 * time.Hour),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	sessionRepo.sessions[session.ID] = session

	return logoutFixture{
		service:      service,
		sessions:     sessionRepo,
		refreshToken: refreshToken,
		session:      session,
		now:          now,
	}
}

func TestLogoutSuccessWithoutRefreshTokenRevokesSessionAndPublishesEvent(t *testing.T) {
	pub := &capturePublisher{}
	fx := newLogoutFixture(t, pub)

	result, err := fx.service.Logout(context.Background(), LogoutInput{
		SessionID:       fx.session.ID,
		DispatchRiderID: fx.session.DispatchRiderID,
		PhoneNumber:     fx.session.PhoneNumber,
		Role:            authmodels.RoleDispatchProvider,
		CorrelationID:   "logout-req-001",
	})
	if err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if result.Message != "Logged out successfully." {
		t.Fatalf("Message = %q, want logged out success", result.Message)
	}

	stored := fx.sessions.sessions[fx.session.ID]
	if stored.RevokedAt == nil {
		t.Fatal("session revoked_at must be set")
	}
	if len(pub.loggedOutEvents) != 1 {
		t.Fatalf("logged out events = %d, want 1", len(pub.loggedOutEvents))
	}
	event := pub.loggedOutEvents[0]
	if event.Event != authclients.TopicLoggedOut {
		t.Fatalf("event = %q, want %q", event.Event, authclients.TopicLoggedOut)
	}
	if event.CorrelationID != "logout-req-001" {
		t.Fatalf("correlation_id = %q, want logout-req-001", event.CorrelationID)
	}
	if event.ProviderID != fx.session.DispatchRiderID {
		t.Fatalf("provider_id = %q, want %q", event.ProviderID, fx.session.DispatchRiderID)
	}
	if event.SessionID != fx.session.ID {
		t.Fatalf("session_id = %q, want %q", event.SessionID, fx.session.ID)
	}
	if event.CreatedAt.IsZero() {
		t.Fatal("created_at must be populated")
	}

	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	if strings.Contains(string(raw), fx.refreshToken) {
		t.Fatal("logged_out event must not contain refresh token value")
	}
}

func TestLogoutSuccessWithOptionalRefreshToken(t *testing.T) {
	pub := &capturePublisher{}
	fx := newLogoutFixture(t, pub)

	refreshToken := fx.refreshToken
	_, err := fx.service.Logout(context.Background(), LogoutInput{
		SessionID:       fx.session.ID,
		DispatchRiderID: fx.session.DispatchRiderID,
		PhoneNumber:     fx.session.PhoneNumber,
		Role:            authmodels.RoleDispatchProvider,
		RefreshToken:    &refreshToken,
		CorrelationID:   "logout-req-002",
	})
	if err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if fx.sessions.sessions[fx.session.ID].RevokedAt == nil {
		t.Fatal("session revoked_at must be set")
	}
	if len(pub.loggedOutEvents) != 1 {
		t.Fatalf("logged out events = %d, want 1", len(pub.loggedOutEvents))
	}
}

func TestLogoutRejectsMismatchedRefreshTokenWithoutRevoking(t *testing.T) {
	pub := &capturePublisher{}
	fx := newLogoutFixture(t, pub)
	otherRefreshToken := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	_, err := fx.service.Logout(context.Background(), LogoutInput{
		SessionID:       fx.session.ID,
		DispatchRiderID: fx.session.DispatchRiderID,
		PhoneNumber:     fx.session.PhoneNumber,
		Role:            authmodels.RoleDispatchProvider,
		RefreshToken:    &otherRefreshToken,
		CorrelationID:   "logout-req-003",
	})
	requireLogoutErrorCode(t, err, apperrors.CodeUnauthorized)
	if fx.sessions.sessions[fx.session.ID].RevokedAt != nil {
		t.Fatal("session must not be revoked when optional refresh token does not match")
	}
	if len(pub.loggedOutEvents) != 0 {
		t.Fatalf("logged out events = %d, want 0", len(pub.loggedOutEvents))
	}
}

func TestLogoutMakesRefreshTokenUnusable(t *testing.T) {
	fx := newLogoutFixture(t, &capturePublisher{})

	_, err := fx.service.Logout(context.Background(), LogoutInput{
		SessionID:       fx.session.ID,
		DispatchRiderID: fx.session.DispatchRiderID,
		PhoneNumber:     fx.session.PhoneNumber,
		Role:            authmodels.RoleDispatchProvider,
	})
	if err != nil {
		t.Fatalf("Logout() error = %v", err)
	}

	_, err = fx.service.Refresh(context.Background(), RefreshInput{RefreshToken: fx.refreshToken})
	requireLogoutErrorCode(t, err, apperrors.CodeUnauthorized)
}

func TestLogoutAlreadyRevokedReturnsUnauthorized(t *testing.T) {
	fx := newLogoutFixture(t, &capturePublisher{})

	_, err := fx.service.Logout(context.Background(), LogoutInput{
		SessionID:       fx.session.ID,
		DispatchRiderID: fx.session.DispatchRiderID,
		PhoneNumber:     fx.session.PhoneNumber,
		Role:            authmodels.RoleDispatchProvider,
	})
	if err != nil {
		t.Fatalf("first Logout() error = %v", err)
	}

	_, err = fx.service.Logout(context.Background(), LogoutInput{
		SessionID:       fx.session.ID,
		DispatchRiderID: fx.session.DispatchRiderID,
		PhoneNumber:     fx.session.PhoneNumber,
		Role:            authmodels.RoleDispatchProvider,
	})
	requireLogoutErrorCode(t, err, apperrors.CodeUnauthorized)
}

func TestLogoutPublishErrorReturnsInternal(t *testing.T) {
	fx := newLogoutFixture(t, &errPublisher{})

	_, err := fx.service.Logout(context.Background(), LogoutInput{
		SessionID:       fx.session.ID,
		DispatchRiderID: fx.session.DispatchRiderID,
		PhoneNumber:     fx.session.PhoneNumber,
		Role:            authmodels.RoleDispatchProvider,
		CorrelationID:   "logout-req-004",
	})
	requireLogoutErrorCode(t, err, apperrors.CodeInternal)
	if fx.sessions.sessions[fx.session.ID].RevokedAt == nil {
		t.Fatal("session should be revoked before publish error is returned")
	}
}

func TestLogoutMissingAuthContextReturnsUnauthorized(t *testing.T) {
	fx := newLogoutFixture(t, &capturePublisher{})

	_, err := fx.service.Logout(context.Background(), LogoutInput{
		SessionID:       "",
		DispatchRiderID: fx.session.DispatchRiderID,
	})
	requireLogoutErrorCode(t, err, apperrors.CodeUnauthorized)
}

func requireLogoutErrorCode(t *testing.T, err error, code apperrors.Code) {
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
