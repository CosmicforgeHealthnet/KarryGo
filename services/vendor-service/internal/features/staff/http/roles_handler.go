package staffhttp

import (
	"context"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"

	staffmodels "cosmicforge/logistics/services/vendor-service/internal/features/staff/models"
	staffusecases "cosmicforge/logistics/services/vendor-service/internal/features/staff/usecases"
)

func (h *Handler) ListPermissions(c *gin.Context) {
	actor, ok := actorContext(c)
	if !ok {
		return
	}
	list, err := h.roles.ListPermissionCatalogue(c.Request.Context(), actor.StaffID)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	views := make([]permissionView, len(list))
	for i, p := range list {
		views[i] = permissionView{Code: p.Code, Description: p.Description, Category: p.Category, IsHighRisk: p.IsHighRisk}
	}
	respondOK(c, gin.H{"permissions": views})
}

func (h *Handler) ListRoles(c *gin.Context) {
	actor, ok := actorContext(c)
	if !ok {
		return
	}
	list, err := h.roles.ListRoles(c.Request.Context(), actor.StaffID, actor.VendorID)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	views := make([]roleView, len(list))
	for i, r := range list {
		views[i] = toRoleView(r, nil)
	}
	respondOK(c, gin.H{"roles": views})
}

func (h *Handler) GetRole(c *gin.Context) {
	actor, ok := actorContext(c)
	if !ok {
		return
	}
	role, codes, err := h.roles.GetRole(c.Request.Context(), actor.StaffID, actor.VendorID, c.Param("id"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respondOK(c, toRoleView(role, codes))
}

func (h *Handler) CreateRole(c *gin.Context) {
	actor, ok := actorContext(c)
	if !ok {
		return
	}
	var req createRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.RespondError(c, apperrors.BadRequest("Check your details.", err))
		return
	}
	role, err := h.roles.CreateCustomRole(c.Request.Context(), staffusecases.CreateCustomRoleInput{
		ActorStaffID: actor.StaffID, VendorID: actor.VendorID, Code: req.Code, Name: req.Name, Description: req.Description,
		PermissionCodes: req.PermissionCodes, StepUpGrantID: req.StepUpGrantID, SessionID: actor.SessionID,
	})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(201, gin.H{"success": true, "data": toRoleView(role, req.PermissionCodes)})
}

func (h *Handler) SetRolePermissions(c *gin.Context) {
	actor, ok := actorContext(c)
	if !ok {
		return
	}
	var req setRolePermissionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.RespondError(c, apperrors.BadRequest("Check your details.", err))
		return
	}
	err := h.roles.SetRolePermissions(c.Request.Context(), staffusecases.SetRolePermissionsInput{
		ActorStaffID: actor.StaffID, VendorID: actor.VendorID, RoleID: c.Param("id"),
		PermissionCodes: req.PermissionCodes, ExpectedVersion: req.ExpectedVersion,
		StepUpGrantID: req.StepUpGrantID, SessionID: actor.SessionID,
	})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respondOK(c, gin.H{"updated": true})
}

func (h *Handler) ListAssignments(c *gin.Context) {
	actor, ok := actorContext(c)
	if !ok {
		return
	}
	list, err := h.roles.ListAssignmentsForStaff(c.Request.Context(), actor.StaffID, actor.VendorID, c.Param("id"),
		func(ctx context.Context, vendorID, id string) error {
			_, err := h.staff.Get(ctx, actor.StaffID, vendorID, id)
			return err
		})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	views := make([]assignmentView, len(list))
	for i, a := range list {
		views[i] = toAssignmentView(a)
	}
	respondOK(c, gin.H{"assignments": views})
}

func (h *Handler) AssignRole(c *gin.Context) {
	actor, ok := actorContext(c)
	if !ok {
		return
	}
	var req assignRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.RespondError(c, apperrors.BadRequest("Check your details.", err))
		return
	}
	assignment, err := h.roles.AssignRole(c.Request.Context(), staffusecases.AssignRoleInput{
		ActorStaffID: actor.StaffID, VendorID: actor.VendorID, TargetStaffID: req.StaffID, RoleID: req.RoleID,
		BranchID: req.BranchID, StepUpGrantID: req.StepUpGrantID, SessionID: actor.SessionID,
	}, h.assignmentTargetMatchesVendor)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(201, gin.H{"success": true, "data": toAssignmentView(assignment)})
}

func (h *Handler) UnassignRole(c *gin.Context) {
	actor, ok := actorContext(c)
	if !ok {
		return
	}
	if err := h.roles.UnassignRole(c.Request.Context(), actor.StaffID, actor.VendorID, c.Param("id")); err != nil {
		httpx.RespondError(c, err)
		return
	}
	respondOK(c, gin.H{"unassigned": true})
}

// assignmentTargetMatchesVendor enforces "an actor cannot assign a role to a
// staff member outside their own vendor business" by delegating to
// StaffRepository's vendor-scoped lookup — a staff id from another vendor
// simply does not resolve here, at the SQL layer, not by post-hoc field
// comparison.
func (h *Handler) assignmentTargetMatchesVendor(ctx context.Context, vendorID, staffID string) error {
	_, err := h.staff.GetTargetForAssignment(ctx, vendorID, staffID)
	return err
}

func toRoleView(r staffmodels.Role, codes []string) roleView {
	return roleView{
		ID: r.ID, VendorID: r.VendorID, Code: r.Code, Name: r.Name, Description: r.Description,
		IsSystem: r.IsSystem, Version: r.Version, PermissionCodes: codes,
	}
}

func toAssignmentView(a staffmodels.Assignment) assignmentView {
	return assignmentView{
		ID: a.ID, StaffID: a.StaffID, RoleID: a.RoleID, BranchID: a.BranchID, AssignedBy: a.AssignedBy,
		CreatedAt: a.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
