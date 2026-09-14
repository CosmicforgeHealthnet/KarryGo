package logisticscapability

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Provider is implemented by an authority that can declare its capability.
//
// The authority computes this from its OWN state (feature flags, configured
// integrations, real vehicle classes), never from a constant this package
// supplies — a shared default would be exactly the kind of inferred truth
// Section 11 forbids.
type Provider interface {
	DeclaredCapability() Capability
}

// ProviderFunc adapts a function to Provider.
type ProviderFunc func() Capability

// DeclaredCapability satisfies Provider.
func (f ProviderFunc) DeclaredCapability() Capability { return f() }

// RegisterRoutes mounts the capability and serviceability reads on an
// authority's already-authenticated internal commerce group.
//
// It deliberately takes a group the caller has ALREADY protected with its own
// HMAC/allowlist/replay middleware rather than installing auth itself: each
// service's internal boundary differs (nonce stores, caller allowlists), and
// a shared package quietly installing a weaker one is how a surface ends up
// less protected than its neighbours.
func RegisterRoutes(group *gin.RouterGroup, provider Provider) {
	if group == nil || provider == nil {
		return
	}
	group.GET(Path, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": provider.DeclaredCapability()})
	})
	group.POST(ServiceabilityPath, func(c *gin.Context) {
		var requirement Requirement
		// An empty body is a valid "can you do a plain single-pickup,
		// single-dropoff mission" question, so a decode failure on no content
		// must not become a 400.
		if c.Request.ContentLength > 0 {
			if err := c.ShouldBindJSON(&requirement); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false,
					"error": gin.H{"code": "validation_failed", "message": "Requirement is invalid."}})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"success": true,
			"data": Evaluate(provider.DeclaredCapability(), requirement)})
	})
}
