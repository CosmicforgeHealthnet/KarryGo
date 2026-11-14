package http

// Request plumbing for this feature's table-driven routes. The behaviour lives
// in internal/platform/tenantcmd (shared with the other table-driven
// corporate handlers); these wrappers only bind it to this package's Route
// and Handler so call sites stay unchanged.

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenantcmd"
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
var errBound = tenantcmd.ErrBound

// listPage is a list response carrying the next keyset cursor.
type listPage = tenantcmd.Page

func (h *Handler) gate(c *gin.Context, p string) (tenant.Context, bool) {
	if h.a == nil {
		return tenantcmd.Gate(c, nil, p)
	}
	return tenantcmd.Gate(c, h.a, p)
}
func bind(c *gin.Context, v any) bool { return tenantcmd.Bind(c, v) }

// createdOperation reports whether a command answers 201 on success.
func createdOperation(op string) bool {
	for _, prefix := range []string{"create", "add", "assign", "record", "report", "register"} {
		if strings.HasPrefix(op, prefix) {
			return true
		}
	}
	return false
}

func (h *Handler) command(c *gin.Context, t tenant.Context, r Route, run func() (any, error)) {
	tenantcmd.StrictCommand(c, h.idem, t.OrganizationID, r.Method, r.Path, createdOperation(r.OperationID),
		func(string) (any, error) { return run() })
}

// serve runs a route: commands through the idempotent envelope, reads
// straight to the success envelope (with meta.nextCursor for keyset lists).
func (h *Handler) serve(c *gin.Context, t tenant.Context, r Route, run func() (any, error)) {
	if r.IdempotencyRequired {
		h.command(c, t, r, run)
		return
	}
	v, e := run()
	tenantcmd.Render(c, v, e)
}
func ver(c *gin.Context, v int64) int64 { return tenantcmd.ExpectedVersion(c, v) }

type vb struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	Reason          string `json:"reason"`
}

func keyset(c *gin.Context) (*time.Time, string, error) { return tenantcmd.Keyset(c) }
