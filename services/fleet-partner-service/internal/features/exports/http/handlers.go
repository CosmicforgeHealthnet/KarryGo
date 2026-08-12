// Package http exposes the Section 25D generic export routes.
//
// Three permissions, deliberately separate: reading a job's status, creating
// or cancelling one, and obtaining the file itself are three different
// capabilities, matching the precedent set by documents.
package http

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/exports/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/exports/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

const (
	PermissionRead     = "exports.read"
	PermissionManage   = "exports.manage"
	PermissionDownload = "exports.download"
)

type Handler struct {
	service     *usecases.Service
	authorizer  *accesscontrol.Authorizer
	idempotency *idempotency.Store
}

func NewHandler(service *usecases.Service, authorizer *accesscontrol.Authorizer,
	idempotencyStore *idempotency.Store) *Handler {
	return &Handler{service: service, authorizer: authorizer, idempotency: idempotencyStore}
}

// RegisterTenantRoutes mounts routes that require resolved organization scope.
func (h *Handler) RegisterTenantRoutes(g *gin.RouterGroup) {
	if h.service == nil {
		return
	}
	handlers := map[string]gin.HandlerFunc{
		"listExports": h.list, "createExport": h.create, "getExport": h.get,
		"exportDownloadURL": h.downloadURL, "cancelExport": h.cancel, "replayExport": h.replay,
	}
	for _, route := range h.TenantRoutes() {
		handler, ok := handlers[route.OperationID]
		if !ok || handler == nil {
			panic("exports: no handler registered for operation " + route.OperationID)
		}
		g.Handle(route.Method, route.Path, handler)
	}
}

func requireTenant(c *gin.Context) (tenant.Context, bool) {
	t, ok := middleware.TenantFromGin(c)
	if !ok {
		respond.Error(c, apperrors.Forbidden("Organization scope is required.", nil))
	}
	return t, ok
}

func roleVersionFrom(c *gin.Context) int64 {
	if value, ok := c.Get(middleware.TenantRoleVersionKey); ok {
		if version, ok := value.(int64); ok {
			return version
		}
	}
	return 1
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

func bind(c *gin.Context, v any) bool {
	if c.ShouldBindJSON(v) != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
		return false
	}
	return true
}

func expectedVersionFrom(c *gin.Context, bodyVersion int64) int64 {
	if bodyVersion > 0 {
		return bodyVersion
	}
	v, _ := strconv.ParseInt(strings.Trim(strings.TrimSpace(c.GetHeader("If-Match")), `"`), 10, 64)
	return v
}

// JobView is the allowlisted response shape. Filters/columns are echoed back
// because the caller supplied them; nothing about the underlying dataset rows
// or the media object's storage location is ever included.
type JobView struct {
	ID               string         `json:"id"`
	BranchID         string         `json:"branchId,omitempty"`
	ExportType       string         `json:"exportType"`
	Format           string         `json:"format"`
	Filters          map[string]any `json:"filters"`
	RequestedColumns []string       `json:"requestedColumns"`
	Status           string         `json:"status"`
	RowCount         int            `json:"rowCount"`
	ExpiresAt        string         `json:"expiresAt,omitempty"`
	Attempts         int            `json:"attempts"`
	LastErrorCode    string         `json:"lastErrorCode,omitempty"`
	Version          int64          `json:"version"`
	CreatedAt        string         `json:"createdAt"`
	UpdatedAt        string         `json:"updatedAt"`
	CompletedAt      string         `json:"completedAt,omitempty"`
	CancelledAt      string         `json:"cancelledAt,omitempty"`
}

func newJobView(job models.Job) JobView {
	view := JobView{
		ID: job.ID, BranchID: job.BranchID, ExportType: job.ExportType, Format: job.Format,
		Filters: map[string]any(job.FilterSnapshot), RequestedColumns: job.RequestedColumns,
		Status: job.Status, RowCount: job.RowCount, Attempts: job.Attempts,
		LastErrorCode: job.LastErrorCode, Version: job.Version,
		CreatedAt: job.CreatedAt.UTC().Format(time.RFC3339), UpdatedAt: job.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if view.Filters == nil {
		view.Filters = map[string]any{}
	}
	if view.RequestedColumns == nil {
		view.RequestedColumns = []string{}
	}
	if job.ExpiresAt != nil {
		view.ExpiresAt = job.ExpiresAt.UTC().Format(time.RFC3339)
	}
	if job.CompletedAt != nil {
		view.CompletedAt = job.CompletedAt.UTC().Format(time.RFC3339)
	}
	if job.CancelledAt != nil {
		view.CancelledAt = job.CancelledAt.UTC().Format(time.RFC3339)
	}
	return view
}

// runIdempotent replays the original raw response bytes on a retried request
// with the same key, matching the shipments/bulk-imports convention exactly.
func (h *Handler) runIdempotent(c *gin.Context, endpoint string, t tenant.Context, body any,
	fn func() (int, any, error)) {
	if h.idempotency == nil {
		respond.Error(c, apperrors.Internal("Idempotency unavailable.", nil))
		return
	}
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		respond.Error(c, apperrors.BadRequest("An Idempotency-Key header is required.", nil))
		return
	}
	raw, _ := json.Marshal(body)
	fingerprint := idempotency.Fingerprint(c.Request.Method, endpoint, raw)
	record, err := h.idempotency.Begin(c.Request.Context(), t.OrganizationID, endpoint, key, fingerprint)
	if err != nil {
		respond.Error(c, idempotency.AsAppError(err))
		return
	}
	if record != nil {
		c.Data(record.ResponseStatus, "application/json; charset=utf-8", record.ResponseBody)
		return
	}
	status, data, callErr := fn()
	requestID := httpx.GetRequestID(c)
	var payload []byte
	if callErr != nil {
		appErr := apperrors.From(callErr)
		fields := appErr.Fields
		if fields == nil {
			fields = []apperrors.FieldViolation{}
		}
		payload, _ = json.Marshal(gin.H{"error": respond.ErrorBody{
			Code: appErr.Code, Message: appErr.Message, Details: fields, RequestID: requestID,
		}})
		status = appErr.Status
	} else {
		payload, _ = json.Marshal(gin.H{"data": data, "meta": gin.H{"requestId": requestID}})
	}
	if err = h.idempotency.Complete(c.Request.Context(), t.OrganizationID, endpoint, key, status, payload); err != nil {
		respond.Error(c, apperrors.Internal("Failed to persist idempotent response.", err))
		return
	}
	c.Data(status, "application/json; charset=utf-8", payload)
}

