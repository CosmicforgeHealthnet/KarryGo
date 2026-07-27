// Package http exposes the corporate organization, staff and provider APIs.
package http

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/auth"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	orgmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/models"
	orgusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/usecases"
	providerusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/usecases"
	staffusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/concurrency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Handler serves the corporate organization/staff/provider routes.
type Handler struct {
	organizations *orgusecases.Service
	staff         *staffusecases.Service
	providers     *providerusecases.Service
	authorizer    *accesscontrol.Authorizer
}

// NewHandler builds the handler.
func NewHandler(organizations *orgusecases.Service, staffService *staffusecases.Service, providers *providerusecases.Service, authorizer *accesscontrol.Authorizer) *Handler {
	return &Handler{organizations: organizations, staff: staffService, providers: providers, authorizer: authorizer}
}

// RegisterOnboardingRoutes mounts routes available to an authenticated
// corporate identity that may not yet have an organization.
func (h *Handler) RegisterOnboardingRoutes(group *gin.RouterGroup) {
	group.POST("/organizations", h.createOrganization)
}

// RegisterTenantRoutes mounts routes that require resolved organization scope.
func (h *Handler) RegisterTenantRoutes(group *gin.RouterGroup) {
	group.GET("/organizations/current", h.currentOrganization)
	group.GET("/organizations/:organizationId/onboarding-status", h.onboardingStatus)
	group.POST("/organizations/:organizationId/documents", h.attachDocument)
	group.GET("/organizations/:organizationId/documents", h.listDocuments)
	group.POST("/organizations/:organizationId/submit", h.submit)
	group.GET("/organizations/:organizationId/verification", h.verificationStatus)
	group.GET("/organizations/:organizationId/wallet-provisioning", h.walletStatus)
	group.POST("/organizations/:organizationId/wallet-provisioning/retry", h.retryWallet)
	group.POST("/organizations/:organizationId/close-request", h.requestClosure)
	group.GET("/organizations/:organizationId/closure-status", h.closureStatus)

	group.GET("/permissions", h.listPermissions)
	group.GET("/roles", h.listRoles)

	group.GET("/staff", h.listStaff)
	group.POST("/staff/invitations", h.inviteStaff)
	group.POST("/staff/:staffId/suspend", h.suspendStaff)
	group.POST("/staff/:staffId/reactivate", h.reactivateStaff)

	group.GET("/providers", h.listProviders)
	group.POST("/providers", h.createProvider)
	group.GET("/providers/:membershipId", h.getProvider)
	group.POST("/providers/:membershipId/suspend", h.suspendProvider)
	group.GET("/providers/:membershipId/provisioning-status", h.providerProvisioningStatus)
	group.GET("/providers/:membershipId/history", h.providerHistory)
}

// requireTenant resolves the tenant context or fails closed.
func requireTenant(c *gin.Context) (tenant.Context, bool) {
	tenantCtx, ok := middleware.TenantFromGin(c)
	if !ok {
		respond.Error(c, apperrors.Forbidden("Organization scope is required.", nil))
		return tenant.Context{}, false
	}
	return tenantCtx, true
}

// requireScopedPath refuses a path organization ID that differs from the
// authenticated tenant, so a path parameter can never widen scope.
func requireScopedPath(c *gin.Context, tenantCtx tenant.Context) bool {
	pathOrg := c.Param("organizationId")
	if pathOrg != "" && pathOrg != tenantCtx.OrganizationID {
		// Never reveal that another organization exists.
		respond.Error(c, apperrors.NotFound("Organization not found.", nil))
		return false
	}
	return true
}

// authorize enforces a permission for the caller's membership.
func (h *Handler) authorize(c *gin.Context, tenantCtx tenant.Context, permission string) bool {
	err := h.authorizer.Require(c.Request.Context(), tenantCtx.OrganizationID,
		tenantCtx.OrganizationUserID, roleVersionFrom(c), permission, accesscontrol.Target{})
	if err != nil {
		respond.Error(c, err)
		return false
	}
	return true
}

func roleVersionFrom(c *gin.Context) int64 {
	if value, ok := c.Get(middleware.TenantRoleVersionKey); ok {
		if version, ok := value.(int64); ok {
			return version
		}
	}
	return 1
}

type createOrganizationRequest struct {
	LegalName          string `json:"legalName"`
	TradingName        string `json:"tradingName"`
	RegistrationNumber string `json:"registrationNumber"`
	CountryCode        string `json:"countryCode"`
	OrganizationType   string `json:"organizationType"`
	Timezone           string `json:"timezone"`
	DefaultCurrency    string `json:"defaultCurrency"`
	OwnerName          string `json:"ownerName"`
}

