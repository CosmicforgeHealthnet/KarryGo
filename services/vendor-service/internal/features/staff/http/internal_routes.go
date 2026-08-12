package staffhttp

import (
	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"

	staffusecases "cosmicforge/logistics/services/vendor-service/internal/features/staff/usecases"
)

// RegisterInternalRoutes mounts the internal-authority staff-provisioning
// route, HMAC-protected via shared/go/serviceauth like every other internal
// route in the platform. This is how the very first staff member (Owner) for
// a vendor business is created — never through a public self-serve signup,
// since Section 5 does not own vendor business creation (see Section 6
// boundary). Today's only intended caller is an operator/internal tool;
// once Section 6 exists, its vendor-onboarding flow calls this same route.
func RegisterInternalRoutes(group *gin.RouterGroup, staff *staffusecases.StaffService, verifier *serviceauth.Verifier) {
	internalGroup := group.Group("/internal")
	internalGroup.Use(serviceauth.Middleware(verifier))
	internalGroup.POST("/staff/provision-owner", func(c *gin.Context) {
		var req provisionOwnerRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpx.RespondError(c, apperrors.BadRequest("Check your details.", err))
			return
		}
		result, err := staff.ProvisionOwner(c.Request.Context(), staffusecases.ProvisionOwnerInput{
			VendorID: req.VendorID, Phone: req.Phone, Email: req.Email,
		})
		if err != nil {
			httpx.RespondError(c, err)
			return
		}
		respondOK(c, toStaffView(result))
	})
}
