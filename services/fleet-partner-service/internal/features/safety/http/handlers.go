// Package http exposes the corporate safety-reference API.
//
// Every route here is tenant-scoped and permission-gated. Reading a reference
// and asserting one are separate permissions on purpose: someone who may look
// up an incident is not automatically someone who may claim one exists.
package http

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/safety/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/safety/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/safety/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Permissions guarding the routes.
const (
	PermissionRead   = "safety.references.read"
	PermissionManage = "safety.references.manage"
)

// Handler serves the corporate safety-reference routes.
type Handler struct {
	service    *usecases.Service
	authorizer *accesscontrol.Authorizer
}

// NewHandler builds the handler.
func NewHandler(service *usecases.Service, authorizer *accesscontrol.Authorizer) *Handler {
	return &Handler{service: service, authorizer: authorizer}
}

// RegisterTenantRoutes mounts routes that require resolved organization scope.
func (h *Handler) RegisterTenantRoutes(group *gin.RouterGroup) {
	group.POST("/safety/references", h.recordReference)
	group.GET("/safety/references", h.listReferences)
	group.GET("/safety/references/:referenceId", h.getReference)
	group.GET("/safety/references/:referenceId/history", h.referenceHistory)
}

// requireTenant resolves the tenant context or fails closed.
func requireTenant(c *gin.Context) (tenant.Context, bool) {
	tenantCtx, ok := middleware.TenantFromGin(c)
	if !ok {
		respond.Error(c, apperrors.Forbidden("Organization scope is required.", nil))
		return tenant.Context{}, false
	}
	return tenantCtx, true
}

