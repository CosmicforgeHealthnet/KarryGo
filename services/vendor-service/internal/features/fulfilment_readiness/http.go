package fulfilment_readiness

// The readiness API.
//
// What is deliberately absent matters as much as what is here. There is no
// route a customer can reach, and none a provider can reach. Both would
// plausibly want one -- a customer watching their order, a rider standing at
// the counter -- and neither can see whether an order is complete, correct and
// paid for. Readiness is the branch's to declare, so the only authenticated
// identity these routes accept is vendor staff.
//
// Every mutation takes If-Match. Two staff members with the same order open on
// two screens is the normal case on a shared console, and a blind write
// silently discards whichever decision arrived first.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/platform/authctx"
	"cosmicforge/logistics/shared/go/httpx"
)

const maxReadinessBody = 32 << 10

// Authorizer answers whether a staff member holds a permission at a branch.
//
// It is a narrow port over the staff feature's own authorization evaluator
// rather than a branch list carried on the session. The difference matters: a
// grant is scoped when it is ASSIGNED, so asking the evaluator is asking the
// authority, while a list on the session would be a copy that goes stale the
// moment somebody's access changes.
type Authorizer interface {
	Authorize(ctx context.Context, staffID, permissionCode string, branchID *string) (bool, error)
}

// Permissions this surface checks.
const (
	PermissionReadinessRead   = "vendor.readiness.read"
	PermissionReadinessManage = "vendor.readiness.manage"
)

// Handler serves the readiness API.
type Handler struct {
	service    *Service
	authorizer Authorizer
}

// NewHandler builds the handler.
func NewHandler(service *Service, authorizer Authorizer) *Handler {
	return &Handler{service: service, authorizer: authorizer}
}

// RegisterRoutes mounts the readiness surface on an already-authenticated
// vendor staff group.
//
// A disabled feature mounts nothing rather than answering 503 on every path: a
// route that exists and refuses still has to be reasoned about, and still
// confirms the capability is there.
func RegisterRoutes(root *gin.RouterGroup, handler *Handler, enabled bool) {
	if root == nil || handler == nil || handler.service == nil || !enabled {
		return
	}
	// Mounted inside the existing /commerce/orders tree rather than beside it.
	// Readiness is a property of a vendor order, and the rest of that order's
	// vendor-facing surface -- accept, ready, handover -- already lives there.
	// A second top-level /orders tree would work, and would have been one more
	// place to look for the same resource.
	orders := root.Group("/commerce/orders/:vendorOrderId/readiness")
	orders.GET("", handler.get)
	orders.GET("/history", handler.history)
	orders.POST("/start", handler.start)
	orders.POST("/estimate", handler.estimate)
	orders.POST("/ready-soon", handler.readySoon)
	orders.POST("/ready", handler.ready)
	orders.POST("/delay", handler.delay)
	orders.POST("/block", handler.block)
}

// actor builds the staff identity from the authenticated session and the
// branch the request is actually about.
//
// The business always comes from the session, never from the body: a
// body-supplied tenant is a boundary one typo wide, and on this surface that
// typo would let one business declare another business's order ready. The
// branch comes from the resource, and the permission is checked against it, so
// a grant scoped to one branch does not reach another.
func (h *Handler) actor(c *gin.Context, permission, branchID string) (Actor, bool) {
	staff, ok := authctx.FromContext(c)
	if !ok {
		unauthorized(c)
		return Actor{}, false
	}
	if h.authorizer == nil {
		respond(c, nil, ErrForbidden)
		return Actor{}, false
	}
	var target *string
	if strings.TrimSpace(branchID) != "" {
		target = &branchID
	}
	allowed, err := h.authorizer.Authorize(c, staff.StaffID, permission, target)
	if err != nil {
		respond(c, nil, err)
		return Actor{}, false
	}
	if !allowed {
		respond(c, nil, ErrForbidden)
		return Actor{}, false
	}
	actor := Actor{Kind: SourceVendorStaff, Ref: staff.StaffID, BusinessID: staff.VendorID}
	if target != nil {
		actor.PermittedBranches = []string{branchID}
	} else {
		actor.AllBranches = true
	}
	return actor, true
}

// branchOf reads the branch a readiness record belongs to, so the permission
// check below is against the resource rather than against whatever the caller
// claimed.
func (h *Handler) branchOf(c *gin.Context, vendorOrderID string) (Readiness, bool) {
	state, err := h.service.Read(c, vendorOrderID)
	if err != nil {
		respond(c, nil, err)
		return Readiness{}, false
	}
	return state, true
}

