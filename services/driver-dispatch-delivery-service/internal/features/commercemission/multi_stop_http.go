package commercemission

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/authority"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/dispatchcommerce"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

const maxMultiStopMissionBody = 1 << 20

type MultiStopMissionHandler struct{ service *MultiStopMissionService }

func NewMultiStopMissionHandler(service *MultiStopMissionService) *MultiStopMissionHandler {
	return &MultiStopMissionHandler{service: service}
}

func RegisterMultiStopMissions(group *gin.RouterGroup, handler *MultiStopMissionHandler,
	verifier *serviceauth.Verifier, nonces authority.NonceClaimer, allowed []string) {
	if handler == nil || handler.service == nil || verifier == nil {
		return
	}
	missions := group.Group("/dispatch/v1/commerce/missions")
	missions.Use(authority.ServiceBoundary(verifier, nonces, allowed)...)
	missions.POST("", handler.create)
	missions.GET("/operations/:operationRef", handler.readOperation)
	missions.GET("/:missionRef", handler.readMission)
	missions.POST("/:missionRef/stops/:stopRef/verify-pickup", handler.verifyPickup)
	missions.POST("/:missionRef/cancel", handler.cancel)
	missions.POST("/:missionRef/reschedule", handler.reschedule)
	missions.POST("/:missionRef/return", handler.requireReturn)
	missions.POST("/:missionRef/provider-replacement", handler.replaceProvider)
}

func (h *MultiStopMissionHandler) cancel(c *gin.Context) { h.internalCommand(c, h.service.Cancel) }
func (h *MultiStopMissionHandler) reschedule(c *gin.Context) {
	h.internalCommand(c, h.service.Reschedule)
}
func (h *MultiStopMissionHandler) requireReturn(c *gin.Context) {
	h.internalCommand(c, h.service.RequireReturn)
}
func (h *MultiStopMissionHandler) replaceProvider(c *gin.Context) {
	h.internalCommand(c, h.service.ReplaceProvider)
}

func (h *MultiStopMissionHandler) internalCommand(c *gin.Context, command func(context.Context, string, string, dispatchcommerce.CommandRequest) (dispatchcommerce.Mission, error)) {
	var request dispatchcommerce.CommandRequest
	if err := decodeMissionBody(c, &request); err != nil {
		httpx.Abort(c, err)
		return
	}
	if header := strings.TrimSpace(c.GetHeader("Idempotency-Key")); header == "" || header != strings.TrimSpace(request.OperationRef) {
		httpx.Abort(c, apperrors.New(400, "IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key must equal operation_ref.", nil))
		return
	}
	mission, err := command(c.Request.Context(), missionCaller(c), c.Param("missionRef"), request)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, dispatchcommerce.Envelope[dispatchcommerce.Mission]{Success: true, Data: mission})
}

func (h *MultiStopMissionHandler) verifyPickup(c *gin.Context) {
	var request dispatchcommerce.PickupVerificationRequest
	if err := decodeMissionBody(c, &request); err != nil {
		httpx.Abort(c, err)
		return
	}
	if header := strings.TrimSpace(c.GetHeader("Idempotency-Key")); header == "" || header != strings.TrimSpace(request.OperationRef) {
		httpx.Abort(c, apperrors.New(400, "IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key must equal operation_ref.", nil))
		return
	}
	mission, err := h.service.VerifyPickup(c.Request.Context(), missionCaller(c), c.Param("missionRef"), c.Param("stopRef"), request)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, dispatchcommerce.Envelope[dispatchcommerce.Mission]{Success: true, Data: mission})
}

func (h *MultiStopMissionHandler) create(c *gin.Context) {
	caller := missionCaller(c)
	if caller != dispatchcommerce.SignatureCaller {
		httpx.Abort(c, apperrors.Unauthorized("This caller may not create Commerce missions.", nil))
		return
	}
	var request dispatchcommerce.MissionCreateRequest
	if err := decodeMissionBody(c, &request); err != nil {
		httpx.Abort(c, err)
		return
	}
	if header := strings.TrimSpace(c.GetHeader("Idempotency-Key")); header == "" || header != strings.TrimSpace(request.OperationRef) {
		httpx.Abort(c, apperrors.New(400, "IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key must equal operation_ref.", nil))
		return
	}
	mission, err := h.service.Create(c.Request.Context(), caller, request)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, dispatchcommerce.Envelope[dispatchcommerce.Mission]{Success: true, Data: mission})
}

func (h *MultiStopMissionHandler) readOperation(c *gin.Context) {
	mission, err := h.service.ReadOperation(c.Request.Context(), missionCaller(c), c.Param("operationRef"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, dispatchcommerce.Envelope[dispatchcommerce.Mission]{Success: true, Data: mission})
}
func (h *MultiStopMissionHandler) readMission(c *gin.Context) {
	mission, err := h.service.ReadMission(c.Request.Context(), missionCaller(c), c.Param("missionRef"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, dispatchcommerce.Envelope[dispatchcommerce.Mission]{Success: true, Data: mission})
}

func decodeMissionBody(c *gin.Context, target any) error {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxMultiStopMissionBody+1))
	if err != nil || len(body) > maxMultiStopMissionBody {
		return apperrors.BadRequest("Request body is invalid.", err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(target); err != nil {
		return apperrors.BadRequest("Request body is invalid.", err)
	}
	var trailing any
	if err = decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return apperrors.BadRequest("Request body must contain exactly one object.", err)
	}
	return nil
}
func missionCaller(c *gin.Context) string {
	raw, _ := c.Get("service_name")
	caller, _ := raw.(string)
	return strings.TrimSpace(caller)
}
