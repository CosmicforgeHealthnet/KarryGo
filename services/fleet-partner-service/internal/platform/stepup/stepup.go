// Package stepup defines step-up authentication for high-risk actions.
//
// It holds three things:
//
//   - Verifier, the narrow port that checks a caller-supplied step-up
//     reference against the auth feature's own authoritative grants
//     (internal/features/auth/usecases.Service.VerifyStepUpGrant). A consuming
//     feature never imports the auth feature directly; the real adapter lives
//     in internal/app (the composition root), same as every other
//     cross-feature dependency in this service.
//   - The route registry: the high-risk corporate routes that always require
//     a fresh step-up grant, each bound to one action class. The tenant
//     middleware (internal/middleware.RequireStepUp) enforces it for every
//     request, so a protected route cannot be mounted without enforcement.
//   - Proof, the per-request evidence the middleware attaches to the request
//     context. A usecase whose step-up requirement depends on the data (a
//     sensitive export category, for example) calls Require with it.
//
// Enforcement for the registry is unconditional: spec 64.1 makes step-up
// mandatory for role/permission changes, approval decisions, wallet
// allocations/withdrawals, payroll finalization, API secret rotation,
// sensitive exports and organization closure, and spec 56.2 repeats the rule.
// The organization's mfaRequiredForSensitiveActions setting (spec 28.1's
// "where configured") does not switch these off; the integrations feature
// applies the same rule to API and webhook secret changes. The setting still
// governs the actions that were step-up protected only by organization
// choice (legal holds, retention execution, security settings, webhook
// updates and delivery replays).
//
// Every check fails closed: a missing proof, a missing verifier or a missing
// reference refuses the action rather than letting it through.
package stepup

import (
	"context"
	"errors"
	"sort"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
)

// Verifier checks that reference is a valid, unexpired step-up grant for
// exactly this identity, session, organization and action class. A grant
// cannot be transferred to another session or reused for a different action
// category — see the auth feature's own VerifyStepUpGrant doc comment for
// why.
type Verifier interface {
	Verify(ctx context.Context, reference, identityID, sessionID, organizationID, actionClass string) error
}

// HeaderReference carries the step-up reference returned by
// POST /auth/step-up/verify on a protected request.
const HeaderReference = "X-Step-Up-Reference"

// DetailActionClass names the error-envelope detail that tells a client which
// action class to start a step-up challenge for (POST /auth/step-up/start
// {"actionClass": ...}).
const DetailActionClass = "actionClass"

// Action classes. A grant is valid for exactly one class, so these group
// actions a person would reasonably confirm together and nothing wider.
const (
	// ActionAccessManage covers role and permission changes: custom role
	// create/update/archive, replacing a staff member's role bindings, and
	// suspending or reactivating a staff member.
	ActionAccessManage = "iam.access.manage"
	// ActionOrganizationClosure covers requesting, approving and executing
	// organization closure.
	ActionOrganizationClosure = "organization.closure"
	// ActionWalletAllocation covers creating and approving a provider
	// sub-wallet allocation.
	ActionWalletAllocation = "wallet.allocation"
	// ActionWalletWithdrawal covers creating and approving a provider
	// sub-wallet withdrawal.
	ActionWalletWithdrawal = "wallet.withdrawal"
	// ActionPayrollFinalize covers approving and posting a payroll period.
	ActionPayrollFinalize = "payroll.finalize"
	// ActionProviderRemove covers removing a provider from the organization.
	ActionProviderRemove = "providers.remove"
	// ActionSensitiveExport covers creating, replaying and downloading an
	// export of a sensitive category (see the exports feature).
	ActionSensitiveExport = "exports.sensitive"
	// ActionApprovalDecide covers approving or rejecting an approval request
	// (spec 64.1: step-up is mandatory for approval decisions).
	ActionApprovalDecide = "approvals.decide"
)

// routes maps "METHOD <gin route template>" to the action class every request
// to that route must prove. Paths are full templates as c.FullPath() reports
// them.
var routes = map[string]string{
	// Role and permission changes.
	"POST /fleet-partner/v1/roles":                  ActionAccessManage,
	"PATCH /fleet-partner/v1/roles/:roleId":         ActionAccessManage,
	"POST /fleet-partner/v1/roles/:roleId/archive":  ActionAccessManage,
	"PUT /fleet-partner/v1/staff/:staffId/roles":    ActionAccessManage,
	"POST /fleet-partner/v1/staff/:staffId/suspend": ActionAccessManage,
	// Reactivation restores a suspended person's access, so it is the same
	// access decision as suspension.
	"POST /fleet-partner/v1/staff/:staffId/reactivate": ActionAccessManage,

	// Organization closure.
	"POST /fleet-partner/v1/organizations/:organizationId/close-request":   ActionOrganizationClosure,
	"POST /fleet-partner/v1/organizations/:organizationId/closure/approve": ActionOrganizationClosure,
	"POST /fleet-partner/v1/organizations/:organizationId/closure/execute": ActionOrganizationClosure,

	// Wallet allocations and withdrawals.
	"POST /fleet-partner/v1/providers/:membershipId/sub-wallet/allocations":                       ActionWalletAllocation,
	"POST /fleet-partner/v1/providers/:membershipId/sub-wallet/allocations/:allocationId/approve": ActionWalletAllocation,
	"POST /fleet-partner/v1/providers/:membershipId/sub-wallet/withdrawals":                       ActionWalletWithdrawal,
	"POST /fleet-partner/v1/providers/:membershipId/sub-wallet/withdrawals/:withdrawalId/approve": ActionWalletWithdrawal,

	// Payroll finalization.
	"POST /fleet-partner/v1/payroll-periods/:periodId/approve": ActionPayrollFinalize,
	"POST /fleet-partner/v1/payroll-periods/:periodId/post":    ActionPayrollFinalize,

	// Provider removal.
	"POST /fleet-partner/v1/providers/:membershipId/remove": ActionProviderRemove,

	// Approval decisions.
	"POST /fleet-partner/v1/approvals/requests/:approvalId/approve": ActionApprovalDecide,
	"POST /fleet-partner/v1/approvals/requests/:approvalId/reject":  ActionApprovalDecide,
}

