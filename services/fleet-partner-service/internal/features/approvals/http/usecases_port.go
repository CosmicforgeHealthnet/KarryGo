package http

import (
	"context"
	"time"

	pgx "github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/approvals/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/approvals/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/approvals/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// UseCases lists the usecases.Service methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type UseCases interface {
	ArchivePolicy(ctx context.Context, input usecases.PolicyInput) (*models.Policy, error)
	Cancel(ctx context.Context, input usecases.CancelInput) (*models.Request, error)
	CreateDelegation(ctx context.Context, input usecases.DelegationInput) (*usecases.DelegationResult, error)
	CreatePolicy(ctx context.Context, input usecases.PolicyInput) (*usecases.PolicyResult, error)
	Decide(ctx context.Context, input usecases.DecideInput) (*models.Request, error)
	Decisions(ctx context.Context, organizationID string, requestID string) ([]models.Decision, error)
	Execute(ctx context.Context, input usecases.ExecuteInput, run func(tx pgx.Tx, request *models.Request) error) (*usecases.ExecuteResult, error)
	Get(ctx context.Context, organizationID string, requestID string) (*models.Request, error)
	GetDelegation(ctx context.Context, organizationID string, permitted []string, delegationID string) (*models.Delegation, error)
	GetPolicy(ctx context.Context, organizationID string, permitted []string, policyID string) (*models.Policy, error)
	History(ctx context.Context, organizationID string, requestID string) ([]repositories.HistoryEntry, error)
	List(ctx context.Context, query tenant.Query, filter repositories.ListFilter) ([]models.Request, error)
	ListDelegations(ctx context.Context, organizationID string, permitted []string, filter repositories.DelegationFilter) ([]models.Delegation, error)
	ListPolicies(ctx context.Context, organizationID string, permitted []string, filter repositories.PolicyFilter) ([]models.Policy, error)
	Now() time.Time
	PolicyVersions(ctx context.Context, organizationID string, permitted []string, policyID string) ([]models.PolicyVersion, error)
	Request(ctx context.Context, input usecases.RequestInput) (*usecases.RequestResult, error)
	RevokeDelegation(ctx context.Context, input usecases.RevokeDelegationInput) (*models.Delegation, error)
	Steps(ctx context.Context, organizationID string, requestID string) ([]models.Step, error)
	UpdatePolicy(ctx context.Context, input usecases.PolicyInput) (*models.Policy, error)
}
