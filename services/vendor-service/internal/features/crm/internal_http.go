package crm

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// InternalHandler receives authoritative CRM events from other services.
//
// This is the only write path into consent, privacy, identity and commercial
// projections. It is mounted at the service root rather than under
// /api/v1/vendor so it is never exposed through the public gateway, matching
// the Section 7 verification callback.
type InternalHandler struct {
	service *Service
}

func NewInternalHandler(service *Service) *InternalHandler { return &InternalHandler{service: service} }

// allowKnownCaller rejects any service that owns no CRM event type at all,
// before the body is even parsed. Per-event-type authority is checked again in
// the service layer, so a caller that owns one event type still cannot send
// another.
func (h *InternalHandler) allowKnownCaller(c *gin.Context) {
	raw, _ := c.Get("service_name")
	caller, _ := raw.(string)
	if caller == "notification-service" && c.FullPath() == "/internal/vendor/v1/crm/campaign-delivery-events" {
		c.Next()
		return
	}
	for _, authority := range EventRegistry {
		if containsString(authority.AllowedCallers, caller) {
			c.Next()
			return
		}
	}
	h.service.metrics.Inc("crm_internal_event_rejections_total", "reason", "unknown_caller")
	c.AbortWithStatus(http.StatusForbidden)
}

type deliveryEventRequest struct {
	EventID       string    `json:"event_id"`
	DeliveryRef   string    `json:"delivery_ref"`
	Status        string    `json:"status"`
	ReasonCode    string    `json:"reason_code"`
	SourceVersion int64     `json:"source_version"`
	OccurredAt    time.Time `json:"occurred_at"`
}

func (h *InternalHandler) PostCampaignDeliveryEvent(c *gin.Context) {
	raw, _ := c.Get("service_name")
	caller, _ := raw.(string)
	var req deliveryEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.RespondError(c, apperrors.BadRequest("Check your delivery event.", err))
		return
	}
	applied, err := h.service.IngestCampaignDeliveryEvent(c, caller, DeliveryEvent{EventID: req.EventID, DeliveryRef: req.DeliveryRef, Status: req.Status, ReasonCode: req.ReasonCode, SourceVersion: req.SourceVersion, OccurredAt: req.OccurredAt})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"event_id": req.EventID, "accepted": true, "applied": applied}})
}

// requireNonce enforces single-use request nonces, so a captured signed
// request cannot be replayed inside its signature validity window.
func (h *InternalHandler) requireNonce(c *gin.Context) {
	raw, _ := c.Get("service_name")
	nonce := c.GetHeader(serviceauth.HeaderNonce)
	if nonce == "" {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	tag, err := h.service.db.Exec(c, `INSERT INTO vendor_service_nonces(caller,nonce,expires_at)
		VALUES($1,$2,now()+interval '10 minutes') ON CONFLICT DO NOTHING`, raw, nonce)
	if err != nil || tag.RowsAffected() != 1 {
		h.service.metrics.Inc("crm_internal_event_rejections_total", "reason", "nonce_replay")
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	c.Next()
}

// PostEvent ingests one authoritative CRM event.
func (h *InternalHandler) PostEvent(c *gin.Context) {
	raw, _ := c.Get("service_name")
	caller, _ := raw.(string)

	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		httpx.RespondError(c, apperrors.BadRequest("Check your details.", err))
		return
	}
	var envelope EventEnvelope
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&envelope); err != nil {
		httpx.RespondError(c, apperrors.BadRequest("Check your details.", err))
		return
	}
	if err = decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		httpx.RespondError(c, apperrors.BadRequest("Request body must contain one JSON value.", err))
		return
	}

	if err = h.service.ProcessEvent(c, caller, envelope); err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"event_id": envelope.EventID, "accepted": true,
	}})
}

// RegisterInternalRoutes mounts the CRM event ingress at the service root.
//
// Registration is skipped when no service-auth verifier is configured, so an
// unauthenticated deployment cannot accidentally expose an unsigned write
// path into consent and privacy data.
func RegisterInternalRoutes(router *gin.Engine, handler *InternalHandler, verifier *serviceauth.Verifier, flags Flags) {
	if verifier == nil || !flags.CRMEnabled {
		return
	}
	group := router.Group("/internal/vendor/v1")
	group.Use(serviceauth.Middleware(verifier), handler.allowKnownCaller, handler.requireNonce)
	group.POST("/crm/events", handler.PostEvent)
	if flags.CampaignsEnabled && flags.CampaignExecutionEnabled {
		group.POST("/crm/campaign-delivery-events", handler.PostCampaignDeliveryEvent)
	}
}
