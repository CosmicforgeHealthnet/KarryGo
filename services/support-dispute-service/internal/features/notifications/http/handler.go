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
	defaultFeedSize = 50
	maxFeedSize     = 100
)

// Handler brokers support-app notification access (feed, realtime token, device
// registration) to notification-service. The app authenticates with its bearer
// token; this handler signs the downstream call with the service HMAC secret.
// The recipient is always the authenticated subject, with the recipient type
// fixed per handler instance (customer or provider).
type Handler struct {
	client        notifications.Client
	recipientType string
}

func NewHandlerFor(client notifications.Client, recipientType string) *Handler {
	return &Handler{client: client, recipientType: recipientType}
}

type registerDeviceRequest struct {
	Token    string `json:"token"`
	Platform string `json:"platform"`
	App      string `json:"app"`
}

// ListFeed returns the caller's recent notifications.
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

	messages, err := h.client.ListMessages(c.Request.Context(), h.recipientType, claims.Subject, limit)
	if err != nil {
		httpx.Abort(c, apperrors.Unavailable("Notifications are unavailable right now.", err))
		return
	}
	if messages == nil {
		messages = []map[string]interface{}{}
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": messages})
}

// RealtimeToken mints a short-lived websocket token for the caller (live chat).
func (h *Handler) RealtimeToken(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}

	result, err := h.client.MintRealtimeToken(c.Request.Context(), h.recipientType, claims.Subject)
	if err != nil {
		httpx.Abort(c, apperrors.Unavailable("Realtime notifications are unavailable right now.", err))
		return
	}
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": result})
}

// RegisterDevice stores the caller's push device token.
func (h *Handler) RegisterDevice(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}

	var req registerDeviceRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Token == "" {
		httpx.Abort(c, apperrors.BadRequest("A device token is required.", err))
		return
	}

	if err := h.client.RegisterDevice(c.Request.Context(), notifications.DeviceInput{
		RecipientType: h.recipientType,
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
