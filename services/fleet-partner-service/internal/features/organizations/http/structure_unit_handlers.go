package http

import (
	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"

	orgmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/models"
	orgrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/repositories"
	orgusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Cost centres, teams and operating locations (spec 37.3, 57.1, 59.2). Same
// conventions as branches and departments: organization.read /
// organization.update, If-Match on edits, out-of-branch units answer as
// missing.

func branchHidden(tenantCtx tenant.Context, branchID *string) bool {
	return branchID != nil && !tenantCtx.BranchPermitted(*branchID)
}

// requestedBranchPermitted refuses writing a unit into a branch the caller
// cannot reach.
func requestedBranchPermitted(c *gin.Context, tenantCtx tenant.Context, branchID *string) bool {
	if branchID != nil && *branchID != "" && !tenantCtx.BranchPermitted(*branchID) {
		respond.Error(c, apperrors.NotFound("Branch not found.", nil))
		return false
	}
	return true
}

// ---- cost centres ----

func costCentreView(unit *orgmodels.CostCentre) gin.H {
	return gin.H{
		"id": unit.ID, "costCentreId": unit.ID, "organizationId": unit.OrganizationID,
		"branchId": unit.BranchID, "code": unit.Code, "name": unit.Name,
		"budgetOwnerUserId": unit.BudgetOwnerUserID, "status": unit.Status,
		"version": unit.Version, "createdAt": unit.CreatedAt, "updatedAt": unit.UpdatedAt,
		"archivedAt": unit.ArchivedAt,
	}
}

type costCentreRequest struct {
	Code              string  `json:"code"`
	Name              string  `json:"name"`
	BranchID          *string `json:"branchId"`
	BudgetOwnerUserID *string `json:"budgetOwnerUserId"`
}

func (h *Handler) listCostCentres(c *gin.Context) {
	tenantCtx, ok := h.tenantWithPermission(c, "organization.read")
	if !ok {
		return
	}
	units, err := h.organizations.ListCostCentres(c.Request.Context(), tenantCtx.OrganizationID,
		c.Query("branchId"), c.Query("includeArchived") == "true")
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]gin.H, 0, len(units))
	for index := range units {
		if !branchHidden(tenantCtx, units[index].BranchID) {
			views = append(views, costCentreView(&units[index]))
		}
	}
	respond.List(c, views, "")
}

func (h *Handler) getCostCentre(c *gin.Context) {
	tenantCtx, ok := h.tenantWithPermission(c, "organization.read")
	if !ok {
		return
	}
	unit, err := h.organizations.GetCostCentre(c.Request.Context(), tenantCtx.OrganizationID, c.Param("costCentreId"))
	if err == nil && branchHidden(tenantCtx, unit.BranchID) {
		err = apperrors.NotFound("Cost centre not found.", nil)
	}
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, costCentreView(unit))
}

func (h *Handler) createCostCentre(c *gin.Context) {
	tenantCtx, ok := h.tenantWithPermission(c, "organization.update")
	if !ok {
		return
	}
	var request costCentreRequest
	if !bindJSON(c, &request) || !requestedBranchPermitted(c, tenantCtx, request.BranchID) {
		return
	}
	unit, err := h.organizations.CreateCostCentre(c.Request.Context(), orgusecases.CostCentreInput{
		Tenant: tenantCtx, Code: request.Code, Name: request.Name, BranchID: request.BranchID,
		BudgetOwnerUserID: request.BudgetOwnerUserID})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.Created(c, costCentreView(unit))
}

// visibleCostCentre resolves a path cost centre the caller may write to.
func (h *Handler) visibleCostCentre(c *gin.Context, tenantCtx tenant.Context) bool {
	unit, err := h.organizations.GetCostCentre(c.Request.Context(), tenantCtx.OrganizationID, c.Param("costCentreId"))
	if err == nil && branchHidden(tenantCtx, unit.BranchID) {
		err = apperrors.NotFound("Cost centre not found.", nil)
	}
	if err != nil {
		respond.Error(c, err)
		return false
	}
	return true
}

func (h *Handler) updateCostCentre(c *gin.Context) {
	tenantCtx, ok := h.tenantWithPermission(c, "organization.update")
	if !ok || !h.visibleCostCentre(c, tenantCtx) {
		return
	}
	expected, err := expectedVersionFrom(c)
	if err != nil {
		respond.Error(c, err)
		return
	}
	var request costCentreRequest
	if !bindJSON(c, &request) || !requestedBranchPermitted(c, tenantCtx, request.BranchID) {
		return
	}
	unit, err := h.organizations.UpdateCostCentre(c.Request.Context(), orgusecases.CostCentreInput{
		Tenant: tenantCtx, CostCentreID: c.Param("costCentreId"), Name: request.Name,
		BranchID: request.BranchID, BudgetOwnerUserID: request.BudgetOwnerUserID, ExpectedVersion: expected})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, costCentreView(unit))
}

