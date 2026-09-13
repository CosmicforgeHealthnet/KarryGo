// Package http exposes the corporate organization, staff and provider APIs.
package http

import (
	"net/http"
		"time"  
		"io"
	"strconv"

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
	sessions      orgusecases.SessionRevoker
}

// NewHandler builds the handler. sessions may be nil (matches every other
// optional dependency convention in this service): the closure/execute
// route then completes without revoking any corporate sessions, which is
// disclosed rather than silently claimed as a security control -- a
// deployment that wires this handler without a session revoker is choosing
// that tradeoff, not hitting a hidden gap.
func NewHandler(organizations *orgusecases.Service, staffService *staffusecases.Service, providers *providerusecases.Service, authorizer *accesscontrol.Authorizer, sessions orgusecases.SessionRevoker) *Handler {
	return &Handler{organizations: organizations, staff: staffService, providers: providers, authorizer: authorizer, sessions: sessions}
}

// RegisterOnboardingRoutes mounts routes available to an authenticated
// corporate identity that may not yet have an organization.
func (h *Handler) RegisterOnboardingRoutes(group *gin.RouterGroup) {
	group.POST("/organizations", h.createOrganization)
}

// RegisterTenantRoutes mounts routes that require resolved organization scope.
func (h *Handler) RegisterTenantRoutes(group *gin.RouterGroup) {
	group.GET("/organizations/current", h.currentOrganization)
	group.PUT("/organizations/:organizationId", h.updateOrganization)
	group.PUT("/organizations/:organizationId/stages/:stage", h.markStage)
	group.POST("/organizations/:organizationId/representatives", h.createRepresentative)   
	group.GET("/organizations/:organizationId/representatives", h.listRepresentatives)   
	group.PUT("/organizations/:organizationId/representatives/:representativeId", h.updateRepresentative) 
	group.GET("/organizations/:organizationId/capabilities", h.getCapabilities)
	group.PUT("/organizations/:organizationId/capabilities", h.updateCapabilities)
	group.GET("/organizations/:organizationId/profile", h.getProfile)
	group.PUT("/organizations/:organizationId/profile", h.updateProfile)
	group.POST("/organizations/:organizationId/documents/upload-authorization", h.requestUploadAuthorization)
	group.POST("/organizations/:organizationId/documents/upload-content", h.completeDocumentUpload)
	group.GET("/organizations/:organizationId/onboarding-status", h.onboardingStatus)
	group.POST("/organizations/:organizationId/documents", h.attachDocument)
	group.GET("/organizations/:organizationId/documents", h.listDocuments)
	group.POST("/organizations/:organizationId/submit", h.submit)
	group.GET("/organizations/:organizationId/verification", h.verificationStatus)
	group.GET("/organizations/:organizationId/wallet-provisioning", h.walletStatus)
	group.POST("/organizations/:organizationId/wallet-provisioning/retry", h.retryWallet)
	group.POST("/organizations/:organizationId/close-request", h.requestClosure)
	group.GET("/organizations/:organizationId/closure-status", h.closureStatus)
	group.POST("/organizations/:organizationId/closure/assess", h.assessClosure)
	group.POST("/organizations/:organizationId/closure/approve", h.approveClosure)
	group.POST("/organizations/:organizationId/closure/reject", h.rejectClosure)
	group.POST("/organizations/:organizationId/closure/execute", h.executeClosure)

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

type updateOrganizationRequest struct {
    LegalName          string `json:"legalName"`
    TradingName        string `json:"tradingName"`
    RegistrationNumber string `json:"registrationNumber"`
    CountryCode        string `json:"countryCode"`
    OrganizationType   string `json:"organizationType"`
    Timezone           string `json:"timezone"`
    DefaultCurrency    string `json:"defaultCurrency"`
}

type createRepresentativeRequest struct {
	FullName                string  `json:"fullName"`
	RoleTitle               string  `json:"roleTitle"`
	Email                   string  `json:"email"`
	Phone                   string  `json:"phone"`
	AuthorityType           string  `json:"authorityType"`
	AuthorityDocumentFileID *string `json:"authorityDocumentFileId"`
	IdentityReference       *string `json:"identityReference"`
	IsOnboardingOwner       bool    `json:"isOnboardingOwner"`
}

type updateRepresentativeRequest struct {
	FullName                string  `json:"fullName"`
	RoleTitle               string  `json:"roleTitle"`
	Email                   string  `json:"email"`
	Phone                   string  `json:"phone"`
	AuthorityType           string  `json:"authorityType"`
	AuthorityDocumentFileID *string `json:"authorityDocumentFileId"`
	Status                  string  `json:"status"`
	EffectiveTo             *time.Time `json:"effectiveTo"`
}

type requestUploadAuthorizationRequest struct {
	DocumentType     string   `json:"documentType"`
	UploadPurpose    string   `json:"uploadPurpose"`
	AllowedMIMETypes []string `json:"allowedMimeTypes"`
	MaxSizeBytes     int64    `json:"maxSizeBytes"`
}

type completeDocumentUploadRequest struct {
	MediaObjectID    string `json:"mediaObjectId"`
	AuthorizationID  string `json:"authorizationId"`
	UploadToken      string `json:"uploadToken"`
	DeclaredMIME     string `json:"declaredMime"`
	DeclaredSize     int64  `json:"declaredSize"`
	DeclaredChecksum string `json:"declaredChecksum"`
	// File content is sent as multipart form data
}

func (h *Handler) requestUploadAuthorization(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok || !requireScopedPath(c, tenantCtx) {
		return
	}
	if !h.authorize(c, tenantCtx, "organization.update") {
		return
	}

	var request requestUploadAuthorizationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
		return
	}

	idempotencyKey := c.GetHeader("Idempotency-Key")
	if idempotencyKey == "" {
		respond.Error(c, apperrors.BadRequest("Idempotency-Key header is required.", nil))
		return
	}

	grant, err := h.organizations.RequestDocumentUploadAuthorization(c.Request.Context(), orgusecases.RequestDocumentUploadAuthorizationInput{
		Tenant:           tenantCtx,
		DocumentType:     request.DocumentType,
		UploadPurpose:    request.UploadPurpose,
		AllowedMIMETypes: request.AllowedMIMETypes,
		MaxSizeBytes:     request.MaxSizeBytes,
		IdempotencyKey:   idempotencyKey,
	})
	if err != nil {
		respond.Error(c, err)
		return
	}

	respond.OK(c, gin.H{
		"authorizationId":  grant.AuthorizationID,
		"mediaObjectId":    grant.MediaObjectID,
		"uploadToken":      grant.UploadToken,
		"uploadPath":       grant.UploadPath,
		"expiresAt":        grant.ExpiresAt,
		"maxSizeBytes":     grant.MaxSizeBytes,
		"allowedMimeTypes": grant.AllowedMIMETypes,
	})
}

