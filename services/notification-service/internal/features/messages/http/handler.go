package messagehttp

import (
	nethttp "net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	messageclients "cosmicforge/logistics/services/notification-service/internal/features/messages/clients"
	messageusecases "cosmicforge/logistics/services/notification-service/internal/features/messages/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/notifications"
)

type Handler struct {
	notifications *messageusecases.NotificationService
	hub           *messageclients.WebSocketHub
}

func NewHandler(notifications *messageusecases.NotificationService, hub *messageclients.WebSocketHub) *Handler {
	return &Handler{notifications: notifications, hub: hub}
}

func (h *Handler) Send(c *gin.Context) {
	var request notifications.Request
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}

	result, err := h.notifications.Send(c.Request.Context(), request)
	if err != nil {
		httpx.Abort(c, err)
		return
	}

	respond(c, nethttp.StatusAccepted, result)
}

func (h *Handler) RegisterDevice(c *gin.Context) {
	var request deviceRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}

	device, err := h.notifications.RegisterDevice(c.Request.Context(), messageusecases.RegisterDeviceInput{
		RecipientType: request.RecipientType,
		RecipientID:   request.RecipientID,
		Token:         request.Token,
		Platform:      request.Platform,
		App:           request.App,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}

	respond(c, nethttp.StatusCreated, device)
}

func (h *Handler) RealtimeToken(c *gin.Context) {
	var request realtimeTokenRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}

	result, err := h.notifications.RealtimeToken(messageusecases.RealtimeTokenInput{
		RecipientType: request.RecipientType,
		RecipientID:   request.RecipientID,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}

	respondOK(c, result)
}

func (h *Handler) WebSocket(c *gin.Context) {
	token := c.Query("token")
	recipientType, recipientID, err := h.notifications.VerifyRealtimeToken(token)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	if err := h.hub.ServeHTTP(c.Writer, c.Request, recipientType, recipientID); err != nil {
		httpx.Abort(c, apperrors.BadRequest("WebSocket upgrade failed.", err))
		return
	}
}

func (h *Handler) GetMessage(c *gin.Context) {
	message, err := h.notifications.GetMessage(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}

	respondOK(c, message)
}

func (h *Handler) ListMessages(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	messages, err := h.notifications.ListMessages(c.Request.Context(), c.Query("recipient_type"), c.Query("recipient_id"), limit)
	if err != nil {
		httpx.Abort(c, err)
		return
	}

	respondOK(c, messages)
}
