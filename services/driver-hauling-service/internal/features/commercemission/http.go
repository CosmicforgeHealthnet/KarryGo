package commercemission

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/hauling-service/internal/features/fleetauthority"
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
// hauling surface uses: HMAC v2, a caller allowlist, and single-use nonce
// replay protection.
//
// The allowlist matters as much as the signature. Creating a mission dispatches
// a real truck against real money, so a service holding a valid signature for
// some unrelated hauling integration must not be able to do it.
func Register(
	group *gin.RouterGroup,
	handler *Handler,
	db *pgxpool.Pool,
	verifier *serviceauth.Verifier,
	allowedCallers []string,
) {
	if handler == nil || handler.service == nil || verifier == nil || db == nil {
		return
	}

	missions := group.Group("/internal/commerce")
	missions.Use(fleetauthority.BoundaryWithDB(db, verifier, allowedCallers)...)
	missions.POST("/missions", handler.create)
	missions.GET("/missions/operations/:operation_ref", handler.read)
	missions.POST("/missions/operations/:operation_ref/cancel", handler.cancel)
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

func (h *Handler) read(c *gin.Context) {
	raw, _ := c.Get("service_name")
	caller, _ := raw.(string)
	if strings.TrimSpace(caller) != commercemission.SignatureCaller {
		httpx.Abort(c, apperrors.Unauthorized("This caller may not read commerce missions.", nil))
		return
	}
	mission, err := h.service.Read(c.Request.Context(), c.Param("operation_ref"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": mission})
}

func (h *Handler) cancel(c *gin.Context) {
	raw, _ := c.Get("service_name")
	caller, _ := raw.(string)
	if strings.TrimSpace(caller) != commercemission.SignatureCaller {
		httpx.Abort(c, apperrors.Unauthorized("This caller may not cancel commerce missions.", nil))
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxMissionBody+1))
	if err != nil || len(body) > maxMissionBody {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	var request commercemission.CancelRequest
	if err = decoder.Decode(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	var trailing any
	if err = decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		httpx.Abort(c, apperrors.BadRequest("Request body must contain exactly one JSON object.", err))
		return
	}
	if strings.TrimSpace(request.OperationRef) == "" {
		request.OperationRef = c.Param("operation_ref")
	}
	mission, err := h.service.Cancel(c.Request.Context(), request)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": mission})
}
