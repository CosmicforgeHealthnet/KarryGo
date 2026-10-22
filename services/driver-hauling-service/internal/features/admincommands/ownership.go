package admincommands

import (
	"context"

	"cosmicforge/logistics/shared/go/admincommand"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"github.com/jackc/pgx/v5"
)

const (
	subjectAsset    = "ASSET"
	subjectProvider = "PROVIDER"
	stateActive     = "ACTIVE"
	stateRestricted = "RESTRICTED"
)

// truckOwnerSQL resolves a truck's owning organization exactly as the existing
// restriction/eligibility bindings do: an ACTIVE truck link whose truck owner
// holds an ACTIVE provider link to the same organization.
const truckOwnerSQL = `SELECT v.provider_id::text, v.status, COALESCE((SELECT l.organization_id FROM hauling_organization_truck_links l
 JOIN hauling_organization_provider_links p ON p.provider_id=v.provider_id AND p.organization_id=l.organization_id AND p.state='ACTIVE'
 WHERE l.truck_id=v.id AND l.state='ACTIVE' ORDER BY l.organization_id LIMIT 1),'') FROM trucks v WHERE v.id::text=$1`

const providerOwnerSQL = `SELECT p.id::text, p.status, COALESCE((SELECT l.organization_id FROM hauling_organization_provider_links l WHERE l.provider_id=p.id AND l.state='ACTIVE'),'') FROM truck_providers p WHERE p.id::text=$1`

// ownerOrganization returns (organization, owner/provider id) for a truck
// (ASSET) or provider (PROVIDER) subject. An unknown subject is not found.
func ownerOrganization(ctx context.Context, tx pgx.Tx, typ, id string) (string, string, error) {
	query := truckOwnerSQL
	if typ == subjectProvider {
		query = providerOwnerSQL
	}
	var owner, status, organization string
	if err := tx.QueryRow(ctx, query, id).Scan(&owner, &status, &organization); err != nil {
		return "", "", mapNotFound(err)
	}
	return organization, owner, nil
}

// requireTenant enforces corporate/tenant ownership: the command's signed
// organization AND tenant must be the subject's owning organization. A subject
// with no authoritative organization binding cannot be acted on through a
// tenant-scoped intervention (the restriction ledger is organization-scoped).
func requireTenant(in admincommand.Execution, organization string) error {
	if organization == "" {
		return admincommand.ErrRejected
	}
	if in.Scope.Organization != organization || in.Scope.Tenant != organization {
		return adminpolicy.ErrDenied
	}
	return nil
}

// requireOptionalTenant applies the same rule when a planning organization is
// recorded (corporate bookings); a direct booking carries none.
func requireOptionalTenant(in admincommand.Execution, organization string) error {
	if organization == "" {
		return nil
	}
	return requireTenant(in, organization)
}

func restrictedAny(ctx context.Context, tx pgx.Tx, typ, id string) (bool, error) {
	var restricted bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM hauling_authority_restrictions WHERE subject_type=$1 AND subject_reference=$2 AND state='APPLIED')`, typ, id).Scan(&restricted)
	return restricted, err
}

type bookingRow struct {
	id, status, preferredType, organization string
	weightKg                                int
	fragile                                 bool
}

const bookingOrgExpr = `COALESCE(NULLIF(corporate_organization_id,''),winner_organization_id::text,'')`
const bookingParentPredicate = `(NULLIF(corporate_organization_id,'') IS NULL OR winner_organization_id IS NULL OR corporate_organization_id=winner_organization_id::text)`

func lockBooking(ctx context.Context, tx pgx.Tx, id string, lock bool) (bookingRow, error) {
	query := `SELECT id::text,status,COALESCE(preferred_truck_type,''),` + bookingOrgExpr + `,cargo_weight_kg,COALESCE(is_fragile,false) FROM haulage_bookings WHERE id::text=$1 AND ` + bookingParentPredicate
	if lock {
		query += ` FOR UPDATE`
	}
	var b bookingRow
	err := tx.QueryRow(ctx, query, id).Scan(&b.id, &b.status, &b.preferredType, &b.organization, &b.weightKg, &b.fragile)
	return b, mapNotFound(err)
}

type recoveryRow struct {
	id, state, bookingID, organization, replacementProvider string
}

func lockRecovery(ctx context.Context, tx pgx.Tx, id string, lock bool) (recoveryRow, error) {
	query := `SELECT t.id::text,t.state,t.booking_id::text,COALESCE(NULLIF(b.corporate_organization_id,''),b.winner_organization_id::text,''),COALESCE(t.replacement_provider_id::text,'')
 FROM hauling_recovery_cases t JOIN haulage_bookings b ON b.id=t.booking_id WHERE t.id::text=$1
 AND (NULLIF(b.corporate_organization_id,'') IS NULL OR b.winner_organization_id IS NULL OR b.corporate_organization_id=b.winner_organization_id::text)`
	if lock {
		query += ` FOR UPDATE OF t`
	}
	var r recoveryRow
	err := tx.QueryRow(ctx, query, id).Scan(&r.id, &r.state, &r.bookingID, &r.organization, &r.replacementProvider)
	return r, mapNotFound(err)
}

type eligibilityRow struct {
	eligibilityID, truckID, legacyRef, organization string
}

// lockEligibility resolves a product-eligibility subject through the same
// live provider+truck organization links its read contract uses.
func lockEligibility(ctx context.Context, tx pgx.Tx, ref string, lock bool) (eligibilityRow, error) {
	query := `SELECT t.id::text,v.id::text,v.inspection_ref,l.organization_id FROM hauling_truck_product_eligibility t
 JOIN trucks v ON v.id=t.truck_id AND v.provider_id=t.provider_id
 JOIN truck_providers p ON p.id=t.provider_id
 JOIN hauling_organization_truck_links l ON l.truck_id=v.id AND l.state='ACTIVE'
 JOIN hauling_organization_provider_links m ON m.provider_id=p.id AND m.organization_id=l.organization_id AND m.state='ACTIVE'
 WHERE t.id::text||':'||l.organization_id::text=$1`
	if lock {
		query += ` FOR UPDATE OF t, v`
	}
	var r eligibilityRow
	err := tx.QueryRow(ctx, query, ref).Scan(&r.eligibilityID, &r.truckID, &r.legacyRef, &r.organization)
	return r, mapNotFound(err)
}
