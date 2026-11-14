package usecases

import (
	"context"
	"errors"

	"cosmicforge/logistics/shared/go/apperrors"

	providerrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/repositories"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Read use cases for the organization provider routes, so HTTP handlers
// depend on the use-case layer rather than on the repository (spec 5.1).

// ListMemberships pages the organization's provider memberships within the
// query's branch scope.
func (s *Service) ListMemberships(ctx context.Context, query tenant.Query) ([]models.Membership, error) {
	return s.opts.Providers.ListMemberships(ctx, query)
}

// ProvisioningStatus returns the provisioning saga of a visible membership.
func (s *Service) ProvisioningStatus(ctx context.Context, tenantCtx tenant.Context, membershipID string) (*models.ProvisioningSaga, error) {
	if _, err := s.VisibleMembership(ctx, tenantCtx, membershipID); err != nil {
		return nil, err
	}
	saga, err := s.opts.Providers.GetSaga(ctx, tenantCtx.OrganizationID, membershipID)
	if errors.Is(err, providerrepos.ErrNotFound) {
		return nil, apperrors.NotFound("Provisioning status not found.", nil)
	}
	return saga, err
}

// History returns the append-only membership history of a visible membership.
func (s *Service) History(ctx context.Context, tenantCtx tenant.Context, membershipID string) ([]models.HistoryEntry, error) {
	if _, err := s.VisibleMembership(ctx, tenantCtx, membershipID); err != nil {
		return nil, err
	}
	return s.opts.Providers.ListHistory(ctx, tenantCtx.OrganizationID, membershipID)
}