func strictBody(c *gin.Context, destination any) error {
	if c.Request.Body == nil || c.Request.ContentLength == 0 {
		return nil
	}
	decoder := json.NewDecoder(io.LimitReader(c.Request.Body, maxReadinessBody+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return ErrValidation
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return ErrValidation
	}
	return nil
}

// expectedVersion reads If-Match. Absent is refused rather than defaulted.
func expectedVersion(c *gin.Context) (int64, error) {
	raw := strings.Trim(strings.TrimSpace(c.GetHeader("If-Match")), `"`)
	if raw == "" {
		return 0, ErrVersion
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 1 {
		return 0, ErrVersion
	}
	return value, nil
}

func respond(c *gin.Context, value any, err error) {
	if err == nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": value})
		return
	}
	status, code, message := http.StatusInternalServerError,
		"READINESS_UNAVAILABLE", "Readiness is temporarily unavailable."
	switch {
	case errors.Is(err, ErrDisabled):
		status, code, message = http.StatusServiceUnavailable,
			"READINESS_DISABLED", "Readiness orchestration is not enabled."
	case errors.Is(err, ErrNotFound):
		status, code, message = http.StatusNotFound,
			"READINESS_NOT_FOUND", "No readiness record for this order."
	case errors.Is(err, ErrForbidden):
		status, code, message = http.StatusForbidden,
			"READINESS_FORBIDDEN", "You may not change readiness for this branch."
	case errors.Is(err, ErrVersion):
		status, code, message = http.StatusConflict,
			"READINESS_VERSION_CONFLICT", "This order moved since you read it."
	case errors.Is(err, ErrTransition):
		status, code, message = http.StatusConflict,
			"READINESS_TRANSITION_REFUSED", err.Error()
	case errors.Is(err, ErrValidation):
		status, code, message = http.StatusBadRequest, "READINESS_INVALID", err.Error()
	}
	c.JSON(status, gin.H{"success": false, "error": gin.H{
		"code": code, "message": message, "request_id": httpx.GetRequestID(c)}})
}

func unauthorized(c *gin.Context) {
	c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": gin.H{
		"code": "UNAUTHORIZED", "message": "Authentication is required.",
		"request_id": httpx.GetRequestID(c)}})
}

func (h *Handler) get(c *gin.Context) {
	state, ok := h.branchOf(c, c.Param("vendorOrderId"))
	if !ok {
		return
	}
	if _, ok = h.actor(c, PermissionReadinessRead, state.BranchID); !ok {
		return
	}
	respond(c, state, nil)
}

func (h *Handler) history(c *gin.Context) {
	state, ok := h.branchOf(c, c.Param("vendorOrderId"))
	if !ok {
		return
	}
	if _, ok = h.actor(c, PermissionReadinessRead, state.BranchID); !ok {
		return
	}
	entries, err := h.service.History(c, c.Param("vendorOrderId"))
	respond(c, gin.H{"history": entries}, err)
}

type startBody struct {
	ParentOrderID  string     `json:"parent_order_id"`
	BranchID       string     `json:"branch_id"`
	VendorType     string     `json:"vendor_type"`
	EstimatedReady *time.Time `json:"estimated_ready_at"`
}

func (h *Handler) start(c *gin.Context) {
	var body startBody
	if err := strictBody(c, &body); err != nil {
		respond(c, nil, err)
		return
	}
	actor, ok := h.actor(c, PermissionReadinessManage, body.BranchID)
	if !ok {
		return
	}
	vendorType, err := ParseVendorType(body.VendorType)
	if err != nil {
		respond(c, nil, err)
		return
	}
	state, err := h.service.Start(c, StartRequest{
		VendorOrderID: c.Param("vendorOrderId"), ParentOrderID: body.ParentOrderID,
		BusinessID: actor.BusinessID, BranchID: body.BranchID,
		VendorType: vendorType, Actor: actor, VendorEstimate: body.EstimatedReady,
	})
	respond(c, state, err)
}

type moveBody struct {
	Substate       string     `json:"substate"`
	ReasonCode     string     `json:"reason_code"`
	EstimatedReady *time.Time `json:"estimated_ready_at"`
}

// advance is the shared body of every state-changing route.
//
// They differ only in the state they move to, so the validation, the version
// guard and the actor check live in one place rather than six.
func (h *Handler) advance(c *gin.Context, to State) {
	state, ok := h.branchOf(c, c.Param("vendorOrderId"))
	if !ok {
		return
	}
	actor, ok := h.actor(c, PermissionReadinessManage, state.BranchID)
	if !ok {
		return
	}
	version, err := expectedVersion(c)
	if err != nil {
		respond(c, nil, err)
		return
	}
	var body moveBody
	if err = strictBody(c, &body); err != nil {
		respond(c, nil, err)
		return
	}
	updated, err := h.service.Advance(c, AdvanceRequest{
		VendorOrderID: c.Param("vendorOrderId"), ExpectedVersion: version,
		Move: Move{To: to, Substate: Substate(strings.ToUpper(strings.TrimSpace(body.Substate))),
			ReasonCode: strings.TrimSpace(body.ReasonCode), EstimatedAt: body.EstimatedReady},
		Actor: actor,
	})
	respond(c, updated, err)
}

// estimate updates the vendor's own ready time without changing state.
func (h *Handler) estimate(c *gin.Context) { h.advance(c, StatePreparing) }

func (h *Handler) readySoon(c *gin.Context) { h.advance(c, StateReadySoon) }
func (h *Handler) ready(c *gin.Context)     { h.advance(c, StateReady) }
func (h *Handler) delay(c *gin.Context)     { h.advance(c, StateDelayed) }
func (h *Handler) block(c *gin.Context)     { h.advance(c, StateBlocked) }