func (h *Handler) completeDocumentUpload(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok || !requireScopedPath(c, tenantCtx) {
		return
	}
	if !h.authorize(c, tenantCtx, "organization.update") {
		return
	}

	// Parse form data
	if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
		respond.Error(c, apperrors.BadRequest("Failed to parse form data.", nil))
		return
	}

	mediaObjectID := c.PostForm("mediaObjectId")
	authorizationID := c.PostForm("authorizationId")
	uploadToken := c.PostForm("uploadToken")
	declaredMIME := c.PostForm("declaredMime")
	declaredSize, _ := strconv.ParseInt(c.PostForm("declaredSize"), 10, 64)
	declaredChecksum := c.PostForm("declaredChecksum")

file, _, err := c.Request.FormFile("file")
	if err != nil {
		respond.Error(c, apperrors.BadRequest("File is required.", nil))
		return
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		respond.Error(c, apperrors.BadRequest("Failed to read file.", nil))
		return
	}

	state, err := h.organizations.CompleteDocumentUpload(c.Request.Context(), orgusecases.CompleteDocumentUploadInput{
		Tenant:           tenantCtx,
		MediaObjectID:    mediaObjectID,
		AuthorizationID:  authorizationID,
		UploadToken:      uploadToken,
		DeclaredMIME:     declaredMIME,
		DeclaredSize:     declaredSize,
		DeclaredChecksum: declaredChecksum,
		Content:          content,
	})
	if err != nil {
		respond.Error(c, err)
		return
	}

	respond.OK(c, gin.H{
		"mediaObjectId":   state.MediaObjectID,
		"uploadStatus":    state.UploadStatus,
		"safetyStatus":    state.SafetyStatus,
		"sizeBytes":       state.SizeBytes,
		"detectedMime":    state.DetectedMIME,
		"sourceVersion":   state.SourceVersion,
	})
}

