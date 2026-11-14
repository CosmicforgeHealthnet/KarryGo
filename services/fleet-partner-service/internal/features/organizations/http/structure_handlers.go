package http

import (
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"

	orgmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/models"
	orgusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/usecases"
	staffmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/models"
	staffusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Organization structure and access administration routes (spec Sections
// 7, 8, 9, 37.3 and 38.3). Every route resolves tenant scope from the
// session, never from the request, and checks a catalogue permission.

// registerStructureRoutes mounts branches, departments, settings, staff
// edits, custom roles and role bindings.
func (h *Handler) registerStructureRoutes(group *gin.RouterGroup) {
	group.GET("/branches", h.listBranches)
	group.POST("/branches", h.createBranch)
	group.GET("/branches/:branchId", h.getBranch)
	group.PATCH("/branches/:branchId", h.updateBranch)
	group.POST("/branches/:branchId/archive", h.archiveBranch)

	group.GET("/departments", h.listDepartments)
	group.POST("/departments", h.createDepartment)
	group.GET("/departments/:departmentId", h.getDepartment)
	group.PATCH("/departments/:departmentId", h.updateDepartment)
	group.POST("/departments/:departmentId/archive", h.archiveDepartment)

	group.GET("/cost-centres", h.listCostCentres)
	group.POST("/cost-centres", h.createCostCentre)
	group.GET("/cost-centres/:costCentreId", h.getCostCentre)
	group.PATCH("/cost-centres/:costCentreId", h.updateCostCentre)
	group.POST("/cost-centres/:costCentreId/archive", h.archiveCostCentre)

	group.GET("/teams", h.listTeams)
	group.POST("/teams", h.createTeam)
	group.GET("/teams/:teamId", h.getTeam)
	group.PATCH("/teams/:teamId", h.updateTeam)
	group.POST("/teams/:teamId/archive", h.archiveTeam)

	group.GET("/locations", h.listLocations)
	group.POST("/locations", h.createLocation)
	group.GET("/locations/:locationId", h.getLocation)
	group.PATCH("/locations/:locationId", h.updateLocation)
	group.POST("/locations/:locationId/archive", h.archiveLocation)

	group.GET("/organization-settings", h.getSettings)
	group.PATCH("/organization-settings", h.updateSettings)

	group.PATCH("/staff/:staffId", h.updateStaff)
	group.GET("/staff/:staffId/role-bindings", h.listRoleBindings)
	group.PUT("/staff/:staffId/roles", h.replaceStaffRoles)

	group.POST("/roles", h.createRole)
	group.GET("/roles/:roleId", h.getRole)
	group.PATCH("/roles/:roleId", h.updateRole)
	group.POST("/roles/:roleId/archive", h.archiveRole)
}

func bindJSON(c *gin.Context, target any) bool {
	if err := c.ShouldBindJSON(target); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
		return false
	}
	return true
}

// tenantWithPermission resolves scope and checks one permission.
func (h *Handler) tenantWithPermission(c *gin.Context, permission string) (tenant.Context, bool) {
	tenantCtx, ok := requireTenant(c)
	if !ok || !h.authorize(c, tenantCtx, permission) {
		return tenant.Context{}, false
	}
	return tenantCtx, true
}

func branchView(branch *orgmodels.Branch) gin.H {
	return gin.H{
		"id": branch.ID, "branchId": branch.ID, "organizationId": branch.OrganizationID,
		"code": branch.Code, "name": branch.Name, "type": branch.Type, "address": branch.Address,
		"latitude": branch.Latitude, "longitude": branch.Longitude, "operatingHours": branch.OperatingHours,
		"timezone": branch.Timezone, "status": branch.Status, "version": branch.Version,
		"createdAt": branch.CreatedAt, "updatedAt": branch.UpdatedAt, "archivedAt": branch.ArchivedAt,
	}
}

func departmentView(department *orgmodels.Department) gin.H {
	return gin.H{
		"id": department.ID, "departmentId": department.ID, "organizationId": department.OrganizationID,
		"branchId": department.BranchID, "parentId": department.ParentID, "costCentreId": department.CostCentreID,
		"code": department.Code, "name": department.Name, "status": department.Status,
		"version": department.Version, "createdAt": department.CreatedAt, "updatedAt": department.UpdatedAt,
		"archivedAt": department.ArchivedAt,
	}
}

func settingsView(settings *orgmodels.Settings) gin.H {
	return gin.H{
		"organizationId": settings.OrganizationID, "assignmentTimeoutSeconds": settings.AssignmentTimeoutSeconds,
		"operatingMode": settings.OperatingMode, "approvalThresholds": settings.ApprovalThresholds,
		"providerVisibility": settings.ProviderVisibility, "financialVisibilityPolicy": settings.FinancialVisibilityPolicy,
		"version": settings.Version, "updatedAt": settings.UpdatedAt,
	}
}

func bindingView(binding staffmodels.RoleBinding) gin.H {
	return gin.H{
		"id": binding.ID, "organizationUserId": binding.OrganizationUserID, "roleId": binding.RoleID,
		"scopeType": binding.ScopeType, "scopeId": binding.ScopeID, "effectiveFrom": binding.EffectiveFrom,
		"expiresAt": binding.ExpiresAt, "grantedBy": binding.GrantedBy, "version": binding.Version,
		"createdAt": binding.CreatedAt,
	}
}

// ---- branches ----

func (h *Handler) listBranches(c *gin.Context) {
	tenantCtx, ok := h.tenantWithPermission(c, "organization.read")
	if !ok {
		return
	}
	branches, err := h.organizations.ListBranches(c.Request.Context(), tenant.Query{
		OrganizationID: tenantCtx.OrganizationID, PermittedBranchIDs: tenantCtx.PermittedBranchIDs, Limit: 100})
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]gin.H, 0, len(branches))
	for index := range branches {
		views = append(views, branchView(&branches[index]))
	}
	respond.List(c, views, "")
}

