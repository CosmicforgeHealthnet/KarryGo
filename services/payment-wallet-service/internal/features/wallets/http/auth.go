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
				c.Set(providerTypeKey, service)
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
