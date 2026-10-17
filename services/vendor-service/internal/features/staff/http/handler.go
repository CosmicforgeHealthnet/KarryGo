// Package staffhttp is the thin HTTP layer for Staff IAM: staff management
// and role/permission management. No business rules live here — every
// authorization decision, delegation check, and step-up requirement is
// enforced inside staffusecases (CLAUDE.md, "Keep handlers thin"; Section 5
// rule 17: scope enforcement must not rely only on middleware).
package staffhttp

import (
	"context"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"

	staffmodels "cosmicforge/logistics/services/vendor-service/internal/features/staff/models"
	staffusecases "cosmicforge/logistics/services/vendor-service/internal/features/staff/usecases"
	"cosmicforge/logistics/services/vendor-service/internal/platform/authctx"
)

// SessionRevoker is the one auth-feature capability this handler needs:
// revoking every active session for a staff member on suspension. Passed as a
// plain func value (wired to *vendorauthusecases.AuthService.RevokeAllSessionsForStaff
// by cmd/main.go) rather than a full port interface, since it is a single
// method — importing internal/features/auth/http or /repositories directly
// would violate the cross-feature import guardrail.
type SessionRevoker func(ctx context.Context, staffID, reason string) error

type Handler struct {
	staff          *staffusecases.StaffService
	roles          *staffusecases.RoleService
	revokeSessions SessionRevoker
}

func NewHandler(staff *staffusecases.StaffService, roles *staffusecases.RoleService, revokeSessions SessionRevoker) *Handler {
	return &Handler{staff: staff, roles: roles, revokeSessions: revokeSessions}
}

func actorContext(c *gin.Context) (authctx.StaffContext, bool) {
	ctx, ok := authctx.FromContext(c)
	if !ok {
		httpx.RespondError(c, apperrors.Unauthorized("Authentication is required.", nil))
	}
	return ctx, ok
}

// ─── Staff ──────────────────────────────────────────────────────────────

func (h *Handler) ListStaff(c *gin.Context) {
	actor, ok := actorContext(c)
	if !ok {
		return
	}
	list, err := h.staff.List(c.Request.Context(), actor.StaffID, actor.VendorID)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	views := make([]staffView, len(list))
	for i, s := range list {
		views[i] = toStaffView(s)
	}
	respondOK(c, gin.H{"staff": views})
}

func (h *Handler) GetStaff(c *gin.Context) {
	actor, ok := actorContext(c)
	if !ok {
		return
	}
	s, err := h.staff.Get(c.Request.Context(), actor.StaffID, actor.VendorID, c.Param("id"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respondOK(c, toStaffView(s))
}

func (h *Handler) InviteStaff(c *gin.Context) {
	actor, ok := actorContext(c)
	if !ok {
		return
	}
	var req inviteStaffRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.RespondError(c, apperrors.BadRequest("Check your details.", err))
		return
	}
	s, err := h.staff.InviteStaff(c.Request.Context(), staffusecases.InviteStaffInput{
		ActorStaffID: actor.StaffID, VendorID: actor.VendorID,
		Phone: req.Phone, Email: req.Email, RoleID: req.RoleID, BranchID: req.BranchID,
	})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(201, gin.H{"success": true, "data": toStaffView(s)})
}

func (h *Handler) SuspendStaff(c *gin.Context) {
	actor, ok := actorContext(c)
	if !ok {
		return
	}
	var req suspendStaffRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.RespondError(c, apperrors.BadRequest("Check your details.", err))
		return
	}
	err := h.staff.Suspend(c.Request.Context(), staffusecases.SuspendStaffInput{
		ActorStaffID: actor.StaffID, VendorID: actor.VendorID, TargetStaffID: c.Param("id"),
		Reason: req.Reason, ExpectedVersion: req.ExpectedVersion,
	}, h.revokeSessions)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respondOK(c, gin.H{"suspended": true})
}

func (h *Handler) ReactivateStaff(c *gin.Context) {
	actor, ok := actorContext(c)
	if !ok {
		return
	}
	var req reactivateStaffRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.RespondError(c, apperrors.BadRequest("Check your details.", err))
		return
	}
	err := h.staff.Reactivate(c.Request.Context(), staffusecases.ReactivateStaffInput{
		ActorStaffID: actor.StaffID, VendorID: actor.VendorID, TargetStaffID: c.Param("id"), ExpectedVersion: req.ExpectedVersion,
	})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respondOK(c, gin.H{"reactivated": true})
}

func toStaffView(s staffmodels.Staff) staffView {
	return staffView{
		ID: s.ID, VendorID: s.VendorID, Status: s.Status, SuspendedReason: s.SuspendedReason,
		Version: s.Version, CreatedAt: s.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		Phone: s.Phone, Email: s.Email,
	}
}

func (h *Handler) CurrentAccess(c *gin.Context) {
	actor, ok := actorContext(c)
	if !ok {
		return
	}
	grants, err := h.roles.CurrentAccess(c.Request.Context(), actor.StaffID)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	respondOK(c, gin.H{"staff_id": actor.StaffID, "vendor_id": actor.VendorID, "grants": grants})
}