func (h *Handler) archiveCostCentre(c *gin.Context) {
	tenantCtx, ok := h.tenantWithPermission(c, "organization.update")
	if !ok || !h.visibleCostCentre(c, tenantCtx) {
		return
	}
	expected, err := expectedVersionFrom(c)
	if err != nil {
		respond.Error(c, err)
		return
	}
	if err := h.organizations.ArchiveCostCentre(c.Request.Context(), tenantCtx, c.Param("costCentreId"), expected); err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, gin.H{"costCentreId": c.Param("costCentreId"), "status": orgmodels.UnitStatusArchived})
}

// ---- teams ----

func teamView(team *orgmodels.Team) gin.H {
	return gin.H{
		"id": team.ID, "teamId": team.ID, "organizationId": team.OrganizationID,
		"branchId": team.BranchID, "departmentId": team.DepartmentID, "code": team.Code,
		"name": team.Name, "supervisorUserId": team.SupervisorUserID, "status": team.Status,
		"version": team.Version, "createdAt": team.CreatedAt, "updatedAt": team.UpdatedAt,
		"archivedAt": team.ArchivedAt,
	}
}

type teamRequest struct {
	Code             string  `json:"code"`
	Name             string  `json:"name"`
	BranchID         *string `json:"branchId"`
	DepartmentID     *string `json:"departmentId"`
	SupervisorUserID *string `json:"supervisorUserId"`
}

func (h *Handler) listTeams(c *gin.Context) {
	tenantCtx, ok := h.tenantWithPermission(c, "organization.read")
	if !ok {
		return
	}
	teams, err := h.organizations.ListTeams(c.Request.Context(), tenantCtx.OrganizationID, orgrepos.TeamFilter{
		BranchID: c.Query("branchId"), DepartmentID: c.Query("departmentId"),
		IncludeArchived: c.Query("includeArchived") == "true"})
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]gin.H, 0, len(teams))
	for index := range teams {
		if !branchHidden(tenantCtx, teams[index].BranchID) {
			views = append(views, teamView(&teams[index]))
		}
	}
	respond.List(c, views, "")
}

func (h *Handler) getTeam(c *gin.Context) {
	tenantCtx, ok := h.tenantWithPermission(c, "organization.read")
	if !ok {
		return
	}
	team, err := h.organizations.GetTeam(c.Request.Context(), tenantCtx.OrganizationID, c.Param("teamId"))
	if err == nil && branchHidden(tenantCtx, team.BranchID) {
		err = apperrors.NotFound("Team not found.", nil)
	}
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, teamView(team))
}

func (h *Handler) createTeam(c *gin.Context) {
	tenantCtx, ok := h.tenantWithPermission(c, "organization.update")
	if !ok {
		return
	}
	var request teamRequest
	if !bindJSON(c, &request) || !requestedBranchPermitted(c, tenantCtx, request.BranchID) {
		return
	}
	team, err := h.organizations.CreateTeam(c.Request.Context(), orgusecases.TeamInput{
		Tenant: tenantCtx, Code: request.Code, Name: request.Name, BranchID: request.BranchID,
		DepartmentID: request.DepartmentID, SupervisorUserID: request.SupervisorUserID})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.Created(c, teamView(team))
}

func (h *Handler) visibleTeam(c *gin.Context, tenantCtx tenant.Context) bool {
	team, err := h.organizations.GetTeam(c.Request.Context(), tenantCtx.OrganizationID, c.Param("teamId"))
	if err == nil && branchHidden(tenantCtx, team.BranchID) {
		err = apperrors.NotFound("Team not found.", nil)
	}
	if err != nil {
		respond.Error(c, err)
		return false
	}
	return true
}

func (h *Handler) updateTeam(c *gin.Context) {
	tenantCtx, ok := h.tenantWithPermission(c, "organization.update")
	if !ok || !h.visibleTeam(c, tenantCtx) {
		return
	}
	expected, err := expectedVersionFrom(c)
	if err != nil {
		respond.Error(c, err)
		return
	}
	var request teamRequest
	if !bindJSON(c, &request) || !requestedBranchPermitted(c, tenantCtx, request.BranchID) {
		return
	}
	team, err := h.organizations.UpdateTeam(c.Request.Context(), orgusecases.TeamInput{
		Tenant: tenantCtx, TeamID: c.Param("teamId"), Name: request.Name, BranchID: request.BranchID,
		DepartmentID: request.DepartmentID, SupervisorUserID: request.SupervisorUserID,
		ExpectedVersion: expected})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, teamView(team))
}

func (h *Handler) archiveTeam(c *gin.Context) {
	tenantCtx, ok := h.tenantWithPermission(c, "organization.update")
	if !ok || !h.visibleTeam(c, tenantCtx) {
		return
	}
	expected, err := expectedVersionFrom(c)
	if err != nil {
		respond.Error(c, err)
		return
	}
	if err := h.organizations.ArchiveTeam(c.Request.Context(), tenantCtx, c.Param("teamId"), expected); err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, gin.H{"teamId": c.Param("teamId"), "status": orgmodels.UnitStatusArchived})
}

