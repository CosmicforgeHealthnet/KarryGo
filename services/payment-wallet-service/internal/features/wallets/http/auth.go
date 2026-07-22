package wallethttp

import (
	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
)

const (
	providerIDKey   = "provider_id"
	providerTypeKey = "provider_type"
)

// canonicalProviderTypes maps a provider service (the token map key) to the
// provider_type used as the ledger owner-id prefix ("<type>:<providerID>").
//
// This MUST match the provider_type each service stamps on its payment intents
// when it settles earnings, or credited funds land under one owner key while the
// earnings/withdrawal endpoints read another (an always-empty account).
//
// Hauling signs its provider tokens under the service key "hauling" but settles
// under provider_type "truck_provider" (see driver-hauling-service .../payments/
// wallet_payment_client.go). Dispatch settles under "dispatch", which already
// equals its service key, so it (and any service not listed) passes through
// unchanged.
var canonicalProviderTypes = map[string]string{
	"hauling": "truck_provider",
}

// canonicalProviderType returns the ledger owner-id provider_type for a service.
func canonicalProviderType(service string) string {
	if mapped, ok := canonicalProviderTypes[service]; ok {
		return mapped
	}
	return service
}

func providerBearerMiddleware(secrets map[string][]byte) gin.HandlerFunc {
	signers := map[string]*sharedauth.TokenSigner{}
	for service, secret := range secrets {
		signers[service] = sharedauth.NewTokenSigner(secret)
	}

	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" {
			httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
			return
		}
		token := bearerToken(header)
		if token == "" {
			httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
			return
		}
		for service, signer := range signers {
			claims, err := signer.Verify(token)
			if err != nil {
				continue
			}
			// The signing secret and the service binding already prove this is a
			// provider access token issued by the matched provider service. Provider
			// apps use service-specific role strings (e.g. "truck_provider" for
			// hauling), so we require a non-empty role bound to the service rather
			// than a single hardcoded role value.
			if claims.Type == sharedauth.TokenTypeAccess && claims.Role != "" && claims.Service == service {
				c.Set(providerIDKey, claims.Subject)
				// Key the ledger owner by the canonical provider_type the owning
				// service settles under, not the raw service key — otherwise
				// hauling earnings (credited under "truck_provider") are read
				// under "hauling" and always show 0.
				c.Set(providerTypeKey, canonicalProviderType(service))
				c.Next()
				return
			}
		}
		httpx.Abort(c, apperrors.Forbidden("You do not have access to this action.", nil))
	}
}

func bearerToken(header string) string {
	const prefix = "Bearer "
	if len(header) <= len(prefix) || header[:len(prefix)] != prefix {
		return ""
	}
	return header[len(prefix):]
}

func providerIdentity(c *gin.Context) (string, string) {
	providerID, _ := c.Get(providerIDKey)
	providerType, _ := c.Get(providerTypeKey)
	return stringValue(providerType), stringValue(providerID)
}

func stringValue(value interface{}) string {
	if value == nil {
		return ""
	}
	text, _ := value.(string)
	return text
}