func (h *Handler) updateOrganization(c *gin.Context) {
    tenantCtx, ok := requireTenant(c)
    if !ok || !requireScopedPath(c, tenantCtx) {
        return
    }
    if !h.authorize(c, tenantCtx, "organization.update") {
        return
    }

    var request updateOrganizationRequest
    if err := c.ShouldBindJSON(&request); err != nil {
        respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
        return
    }

    expectedVersion, err := expectedVersionFrom(c)
    if err != nil {
        respond.Error(c, err)
        return
    }

    organization, err := h.organizations.UpdateOrganization(c.Request.Context(), orgusecases.UpdateOrganizationInput{
        OrganizationID:     tenantCtx.OrganizationID,
        LegalName:          request.LegalName,
        TradingName:        request.TradingName,
        RegistrationNumber: request.RegistrationNumber,
        CountryCode:        request.CountryCode,
        Timezone:           request.Timezone,
        DefaultCurrency:    request.DefaultCurrency,
        ExpectedVersion:    expectedVersion,
        RequestID:          tenantCtx.RequestID,
        CorrelationID:      tenantCtx.CorrelationID,
    })
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

func (h *Handler) createRepresentative(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok || !requireScopedPath(c, tenantCtx) {
		return
	}
	if !h.authorize(c, tenantCtx, "organization.update") {
		return
	}

	var request createRepresentativeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
		return
	}

	rep, err := h.organizations.CreateRepresentative(c.Request.Context(), orgusecases.CreateRepresentativeInput{
		OrganizationID:          tenantCtx.OrganizationID,
		FullName:                request.FullName,
		RoleTitle:               request.RoleTitle,
		Email:                   request.Email,
		Phone:                   request.Phone,
		AuthorityType:           request.AuthorityType,
		AuthorityDocumentFileID: request.AuthorityDocumentFileID,
		IdentityReference:       request.IdentityReference,
		IsOnboardingOwner:       request.IsOnboardingOwner,
		RequestID:               tenantCtx.RequestID,
		CorrelationID:           tenantCtx.CorrelationID,
	})
	if err != nil {
		respond.Error(c, err)
		return
	}

	respond.Created(c, gin.H{
		"representativeId": rep.ID,
		"fullName":         rep.FullName,
		"roleTitle":        rep.RoleTitle,
		"status":           rep.Status,
	})
}

func (h *Handler) listRepresentatives(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok || !requireScopedPath(c, tenantCtx) {
		return
	}
	if !h.authorize(c, tenantCtx, "organization.read") {
		return
	}

	reps, err := h.organizations.ListRepresentatives(c.Request.Context(), tenantCtx.OrganizationID)
	if err != nil {
		respond.Error(c, err)
		return
	}

	respond.List(c, reps, "")
}

func (h *Handler) updateRepresentative(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok || !requireScopedPath(c, tenantCtx) {
		return
	}
	if !h.authorize(c, tenantCtx, "organization.update") {
		return
	}

	repID := c.Param("representativeId")
	if repID == "" {
		respond.Error(c, apperrors.BadRequest("Representative ID is required.", nil))
		return
	}

	var request updateRepresentativeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
		return
	}

	expectedVersion, err := expectedVersionFrom(c)
	if err != nil {
		respond.Error(c, err)
		return
	}

	rep, err := h.organizations.UpdateRepresentative(c.Request.Context(), orgusecases.UpdateRepresentativeInput{
		OrganizationID:          tenantCtx.OrganizationID,
		RepresentativeID:        repID,
		FullName:                request.FullName,
		RoleTitle:               request.RoleTitle,
		Email:                   request.Email,
		Phone:                   request.Phone,
		AuthorityType:           request.AuthorityType,
		AuthorityDocumentFileID: request.AuthorityDocumentFileID,
		Status:                  request.Status,
		EffectiveTo:             request.EffectiveTo,
		ExpectedVersion:         expectedVersion,
		RequestID:               tenantCtx.RequestID,
		CorrelationID:           tenantCtx.CorrelationID,
	})
	if err != nil {
		respond.Error(c, err)
		return
	}

	respond.OK(c, gin.H{
		"representativeId": repID,
		"fullName":         rep.FullName,
		"roleTitle":        rep.RoleTitle,
		"status":           rep.Status,
	})
}

type markStageRequest struct {
	Status string `json:"status"`
}

type updateCapabilitiesRequest struct {
	Capabilities []string `json:"capabilities"`
}

