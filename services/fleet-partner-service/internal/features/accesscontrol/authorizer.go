// Package accesscontrol implements deny-by-default authorization for
// corporate routes.
//
// Authority exists only where an unexpired, unrevoked role binding grants the
// permission within a scope that covers the target. Nothing is inferred from
// token claims, ownership flags or organization membership alone.
package accesscontrol

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cache"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
)

// BindingReader exposes the effective permissions of a membership.
type BindingReader interface {
	EffectivePermissions(ctx context.Context, organizationID string, memberID string) (map[string][]models.RoleBinding, error)
}

// Decision describes one authorization outcome.
type Decision struct {
	Allowed bool
	Reason  string
}

// Target narrows a permission check to a branch or department.
type Target struct {
	BranchID     string
	DepartmentID string
}

// Authorizer evaluates permission checks.
type Authorizer struct {
	bindings BindingReader
	cache    *cache.Cache
	cacheTTL time.Duration
	now      func() time.Time
}

// NewAuthorizer builds the authorizer. The cache may be nil.
func NewAuthorizer(bindings BindingReader, entryCache *cache.Cache, cacheTTL time.Duration) *Authorizer {
	if cacheTTL <= 0 {
		cacheTTL = 30 * time.Second
	}
	return &Authorizer{bindings: bindings, cache: entryCache, cacheTTL: cacheTTL, now: time.Now}
}

// Authorize reports whether a membership holds a permission over a target.
//
// roleVersion participates in the cache key, so any role change immediately
// invalidates cached decisions without a broadcast: a stale entry simply
// belongs to a key nobody looks up any more.
func (a *Authorizer) Authorize(ctx context.Context, organizationID string, memberID string, roleVersion int64, permission string, target Target) (Decision, error) {
	if organizationID == "" || memberID == "" || permission == "" {
		return Decision{Allowed: false, Reason: "missing_scope"}, nil
	}

	cacheKey := cacheKeyFor(organizationID, memberID, roleVersion, permission, target)
	if a.cache != nil {
		if value, err := a.cache.Get(ctx, cacheKey); err == nil {
			return Decision{Allowed: value == "allow", Reason: "cached"}, nil
		}
	}

	permissions, err := a.bindings.EffectivePermissions(ctx, organizationID, memberID)
	if err != nil {
		return Decision{}, fmt.Errorf("accesscontrol: load permissions: %w", err)
	}

	decision := evaluate(permissions[permission], target, a.now())

	if a.cache != nil {
		value := "deny"
		if decision.Allowed {
			value = "allow"
		}
		// Always TTL-bounded; the cache never becomes authority.
		_ = a.cache.Set(ctx, cacheKey, value, a.cacheTTL)
	}
	return decision, nil
}

// Require returns a 403 error when the permission is not held.
func (a *Authorizer) Require(ctx context.Context, organizationID string, memberID string, roleVersion int64, permission string, target Target) error {
	decision, err := a.Authorize(ctx, organizationID, memberID, roleVersion, permission, target)
	if err != nil {
		return err
	}
	if !decision.Allowed {
		return apperrors.Forbidden("You do not have permission to perform this action.", nil)
	}
	return nil
}

// evaluate applies scope rules to the bindings granting a permission.
//
//   - ORGANIZATION scope covers every branch and department.
//   - BRANCH scope covers only its own branch (and departments in it, which
//     the caller resolves by passing the department's branch).
//   - DEPARTMENT scope covers only its own department.
func evaluate(bindings []models.RoleBinding, target Target, now time.Time) Decision {
	for _, binding := range bindings {
		if !binding.Active(now) {
			continue
		}
		switch binding.ScopeType {
		case models.ScopeOrganization:
			return Decision{Allowed: true, Reason: "organization_scope"}
		case models.ScopeBranch:
			if binding.ScopeID != nil && target.BranchID != "" && *binding.ScopeID == target.BranchID {
				return Decision{Allowed: true, Reason: "branch_scope"}
			}
		case models.ScopeDepartment:
			if binding.ScopeID != nil && target.DepartmentID != "" && *binding.ScopeID == target.DepartmentID {
				return Decision{Allowed: true, Reason: "department_scope"}
			}
		}
	}
	return Decision{Allowed: false, Reason: "no_matching_binding"}
}

// RequireGrantable refuses privilege escalation: an actor may only grant
// permissions they themselves currently hold at organization scope.
func (a *Authorizer) RequireGrantable(ctx context.Context, organizationID string, actorMemberID string, actorRoleVersion int64, requested []string) error {
	held, err := a.bindings.EffectivePermissions(ctx, organizationID, actorMemberID)
	if err != nil {
		return fmt.Errorf("accesscontrol: load actor permissions: %w", err)
	}
	now := a.now()
	for _, permission := range requested {
		decision := evaluate(held[permission], Target{}, now)
		if !decision.Allowed {
			return errorcodes.Forbidden(errorcodes.PermissionEscalationForbidden,
				"You cannot grant a permission that you do not hold.")
		}
	}
	return nil
}

func cacheKeyFor(organizationID string, memberID string, roleVersion int64, permission string, target Target) string {
	return "authz:" + organizationID + ":" + memberID + ":v" + strconv.FormatInt(roleVersion, 10) +
		":" + permission + ":b" + target.BranchID + ":d" + target.DepartmentID
}
