package usecases

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Store lists the repositories.Repository methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type Store interface {
	ActivePolicy(ctx context.Context, q database.Querier, organizationID string, policyKey string) (*models.Policy, error)
	AppendHistory(ctx context.Context, q database.Querier, entry *models.HistoryEntry) error
	AssetHasLiveAssignment(ctx context.Context, q database.Querier, organizationID string, assetReference string, excludeAssignmentID string) (bool, error)
	BranchState(ctx context.Context, q database.Querier, organizationID string, branchID string) (repositories.BranchState, error)
	CapabilityApproved(ctx context.Context, q database.Querier, organizationID string, capability string) (bool, error)
	CompanyState(ctx context.Context, q database.Querier, organizationID string) (repositories.CompanyState, error)
	Current(ctx context.Context, q database.Querier, organizationID string, subjectType string, subjectID string, operationType string) (*models.Decision, error)
	DocumentStates(ctx context.Context, q database.Querier, organizationID string, requiredTypes []string) ([]repositories.DocumentState, error)
	GetByID(ctx context.Context, organizationID string, decisionID string) (*models.Decision, error)
	List(ctx context.Context, query tenant.Query, filter repositories.ListFilter) ([]models.Decision, error)
	ListHistory(ctx context.Context, organizationID string, decisionID string) ([]models.HistoryEntry, error)
	ListPolicies(ctx context.Context, organizationID string, policyKey string) ([]models.Policy, error)
	MembershipHasLiveAssignment(ctx context.Context, q database.Querier, organizationID string, membershipID string, excludeAssignmentID string) (bool, error)
	MembershipState(ctx context.Context, q database.Querier, organizationID string, membershipID string) (repositories.MembershipState, error)
	OnboardingComplete(ctx context.Context, q database.Querier, organizationID string) (bool, error)
	PublishPolicy(ctx context.Context, q database.Querier, organizationID string, policy *models.Policy) error
	Record(ctx context.Context, q database.Querier, organizationID string, decision *models.Decision) error
	ReferenceDocumentStates(ctx context.Context, q database.Querier, organizationID string, ownerType string, ownerID string, requiredTypes []string) ([]repositories.ReferenceDocumentState, error)
	RestrictionState(ctx context.Context, q database.Querier, organizationID string) (repositories.RestrictionState, error)
	SubscriptionState(ctx context.Context, q database.Querier, organizationID string) (repositories.SubscriptionState, error)
	VerificationState(ctx context.Context, q database.Querier, organizationID string) (repositories.VerificationState, error)
}
