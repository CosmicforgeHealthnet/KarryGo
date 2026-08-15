package crm

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/httpx"

	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
)

// Section 8H — export HTTP surface.
//
// Five routes. What is absent is as deliberate as what is present: there is no
// query parameter, no `select`, no `where`, no `sql`, no `columns` and no
// filename or path anywhere a caller can reach. A request names a dataset from
// a closed registry and narrows it along that dataset's own declared
// dimensions; everything else is written in Go.
//
// Download is a POST because it is a state-changing act, not a fetch: it spends
// a step-up grant, writes an audit record and asks the private store to mint a
// short-lived token. Modelling it as a GET would make it cacheable, linkable
// and repeatable, which is exactly what a release of customer data must not be.

// Section 8H export permissions.
const (
	PermExportsRead     = "vendor.crm.exports.read"
	PermExportsCreate   = "vendor.crm.exports.create"
	PermExportsDownload = "vendor.crm.exports.download"
	PermExportsCancel   = "vendor.crm.exports.cancel"
)

// ExportHandler serves the Section 8H export routes.
type ExportHandler struct {
	*SegmentationHandler
}

// NewExportHandler wires the handler.
func NewExportHandler(service *Service, store *idempotency.Store) *ExportHandler {
	return &ExportHandler{SegmentationHandler: NewSegmentationHandler(service, store)}
}

func exportID(c *gin.Context) string { return strings.TrimSpace(c.Param("exportId")) }

// ExportView is the wire shape of an export job.
//
// There is no URL, no path and no artifact location. `artifact_ref` is
// deliberately absent too: it is an internal storage handle, and publishing it
// would invite a client to try reaching the store directly.
type ExportView struct {
	ExportRef       string       `json:"export_ref"`
	ExportType      string       `json:"export_type"`
	Format          string       `json:"format"`
	Status          string       `json:"status"`
	Scope           string       `json:"scope"`
	BranchRef       *string      `json:"branch_ref,omitempty"`
	RequestedByRef  string       `json:"requested_by_staff_ref"`
	Filter          ExportFilter `json:"filter"`
	FieldSetVersion int          `json:"field_set_version"`
	Fields          []string     `json:"fields"`
	RowLimit        int          `json:"row_limit"`
	RowCount        int64        `json:"row_count"`
	ChecksumSHA256  *string      `json:"checksum_sha256,omitempty"`
	SizeBytes       *int64       `json:"size_bytes,omitempty"`

	RequestedAt time.Time  `json:"requested_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	CancelledAt *time.Time `json:"cancelled_at,omitempty"`
	FailedAt    *time.Time `json:"failed_at,omitempty"`
	ErrorCode   *string    `json:"error_code,omitempty"`
	Version     int64      `json:"version"`
}

// ExportViewOf renders an export job for the wire.
func ExportViewOf(e Export) ExportView {
	return ExportView{
		ExportRef: e.ID, ExportType: e.ExportType, Format: e.Format, Status: e.Status,
		Scope: e.Scope, BranchRef: e.BranchID, RequestedByRef: e.RequestedBy,
		Filter: e.Filter, FieldSetVersion: e.FieldSetVersion, Fields: ExportFieldNames(e.ExportType),
		RowLimit: e.RowLimit, RowCount: e.RowCount, ChecksumSHA256: e.Checksum, SizeBytes: e.ArtifactBytes,
		RequestedAt: e.RequestedAt, StartedAt: e.StartedAt, CompletedAt: e.CompletedAt,
		ExpiresAt: e.ExpiresAt, CancelledAt: e.CancelledAt, FailedAt: e.FailedAt,
		ErrorCode: e.ErrorCode, Version: e.Version,
	}
}

// ListExports pages the caller's visible export jobs.
func (h *ExportHandler) ListExports(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermExportsRead)
	if !ok {
		return
	}
	limit, offset, err := parsePage(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	exports, hasMore, err := h.service.ListExports(c, actor, scope, ExportListOptions{
		Limit: limit, Offset: offset,
		Status: c.Query("status"), ExportType: c.Query("export_type")})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	views := make([]ExportView, 0, len(exports))
	for _, export := range exports {
		views = append(views, ExportViewOf(export))
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"exports": views, "limit": limit, "offset": offset, "has_more": hasMore}})
}

// GetExport reads one export job.
func (h *ExportHandler) GetExport(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermExportsRead)
	if !ok {
		return
	}
	export, err := h.service.GetExport(c, actor, scope, exportID(c))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": ExportViewOf(export)})
}

type createExportRequest struct {
	ExportType    string       `json:"export_type"`
	Format        string       `json:"format"`
	ScopeType     string       `json:"scope_type"`
	BranchRef     string       `json:"branch_ref"`
	RowLimit      int          `json:"row_limit"`
	Filter        ExportFilter `json:"filter"`
	StepUpGrantID string       `json:"step_up_grant_id"`
}

// CreateExport queues a durable export job.
func (h *ExportHandler) CreateExport(c *gin.Context) {
	h.command(c, http.StatusAccepted, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermExportsCreate)
		if err != nil {
			return nil, err
		}
		var req createExportRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		export, err := h.service.CreateExport(c, actor, scope, CreateExportInput{
			ExportType: req.ExportType, Format: req.Format, ScopeType: req.ScopeType,
			BranchRef: req.BranchRef, RowLimit: req.RowLimit, Filter: req.Filter,
			StepUpGrantID: req.StepUpGrantID, SessionID: actor.SessionID})
		if err != nil {
			return nil, err
		}
		return ExportViewOf(export), nil
	})
}

type cancelExportRequest struct {
	ExpectedVersion int64 `json:"expected_version"`
}

// CancelExport stops a job and revokes its artifact.
func (h *ExportHandler) CancelExport(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermExportsCancel)
		if err != nil {
			return nil, err
		}
		var req cancelExportRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		export, err := h.service.CancelExport(c, actor, scope, exportID(c), req.ExpectedVersion)
		if err != nil {
			return nil, err
		}
		return ExportViewOf(export), nil
	})
}

type downloadExportRequest struct {
	StepUpGrantID string `json:"step_up_grant_id"`
}

// DownloadExport releases short-lived private access to a completed artifact.
func (h *ExportHandler) DownloadExport(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermExportsDownload)
		if err != nil {
			return nil, err
		}
		var req downloadExportRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		access, export, err := h.service.AuthorizeExportDownload(c, actor, scope, exportID(c),
			ExportAccessInput{StepUpGrantID: req.StepUpGrantID, SessionID: actor.SessionID})
		if err != nil {
			return nil, err
		}
		// The token and path travel once, in this response. Neither is stored,
		// logged or reusable after the store's short window closes.
		return gin.H{
			"export": ExportViewOf(export),
			"access": gin.H{
				"download_path":  access.Path,
				"download_token": access.Token,
				"expires_at":     access.ExpiresAt,
				"media_type":     access.MediaType,
				"size_bytes":     access.Bytes,
			},
		}, nil
	})
}

// RegisterExportRoutes wires Section 8H exports.
//
// Five routes and no internal one: no other service produces or consumes a
// Vendor CRM export, so there is nothing for an internal ingress to receive.
func RegisterExportRoutes(group *gin.RouterGroup, h *ExportHandler, flags Flags) {
	if !flags.CRMEnabled || !flags.ExportsEnabled {
		return
	}
	exports := group.Group("/crm/exports")
	exports.GET("", h.ListExports)
	exports.POST("", h.CreateExport)
	exports.GET("/:exportId", h.GetExport)
	exports.POST("/:exportId/cancel", h.CancelExport)
	exports.POST("/:exportId/download", h.DownloadExport)
}
