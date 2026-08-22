package inventory

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
)

// PostgresEvidenceRecorder writes generic Inventory evidence to the existing
// Vendor audit authority inside the stock mutation transaction.
type PostgresEvidenceRecorder struct{}

func NewPostgresEvidenceRecorder() PostgresEvidenceRecorder {
	return PostgresEvidenceRecorder{}
}

func (PostgresEvidenceRecorder) RecordInventoryEvidence(
	ctx context.Context,
	tx InventoryTransaction,
	evidence Evidence,
) error {
	postgresTx, ok :=
		tx.(PostgresTransactionAccessor)

	if !ok {
		return ErrDependencyMissing
	}

	branchID := ""

	if evidence.Scope.BranchID != nil {
		branchID = *evidence.Scope.BranchID
	}

	actorType := "vendor_staff"

	if evidence.Actor.StaffID == "" {
		actorType = "system"
	}

	metadata, err :=
		json.Marshal(evidence.Metadata)

	if err != nil {
		return err
	}

	_, err = postgresTx.PGX().Exec(
		ctx,
		`
        INSERT INTO vendor_audit_log (
            id,
            vendor_id,
            actor_type,
            actor_id,
            action,
            aggregate_type,
            aggregate_id,
            branch_id,
            correlation_id,
            after_data,
            reason
        )
        VALUES (
            $1,$2,$3,NULLIF($4,''),
            $5,$6,$7,NULLIF($8,''),
            NULLIF($9,''),$10,NULLIF($11,'')
        )
        `,
		"val_"+uuid.NewString(),
		evidence.Actor.BusinessID,
		actorType,
		evidence.Actor.Identity(),
		evidence.Action,
		evidence.AggregateType,
		evidence.AggregateRef,
		branchID,
		evidence.Actor.CorrelationID,
		json.RawMessage(metadata),
		evidence.ReasonCode,
	)

	return err
}
