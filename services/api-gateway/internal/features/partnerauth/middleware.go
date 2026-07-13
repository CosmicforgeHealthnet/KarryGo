package partnerauth

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

const (
	// keyPrefixLength is the number of leading characters of a raw API key used
	// as the public, non-secret lookup prefix.
	keyPrefixLength = 12
	// minAPIKeyLength is the shortest length a plausible API key may have. Keys
	// shorter than this are rejected before any repository lookup.
	minAPIKeyLength = 16
)

// ExtractAPIKey pulls the raw API key from the request, preferring the
// X-API-Key header and falling back to an Authorization: Bearer token.
// The returned value is whitespace-trimmed. It returns "" when no key is present.
func ExtractAPIKey(c *gin.Context) string {
	if v := strings.TrimSpace(c.GetHeader("X-API-Key")); v != "" {
		return v
	}

	authz := strings.TrimSpace(c.GetHeader("Authorization"))
	const bearer = "Bearer "
	if len(authz) > len(bearer) && strings.EqualFold(authz[:len(bearer)], bearer) {
		return strings.TrimSpace(authz[len(bearer):])
	}

	return ""
}

// apiKeyError carries the status/code/message a failed API key check should
// respond with. It is exported (via AsAPIKeyError) so callers outside this
// package - specifically partneraccount's dashboard dual-auth middleware - can
// render the identical error shape without duplicating the status/code/message
// strings.
type apiKeyError struct {
	Status  int
	Code    string
	Message string
}

func (e *apiKeyError) Error() string { return e.Message }

var (
	errInvalidAPIKey = &apiKeyError{http.StatusUnauthorized, "invalid_api_key", "Your API key is invalid."}
	errAPIKeyRevoked = &apiKeyError{http.StatusForbidden, "api_key_revoked", "Your API key is no longer active."}
	errAPIKeyExpired = &apiKeyError{http.StatusUnauthorized, "api_key_expired", "This API key has expired."}
)

// AsAPIKeyError extracts the status/code/message from an error returned by
// VerifyAPIKey. ok is false if err did not come from VerifyAPIKey.
func AsAPIKeyError(err error) (status int, code, message string, ok bool) {
	e, matched := err.(*apiKeyError)
	if !matched {
		return 0, "", "", false
	}
	return e.Status, e.Code, e.Message, true
}

// VerifyAPIKey performs the full API key check - length, prefix lookup, bcrypt
// compare, status, environment, expiry, partner id - without touching the Gin
// context or writing any HTTP response. It is the single source of truth
// shared by RequirePartnerAPIKey (below) and by
// partneraccount.RequirePartnerSessionOrAPIKey, so both paths enforce
// identical rules instead of two copies that can drift.
//
// now is passed in explicitly (rather than calling time.Now() internally) so
// the expiry check is testable with a fixed clock, matching how
// partneraccount.Service already takes an injectable now func().
//
// Errors never reveal whether a given prefix exists: all failure paths return
// the same generic errInvalidAPIKey (except revoked/expired, which are
// intentionally distinguishable so a partner knows what to do about it).
func VerifyAPIKey(ctx context.Context, repo APIKeyRepository, raw string, now time.Time) (*APIKey, error) {
	if len(raw) < minAPIKeyLength {
		return nil, errInvalidAPIKey
	}

	prefix := raw[:keyPrefixLength]
	record, err := repo.FindByPrefix(ctx, prefix)
	if err != nil || record == nil {
		return nil, errInvalidAPIKey
	}

	if bcrypt.CompareHashAndPassword([]byte(record.KeyHash), []byte(raw)) != nil {
		return nil, errInvalidAPIKey
	}

	switch record.Status {
	case APIKeyStatusActive:
		// continue
	case APIKeyStatusRevoked:
		return nil, errAPIKeyRevoked
	default:
		return nil, errAPIKeyRevoked
	}

	if record.ExpiresAt != nil && !record.ExpiresAt.After(now) {
		return nil, errAPIKeyExpired
	}

	if record.Environment != EnvironmentSandbox && record.Environment != EnvironmentLive {
		return nil, errInvalidAPIKey
	}

	if record.PartnerID == "" {
		return nil, errInvalidAPIKey
	}

	return record, nil
}

