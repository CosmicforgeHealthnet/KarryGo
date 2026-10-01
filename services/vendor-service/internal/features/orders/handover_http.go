package orders

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	handover "cosmicforge/logistics/services/vendor-service/internal/integrations/handover"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"github.com/gin-gonic/gin"
)

func registerHandoverRoutes(root *gin.RouterGroup, s *Service) {
	if len(s.handover) == 0 {
		return
	}
	// A sibling of the generic command group: its middleware hashes request
	// bodies and must never process a custody credential.
	g := root.Group("/commerce/orders")
	g.GET("/:vendorOrderId/missions/:missionId/handover/:stage", func(c *gin.Context) {
		a, ok := vendorActor(c)
		if !ok {
			return
		}
		o, err := s.ReadHandover(c.Request.Context(), a, c.Param("vendorOrderId"), c.Param("missionId"), c.Param("stage"))
		respondHandover(c, o, err)
	})
	g.POST("/:vendorOrderId/missions/:missionId/handover/pickup/confirm", func(c *gin.Context) {
		a, ok := vendorActor(c)
		if !ok {
			return
		}
		var input struct {
			HandoverRef string `json:"handover_ref"`
			Method      string `json:"method"`
			Credential  string `json:"credential"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 8<<10))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil {
			respondHandover(c, handover.Observation{}, handover.ErrInvalid)
			return
		}
		var trailing any
		if !errors.Is(decoder.Decode(&trailing), io.EOF) {
			respondHandover(c, handover.Observation{}, handover.ErrInvalid)
			return
		}
		o, err := s.ConfirmPickupHandover(c.Request.Context(), a, c.Param("vendorOrderId"), c.Param("missionId"), handover.Confirmation{HandoverRef: input.HandoverRef, Method: input.Method, Credential: input.Credential, OperationRef: c.GetHeader("Idempotency-Key")})
		respondHandover(c, o, err)
	})
}

func respondHandover(c *gin.Context, o handover.Observation, err error) {
	if err == nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": o})
		return
	}
	status, code := http.StatusServiceUnavailable, "HANDOVER_NOT_AVAILABLE"
	switch {
	case errors.Is(err, handover.ErrInvalid):
		status, code = http.StatusBadRequest, "HANDOVER_SCOPE_INVALID"
	case errors.Is(err, ErrForbidden):
		status, code = http.StatusForbidden, "FORBIDDEN"
	case errors.Is(err, ErrOrderNotFound):
		status, code = http.StatusNotFound, "NOT_FOUND"
	case errors.Is(err, handover.ErrRejected), errors.Is(err, ErrOrderConflict):
		status, code = http.StatusConflict, "HANDOVER_REJECTED"
	case errors.Is(err, handover.ErrReceiverSecureChannelRequired):
		status, code = http.StatusPreconditionFailed, "RECEIVER_SECURE_CHANNEL_REQUIRED"
	case errors.Is(err, handover.ErrUnknown):
		code = "HANDOVER_OUTCOME_UNKNOWN"
	}
	// Never reflect arbitrary upstream errors, request bodies or credentials.
	httpx.RespondError(c, apperrors.New(status, apperrors.Code(code), "Canonical handover confirmation is required.", nil))
}
