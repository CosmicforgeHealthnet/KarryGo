// Package http exposes the marketplace HTTP surface.
//
// The routes here are INTERNAL: they are called by other KarryGo services over
// authenticated HTTP, never by a browser, a provider app or a corporate user.
// Provider and corporate traffic reaches this service through its own
// authenticated route classes.
package http

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/usecases"
)

// InternalHandler serves service-to-service marketplace routes.
type InternalHandler struct {
	service *usecases.Service
	// allowedCallers is the per-endpoint authorization gate. Service
	// authentication proves WHO is calling; this decides whether that caller
	// may reach this particular route.
	allowedCallers map[string]bool
}

// NewInternalHandler builds the handler.
func NewInternalHandler(service *usecases.Service, allowedCallers []string) *InternalHandler {
	permitted := make(map[string]bool, len(allowedCallers))
	for _, name := range allowedCallers {
		permitted[name] = true
	}
	return &InternalHandler{service: service, allowedCallers: permitted}
}

// RegisterInternalMarketplaceRoutes mounts the internal marketplace routes.
//
// The caller is expected to have already applied service authentication to the
// group; this adds the per-route caller allowlist on top.
func RegisterInternalMarketplaceRoutes(group *gin.RouterGroup, handler *InternalHandler) {
	group.GET("/assignments/:assignmentId/provider-acceptance",
		handler.requireAllowedCaller, handler.GetProviderAcceptance)
	group.GET("/assignments/:assignmentId/provider-decision-receipt",
		handler.requireAllowedCaller, handler.GetProviderDecisionReceipt)
}

