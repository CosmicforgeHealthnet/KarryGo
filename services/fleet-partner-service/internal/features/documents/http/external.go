package http

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
)

// The provider-facing and customer-facing document routes.
//
// These sit behind the internal HMAC service route class, never behind a
// provider or customer bearer token: fleet-partner-service issues neither.
// A driver service has already authenticated the provider's own session and
// vouches for the provider identity it asserts here; customer-service has
// already authenticated the customer's own session and vouches for the
// customer identity it asserts here. Fleet independently resolves each
// assertion to something it actually owns before disclosing anything — see
// usecases.Service.ResolveProviderMembership and requireCustomerOwnsOrder.
//
// callerOwnsProviderService pairs an authenticated caller with the provider
// service it is allowed to speak for — the same vocabulary the marketplace
// feature's provider-response routes use, redeclared locally because a
// feature may not import another feature's http package.
var callerOwnsProviderService = map[string]string{
	"driver-dispatch-delivery-service": "DISPATCH",
	"driver-taxi-service":              "TAXI",
	"driver-hauling-service":           "HAULING",
}

// ExternalHandler serves the provider-facing and customer-facing document
// routes on the HMAC-authenticated internal surface.
type ExternalHandler struct {
	service *usecases.Service
	// allowedProviderCallers and allowedCustomerCallers are separate
	// allowlists: a caller trusted to relay provider identity is not
	// automatically trusted to relay customer identity, and vice versa.
	allowedProviderCallers map[string]bool
	allowedCustomerCallers map[string]bool
}

// NewExternalHandler builds the handler with independent caller allowlists.
func NewExternalHandler(service *usecases.Service, providerCallers []string,
	customerCallers []string) *ExternalHandler {
	toSet := func(names []string) map[string]bool {
		set := map[string]bool{}
		for _, name := range names {
			if trimmed := strings.TrimSpace(name); trimmed != "" {
				set[trimmed] = true
			}
		}
		return set
	}
	return &ExternalHandler{
		service:                service,
		allowedProviderCallers: toSet(providerCallers),
		allowedCustomerCallers: toSet(customerCallers),
	}
}

// RegisterExternalRoutes mounts the provider-facing and customer-facing
// document routes.
//
// Exact routes:
//
//	GET  /providers/:providerId/documents
//	GET  /providers/:providerId/documents/:documentId
//	POST /providers/:providerId/documents/:documentId/download-requests
//	GET  /customers/:customerId/orders/:orderId/documents
//	GET  /customers/:customerId/orders/:orderId/documents/:documentId
//	POST /customers/:customerId/orders/:orderId/documents/:documentId/download-requests
func RegisterExternalRoutes(group *gin.RouterGroup, handler *ExternalHandler) {
	group.GET("/providers/:providerId/documents",
		handler.requireProviderCaller, handler.listProviderDocuments)
	group.GET("/providers/:providerId/documents/:documentId",
		handler.requireProviderCaller, handler.getProviderDocument)
	group.POST("/providers/:providerId/documents/:documentId/download-requests",
		handler.requireProviderCaller, handler.requestProviderDownload)

	group.GET("/customers/:customerId/orders/:orderId/documents",
		handler.requireCustomerCaller, handler.listCustomerDocuments)
	group.GET("/customers/:customerId/orders/:orderId/documents/:documentId",
		handler.requireCustomerCaller, handler.getCustomerDocument)
	group.POST("/customers/:customerId/orders/:orderId/documents/:documentId/download-requests",
		handler.requireCustomerCaller, handler.requestCustomerDownload)
}

func callerName(c *gin.Context) string {
	raw, _ := c.Get(middleware.ServiceCallerKey)
	name, _ := raw.(string)
	return name
}

// requireProviderCaller resolves the calling driver service to the provider
// service it may speak for, and stores it for the handler.
//
// The value stored is the caller's own raw HMAC service name (e.g.
// "driver-dispatch-delivery-service"), NOT the "DISPATCH"/"TAXI"/"HAULING"
// abbreviation callerOwnsProviderService's values hold. That abbreviation is
// the marketplace feature's own vocabulary for its assignments/offers
// tables (via sourceServiceFor) — a different table with a different
// convention. fleet_partner_organization_provider_memberships.provider_service
// is populated at membership creation from platformclients.OwningService,
// which returns the raw caller name, so a membership lookup querying
// "DISPATCH" against that column would never match any real row. The map is
// still used here, but only as the allowlist of which callers may assert a
// provider identity at all — its values are not read.
func (h *ExternalHandler) requireProviderCaller(c *gin.Context) {
	caller := callerName(c)
	if !h.allowedProviderCallers[caller] {
		respond.Error(c, apperrors.Forbidden("Service is not allowed to call this endpoint.", nil))
		c.Abort()
		return
	}
	if _, known := callerOwnsProviderService[caller]; !known {
		respond.Error(c, apperrors.Forbidden("This service may not read provider documents.", nil))
		c.Abort()
		return
	}
	c.Set("documents.providerService", caller)
	c.Next()
}

