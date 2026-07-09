package partnerauth

import (
	"context"
	"net/http"
	"strings"

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

		if len(raw) < minAPIKeyLength {
			writeInvalidAPIKey(c)
			return
		}

		prefix := raw[:keyPrefixLength]
		record, err := repo.FindByPrefix(c.Request.Context(), prefix)
		if err != nil || record == nil {
			// Do not distinguish between "not found" and lookup error, and do not
			// reveal whether the prefix existed.
			writeInvalidAPIKey(c)
			return
		}

		if bcrypt.CompareHashAndPassword([]byte(record.KeyHash), []byte(raw)) != nil {
			writeInvalidAPIKey(c)
			return
		}

		switch record.Status {
		case APIKeyStatusActive:
			// continue
		case APIKeyStatusRevoked:
			WriteError(c, http.StatusForbidden, "api_key_revoked", "Your API key has been revoked.")
			return
		default:
			WriteError(c, http.StatusForbidden, "api_key_revoked_or_inactive",
				"Your API key is no longer active.")
			return
		}

		if record.Environment != EnvironmentSandbox && record.Environment != EnvironmentLive {
			writeInvalidAPIKey(c)
			return
		}

		if record.PartnerID == "" {
			writeInvalidAPIKey(c)
			return
		}

		c.Set(ctxPartnerID, record.PartnerID)
		c.Set(ctxPartnerEnvironment, record.Environment)
		c.Set(ctxPartnerAPIKeyID, record.ID)
		c.Set(ctxPartnerAPIKeyPrefix, record.KeyPrefix)

		// Record usage without blocking or failing the request. The goroutine
		// recovers from any panic and never touches secrets.
		keyID := record.ID
		go func() {
			defer func() { _ = recover() }()
			_ = repo.UpdateLastUsed(context.Background(), keyID)
		}()

		c.Next()
	}
}

func writeInvalidAPIKey(c *gin.Context) {
	WriteError(c, http.StatusUnauthorized, "invalid_api_key", "Your API key is invalid.")
}
