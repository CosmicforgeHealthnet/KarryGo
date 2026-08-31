package idempotency

import (
	"bytes"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/platform/authctx"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

const maxCommandBody = 1 << 20

type replayRecorder struct {
	gin.ResponseWriter
	body bytes.Buffer
}

func (w *replayRecorder) Write(data []byte) (int, error) {
	w.body.Write(data)
	return w.ResponseWriter.Write(data)
}

func (w *replayRecorder) WriteString(data string) (int, error) {
	w.body.WriteString(data)
	return w.ResponseWriter.WriteString(data)
}

// CommandMiddleware applies the service-wide Vendor idempotency contract to
// every state-changing route in a module group. Read routes pass through.
// A nil store is intentionally a no-op so route-contract tests can register
// handlers without constructing infrastructure; production wiring always
// supplies the shared store.
func CommandMiddleware(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		if store == nil || c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead || c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}

		staff, ok := authctx.FromContext(c)
		if !ok {
			httpx.RespondError(c, apperrors.Unauthorized("Authentication is required.", nil))
			c.Abort()
			return
		}

		raw, err := io.ReadAll(io.LimitReader(c.Request.Body, maxCommandBody+1))
		if err != nil || len(raw) > maxCommandBody {
			httpx.RespondError(c, apperrors.BadRequest("Could not read the command body.", err))
			c.Abort()
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(raw))

		key := c.GetHeader("Idempotency-Key")
		endpoint := c.Request.Method + " " + c.FullPath()
		record, err := store.Begin(c, staff.VendorID, endpoint, key, Fingerprint(c.Request.Method, c.Request.URL.Path, raw))
		if err != nil {
			if errors.Is(err, ErrFingerprintMismatch) || errors.Is(err, ErrInProgress) {
				httpx.RespondError(c, apperrors.Conflict("This idempotency key is already in use for another request.", err))
			} else {
				httpx.RespondError(c, apperrors.BadRequest("Idempotency-Key is required.", err))
			}
			c.Abort()
			return
		}
		if record != nil {
			c.Data(record.ResponseStatus, "application/json", record.ResponseBody)
			c.Abort()
			return
		}

		writer := &replayRecorder{ResponseWriter: c.Writer}
		c.Writer = writer
		c.Next()
		if err = store.Complete(c, staff.VendorID, endpoint, key, c.Writer.Status(), writer.body.Bytes()); err != nil {
			// The response may already be committed. Releasing the claim is safer
			// than leaving an IN_PROGRESS record that permanently blocks retries.
			_ = store.Release(c, staff.VendorID, endpoint, key)
		}
	}
}
