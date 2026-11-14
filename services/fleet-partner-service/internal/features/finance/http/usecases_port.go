package http

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/finance/models"
)

// OperatorService is the slice of the finance use cases the operator
// handler calls.
type OperatorService interface {
	ListOperations(ctx context.Context, scope models.Scope, filter models.OperationFilter) ([]models.OperationSummary, error)
	GetOperation(ctx context.Context, scope models.Scope, id string) (models.OperationSummary, error)
	OperationHistory(ctx context.Context, scope models.Scope, id string) ([]models.HistoryEntry, error)
	OperationAudit(ctx context.Context, scope models.Scope, id string) ([]models.AuditEntry, error)
	ReplayOperation(ctx context.Context, organizationID, operationID, actorID, correlationID string) error
	ListProjections(ctx context.Context, scope models.Scope, filter models.ProjectionFilter) ([]models.ProjectionSummary, error)
	GetProjection(ctx context.Context, scope models.Scope, id string) (models.ProjectionSummary, error)
	ReplayProjection(ctx context.Context, organizationID, projectionID, actorID, correlationID string) error
	RequestReadBack(ctx context.Context, scope models.Scope, id, actorType, actorID, correlationID string) error
	RequestSettlement(ctx context.Context, in models.SettlementRequest) (*models.Operation, error)
	RequestRefund(ctx context.Context, in models.RefundRequest) (*models.Operation, error)
	RequestReversal(ctx context.Context, in models.ReversalRequest) (*models.Operation, error)
}