func (h *Handler) createOrganization(c *gin.Context) {
	var request createOrganizationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
		return
	}
	if c.GetHeader("Idempotency-Key") == "" {
		respond.Error(c, apperrors.BadRequest("Idempotency-Key header is required.", nil))
		return
	}
	claims, _ := auth.ClaimsFromContext(c)

	organization, err := h.organizations.CreateDraft(c.Request.Context(), orgusecases.CreateDraftInput{
		IdentityID:         claims.Subject,
		OwnerName:          request.OwnerName,
		LegalName:          request.LegalName,
		TradingName:        request.TradingName,
		RegistrationNumber: request.RegistrationNumber,
		CountryCode:        request.CountryCode,
		OrganizationType:   request.OrganizationType,
		Timezone:           request.Timezone,
		DefaultCurrency:    request.DefaultCurrency,
	})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.Created(c, gin.H{
		"organizationId": organization.ID,
		"status":         organization.Status,
		"version":        organization.Version,
	})
}

func (h *Handler) currentOrganization(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, "organization.read") {
		return
	}
	organizations, _ := h.organizations.Repositories()
	organization, err := organizations.GetByID(c.Request.Context(), tenantCtx.OrganizationID)
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, gin.H{
		"organizationId":  organization.ID,
		"legalName":       organization.LegalName,
		"tradingName":     organization.TradingName,
		"status":          organization.Status,
		"countryCode":     organization.CountryCode,
		"timezone":        organization.Timezone,
		"defaultCurrency": organization.DefaultCurrency,
		"version":         organization.Version,
	})
}

func (h *Handler) onboardingStatus(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok || !requireScopedPath(c, tenantCtx) {
		return
	}
	if !h.authorize(c, tenantCtx, "organization.read") {
		return
	}
	status, err := h.organizations.GetOnboardingStatus(c.Request.Context(), tenantCtx.OrganizationID)
	if err != nil {
		respond.Error(c, err)
		return
	}
	stages := make([]gin.H, 0, len(status.Stages))
	for _, stage := range status.Stages {
		stages = append(stages, gin.H{
			"stage": stage.Stage, "status": stage.Status,
			"missingFields": stage.MissingFields, "blockingIssues": stage.BlockingIssues,
			"completedAt": stage.CompletedAt,
		})
	}
	body := gin.H{
		"organizationId":      status.OrganizationID,
		"organizationStatus":  status.OrganizationStatus,
		"organizationVersion": status.OrganizationVersion,
		"stages":              stages,
		"allowedNextActions":  status.AllowedNextActions,
		"updatedAt":           status.UpdatedAt,
	}
	if status.Verification != nil {
		body["verification"] = gin.H{
			"status":            status.Verification.Status,
			"submissionVersion": status.Verification.SubmissionVersion,
			"issues":            status.VerificationIssues,
		}
	}
	if status.Wallet != nil {
		// Only the provisioning state — never a balance.
		body["walletProvisioning"] = gin.H{"status": status.Wallet.Status}
	}
	respond.OK(c, body)
}

type attachDocumentRequest struct {
	DocumentType string `json:"documentType"`
	FileID       string `json:"fileId"`
}

func (h *Handler) attachDocument(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok || !requireScopedPath(c, tenantCtx) {
		return
	}
	if !h.authorize(c, tenantCtx, "organization.update") {
		return
	}
	var request attachDocumentRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
		return
	}
	document, err := h.organizations.AttachDocument(c.Request.Context(), orgusecases.AttachDocumentInput{
		Tenant: tenantCtx, DocumentType: request.DocumentType, FileID: request.FileID,
	})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.Created(c, gin.H{"documentId": document.ID, "status": document.Status})
}

func (h *Handler) listDocuments(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok || !requireScopedPath(c, tenantCtx) {
		return
	}
	if !h.authorize(c, tenantCtx, "organization.read") {
		return
	}
	_, onboarding := h.organizations.Repositories()
	documents, err := onboarding.ListDocuments(c.Request.Context(), tenantCtx.OrganizationID)
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.List(c, documents, "")
}

func (h *Handler) submit(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok || !requireScopedPath(c, tenantCtx) {
		return
	}
	if !h.authorize(c, tenantCtx, "organization.submit") {
		return
	}
	if c.GetHeader("Idempotency-Key") == "" {
		respond.Error(c, apperrors.BadRequest("Idempotency-Key header is required.", nil))
		return
	}
	expectedVersion, err := expectedVersionFrom(c)
	if err != nil {
		respond.Error(c, err)
		return
	}
	request, err := h.organizations.Submit(c.Request.Context(), orgusecases.SubmitInput{
		Tenant: tenantCtx, ExpectedVersion: expectedVersion,
	})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.Created(c, gin.H{
		"verificationRequestId": request.ID,
		"submissionVersion":     request.SubmissionVersion,
		"status":                request.Status,
	})
}

