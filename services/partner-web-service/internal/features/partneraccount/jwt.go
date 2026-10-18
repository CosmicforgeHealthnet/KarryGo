package partneraccount

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// PartnerSessionClaims are the HS256 JWT claims for a dashboard session.
type PartnerSessionClaims struct {
	PartnerID string `json:"partner_id"`
	Sub       string `json:"sub"`
	Exp       int64  `json:"exp"`
	Iat       int64  `json:"iat"`
	Jti       string `json:"jti"`
}

// jwtHeader is the fixed HS256 header.
var jwtHeaderB64 = b64(`{"alg":"HS256","typ":"JWT"}`)

var errInvalidJWT = errors.New("invalid session token")

// signSessionJWT issues an HS256 session JWT and returns the token plus its
// expiry. PARTNER_JWT_SECRET is the signing key.
func signSessionJWT(secret, partnerID string, ttl time.Duration, now time.Time) (string, time.Time, error) {
	exp := now.Add(ttl)
	claims := PartnerSessionClaims{
		PartnerID: partnerID,
		Sub:       partnerID,
		Exp:       exp.Unix(),
		Iat:       now.Unix(),
		Jti:       uuid.NewString(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", time.Time{}, err
	}
	signingInput := jwtHeaderB64 + "." + base64.RawURLEncoding.EncodeToString(payload)
	sig := sign(secret, signingInput)
	return signingInput + "." + sig, exp, nil
}

// parseSessionJWT verifies the signature and expiry and returns the partner id.
func parseSessionJWT(secret, token string, now time.Time) (string, error) {
	// token = header.payload.signature
	firstDot := strings.IndexByte(token, '.')
	if firstDot < 0 {
		return "", errInvalidJWT
	}
	secondDot := strings.IndexByte(token[firstDot+1:], '.')
	if secondDot < 0 {
		return "", errInvalidJWT
	}
	secondDot += firstDot + 1

	signingInput := token[:secondDot]
	sig := token[secondDot+1:]

	expected := sign(secret, signingInput)
	if !hmac.Equal([]byte(sig), []byte(expected)) {
		return "", errInvalidJWT
	}

	payloadB64 := token[firstDot+1 : secondDot]
	payload, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return "", errInvalidJWT
	}
	var claims PartnerSessionClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", errInvalidJWT
	}
	if claims.PartnerID == "" || claims.Exp == 0 {
		return "", errInvalidJWT
	}
	if now.Unix() >= claims.Exp {
		return "", errInvalidJWT
	}
	return claims.PartnerID, nil
}

func sign(secret, signingInput string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signingInput))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// hashToken returns the SHA-256 hex of a raw JWT, used only for session lookup.
func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func b64(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
