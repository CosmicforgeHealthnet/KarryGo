package authusecases

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	authmodels "karrygo/services/driver-dispatch-delivery-service/internal/features/auth/models"
	authrepositories "karrygo/services/driver-dispatch-delivery-service/internal/features/auth/repositories"
	"karrygo/shared/go/apperrors"
)

const sessionCacheKeyPrefix = "dispatch_rider_auth:session:"

type SessionUsecase struct {
	repository authrepositories.SessionRepository
	redis      *redis.Client
	ttl        time.Duration
	now        func() time.Time
}

func NewSessionUsecase(repository authrepositories.SessionRepository, _ []byte, ttl time.Duration) *SessionUsecase {
	return &SessionUsecase{
		repository: repository,
		ttl:        ttl,
		now:        time.Now,
	}
}

// WithRedis attaches a Redis client for session caching.
// Cache writes are non-fatal; DB remains the source of truth.
func (u *SessionUsecase) WithRedis(rc *redis.Client) *SessionUsecase {
	u.redis = rc
	return u
}

func (u *SessionUsecase) GenerateSecureToken(byteLength int) (string, error) {
	if byteLength <= 0 {
		byteLength = 32
	}

	raw := make([]byte, byteLength)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}

	return hex.EncodeToString(raw), nil
}

func (u *SessionUsecase) HashRefreshToken(refreshToken string) string {
	sum := sha256.Sum256([]byte(refreshToken))
	return hex.EncodeToString(sum[:])
}

func (u *SessionUsecase) Create(ctx context.Context, sessionID string, dispatchRiderID string, phoneNumber string, refreshToken string, metadata RequestMetadata) (authmodels.Session, error) {
	session := authmodels.Session{
		ID:               sessionID,
		DispatchRiderID:  dispatchRiderID,
		PhoneNumber:      phoneNumber,
		RefreshTokenHash: u.HashRefreshToken(refreshToken),
		DeviceID:         metadata.DeviceID,
		DeviceType:       metadata.DeviceType,
		IPAddress:        metadata.IPAddress,
		UserAgent:        metadata.UserAgent,
		ExpiresAt:        u.now().Add(u.ttl),
	}

	saved, err := u.repository.Create(ctx, session)
	if err != nil {
		return authmodels.Session{}, err
	}

	// Cache session in Redis for fast lookup.
	// Key: dispatch_rider_auth:session:{session_id}  TTL: same as session lifetime.
	// Cache errors are non-fatal; the DB is the authoritative source of truth.
	if u.redis != nil {
		if data, jsonErr := json.Marshal(saved); jsonErr == nil {
			_ = u.redis.Set(ctx, sessionCacheKeyPrefix+saved.ID, data, u.ttl).Err()
		}
	}

	return saved, nil
}

func (u *SessionUsecase) ValidateRefreshToken(ctx context.Context, refreshToken string) (authmodels.Session, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return authmodels.Session{}, refreshSessionUnauthorized()
	}

	session, ok, err := u.repository.FindByRefreshTokenHash(ctx, u.HashRefreshToken(refreshToken))
	if err != nil {
		return authmodels.Session{}, err
	}
	if !ok {
		return authmodels.Session{}, refreshSessionUnauthorized()
	}
	if session.RevokedAt != nil {
		return authmodels.Session{}, refreshSessionUnauthorized()
	}
	if !session.ExpiresAt.After(u.now()) {
		return authmodels.Session{}, refreshSessionUnauthorized()
	}

	return session, nil
}

func (u *SessionUsecase) RotateRefreshToken(ctx context.Context, sessionID string, refreshToken string) error {
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(refreshToken) == "" {
		return refreshSessionUnauthorized()
	}

	if err := u.repository.RotateRefreshToken(ctx, sessionID, u.HashRefreshToken(refreshToken)); err != nil {
		if errors.Is(err, authrepositories.ErrSessionNotFound) {
			return refreshSessionUnauthorized()
		}
		return apperrors.Internal("Session could not be updated.", err)
	}

	if u.redis != nil {
		_ = u.redis.Del(ctx, sessionCacheKeyPrefix+sessionID).Err()
	}

	return nil
}

func (u *SessionUsecase) Revoke(ctx context.Context, sessionID string) error {
	if strings.TrimSpace(sessionID) == "" {
		return logoutSessionUnauthorized()
	}

	if err := u.repository.Revoke(ctx, sessionID); err != nil {
		if errors.Is(err, authrepositories.ErrSessionNotFound) {
			return logoutSessionUnauthorized()
		}
		return apperrors.Internal("Session could not be revoked.", err)
	}

	return nil
}

func (u *SessionUsecase) WithClock(now func() time.Time) *SessionUsecase {
	u.now = now
	return u
}
