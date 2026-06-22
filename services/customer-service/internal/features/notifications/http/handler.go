package notificationhttp

import (
	nethttp "net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/notifications"
)

const (
	recipientType   = notifications.RecipientCustomer
	defaultFeedSize = 50
	maxFeedSize     = 100
)

// Handler brokers customer-app notification access to notification-service. The
// app authenticates with its customer bearer token; this handler signs the
// downstream call with the service HMAC secret, so the app never holds it. The
// recipient is always the authenticated customer (token subject), so a customer
// can only ever read or act on their own notifications.
type Handler struct {
	client notifications.Client
}

func NewHandler(client notifications.Client) *Handler {
	return &Handler{client: client}
}

type registerDeviceRequest struct {
	Token    string `json:"token"`
	Platform string `json:"platform"`
	App      string `json:"app"`
}

// ListFeed returns the customer's recent notifications.
func (h *Handler) ListFeed(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}

	limit := defaultFeedSize
	if raw := c.Query("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > maxFeedSize {
		limit = maxFeedSize
	}

	messages, err := h.client.ListMessages(c.Request.Context(), recipientType, claims.Subject, limit)
	if err != nil {
		httpx.Abort(c, apperrors.Unavailable("Notifications are unavailable right now.", err))
		return
	}
	if messages == nil {
		messages = []map[string]interface{}{}
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": messages})
}

// RealtimeToken mints a short-lived websocket token for the customer.
func (h *Handler) RealtimeToken(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}

	result, err := h.client.MintRealtimeToken(c.Request.Context(), recipientType, claims.Subject)
	if err != nil {
		httpx.Abort(c, apperrors.Unavailable("Realtime notifications are unavailable right now.", err))
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": result})
}

// RegisterDevice stores the customer's push device token.
func (h *Handler) RegisterDevice(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}

	var req registerDeviceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("A device token is required.", err))
		return
	}
	if req.Token == "" {
		httpx.Abort(c, apperrors.BadRequest("A device token is required.", nil))
		return
	}

	if err := h.client.RegisterDevice(c.Request.Context(), notifications.DeviceInput{
		RecipientType: recipientType,
		RecipientID:   claims.Subject,
		Token:         req.Token,
		Platform:      req.Platform,
		App:           req.App,
	}); err != nil {
		httpx.Abort(c, apperrors.Unavailable("Could not register the device right now.", err))
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true})
}
