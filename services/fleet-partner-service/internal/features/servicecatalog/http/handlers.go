// Package http implements the Section 15 service catalogue HTTP surface.
package http

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/servicecatalog/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/servicecatalog/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cursor"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Permissions guarding the Section 15 service catalogue routes. These are the
// exact codes seeded by migration 000025.
const (
	PermissionCatalogRead       = "servicecatalog.items.read"
	PermissionCatalogManage     = "servicecatalog.items.manage"
	PermissionCatalogActivate   = "servicecatalog.items.activate"
	PermissionCatalogDeactivate = "servicecatalog.items.deactivate"
	PermissionCatalogArchive    = "servicecatalog.items.archive"
)

// Handler serves the Section 15 service catalogue routes.
type Handler struct {
	service    *usecases.Service
	authorizer *accesscontrol.Authorizer
}

// NewHandler builds the handler.
func NewHandler(service *usecases.Service, authorizer *accesscontrol.Authorizer) *Handler {
	return &Handler{service: service, authorizer: authorizer}
}

// RegisterTenantRoutes mounts the corporate service catalogue routes.
//
// The routes are mounted whenever the service exists. With the enablement
// flag off they answer not-found rather than disappearing from the surface,
// so a disabled capability is observable instead of indistinguishable from a
// deployment that never had it.
func (h *Handler) RegisterTenantRoutes(group *gin.RouterGroup) {
	if h.service == nil {
		return
	}
	mount(group, h.TenantRoutes(), map[string]gin.HandlerFunc{
		"listServiceCatalogItems":      h.list,
		"createServiceCatalogItem":     h.create,
		"getServiceCatalogItem":        h.get,
		"updateServiceCatalogItem":     h.update,
		"activateServiceCatalogItem":   h.activate,
		"deactivateServiceCatalogItem": h.deactivate,
		"archiveServiceCatalogItem":    h.archive,
	})
}

func mount(group *gin.RouterGroup, routes []Route, handlers map[string]gin.HandlerFunc) {
	for _, route := range routes {
		handler, ok := handlers[route.OperationID]
		if !ok || handler == nil {
			panic("servicecatalog: no handler registered for operation " + route.OperationID)
		}
		group.Handle(route.Method, route.Path, handler)
	}
}

// ---------------------------------------------------------------------------
// Request plumbing
// ---------------------------------------------------------------------------

func requireTenant(c *gin.Context) (tenant.Context, bool) {
	tenantCtx, ok := middleware.TenantFromGin(c)
	if !ok {
		respond.Error(c, apperrors.Forbidden("Organization scope is required.", nil))
		return tenant.Context{}, false
	}
	return tenantCtx, true
}

func (h *Handler) authorize(c *gin.Context, t tenant.Context, permission string) bool {
	if h.authorizer == nil {
		respond.Error(c, apperrors.Forbidden("You do not have permission to perform this action.", nil))
		return false
	}
	if err := h.authorizer.Require(c.Request.Context(), t.OrganizationID, t.OrganizationUserID,
		roleVersionFrom(c), permission, accesscontrol.Target{BranchID: c.Query("branchId")}); err != nil {
		respond.Error(c, err)
		return false
	}
	return true
}

func roleVersionFrom(c *gin.Context) int64 {
	if value, ok := c.Get(middleware.TenantRoleVersionKey); ok {
		if version, ok := value.(int64); ok {
			return version
		}
	}
	return 1
}

func idempotencyKey(c *gin.Context) string { return strings.TrimSpace(c.GetHeader("Idempotency-Key")) }

func correlationID(c *gin.Context) string { return middleware.GetCorrelationID(c) }

func expectedVersion(c *gin.Context, bodyValue int64) int64 {
	if bodyValue != 0 {
		return bodyValue
	}
	raw := strings.Trim(strings.TrimSpace(c.GetHeader("If-Match")), `"`)
	if raw == "" {
		return 0
	}
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func bindJSON(c *gin.Context, target any) bool {
	if err := c.ShouldBindJSON(target); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
		return false
	}
	return true
}

