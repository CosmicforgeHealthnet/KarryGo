package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	TokenTypeAccess  = "access"
	TokenTypeRefresh = "refresh"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("expired token")
)

type Claims struct {
	Subject   string `json:"sub"`
	Role      string `json:"role"`
	Service   string `json:"service"`
	SessionID string `json:"session_id"`
	Type      string `json:"typ"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

type TokenSigner struct {
	secret []byte
	now    func() time.Time
}

func NewTokenSigner(secret []byte) *TokenSigner {
	return &TokenSigner{
		secret: secret,
		now:    time.Now,
	}
}

func (s *TokenSigner) Sign(claims Claims) (string, error) {
	if len(s.secret) == 0 {
		return "", fmt.Errorf("token secret is required")
	}
	if claims.Subject == "" || claims.Role == "" || claims.Service == "" || claims.SessionID == "" {
		return "", fmt.Errorf("subject, role, service, and session id are required")
	}
	if claims.Type == "" {
		claims.Type = TokenTypeAccess
	}
	if claims.IssuedAt == 0 {
		claims.IssuedAt = s.now().Unix()
	}
	if claims.ExpiresAt == 0 {
		return "", fmt.Errorf("token expiry is required")
	}

	header := map[string]string{"alg": "HS256", "typ": "JWT"}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	unsigned := encodeSegment(headerJSON) + "." + encodeSegment(claimsJSON)
	signature := signSegment(s.secret, unsigned)
	return unsigned + "." + signature, nil
}

func (s *TokenSigner) Verify(token string) (Claims, error) {
	var claims Claims
	if len(s.secret) == 0 {
		return claims, fmt.Errorf("token secret is required")
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return claims, ErrInvalidToken
	}

	unsigned := parts[0] + "." + parts[1]
	expected := signSegment(s.secret, unsigned)
	if !hmac.Equal([]byte(expected), []byte(parts[2])) {
		return claims, ErrInvalidToken
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return claims, ErrInvalidToken
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return claims, ErrInvalidToken
	}
	if claims.ExpiresAt <= s.now().Unix() {
		return claims, ErrExpiredToken
	}

	return claims, nil
}

func (s *TokenSigner) WithClock(now func() time.Time) *TokenSigner {
	s.now = now
	return s
}

func encodeSegment(value []byte) string {
	return base64.RawURLEncoding.EncodeToString(value)
}

func signSegment(secret []byte, unsigned string) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(unsigned))
	return encodeSegment(mac.Sum(nil))
}