type createExportRequest struct {
	BranchID         string         `json:"branchId"`
	ExportType       string         `json:"exportType"`
	Format           string         `json:"format"`
	Filters          map[string]any `json:"filters"`
	RequestedColumns []string       `json:"requestedColumns"`
}

func (h *Handler) create(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionManage) {
		return
	}
	var body createExportRequest
	if !bind(c, &body) {
		return
	}
	for _, permission := range datasetPermissions(body.ExportType) {
		if !h.authorize(c, t, permission) {
			return
		}
	}
	h.runIdempotent(c, "POST /exports", t, body, func() (int, any, error) {
		job, err := h.service.RequestExport(c.Request.Context(), usecases.RequestExportInput{
			Actor: t, BranchID: body.BranchID, ExportType: body.ExportType, Format: body.Format,
			Filters: body.Filters, RequestedColumns: body.RequestedColumns,
			IdempotencyKey: c.GetHeader("Idempotency-Key"),
		})
		if err != nil {
			return 0, nil, err
		}
		return http.StatusAccepted, newJobView(job), nil
	})
}

// datasetPermissions prevents the broad exports.manage capability from
// becoming a side door around an owning domain's read permission.
func datasetPermissions(exportType string) []string {
	switch exportType {
	case models.TypeComplianceIncidents:
		return []string{"compliance.read", "incidents.read"}
	case models.TypeInvoicesReceivables:
		return []string{"invoices.read", "receivables.read"}
	case models.TypeFulfilmentOrders:
		return []string{"fulfilment.read"}
	case models.TypeSupportCases:
		return []string{"support.read"}
	default:
		return nil
	}
}

func (h *Handler) list(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionRead) {
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
	jobs, err := h.service.List(c.Request.Context(), t.OrganizationID, limit)
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]JobView, 0, len(jobs))
	for _, job := range jobs {
		views = append(views, newJobView(job))
	}
	respond.OK(c, gin.H{"exports": views})
}

func (h *Handler) get(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionRead) {
		return
	}
	job, err := h.service.Get(c.Request.Context(), t.OrganizationID, c.Param("exportId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, newJobView(job))
}

func (h *Handler) downloadURL(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionDownload) {
		return
	}
	grant, err := h.service.RequestDownloadURL(c.Request.Context(), t, c.Param("exportId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	// The grant is returned once and never stored: no permanent URL, no
	// signed URL persisted anywhere, matching the documents download contract.
	respond.OK(c, gin.H{
		"downloadToken": grant.Token,
		"expiresAt":     grant.ExpiresAt.UTC().Format(time.RFC3339),
		"contentType":   grant.ContentType,
		"sizeBytes":     grant.SizeBytes,
	})
}

type cancelExportRequest struct {
	ExpectedVersion int64 `json:"expectedVersion"`
}

func (h *Handler) cancel(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionManage) {
		return
	}
	var body cancelExportRequest
	if !bind(c, &body) {
		return
	}
	body.ExpectedVersion = expectedVersionFrom(c, body.ExpectedVersion)
	h.runIdempotent(c, "POST /exports/:exportId/cancel", t, body, func() (int, any, error) {
		job, err := h.service.Cancel(c.Request.Context(), t.OrganizationID, c.Param("exportId"), body.ExpectedVersion)
		if err != nil {
			return 0, nil, err
		}
		return http.StatusOK, newJobView(job), nil
	})
}

func (h *Handler) replay(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionManage) {
		return
	}
	h.runIdempotent(c, "POST /exports/:exportId/replay", t, struct{}{}, func() (int, any, error) {
		if err := h.service.Replay(c.Request.Context(), t.OrganizationID, c.Param("exportId")); err != nil {
			return 0, nil, err
		}
		return http.StatusAccepted, gin.H{"status": "REQUEUED"}, nil
	})
}
