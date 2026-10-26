package vendors

import (
	"context"
	"cosmicforge/logistics/shared/go/apperrors"
	"errors"
	"github.com/jackc/pgx/v5"
)

// AdministrativeBranchDomainEligibility applies the existing operational and
// verification reducer to the branch capability that can fulfil a new order.
// Historical merchants without an onboarding profile retain their legacy
// booking contract; once onboarding starts, a current eligible capability is
// mandatory. The checkout transaction holds the capability and verification
// evidence with share locks so a concurrent restriction cannot be missed.
func AdministrativeBranchDomainEligibility(ctx context.Context, tx pgx.Tx, businessID, branchID, domain string) (bool, string, error) {
	var onboardingStatus string
	err := tx.QueryRow(ctx, `SELECT status FROM vendor_onboarding_profiles WHERE business_id=$1 FOR SHARE`, businessID).Scan(&onboardingStatus)
	if errors.Is(err, pgx.ErrNoRows) || onboardingStatus == "NOT_STARTED" {
		return true, "LEGACY_ONBOARDING_NOT_STARTED", nil
	}
	if err != nil {
		return false, "VERIFICATION_STATE_UNAVAILABLE", err
	}

	family := domain
	if family != "FOOD" && family != "SHOP" {
		return false, "CAPABILITY_FAMILY_UNAVAILABLE", nil
	}
	rows, err := tx.Query(ctx, `
		SELECT bc.id
		  FROM vendor_branch_capabilities bc
		  JOIN vendor_capabilities c ON c.id=bc.vendor_capability_id AND c.business_id=bc.business_id
		 WHERE bc.business_id=$1 AND bc.branch_id=$2
		   AND CASE WHEN bc.capability_type IN ('RESTAURANT','FAST_FOOD','CLOUD_KITCHEN','PASTRY_BAKERY','CATERING') THEN 'FOOD'
		            WHEN bc.capability_type IN ('SHOP_GROCERY','MARKET','WHOLESALE') THEN 'SHOP'
		            ELSE '' END=$3
		 ORDER BY bc.id FOR SHARE`, businessID, branchID, family)
	if err != nil {
		return false, "CAPABILITY_STATE_UNAVAILABLE", err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return false, "CAPABILITY_STATE_UNAVAILABLE", err
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return false, "CAPABILITY_STATE_UNAVAILABLE", err
	}
	rows.Close()
	if len(ids) == 0 {
		return false, "CAPABILITY_NOT_CONFIGURED", nil
	}

	// Lock the current verification evidence before reducing it. This uses the
	// same dimensions as verificationEffectiveWith and never creates or changes
	// a Verification-owned decision.
	verificationRows, err := tx.Query(ctx, `
		SELECT p.id FROM vendor_verification_projections p
		 WHERE p.business_id=$1 AND (
			(p.dimension IN ('IDENTITY','BUSINESS') AND p.subject_type IN ('BUSINESS','REPRESENTATIVE')) OR
			(p.dimension='LOCATION' AND p.subject_type='BRANCH' AND p.subject_ref=$2) OR
			(p.dimension='FOOD_KITCHEN' AND p.subject_type='BRANCH_CAPABILITY' AND p.subject_ref=ANY($3::text[]))
		 ) ORDER BY p.id FOR SHARE`, businessID, branchID, ids)
	if err != nil {
		return false, "VERIFICATION_STATE_UNAVAILABLE", err
	}
	for verificationRows.Next() {
		var ignored string
		if err = verificationRows.Scan(&ignored); err != nil {
			verificationRows.Close()
			return false, "VERIFICATION_STATE_UNAVAILABLE", err
		}
	}
	if err = verificationRows.Err(); err != nil {
		verificationRows.Close()
		return false, "VERIFICATION_STATE_UNAVAILABLE", err
	}
	verificationRows.Close()

	lastReason := "VERIFICATION_REQUIREMENTS_NOT_SATISFIED"
	for _, id := range ids {
		allowed, reason, eligibilityErr := AdministrativeBranchCapabilityEligibility(ctx, tx, id)
		if eligibilityErr != nil {
			return false, reason, eligibilityErr
		}
		if allowed {
			return true, reason, nil
		}
		lastReason = reason
	}
	return false, lastReason, nil
}

