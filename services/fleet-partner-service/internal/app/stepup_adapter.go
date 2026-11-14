package app

import (
	"context"
	"errors"

	authrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/repositories"
	authusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/stepup"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// stepUpAdapter lets a feature consume step-up verification
// (internal/platform/stepup.Verifier) without importing the auth feature's
// usecases package directly. This adapter lives at the composition root
// because it is the only place allowed to know both.
type stepUpAdapter struct {
	auth *authusecases.Service
}

// Verify implements stepup.Verifier.
func (a stepUpAdapter) Verify(ctx context.Context, reference, identityID, sessionID, organizationID, actionClass string) error {
	return a.auth.VerifyStepUpGrant(ctx, reference, identityID, sessionID, organizationID, actionClass)
}

// StepUpVerifier is the verifier the router's step-up middleware
// (app.Options.StepUp) checks protected routes against. Nil when the auth
// service is not built, which makes protected routes fail closed.
func (c *Components) StepUpVerifier() stepup.Verifier {
	if c == nil || c.Auth == nil {
		return nil
	}
	return stepUpAdapter{auth: c.Auth}
}

// StepUpOrganizationScope returns the organization a step-up challenge started
// on this session binds to: the same organization the tenant middleware
// (middleware.ResolveTenant) resolves for the session's protected requests.
//
// POST /auth/step-up/start is an identity route, so no tenant context exists
// when it runs. Without this the grant bound to an empty organization and no
// organization-scoped check could ever accept it. An identity with no
// resolvable membership gets "" (the grant then only serves identity-level
// actions such as a phone change).
func StepUpOrganizationScope(sessions *authrepos.SessionRepository, resolver tenant.MembershipResolver) func(ctx context.Context, identityID, sessionID string) (string, error) {
	return func(ctx context.Context, identityID, sessionID string) (string, error) {
		if sessions != nil {
			organizationID, err := sessions.OrganizationForSession(ctx, sessionID, identityID)
			if err != nil {
				return "", err
			}
			if organizationID != "" {
				return organizationID, nil
			}
		}
		if resolver == nil {
			return "", nil
		}
		membership, err := resolver.ResolveMembership(ctx, identityID)
		if errors.Is(err, tenant.ErrMembershipNotFound) || errors.Is(err, tenant.ErrMembershipSuspended) || errors.Is(err, tenant.ErrResolverUnconfigured) {
			return "", nil
		}
		if err != nil {
			return "", err
		}
		return membership.OrganizationID, nil
	}
}
