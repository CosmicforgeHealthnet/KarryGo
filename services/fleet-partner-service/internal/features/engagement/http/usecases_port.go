package http

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Service is the slice of the engagement use cases the tenant handler calls.
type Service interface {
	ListSupportCases(ctx context.Context, t tenant.Context) ([]map[string]any, error)
	CreateSupportCase(ctx context.Context, t tenant.Context, key string, in models.SupportCreateInput) (map[string]any, error)
	GetSupportCase(ctx context.Context, t tenant.Context, caseID string) (map[string]any, error)
	ListQualityCases(ctx context.Context, t tenant.Context) ([]map[string]any, error)
	CreateQualityCase(ctx context.Context, t tenant.Context, in models.QualityInput) (map[string]any, error)
	GetQualityCase(ctx context.Context, t tenant.Context, id string) (map[string]any, error)
	UpdateQualityCase(ctx context.Context, t tenant.Context, id string, in models.QualityInput, resolve bool) (map[string]any, error)
	ListMeasurements(ctx context.Context, t tenant.Context) ([]map[string]any, error)
	GetMeasurement(ctx context.Context, t tenant.Context, id string) (map[string]any, error)
	ListEscalations(ctx context.Context, t tenant.Context) ([]map[string]any, error)
	GetEscalation(ctx context.Context, t tenant.Context, id string) (map[string]any, error)
	EscalationCommand(ctx context.Context, t tenant.Context, id, command string, expected int64) (map[string]any, error)
}

// IngressService is the slice of the engagement use cases the internal
// handler calls.
type IngressService interface {
	ApplySupportEvent(ctx context.Context, in models.SupportEvent) error
	RecordMeasurement(ctx context.Context, in models.MeasurementInput) (map[string]any, error)
}
