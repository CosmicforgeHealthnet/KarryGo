package admincontext

import (
	"context"

	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminread"
	"github.com/jackc/pgx/v5"
)

// CandidateRefs is an owner-internal index lookup, not authorization. Resolve
// and governance/staff policy must validate every candidate before disclosure.
func (s *Store) CandidateRefs(ctx context.Context, tx pgx.Tx, kind, capability string, scope adminpolicy.Scope, after string, limit int) ([]string, error) {
	if limit < 1 || limit > 101 || after != "" && !adminread.Opaque(after) {
		return nil, ErrUnmapped
	}
	rows, err := tx.Query(ctx, `SELECT DISTINCT resource_ref FROM admin_owner_context_heads WHERE owner=$1 AND resource_kind=$2 AND product=$3 AND market=$4 AND zone=$5 AND organization_ref=$6 AND tenant_ref=$7 AND $8=ANY(capabilities) AND resource_ref>$9 ORDER BY resource_ref LIMIT $10`, s.Authority.Owner, kind, scope.Product, scope.Market, scope.Zone, scope.Organization, scope.Tenant, capability, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	refs := []string{}
	for rows.Next() {
		var ref string
		if err := rows.Scan(&ref); err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, rows.Err()
}
