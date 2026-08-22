package commercemission

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/authority"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/commercemission"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

const maxMissionBody = 32 << 10

// Handler serves the internal commerce mission endpoint.
type Handler struct {
	service *Service
}

// NewHandler builds the handler.
func NewHandler(service *Service) *Handler { return &Handler{service: service} }

// Register mounts mission creation behind the same three gates every private
// dispatch surface uses: HMAC v2, a caller allowlist, and single-use nonce
// replay protection.
//
// The allowlist matters as much as the signature. Creating a mission dispatches
// a real rider against real money, so a service holding a valid signature for
// some unrelated dispatch integration must not be able to do it.
func Register(
	group *gin.RouterGroup,
	handler *Handler,
	verifier *serviceauth.Verifier,
	nonces authority.NonceClaimer,
	allowedCallers []string,
) {
	if handler == nil || handler.service == nil || verifier == nil {
		return
	}

	missions := group.Group("/commerce")
	missions.Use(authority.ServiceBoundary(verifier, nonces, allowedCallers)...)
	missions.POST("/missions", handler.create)
}

func (h *Handler) create(c *gin.Context) {
	raw, _ := c.Get("service_name")
	caller, _ := raw.(string)

	if strings.TrimSpace(caller) != commercemission.SignatureCaller {
		httpx.Abort(c, apperrors.Unauthorized(
			"This caller may not create commerce missions.", nil))
		return
	}

	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxMissionBody+1))
	if err != nil || len(body) > maxMissionBody {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}

	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()

	var request commercemission.Request
	if err = decoder.Decode(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	var trailing any
	if err = decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		httpx.Abort(c, apperrors.BadRequest(
			"Request body must contain exactly one JSON object.", err))
		return
	}

	mission, err := h.service.Create(c.Request.Context(), request)
	if err != nil {
		httpx.Abort(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": mission})
}
