// Package stepup defines the narrow port high-risk-action usecases call to
// verify a caller-supplied step-up reference against the auth feature's own
// authoritative grants (internal/features/auth/usecases.Service.
// VerifyStepUpGrant). It exists so a consuming feature (legalhold,
// securitysettings, …) never imports the auth feature's usecases package
// directly — the real adapter lives in internal/app (the composition root),
// same as every other cross-feature dependency in this service
// (documents.MediaPort, orders.CRMPort, …).
package stepup

import "context"

// Verifier checks that reference is a valid, unexpired step-up grant for
// exactly this identity, session, organization and action class. A grant
// cannot be transferred to another session or reused for a different action
// category — see the auth feature's own VerifyStepUpGrant doc comment for
// why.
type Verifier interface {
	Verify(ctx context.Context, reference, identityID, sessionID, organizationID, actionClass string) error
}