func (h *Handler) getCapabilities(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok || !requireScopedPath(c, tenantCtx) {
		return
	}
	if !h.authorize(c, tenantCtx, "organization.read") {
		return
	}

	capabilities, err := h.organizations.GetCapabilities(c.Request.Context(), tenantCtx.OrganizationID)
	if err != nil {
		respond.Error(c, err)
		return
	}

	respond.OK(c, capabilities)
}

func (h *Handler) updateCapabilities(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok || !requireScopedPath(c, tenantCtx) {
		return
	}
	if !h.authorize(c, tenantCtx, "organization.update") {
		return
	}

	var request updateCapabilitiesRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
		return
	}

	capabilities, err := h.organizations.SetCapabilities(c.Request.Context(), orgusecases.SetCapabilitiesInput{
		OrganizationID: tenantCtx.OrganizationID,
		Capabilities:   request.Capabilities,
		RequestID:      tenantCtx.RequestID,
		CorrelationID:  tenantCtx.CorrelationID,
	})
	if err != nil {
		respond.Error(c, err)
		return
	}

	respond.OK(c, gin.H{
		"capabilities": capabilities,
	})
}

type updateProfileRequest struct {
	BusinessType        *string        `json:"businessType"`
	IncorporationDate   *time.Time     `json:"incorporationDate"`
	TaxReference        *string        `json:"taxReference"`
	RegisteredAddress   map[string]any `json:"registeredAddress"`
	OperatingAddress    map[string]any `json:"operatingAddress"`
	BusinessEmail       *string        `json:"businessEmail"`
	BusinessPhone       *string        `json:"businessPhone"`
	Website             *string        `json:"website"`
	BusinessDescription *string        `json:"businessDescription"`
}

func (h *Handler) getProfile(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok || !requireScopedPath(c, tenantCtx) {
		return
	}
	if !h.authorize(c, tenantCtx, "organization.read") {
		return
	}

	profile, err := h.organizations.GetProfile(c.Request.Context(), tenantCtx.OrganizationID)
	if err != nil {
		respond.Error(c, err)
		return
	}

	// Handle nil profile (no profile exists yet)
	if profile == nil {
		respond.OK(c, gin.H{})
		return
	}

	respond.OK(c, profile)
}

func (h *Handler) updateProfile(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok || !requireScopedPath(c, tenantCtx) {
		return
	}
	if !h.authorize(c, tenantCtx, "organization.update") {
		return
	}

	var request updateProfileRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
		return
	}

	expectedVersion, err := expectedVersionFrom(c)
	if err != nil {
		respond.Error(c, err)
		return
	}

	profile, err := h.organizations.UpdateProfile(c.Request.Context(), orgusecases.UpdateProfileInput{
		OrganizationID:      tenantCtx.OrganizationID,
		BusinessType:        request.BusinessType,
		IncorporationDate:   request.IncorporationDate,
		TaxReference:        request.TaxReference,
		RegisteredAddress:   request.RegisteredAddress,
		OperatingAddress:    request.OperatingAddress,
		BusinessEmail:       request.BusinessEmail,
		BusinessPhone:       request.BusinessPhone,
		Website:             request.Website,
		BusinessDescription: request.BusinessDescription,
		ExpectedVersion:     expectedVersion,
		RequestID:           tenantCtx.RequestID,
		CorrelationID:       tenantCtx.CorrelationID,
	})
	if err != nil {
		respond.Error(c, err)
		return
	}

	respond.OK(c, gin.H{
		"organizationId":      profile.OrganizationID,
		"businessType":        profile.BusinessType,
		"incorporationDate":   profile.IncorporationDate,
		"taxReference":        profile.TaxReference,
		"registeredAddress":   profile.RegisteredAddress,
		"operatingAddress":    profile.OperatingAddress,
		"businessEmail":       profile.BusinessEmail,
		"businessPhone":       profile.BusinessPhone,
		"website":             profile.Website,
		"businessDescription": profile.BusinessDescription,
		"version":             profile.Version,
	})
}


