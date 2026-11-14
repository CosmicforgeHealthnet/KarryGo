package http

import (
	"context"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/usecases"
	providermodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/models"
	providerusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/usecases"
	staffmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/models"
	staffusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// OrganizationUseCases lists the usecases.Service methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type OrganizationUseCases interface {
	ApplyVerificationOutcome(ctx context.Context, input usecases.VerificationOutcomeInput) (*usecases.VerificationOutcomeResult, error)
	ApproveClosure(ctx context.Context, t tenant.Context, approvedByActorID string) (*models.ClosureRequest, error)
	ArchiveBranch(ctx context.Context, tenantCtx tenant.Context, branchID string, expectedVersion int64) error
	ArchiveCostCentre(ctx context.Context, tenantCtx tenant.Context, id string, expectedVersion int64) error
	ArchiveDepartment(ctx context.Context, tenantCtx tenant.Context, departmentID string, expectedVersion int64) error
	ArchiveLocation(ctx context.Context, tenantCtx tenant.Context, id string, expectedVersion int64) error
	ArchiveTeam(ctx context.Context, tenantCtx tenant.Context, id string, expectedVersion int64) error
	AssessClosure(ctx context.Context, organizationID string, checkers []usecases.ClosureBlockerChecker, providers usecases.ProviderMembershipCounter) (*models.ClosureRequest, []models.ClosureBlocker, error)
	AttachDocument(ctx context.Context, input usecases.AttachDocumentInput) (*models.Document, error)
	CompleteDocumentUpload(ctx context.Context, input usecases.CompleteDocumentUploadInput) (*platformclients.MediaObjectState, error)
	CreateBranch(ctx context.Context, input usecases.BranchInput) (*models.Branch, error)
	CreateCostCentre(ctx context.Context, input usecases.CostCentreInput) (*models.CostCentre, error)
	CreateDepartment(ctx context.Context, input usecases.DepartmentInput) (*models.Department, error)
	CreateDraft(ctx context.Context, input usecases.CreateDraftInput) (*models.Organization, error)
	CreateLocation(ctx context.Context, input usecases.LocationInput) (*models.OperatingLocation, error)
	CreateRepresentative(ctx context.Context, input usecases.CreateRepresentativeInput) (*models.Representative, error)
	CreateTeam(ctx context.Context, input usecases.TeamInput) (*models.Team, error)
	EvaluateActivation(ctx context.Context, organizationID string) (*usecases.ActivationAssessment, error)
	ExecuteClosure(ctx context.Context, organizationID string, sessions usecases.SessionRevoker, correlationID string) error
	GetBranch(ctx context.Context, organizationID string, branchID string) (*models.Branch, error)
	GetCapabilities(ctx context.Context, organizationID string) (map[string]string, error)
	GetClosureStatus(ctx context.Context, organizationID string) (*models.ClosureRequest, []models.ClosureBlocker, error)
	GetCostCentre(ctx context.Context, organizationID string, id string) (*models.CostCentre, error)
	GetDepartment(ctx context.Context, organizationID string, departmentID string) (*models.Department, error)
	GetLocation(ctx context.Context, organizationID string, id string) (*models.OperatingLocation, error)
	GetOnboardingStatus(ctx context.Context, organizationID string) (*usecases.OnboardingStatus, error)
	GetProfile(ctx context.Context, organizationID string) (*models.Profile, error)
	GetSettings(ctx context.Context, organizationID string) (*models.Settings, error)
	GetTeam(ctx context.Context, organizationID string, id string) (*models.Team, error)
	ListBranches(ctx context.Context, query tenant.Query) ([]models.Branch, error)
	ListCostCentres(ctx context.Context, organizationID string, branchID string, includeArchived bool) ([]models.CostCentre, error)
	ListDepartments(ctx context.Context, organizationID string, branchID string, includeArchived bool) ([]models.Department, error)
	ListLocations(ctx context.Context, organizationID string, filter repositories.LocationFilter) ([]models.OperatingLocation, error)
	ListRepresentatives(ctx context.Context, organizationID string) ([]models.Representative, error)
	ListTeams(ctx context.Context, organizationID string, filter repositories.TeamFilter) ([]models.Team, error)
	MarkStage(ctx context.Context, organizationID string, stage string, status string, missing []string, blocking []string) error
	PreviewDocument(ctx context.Context, t tenant.Context, documentID string) (*usecases.DocumentPreview, error)
	RejectClosure(ctx context.Context, t tenant.Context, reasonText string) (*models.ClosureRequest, error)
	RemoveDocument(ctx context.Context, t tenant.Context, id string) error
	GetOrganization(ctx context.Context, organizationID string) (*models.Organization, error)
	ListDocuments(ctx context.Context, organizationID string) ([]models.Document, error)
	LatestVerificationRequest(ctx context.Context, organizationID string) (*models.VerificationRequest, error)
	GetWalletProvisioning(ctx context.Context, organizationID string) (*models.WalletProvisioning, error)
	ListVerificationIssues(ctx context.Context, organizationID string, requestID string) ([]models.VerificationIssue, error)
	RequestClosure(ctx context.Context, input usecases.RequestClosureInput) (*models.ClosureRequest, error)
	RequestDocumentUploadAuthorization(ctx context.Context, input usecases.RequestDocumentUploadAuthorizationInput) (*platformclients.MediaUploadGrant, error)
	RequestWalletProvisioning(ctx context.Context, tenantCtx tenant.Context) (*models.WalletProvisioning, error)
	SetCapabilities(ctx context.Context, input usecases.SetCapabilitiesInput) ([]string, error)
	Submit(ctx context.Context, input usecases.SubmitInput) (*models.VerificationRequest, error)
	UpdateBranch(ctx context.Context, input usecases.BranchInput) (*models.Branch, error)
	UpdateCostCentre(ctx context.Context, input usecases.CostCentreInput) (*models.CostCentre, error)
	UpdateDepartment(ctx context.Context, input usecases.DepartmentInput) (*models.Department, error)
	UpdateLocation(ctx context.Context, input usecases.LocationInput) (*models.OperatingLocation, error)
	UpdateOrganization(ctx context.Context, input usecases.UpdateOrganizationInput) (*models.Organization, error)
	UpdateProfile(ctx context.Context, input usecases.UpdateProfileInput) (*models.Profile, error)
	UpdateRepresentative(ctx context.Context, input usecases.UpdateRepresentativeInput) (*models.Representative, error)
	UpdateSettings(ctx context.Context, patch usecases.SettingsPatch) (*models.Settings, error)
	UpdateTeam(ctx context.Context, input usecases.TeamInput) (*models.Team, error)
}

