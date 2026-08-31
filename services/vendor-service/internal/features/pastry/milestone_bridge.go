package pastry

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/vendor-service/internal/features/production"
)

// SyncProductionProgress is the narrow reverse adapter for custom-cake work.
// Production status is evidence, but Pastry remains the milestone authority.
// Only the explicitly mapped final production states can advance the READY
// milestone; generic job steps and intermediate states never alter commercial
// change/cancellation eligibility.
func (s *Service) SyncProductionProgress(ctx context.Context, tx pgx.Tx, actor production.Actor, jobID, status string) error {
	if s == nil || tx == nil {
		return nil
	}
	status = strings.ToUpper(strings.TrimSpace(status))
	if status != "READY" && status != "COMPLETED" {
		return nil
	}
	var requestID, branch string
	if err := tx.QueryRow(ctx, `
		SELECT q.request_id,r.branch_id
		  FROM production_jobs j
		  JOIN pastry_quotes q ON q.id=j.source_ref
		  JOIN pastry_custom_requests r ON r.id=q.request_id
		 WHERE j.id=$1 AND j.business_id=$2 AND j.source_type='CUSTOM_CAKE_QUOTE'`, jobID, actor.VendorID).Scan(&requestID, &branch); err != nil {
		if err == pgx.ErrNoRows {
			return nil
		}
		return err
	}
	var milestoneID, current string
	var version int64
	if err := tx.QueryRow(ctx, `
		SELECT id,state,version
		  FROM pastry_milestones
		 WHERE request_id=$1 AND milestone_type='READY'
		 FOR UPDATE`, requestID).Scan(&milestoneID, &current, &version); err != nil {
		if err == pgx.ErrNoRows {
			return nil
		}
		return err
	}
	if current == "COMPLETED" || current == "CANCELLED" || (status == "READY" && current == "READY") {
		return nil
	}
	// A production job cannot skip the Pastry milestone sequence. If an
	// operator has not explicitly completed the preceding milestones, retain
	// the production truth and leave commercial gating unchanged.
	var incomplete bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM pastry_milestones
			 WHERE request_id=$1 AND position < 5
			   AND state NOT IN ('COMPLETED','READY'))`, requestID).Scan(&incomplete); err != nil {
		return err
	}
	if incomplete {
		return nil
	}
	target := status
	if target == "COMPLETED" && current != "ACTIVE" && current != "READY" {
		// The existing Pastry transition authority requires an ACTIVE/READY
		// milestone before COMPLETED; do not manufacture an out-of-order
		// commercial milestone from a pending or blocked row.
		return nil
	}
	if current == target {
		return nil
	}
	if _, err := tx.Exec(ctx, `UPDATE pastry_milestones SET state=$1,version=version+1 WHERE id=$2 AND version=$3`, target, milestoneID, version); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO pastry_milestone_history(id,milestone_id,from_state,to_state,actor_id,correlation_id,note) VALUES($1,$2,$3,$4,NULLIF($5,''),NULLIF($6,''),$7)`, "pmh_"+uuid.NewString(), milestoneID, current, target, actor.StaffID, actor.CorrelationID, "mapped from custom-cake production job"); err != nil {
		return err
	}
	if s.production != nil {
		return s.production.RecordMutation(ctx, tx, actor,
			"pastry.milestone."+strings.ToLower(target),
			"pastry.milestone."+strings.ToLower(target), "pastry_milestone", milestoneID, branch, version+1,
			map[string]any{"state": current},
			map[string]any{"state": target},
			map[string]any{"request_id": requestID, "milestone": "READY", "production_job_id": jobID}, "")
	}
	return nil
}

var _ production.PastryMilestoneBridge = (*Service)(nil)
