package taxonomy

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Section 13I permission vocabulary. The codes are catalogued by migration
// 000059; these constants are the single place the Go side names them.
//
// PermissionTaxonomyPlatformAdmin is deliberately NOT a vendor staff
// capability: it is granted to no vendor role family, and the platform
// administration surface that would consume it is a root-mounted HMAC-v2
// internal API rather than a bearer route. It is named here so the IAM tests
// can assert its absence, and so a future platform operator console has one
// vocabulary to bind to.
const (
	PermissionTaxonomyRead          = "vendor.taxonomy.read"
	PermissionTaxonomyMappingManage = "vendor.taxonomy.mapping.manage"
	PermissionTaxonomyPlatformAdmin = "vendor.taxonomy.platform.admin"
)

// Scope mirrors the scope shape every other Vendor feature uses: either the
// whole business, or an explicit set of branches.
type Scope struct {
	Global    bool
	BranchIDs []string
}

// Empty reports a scope that authorises nothing.
func (s Scope) Empty() bool { return !s.Global && len(s.BranchIDs) == 0 }

// Covers reports whether the scope reaches a branch.
func (s Scope) Covers(branchID string) bool {
	if s.Global {
		return true
	}
	for _, candidate := range s.BranchIDs {
		if candidate == branchID {
			return true
		}
	}
	return false
}

// ScopeResolver answers "at which scopes does this staff member hold this
// permission". It is a port so the taxonomy feature never imports the staff
// feature's repositories directly (see internal/architecture_test.go).
type ScopeResolver interface {
	ResolveScope(ctx context.Context, staffID, permission string) (Scope, error)
}

// ---------------------------------------------------------------------------
// Authoritative source ownership
// ---------------------------------------------------------------------------

// Vendor-facing source types. This vocabulary is bounded on purpose: a mapping
// is only reachable through the vendor API for a source whose ownership this
// service can resolve authoritatively from the owning catalogue table. The
// mapping table itself keeps source_type free-text, because the platform
// surface classifies resources this list does not cover.
const (
	SourceTypeFoodItem    = "FOOD_ITEM"
	SourceTypeShopProduct = "SHOP_PRODUCT"
	SourceTypeShopVariant = "SHOP_VARIANT"
)

// SourceOwnership is the authoritative answer to "who owns this resource",
// read from the owning catalogue table rather than from the request.
type SourceOwnership struct {
	SourceType string
	SourceRef  string

	BusinessRef string

	// BranchRef is empty for business-wide resources. Shop products and
	// variants are defined once per business; Food items belong to a branch.
	BranchRef string
}

// NamespaceForSourceType binds each vendor source type to exactly one
// canonical namespace. This is the structural half of FOOD/SHOP isolation: a
// Food item can never be classified inside the Shop taxonomy, whatever the
// caller submits, because the binding is decided here and not by the request.
func NamespaceForSourceType(sourceType string) (NamespaceCode, bool) {
	switch strings.ToUpper(strings.TrimSpace(sourceType)) {
	case SourceTypeFoodItem:
		return NamespaceFood, true
	case SourceTypeShopProduct, SourceTypeShopVariant:
		return NamespaceShop, true
	default:
		return "", false
	}
}

// VendorAccess resolves vendor authorization and authoritative resource
// ownership for the Section 13I vendor-facing taxonomy API.
//
// Every method answers from the database or the scope resolver. Nothing here
// trusts a business_ref or branch_ref that arrived in a request body, path or
// query string — that is the whole point of the type.
type VendorAccess struct {
	pool   *pgxpool.Pool
	scopes ScopeResolver
}

func NewVendorAccess(pool *pgxpool.Pool, scopes ScopeResolver) *VendorAccess {
	return &VendorAccess{pool: pool, scopes: scopes}
}

// Authorize resolves the scopes at which a staff member holds a permission.
// An absent permission is deny-by-default: the resolver returns an empty
// scope and this returns ErrForbidden.
func (a *VendorAccess) Authorize(
	ctx context.Context,
	staffID string,
	permission string,
) (Scope, error) {
	if a == nil || a.scopes == nil || staffID == "" || permission == "" {
		return Scope{}, ErrForbidden
	}
	scope, err := a.scopes.ResolveScope(ctx, staffID, permission)
	if err != nil {
		return Scope{}, err
	}
	if scope.Empty() {
		return Scope{}, ErrForbidden
	}
	return scope, nil
}

// ResolveSource establishes, authoritatively, that the named resource exists,
// belongs to the acting business, sits inside the acting staff member's branch
// scope, and belongs to the namespace being addressed.
//
// It returns ErrNotFound — never ErrForbidden — when a resource exists but
// belongs to somebody else. Distinguishing the two would let a caller
// enumerate another vendor's catalogue by watching the status code, which is
// the same reasoning Section 12A records for the Shop catalogue.
func (a *VendorAccess) ResolveSource(
	ctx context.Context,
	namespace NamespaceCode,
	sourceType string,
	sourceRef string,
	businessID string,
	scope Scope,
) (SourceOwnership, error) {
	if a == nil || a.pool == nil {
		return SourceOwnership{}, ErrInvalidArgument
	}

	sourceType = strings.ToUpper(strings.TrimSpace(sourceType))
	sourceRef = strings.TrimSpace(sourceRef)
	businessID = strings.TrimSpace(businessID)

	if sourceType == "" || sourceRef == "" || businessID == "" {
		return SourceOwnership{}, ErrInvalidArgument
	}

	bound, ok := NamespaceForSourceType(sourceType)
	if !ok {
		return SourceOwnership{}, ErrInvalidArgument
	}

	// The cross-namespace gate. A Food source addressed through the Shop
	// namespace (or the reverse) is refused before any lookup happens.
	if bound != namespace {
		return SourceOwnership{}, ErrNamespaceMismatch
	}

	var (
		ownerBusiness string
		ownerBranch   string
		err           error
	)

	switch sourceType {
	case SourceTypeFoodItem:
		err = a.pool.QueryRow(
			ctx,
			`SELECT business_id, branch_id FROM food_items WHERE id=$1`,
			sourceRef,
		).Scan(&ownerBusiness, &ownerBranch)

	case SourceTypeShopProduct:
		err = a.pool.QueryRow(
			ctx,
			`SELECT business_id FROM shop_products WHERE id=$1`,
			sourceRef,
		).Scan(&ownerBusiness)

	case SourceTypeShopVariant:
		err = a.pool.QueryRow(
			ctx,
			`SELECT business_id FROM shop_variants WHERE id=$1`,
			sourceRef,
		).Scan(&ownerBusiness)
	}

	if err != nil {
		if err == pgx.ErrNoRows {
			return SourceOwnership{}, ErrNotFound
		}
		return SourceOwnership{}, err
	}

	if ownerBusiness != businessID {
		return SourceOwnership{}, ErrNotFound
	}

	// A branch-scoped resource additionally has to sit inside the branch scope
	// this staff member actually holds the permission at. A business-wide
	// resource requires a business-wide grant.
	if ownerBranch != "" {
		if !scope.Covers(ownerBranch) {
			return SourceOwnership{}, ErrNotFound
		}
	} else if !scope.Global {
		return SourceOwnership{}, ErrForbidden
	}

	return SourceOwnership{
		SourceType:  sourceType,
		SourceRef:   sourceRef,
		BusinessRef: ownerBusiness,
		BranchRef:   ownerBranch,
	}, nil
}
