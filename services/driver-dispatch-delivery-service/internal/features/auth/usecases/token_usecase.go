package authusecases

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	authmodels "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/models"
)

const (
	TokenTypeAccess  = "access"
	TokenTypeRefresh = "refresh"
	TokenService     = "dispatch"
)

var (
	ErrInvalidToken              = errors.New("invalid token")
	ErrExpiredToken              = errors.New("expired token")
	ErrRevokedToken              = errors.New("revoked token")
	ErrTokenAuthorityUnavailable = errors.New("token revocation authority unavailable")
)

type DispatchRiderClaims struct {
	DispatchRiderID string `json:"dispatch_rider_id"`
	Subject         string `json:"sub"`
	PhoneNumber     string `json:"phone_number"`
	SessionID       string `json:"session_id"`
	SID             string `json:"sid"`
	JWTID           string `json:"jti"`
	Role            string `json:"role,omitempty"`
	Service         string `json:"service"`
	TokenType       string `json:"token_type"`
	Type            string `json:"typ"`
	IssuedAt        int64  `json:"iat"`
	ExpiresAt       int64  `json:"exp"`
}

type TokenUsecase struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
	now        func() time.Time
	redis      *redis.Client
}

func (u *TokenUsecase) WithRedis(client *redis.Client) *TokenUsecase {
	u.redis = client
	return u
}

func NewTokenUsecase(secret []byte, accessTTL time.Duration, refreshTTL time.Duration) *TokenUsecase {
	return &TokenUsecase{
		secret:     secret,
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
		now:        time.Now,
	}
}

func (u *TokenUsecase) GenerateAccessToken(dispatchRiderID string, phoneNumber string, sessionID string) (string, time.Time, error) {
	return u.generate(dispatchRiderID, phoneNumber, sessionID, TokenTypeAccess, u.accessTTL)
}

func (u *TokenUsecase) GenerateRefreshToken(dispatchRiderID string, phoneNumber string, sessionID string) (string, time.Time, error) {
	return u.generate(dispatchRiderID, phoneNumber, sessionID, TokenTypeRefresh, u.refreshTTL)
}

func (u *TokenUsecase) ValidateAccessToken(token string) (DispatchRiderClaims, error) {
	claims, err := u.validate(token)
	if err != nil {
		return DispatchRiderClaims{}, err
	}
	if claims.TokenType != TokenTypeAccess {
		return DispatchRiderClaims{}, ErrInvalidToken
	}
	return claims, nil
}

// ValidateAccessTokenContext also consults the revocation authority. Redis
// errors fail closed; a security decision is never guessed during an outage.
func (u *TokenUsecase) ValidateAccessTokenContext(ctx context.Context, token string) (DispatchRiderClaims, error) {
	claims, err := u.ValidateAccessToken(token)
	if err != nil {
		return DispatchRiderClaims{}, err
	}
	if u.redis == nil {
		return claims, nil
	}
	revoked, err := u.redis.Exists(ctx, revokedJTIKey(claims.JWTID)).Result()
	if err != nil {
		return DispatchRiderClaims{}, ErrTokenAuthorityUnavailable
	}
	if revoked != 0 {
		return DispatchRiderClaims{}, ErrRevokedToken
	}
	return claims, nil
}

// RevokeAccessToken stores only a hash of the JTI and lets Redis remove it at
// the token's natural expiry. No bearer token is persisted.
func (u *TokenUsecase) RevokeAccessToken(ctx context.Context, jti string, expiresAt int64) error {
	if u.redis == nil || strings.TrimSpace(jti) == "" {
		return nil
	}
	ttl := time.Until(time.Unix(expiresAt, 0))
	if ttl <= 0 {
		return nil
	}
	if err := u.redis.Set(ctx, revokedJTIKey(jti), "1", ttl).Err(); err != nil {
		return ErrTokenAuthorityUnavailable
	}
	return nil
}

func revokedJTIKey(jti string) string {
	sum := sha256.Sum256([]byte(jti))
	return "dispatch_rider_auth:revoked_jti:" + hex.EncodeToString(sum[:])
}

func (u *TokenUsecase) ValidateRefreshToken(token string) (DispatchRiderClaims, error) {
	claims, err := u.validate(token)
	if err != nil {
		return DispatchRiderClaims{}, err
	}
	if claims.TokenType != TokenTypeRefresh {
		return DispatchRiderClaims{}, ErrInvalidToken
	}
	return claims, nil
}

func (u *TokenUsecase) generate(dispatchRiderID string, phoneNumber string, sessionID string, tokenType string, ttl time.Duration) (string, time.Time, error) {
	now := u.now().UTC()
	expiresAt := now.Add(ttl)
	claims := DispatchRiderClaims{
		DispatchRiderID: dispatchRiderID,
		Subject:         dispatchRiderID,
		PhoneNumber:     phoneNumber,
		SessionID:       sessionID,
		SID:             sessionID,
		JWTID:           uuid.NewString(),
		Role:            authmodels.RoleDispatchProvider,
		Service:         TokenService,
		TokenType:       tokenType,
		Type:            tokenType,
		IssuedAt:        now.Unix(),
		ExpiresAt:       expiresAt.Unix(),
	}

	headerJSON, err := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	if err != nil {
		return "", time.Time{}, err
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", time.Time{}, err
	}

	unsigned := encodeJWTPart(headerJSON) + "." + encodeJWTPart(claimsJSON)
	return unsigned + "." + u.sign(unsigned), expiresAt, nil
}

func (u *TokenUsecase) validate(token string) (DispatchRiderClaims, error) {
	var claims DispatchRiderClaims
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return claims, ErrInvalidToken
	}

	unsigned := parts[0] + "." + parts[1]
	if !hmac.Equal([]byte(u.sign(unsigned)), []byte(parts[2])) {
		return claims, ErrInvalidToken
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return claims, ErrInvalidToken
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return claims, ErrInvalidToken
	}

	if claims.DispatchRiderID == "" || claims.PhoneNumber == "" || claims.SessionID == "" {
		return claims, ErrInvalidToken
	}
	if _, err := uuid.Parse(claims.JWTID); err != nil {
		return claims, ErrInvalidToken
	}
	if _, err := uuid.Parse(claims.DispatchRiderID); err != nil {
		return claims, ErrInvalidToken
	}
	if _, err := uuid.Parse(claims.SessionID); err != nil {
		return claims, ErrInvalidToken
	}
	if claims.TokenType != TokenTypeAccess && claims.TokenType != TokenTypeRefresh {
		return claims, ErrInvalidToken
	}
	if claims.ExpiresAt <= u.now().Unix() {
		return claims, ErrExpiredToken
	}

	return claims, nil
}

func (u *TokenUsecase) sign(unsigned string) string {
	mac := hmac.New(sha256.New, u.secret)
	_, _ = mac.Write([]byte(unsigned))
	return encodeJWTPart(mac.Sum(nil))
}

func (u *TokenUsecase) AccessTTLSeconds() int64 {
	return int64(u.accessTTL.Seconds())
}

func (u *TokenUsecase) WithClock(now func() time.Time) *TokenUsecase {
	u.now = now
	return u
}

func encodeJWTPart(value []byte) string {
	return base64.RawURLEncoding.EncodeToString(value)
}