// ProviderUseCases lists the usecases.Service methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type ProviderUseCases interface {
	usecases.ProviderMembershipCounter
	Availability(ctx context.Context, tenantCtx tenant.Context, membershipID string) (*providerusecases.ProviderAvailability, error)
	Capabilities(ctx context.Context, organizationID string, membershipID string) ([]providermodels.Capability, error)
	ChangeStatus(ctx context.Context, input providerusecases.ChangeStatusInput) error
	CreateProvider(ctx context.Context, input providerusecases.CreateProviderInput) (*providerusecases.CreateProviderResult, error)
	Declines(ctx context.Context, organizationID string, membershipID string, limit int) ([]providerusecases.Decline, error)
	HistoryPageOf(ctx context.Context, tenantCtx tenant.Context, membershipID string, limit int, afterOccurredAt *time.Time, afterID string) (*providerusecases.HistoryPage, error)
	ListMemberships(ctx context.Context, query tenant.Query) ([]providermodels.Membership, error)
	Policy(ctx context.Context, organizationID string, membershipID string) (*providermodels.Policy, error)
	ProviderBranches(ctx context.Context, tenantCtx tenant.Context, membershipID string) ([]providerusecases.ProviderBranch, error)
	ProviderServiceRoles(ctx context.Context, tenantCtx tenant.Context, membershipID string) (*providerusecases.ProviderServiceRoles, error)
	ProviderVehicles(ctx context.Context, tenantCtx tenant.Context, membershipID string, includeHistory bool) ([]providerusecases.ProviderVehicle, error)
	ProvisioningStatus(ctx context.Context, tenantCtx tenant.Context, membershipID string) (*providermodels.ProvisioningSaga, error)
	Reactivate(ctx context.Context, tenantCtx tenant.Context, membershipID string, expectedVersion int64) error
	Remove(ctx context.Context, tenantCtx tenant.Context, membershipID string, reasonCode string, expectedVersion int64) error
	ReplaceCapabilities(ctx context.Context, tenantCtx tenant.Context, membershipID string, capabilities []providermodels.Capability) ([]providermodels.Capability, error)
	ReplacePolicy(ctx context.Context, tenantCtx tenant.Context, policy providermodels.Policy, declineRequiredSet bool) (*providermodels.Policy, error)
	ResendLogin(ctx context.Context, input providerusecases.ResendLoginInput) (*providerusecases.ResendLoginResult, error)
	UpdateMembership(ctx context.Context, input providerusecases.UpdateMembershipInput) (*providermodels.Membership, error)
	VisibleMembership(ctx context.Context, tenantCtx tenant.Context, membershipID string) (*providermodels.Membership, error)
}

// StaffUseCases lists the usecases.Service methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type StaffUseCases interface {
	ArchiveRole(ctx context.Context, input staffusecases.RoleInput) error
	CancelInvitation(ctx context.Context, tenantCtx tenant.Context, invitationID string) error
	ChangeStaffStatus(ctx context.Context, input staffusecases.ChangeStatusInput) error
	CreateRole(ctx context.Context, guard staffusecases.PermissionGuard, input staffusecases.RoleInput) (*staffmodels.Role, error)
	GetStaff(ctx context.Context, organizationID string, memberID string) (*staffmodels.OrganizationUser, error)
	Invite(ctx context.Context, input staffusecases.InviteInput) (*staffmodels.StaffInvitation, error)
	ListInvitations(ctx context.Context, tenantCtx tenant.Context, limit int) ([]staffmodels.StaffInvitation, error)
	ListRoleBindings(ctx context.Context, organizationID string, memberID string) ([]staffmodels.RoleBinding, error)
	ReplaceRoleBindings(ctx context.Context, guard staffusecases.PermissionGuard, input staffusecases.ReplaceRoleBindingsInput) error
	ResendInvitation(ctx context.Context, tenantCtx tenant.Context, invitationID string) (*staffmodels.StaffInvitation, error)
	ListPermissions(ctx context.Context) ([]staffmodels.Permission, error)
	ListRoles(ctx context.Context, organizationID string) ([]staffmodels.Role, error)
	GetRole(ctx context.Context, organizationID string, roleID string) (*staffmodels.Role, error)
	StaffDirectory(ctx context.Context, organizationID string, page staffusecases.StaffDirectoryPage) (*staffusecases.StaffDirectoryResult, error)
	StaffMember(ctx context.Context, organizationID string, memberID string) (*staffmodels.StaffDirectoryEntry, error)
	UpdateRole(ctx context.Context, guard staffusecases.PermissionGuard, input staffusecases.RoleInput) (*staffmodels.Role, error)
	UpdateStaff(ctx context.Context, input staffusecases.UpdateStaffInput) error
}