func (h *Handler) verificationStatus(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok || !requireScopedPath(c, tenantCtx) {
		return
	}
	if !h.authorize(c, tenantCtx, "organization.read") {
		return
	}
	_, onboarding := h.organizations.Repositories()
	request, err := onboarding.LatestVerificationRequest(c.Request.Context(), tenantCtx.OrganizationID)
	if err != nil {
		respond.Error(c, err)
		return
	}
	issues, err := onboarding.ListVerificationIssues(c.Request.Context(), tenantCtx.OrganizationID, request.ID)
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, gin.H{
		"status": request.Status, "submissionVersion": request.SubmissionVersion, "issues": issues,
	})
}

func (h *Handler) walletStatus(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok || !requireScopedPath(c, tenantCtx) {
		return
	}
	if !h.authorize(c, tenantCtx, "organization.read") {
		return
	}
	_, onboarding := h.organizations.Repositories()
	wallet, err := onboarding.GetWalletProvisioning(c.Request.Context(), tenantCtx.OrganizationID)
	if err != nil {
		respond.Error(c, err)
		return
	}
	// The wallet ledger belongs to payment-wallet-service: no balance here.
	respond.OK(c, gin.H{"status": wallet.Status, "attempts": wallet.Attempts})
}

func (h *Handler) retryWallet(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok || !requireScopedPath(c, tenantCtx) {
		return
	}
	if !h.authorize(c, tenantCtx, "organization.update") {
		return
	}
	wallet, err := h.organizations.RequestWalletProvisioning(c.Request.Context(), tenantCtx)
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, gin.H{"status": wallet.Status})
}

type closureRequest struct {
	ReasonCode string `json:"reasonCode"`
	ReasonText string `json:"reasonText"`
}

func (h *Handler) requestClosure(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok || !requireScopedPath(c, tenantCtx) {
		return
	}
	if !h.authorize(c, tenantCtx, "organization.close") {
		return
	}
	var request closureRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
		return
	}
	closure, err := h.organizations.RequestClosure(c.Request.Context(), orgusecases.RequestClosureInput{
		Tenant: tenantCtx, ReasonCode: request.ReasonCode, ReasonText: request.ReasonText,
	})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.Created(c, gin.H{"closureRequestId": closure.ID, "status": closure.Status})
}

func (h *Handler) closureStatus(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok || !requireScopedPath(c, tenantCtx) {
		return
	}
	if !h.authorize(c, tenantCtx, "organization.read") {
		return
	}
	closure, blockers, err := h.organizations.GetClosureStatus(c.Request.Context(), tenantCtx.OrganizationID)
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, gin.H{"status": closure.Status, "blockers": blockers})
}

func (h *Handler) listPermissions(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, "roles.read") {
		return
	}
	permissions, err := h.staff.Roles().ListPermissions(c.Request.Context())
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.List(c, permissions, "")
}

func (h *Handler) listRoles(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, "roles.read") {
		return
	}
	roles, err := h.staff.Roles().ListRoles(c.Request.Context(), tenantCtx.OrganizationID)
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.List(c, roles, "")
}

func (h *Handler) listStaff(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, "staff.read") {
		return
	}
	members, err := h.staff.ListStaff(c.Request.Context(), tenant.Query{
		OrganizationID:     tenantCtx.OrganizationID,
		PermittedBranchIDs: tenantCtx.PermittedBranchIDs,
	})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.List(c, members, "")
}

type inviteStaffRequest struct {
	FullName  string   `json:"fullName"`
	Email     string   `json:"email"`
	Phone     string   `json:"phone"`
	RoleIDs   []string `json:"roleIds"`
	BranchIDs []string `json:"branchIds"`
}

func (h *Handler) inviteStaff(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, "staff.invite") {
		return
	}
	var request inviteStaffRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
		return
	}
	invitation, err := h.staff.Invite(c.Request.Context(), staffusecases.InviteInput{
		Tenant: tenantCtx, FullName: request.FullName, Email: request.Email, Phone: request.Phone,
		RoleIDs: request.RoleIDs, BranchIDs: request.BranchIDs,
	})
	if err != nil {
		respond.Error(c, err)
		return
	}
	// The invitation token is never returned in the API response.
	respond.Created(c, gin.H{
		"invitationId": invitation.ID,
		"status":       invitation.Status,
		"expiresAt":    invitation.ExpiresAt,
	})
}

func (h *Handler) suspendStaff(c *gin.Context) { h.changeStaffStatus(c, "SUSPENDED", "staff.suspend") }

