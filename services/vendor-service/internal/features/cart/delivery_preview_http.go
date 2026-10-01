package cart

import (
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"github.com/gin-gonic/gin"
)

// RegisterDeliveryPreviewRoute adds the non-committing read separately from
// cart commands. Its use case is composed over existing owner authorities.
func RegisterDeliveryPreviewRoute(root *gin.RouterGroup, signer *sharedauth.TokenSigner, handler gin.HandlerFunc) {
	if root == nil || signer == nil || handler == nil {
		return
	}
	root.GET("/carts/:cartId/delivery-preview", sharedauth.BearerMiddleware(signer, "customer", "customer"), handler)
}
