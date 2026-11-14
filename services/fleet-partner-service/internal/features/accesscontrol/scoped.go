package accesscontrol

import (
	"context"
	"fmt"
	"sort"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// RequireScoped authorizes a permission and returns the tenant context
// narrowed to the branches where that permission is actually granted.
//
// Branch-scoped role bindings must never be satisfied by a branch the caller
// names in the request: object checks downstream compare the object's own
// branch with the returned context's PermittedBranchIDs, so a member whose
// only grant is "Operations at branch A" cannot read or change branch B by
// adding ?branchId=A. ORGANIZATION-scoped grants leave the context as is.
// DEPARTMENT-scoped grants do not confer branch-wide object access; a
// handler that authorizes department-owned objects must check the
// department explicitly with Require.
func (a *Authorizer) RequireScoped(ctx context.Context, t tenant.Context, roleVersion int64, permission string) (tenant.Context, error) {
	if t.OrganizationID == "" || t.OrganizationUserID == "" || permission == "" {
		return t, apperrors.Forbidden("You do not have permission to perform this action.", nil)
	}
	permissions, err := a.bindings.EffectivePermissions(ctx, t.OrganizationID, t.OrganizationUserID)
	if err != nil {
		return t, fmt.Errorf("accesscontrol: load permissions: %w", err)
	}
	branches, orgWide := grantedBranches(permissions[permission], a)
	if orgWide {
		return t, nil
	}
	narrowed := intersectBranches(t.PermittedBranchIDs, branches)
	if len(narrowed) == 0 {
		return t, apperrors.Forbidden("You do not have permission to perform this action.", nil)
	}
	t.PermittedBranchIDs = narrowed
	return t, nil
}

func grantedBranches(bindings []models.RoleBinding, a *Authorizer) ([]string, bool) {
	now := a.now()
	seen := map[string]bool{}
	for _, binding := range bindings {
		if !binding.Active(now) {
			continue
		}
		switch binding.ScopeType {
		case models.ScopeOrganization:
			return nil, true
		case models.ScopeBranch:
			if binding.ScopeID != nil && *binding.ScopeID != "" {
				seen[*binding.ScopeID] = true
			}
		}
	}
	branches := make([]string, 0, len(seen))
	for id := range seen {
		branches = append(branches, id)
	}
	sort.Strings(branches)
	return branches, false
}

// intersectBranches combines the membership's own branch restriction (empty
// means all branches) with the branches a permission is granted at.
func intersectBranches(permitted []string, granted []string) []string {
	if len(permitted) == 0 {
		return granted
	}
	allowed := map[string]bool{}
	for _, id := range permitted {
		allowed[id] = true
	}
	out := []string{}
	for _, id := range granted {
		if allowed[id] {
			out = append(out, id)
		}
	}
	return out
}