func (h *Handler) getBranch(c *gin.Context) {
	tenantCtx, ok := h.tenantWithPermission(c, "organization.read")
	if !ok {
		return
	}
	if !tenantCtx.BranchPermitted(c.Param("branchId")) {
		respond.Error(c, apperrors.NotFound("Branch not found.", nil))
		return
	}
	branch, err := h.organizations.GetBranch(c.Request.Context(), tenantCtx.OrganizationID, c.Param("branchId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, branchView(branch))
}

type branchRequest struct {
	Code           string   `json:"code"`
	Name           string   `json:"name"`
	Type           string   `json:"type"`
	Address        string   `json:"address"`
	Latitude       *float64 `json:"latitude"`
	Longitude      *float64 `json:"longitude"`
	OperatingHours string   `json:"operatingHours"`
	Timezone       string   `json:"timezone"`
	Status         string   `json:"status"`
}

func (h *Handler) createBranch(c *gin.Context) {
	tenantCtx, ok := h.tenantWithPermission(c, "organization.update")
	if !ok {
		return
	}
	var request branchRequest
	if !bindJSON(c, &request) {
		return
	}
	branch, err := h.organizations.CreateBranch(c.Request.Context(), orgusecases.BranchInput{
		Tenant: tenantCtx, Code: request.Code, Name: request.Name, Type: request.Type, Address: request.Address,
		Latitude: request.Latitude, Longitude: request.Longitude, OperatingHours: request.OperatingHours,
		Timezone: request.Timezone})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.Created(c, branchView(branch))
}

func (h *Handler) updateBranch(c *gin.Context) {
	tenantCtx, ok := h.tenantWithPermission(c, "organization.update")
	if !ok {
		return
	}
	if !tenantCtx.BranchPermitted(c.Param("branchId")) {
		respond.Error(c, apperrors.NotFound("Branch not found.", nil))
		return
	}
	expected, err := expectedVersionFrom(c)
	if err != nil {
		respond.Error(c, err)
		return
	}
	var request branchRequest
	if !bindJSON(c, &request) {
		return
	}
	branch, err := h.organizations.UpdateBranch(c.Request.Context(), orgusecases.BranchInput{
		Tenant: tenantCtx, BranchID: c.Param("branchId"), Name: request.Name, Type: request.Type,
		Address: request.Address, Latitude: request.Latitude, Longitude: request.Longitude,
		OperatingHours: request.OperatingHours, Timezone: request.Timezone, Status: request.Status,
		ExpectedVersion: expected})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, branchView(branch))
}

func (h *Handler) archiveBranch(c *gin.Context) {
	tenantCtx, ok := h.tenantWithPermission(c, "organization.update")
	if !ok {
		return
	}
	if !tenantCtx.BranchPermitted(c.Param("branchId")) {
		respond.Error(c, apperrors.NotFound("Branch not found.", nil))
		return
	}
	expected, err := expectedVersionFrom(c)
	if err != nil {
		respond.Error(c, err)
		return
	}
	if err := h.organizations.ArchiveBranch(c.Request.Context(), tenantCtx, c.Param("branchId"), expected); err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, gin.H{"branchId": c.Param("branchId"), "status": orgmodels.BranchStatusArchived})
}

