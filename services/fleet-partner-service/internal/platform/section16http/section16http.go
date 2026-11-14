// Package section16http holds the gin plumbing the three Section 16 handler
// packages (dispatch, routes, tracking) share. Binding and the If-Match rule
// delegate to tenantcmd; the gate message, the parsed-body idempotency
// fingerprint, use-case-chosen status codes, field details, the 409 mapping
// of the persistence conflict sentinel and the paging error text are the
// Section 16 contract and stay here unchanged. It exists so the split features keep
// one implementation of the permission gate, the idempotent command
// envelope, the If-Match version rule and list paging. It holds no domain
// logic.
package section16http

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cursor"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/pagination"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenantcmd"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

// Authorizer narrows a tenant to the branches where a permission is granted.
type Authorizer interface {
	RequireScoped(ctx context.Context, t tenant.Context, roleVersion int64, permission string) (tenant.Context, error)
}

// Kit carries the shared handler collaborators.
type Kit struct {
	Authorizer  Authorizer
	Idempotency *idempotency.Store
	// Conflict is the persistence sentinel answered as HTTP 409.
	Conflict error
}

// Gate resolves the tenant and requires permission. The tenant is narrowed
// to the branches where the permission is granted; a caller-chosen
// ?branchId never widens it.
func (k Kit) Gate(c *gin.Context, permission string) (tenant.Context, bool) {
	t, ok := middleware.TenantFromGin(c)
	if !ok {
		respond.Error(c, apperrors.Forbidden("Organization scope is required.", nil))
		return t, false
	}
	if k.Authorizer == nil {
		respond.Error(c, apperrors.Forbidden("Permission denied.", nil))
		return t, false
	}
	rv := int64(1)
	if raw, ok := c.Get(middleware.TenantRoleVersionKey); ok {
		rv, _ = raw.(int64)
	}
	scoped, e := k.Authorizer.RequireScoped(c, t, rv, permission)
	if e != nil {
		respond.Error(c, e)
		return t, false
	}
	middleware.SetTenant(c, scoped)
	return scoped, true
}

// Version returns the body's expected version, else the If-Match header's.
// It is the same rule every table-driven handler uses (tenantcmd).
func Version(c *gin.Context, body int64) int64 { return tenantcmd.ExpectedVersion(c, body) }

// Bind decodes the JSON body, answering 400 when it is not valid JSON.
func Bind(c *gin.Context, v any) bool { return tenantcmd.Bind(c, v) }

func (k Kit) mapErr(e error) error {
	if k.Conflict != nil && errors.Is(e, k.Conflict) {
		return apperrors.Conflict("Expected version or idempotency fingerprint conflicts with current state.", e)
	}
	return e
}

// Command runs an idempotent write: the Idempotency-Key header is required,
// a replay returns the stored response and the outcome is stored.
func (k Kit) Command(c *gin.Context, t tenant.Context, endpoint string, body any, run func() (int, any, error)) {
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		respond.Error(c, apperrors.BadRequest("Idempotency-Key header is required.", nil))
		return
	}
	if k.Idempotency == nil {
		respond.Error(c, apperrors.Internal("Idempotency store is unavailable.", nil))
		return
	}
	raw, _ := json.Marshal(body)
	fp := idempotency.Fingerprint(c.Request.Method, endpoint, raw)
	rec, e := k.Idempotency.Begin(c, t.OrganizationID, endpoint, key, fp)
	if e != nil {
		respond.Error(c, idempotency.AsAppError(e))
		return
	}
	if rec != nil {
		c.Data(rec.ResponseStatus, "application/json; charset=utf-8", rec.ResponseBody)
		return
	}
	status, data, e := run()
	var payload []byte
	if e != nil {
		appErr := apperrors.From(k.mapErr(e))
		details := appErr.Fields
		if details == nil {
			details = []apperrors.FieldViolation{}
		}
		payload, _ = json.Marshal(gin.H{"error": respond.ErrorBody{Code: appErr.Code, Message: appErr.Message, Details: details, RequestID: httpx.GetRequestID(c)}})
		status = appErr.Status
	} else {
		payload, _ = json.Marshal(gin.H{"data": data, "meta": gin.H{"requestId": httpx.GetRequestID(c)}})
	}
	if e = k.Idempotency.Complete(c, t.OrganizationID, endpoint, key, status, payload); e != nil {
		respond.Error(c, e)
		return
	}
	c.Data(status, "application/json; charset=utf-8", payload)
}

// Write answers a read: the standard success envelope, or the error with the
// conflict sentinel mapped to 409.
func (k Kit) Write(c *gin.Context, v any, e error) {
	if e != nil {
		respond.Error(c, k.mapErr(e))
		return
	}
	respond.OK(c, v)
}

// WriteList answers a paged read with meta.nextCursor when another page
// exists.
func (k Kit) WriteList(c *gin.Context, v any, next string, e error) {
	if e != nil {
		respond.Error(c, k.mapErr(e))
		return
	}
	respond.List(c, v, next)
}

// Page is a validated list position: an offset page (kept for
// compatibility) or a keyset position After/AfterID, never both.
type Page struct {
	Limit, Offset int
	After         *time.Time
	AfterID       string
}

// ParsePage validates limit, offset and the opaque cursor. Malformed values
// and a cursor sent together with an offset are a validation error (422).
func ParsePage(c *gin.Context) (Page, error) {
	page, e := pagination.Parse(c.Query("limit"), c.Query("offset"))
	if e != nil {
		return Page{}, e
	}
	out := Page{Limit: page.Limit, Offset: page.Offset}
	raw := c.Query("cursor")
	if raw == "" {
		return out, nil
	}
	if strings.TrimSpace(c.Query("offset")) != "" {
		return Page{}, apperrors.Validation("Send either cursor or offset, not both.", []apperrors.FieldViolation{{Field: "cursor", Message: "Cannot be combined with offset."}})
	}
	position, ok, e := cursor.Decode(raw)
	if e != nil || !ok {
		return Page{}, apperrors.Validation("The cursor is not valid.", []apperrors.FieldViolation{{Field: "cursor", Message: "Malformed."}})
	}
	after := position.SortTime()
	out.After, out.AfterID = &after, position.LastID
	return out, nil
}
