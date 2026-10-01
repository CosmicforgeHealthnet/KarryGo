package orchestration

// The orchestration API.
//
// It is almost entirely read-only, and that is the design. An orchestration is
// a decision the planner makes from the authorities' own answers; a route that
// let a request set the selected authority, the commit time or the mission
// reference would let a caller impersonate the planner, the clock or the
// executing service.
//
// The one mutation is an override, and it is separately permissioned and
// separately audited, because replacing a decision made from evidence with one
// made by a person is exactly the operation worth being able to explain later.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/integrations/logistics"
	"cosmicforge/logistics/services/vendor-service/internal/platform/authctx"
	"cosmicforge/logistics/shared/go/httpx"
)

const maxOrchestrationBody = 16 << 10

// Permissions this surface checks.
const (
	PermissionOrchestrationRead     = "vendor.orchestration.read"
	PermissionOrchestrationOverride = "vendor.orchestration.override"
)

// Authorizer answers whether a staff member holds a permission.
type Authorizer interface {
	Authorize(ctx context.Context, staffID, permissionCode string, branchID *string) (bool, error)
}

// Handler serves the orchestration API.
type Handler struct {
	service    *Service
	repo       *Repository
	authorizer Authorizer
}

// NewHandler builds the handler.
func NewHandler(service *Service, repo *Repository, authorizer Authorizer) *Handler {
	return &Handler{service: service, repo: repo, authorizer: authorizer}
}

// RegisterRoutes mounts the vendor-facing surface.
func RegisterRoutes(root *gin.RouterGroup, handler *Handler, enabled bool) {
	if root == nil || handler == nil || handler.repo == nil || !enabled {
		return
	}
	// Mounted inside the existing fulfilment-group tree, reusing its own
	// :groupId parameter, because an orchestration is a property of a
	// fulfilment group.
	//
	// The alternative -- a sibling /fulfilment/:groupRef tree -- does register
	// (gin resolves the overlap with the static /groups, /units and /parcels
	// segments rather than refusing it), but it leaves a wildcard sitting
	// beside them that would quietly capture any future
	// /fulfilment/<anything>/orchestration. One tree with one parameter name
	// is the shape that cannot drift.
	group := root.Group("/fulfilment/groups/:groupId/orchestration")
	group.GET("", handler.get)
	group.GET("/decisions", handler.decisions)
	group.POST("/authority-override", handler.override)
}

// RegisterInternalRoutes mounts the read surface for other services.
//
// It is read-only. Another service may ask what Vendor decided; none may tell
// Vendor what to decide.
//
// The handler it mounts is not the staff one. A calling service carries no
// staff session, so the staff-permission check would refuse every request; the
// boundary here is the group's own HMAC signature, caller allowlist and replay
// protection, which the caller passes in already applied. Mounting the staff
// handler and then weakening authctx to accommodate it would have made the
// staff surface reachable without a session, which is the opposite trade.
func RegisterInternalRoutes(root *gin.RouterGroup, handler *Handler, enabled bool) {
	if root == nil || handler == nil || handler.repo == nil || !enabled {
		return
	}
	root.GET("/fulfilment/groups/:groupId/orchestration", handler.getInternal)
}

func (h *Handler) allowed(c *gin.Context, permission string) bool {
	staff, ok := authctx.FromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": gin.H{
			"code": "UNAUTHORIZED", "message": "Authentication is required.",
			"request_id": httpx.GetRequestID(c)}})
		return false
	}
	if h.authorizer == nil {
		respond(c, nil, ErrValidation)
		return false
	}
	granted, err := h.authorizer.Authorize(c, staff.StaffID, permission, nil)
	if err != nil {
		respond(c, nil, err)
		return false
	}
	if !granted {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "error": gin.H{
			"code": "ORCHESTRATION_FORBIDDEN", "message": "You may not use this surface.",
			"request_id": httpx.GetRequestID(c)}})
		return false
	}
	return true
}

