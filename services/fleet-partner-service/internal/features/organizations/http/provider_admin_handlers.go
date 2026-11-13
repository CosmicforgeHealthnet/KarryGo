package http

import (
	"time"

	"github.com/gin-gonic/gin"

	providermodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/models"
	providerusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
)

// Managed-provider administration routes (spec Sections 10 and 39.3).
func (h *Handler) registerProviderAdminRoutes(group *gin.RouterGroup) {
	group.PATCH("/providers/:membershipId", h.updateProvider)
	group.POST("/providers/:membershipId/reactivate", h.reactivateProvider)
	group.POST("/providers/:membershipId/remove", h.removeProvider)
	group.GET("/providers/:membershipId/capabilities", h.providerCapabilities)
	group.PUT("/providers/:membershipId/capabilities", h.replaceProviderCapabilities)
	group.GET("/providers/:membershipId/policies", h.providerPolicy)
	group.PUT("/providers/:membershipId/policies", h.replaceProviderPolicy)
	group.GET("/providers/:membershipId/declines", h.providerDeclines)
}

type updateProviderRequest struct {
	HomeBranchID     *string `json:"homeBranchId"`
	RelationshipType *string `json:"relationshipType"`
	EmploymentType   *string `json:"employmentType"`
	EmployeeNumber   *string `json:"employeeNumber"`
}

func (h *Handler) updateProvider(c *gin.Context) {
	tenantCtx, ok := h.tenantWithPermission(c, "providers.manage")
	if !ok {
		return
	}
	expected, err := expectedVersionFrom(c)
	if err != nil {
		respond.Error(c, err)
		return
	}
	var request updateProviderRequest
	if !bindJSON(c, &request) {
		return
	}
	membership, err := h.providers.UpdateMembership(c.Request.Context(), providerusecases.UpdateMembershipInput{
		Tenant: tenantCtx, MembershipID: c.Param("membershipId"), HomeBranchID: request.HomeBranchID,
		RelationshipType: request.RelationshipType, EmploymentType: request.EmploymentType,
		EmployeeNumber: request.EmployeeNumber, ExpectedVersion: expected})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, providerViews([]providerMembership{*membership})[0])
}

func (h *Handler) reactivateProvider(c *gin.Context) {
	tenantCtx, ok := h.tenantWithPermission(c, "providers.manage")
	if !ok {
		return
	}
	expected, err := expectedVersionFrom(c)
	if err != nil {
		respond.Error(c, err)
		return
	}
	if err := h.providers.Reactivate(c.Request.Context(), tenantCtx, c.Param("membershipId"), expected); err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, gin.H{"membershipId": c.Param("membershipId"), "status": providermodels.StatusActive})
}

type removeProviderRequest struct {
	ReasonCode string `json:"reasonCode"`
}