// ---- departments ----

func (h *Handler) listDepartments(c *gin.Context) {
	tenantCtx, ok := h.tenantWithPermission(c, "organization.read")
	if !ok {
		return
	}
	departments, err := h.organizations.ListDepartments(c.Request.Context(), tenantCtx.OrganizationID,
		c.Query("branchId"), c.Query("includeArchived") == "true")
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]gin.H, 0, len(departments))
	for index := range departments {
		department := departments[index]
		if department.BranchID != nil && !tenantCtx.BranchPermitted(*department.BranchID) {
			continue
		}
		views = append(views, departmentView(&department))
	}
	respond.List(c, views, "")
}

func (h *Handler) getDepartment(c *gin.Context) {
	tenantCtx, ok := h.tenantWithPermission(c, "organization.read")
	if !ok {
		return
	}
	department, err := h.organizations.GetDepartment(c.Request.Context(), tenantCtx.OrganizationID, c.Param("departmentId"))
	if err == nil && department.BranchID != nil && !tenantCtx.BranchPermitted(*department.BranchID) {
		err = apperrors.NotFound("Department not found.", nil)
	}
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, departmentView(department))
}

type departmentRequest struct {
	Code         string  `json:"code"`
	Name         string  `json:"name"`
	BranchID     *string `json:"branchId"`
	ParentID     *string `json:"parentId"`
	CostCentreID *string `json:"costCentreId"`
}

func (h *Handler) createDepartment(c *gin.Context) {
	tenantCtx, ok := h.tenantWithPermission(c, "organization.update")
	if !ok {
		return
	}
	var request departmentRequest
	if !bindJSON(c, &request) {
		return
	}
	department, err := h.organizations.CreateDepartment(c.Request.Context(), orgusecases.DepartmentInput{
		Tenant: tenantCtx, Code: request.Code, Name: request.Name, BranchID: request.BranchID,
		ParentID: request.ParentID, CostCentreID: request.CostCentreID})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.Created(c, departmentView(department))
}

func (h *Handler) updateDepartment(c *gin.Context) {
	tenantCtx, ok := h.tenantWithPermission(c, "organization.update")
	if !ok {
		return
	}
	expected, err := expectedVersionFrom(c)
	if err != nil {
		respond.Error(c, err)
		return
	}
	var request departmentRequest
	if !bindJSON(c, &request) {
		return
	}
	department, err := h.organizations.UpdateDepartment(c.Request.Context(), orgusecases.DepartmentInput{
		Tenant: tenantCtx, DepartmentID: c.Param("departmentId"), Name: request.Name, BranchID: request.BranchID,
		ParentID: request.ParentID, CostCentreID: request.CostCentreID, ExpectedVersion: expected})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, departmentView(department))
}

func (h *Handler) archiveDepartment(c *gin.Context) {
	tenantCtx, ok := h.tenantWithPermission(c, "organization.update")
	if !ok {
		return
	}
	expected, err := expectedVersionFrom(c)
	if err != nil {
		respond.Error(c, err)
		return
	}
	if err := h.organizations.ArchiveDepartment(c.Request.Context(), tenantCtx, c.Param("departmentId"), expected); err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, gin.H{"departmentId": c.Param("departmentId"), "status": "ARCHIVED"})
}

// ---- settings ----

func (h *Handler) getSettings(c *gin.Context) {
	tenantCtx, ok := h.tenantWithPermission(c, "organization.read")
	if !ok {
		return
	}
	settings, err := h.organizations.GetSettings(c.Request.Context(), tenantCtx.OrganizationID)
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, settingsView(settings))
}

type settingsRequest struct {
	AssignmentTimeoutSeconds *int           `json:"assignmentTimeoutSeconds"`
	ProviderVisibility       *string        `json:"providerVisibility"`
	ApprovalThresholds       map[string]any `json:"approvalThresholds"`
}

