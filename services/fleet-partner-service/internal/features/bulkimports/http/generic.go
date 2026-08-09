// Section 25C: the generic import HTTP surface, additive alongside the
// legacy /orders/imports routes in handlers.go/manifest.go (untouched).
package http

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/bulkimports/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
)

const (
	PermissionValidate = "imports.validate"
	PermissionCommit   = "imports.commit"
	PermissionRetry    = "imports.retry"
)

type createImportReq struct {
	BranchID      string `json:"branchId"`
	ImportType    string `json:"importType"`
	MediaFileID   string `json:"mediaFileId"`
	FileFormat    string `json:"fileFormat"`
	Mode          string `json:"mode"`
	SchemaVersion int    `json:"schemaVersion"`
}

func (h *Handler) createImport(c *gin.Context) {
	t, ok := ten(c)
	if !ok || !h.auth(c, t, PermissionManage) {
		return
	}
	var b createImportReq
	if !bind(c, &b) {
		return
	}
	h.run(c, "POST /imports", t, b, func() (int, any, error) {
		v, e := h.s.CreateGeneric(c.Request.Context(), t, usecases.CreateGenericInput{
			BranchID: b.BranchID, ImportType: b.ImportType, MediaFileID: b.MediaFileID,
			FileFormat: b.FileFormat, Mode: b.Mode, SchemaVersion: b.SchemaVersion,
		})
		return http.StatusAccepted, v, e
	})
}

func (h *Handler) listImports(c *gin.Context) {
	t, ok := ten(c)
	if !ok || !h.auth(c, t, PermissionRead) {
		return
	}
	v, e := h.s.ListGeneric(c.Request.Context(), t)
	if e != nil {
		respond.Error(c, e)
		return
	}
	respond.OK(c, v)
}

func (h *Handler) getImport(c *gin.Context) {
	t, ok := ten(c)
	if !ok || !h.auth(c, t, PermissionRead) {
		return
	}
	v, rows, e := h.s.GetGeneric(c.Request.Context(), t, c.Param("importId"))
	if e != nil {
		respond.Error(c, e)
		return
	}
	respond.OK(c, gin.H{"import": v, "rows": rows})
}

func (h *Handler) listImportRows(c *gin.Context) {
	t, ok := ten(c)
	if !ok || !h.auth(c, t, PermissionRead) {
		return
	}
	_, rows, e := h.s.GetGeneric(c.Request.Context(), t, c.Param("importId"))
	if e != nil {
		respond.Error(c, e)
		return
	}
	respond.OK(c, gin.H{"rows": rows})
}

func (h *Handler) validateImport(c *gin.Context) {
	t, ok := ten(c)
	if !ok || !h.auth(c, t, PermissionValidate) {
		return
	}
	h.run(c, "POST /imports/:importId/validate", t, struct{}{}, func() (int, any, error) {
		v, e := h.s.Validate(c.Request.Context(), t, c.Param("importId"))
		return http.StatusAccepted, v, e
	})
}

func (h *Handler) commitImport(c *gin.Context) {
	t, ok := ten(c)
	if !ok || !h.auth(c, t, PermissionCommit) {
		return
	}
	h.run(c, "POST /imports/:importId/commit", t, struct{}{}, func() (int, any, error) {
		v, e := h.s.Commit(c.Request.Context(), t, c.Param("importId"))
		return http.StatusAccepted, v, e
	})
}

func (h *Handler) retryImport(c *gin.Context) {
	t, ok := ten(c)
	if !ok || !h.auth(c, t, PermissionRetry) {
		return
	}
	h.run(c, "POST /imports/:importId/retry", t, struct{}{}, func() (int, any, error) {
		count, e := h.s.RetryImport(c.Request.Context(), t, c.Param("importId"))
		return http.StatusAccepted, gin.H{"status": "REQUEUED", "rowsRequeued": count}, e
	})
}

func (h *Handler) retryImportRow(c *gin.Context) {
	t, ok := ten(c)
	if !ok || !h.auth(c, t, PermissionRetry) {
		return
	}
	h.run(c, "POST /imports/:importId/rows/:rowId/retry", t, struct{}{}, func() (int, any, error) {
		e := h.s.RetryRow(c.Request.Context(), t, c.Param("importId"), c.Param("rowId"))
		return http.StatusAccepted, gin.H{"status": "REQUEUED"}, e
	})
}

type cancelImportReq struct {
	ExpectedVersion int64 `json:"expectedVersion"`
}

func (h *Handler) cancelImport(c *gin.Context) {
	t, ok := ten(c)
	if !ok || !h.auth(c, t, PermissionManage) {
		return
	}
	var b cancelImportReq
	if !bind(c, &b) {
		return
	}
	b.ExpectedVersion = ver(c, b.ExpectedVersion)
	h.run(c, "POST /imports/:importId/cancel", t, b, func() (int, any, error) {
		v, e := h.s.CancelGeneric(c.Request.Context(), t, c.Param("importId"), b.ExpectedVersion)
		return http.StatusOK, v, e
	})
}

func (h *Handler) replayImport(c *gin.Context) {
	t, ok := ten(c)
	if !ok || !h.auth(c, t, PermissionRetry) {
		return
	}
	h.run(c, "POST /imports/:importId/replay", t, struct{}{}, func() (int, any, error) {
		e := h.s.ReplayGeneric(c.Request.Context(), t, c.Param("importId"))
		return http.StatusAccepted, gin.H{"status": "REQUEUED"}, e
	})
}

type correctImportRowReq struct {
	ExpectedVersion int               `json:"expectedVersion"`
	RowData         map[string]string `json:"rowData"`
}

// correctImportRow implements POST .../rows/:rowId/corrections. It requires
// PermissionRetry (imports.retry) — the spec allows either that or a
// dedicated correction permission, and this domain already uses imports.retry
// for the other row-level recovery action (retryImportRow), so a second,
// narrower permission would separate two closely related recovery actions
// without a stated reason to.
func (h *Handler) correctImportRow(c *gin.Context) {
	t, ok := ten(c)
	if !ok || !h.auth(c, t, PermissionRetry) {
		return
	}
	var b correctImportRowReq
	if !bind(c, &b) {
		return
	}
	h.run(c, "POST /imports/:importId/rows/:rowId/corrections", t, b, func() (int, any, error) {
		v, e := h.s.SubmitCorrection(c.Request.Context(), t, usecases.SubmitCorrectionInput{
			ImportID: c.Param("importId"), RowID: c.Param("rowId"),
			ExpectedVersion: b.ExpectedVersion, RowData: b.RowData,
		})
		return http.StatusCreated, v, e
	})
}
