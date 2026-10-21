package providerapprovalhttp

import (
	"log"
	nethttp "net/http"

	"github.com/gin-gonic/gin"

	providerapprovalusecases "cosmicforge/logistics/services/taxi-service/internal/features/provider_approval/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// ApproverService is the only caller allowed to approve drivers.
const ApproverService = "admin-backoffice-service"

type approveRequest struct {
	ProviderID string `json:"provider_id"`
	Phone      string `json:"phone"`
	// Note says why (e.g. "documents checked by Ada"); it is logged.
	Note string `json:"note"`
}

// RegisterRoutes mounts POST /internal/admin/providers/approve behind service
// signatures from ApproverService. Skipped when no verifier is configured.
func RegisterRoutes(group *gin.RouterGroup, svc *providerapprovalusecases.Service, verifier *serviceauth.Verifier) {
	if verifier == nil {
		return
	}
	group.POST("/internal/admin/providers/approve", serviceauth.Middleware(verifier), func(c *gin.Context) {
		if c.GetString("service_name") != ApproverService {
			httpx.Abort(c, apperrors.Forbidden("This caller cannot approve drivers.", nil))
			return
		}
		var req approveRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
			return
		}
		d, err := svc.Approve(c.Request.Context(), providerapprovalusecases.ApproveInput{ProviderID: req.ProviderID, Phone: req.Phone})
		if err != nil {
			httpx.Abort(c, err)
			return
		}
		log.Printf("driver approved: provider=%s note=%q", d.ID, req.Note)
		c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": d})
	})
}
