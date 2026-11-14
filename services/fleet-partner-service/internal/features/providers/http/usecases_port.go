package http

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/usecases"
)

// UseCases lists the usecases.Service methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type UseCases interface {
	GetMembershipSummaryForProvider(ctx context.Context, providerService string, providerID string) (usecases.MembershipSummary, error)
	LiveMembershipForProvider(ctx context.Context, providerService string, providerID string) (*models.Membership, error)
}