// ---------------------------------------------------------------------------
// Views
// ---------------------------------------------------------------------------

// CatalogItemView is the wire representation of a catalogue item.
type CatalogItemView struct {
	ID                      string         `json:"id"`
	OrganizationID          string         `json:"organizationId"`
	BranchID                string         `json:"branchId,omitempty"`
	Name                    string         `json:"name"`
	Code                    string         `json:"code"`
	Category                string         `json:"category"`
	Description             string         `json:"description"`
	CapabilityReference     string         `json:"capabilityReference,omitempty"`
	RequiredFieldsSchema    map[string]any `json:"requiredFieldsSchema"`
	SupportedVehicleClasses []string       `json:"supportedVehicleClasses"`
	SupportedPackageTypes   []string       `json:"supportedPackageTypes"`
	PricingPolicyReference  string         `json:"pricingPolicyReference,omitempty"`
	ApprovalPolicyReference string         `json:"approvalPolicyReference,omitempty"`
	ProofPolicyReference    string         `json:"proofPolicyReference,omitempty"`
	Status                  string         `json:"status"`
	Version                 int64          `json:"version"`
	CreatedAt               string         `json:"createdAt"`
	UpdatedAt               string         `json:"updatedAt"`
	ArchivedAt              *string        `json:"archivedAt,omitempty"`
}

// NewCatalogItemView projects the domain model to the wire shape.
func NewCatalogItemView(item models.CatalogItem) CatalogItemView {
	view := CatalogItemView{
		ID: item.ID, OrganizationID: item.OrganizationID, BranchID: item.BranchID,
		Name: item.Name, Code: item.Code, Category: item.Category, Description: item.Description,
		CapabilityReference:     item.CapabilityReference,
		RequiredFieldsSchema:    map[string]any(item.RequiredFieldsSchema),
		SupportedVehicleClasses: item.SupportedVehicleClasses,
		SupportedPackageTypes:   item.SupportedPackageTypes,
		PricingPolicyReference:  item.PricingPolicyReference,
		ApprovalPolicyReference: item.ApprovalPolicyReference,
		ProofPolicyReference:    item.ProofPolicyReference,
		Status:                  item.Status,
		Version:                 item.Version,
		CreatedAt:               item.CreatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
		UpdatedAt:               item.UpdatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
	}
	if item.ArchivedAt != nil {
		formatted := item.ArchivedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
		view.ArchivedAt = &formatted
	}
	if view.RequiredFieldsSchema == nil {
		view.RequiredFieldsSchema = map[string]any{}
	}
	if view.SupportedVehicleClasses == nil {
		view.SupportedVehicleClasses = []string{}
	}
	if view.SupportedPackageTypes == nil {
		view.SupportedPackageTypes = []string{}
	}
	return view
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

func (h *Handler) list(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionCatalogRead) {
		return
	}
	limit := 0
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			respond.Error(c, apperrors.BadRequest("limit must be an integer.", nil))
			return
		}
		limit = parsed
	}
	items, next, err := h.service.List(c.Request.Context(), t, usecases.ListInput{
		Status: c.Query("status"), Category: c.Query("category"), BranchID: c.Query("branchId"),
		Cursor: c.Query("cursor"), Limit: cursor.ClampLimit(limit),
	})
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]CatalogItemView, 0, len(items))
	for _, item := range items {
		views = append(views, NewCatalogItemView(item))
	}
	respond.List(c, views, next)
}