// ApplyAPIKeyContext sets the partner identity context keys from a verified API
// key record, including its permission scopes. Exported so
// partneraccount.RequirePartnerSessionOrAPIKey sets the exact same context
// shape RequirePartnerAPIKey does below.
func ApplyAPIKeyContext(c *gin.Context, record *APIKey) {
	c.Set(ctxPartnerID, record.PartnerID)
	c.Set(ctxPartnerEnvironment, record.Environment)
	c.Set(ctxPartnerAPIKeyID, record.ID)
	c.Set(ctxPartnerAPIKeyPrefix, record.KeyPrefix)
	c.Set(ctxPartnerAPIKeyPermissions, record.Permissions)
}

// RecordAPIKeyUsage asynchronously updates last_used_at for a verified key. It
// recovers from any panic and never touches secrets. Exported for reuse by
// partneraccount's dual-auth middleware.
func RecordAPIKeyUsage(repo APIKeyRepository, keyID string) {
	go func() {
		defer func() { _ = recover() }()
		_ = repo.UpdateLastUsed(context.Background(), keyID)
	}()
}

// PartnerAPIKeyPermissions returns the authenticated API key's granted
// permission scopes, or nil if the request was not authenticated by an API key
// (e.g. dashboard session auth, or no auth middleware ran at all).
func PartnerAPIKeyPermissions(c *gin.Context) []string {
	v, ok := c.Get(ctxPartnerAPIKeyPermissions)
	if !ok {
		return nil
	}
	perms, _ := v.([]string)
	return perms
}

// RequirePermission gates a route to API keys holding AT LEAST ONE of the
// listed permissions (OR semantics - this is what lets GetDelivery accept
// either read_deliveries or track_deliveries, since this codebase has one
// endpoint doing both jobs).
//
// Session-authenticated dashboard requests are NEVER scope-checked here: if
// there is no API key id in context (partner_api_key_id unset), this
// middleware passes the request through unchecked. Permission scoping only
// makes sense for API keys that might be handed to a third party - a partner's
// own logged-in session always has full access to their own data.
//
// This must run AFTER RequirePartnerAPIKey or
// partneraccount.RequirePartnerSessionOrAPIKey in the middleware chain, since
// it reads context those set.
func RequirePermission(perms ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if PartnerAPIKeyID(c) == "" {
			c.Next()
			return
		}
		granted := PartnerAPIKeyPermissions(c)
		if !hasAnyPermission(granted, perms) {
			WriteError(c, http.StatusForbidden, "insufficient_scope",
				"This API key does not have permission for this action.")
			return
		}
		c.Next()
	}
}

func hasAnyPermission(granted, required []string) bool {
	set := make(map[string]struct{}, len(granted))
	for _, g := range granted {
		set[g] = struct{}{}
	}
	for _, r := range required {
		if _, ok := set[r]; ok {
			return true
		}
	}
	return false
}

// RequirePartnerAPIKey authenticates a Partner API request using a bcrypt-hashed
// API key looked up by its public prefix. On success it populates partner
// identity into the Gin context and records last-used asynchronously.
//
// Errors never reveal whether a given prefix exists: all lookup, bcrypt, and
// validity failures return the same generic invalid_api_key response.
func RequirePartnerAPIKey(repo APIKeyRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := ExtractAPIKey(c)
		if raw == "" {
			WriteError(c, http.StatusUnauthorized, "missing_api_key",
				"Provide your API key in the X-API-Key header or Authorization Bearer token.")
			return
		}

		record, err := VerifyAPIKey(c.Request.Context(), repo, raw, time.Now())
		if err != nil {
			status, code, message, ok := AsAPIKeyError(err)
			if !ok {
				status, code, message = http.StatusUnauthorized, "invalid_api_key", "Your API key is invalid."
			}
			WriteError(c, status, code, message)
			return
		}

		ApplyAPIKeyContext(c, record)
		RecordAPIKeyUsage(repo, record.ID)

		c.Next()
	}
}