func (h *Handler) markStage(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok || !requireScopedPath(c, tenantCtx) {
		return
	}
	if !h.authorize(c, tenantCtx, "organization.update") {
		return
	}

	stage := c.Param("stage")
	if stage == "" {
		respond.Error(c, apperrors.BadRequest("Stage is required.", nil))
		return
	}

	var request markStageRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
		return
	}

	status := request.Status
	if status == "" {
		status = orgmodels.StageStatusComplete
	}

	// Validate stage exists
	validStage := false
	for _, s := range orgmodels.AllStages {
		if s == stage {
			validStage = true
			break
		}
	}
	if !validStage {
		respond.Error(c, apperrors.BadRequest("Invalid stage name.", nil))
		return
	}

	// Validate status
	validStatus := false
	validStatuses := []string{
		orgmodels.StageStatusNotStarted,
		orgmodels.StageStatusInProgress,
		orgmodels.StageStatusComplete,
		orgmodels.StageStatusBlocked,
		orgmodels.StageStatusChangesRequired,
	}
	for _, s := range validStatuses {
		if s == status {
			validStatus = true
			break
		}
	}
	if !validStatus {
		respond.Error(c, apperrors.BadRequest("Invalid status value.", nil))
		return
	}

	err := h.organizations.MarkStage(c.Request.Context(), tenantCtx.OrganizationID, stage, status, nil, nil)
	if err != nil {
		respond.Error(c, err)
		return
	}

	respond.OK(c, gin.H{
		"stage":  stage,
		"status": status,
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

// assessClosure re-evaluates every closure blocker and records the outcome,
// moving the request to APPROVAL_REQUIRED when clear or back to BLOCKED
// otherwise. Uses orgusecases.DefaultClosureCheckers() -- the fail-closed
// UNKNOWN checker set for obligations whose authoritative service contract
// does not exist yet -- plus a real, DB-backed provider-membership count via
// h.providers, exactly the same checkers a future scheduled re-assessment
// job would use, so a caller-triggered assessment and a scheduled one can
// never disagree about what counts as a blocker.
func (h *Handler) assessClosure(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok || !requireScopedPath(c, tenantCtx) {
		return
	}
	if !h.authorize(c, tenantCtx, "organization.closure.manage") {
		return
	}
	closure, blockers, err := h.organizations.AssessClosure(c.Request.Context(), tenantCtx.OrganizationID,
		orgusecases.DefaultClosureCheckers(), h.providers)
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, gin.H{"status": closure.Status, "blockers": blockers})
}

// approveClosure moves an unblocked, fully-assessed request from
// APPROVAL_REQUIRED to APPROVED -- the step ExecuteClosure requires before
// it will do anything.
func (h *Handler) approveClosure(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok || !requireScopedPath(c, tenantCtx) {
		return
	}
	if !h.authorize(c, tenantCtx, "organization.closure.manage") {
		return
	}
	closure, err := h.organizations.ApproveClosure(c.Request.Context(), tenantCtx, tenantCtx.ActorID)
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, gin.H{"status": closure.Status})
}

type rejectClosureRequest struct {
	ReasonText string `json:"reasonText"`
}

// rejectClosure ends an in-flight closure request without executing it.
func (h *Handler) rejectClosure(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok || !requireScopedPath(c, tenantCtx) {
		return
	}
	if !h.authorize(c, tenantCtx, "organization.closure.manage") {
		return
	}
	var request rejectClosureRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&request); err != nil {
			respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
			return
		}
	}
	closure, err := h.organizations.RejectClosure(c.Request.Context(), tenantCtx, request.ReasonText)
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, gin.H{"status": closure.Status})
}

// executeClosure performs the non-destructive closure steps (revoke
// corporate sessions, mark the organization CLOSED) once a request is
// APPROVED. Nothing is hard-deleted; immutable financial, trip, safety,
// compliance and audit records are always retained.
func (h *Handler) executeClosure(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok || !requireScopedPath(c, tenantCtx) {
		return
	}
	if !h.authorize(c, tenantCtx, "organization.closure.manage") {
		return
	}
	if err := h.organizations.ExecuteClosure(c.Request.Context(), tenantCtx.OrganizationID,
		h.sessions, tenantCtx.CorrelationID); err != nil {
		respond.Error(c, err)
		return
	}
	// ExecuteClosure only ever returns nil after its whole transaction --
	// including the final UpdateClosureStatus(...COMPLETED...) -- commits, so
	// COMPLETED is the only status a successful call can have produced. It
	// cannot be re-read via GetClosureStatus: that query's "open request"
	// filter deliberately excludes COMPLETED (see GetOpenClosureRequest),
	// so calling it here would 404 immediately after a successful execution.
	respond.OK(c, gin.H{"status": orgmodels.ClosureStatusCompleted})
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