func respond(c *gin.Context, value any, err error) {
	if err == nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": value})
		return
	}
	status, code, message := http.StatusInternalServerError,
		"ORCHESTRATION_UNAVAILABLE", "Fulfilment orchestration is temporarily unavailable."
	switch {
	case errors.Is(err, ErrDisabled):
		status, code, message = http.StatusServiceUnavailable,
			"ORCHESTRATION_DISABLED", "Just-in-time orchestration is not enabled."
	case errors.Is(err, ErrNotFound):
		status, code, message = http.StatusNotFound,
			"ORCHESTRATION_NOT_FOUND", "No orchestration for this fulfilment group."
	case errors.Is(err, ErrCommitted):
		status, code, message = http.StatusConflict, "ORCHESTRATION_COMMITTED",
			"An execution authority already owns this work."
	case errors.Is(err, ErrVersion):
		status, code, message = http.StatusConflict,
			"ORCHESTRATION_VERSION_CONFLICT", "This orchestration moved since you read it."
	case errors.Is(err, ErrTransition):
		status, code, message = http.StatusConflict, "ORCHESTRATION_TRANSITION_REFUSED", err.Error()
	case errors.Is(err, ErrNoEligibleAuthority):
		status, code, message = http.StatusUnprocessableEntity,
			"NO_ELIGIBLE_AUTHORITY", "No configured authority can carry this."
	case errors.Is(err, ErrAuthoritiesUnreachable):
		// Deliberately 503 rather than 422. Nobody answered, so this is an
		// outage to retry, not a fact about the goods.
		status, code, message = http.StatusServiceUnavailable,
			"AUTHORITIES_UNREACHABLE", "No execution authority could be reached."
	case errors.Is(err, ErrValidation):
		status, code, message = http.StatusBadRequest, "ORCHESTRATION_INVALID", err.Error()
	}
	c.JSON(status, gin.H{"success": false, "error": gin.H{
		"code": code, "message": message, "request_id": httpx.GetRequestID(c)}})
}

func (h *Handler) get(c *gin.Context) {
	if !h.allowed(c, PermissionOrchestrationRead) {
		return
	}
	h.read(c)
}

// getInternal serves the same read to an authenticated service caller.
func (h *Handler) getInternal(c *gin.Context) { h.read(c) }

func (h *Handler) read(c *gin.Context) {
	current, err := h.repo.GetByGroup(c, c.Param("groupId"))
	if err != nil {
		respond(c, nil, err)
		return
	}
	pickups, err := h.repo.Pickups(c, current.ID)
	if err != nil {
		respond(c, nil, err)
		return
	}
	respond(c, gin.H{"orchestration": current, "pickups": pickups}, nil)
}

func (h *Handler) decisions(c *gin.Context) {
	if !h.allowed(c, PermissionOrchestrationRead) {
		return
	}
	current, err := h.repo.GetByGroup(c, c.Param("groupId"))
	if err != nil {
		respond(c, nil, err)
		return
	}
	events, err := h.repo.Decisions(c, current.ID, 0)
	respond(c, gin.H{"decisions": events}, err)
}

type overrideBody struct {
	Authority   string `json:"authority"`
	ProductCode string `json:"product_code"`
	Reason      string `json:"reason"`
}

// override replaces the automatically selected authority.
//
// Three things make it safe enough to exist. It needs its own high-risk
// permission. It records the actor and the reason beside the automatic decision
// it replaced, so the two can be compared. And it is refused outright once the
// work is committed -- at that point an execution authority owns it, and
// changing Vendor's record would only make the two disagree.
func (h *Handler) override(c *gin.Context) {
	if !h.allowed(c, PermissionOrchestrationOverride) {
		return
	}
	staff, _ := authctx.FromContext(c)

	var body overrideBody
	decoder := json.NewDecoder(io.LimitReader(c.Request.Body, maxOrchestrationBody+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		respond(c, nil, ErrValidation)
		return
	}
	if strings.TrimSpace(body.Reason) == "" {
		respond(c, nil, errors.Join(ErrValidation, errors.New("an override reason is required")))
		return
	}
	authority, err := logistics.Parse(body.Authority)
	if err != nil {
		respond(c, nil, ErrValidation)
		return
	}
	overridden, err := h.service.Override(c, OverrideRequest{
		GroupRef: c.Param("groupId"), Authority: authority,
		ProductCode: strings.TrimSpace(body.ProductCode),
		Reason:      strings.TrimSpace(body.Reason), ActorRef: staff.StaffID,
	})
	respond(c, overridden, err)
}
