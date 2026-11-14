package http

// Request plumbing shared by every route of this feature: tenant gate with
// branch narrowing, JSON binding, the Section 18 idempotent command envelope,
// If-Match versions and keyset paging. It is kept byte-compatible with the
// contract the routes have always served.

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cursor"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

type Route struct {
	Method, Path, OperationID, Permission, FeatureFlag, AuthClass string
	IdempotencyRequired, ExpectedVersionRequired                  bool
}

type Handler struct {
	s    UseCases
	a    *accesscontrol.Authorizer
	idem *idempotency.Store
}

func NewHandler(s UseCases, a *accesscontrol.Authorizer, i *idempotency.Store) *Handler {
	return &Handler{s: s, a: a, idem: i}
}
func (h *Handler) TenantRoutes() []Route { return TenantRoutes() }
func (h *Handler) RegisterTenantRoutes(g *gin.RouterGroup) {
	for _, r := range TenantRoutes() {
		r := r
		g.Handle(r.Method, r.Path, func(c *gin.Context) { h.handle(c, r) })
	}
}

// errBound marks a request whose body failed to bind; the error response is
// already written.
var errBound = errors.New("bound")

// listPage is a list response carrying the next keyset cursor.
type listPage struct {
	data any
	next string
}

func (h *Handler) gate(c *gin.Context, p string) (tenant.Context, bool) {
	t, ok := middleware.TenantFromGin(c)
	if !ok || h.a == nil {
		respond.Error(c, apperrors.Forbidden("Organization scope is required.", nil))
		return t, false
	}
	rv := int64(1)
	if x, ok := c.Get(middleware.TenantRoleVersionKey); ok {
		rv, _ = x.(int64)
	}
	scoped, e := h.a.RequireScoped(c, t, rv, p)
	if e == nil {
		// Narrowed to the branches where the permission is granted;
		// a caller-chosen ?branchId never widens it.
		t = scoped
		middleware.SetTenant(c, scoped)
	}
	if e != nil {
		respond.Error(c, e)
		return t, false
	}
	return t, true
}
func bind(c *gin.Context, v any) bool {
	if e := c.ShouldBindJSON(v); e != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", e))
		return false
	}
	return true
}
func (h *Handler) command(c *gin.Context, t tenant.Context, r Route, run func() (any, error)) {
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		respond.Error(c, apperrors.BadRequest("Idempotency-Key header is required.", nil))
		return
	}
	if h.idem == nil {
		respond.Error(c, apperrors.Internal("Idempotency store is unavailable.", nil))
		return
	}
	raw, e := c.GetRawData()
	if e != nil {
		respond.Error(c, apperrors.BadRequest("Request body could not be read.", e))
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	if len(raw) == 0 || !json.Valid(raw) {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
		return
	}
	ep := r.Method + " " + r.Path
	rec, e := h.idem.Begin(c, t.OrganizationID, ep, key, idempotency.Fingerprint(r.Method, ep, raw))
	if e != nil {
		respond.Error(c, idempotency.AsAppError(e))
		return
	}
	if rec != nil {
		c.Data(rec.ResponseStatus, "application/json; charset=utf-8", rec.ResponseBody)
		return
	}
	v, e := run()
	if e == errBound {
		return
	}
	status := http.StatusOK
	if strings.HasPrefix(r.OperationID, "create") || strings.HasPrefix(r.OperationID, "add") || strings.HasPrefix(r.OperationID, "assign") || strings.HasPrefix(r.OperationID, "record") || strings.HasPrefix(r.OperationID, "report") || strings.HasPrefix(r.OperationID, "register") {
		status = http.StatusCreated
	}
	var payload []byte
	if e != nil {
		ae := apperrors.From(e)
		status = ae.Status
		payload, _ = json.Marshal(gin.H{"error": respond.ErrorBody{Code: ae.Code, Message: ae.Message, Details: []apperrors.FieldViolation{}, RequestID: httpx.GetRequestID(c)}})
	} else {
		payload, _ = json.Marshal(gin.H{"data": v, "meta": gin.H{"requestId": httpx.GetRequestID(c)}})
	}
	if e = h.idem.Complete(c, t.OrganizationID, ep, key, status, payload); e != nil {
		respond.Error(c, e)
		return
	}
	c.Data(status, "application/json; charset=utf-8", payload)
}

// serve runs a route: commands through the idempotent envelope, reads
// straight to the success envelope (with meta.nextCursor for keyset lists).
func (h *Handler) serve(c *gin.Context, t tenant.Context, r Route, run func() (any, error)) {
	if r.IdempotencyRequired {
		h.command(c, t, r, run)
		return
	}
	v, e := run()
	if e == errBound {
		return
	}
	if e != nil {
		respond.Error(c, e)
		return
	}
	if p, ok := v.(listPage); ok {
		respond.List(c, p.data, p.next)
		return
	}
	respond.OK(c, v)
}
func ver(c *gin.Context, v int64) int64 {
	if v > 0 {
		return v
	}
	n, _ := strconv.ParseInt(strings.Trim(strings.TrimSpace(c.GetHeader("If-Match")), `"`), 10, 64)
	return n
}

type vb struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	Reason          string `json:"reason"`
}

// keyset reads ?cursor= into a keyset position. A malformed cursor, or a
// cursor sent together with the compatibility offset, is a validation error.
func keyset(c *gin.Context) (*time.Time, string, error) {
	if c.Query("cursor") != "" && c.Query("offset") != "" {
		return nil, "", apperrors.Validation("Use either cursor or offset paging, not both.",
			[]apperrors.FieldViolation{{Field: "cursor", Message: "Cannot be combined with offset."}})
	}
	position, ok, err := cursor.Decode(c.Query("cursor"))
	if err != nil {
		return nil, "", apperrors.Validation("The cursor is not valid.",
			[]apperrors.FieldViolation{{Field: "cursor", Message: "Malformed."}})
	}
	if !ok {
		return nil, "", nil
	}
	after := position.SortTime()
	return &after, position.LastID, nil
}
