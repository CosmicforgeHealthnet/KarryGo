package http

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// UseCases lists the usecases.Service methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type UseCases interface {
	Evaluate(ctx context.Context, input usecases.EvaluateInput) (*models.Decision, error)
	Get(ctx context.Context, organizationID string, decisionID string) (*models.Decision, error)
	History(ctx context.Context, organizationID string, decisionID string) ([]models.HistoryEntry, error)
	List(ctx context.Context, query tenant.Query, filter repositories.ListFilter) ([]models.Decision, error)
	ListPolicies(ctx context.Context, organizationID string, policyKey string) ([]models.Policy, error)
	PublishPolicy(ctx context.Context, actor usecases.Actor, policy *models.Policy) (*models.Policy, error)
	Require(ctx context.Context, input usecases.EvaluateInput) (*models.Decision, error)
}
