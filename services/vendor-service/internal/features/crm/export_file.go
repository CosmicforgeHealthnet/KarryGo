package crm

import (
	"context"
	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
)

type ExportContentReader interface {
	ReadExportContent(context.Context, string, string, string, int64) ([]byte, error)
}

func (h *ExportHandler) DownloadFile(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermExportsDownload)
	if !ok {
		return
	}
	var req downloadExportRequest
	if err := decodeBodyFromRequest(c, &req); err != nil {
		httpx.RespondError(c, err)
		return
	}
	reader, ok := h.service.artifacts.(ExportContentReader)
	if !ok {
		httpx.RespondError(c, errExportStorageUnavailable())
		return
	}

	key := c.GetHeader("Idempotency-Key")
	endpoint := c.Request.Method + " " + c.FullPath()
	record, err := h.idempotency.Begin(c, actor.VendorID, endpoint, key, idempotency.Fingerprint(c.Request.Method, c.Request.URL.Path, []byte(req.StepUpGrantID+"|"+actor.StaffID)))
	if err != nil || record != nil {
		httpx.RespondError(c, apperrors.Conflict("Verify again to request a new file download.", nil))
		return
	}
	completed := false
	defer func() {
		if !completed {
			_ = h.idempotency.Release(c, actor.VendorID, endpoint, key)
		}
	}()
	access, export, err := h.service.AuthorizeExportDownload(c, actor, scope, exportID(c), ExportAccessInput{StepUpGrantID: req.StepUpGrantID, SessionID: actor.SessionID})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	content, err := reader.ReadExportContent(c.Request.Context(), actor.VendorID, access.Ref, access.Token, h.service.exports.MaxBytes)
	if err != nil || export.ArtifactBytes == nil || int64(len(content)) != *export.ArtifactBytes {
		httpx.RespondError(c, errExportStorageUnavailable())
		return
	}
	sum := sha256.Sum256(content)
	if export.Checksum == nil || hex.EncodeToString(sum[:]) != *export.Checksum {
		httpx.RespondError(c, errExportStorageUnavailable())
		return
	}
	if err = h.idempotency.Complete(c, actor.VendorID, endpoint, key, http.StatusOK, []byte(`{"success":true,"data":{"downloaded":true}}`)); err != nil {
		httpx.RespondError(c, err)
		return
	}
	completed = true
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Disposition", "attachment")
	c.Data(http.StatusOK, access.MediaType, content)
}
func decodeBodyFromRequest(c *gin.Context, out any) error {
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return apperrors.BadRequest("Could not read request body.", nil)
	}
	return decodeBody(raw, out)
}
func exportCreateFingerprint(method, path string, actor Actor, request createExportRequest) string {
	request.StepUpGrantID = ""
	raw, _ := json.Marshal(struct {
		StaffID string              `json:"staff_id"`
		Request createExportRequest `json:"request"`
	}{actor.StaffID, request})
	return idempotency.Fingerprint(method, path, raw)
}
func (h *ExportHandler) createCommand(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermExportsCreate)
	if !ok {
		return
	}
	var req createExportRequest
	if err := decodeBodyFromRequest(c, &req); err != nil {
		httpx.RespondError(c, err)
		return
	}
	key := c.GetHeader("Idempotency-Key")
	endpoint := c.Request.Method + " " + c.FullPath()
	record, err := h.idempotency.Begin(c, actor.VendorID, endpoint, key, exportCreateFingerprint(c.Request.Method, c.Request.URL.Path, actor, req))
	if err != nil {
		if errors.Is(err, idempotency.ErrFingerprintMismatch) || errors.Is(err, idempotency.ErrInProgress) {
			httpx.RespondError(c, apperrors.Conflict("This export submission is already in progress or has different settings.", err))
		} else {
			httpx.RespondError(c, apperrors.BadRequest("Idempotency-Key is required.", err))
		}
		return
	}
	if record != nil {
		var prior struct {
			Data ExportView `json:"data"`
		}
		if json.Unmarshal(record.ResponseBody, &prior) != nil {
			httpx.RespondError(c, errExportStorageUnavailable())
			return
		}
		if _, err = h.service.GetExport(c, actor, scope, prior.Data.ExportRef); err != nil {
			httpx.RespondError(c, err)
			return
		}
		if err = h.service.consumeExportStepUp(c, actor, actor.SessionID, req.StepUpGrantID, StepUpPurposeExportCreate); err != nil {
			httpx.RespondError(c, err)
			return
		}
		c.Data(record.ResponseStatus, "application/json", record.ResponseBody)
		return
	}
	export, err := h.service.CreateExport(c, actor, scope, CreateExportInput{ExportType: req.ExportType, Format: req.Format, ScopeType: req.ScopeType, BranchRef: req.BranchRef, RowLimit: req.RowLimit, Filter: req.Filter, StepUpGrantID: req.StepUpGrantID, SessionID: actor.SessionID})
	if err != nil {
		_ = h.idempotency.Release(c, actor.VendorID, endpoint, key)
		httpx.RespondError(c, err)
		return
	}
	body, _ := json.Marshal(gin.H{"success": true, "data": ExportViewOf(export)})
	if err = h.idempotency.Complete(c, actor.VendorID, endpoint, key, http.StatusAccepted, body); err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.Data(http.StatusAccepted, "application/json", body)
}