// AdministrativeOperationalStatusTx uses the native ACTIVE/PAUSED state
// machine. It cannot activate DRAFT, SUSPENDED or verification-owned states.
// Caller supplies the source transaction that also commits its command receipt.
func (s *Service) AdministrativeOperationalStatusTx(ctx context.Context, tx pgx.Tx, actor Actor, branch string, reopen bool, expected int64) (int64, string, string, error) {
	table, id, aggregate, event := "vendor_businesses", actor.VendorID, "vendor_business", "vendor.business.status_changed"
	permission := "vendor.business.manage"
	if branch != "" {
		table, id, aggregate, event = "vendor_branches", branch, "vendor_branch", "vendor.branch.status_changed"
		permission = "vendor.branches.manage"
	}
	// This operation is invoked while the secure Admin command owns a source
	// transaction and a per-subject advisory lock. Re-entering the ordinary
	// pool-backed authorizer from here can exhaust a small pool when identical
	// commands arrive concurrently: every waiter holds a transaction while the
	// lock holder waits for another connection. Resolve and lock the same native
	// grant through the caller's transaction instead. The Admin executor also
	// performs its own source authorization; this check deliberately preserves
	// the Vendor service's independent deny-by-default boundary.
	if err := requireAdministrativePermissionTx(ctx, tx, actor, branch, permission); err != nil {
		return 0, "", "", err
	}
	var current string
	var version int64
	query := `SELECT status,version FROM ` + table + ` WHERE id=$1`
	args := []any{id}
	if branch != "" {
		query += ` AND business_id=$2`
		args = append(args, actor.VendorID)
	}
	if err := tx.QueryRow(ctx, query+` FOR UPDATE`, args...).Scan(&current, &version); err != nil {
		return 0, "", "", err
	}
	if expected != version {
		return 0, "", "", conflict(nil)
	}
	next := "PAUSED"
	if reopen {
		next = "ACTIVE"
	}
	allowed := vendorBusinessTransitionAllowed(BusinessStatus(current), BusinessStatus(next))
	if branch != "" {
		allowed = branchTransitionAllowed(BranchStatus(current), BranchStatus(next))
	}
	if !allowed {
		return 0, "", "", apperrors.Conflict("Native operational transition unavailable.", nil)
	}
	if reopen {
		rows, err := tx.Query(ctx, `SELECT id,business_id,branch_id,vendor_capability_id,capability_type FROM vendor_branch_capabilities WHERE business_id=$1 AND ($2='' OR branch_id=$2)`, actor.VendorID, branch)
		if err != nil {
			return 0, "", "", err
		}
		caps := []BranchCapability{}
		for rows.Next() {
			var c BranchCapability
			if err = rows.Scan(&c.ID, &c.BusinessID, &c.BranchID, &c.VendorCapabilityID, &c.CapabilityType); err != nil {
				rows.Close()
				return 0, "", "", err
			}
			caps = append(caps, c)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return 0, "", "", err
		}
		if len(caps) == 0 {
			return 0, "", "", apperrors.Conflict("Reopening requires native operational capability records.", nil)
		}
		for _, c := range caps {
			if okay, _ := verificationEffectiveWith(ctx, tx, c); !okay {
				return 0, "", "", apperrors.Conflict("Verification authority has not cleared reopening.", nil)
			}
		}
	}
	tag, err := tx.Exec(ctx, `UPDATE `+table+` SET status=$2,version=version+1,updated_at=now() WHERE id=$1 AND version=$3 AND status=$4`, id, next, expected, current)
	if err != nil {
		return 0, "", "", err
	}
	if tag.RowsAffected() != 1 {
		return 0, "", "", errors.New("native operational version conflict")
	}
	if err = s.publish(ctx, tx, event, aggregate, id, version+1, actor, map[string]any{"status": next}); err != nil {
		return 0, "", "", err
	}
	return version, current, next, nil
}

func requireAdministrativePermissionTx(ctx context.Context, tx pgx.Tx, actor Actor, branch, permission string) error {
	if actor.StaffID == "" || actor.VendorID == "" || permission == "" {
		return apperrors.Unauthorized("Authentication is required.", nil)
	}
	var staff string
	err := tx.QueryRow(ctx, `SELECT s.id FROM vendor_staff s
 JOIN vendor_staff_role_assignments a ON a.staff_id=s.id
 JOIN vendor_role_permissions rp ON rp.role_id=a.role_id
 JOIN vendor_permissions p ON p.id=rp.permission_id
 WHERE s.id=$1 AND s.vendor_id=$2 AND s.status='active' AND a.revoked_at IS NULL
 AND p.code=$4 AND (a.branch_id IS NULL OR ($3<>'' AND a.branch_id=$3))
 ORDER BY a.id LIMIT 1 FOR SHARE OF s,a,rp`, actor.StaffID, actor.VendorID, branch, permission).Scan(&staff)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.Forbidden("You do not have access to this action.", nil)
	}
	return err
}

// AdministrativeBranchCapabilityEligibility uses the same native operational
// and verification reducer as merchant-facing reads; status alone is not a grant.
func AdministrativeBranchCapabilityEligibility(ctx context.Context, tx pgx.Tx, id string) (bool, string, error) {
	c, err := scanBranchCapability(tx.QueryRow(ctx, `SELECT `+branchCapabilityCols+` FROM vendor_branch_capabilities bc JOIN vendor_businesses vb ON vb.id=bc.business_id JOIN vendor_branches b ON b.id=bc.branch_id AND b.business_id=bc.business_id JOIN vendor_capabilities c ON c.id=bc.vendor_capability_id AND c.business_id=bc.business_id WHERE bc.id=$1`, id))
	if err != nil {
		return false, "STATE_UNAVAILABLE", err
	}
	if !c.Effective {
		return false, c.EffectiveReason, nil
	}
	allowed, reason := verificationEffectiveWith(ctx, tx, c)
	return allowed, reason, nil
}
