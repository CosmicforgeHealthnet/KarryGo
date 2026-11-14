package fleetpartner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
)

const settlementInstructionColumns = `instruction_id,organization_id,membership_id,provider_id,source_service,source_job_id,
	source_assignment_id,compensation_plan_id,compensation_plan_version,compensation_resolution_version,
	gross_basis_minor,provider_share_minor,organization_share_minor,platform_share_minor,currency,
	visibility_policy_snapshot,status,idempotency_key,request_fingerprint,version,created_at,
	wallet_posting_operation_id,wallet_settlement_reference,delivery_attempts,last_error_code,posted_at,updated_at`

func scanSettlementInstruction(row pgx.Row) (ManagedWorkSettlementInstruction, error) {
	var in ManagedWorkSettlementInstruction
	var visibility []byte
	err := row.Scan(&in.InstructionID, &in.OrganizationID, &in.MembershipID, &in.ProviderID,
		&in.SourceService, &in.SourceJobID, &in.SourceAssignmentID, &in.CompensationPlanID,
		&in.CompensationPlanVersion, &in.CompensationResolutionVersion, &in.GrossBasisMinor,
		&in.ProviderShareMinor, &in.OrganizationShareMinor, &in.PlatformShareMinor, &in.Currency,
		&visibility, &in.Status, &in.IdempotencyKey, &in.RequestFingerprint, &in.Version, &in.CreatedAt,
		&in.WalletPostingOperationID, &in.WalletSettlementReference, &in.DeliveryAttempts,
		&in.LastErrorCode, &in.PostedAt, &in.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ManagedWorkSettlementInstruction{}, ErrNotFound
	}
	if err != nil {
		return ManagedWorkSettlementInstruction{}, fmt.Errorf("fleetpartner: scan settlement instruction: %w", err)
	}
	if len(visibility) > 0 {
		_ = json.Unmarshal(visibility, &in.VisibilityPolicySnapshot)
	}
	return in, nil
}

// SettlementInstruction reads one instruction, always scoped to its
// organization. forUpdate locks the row for the caller's transaction.
func (r *Repository) SettlementInstruction(ctx context.Context, q database.Querier, organizationID, instructionID string, forUpdate bool) (ManagedWorkSettlementInstruction, error) {
	sql := `SELECT ` + settlementInstructionColumns + ` FROM fleet_partner_managed_work_settlement_instructions WHERE organization_id=$1 AND instruction_id=$2`
	if forUpdate {
		sql += ` FOR UPDATE`
	}
	return scanSettlementInstruction(r.q(q).QueryRow(ctx, sql, organizationID, instructionID))
}

// RecordSettlementDelivery stores one delivery outcome on a locked row.
//
// The version column is NOT incremented: it is the instruction version Wallet
// keys its posting on, and an instruction does not change by being delivered.
// Bumping it would make a retry look like a different instruction.
func (r *Repository) RecordSettlementDelivery(ctx context.Context, q database.Querier, current ManagedWorkSettlementInstruction, status, operationID, reference, errorCode string) (ManagedWorkSettlementInstruction, error) {
	return scanSettlementInstruction(r.q(q).QueryRow(ctx, `
		UPDATE fleet_partner_managed_work_settlement_instructions
		SET status=$3,
		    wallet_posting_operation_id=COALESCE(NULLIF($4,''),wallet_posting_operation_id),
		    wallet_settlement_reference=COALESCE(NULLIF($5,''),wallet_settlement_reference),
		    last_error_code=$6,
		    delivery_attempts=delivery_attempts+1,
		    posted_at=CASE WHEN $3='POSTED' THEN now() ELSE posted_at END,
		    updated_at=now()
		WHERE organization_id=$1 AND instruction_id=$2
		RETURNING `+settlementInstructionColumns,
		current.OrganizationID, current.InstructionID, status, operationID, reference, errorCode))
}

// LinkWorkSettledComponentsToPosting records which posting operation paid the
// components an instruction settled, so payroll's exclusion rows point at
// the money movement and not only at the decision.
func (r *Repository) LinkWorkSettledComponentsToPosting(ctx context.Context, q database.Querier, organizationID, instructionID, operationID string) error {
	_, err := r.q(q).Exec(ctx, `
		UPDATE fleet_partner_work_settled_components
		SET wallet_posting_operation_id=$3
		WHERE organization_id=$1 AND settlement_instruction_id=$2 AND wallet_posting_operation_id=''`,
		organizationID, instructionID, operationID)
	if err != nil {
		return fmt.Errorf("fleetpartner: link settled components: %w", err)
	}
	return nil
}

// UndeliveredInstruction identifies one instruction the sweep must deliver.
type UndeliveredInstruction struct{ OrganizationID, InstructionID string }

// UndeliveredSettlementInstructions lists instructions that are not final and
// have no live delivery job.
func (r *Repository) UndeliveredSettlementInstructions(ctx context.Context, jobType string, limit int) ([]UndeliveredInstruction, error) {
	rows, err := r.db.Query(ctx, `
		SELECT i.organization_id, i.instruction_id
		FROM fleet_partner_managed_work_settlement_instructions i
		WHERE i.status IN ('CREATED','PENDING_WALLET','RECONCILIATION_REQUIRED')
		  AND NOT EXISTS (
		    SELECT 1 FROM fleet_partner_jobs j
		    WHERE j.organization_id = i.organization_id AND j.job_type = $1
		      AND j.dedupe_key = 'settlement-instruction:' || i.instruction_id
		      AND j.status IN ('PENDING','RETRY_SCHEDULED','CLAIMED','RUNNING'))
		ORDER BY i.created_at
		LIMIT $2`, jobType, limit)
	if err != nil {
		return nil, fmt.Errorf("fleetpartner: list undelivered instructions: %w", err)
	}
	defer rows.Close()
	var out []UndeliveredInstruction
	for rows.Next() {
		var u UndeliveredInstruction
		if err := rows.Scan(&u.OrganizationID, &u.InstructionID); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