func (h *Handler) get(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionCatalogRead) {
		return
	}
	item, err := h.service.Get(c.Request.Context(), t, c.Param("serviceId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewCatalogItemView(item))
}

type createCatalogItemRequest struct {
	BranchID                string         `json:"branchId"`
	Name                    string         `json:"name"`
	Code                    string         `json:"code"`
	Category                string         `json:"category"`
	Description             string         `json:"description"`
	CapabilityReference     string         `json:"capabilityReference"`
	RequiredFieldsSchema    map[string]any `json:"requiredFieldsSchema"`
	SupportedVehicleClasses []string       `json:"supportedVehicleClasses"`
	SupportedPackageTypes   []string       `json:"supportedPackageTypes"`
	PricingPolicyReference  string         `json:"pricingPolicyReference"`
	ApprovalPolicyReference string         `json:"approvalPolicyReference"`
	ProofPolicyReference    string         `json:"proofPolicyReference"`
}

func (h *Handler) create(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionCatalogManage) {
		return
	}
	var body createCatalogItemRequest
	if !bindJSON(c, &body) {
		return
	}
	item, err := h.service.Create(c.Request.Context(), t, usecases.CreateInput{
		BranchID: body.BranchID, Name: body.Name, Code: body.Code, Category: body.Category,
		Description: body.Description, CapabilityReference: body.CapabilityReference,
		RequiredFieldsSchema:    models.JSONMap(body.RequiredFieldsSchema),
		SupportedVehicleClasses: body.SupportedVehicleClasses,
		SupportedPackageTypes:   body.SupportedPackageTypes,
		PricingPolicyReference:  body.PricingPolicyReference,
		ApprovalPolicyReference: body.ApprovalPolicyReference,
		ProofPolicyReference:    body.ProofPolicyReference,
	})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.Created(c, NewCatalogItemView(item))
}

type updateCatalogItemRequest struct {
	Name                    string         `json:"name"`
	Description             string         `json:"description"`
	CapabilityReference     string         `json:"capabilityReference"`
	RequiredFieldsSchema    map[string]any `json:"requiredFieldsSchema"`
	SupportedVehicleClasses []string       `json:"supportedVehicleClasses"`
	SupportedPackageTypes   []string       `json:"supportedPackageTypes"`
	PricingPolicyReference  string         `json:"pricingPolicyReference"`
	ApprovalPolicyReference string         `json:"approvalPolicyReference"`
	ProofPolicyReference    string         `json:"proofPolicyReference"`
	ExpectedVersion         int64          `json:"expectedVersion"`
}

func (h *Handler) update(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionCatalogManage) {
		return
	}
	var body updateCatalogItemRequest
	if !bindJSON(c, &body) {
		return
	}
	item, err := h.service.Update(c.Request.Context(), t, c.Param("serviceId"),
		expectedVersion(c, body.ExpectedVersion), usecases.UpdateInput{
			Name: body.Name, Description: body.Description, CapabilityReference: body.CapabilityReference,
			RequiredFieldsSchema:    models.JSONMap(body.RequiredFieldsSchema),
			SupportedVehicleClasses: body.SupportedVehicleClasses,
			SupportedPackageTypes:   body.SupportedPackageTypes,
			PricingPolicyReference:  body.PricingPolicyReference,
			ApprovalPolicyReference: body.ApprovalPolicyReference,
			ProofPolicyReference:    body.ProofPolicyReference,
		})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewCatalogItemView(item))
}

type versionOnlyRequest struct {
	ExpectedVersion int64 `json:"expectedVersion"`
}

func (h *Handler) activate(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionCatalogActivate) {
		return
	}
	var body versionOnlyRequest
	if !bindJSON(c, &body) {
		return
	}
	item, err := h.service.Activate(c.Request.Context(), t, c.Param("serviceId"),
		expectedVersion(c, body.ExpectedVersion))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewCatalogItemView(item))
}

func (h *Handler) deactivate(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionCatalogDeactivate) {
		return
	}
	var body versionOnlyRequest
	if !bindJSON(c, &body) {
		return
	}
	item, err := h.service.Deactivate(c.Request.Context(), t, c.Param("serviceId"),
		expectedVersion(c, body.ExpectedVersion))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewCatalogItemView(item))
}

func (h *Handler) archive(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionCatalogArchive) {
		return
	}
	var body versionOnlyRequest
	if !bindJSON(c, &body) {
		return
	}
	item, err := h.service.Archive(c.Request.Context(), t, c.Param("serviceId"),
		expectedVersion(c, body.ExpectedVersion))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewCatalogItemView(item))
}
