package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/pricing-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/pricing-service/internal/platform/servicetrust"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

type capturedResponse struct {
	Status int    `json:"status"`
	Body   []byte `json:"body"`
}

type captureWriter struct {
	gin.ResponseWriter
	body bytes.Buffer
}

func (w *captureWriter) Write(value []byte) (int, error) {
	w.body.Write(value)
	return w.ResponseWriter.Write(value)
}

func (w *captureWriter) WriteString(value string) (int, error) {
	w.body.WriteString(value)
	return w.ResponseWriter.WriteString(value)
}

// requireIdempotency makes every state-changing Admin request single-execution.
// A crash after the domain transaction but before response persistence leaves
// the claim IN_PROGRESS, which fails closed instead of replaying the mutation.
func requireIdempotency(store *idempotency.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodGet || c.FullPath() == "/internal/pricing/v1/admin/simulate" {
			c.Next()
			return
		}
		if store == nil {
			httpx.Abort(c, apperrors.Unavailable("Pricing administrative idempotency is unavailable.", errors.New("idempotency store not configured")))
			return
		}
		key := strings.TrimSpace(c.GetHeader("X-Idempotency-Key"))
		if key == "" || len(key) > 128 || strings.ContainsAny(key, "\r\n") {
			httpx.Abort(c, apperrors.BadRequest("X-Idempotency-Key is required for this administrative command.", errors.New("invalid idempotency key")))
			return
		}
		body, err := c.GetRawData()
		if err != nil {
			httpx.Abort(c, apperrors.BadRequest("Could not read the request body.", err))
			return
		}
		c.Request.Body = http.NoBody
		if len(body) > 0 {
			c.Request.Body = ioNopCloser{bytes.NewReader(body)}
		}
		caller, endpoint := servicetrust.CallerFrom(c), c.Request.Method+" "+c.FullPath()
		outcome, err := store.Begin(c.Request.Context(), caller, endpoint, key, idempotency.Fingerprint(c.Request.Method, c.FullPath(), body))
		if err != nil {
			if errors.Is(err, idempotency.ErrInProgress) || errors.Is(err, idempotency.ErrFingerprintMismatch) {
				httpx.Abort(c, apperrors.Conflict("This idempotency key is already in use.", err))
			} else {
				httpx.Abort(c, apperrors.Unavailable("Pricing administrative idempotency is unavailable.", err))
			}
			return
		}
		if !outcome.Proceed {
			var stored capturedResponse
			if json.Unmarshal(outcome.Response, &stored) != nil || stored.Status < 200 || stored.Status > 499 {
				httpx.Abort(c, apperrors.Unavailable("Stored administrative outcome is invalid.", errors.New("invalid idempotency response")))
				return
			}
			c.Data(stored.Status, "application/json; charset=utf-8", stored.Body)
			c.Abort()
			return
		}
		writer := &captureWriter{ResponseWriter: c.Writer}
		c.Writer = writer
		c.Next()
		status := writer.Status()
		if status >= 500 {
			_ = store.Release(c.Request.Context(), caller, endpoint, key)
			return
		}
		encoded, err := json.Marshal(capturedResponse{Status: status, Body: writer.body.Bytes()})
		if err == nil {
			// Failure leaves the original claim IN_PROGRESS. That is a deliberate
			// UNKNOWN_OUTCOME fence and never authorizes automatic replay.
			_ = store.Complete(c.Request.Context(), nil, caller, endpoint, key, encoded)
		}
	}
}

type ioNopCloser struct{ *bytes.Reader }

func (ioNopCloser) Close() error { return nil }
