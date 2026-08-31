package idempotency

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
)

// CustomerCommandMiddleware reuses the service-wide idempotency Store but scopes
// keys by the authenticated Customer Service subject rather than Vendor staff.
//
// Authentication middleware must run first. This middleware independently
// verifies the expected customer claim shape as a fail-closed boundary.
//
// Read routes pass through unchanged.
func CustomerCommandMiddleware(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		if store == nil ||
			c.Request.Method == http.MethodGet ||
			c.Request.Method == http.MethodHead ||
			c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}

		claims, ok := sharedauth.ClaimsFromContext(c)
		if !ok || strings.TrimSpace(claims.Subject) == "" {
			httpx.RespondError(
				c,
				apperrors.Unauthorized(
					"Authentication is required.",
					nil,
				),
			)
			c.Abort()
			return
		}

		if claims.Type != sharedauth.TokenTypeAccess ||
			claims.Role != "customer" ||
			claims.Service != "customer" {
			httpx.RespondError(
				c,
				apperrors.Forbidden(
					"You do not have access to this action.",
					nil,
				),
			)
			c.Abort()
			return
		}

		customerID := strings.TrimSpace(claims.Subject)

		raw, err := io.ReadAll(
			io.LimitReader(
				c.Request.Body,
				maxCommandBody+1,
			),
		)
		if err != nil || len(raw) > maxCommandBody {
			httpx.RespondError(
				c,
				apperrors.BadRequest(
					"Could not read the command body.",
					err,
				),
			)
			c.Abort()
			return
		}

		c.Request.Body = io.NopCloser(
			bytes.NewReader(raw),
		)

		key := c.GetHeader("Idempotency-Key")
		endpoint := c.Request.Method + " " + c.FullPath()

		record, err := store.Begin(
			c,
			customerID,
			endpoint,
			key,
			Fingerprint(
				c.Request.Method,
				c.Request.URL.Path,
				raw,
			),
		)
		if err != nil {
			switch {
			case errors.Is(err, ErrFingerprintMismatch),
				errors.Is(err, ErrInProgress):
				httpx.RespondError(
					c,
					apperrors.Conflict(
						"This idempotency key is already in use for another request.",
						err,
					),
				)

			default:
				httpx.RespondError(
					c,
					apperrors.BadRequest(
						"Idempotency-Key is required.",
						err,
					),
				)
			}

			c.Abort()
			return
		}

		if record != nil {
			c.Data(
				record.ResponseStatus,
				"application/json",
				record.ResponseBody,
			)
			c.Abort()
			return
		}

		writer := &replayRecorder{
			ResponseWriter: c.Writer,
		}

		c.Writer = writer
		c.Next()

		if err = store.Complete(
			c,
			customerID,
			endpoint,
			key,
			c.Writer.Status(),
			writer.body.Bytes(),
		); err != nil {
			_ = store.Release(
				c,
				customerID,
				endpoint,
				key,
			)
		}
	}
}
