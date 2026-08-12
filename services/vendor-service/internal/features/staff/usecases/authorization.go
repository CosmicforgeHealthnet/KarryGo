// Package staffusecases implements Staff IAM business rules: provisioning,
// suspension, role/permission management, and the authorization evaluator
// every future business feature will call.
//
// The evaluator asks only "does this actor hold this permission at this
// scope" — never "is this actor Branch Manager." Role names exist only for
// management/usability; permissions are the sole authorization authority
// (Section 5 non-negotiable rule 16).
package staffusecases

import (
	"context"

	staffrepositories "cosmicforge/logistics/services/vendor-service/internal/features/staff/repositories"
)

// Target narrows a permission check to a branch. A nil BranchID means "no
// branch restriction" — appropriate for vendor-wide operations (e.g. reading
// the business's own settings). Checking a branch-scoped operation without a
// Target is a caller bug, not a security decision this evaluator should paper
// over — every branch-scoped route must supply one.
type Target struct {
	BranchID *string
}

// Decision is the evaluator's answer, always with a Reason even when denied —
// useful for audit/observability, never returned to an unauthorized caller as
// detailed justification (the HTTP layer maps any deny to a generic
// "forbidden").
type Decision struct {
	Allowed bool
	Reason  string
}

// AuthorizationEvaluator resolves a staff member's effective permission set
// (from their active role assignments, joined through role_permissions) and
// answers scoped authorization questions. Deny-by-default: a staff member
// with no matching active, non-expired assignment granting the exact
// permission at a covering scope is denied, full stop — nothing is inferred
// from staff status, vendor ownership, or any other signal.
type AuthorizationEvaluator struct {
	assignments *staffrepositories.AssignmentRepository
}

func NewAuthorizationEvaluator(assignments *staffrepositories.AssignmentRepository) *AuthorizationEvaluator {
	return &AuthorizationEvaluator{assignments: assignments}
}

// EffectiveGrant is one (permission, scope) pair a staff member currently
// holds via some active assignment.
type EffectiveGrant struct {
	PermissionCode string
	BranchID       *string // nil == vendor-wide
	IsHighRisk     bool
}

// EffectivePermissions returns every permission a staff member currently
// holds, each annotated with the branch scope that granted it. Callers that
// only need a yes/no answer should prefer Authorize; this is for
// "what can this actor do" listings (e.g. rendering a permissions summary).
func (e *AuthorizationEvaluator) EffectivePermissions(ctx context.Context, staffID string) ([]EffectiveGrant, error) {
	rows, err := e.assignments.EffectivePermissions(ctx, staffID)
	if err != nil {
		return nil, err
	}
	grants := make([]EffectiveGrant, 0, len(rows))
	for _, row := range rows {
		grants = append(grants, EffectiveGrant{PermissionCode: row.PermissionCode, BranchID: row.BranchID, IsHighRisk: row.IsHighRisk})
	}
	return grants, nil
}

// Authorize reports whether staffID holds permissionCode covering target.
//
// A vendor-wide grant (BranchID nil on the grant) covers every target,
// including a branch-scoped one. A branch-scoped grant only covers a target
// with the exact same BranchID — it never covers a vendor-wide target
// (target.BranchID nil) and never covers a different branch. This is what
// makes "a Branch Manager cannot gain another branch merely by submitting its
// branch ID" true: the grant itself is scoped at assignment time, not at
// request time.
func (e *AuthorizationEvaluator) Authorize(ctx context.Context, staffID, permissionCode string, target Target) (Decision, error) {
	if staffID == "" || permissionCode == "" {
		return Decision{Allowed: false, Reason: "missing_scope"}, nil
	}
	grants, err := e.EffectivePermissions(ctx, staffID)
	if err != nil {
		return Decision{}, err
	}
	for _, grant := range grants {
		if grant.PermissionCode != permissionCode {
			continue
		}
		if grant.BranchID == nil {
			return Decision{Allowed: true, Reason: "vendor_wide_grant"}, nil
		}
		if target.BranchID != nil && *target.BranchID == *grant.BranchID {
			return Decision{Allowed: true, Reason: "branch_scoped_grant"}, nil
		}
	}
	return Decision{Allowed: false, Reason: "no_covering_grant"}, nil
}

// HasPermission is a convenience helper over an already-fetched grant list —
// used where a caller has already loaded EffectivePermissions once and wants
// a plain vendor-wide-or-any-branch yes/no without a second query (e.g.
// "does this actor hold vendor.roles.manage at all, at any scope").
func HasPermission(grants []EffectiveGrant, permissionCode string) bool {
	for _, g := range grants {
		if g.PermissionCode == permissionCode {
			return true
		}
	}
	return false
}
