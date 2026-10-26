// Package adminresources exposes Verification's fixed, privacy-minimized
// Secure Admin owner projections (Section 18). Document references, identity
// numbers, bank data, remediation text and reviewer free text are never
// projected; automation output appears only as bounded finding codes.
package adminresources

import (
	"context"

	"cosmicforge/logistics/shared/go/adminowner"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	Owner  = "verification-compliance-service"
	Prefix = "VERIFICATION_COMPLIANCE"
)

// Native mutation permissions held in verification_admin_staff_grants.
const (
	PermissionReview = "verification.case.review"
	PermissionDecide = "verification.case.decide"
)

type SourceReader struct{ *adminowner.SQLSource }

// vendorCases projects counts, never the document references themselves.
const vendorCases = `(SELECT c.id,c.business_id,c.dimension,c.subject_type,c.subject_ref,c.status,c.source_version,c.reason_code,c.expires_at,c.automation_recommendation,c.created_at,c.updated_at,
 jsonb_array_length(c.document_refs) AS document_count,
 (SELECT count(*) FROM vendor_case_automation_findings f WHERE f.case_id=c.id) AS finding_count,
 (SELECT count(*) FROM vendor_case_appeals a WHERE a.case_id=c.id) AS appeal_count
 FROM vendor_verification_cases c)`

const assetCases = `(SELECT a.id,a.authority,a.provider_id,a.asset_id,a.capability_code,a.product_code,a.status,a.source_version,a.expires_at,a.created_at,a.updated_at,
 CASE WHEN a.policy_version ~ '^[0-9]{1,15}$' THEN a.policy_version::bigint END AS policy_version_number
 FROM asset_capability_cases a)`

func Source() *SourceReader {
	return &SourceReader{adminowner.NewSQLSource(Owner, map[string]adminowner.SQLSpec{
		"vendor-cases": {Table: vendorCases, StateColumn: "status", VersionColumn: "source_version", BindingColumns: []string{"business_id", "dimension", "subject_type", "subject_ref"},
			Dimensions: map[string]string{"dimension": "dimension", "subject_type": "subject_type", "automation_recommendation": "automation_recommendation", "expires_at": "expires_at", "reason_code": "reason_code"},
			Counters:   map[string]string{"document_count": "document_count", "finding_count": "finding_count", "appeal_count": "appeal_count"},
			References: map[string]string{"business": "business_id", "subject": "subject_ref"},
			// The formal timeline: status per version, never who or why in prose.
			TimelineTable: "vendor_verification_case_history", TimelineParentColumn: "case_id", TimelineTypeColumn: "status", TimelineTimeColumn: "created_at"},
		"vendor-case-findings": {Table: "vendor_case_automation_findings", StateColumn: "outcome", CreatedAtExpression: "t.evaluated_at", UpdatedAtExpression: "t.evaluated_at", BindingColumns: []string{"case_id", "case_version", "check_code"},
			Dimensions: map[string]string{"check_code": "check_code", "outcome": "outcome", "severity": "severity"},
			Counters:   map[string]string{"case_version": "case_version"},
			References: map[string]string{"case": "case_id"}},
		"asset-capability-cases": {Table: assetCases, StateColumn: "status", VersionColumn: "source_version", BindingColumns: []string{"authority", "provider_id", "asset_id", "capability_code", "product_code"},
			Dimensions: map[string]string{"authority": "authority", "capability_code": "capability_code", "product_code": "product_code", "expires_at": "expires_at"},
			Counters:   map[string]string{"policy_version": "policy_version_number"},
			References: map[string]string{"asset": "asset_id", "provider": "provider_id"}},
		"organization-submissions": {Table: "organization_verification_submissions", StateColumn: "status", VersionColumn: "source_version", OrganizationColumn: "organization_id", CanonicalTenantIsOrganization: true, BindingColumns: []string{"organization_id", "submission_id"},
			Dimensions: map[string]string{"restriction_status": "restriction_status"},
			References: map[string]string{"organization": "organization_id"}},
	})}
}

var readPermission = map[string]string{
	"vendor-cases":             "verification.vendor_cases.read",
	"vendor-case-findings":     "verification.findings.read",
	"asset-capability-cases":   "verification.asset_cases.read",
	"organization-submissions": "verification.organization_submissions.read",
}

// ReadPermission is the native grant a kind requires.
func ReadPermission(kind string) string { return readPermission[kind] }

const currentGrant = `SELECT id FROM verification_admin_staff_grants WHERE staff_ref=$1 AND permission=$2 AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at>now()) ORDER BY id LIMIT 1`

// requireGrant reads a current grant. Reads run inside read-only snapshots
// (command history), so they cannot take a row lock.
func requireGrant(ctx context.Context, tx pgx.Tx, staff, permission string) error {
	var id string
	return tx.QueryRow(ctx, currentGrant, staff, permission).Scan(&id)
}

// AuthorizeStaff requires a current native read grant for the kind, on top of
// the delegated policy and the signed owner context.
func (s *SourceReader) AuthorizeStaff(ctx context.Context, tx pgx.Tx, staff, kind, _ string) error {
	permission := readPermission[kind]
	if permission == "" {
		return pgx.ErrNoRows
	}
	return requireGrant(ctx, tx, staff, permission)
}

// RequireMutationGrant checks a native mutation grant inside the command tx
// and share-locks it, so a concurrent revocation waits for the decision.
func RequireMutationGrant(ctx context.Context, tx pgx.Tx, staff, permission string) error {
	var id string
	return tx.QueryRow(ctx, currentGrant+` FOR SHARE`, staff, permission).Scan(&id)
}

// Runtime builds the owner read/context handler. Everything is default off.
func Runtime(ctx context.Context, db *pgxpool.Pool) (*adminowner.Handler, error) {
	p := adminowner.VerificationProfile()
	return adminowner.FromEnvironment(ctx, db, Source(), p.Definitions, Owner, p.Prefix, p.Path)
}

var _ adminowner.StaffAuthorizer = (*SourceReader)(nil)