func (h *ExternalHandler) requireCustomerCaller(c *gin.Context) {
	if !h.allowedCustomerCallers[callerName(c)] {
		respond.Error(c, apperrors.Forbidden("Service is not allowed to call this endpoint.", nil))
		c.Abort()
		return
	}
	c.Next()
}

func (h *ExternalHandler) providerService(c *gin.Context) string {
	raw, _ := c.Get("documents.providerService")
	service, _ := raw.(string)
	return service
}

// externalDocumentView is the narrow, provider/customer-safe document DTO.
//
// Deliberately built field by field rather than reusing the corporate
// DocumentView: retention class, legal hold, media source version and the
// other governance fields on the corporate view are internal detail an
// external caller has no business seeing, and a shared struct would leak the
// next governance field the corporate view gains by default.
type externalDocumentView struct {
	ID           string `json:"documentId"`
	Category     string `json:"category"`
	DocumentType string `json:"documentType"`
	ExpiryDate   string `json:"expiryDate,omitempty"`
	Verified     bool   `json:"verified"`
	Downloadable bool   `json:"downloadable"`
	CreatedAt    string `json:"createdAt"`
}

func newExternalDocumentView(document models.Document) externalDocumentView {
	view := externalDocumentView{
		ID: document.ID, Category: document.Category, DocumentType: document.DocumentType,
		Verified: document.Verified(), Downloadable: document.Downloadable(),
		CreatedAt: document.CreatedAt.UTC().Format(time.RFC3339),
	}
	if document.ExpiryDate != nil {
		view.ExpiryDate = document.ExpiryDate.UTC().Format("2006-01-02")
	}
	return view
}

func respondExternal(c *gin.Context, status int, data any) {
	c.JSON(status, gin.H{"data": data, "meta": gin.H{"requestId": c.GetHeader("X-Request-ID")}})
}

// --- provider handlers -----------------------------------------------------

func (h *ExternalHandler) listProviderDocuments(c *gin.Context) {
	documents, err := h.service.ListProviderDocuments(c.Request.Context(), h.providerService(c),
		c.Param("providerId"), middleware.GetCorrelationID(c))
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]externalDocumentView, 0, len(documents))
	for _, document := range documents {
		views = append(views, newExternalDocumentView(document))
	}
	respondExternal(c, http.StatusOK, gin.H{"documents": views})
}

func (h *ExternalHandler) getProviderDocument(c *gin.Context) {
	document, err := h.service.GetProviderDocument(c.Request.Context(), h.providerService(c),
		c.Param("providerId"), c.Param("documentId"), middleware.GetCorrelationID(c))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respondExternal(c, http.StatusOK, newExternalDocumentView(*document))
}

func (h *ExternalHandler) requestProviderDownload(c *gin.Context) {
	grant, err := h.service.AuthorizeProviderDownload(c.Request.Context(), h.providerService(c),
		c.Param("providerId"), c.Param("documentId"), middleware.GetCorrelationID(c),
		c.GetHeader("X-Causation-ID"), c.GetHeader("X-Request-ID"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respondExternal(c, http.StatusOK, downloadGrantView(grant))
}

// --- customer handlers ------------------------------------------------------

func (h *ExternalHandler) listCustomerDocuments(c *gin.Context) {
	documents, err := h.service.ListCustomerDocuments(c.Request.Context(), c.GetHeader("X-Organization-ID"),
		c.Param("orderId"), c.Param("customerId"), middleware.GetCorrelationID(c))
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]externalDocumentView, 0, len(documents))
	for _, document := range documents {
		views = append(views, newExternalDocumentView(document))
	}
	respondExternal(c, http.StatusOK, gin.H{"documents": views})
}

func (h *ExternalHandler) getCustomerDocument(c *gin.Context) {
	document, err := h.service.GetCustomerDocument(c.Request.Context(), c.GetHeader("X-Organization-ID"),
		c.Param("orderId"), c.Param("customerId"), c.Param("documentId"), middleware.GetCorrelationID(c))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respondExternal(c, http.StatusOK, newExternalDocumentView(*document))
}

func (h *ExternalHandler) requestCustomerDownload(c *gin.Context) {
	grant, err := h.service.AuthorizeCustomerDownload(c.Request.Context(), c.GetHeader("X-Organization-ID"),
		c.Param("orderId"), c.Param("customerId"), c.Param("documentId"), middleware.GetCorrelationID(c),
		c.GetHeader("X-Causation-ID"), c.GetHeader("X-Request-ID"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respondExternal(c, http.StatusOK, downloadGrantView(grant))
}

func downloadGrantView(grant *usecases.DownloadGrant) gin.H {
	return gin.H{
		"documentId":    grant.DocumentID,
		"downloadToken": grant.Token,
		"expiresAt":     grant.ExpiresAt.UTC().Format(time.RFC3339),
		"contentType":   grant.ContentType,
		"sizeBytes":     grant.SizeBytes,
	}
}
