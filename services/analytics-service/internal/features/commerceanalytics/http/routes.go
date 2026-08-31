package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/analytics-service/internal/features/commerceanalytics/repositories"
	"cosmicforge/logistics/services/analytics-service/internal/features/commerceanalytics/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/commerceanalytics"
	"cosmicforge/logistics/shared/go/serviceauth"
)

type Handler struct {
	service        *usecases.Service
	verifier       *serviceauth.Verifier
	allowedCallers map[string]bool
	nonce          *ReplayCache
	limiter        RequestLimiter
	readRateMax    int
	readRateWindow time.Duration
}

type RequestLimiter interface {
	Allow(context.Context, string, string, int, time.Duration) error
}

func NewHandler(service *usecases.Service, verifier *serviceauth.Verifier, callers []string) *Handler {
	allowed := map[string]bool{}
	for _, caller := range callers {
		caller = strings.TrimSpace(caller)
		if caller != "" {
			allowed[caller] = true
		}
	}
	return &Handler{service: service, verifier: verifier, allowedCallers: allowed, nonce: NewReplayCache(10 * time.Minute)}
}

func (h *Handler) WithReadRateLimit(limiter RequestLimiter, max int, window time.Duration) *Handler {
	if h != nil {
		h.limiter, h.readRateMax, h.readRateWindow = limiter, max, window
	}
	return h
}

func RegisterRoutes(group *gin.RouterGroup, handler *Handler) {
	group.GET("/commerce/vendor-reports", handler.getReport)
	group.GET("/commerce/vendor-reliability", handler.getReliability)
}

func RegisterInternalRoutes(router *gin.Engine, handler *Handler) {
	router.POST("/internal/analytics/v1/commerce-events", handler.ingest)
}

func (h *Handler) ingest(c *gin.Context) {
	caller, ok := h.authorizeService(c)
	if !ok {
		return
	}
	var raw json.RawMessage
	if err := c.ShouldBindJSON(&raw); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"code": "validation_failed", "message": "invalid analytics event"}})
		return
	}
	event, err := normalizeEnvelope(raw)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"code": "validation_failed", "message": err.Error()}})
		return
	}
	if event.SourceService != caller {
		c.JSON(http.StatusForbidden, gin.H{"error": "source caller mismatch"})
		return
	}
	created, err := h.service.Ingest(c.Request.Context(), event)
	if errors.Is(err, repositories.ErrDuplicateEvent) {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"status": "duplicate", "created": false}})
		return
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"code": "analytics_event_rejected", "message": err.Error()}})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"success": true, "data": gin.H{"status": "processed", "created": created}})
}

type vendorEnvelope struct {
	EventID          string          `json:"eventId"`
	EventType        string          `json:"eventType"`
	SchemaVersion    int             `json:"schemaVersion"`
	AggregateType    string          `json:"aggregateType"`
	AggregateID      string          `json:"aggregateId"`
	AggregateVersion int64           `json:"aggregateVersion"`
	VendorID         string          `json:"vendorId"`
	CorrelationID    string          `json:"correlationId"`
	OccurredAt       time.Time       `json:"occurredAt"`
	Producer         string          `json:"producer"`
	Payload          json.RawMessage `json:"payload"`
}

func normalizeEnvelope(raw json.RawMessage) (commerceanalytics.Envelope, error) {
	var dispatch vendorEnvelope
	if err := json.Unmarshal(raw, &dispatch); err == nil && commerceanalytics.IsDispatchRouteEvent(dispatch.EventType) {
		var source struct {
			SourceService string `json:"sourceService"`
		}
		_ = json.Unmarshal(raw, &source)
		return commerceanalytics.Envelope{EventID: dispatch.EventID, EventType: dispatch.EventType, SchemaVersion: dispatch.SchemaVersion, OccurredAt: dispatch.OccurredAt, SourceService: source.SourceService, SourceAggregate: "dispatch_route_metric", SourceID: dispatch.EventID, SourceVersion: 1, Payload: dispatch.Payload}, nil
	}
	var event commerceanalytics.Envelope
	if err := json.Unmarshal(raw, &event); err != nil {
		return commerceanalytics.Envelope{}, err
	}
	if event.SourceService != "" {
		return event, nil
	}
	var vendor vendorEnvelope
	if err := json.Unmarshal(raw, &vendor); err != nil {
		return commerceanalytics.Envelope{}, err
	}
	return commerceanalytics.Envelope{
		EventID:         vendor.EventID,
		EventType:       vendor.EventType,
		SchemaVersion:   vendor.SchemaVersion,
		OccurredAt:      vendor.OccurredAt,
		SourceService:   vendor.Producer,
		SourceAggregate: vendor.AggregateType,
		SourceID:        vendor.AggregateID,
		SourceVersion:   vendor.AggregateVersion,
		BusinessID:      vendor.VendorID,
		VendorID:        vendor.VendorID,
		CorrelationID:   vendor.CorrelationID,
		Payload:         vendor.Payload,
	}, nil
}

