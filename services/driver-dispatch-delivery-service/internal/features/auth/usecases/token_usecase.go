package authusecases

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	authmodels "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/models"
)

const (
	TokenTypeAccess  = "access"
	TokenTypeRefresh = "refresh"
	TokenService     = "dispatch"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("expired token")
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
