package http

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/usecases"
)

// UseCases lists the usecases.Service methods this package calls. The concrete
// type satisfies it implicitly and is wired in internal/app.
type UseCases interface {
	ChangePhone(ctx context.Context, identityID string, sessionID string, organizationID string, phone string, reference string) error
	CurrentProfile(ctx context.Context, id string, sessionID string, requestID string) (*usecases.IdentityProfile, error)
	IdentityByID(ctx context.Context, identityID string) (*models.Identity, error)
	ListSessions(ctx context.Context, identityID string) ([]models.Session, error)
	LogoutRefresh(ctx context.Context, token string, requestID string, correlationID string) error
	Refresh(ctx context.Context, input usecases.RefreshInput) (*usecases.TokenPair, error)
	Resend(ctx context.Context, input usecases.ResendInput) (usecases.StartResult, error)
	RevokeSession(ctx context.Context, identityID string, sessionID string, requestID string, correlationID string) error
	SelectOrganization(ctx context.Context, input usecases.SelectOrganizationInput) (*usecases.TokenPair, error)
	SignIn(ctx context.Context, input usecases.StartInput) (usecases.StartResult, error)
	Start(ctx context.Context, input usecases.StartInput) (usecases.StartResult, error)
	Verify(ctx context.Context, input usecases.VerifyInput) (usecases.VerifyResult, error)
}