func (h *Handler) getReport(c *gin.Context) {
	caller, ok := h.authorizeService(c)
	if !ok {
		return
	}
	businessID := c.Query("business_id")
	if businessID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"code": "validation_failed", "message": "business_id is required"}})
		return
	}
	if !h.authorizedAnalyticsScope(c, businessID) {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "error": gin.H{"code": "forbidden", "message": "analytics scope is not authorized"}})
		return
	}
	if !h.allowRead(c, caller, businessID) {
		return
	}
	start, end := parseWindowTimes(c)
	report, err := h.service.Report(c.Request.Context(), businessID, c.Query("branch_id"), c.DefaultQuery("window", "30D"), start, end)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"code": "validation_failed", "message": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": report})
}

func (h *Handler) getReliability(c *gin.Context) {
	caller, ok := h.authorizeService(c)
	if !ok {
		return
	}
	businessID := c.Query("business_id")
	if businessID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"code": "validation_failed", "message": "business_id is required"}})
		return
	}
	if !h.authorizedAnalyticsScope(c, businessID) {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "error": gin.H{"code": "forbidden", "message": "analytics scope is not authorized"}})
		return
	}
	if !h.allowRead(c, caller, businessID) {
		return
	}
	start, end := parseWindowTimes(c)
	score, err := h.service.Reliability(c.Request.Context(), businessID, c.Query("branch_id"), c.DefaultQuery("window", "30D"), start, end)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": gin.H{"code": "validation_failed", "message": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": score})
}

func (h *Handler) allowRead(c *gin.Context, caller, businessID string) bool {
	if h.limiter == nil || h.readRateMax <= 0 {
		return true
	}
	key := caller + ":" + strings.TrimSpace(businessID) + ":" + strings.TrimSpace(c.Query("branch_id"))
	if err := h.limiter.Allow(c.Request.Context(), "expensive_vendor_analytics", key, h.readRateMax, h.readRateWindow); err != nil {
		appErr := apperrors.From(err)
		c.JSON(appErr.Status, gin.H{"success": false, "error": gin.H{"code": appErr.Code, "message": appErr.Message}})
		return false
	}
	return true
}

func (h *Handler) authorizeService(c *gin.Context) (string, bool) {
	if h.verifier == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": gin.H{"code": "service_auth_not_configured", "message": "service authentication is not configured"}})
		return "", false
	}
	caller, err := h.verifier.VerifyRequest(c.Request)
	if err != nil || !h.allowedCallers[caller] {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": gin.H{"code": "unauthorized", "message": "invalid service signature"}})
		return "", false
	}
	if h.nonce != nil && !h.nonce.Consume(caller, c.GetHeader(serviceauth.HeaderNonce), time.Now().UTC()) {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": gin.H{"code": "replay_rejected", "message": "service nonce has already been used"}})
		return "", false
	}
	return caller, true
}

func (h *Handler) authorizedAnalyticsScope(c *gin.Context, businessID string) bool {
	caller := c.GetHeader(serviceauth.HeaderServiceName)
	if caller == "admin-backoffice-service" {
		return true
	}
	return c.GetHeader("X-Analytics-Business-ID") == businessID
}

type ReplayCache struct {
	mu   sync.Mutex
	ttl  time.Duration
	seen map[string]time.Time
}

func NewReplayCache(ttl time.Duration) *ReplayCache {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &ReplayCache{ttl: ttl, seen: map[string]time.Time{}}
}

func (r *ReplayCache) Consume(caller, nonce string, now time.Time) bool {
	if r == nil || nonce == "" {
		return false
	}
	key := caller + ":" + nonce
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, expires := range r.seen {
		if !expires.After(now) {
			delete(r.seen, k)
		}
	}
	if _, exists := r.seen[key]; exists {
		return false
	}
	r.seen[key] = now.Add(r.ttl)
	return true
}

func parseWindowTimes(c *gin.Context) (time.Time, time.Time) {
	start, _ := time.Parse(time.RFC3339, c.Query("start"))
	end, _ := time.Parse(time.RFC3339, c.Query("end"))
	return start, end
}
