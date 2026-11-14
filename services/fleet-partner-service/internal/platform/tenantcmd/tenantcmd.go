// Package tenantcmd holds the HTTP plumbing shared by the table-driven corporate
// handlers (engagement, communications, financeops): permission gating with
// branch narrowing, JSON binding, If-Match fallback and the idempotent command
// envelope. It holds no business rule; each feature's handler decides which
// use case to call.
package tenantcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

// ScopedAuthorizer narrows a tenant to the branches where a permission is
// granted. accesscontrol.Authorizer satisfies it.
type ScopedAuthorizer interface {
	RequireScoped(ctx context.Context, t tenant.Context, roleVersion int64, permission string) (tenant.Context, error)
}

// ErrBound means the handler already rendered a response (for example a
// binding error) and the command envelope must not render another.
var ErrBound = errors.New("request already rendered")

func isNil(a ScopedAuthorizer) bool {
	if a == nil {
		return true
	}
	v := reflect.ValueOf(a)
	return v.Kind() == reflect.Pointer && v.IsNil()
}

// Gate resolves the tenant and enforces permission. On success the tenant is
// narrowed to the branches where the permission is granted; a caller-chosen
// ?branchId never widens it.
func Gate(c *gin.Context, a ScopedAuthorizer, permission string) (tenant.Context, bool) {
	t, ok := middleware.TenantFromGin(c)
	if !ok || isNil(a) {
		respond.Error(c, apperrors.Forbidden("Organization scope is required.", nil))
		return t, false
	}
	rv := int64(1)
	if x, ok := c.Get(middleware.TenantRoleVersionKey); ok {
		rv, _ = x.(int64)
	}
	scoped, err := a.RequireScoped(c, t, rv, permission)
	if err != nil {
		respond.Error(c, err)
		return t, false
	}
	middleware.SetTenant(c, scoped)
	return scoped, true
}

// Bind decodes the JSON body, rendering a 400 on failure.
func Bind(c *gin.Context, v any) bool {
	if err := c.ShouldBindJSON(v); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", err))
		return false
	}
	return true
}

// ExpectedVersion returns v when set, otherwise the If-Match header value.
func ExpectedVersion(c *gin.Context, v int64) int64 {
	if v > 0 {
		return v
	}
	n, _ := strconv.ParseInt(strings.Trim(strings.TrimSpace(c.GetHeader("If-Match")), `"`), 10, 64)
	return n
}

// Command runs one idempotent tenant command. The Idempotency-Key header is
// required; a replay with the same key and body returns the recorded response;
// created selects 201 over 200 on success. run returning ErrBound means it has
// already rendered the response.
func Command(c *gin.Context, store *idempotency.Store, organizationID, method, path string, created bool, run func(key string) (any, error)) {
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		respond.Error(c, apperrors.BadRequest("Idempotency-Key header is required.", nil))
		return
	}
	if store == nil {
		respond.Error(c, apperrors.Internal("Idempotency store is unavailable.", nil))
		return
	}
	raw, err := c.GetRawData()
	if err != nil || (!json.Valid(raw) && len(bytes.TrimSpace(raw)) > 0) {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", err))
		return
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = []byte(`{}`)
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	endpoint := method + " " + path
	rec, err := store.Begin(c, organizationID, endpoint, key, idempotency.Fingerprint(method, endpoint, raw))
	if err != nil {
		respond.Error(c, idempotency.AsAppError(err))
		return
	}
	if rec != nil {
		c.Data(rec.ResponseStatus, "application/json; charset=utf-8", rec.ResponseBody)
		return
	}
	value, runErr := run(key)
	if errors.Is(runErr, ErrBound) {
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	var payload []byte
	if runErr != nil {
		ae := apperrors.From(runErr)
		status = ae.Status
		payload, _ = json.Marshal(gin.H{"error": respond.ErrorBody{Code: ae.Code, Message: ae.Message, Details: []apperrors.FieldViolation{}, RequestID: httpx.GetRequestID(c)}})
	} else {
		payload, _ = json.Marshal(gin.H{"data": value, "meta": gin.H{"requestId": httpx.GetRequestID(c)}})
	}
	if err := store.Complete(c, organizationID, endpoint, key, status, payload); err != nil {
		respond.Error(c, err)
		return
	}
	c.Data(status, "application/json; charset=utf-8", payload)
}