func (h *Handler) removeProvider(c *gin.Context) {
	tenantCtx, ok := h.tenantWithPermission(c, "providers.remove")
	if !ok {
		return
	}
	expected, err := expectedVersionFrom(c)
	if err != nil {
		respond.Error(c, err)
		return
	}
	var request removeProviderRequest
	_ = c.ShouldBindJSON(&request)
	if err := h.providers.Remove(c.Request.Context(), tenantCtx, c.Param("membershipId"), request.ReasonCode, expected); err != nil {
		respond.Error(c, err)
		return
	}
	membership, err := h.providers.Repository().GetMembership(c.Request.Context(), tenantCtx.OrganizationID, c.Param("membershipId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, gin.H{"membershipId": membership.ID, "status": membership.Status})
}

func capabilityView(capability providermodels.Capability) gin.H {
	return gin.H{
		"id": capability.ID, "serviceType": capability.ServiceType, "vehicleClasses": capability.VehicleClasses,
		"operatingZones": capability.OperatingZones, "qualifications": capability.Qualifications,
		"validFrom": capability.ValidFrom, "validUntil": capability.ValidUntil, "status": capability.Status,
	}
}

func (h *Handler) providerCapabilities(c *gin.Context) {
	tenantCtx, ok := h.tenantWithPermission(c, "providers.read")
	if !ok {
		return
	}
	capabilities, err := h.providers.Capabilities(c.Request.Context(), tenantCtx.OrganizationID, c.Param("membershipId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]gin.H, 0, len(capabilities))
	for _, capability := range capabilities {
		views = append(views, capabilityView(capability))
	}
	respond.List(c, views, "")
}

type capabilityRequest struct {
	ServiceType    string     `json:"serviceType"`
	VehicleClasses []string   `json:"vehicleClasses"`
	OperatingZones []string   `json:"operatingZones"`
	Qualifications []string   `json:"qualifications"`
	ValidUntil     *time.Time `json:"validUntil"`
}

func (h *Handler) replaceProviderCapabilities(c *gin.Context) {
	tenantCtx, ok := h.tenantWithPermission(c, "providers.manage_capabilities")
	if !ok {
		return
	}
	var request struct {
		Capabilities []capabilityRequest `json:"capabilities"`
	}
	if !bindJSON(c, &request) {
		return
	}
	capabilities := make([]providermodels.Capability, 0, len(request.Capabilities))
	for _, item := range request.Capabilities {
		capabilities = append(capabilities, providermodels.Capability{ServiceType: item.ServiceType,
			VehicleClasses: item.VehicleClasses, OperatingZones: item.OperatingZones,
			Qualifications: item.Qualifications, ValidUntil: item.ValidUntil})
	}
	saved, err := h.providers.ReplaceCapabilities(c.Request.Context(), tenantCtx, c.Param("membershipId"), capabilities)
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]gin.H, 0, len(saved))
	for _, capability := range saved {
		views = append(views, capabilityView(capability))
	}
	respond.OK(c, gin.H{"membershipId": c.Param("membershipId"), "capabilities": views})
}

func policyView(policy *providermodels.Policy) gin.H {
	return gin.H{
		"membershipId": policy.MembershipID, "assignmentMode": policy.AssignmentMode,
		"providerDeclineRequired": policy.ProviderDeclineRequired, "visibilityPolicy": policy.VisibilityPolicy,
		"shiftRestrictions": policy.ShiftRestrictions, "zoneRestrictions": policy.ZoneRestrictions,
		"workloadRestrictions": policy.WorkloadRestrictions, "version": policy.Version, "updatedAt": policy.UpdatedAt,
	}
}

func (h *Handler) providerPolicy(c *gin.Context) {
	tenantCtx, ok := h.tenantWithPermission(c, "providers.read")
	if !ok {
		return
	}
	policy, err := h.providers.Policy(c.Request.Context(), tenantCtx.OrganizationID, c.Param("membershipId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, policyView(policy))
}

type policyRequest struct {
	AssignmentMode          string         `json:"assignmentMode"`
	ProviderDeclineRequired *bool          `json:"providerDeclineRequired"`
	VisibilityPolicy        string         `json:"visibilityPolicy"`
	ShiftRestrictions       map[string]any `json:"shiftRestrictions"`
	ZoneRestrictions        map[string]any `json:"zoneRestrictions"`
	WorkloadRestrictions    map[string]any `json:"workloadRestrictions"`
}

func (h *Handler) replaceProviderPolicy(c *gin.Context) {
	tenantCtx, ok := h.tenantWithPermission(c, "providers.manage_policies")
	if !ok {
		return
	}
	var request policyRequest
	if !bindJSON(c, &request) {
		return
	}
	decline := true
	if request.ProviderDeclineRequired != nil {
		decline = *request.ProviderDeclineRequired
	}
	policy, err := h.providers.ReplacePolicy(c.Request.Context(), tenantCtx, providermodels.Policy{
		MembershipID: c.Param("membershipId"), AssignmentMode: request.AssignmentMode,
		ProviderDeclineRequired: decline, VisibilityPolicy: request.VisibilityPolicy,
		ShiftRestrictions: request.ShiftRestrictions, ZoneRestrictions: request.ZoneRestrictions,
		WorkloadRestrictions: request.WorkloadRestrictions}, request.ProviderDeclineRequired != nil)
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, policyView(policy))
}

func (h *Handler) providerDeclines(c *gin.Context) {
	tenantCtx, ok := h.tenantWithPermission(c, "providers.read")
	if !ok {
		return
	}
	declines, err := h.providers.Declines(c.Request.Context(), tenantCtx.OrganizationID, c.Param("membershipId"), 100)
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.List(c, declines, "")
}
