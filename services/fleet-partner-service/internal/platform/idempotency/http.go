package idempotency

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/shared/go/apperrors"
)

type captureWriter struct {
	gin.ResponseWriter
	body   bytes.Buffer
	status int
}

func (w *captureWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
}
func (w *captureWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(b)
}
func (w *captureWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }
func (w *captureWriter) Status() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}
func (w *captureWriter) Size() int     { return w.body.Len() }
func (w *captureWriter) Written() bool { return w.status != 0 }

// ExecuteHTTP applies the shared durable store to an HTTP command and records
// the exact response. The callback is invoked only for the first fingerprint.
func ExecuteHTTP(c *gin.Context, store *Store, organizationID, endpoint string, run func()) {
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		respond.Error(c, apperrors.BadRequest("Idempotency-Key header is required.", nil))
		return
	}
	if store == nil {
		respond.Error(c, apperrors.Unavailable("Idempotency store is unavailable.", nil))
		return
	}
	raw, err := c.GetRawData()
	if err != nil || !json.Valid(raw) {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", err))
		return
	}
	record, err := store.Begin(c, organizationID, endpoint, key, Fingerprint(c.Request.Method, endpoint, raw))
	if err != nil {
		respond.Error(c, AsAppError(err))
		return
	}
	if record != nil {
		c.Data(record.ResponseStatus, "application/json", record.ResponseBody)
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	original := c.Writer
	capture := &captureWriter{ResponseWriter: original}
	c.Writer = capture
	run()
	c.Writer = original
	status, payload := capture.Status(), capture.body.Bytes()
	if err = store.Complete(c, organizationID, endpoint, key, status, payload); err != nil {
		respond.Error(c, err)
		return
	}
	c.Data(status, original.Header().Get("Content-Type"), payload)
}