func (h *Handler) updateSettings(c *gin.Context) {
	tenantCtx, ok := h.tenantWithPermission(c, "organization.update")
	if !ok {
		return
	}
	expected, err := expectedVersionFrom(c)
	if err != nil {
		respond.Error(c, err)
		return
	}
	var request settingsRequest
	if !bindJSON(c, &request) {
		return
	}
	settings, err := h.organizations.UpdateSettings(c.Request.Context(), orgusecases.SettingsPatch{
		Tenant: tenantCtx, AssignmentTimeoutSeconds: request.AssignmentTimeoutSeconds,
		ProviderVisibility: request.ProviderVisibility, ApprovalThresholds: request.ApprovalThresholds,
		ExpectedVersion: expected})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, settingsView(settings))
}

// ---- staff edits and role bindings ----

type updateStaffRequest struct {
	FullName      string   `json:"fullName"`
	JobTitle      *string  `json:"jobTitle"`
	BranchIDs     []string `json:"branchIds"`
	DepartmentIDs []string `json:"departmentIds"`
}

func (h *Handler) updateStaff(c *gin.Context) {
	tenantCtx, ok := h.tenantWithPermission(c, "staff.manage")
	if !ok {
		return
	}
	expected, err := expectedVersionFrom(c)
	if err != nil {
		respond.Error(c, err)
		return
	}
	var request updateStaffRequest
	if !bindJSON(c, &request) {
		return
	}
	member, err := h.staff.GetStaff(c.Request.Context(), tenantCtx.OrganizationID, c.Param("staffId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	if request.FullName == "" {
		request.FullName = member.FullName
	}
	if request.BranchIDs == nil {
		request.BranchIDs = member.PermittedBranchIDs
	}
	if request.DepartmentIDs == nil {
		request.DepartmentIDs = member.PermittedDepartmentIDs
	}
	if request.JobTitle == nil {
		request.JobTitle = member.JobTitle
	}
	// Branch and department references must belong to this organization.
	for _, branchID := range request.BranchIDs {
		if _, err := h.organizations.GetBranch(c.Request.Context(), tenantCtx.OrganizationID, branchID); err != nil {
			respond.Error(c, apperrors.Validation("A selected branch is not available.",
				[]apperrors.FieldViolation{{Field: "branchIds", Message: "Unknown branch."}}))
			return
		}
	}
	for _, departmentID := range request.DepartmentIDs {
		if _, err := h.organizations.GetDepartment(c.Request.Context(), tenantCtx.OrganizationID, departmentID); err != nil {
			respond.Error(c, apperrors.Validation("A selected department is not available.",
				[]apperrors.FieldViolation{{Field: "departmentIds", Message: "Unknown department."}}))
			return
		}
	}
	if expected == 0 {
		expected = member.Version
	}
	if err := h.staff.UpdateStaff(c.Request.Context(), staffusecases.UpdateStaffInput{
		Tenant: tenantCtx, MemberID: member.ID, FullName: request.FullName, JobTitle: request.JobTitle,
		BranchIDs: request.BranchIDs, DepartmentIDs: request.DepartmentIDs, ExpectedVersion: expected}); err != nil {
		respond.Error(c, err)
		return
	}
	entry, err := h.staff.StaffMember(c.Request.Context(), tenantCtx.OrganizationID, member.ID)
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, entry)
}

