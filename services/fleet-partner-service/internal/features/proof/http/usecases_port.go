package http

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/proof/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// UseCases is the slice of the proof use cases the handler calls.
type UseCases interface {
	ListPolicies(ctx context.Context, t tenant.Context) ([]models.Policy, error)
	GetPolicy(ctx context.Context, t tenant.Context, id string) (models.Policy, error)
	SavePolicy(ctx context.Context, t tenant.Context, id string, expected int64, v models.Policy) (models.Policy, models.PolicySnapshot, error)
	CreateRecord(ctx context.Context, t tenant.Context, in models.RecordInput) (models.Record, error)
	ListRecords(ctx context.Context, t tenant.Context) ([]models.Record, error)
	GetRecord(ctx context.Context, t tenant.Context, id string) (models.RecordDetail, error)
	Decide(ctx context.Context, t tenant.Context, id, decision string, in models.DecisionInput) (models.RecordDetail, error)
}