func (h *Handler) reactivateStaff(c *gin.Context) { h.changeStaffStatus(c, "ACTIVE", "staff.manage") }

func (h *Handler) changeStaffStatus(c *gin.Context, status string, permission string) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, permission) {
		return
	}
	expectedVersion, err := expectedVersionFrom(c)
	if err != nil {
		respond.Error(c, err)
		return
	}
	if err := h.staff.ChangeStaffStatus(c.Request.Context(), staffusecases.ChangeStatusInput{
		Tenant: tenantCtx, MemberID: c.Param("staffId"), Status: status,
		ReasonCode: "requested_by_organization", ExpectedVersion: expectedVersion,
	}); err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, gin.H{"staffId": c.Param("staffId"), "status": status})
}

type createProviderRequest struct {
	LegalName        string `json:"legalName"`
	Email            string `json:"email"`
	Phone            string `json:"phone"`
	ServiceCategory  string `json:"serviceCategory"`
	HomeBranchID     string `json:"homeBranchId"`
	RelationshipType string `json:"relationshipType"`
	EmploymentType   string `json:"employmentType"`
	EmployeeNumber   string `json:"employeeNumber"`
}

func (h *Handler) createProvider(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, "providers.create") {
		return
	}
	var request createProviderRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
		return
	}
	result, err := h.providers.CreateProvider(c.Request.Context(), providerusecases.CreateProviderInput{
		Tenant: tenantCtx, LegalName: request.LegalName, Email: request.Email, Phone: request.Phone,
		ServiceCategory: request.ServiceCategory, HomeBranchID: request.HomeBranchID,
		RelationshipType: request.RelationshipType, EmploymentType: request.EmploymentType,
		EmployeeNumber: request.EmployeeNumber, IdempotencyKey: c.GetHeader("Idempotency-Key"),
	})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.Created(c, gin.H{
		"membershipId":   result.Membership.ID,
		"status":         result.Membership.Status,
		"invitationSent": result.InvitationSent,
	})
}

func (h *Handler) listProviders(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, "providers.read") {
		return
	}
	memberships, err := h.providers.Repository().ListMemberships(c.Request.Context(), tenant.Query{
		OrganizationID:     tenantCtx.OrganizationID,
		PermittedBranchIDs: tenantCtx.PermittedBranchIDs,
	})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.List(c, providerViews(memberships), "")
}

func (h *Handler) getProvider(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, "providers.read") {
		return
	}
	membership, err := h.providers.Repository().GetMembership(c.Request.Context(),
		tenantCtx.OrganizationID, c.Param("membershipId"))
	if err != nil {
		respond.Error(c, apperrors.NotFound("Provider not found.", nil))
		return
	}
	respond.OK(c, providerViews([]providerMembership{*membership})[0])
}

func (h *Handler) suspendProvider(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, "providers.suspend") {
		return
	}
	expectedVersion, err := expectedVersionFrom(c)
	if err != nil {
		respond.Error(c, err)
		return
	}
	if err := h.providers.ChangeStatus(c.Request.Context(), providerusecases.ChangeStatusInput{
		Tenant: tenantCtx, MembershipID: c.Param("membershipId"), TargetStatus: "SUSPENDED",
		ReasonCode: "requested_by_organization", ExpectedVersion: expectedVersion,
	}); err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, gin.H{"membershipId": c.Param("membershipId"), "status": "SUSPENDED"})
}

func (h *Handler) providerProvisioningStatus(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, "providers.read") {
		return
	}
	saga, err := h.providers.Repository().GetSaga(c.Request.Context(),
		tenantCtx.OrganizationID, c.Param("membershipId"))
	if err != nil {
		respond.Error(c, apperrors.NotFound("Provisioning status not found.", nil))
		return
	}
	respond.OK(c, gin.H{
		"overallStatus":            saga.OverallStatus,
		"providerResolutionStatus": saga.ProviderResolutionStatus,
		"providerIdentityStatus":   saga.ProviderIdentityStatus,
		"associationStatus":        saga.AssociationStatus,
		"subWalletStatus":          saga.SubWalletStatus,
		"notificationStatus":       saga.NotificationStatus,
	})
}

func (h *Handler) providerHistory(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, "providers.read") {
		return
	}
	history, err := h.providers.Repository().ListHistory(c.Request.Context(),
		tenantCtx.OrganizationID, c.Param("membershipId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.List(c, history, "")
}

func expectedVersionFrom(c *gin.Context) (int64, error) {
	ifMatch := c.GetHeader("If-Match")
	if ifMatch == "" {
		return 0, nil
	}
	return concurrency.ParseIfMatch(ifMatch)
}

var _ = orgmodels.StageActivation
var _ = http.StatusOK
