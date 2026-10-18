package partneraccount

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"cosmicforge/logistics/services/partner-web-service/internal/features/partner/delivery"
)

// The MAC authenticates stable revocation correlation even after the secret has
// rotated. A session UUID alone must never authorize logout. Refresh additionally
// requires the current secret digest in storage; a retired credential can only
// revoke, never authenticate. Domain separation keeps this proof distinct from JWTs.
const refreshProofDomain = "karrygo:partner-refresh:v1:"

func (s *Service) parseRefreshCredential(raw string) (sessionID, secret string, ok bool) {
	if len(raw) != 124 { // canonical UUID + two 32-byte base64url values + separators
		return "", "", false
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return "", "", false
	}
	id, err := uuid.Parse(parts[0])
	if err != nil || id.String() != parts[0] {
		return "", "", false
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	if err != nil || len(decoded) != 32 || !hmac.Equal([]byte(parts[2]), []byte(sign(s.jwtSecret, refreshProofDomain+parts[0]+"."+parts[1]))) {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func (s *Service) newCredentials(partnerID, sessionID string, meta RequestMeta) (*SessionResult, CreateSessionParams, error) {
	if sessionID == "" {
		sessionID = uuid.NewString()
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, CreateSessionParams{}, err
	}
	secret := base64.RawURLEncoding.EncodeToString(raw)
	payload := sessionID + "." + secret
	refresh := payload + "." + sign(s.jwtSecret, refreshProofDomain+payload)
	access, expiry, err := signSessionJWT(s.jwtSecret, partnerID, s.jwtTTL, s.now())
	if err != nil {
		return nil, CreateSessionParams{}, err
	}
	refreshExpiry := s.now().Add(s.refreshTTL)
	return &SessionResult{Token: access, ExpiresAt: expiry, RefreshToken: refresh, RefreshExpiresAt: refreshExpiry}, CreateSessionParams{
		ID: sessionID, PartnerID: partnerID, TokenHash: hashToken(access), RefreshHash: hashToken(secret), ExpiresAt: refreshExpiry, IPAddress: meta.IPAddress, UserAgent: meta.UserAgent}, nil
}

// Refresh rotates both credentials in the existing canonical partner_sessions
// row. The repository CAS permits one winner, and never reactivates a revoked row.
func (s *Service) Refresh(ctx context.Context, raw string, meta RequestMeta) (*SessionResult, error) {
	invalid := func() error { return apiErr(http.StatusUnauthorized, "invalid_session", "Session expired or invalid.") }
	sessionID, secret, valid := s.parseRefreshCredential(raw)
	if !valid {
		return nil, invalid()
	}
	hash := hashToken(secret)
	session, err := s.sessions.FindByRefreshHash(ctx, hash)
	if err != nil {
		return nil, delivery.ErrInternal()
	}
	if session == nil || session.ID != sessionID || session.RevokedAt != nil || !session.ExpiresAt.After(s.now()) {
		return nil, invalid()
	}
	account, err := s.accounts.FindByID(ctx, session.PartnerID)
	if err != nil {
		return nil, delivery.ErrInternal()
	}
	if account == nil || !isActive(account.Status) {
		return nil, invalid()
	}
	result, next, err := s.newCredentials(session.PartnerID, session.ID, meta)
	if err != nil {
		return nil, delivery.ErrInternal()
	}
	changed, err := s.sessions.Rotate(ctx, hash, next, s.now())
	if err != nil {
		return nil, delivery.ErrInternal()
	}
	if !changed {
		return nil, invalid()
	}
	result.Account = *account
	return result, nil
}

func (s *Service) LogoutRefresh(ctx context.Context, raw string) error {
	if raw == "" {
		return nil
	}
	sessionID, _, valid := s.parseRefreshCredential(raw)
	if !valid {
		return apiErr(http.StatusUnauthorized, "invalid_session", "Session expired or invalid.")
	}
	found, err := s.sessions.RevokeSession(ctx, sessionID)
	if err != nil {
		return delivery.ErrInternal()
	}
	if !found {
		return apiErr(http.StatusUnauthorized, "invalid_session", "Session expired or invalid.")
	}
	return nil
}