// RouteActionClass reports the action class a route requires, if any.
func RouteActionClass(method, fullPath string) (string, bool) {
	class, ok := routes[method+" "+fullPath]
	return class, ok
}

// Route is one protected route, for contract tests and documentation.
type Route struct {
	Method      string
	Path        string
	ActionClass string
}

// Routes returns every protected route, sorted by method and path.
func Routes() []Route {
	out := make([]Route, 0, len(routes))
	for key, class := range routes {
		for i := 0; i < len(key); i++ {
			if key[i] == ' ' {
				out = append(out, Route{Method: key[:i], Path: key[i+1:], ActionClass: class})
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Method < out[j].Method
	})
	return out
}

// Proof is what a request offers as step-up evidence: the reference it sent
// and the trusted identity, session and organization it was authenticated as.
// The middleware builds it from the verified token and the resolved tenant,
// never from request input other than the reference itself.
type Proof struct {
	Reference      string
	IdentityID     string
	SessionID      string
	OrganizationID string

	verifier Verifier
	verified map[string]bool
}

// NewProof builds a request's proof. verifier may be nil; Require then
// refuses every check.
func NewProof(verifier Verifier, reference, identityID, sessionID, organizationID string) *Proof {
	return &Proof{
		Reference: reference, IdentityID: identityID, SessionID: sessionID, OrganizationID: organizationID,
		verifier: verifier, verified: map[string]bool{},
	}
}

type contextKey struct{}

// ContextKey is the string key the proof is also stored under on a gin
// context, so a handler passing the *gin.Context itself as context.Context
// still resolves it.
const ContextKey = "fleet_partner_step_up_proof"

// WithProof attaches the proof to ctx.
func WithProof(ctx context.Context, proof *Proof) context.Context {
	return context.WithValue(ctx, contextKey{}, proof)
}

// FromContext returns the request's proof, if the step-up middleware ran.
func FromContext(ctx context.Context) (*Proof, bool) {
	if ctx == nil {
		return nil, false
	}
	if proof, ok := ctx.Value(contextKey{}).(*Proof); ok && proof != nil {
		return proof, true
	}
	if proof, ok := ctx.Value(ContextKey).(*Proof); ok && proof != nil {
		return proof, true
	}
	return nil, false
}

// Required builds the refusal for an action that needs a step-up grant the
// caller did not supply.
func Required(actionClass string) *apperrors.Error {
	err := errorcodes.Forbidden(errorcodes.StepUpRequired, "This action requires additional verification.")
	err.Fields = []apperrors.FieldViolation{{Field: DetailActionClass, Message: actionClass}}
	return err
}

// Unconfigured is the refusal when a step-up check must run but no verifier
// is wired. A missing dependency never turns a required check into a pass.
func Unconfigured() *apperrors.Error {
	return errorcodes.Unavailable(errorcodes.FleetServiceUnavailable, "Additional verification is temporarily unavailable.")
}

// Require proves ctx carries a valid step-up grant for actionClass. It fails
// closed: no proof (the call did not come through the step-up middleware), no
// verifier or no reference all refuse.
func Require(ctx context.Context, actionClass string) error {
	proof, ok := FromContext(ctx)
	if !ok {
		return Required(actionClass)
	}
	return proof.Require(ctx, actionClass)
}

// Require checks the proof against one action class. A successful check is
// remembered for the rest of the request so a second call does not repeat
// the lookup.
func (p *Proof) Require(ctx context.Context, actionClass string) error {
	if p.verified[actionClass] {
		return nil
	}
	if p.verifier == nil {
		return Unconfigured()
	}
	if p.Reference == "" || p.IdentityID == "" || p.SessionID == "" || p.OrganizationID == "" {
		return Required(actionClass)
	}
	if err := p.verifier.Verify(ctx, p.Reference, p.IdentityID, p.SessionID, p.OrganizationID, actionClass); err != nil {
		return withActionClass(err, actionClass)
	}
	p.verified[actionClass] = true
	return nil
}

// VerifiedReference returns the step-up reference ctx proved during this
// request, or "" when none was verified. Audit records cite it as step-up
// evidence.
func VerifiedReference(ctx context.Context) string {
	proof, ok := FromContext(ctx)
	if !ok || len(proof.verified) == 0 {
		return ""
	}
	return proof.Reference
}

// withActionClass adds the action-class detail to a step-up refusal so the
// client can start the right challenge. Other errors pass through unchanged.
func withActionClass(err error, actionClass string) error {
	var appErr *apperrors.Error
	if !errors.As(err, &appErr) || (appErr.Code != errorcodes.StepUpRequired && appErr.Code != errorcodes.StepUpExpired) {
		return err
	}
	decorated := *appErr
	decorated.Fields = append(append([]apperrors.FieldViolation{}, appErr.Fields...),
		apperrors.FieldViolation{Field: DetailActionClass, Message: actionClass})
	return &decorated
}
