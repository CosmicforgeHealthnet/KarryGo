package partnerauth

import "github.com/gin-gonic/gin"

// Gin context keys populated on successful partner authentication.
const (
	ctxPartnerID                = "partner_id"
	ctxPartnerEnvironment       = "partner_environment"
	ctxPartnerAPIKeyID          = "partner_api_key_id"
	ctxPartnerAPIKeyPrefix      = "partner_api_key_prefix"
	ctxPartnerAPIKeyPermissions = "partner_api_key_permissions"
)

// PartnerID returns the authenticated partner's ID, or "" if unauthenticated.
func PartnerID(c *gin.Context) string { return ctxString(c, ctxPartnerID) }

// PartnerEnvironment returns the authenticated key environment (sandbox/live), or "".
func PartnerEnvironment(c *gin.Context) string { return ctxString(c, ctxPartnerEnvironment) }

// PartnerAPIKeyID returns the authenticated API key ID, or "".
func PartnerAPIKeyID(c *gin.Context) string { return ctxString(c, ctxPartnerAPIKeyID) }

// PartnerAPIKeyPrefix returns the authenticated API key prefix, or "".
func PartnerAPIKeyPrefix(c *gin.Context) string { return ctxString(c, ctxPartnerAPIKeyPrefix) }

func ctxString(c *gin.Context, key string) string {
	v, ok := c.Get(key)
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}