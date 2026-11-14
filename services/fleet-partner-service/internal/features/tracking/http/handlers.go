// Package http serves the Section 16 tracking-projection routes: the
// corporate reads (/tracking...) and the HMAC-authenticated ingestion of
// authoritative Dispatch/Hauling events (POST /tracking/events).
package http

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	dispatchmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/dispatch/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/tracking/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/section16http"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

const PermissionTrackingRead = "tracking.read"

// Route is one route manifest entry.
type Route struct {
	Method, Path, OperationID, Permission, FeatureFlag, AuthClass string
	IdempotencyRequired, ExpectedVersionRequired                  bool
}

func TenantRoutes() []Route {
	return []Route{
		{"GET", "/tracking", "listTrackingProjections", PermissionTrackingRead, "TRACKING_PROJECTIONS_ENABLED", "corporate", false, false},
		{"GET", "/tracking/:canonicalTripReference", "getTrackingProjection", PermissionTrackingRead, "TRACKING_PROJECTIONS_ENABLED", "corporate", false, false},
	}
}
func InternalRoutes() []Route {
	return []Route{{"POST", "/tracking/events", "ingestTrackingEvent", "tracking.events.ingest", "TRACKING_PROJECTIONS_ENABLED", "internal-hmac-v2", true, false}}
}

// Service is the use-case surface the tracking handlers call.
type Service interface {
	ListTracking(ctx context.Context, t tenant.Context, f dispatchmodels.PageFilter) ([]models.TrackingProjection, string, error)
	GetTracking(ctx context.Context, t tenant.Context, trip string) (models.TrackingProjection, error)
	IngestTracking(ctx context.Context, e models.TrackingEvent) (models.TrackingProjection, string, error)
}

type Handler struct {
	s Service
	k section16http.Kit
}

func NewHandler(s Service, a *accesscontrol.Authorizer) *Handler {
	k := section16http.Kit{Conflict: dispatchmodels.ErrConflict}
	if a != nil {
		k.Authorizer = a
	}
	return &Handler{s: s, k: k}
}
func (h *Handler) TenantRoutes() []Route   { return TenantRoutes() }
func (h *Handler) InternalRoutes() []Route { return InternalRoutes() }
func (h *Handler) RegisterTenantRoutes(g *gin.RouterGroup) {
	handlers := map[string]gin.HandlerFunc{"listTrackingProjections": h.listTracking, "getTrackingProjection": h.getTracking}
	for _, r := range TenantRoutes() {
		g.Handle(r.Method, r.Path, handlers[r.OperationID])
	}
}
func (h *Handler) RegisterInternalRoutes(g *gin.RouterGroup) {
	g.POST("/tracking/events", h.ingestTracking)
}

func (h *Handler) listTracking(c *gin.Context) {
	t, ok := h.k.Gate(c, PermissionTrackingRead)
	if !ok {
		return
	}
	f, e := parseFilter(c)
	if e != nil {
		respond.Error(c, e)
		return
	}
	v, next, e := h.s.ListTracking(c, t, f)
	h.k.WriteList(c, v, next, e)
}
func (h *Handler) getTracking(c *gin.Context) {
	t, ok := h.k.Gate(c, PermissionTrackingRead)
	if !ok {
		return
	}
	v, e := h.s.GetTracking(c, t, c.Param("canonicalTripReference"))
	h.k.Write(c, v, e)
}
func (h *Handler) ingestTracking(c *gin.Context) {
	if strings.TrimSpace(c.GetHeader("Idempotency-Key")) == "" {
		respond.Error(c, apperrors.BadRequest("Idempotency-Key header is required.", nil))
		return
	}
	caller, _ := c.Get("service_name")
	source, _ := caller.(string)
	if source != "driver-dispatch-delivery-service" && source != "driver-hauling-service" {
		respond.Error(c, apperrors.Forbidden("Authoritative tracking caller required.", nil))
		return
	}
	var b models.TrackingEvent
	if !section16http.Bind(c, &b) {
		return
	}
	b.SourceService = source
	v, d, e := h.s.IngestTracking(c, b)
	if e != nil {
		respond.Error(c, e)
		return
	}
	respond.OK(c, gin.H{"projection": v, "disposition": d})
}

// parseFilter reads the shared list filter. Malformed, negative or oversized
// limit/offset values, a malformed cursor and a cursor sent with an offset
// are a validation error rather than a SQL failure.
func parseFilter(c *gin.Context) (dispatchmodels.PageFilter, error) {
	page, e := section16http.ParsePage(c)
	if e != nil {
		return dispatchmodels.PageFilter{}, e
	}
	return dispatchmodels.PageFilter{BranchID: c.Query("branchId"), Status: strings.ToUpper(c.Query("status")), Service: strings.ToUpper(c.Query("service")), ProviderID: c.Query("providerId"), VehicleReference: c.Query("vehicleOrTruckReference"), AlertType: strings.ToUpper(c.Query("alertType")), Limit: page.Limit, Offset: page.Offset, After: page.After, AfterID: page.AfterID}, nil
}