// ---- operating locations ----

func locationView(location *orgmodels.OperatingLocation) gin.H {
	return gin.H{
		"id": location.ID, "locationId": location.ID, "organizationId": location.OrganizationID,
		"branchId": location.BranchID, "code": location.Code, "name": location.Name,
		"locationType": location.LocationType, "address": location.Address,
		"latitude": location.Latitude, "longitude": location.Longitude,
		"operatingHours": location.OperatingHours, "status": location.Status,
		"version": location.Version, "createdAt": location.CreatedAt, "updatedAt": location.UpdatedAt,
		"archivedAt": location.ArchivedAt,
	}
}

type locationRequest struct {
	Code           string         `json:"code"`
	Name           string         `json:"name"`
	LocationType   string         `json:"locationType"`
	BranchID       *string        `json:"branchId"`
	Address        map[string]any `json:"address"`
	Latitude       *float64       `json:"latitude"`
	Longitude      *float64       `json:"longitude"`
	OperatingHours map[string]any `json:"operatingHours"`
	Status         string         `json:"status"`
}

func (h *Handler) listLocations(c *gin.Context) {
	tenantCtx, ok := h.tenantWithPermission(c, "organization.read")
	if !ok {
		return
	}
	locations, err := h.organizations.ListLocations(c.Request.Context(), tenantCtx.OrganizationID, orgrepos.LocationFilter{
		BranchID: c.Query("branchId"), LocationType: c.Query("locationType"),
		IncludeArchived: c.Query("includeArchived") == "true"})
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]gin.H, 0, len(locations))
	for index := range locations {
		if !branchHidden(tenantCtx, locations[index].BranchID) {
			views = append(views, locationView(&locations[index]))
		}
	}
	respond.List(c, views, "")
}

func (h *Handler) getLocation(c *gin.Context) {
	tenantCtx, ok := h.tenantWithPermission(c, "organization.read")
	if !ok {
		return
	}
	location, err := h.organizations.GetLocation(c.Request.Context(), tenantCtx.OrganizationID, c.Param("locationId"))
	if err == nil && branchHidden(tenantCtx, location.BranchID) {
		err = apperrors.NotFound("Location not found.", nil)
	}
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, locationView(location))
}

func (h *Handler) createLocation(c *gin.Context) {
	tenantCtx, ok := h.tenantWithPermission(c, "organization.update")
	if !ok {
		return
	}
	var request locationRequest
	if !bindJSON(c, &request) || !requestedBranchPermitted(c, tenantCtx, request.BranchID) {
		return
	}
	location, err := h.organizations.CreateLocation(c.Request.Context(), orgusecases.LocationInput{
		Tenant: tenantCtx, Code: request.Code, Name: request.Name, LocationType: request.LocationType,
		BranchID: request.BranchID, Address: request.Address, Latitude: request.Latitude,
		Longitude: request.Longitude, OperatingHours: request.OperatingHours})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.Created(c, locationView(location))
}

func (h *Handler) visibleLocation(c *gin.Context, tenantCtx tenant.Context) bool {
	location, err := h.organizations.GetLocation(c.Request.Context(), tenantCtx.OrganizationID, c.Param("locationId"))
	if err == nil && branchHidden(tenantCtx, location.BranchID) {
		err = apperrors.NotFound("Location not found.", nil)
	}
	if err != nil {
		respond.Error(c, err)
		return false
	}
	return true
}

func (h *Handler) updateLocation(c *gin.Context) {
	tenantCtx, ok := h.tenantWithPermission(c, "organization.update")
	if !ok || !h.visibleLocation(c, tenantCtx) {
		return
	}
	expected, err := expectedVersionFrom(c)
	if err != nil {
		respond.Error(c, err)
		return
	}
	var request locationRequest
	if !bindJSON(c, &request) || !requestedBranchPermitted(c, tenantCtx, request.BranchID) {
		return
	}
	location, err := h.organizations.UpdateLocation(c.Request.Context(), orgusecases.LocationInput{
		Tenant: tenantCtx, LocationID: c.Param("locationId"), Name: request.Name,
		LocationType: request.LocationType, BranchID: request.BranchID, Address: request.Address,
		Latitude: request.Latitude, Longitude: request.Longitude, OperatingHours: request.OperatingHours,
		Status: request.Status, ExpectedVersion: expected})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, locationView(location))
}

func (h *Handler) archiveLocation(c *gin.Context) {
	tenantCtx, ok := h.tenantWithPermission(c, "organization.update")
	if !ok || !h.visibleLocation(c, tenantCtx) {
		return
	}
	expected, err := expectedVersionFrom(c)
	if err != nil {
		respond.Error(c, err)
		return
	}
	if err := h.organizations.ArchiveLocation(c.Request.Context(), tenantCtx, c.Param("locationId"), expected); err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, gin.H{"locationId": c.Param("locationId"), "status": orgmodels.UnitStatusArchived})
}