func (h *Handler) listRoleBindings(c *gin.Context) {
	tenantCtx, ok := h.tenantWithPermission(c, "roles.read")
	if !ok {
		return
	}
	bindings, err := h.staff.ListRoleBindings(c.Request.Context(), tenantCtx.OrganizationID, c.Param("staffId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]gin.H, 0, len(bindings))
	for _, binding := range bindings {
		views = append(views, bindingView(binding))
	}
	respond.List(c, views, "")
}

type roleBindingRequest struct {
	RoleID    string     `json:"roleId"`
	ScopeType string     `json:"scopeType"`
	ScopeID   string     `json:"scopeId"`
	ExpiresAt *time.Time `json:"expiresAt"`
}

type replaceRolesRequest struct {
	RoleIDs  []string             `json:"roleIds"`
	Bindings []roleBindingRequest `json:"bindings"`
}

func (h *Handler) replaceStaffRoles(c *gin.Context) {
	tenantCtx, ok := h.tenantWithPermission(c, "roles.manage")
	if !ok {
		return
	}
	var request replaceRolesRequest
	if !bindJSON(c, &request) {
		return
	}
	if len(request.RoleIDs) == 0 && len(request.Bindings) == 0 {
		respond.Error(c, apperrors.Validation("Give at least one role.",
			[]apperrors.FieldViolation{{Field: "roleIds", Message: "Required."}}))
		return
	}
	specs := make([]staffusecases.BindingSpec, 0, len(request.Bindings))
	for _, binding := range request.Bindings {
		specs = append(specs, staffusecases.BindingSpec{RoleID: binding.RoleID, ScopeType: binding.ScopeType,
			ScopeID: binding.ScopeID, ExpiresAt: binding.ExpiresAt})
	}
	if err := h.staff.ReplaceRoleBindings(c.Request.Context(), h.authorizer, staffusecases.ReplaceRoleBindingsInput{
		Tenant: tenantCtx, MemberID: c.Param("staffId"), RoleIDs: request.RoleIDs, Bindings: specs,
		ActorMemberID: tenantCtx.OrganizationUserID, ActorRoleVersion: roleVersionFrom(c)}); err != nil {
		respond.Error(c, err)
		return
	}
	h.listRoleBindingsAfterWrite(c, tenantCtx)
}

func (h *Handler) listRoleBindingsAfterWrite(c *gin.Context, tenantCtx tenant.Context) {
	bindings, err := h.staff.ListRoleBindings(c.Request.Context(), tenantCtx.OrganizationID, c.Param("staffId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]gin.H, 0, len(bindings))
	for _, binding := range bindings {
		views = append(views, bindingView(binding))
	}
	respond.OK(c, gin.H{"staffId": c.Param("staffId"), "bindings": views})
}

// ---- custom roles ----

type roleRequest struct {
	Code        string   `json:"code"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

func (h *Handler) createRole(c *gin.Context) {
	tenantCtx, ok := h.tenantWithPermission(c, "roles.manage")
	if !ok {
		return
	}
	var request roleRequest
	if !bindJSON(c, &request) {
		return
	}
	role, err := h.staff.CreateRole(c.Request.Context(), h.authorizer, staffusecases.RoleInput{
		Tenant: tenantCtx, Code: request.Code, Name: request.Name, Description: request.Description,
		Permissions: request.Permissions, ActorMemberID: tenantCtx.OrganizationUserID, ActorRoleVersion: roleVersionFrom(c)})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.Created(c, role)
}

func (h *Handler) getRole(c *gin.Context) {
	tenantCtx, ok := h.tenantWithPermission(c, "roles.read")
	if !ok {
		return
	}
	role, err := h.staff.Roles().GetRole(c.Request.Context(), tenantCtx.OrganizationID, c.Param("roleId"))
	if err != nil {
		respond.Error(c, apperrors.NotFound("Role not found.", nil))
		return
	}
	respond.OK(c, role)
}

func (h *Handler) updateRole(c *gin.Context) {
	tenantCtx, ok := h.tenantWithPermission(c, "roles.manage")
	if !ok {
		return
	}
	expected, err := expectedVersionFrom(c)
	if err != nil {
		respond.Error(c, err)
		return
	}
	var request roleRequest
	if !bindJSON(c, &request) {
		return
	}
	role, err := h.staff.UpdateRole(c.Request.Context(), h.authorizer, staffusecases.RoleInput{
		Tenant: tenantCtx, RoleID: c.Param("roleId"), Name: request.Name, Description: request.Description,
		Permissions: request.Permissions, ExpectedVersion: expected,
		ActorMemberID: tenantCtx.OrganizationUserID, ActorRoleVersion: roleVersionFrom(c)})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, role)
}

func (h *Handler) archiveRole(c *gin.Context) {
	tenantCtx, ok := h.tenantWithPermission(c, "roles.manage")
	if !ok {
		return
	}
	expected, err := expectedVersionFrom(c)
	if err != nil {
		respond.Error(c, err)
		return
	}
	if err := h.staff.ArchiveRole(c.Request.Context(), staffusecases.RoleInput{
		Tenant: tenantCtx, RoleID: c.Param("roleId"), ExpectedVersion: expected}); err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, gin.H{"roleId": c.Param("roleId"), "status": "ARCHIVED"})
}
