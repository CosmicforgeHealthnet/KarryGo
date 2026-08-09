// Package http implements the Section 25 search HTTP surface.
package http

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/search/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/search/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// PermissionSearchRead is the exact code seeded by migration 000053.
const PermissionSearchRead = "search.read"

// PermissionSearchRepair is the exact code seeded by migration 000058,
// granted only to operator-level roles.
const PermissionSearchRepair = "search.repair"
const PermissionSearchReplay = "search.replay"

// Handler serves the Section 25 search route.
type Handler struct {
	service    *usecases.Service
	authorizer *accesscontrol.Authorizer
}

// NewHandler builds the handler.
func NewHandler(service *usecases.Service, authorizer *accesscontrol.Authorizer) *Handler {
	return &Handler{service: service, authorizer: authorizer}
}

// RegisterTenantRoutes mounts the corporate search routes.
func (h *Handler) RegisterTenantRoutes(group *gin.RouterGroup) {
	if h.service == nil {
		return
	}
	mount(group, h.TenantRoutes(), map[string]gin.HandlerFunc{
		"search":               h.search,
		"reindexSearchSubject": h.reindexSubject,
		"removeSearchSubject":  h.removeSubject,
		"replaySearchIndexJob": h.replayIndexJob,
	})
}

func (h *Handler) replayIndexJob(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionSearchReplay) {
		return
	}
	if err := h.service.ReplayIndexJob(c, t.OrganizationID, c.Param("jobId")); err != nil {
		respond.Error(c, apperrors.NotFound("Search indexing job not found.", err))
		return
	}
	respond.JSON(c, 202, gin.H{"status": "REQUEUED"})
}

func mount(group *gin.RouterGroup, routes []Route, handlers map[string]gin.HandlerFunc) {
	for _, route := range routes {
		handler, ok := handlers[route.OperationID]
		if !ok || handler == nil {
			panic("search: no handler registered for operation " + route.OperationID)
		}
		group.Handle(route.Method, route.Path, handler)
	}
}

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

// DocumentView is the wire representation of a search result. It carries
// only a safe summary and navigation metadata — never the full source
// record — matching "do not return entire source records through search."
type DocumentView struct {
	SubjectType          string         `json:"subjectType"`
	SubjectReference     string         `json:"subjectReference"`
	Title                string         `json:"title"`
	Status               string         `json:"status,omitempty"`
	ExactReferenceFields map[string]any `json:"exactReferenceFields"`
	SourceVersion        int64          `json:"sourceVersion"`
	IndexedAt            string         `json:"indexedAt"`
}

// NewDocumentView projects the domain model to the wire shape.
func NewDocumentView(d models.Document) DocumentView {
	view := DocumentView{
		SubjectType: d.SubjectType, SubjectReference: d.SubjectReference, Title: d.Title, Status: d.Status,
		ExactReferenceFields: map[string]any(d.ExactReferenceFields), SourceVersion: d.SourceVersion,
		IndexedAt: d.IndexedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
	}
	if view.ExactReferenceFields == nil {
		view.ExactReferenceFields = map[string]any{}
	}
	return view
}

func (h *Handler) search(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionSearchRead) {
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
	documents, err := h.service.Search(c.Request.Context(), t, usecases.SearchInput{
		Text: c.Query("q"), SubjectType: c.Query("subjectType"), ExactReference: c.Query("exactReference"),
		Status: c.Query("status"), Limit: limit, CallerRoleVersion: roleVersionFrom(c),
	})
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]DocumentView, 0, len(documents))
	for _, document := range documents {
		views = append(views, NewDocumentView(document))
	}
	respond.OK(c, views)
}

// reindexSubjectBody is the operator repair request: everything Service.Index
// needs to force one subject's projection, independent of whichever feature
// normally drives it.
type reindexSubjectBody struct {
	BranchID             string         `json:"branchId"`
	SubjectType          string         `json:"subjectType"`
	SubjectReference     string         `json:"subjectReference"`
	Title                string         `json:"title"`
	SearchableText       string         `json:"searchableText"`
	ExactReferenceFields map[string]any `json:"exactReferenceFields"`
	Status               string         `json:"status"`
	RequiredPermission   string         `json:"requiredPermission"`
	SourceVersion        int64          `json:"sourceVersion"`
}

// reindexSubject lets an operator force one subject's search projection to a
// known-correct state.
//
// This is the repair primitive: whatever authoritative facts the operator
// supplies are exactly what Index would have received from the owning
// feature. It obeys the same forward-only source_version guard as every
// other reindex call — an operator cannot use this to push a projection
// backwards past a version the owning feature has already indexed.
func (h *Handler) reindexSubject(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionSearchRepair) {
		return
	}
	var body reindexSubjectBody
	if err := c.ShouldBindJSON(&body); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body is invalid.", nil))
		return
	}
	document, err := h.service.Index(c.Request.Context(), t.OrganizationID, usecases.IndexInput{
		BranchID: body.BranchID, SubjectType: body.SubjectType, SubjectReference: body.SubjectReference,
		Title: body.Title, SearchableText: body.SearchableText,
		ExactReferenceFields: models.JSONMap(body.ExactReferenceFields),
		Status:               body.Status, RequiredPermission: body.RequiredPermission,
		SourceVersion: body.SourceVersion,
	})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewDocumentView(document))
}

type removeSubjectBody struct {
	SubjectType      string `json:"subjectType"`
	SubjectReference string `json:"subjectReference"`
}

// removeSubject lets an operator take one subject out of search results
// without deleting its projection history — the same effect a feature's own
// delete/archive path produces via RemoveFromIndex, available directly for
// the case where that call was missed or failed.
func (h *Handler) removeSubject(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionSearchRepair) {
		return
	}
	var body removeSubjectBody
	if err := c.ShouldBindJSON(&body); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body is invalid.", nil))
		return
	}
	if strings.TrimSpace(body.SubjectType) == "" || strings.TrimSpace(body.SubjectReference) == "" {
		respond.Error(c, apperrors.BadRequest("subjectType and subjectReference are required.", nil))
		return
	}
	if err := h.service.RemoveFromIndex(c.Request.Context(), t.OrganizationID,
		body.SubjectType, body.SubjectReference); err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, gin.H{"removed": true})
}
