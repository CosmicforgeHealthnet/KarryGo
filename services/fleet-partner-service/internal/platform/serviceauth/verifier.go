package serviceauth

import (
	"context"
	"net/http"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"
	sharedserviceauth "cosmicforge/logistics/shared/go/serviceauth"
)

// ScopeVerifier resolves an inbound internal request to a specific,
// version-identified Credential (trying every one of that caller's active
// candidate secrets against the request's HMAC signature, exactly as
// shared/go/serviceauth already verifies a single one) and then authorizes
// that credential against the request's method/route/event scope.
type ScopeVerifier struct {
	store   *Store
	secrets VersionedSecrets
	maxSkew time.Duration
	now     func() time.Time
}

// NewScopeVerifier builds the verifier. maxSkew must match the freshness
// window used everywhere else HMAC signatures are checked in this service
// (5 minutes at this service's one construction site) -- a mismatch would
// make one candidate credential accept a timestamp the others reject for no
// scope-related reason.
func NewScopeVerifier(store *Store, secrets VersionedSecrets, maxSkew time.Duration) *ScopeVerifier {
	if maxSkew <= 0 {
		maxSkew = 5 * time.Minute
	}
	return &ScopeVerifier{store: store, secrets: secrets, maxSkew: maxSkew, now: time.Now}
}

// ErrNoActiveCredential means the caller has no credential row whose
// lifecycle window is currently open (distinct from a signature mismatch --
// this can mean "not yet active", "expired" or "revoked", none of which
// this error distinguishes to the caller, deliberately: an external
// response must not reveal which).
var ErrNoActiveCredential = apperrors.Forbidden("Service is not allowed to call this endpoint.", nil)

// Resolve verifies req's HMAC signature against every ACTIVE (see
// Credential.IsActive) candidate credential configured for serviceName,
// newest version first, returning the first one whose secret validates the
// signature. Does not check method/route/event scope -- call Authorize with
// the result for that.
//
// Trying multiple candidates is what makes rotation overlap possible: two
// credential versions can both be genuinely active (old one not yet
// expired, new one already active) and either one's secret must be
// accepted during that window.
func (v *ScopeVerifier) Resolve(ctx context.Context, serviceName string, req *http.Request) (Credential, error) {
	candidates, err := v.store.ActiveCandidates(ctx, serviceName)
	if err != nil {
		return Credential{}, err
	}
	now := v.now()
	tried := false
	for _, candidate := range candidates {
		if !candidate.IsActive(now) {
			continue
		}
		secret := v.secrets.Get(serviceName, candidate.Version)
		if len(secret) == 0 {
			continue
		}
		tried = true
		verifier := sharedserviceauth.NewVerifier(sharedserviceauth.Secrets{serviceName: secret}, v.maxSkew)
		if _, err := verifier.VerifyRequest(req); err == nil {
			return candidate, nil
		}
	}
	if !tried {
		return Credential{}, ErrNoActiveCredential
	}
	// At least one active, secret-backed candidate existed but none verified
	// the signature -- a genuine authentication failure (wrong/corrupted
	// secret, tampered body, bad timestamp), not a scope/lifecycle gap.
	return Credential{}, apperrors.Unauthorized("Service authentication is invalid.", nil)
}

// Authorize checks method, route and (when eventType is non-empty) event
// scope against an already-resolved credential. routePattern must be the
// gin route TEMPLATE (c.FullPath()), matching
// internal/platform/riskregistry's own contract.
func (v *ScopeVerifier) Authorize(cred Credential, method, routePattern, eventType string) error {
	if !cred.AllowsMethod(method) {
		return apperrors.Forbidden("Service is not allowed to call this endpoint.", nil)
	}
	if !cred.AllowsRoute(routePattern) {
		return apperrors.Forbidden("Service is not allowed to call this endpoint.", nil)
	}
	if !cred.AllowsEventType(eventType) {
		return apperrors.Forbidden("Service is not allowed to produce this event type.", nil)
	}
	return nil
}

// TouchLastUsed records cred's use, logging (via the returned error, which
// callers should log and NOT propagate) rather than failing the request --
// an otherwise-fully-authorized request must never fail because a
// bookkeeping write did.
func (v *ScopeVerifier) TouchLastUsed(ctx context.Context, cred Credential) error {
	return v.store.TouchLastUsed(ctx, cred.ID)
}