func (h *InternalHandler) GetProviderDecisionReceipt(c *gin.Context) {
	service, err := h.callerProviderService(c)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	values, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err != nil {
		httpx.Abort(c, apperrors.BadRequest("Read-back query is invalid.", nil))
		return
	}
	if c.Request.ContentLength != 0 || len(c.Request.TransferEncoding) > 0 {
		err = apperrors.BadRequest("Read-back cannot contain a body.", nil)
	}
	for key, items := range values {
		if (key != "providerId" && key != "clientResponseId") || len(items) != 1 {
			err = apperrors.BadRequest("Read-back query is invalid.", nil)
		}
	}
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	receipt, err := h.service.ProviderDecisionReceipt(c.Request.Context(), c.Param("assignmentId"), values.Get("providerId"), providerServiceTokens[service], values.Get("clientResponseId"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": receipt})
}

// requireAllowedCaller enforces the per-endpoint service allowlist.
func (h *InternalHandler) requireAllowedCaller(c *gin.Context) {
	// The name comes from the verified signature set by the service-auth
	// middleware — never from a header or body the caller controls directly.
	raw, _ := c.Get("service_name")
	serviceName, _ := raw.(string)
	if serviceName == "" || !h.allowedCallers[serviceName] {
		httpx.Abort(c, apperrors.Forbidden("Service is not allowed to call this endpoint.", nil))
		return
	}
	c.Next()
}

// GetProviderAcceptance tells an execution service whether the assigned
// provider has actually accepted, and whether that acceptance is still valid.
//
// This is a READ-ONLY endpoint. It deliberately offers no way for the calling
// service to change assignment state: dispatch may ask whether a provider
// accepted, but it may never record, alter or infer that acceptance.
func (h *InternalHandler) GetProviderAcceptance(c *gin.Context) {
	view, err := h.service.VerifyProviderAcceptance(c.Request.Context(), c.Param("assignmentId"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": view, "meta": gin.H{
		"requestId": c.GetHeader("X-Request-ID"),
	}})
}

// RegisterProviderReadRoutes mounts the read-only provider views used by an
// execution service to render a provider's offers and assignments.
//
// Exact routes:
//
//	GET /internal/fleet-partner/v1/providers/:providerId/offers
//	GET /internal/fleet-partner/v1/providers/:providerId/offers/:offerId
//	GET /internal/fleet-partner/v1/providers/:providerId/assignments
//	GET /internal/fleet-partner/v1/providers/:providerId/assignments/:assignmentId
//	GET /internal/fleet-partner/v1/providers/:providerId/memberships
//
// providerId in the path is NOT self-asserted trust: the calling service has
// already authenticated the provider's session and is vouching for it, and
// fleet independently scopes every query to that provider through the
// membership join. A caller may only speak for the provider service it owns.
func RegisterProviderReadRoutes(group *gin.RouterGroup, handler *InternalHandler) {
	group.GET("/providers/:providerId/offers",
		handler.requireAllowedCaller, handler.ListProviderOffers)
	group.GET("/providers/:providerId/offers/:offerId",
		handler.requireAllowedCaller, handler.GetProviderOffer)
	group.GET("/providers/:providerId/assignments",
		handler.requireAllowedCaller, handler.ListProviderAssignments)
	group.GET("/providers/:providerId/assignments/:assignmentId",
		handler.requireAllowedCaller, handler.GetProviderAssignment)
	group.GET("/providers/:providerId/memberships",
		handler.requireAllowedCaller, handler.ListProviderMemberships)
}

// callerProviderService resolves which provider service this caller may read.
func (h *InternalHandler) callerProviderService(c *gin.Context) (string, error) {
	raw, _ := c.Get("service_name")
	callerService, _ := raw.(string)
	owned, known := callerOwnsProviderService[callerService]
	if !known {
		return "", apperrors.Forbidden("This service may not read provider work.", nil)
	}
	return owned, nil
}

func pageParams(c *gin.Context) (limit int, offset int) {
	limit, _ = strconv.Atoi(c.Query("limit"))
	offset, _ = strconv.Atoi(c.Query("offset"))
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

// ListProviderOffers returns the offers addressed to one provider.
func (h *InternalHandler) ListProviderOffers(c *gin.Context) {
	providerService, err := h.callerProviderService(c)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	limit, offset := pageParams(c)
	page, err := h.service.ListProviderOffers(c.Request.Context(), providerService,
		c.Param("providerId"), limit, offset)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondInternal(c, http.StatusOK, page)
}

// GetProviderOffer returns one offer owned by this provider.
func (h *InternalHandler) GetProviderOffer(c *gin.Context) {
	providerService, err := h.callerProviderService(c)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	offer, err := h.service.GetProviderOffer(c.Request.Context(), providerService,
		c.Param("providerId"), c.Param("offerId"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondInternal(c, http.StatusOK, offer)
}

// ListProviderAssignments returns one provider's assignments.
func (h *InternalHandler) ListProviderAssignments(c *gin.Context) {
	providerService, err := h.callerProviderService(c)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	limit, offset := pageParams(c)
	page, err := h.service.ListProviderAssignments(c.Request.Context(), providerService,
		c.Param("providerId"), limit, offset)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondInternal(c, http.StatusOK, page)
}

// GetProviderAssignment returns one assignment owned by this provider.
func (h *InternalHandler) GetProviderAssignment(c *gin.Context) {
	providerService, err := h.callerProviderService(c)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	assignment, err := h.service.GetProviderAssignment(c.Request.Context(), providerService,
		c.Param("providerId"), c.Param("assignmentId"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondInternal(c, http.StatusOK, assignment)
}

// callerRawProviderService validates the caller against the same allowlist
// callerProviderService uses, but returns the caller's own raw HMAC service
// name (e.g. "driver-dispatch-delivery-service") rather than the
// "DISPATCH"/"TAXI"/"HAULING" abbreviation callerOwnsProviderService's
// values hold.
//
// That abbreviation is correct for ListProviderOffers/ListProviderAssignments
// above (via sourceServiceFor, matching the vocabulary marketplace's own
// assignments/offers tables use) but wrong for a membership lookup:
// fleet_partner_organization_provider_memberships.provider_service is
// populated at membership creation from platformclients.OwningService, which
// returns the raw caller name, not the abbreviation. Querying that column
// with "DISPATCH" would never match a real row.
func (h *InternalHandler) callerRawProviderService(c *gin.Context) (string, error) {
	raw, _ := c.Get("service_name")
	callerService, _ := raw.(string)
	if _, known := callerOwnsProviderService[callerService]; !known {
		return "", apperrors.Forbidden("This service may not read provider work.", nil)
	}
	return callerService, nil
}

// ListProviderMemberships returns every membership this provider identity
// has ever held, across every organization and any status.
func (h *InternalHandler) ListProviderMemberships(c *gin.Context) {
	providerService, err := h.callerRawProviderService(c)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	memberships, err := h.service.ListProviderMemberships(c.Request.Context(), providerService, c.Param("providerId"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondInternal(c, http.StatusOK, memberships)
}

// respondInternal writes the canonical envelope.
func respondInternal(c *gin.Context, status int, data any) {
	c.JSON(status, gin.H{"data": data, "meta": gin.H{
		"requestId": c.GetHeader("X-Request-ID"),
	}})
}
