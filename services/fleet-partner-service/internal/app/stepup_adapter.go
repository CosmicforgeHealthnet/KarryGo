package app

import (
	"context"

	authusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/usecases"
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
