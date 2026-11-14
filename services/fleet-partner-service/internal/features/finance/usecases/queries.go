package usecases

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/finance/models"
)

// ListOperations returns the tenant's durable Wallet operations.
func (s *Service) ListOperations(ctx context.Context, scope models.Scope,
	filter models.OperationFilter) ([]models.OperationSummary, error) {
	if strings.TrimSpace(scope.OrganizationID) == "" {
		return nil, models.ErrNotFound
	}
	return s.repo.ListOperations(ctx, scope, filter)
}

// GetOperation reads one operation. Ownership is part of the query, so an
// identifier belonging to another tenant is indistinguishable from one that
// does not exist.
func (s *Service) GetOperation(ctx context.Context, scope models.Scope, id string) (models.OperationSummary, error) {
	if strings.TrimSpace(scope.OrganizationID) == "" || strings.TrimSpace(id) == "" {
		return models.OperationSummary{}, models.ErrNotFound
	}
	return s.repo.GetOperation(ctx, scope, id)
}

// OperationHistory returns the append-only transition history for one
// operation the caller is entitled to see.
func (s *Service) OperationHistory(ctx context.Context, scope models.Scope, id string) ([]models.HistoryEntry, error) {
	if _, err := s.GetOperation(ctx, scope, id); err != nil {
		return nil, err
	}
	return s.repo.OperationHistory(ctx, scope.OrganizationID, id)
}

// OperationAudit returns the audit trail recorded against one operation.
func (s *Service) OperationAudit(ctx context.Context, scope models.Scope, id string) ([]models.AuditEntry, error) {
	if _, err := s.GetOperation(ctx, scope, id); err != nil {
		return nil, err
	}
	return s.repo.OperationAudit(ctx, scope.OrganizationID, id)
}

// ListProjections returns the tenant's finance projections. With
// ReconciliationOnly set it returns exactly the open reconciliation cases.
func (s *Service) ListProjections(ctx context.Context, scope models.Scope,
	filter models.ProjectionFilter) ([]models.ProjectionSummary, error) {
	if strings.TrimSpace(scope.OrganizationID) == "" {
		return nil, models.ErrNotFound
	}
	return s.repo.ListProjections(ctx, scope, filter)
}

// GetProjection reads one projection inside the caller's scope.
func (s *Service) GetProjection(ctx context.Context, scope models.Scope, id string) (models.ProjectionSummary, error) {
	if strings.TrimSpace(scope.OrganizationID) == "" || strings.TrimSpace(id) == "" {
		return models.ProjectionSummary{}, models.ErrNotFound
	}
	return s.repo.GetProjection(ctx, scope, id)
}

// RequestReadBack schedules an authoritative read-back of one projection.
//
// It never writes the projection's value: it only asks the reconciler to go
// and ask the authoritative service. Until that returns, the projection is not
// CURRENT, and so it authorizes nothing.
func (s *Service) RequestReadBack(ctx context.Context, scope models.Scope, id, actorType, actorID,
	correlationID string) error {
	projection, err := s.GetProjection(ctx, scope, id)
	if err != nil {
		return err
	}
	return s.withAudit(ctx, scope.OrganizationID, actorType, actorID,
		"finance.projection.read_back_requested", "finance_projection", id, correlationID,
		map[string]any{"projectionType": projection.ProjectionType,
			"previousFreshness": projection.FreshnessState},
		func(tx pgx.Tx) error {
			scheduled, execErr := s.repo.RequestReadBack(ctx, tx, scope.OrganizationID, id)
			if execErr != nil {
				return execErr
			}
			if !scheduled {
				// Already in flight: asking again would reset an in-progress
				// reconciliation and lose its attempt history.
				return models.ErrReadBackNotAvailable
			}
			return nil
		})
}
