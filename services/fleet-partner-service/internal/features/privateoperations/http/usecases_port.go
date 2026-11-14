package http

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// UseCases lists the usecases.Service methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type UseCases interface {
	Cancel(ctx context.Context, t tenant.Context, id string, version int64) (models.Assignment, error)
	Create(ctx context.Context, t tenant.Context, in usecases.CreateInput) (models.Assignment, error)
	Get(ctx context.Context, t tenant.Context, id string) (models.Assignment, error)
	GetExecution(ctx context.Context, t tenant.Context, id string) (models.ExecutionOperation, error)
	GetProviderAssignment(ctx context.Context, id string, providerID string, executionService string) (models.Assignment, error)
	List(ctx context.Context, t tenant.Context, status string) ([]models.Assignment, error)
	Reassign(ctx context.Context, t tenant.Context, id string, in usecases.ReassignInput) (models.Assignment, error)
	ReplayExecution(ctx context.Context, t tenant.Context, id string) error
	ReplayReconciliation(ctx context.Context, t tenant.Context, id string) error
	Respond(ctx context.Context, org string, id string, in usecases.RespondInput) (models.Assignment, error)
}