// authorize enforces a permission for the caller's membership.
func (h *Handler) authorize(c *gin.Context, tenantCtx tenant.Context, permission string) bool {
	if h.authorizer == nil {
		// No authorizer means no way to prove authority. Denying is the only
		// safe answer: a missing gate must never read as an open one.
		respond.Error(c, apperrors.Forbidden("You do not have permission to perform this action.", nil))
		return false
	}
	if err := h.authorizer.Require(c.Request.Context(), tenantCtx.OrganizationID,
		tenantCtx.OrganizationUserID, roleVersionFrom(c), permission,
		accesscontrol.Target{}); err != nil {
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

// recordReferenceRequest is the corporate write body.
type recordReferenceRequest struct {
	ReferenceType       string `json:"referenceType"`
	ExternalReference   string `json:"externalReference"`
	AssignmentID        string `json:"assignmentId"`
	PlatformTripID      string `json:"platformTripId"`
	ExecutingProviderID string `json:"executingProviderId"`
	VehicleReference    string `json:"vehicleReference"`
}

// ReferenceView is the allowlisted response shape.
//
// Built field by field. The row carries no case content to leak, but the view
// is assembled explicitly anyway so a later column cannot become a response
// field by accident.
type ReferenceView struct {
	ID                     string  `json:"id"`
	ReferenceType          string  `json:"referenceType"`
	AuthorityService       string  `json:"authorityService"`
	ExternalReference      string  `json:"externalReference"`
	AssignmentID           string  `json:"assignmentId,omitempty"`
	PlatformTripID         string  `json:"platformTripId,omitempty"`
	ExecutingProviderID    string  `json:"executingProviderId,omitempty"`
	VehicleReference       string  `json:"vehicleReference,omitempty"`
	VerificationState      string  `json:"verificationState"`
	AuthorizesConsequences bool    `json:"authorizesConsequences"`
	StatusProjection       string  `json:"statusProjection,omitempty"`
	CreatedByActorType     string  `json:"createdByActorType"`
	CreatedAt              string  `json:"createdAt"`
	Version                int64   `json:"version"`
	Replayed               *bool   `json:"replayed,omitempty"`
	Warning                *string `json:"warning,omitempty"`
}

// unverifiedWarning is returned with every unverified reference.
//
// Present in the payload rather than only in documentation: a client that has
// to read a manual to learn a record is unconfirmed will eventually not read
// it.
const unverifiedWarning = "This reference could not be confirmed with the authoritative service and must not be used to justify operational or financial action."

func newReferenceView(reference *models.Reference) ReferenceView {
	view := ReferenceView{
		ID:                     reference.ID,
		ReferenceType:          reference.ReferenceType,
		AuthorityService:       reference.AuthorityService,
		ExternalReference:      reference.ExternalReference,
		AssignmentID:           deref(reference.AssignmentID),
		PlatformTripID:         deref(reference.PlatformTripID),
		ExecutingProviderID:    deref(reference.ExecutingProviderID),
		VehicleReference:       deref(reference.VehicleReference),
		VerificationState:      reference.VerificationState,
		AuthorizesConsequences: reference.AuthorizesConsequences(),
		StatusProjection:       deref(reference.StatusProjection),
		CreatedByActorType:     reference.CreatedByActorType,
		CreatedAt:              reference.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		Version:                reference.Version,
	}
	if !reference.Verified() {
		warning := unverifiedWarning
		view.Warning = &warning
	}
	return view
}

// recordReference attaches an authoritative case to organization work.
func (h *Handler) recordReference(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, PermissionManage) {
		return
	}

	var body recordReferenceRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	idempotencyKey := c.GetHeader("Idempotency-Key")
	if idempotencyKey == "" {
		respond.Error(c, apperrors.BadRequest("An Idempotency-Key header is required.", nil))
		return
	}

	result, err := h.service.RecordReference(c.Request.Context(), usecases.RecordReferenceInput{
		Actor: usecases.Actor{
			OrganizationID: tenantCtx.OrganizationID,
			ActorType:      models.ActorOrganizationUser,
			ActorID:        tenantCtx.OrganizationUserID,
		},
		ReferenceType:       body.ReferenceType,
		ExternalReference:   body.ExternalReference,
		AssignmentID:        body.AssignmentID,
		PlatformTripID:      body.PlatformTripID,
		ExecutingProviderID: body.ExecutingProviderID,
		VehicleReference:    body.VehicleReference,
		IdempotencyKey:      idempotencyKey,
		CorrelationID:       middleware.GetCorrelationID(c),
		CausationID:         c.GetHeader("X-Causation-ID"),
	})
	if err != nil {
		respond.Error(c, err)
		return
	}

	view := newReferenceView(result.Reference)
	replayed := result.Replayed
	view.Replayed = &replayed
	status := http.StatusCreated
	if result.Replayed {
		status = http.StatusOK
	}
	c.JSON(status, gin.H{"data": view, "meta": gin.H{
		"requestId": c.GetHeader("X-Request-ID"),
	}})
}

// listReferences returns the organization's references.
func (h *Handler) listReferences(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, PermissionRead) {
		return
	}

	limit, _ := strconv.Atoi(c.Query("limit"))
	references, err := h.service.List(c.Request.Context(), tenant.Query{
		OrganizationID:     tenantCtx.OrganizationID,
		PermittedBranchIDs: tenantCtx.PermittedBranchIDs,
		Limit:              limit,
	}, repositories.ListFilter{
		ReferenceType:       c.Query("referenceType"),
		AssignmentID:        c.Query("assignmentId"),
		PlatformTripID:      c.Query("platformTripId"),
		ExecutingProviderID: c.Query("executingProviderId"),
	})
	if err != nil {
		respond.Error(c, err)
		return
	}

	views := make([]ReferenceView, 0, len(references))
	for index := range references {
		views = append(views, newReferenceView(&references[index]))
	}
	respond.OK(c, gin.H{"references": views})
}

// getReference returns one reference.
func (h *Handler) getReference(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, PermissionRead) {
		return
	}
	reference, err := h.service.Get(c.Request.Context(), tenantCtx.OrganizationID,
		c.Param("referenceId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, newReferenceView(reference))
}

// referenceHistory returns the append-only history for one reference.
func (h *Handler) referenceHistory(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, PermissionRead) {
		return
	}
	entries, err := h.service.History(c.Request.Context(), tenantCtx.OrganizationID,
		c.Param("referenceId"))
	if err != nil {
		respond.Error(c, err)
		return
	}

	views := make([]gin.H, 0, len(entries))
	for _, entry := range entries {
		views = append(views, gin.H{
			"id":         entry.ID,
			"action":     entry.Action,
			"fromState":  entry.FromState,
			"toState":    entry.ToState,
			"actorType":  entry.ActorType,
			"occurredAt": entry.OccurredAt,
		})
	}
	respond.OK(c, gin.H{"history": views})
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
