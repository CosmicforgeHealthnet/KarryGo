package vendors

import (
	"context"
	"cosmicforge/logistics/shared/go/apperrors"
	"errors"
	"github.com/jackc/pgx/v5"
)

// AdministrativeOperationalStatusTx uses the native ACTIVE/PAUSED state
// machine. It cannot activate DRAFT, SUSPENDED or verification-owned states.
// Caller supplies the source transaction that also commits its command receipt.
func (s *Service) AdministrativeOperationalStatusTx(ctx context.Context, tx pgx.Tx, actor Actor, branch string, reopen bool, expected int64) (int64, string, string, error) {
	table, id, aggregate, event := "vendor_businesses", actor.VendorID, "vendor_business", "vendor.business.status_changed"
	permission := "vendor.business.manage"
	var target *string
	if branch != "" {
		table, id, aggregate, event = "vendor_branches", branch, "vendor_branch", "vendor.branch.status_changed"
		permission = "vendor.branches.manage"
		target = &branch
	}
	if err := s.require(ctx, actor, permission, target); err != nil {
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
