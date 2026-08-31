package idempotency

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

// AsAppError maps this package's sentinel errors onto the platform's
// apperrors envelope, with the exact wire code addendum §7.1 specifies:
// "Reusing the same key with a different request payload returns 409
// idempotency_conflict."
func AsAppError(err error) error {
	switch {
	case errors.Is(err, ErrInProgress):
		return apperrors.New(http.StatusConflict, "idempotency_conflict",
			"A request with this idempotency key is already in progress.", err)
	case errors.Is(err, ErrFingerprintMismatch):
		return apperrors.New(http.StatusConflict, "idempotency_conflict",
			"This idempotency key was already used with a different request.", err)
	default:
		return err
	}
}

// captureWriter buffers a handler's response instead of writing it to the
// wire immediately, so Execute can inspect the final status/body before
// deciding what to persist for replay.
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

// Execute wraps a handler with the idempotency contract for one endpoint.
// endpoint is a fixed label (e.g. "POST /customer/bookings"), not the raw gin
// route pattern, so a path-parameter naming change never silently changes
// which stored keys a request matches.
//
// This is a direct function call around run, not gin middleware: taxi's
// existing handlers call httpx.Abort(c, err) on failure, which only marks
// gin's error list for a separately-registered ErrorHandler middleware to
// write later. Relying on that ordering here would make correctness depend
// on exactly where in the middleware chain this function's caller is mounted
// relative to ErrorHandler. Instead, Execute takes over the write explicitly:
// it swaps in a capture writer, calls run synchronously, and if run left an
// error on the context, writes that error's response itself (via
// httpx.RespondError, which is idempotent -- it no-ops if something already
// wrote) before restoring the real writer. The full sequence -- claim, run,
// write, persist -- stays inside one function with no dependency on
// middleware order.
func Execute(c *gin.Context, store *Store, scopeID, endpoint string, run func()) {
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		httpx.Abort(c, apperrors.BadRequest("Idempotency-Key header is required.", nil))
		httpx.RespondError(c, lastError(c))
		return
	}
	if store == nil {
		httpx.Abort(c, apperrors.Unavailable("Idempotency store is unavailable.", nil))
		httpx.RespondError(c, lastError(c))
		return
	}

	raw, err := c.GetRawData()
	if err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body could not be read.", err))
		httpx.RespondError(c, lastError(c))
		return
	}
	if len(raw) > 0 && !json.Valid(raw) {
		httpx.Abort(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
		httpx.RespondError(c, lastError(c))
		return
	}

	record, err := store.Begin(c.Request.Context(), scopeID, endpoint, key, Fingerprint(c.Request.Method, endpoint, raw))
	if err != nil {
		httpx.Abort(c, AsAppError(err))
		httpx.RespondError(c, lastError(c))
		return
	}
	if record != nil {
		c.Data(record.ResponseStatus, "application/json", record.ResponseBody)
		return
	}

	// Restore the body so the wrapped handler can still bind it normally --
	// GetRawData above already drained the original reader.
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))

	original := c.Writer
	capture := &captureWriter{ResponseWriter: original}
	c.Writer = capture

	run()

	if len(c.Errors) > 0 && !capture.Written() {
		httpx.RespondError(c, lastError(c))
	}

	c.Writer = original
	status, payload := capture.Status(), capture.body.Bytes()

	if compErr := store.Complete(c.Request.Context(), scopeID, endpoint, key, status, payload); compErr != nil {
		// The handler already produced a real response; a bookkeeping failure
		// on the idempotency record must not turn that into a second error
		// response on top of one already written.
		httpx.Abort(c, compErr)
	}

	c.Data(status, "application/json", payload)
}

// lastError returns the most recently recorded gin error, or a generic
// internal error if run somehow left none despite Abort having been called.
func lastError(c *gin.Context) error {
	if len(c.Errors) == 0 {
		return apperrors.Internal("Something went wrong. Please try again.", nil)
	}
	return c.Errors.Last().Err
}
