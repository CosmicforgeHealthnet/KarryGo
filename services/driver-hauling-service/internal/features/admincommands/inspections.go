package admincommands

import (
	"context"
	"errors"
	"time"

	"cosmicforge/logistics/shared/go/admincommand"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Inspection case state machine (VersionOf CASE, case_ref = inspection id):
//
//	(none) --RECORD_INSPECTION--> RECORDED --ATTACH_INSPECTION_EVIDENCE--> RECORDED
//	RECORDED --CONCLUDE_INSPECTION(PASS|FAIL|CONDITIONAL)--> CONCLUDED (terminal)
//
// FAIL restricts the truck through the authority restriction ledger under the
// truck's owning organization. PASS never lifts any restriction.
const (
	inspectionRecorded  = "RECORDED"
	inspectionConcluded = "CONCLUDED"
)

type inspectionRow struct {
	id, truckID, providerID, organization, state string
	performedOn                                  time.Time
	version                                      int64
}

func (e *Executor) recordInspection(ctx context.Context, tx pgx.Tx, in admincommand.Execution) (admincommand.Transition, error) {
	organization, provider, err := ownerOrganization(ctx, tx, subjectAsset, in.SubjectID)
	if err != nil {
		return admincommand.Transition{}, err
	}
	if err := requireTenant(in, organization); err != nil {
		return admincommand.Transition{}, err
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM hauling_admin_inspections WHERE id=$1)`, in.CaseRef).Scan(&exists); err != nil {
		return admincommand.Transition{}, err
	}
	if exists || in.ExpectedVersion != 0 {
		// A case is created exactly once, at expected version 0.
		return admincommand.Transition{}, admincommand.ErrConflict
	}
	performed, _ := time.Parse("2006-01-02", in.Params["performed_on"])
	if performed.After(in.Now.UTC()) {
		return admincommand.Transition{}, admincommand.ErrRejected
	}
	evidence := 0
	if in.Params["media_object_id"] != "" {
		evidence = 1
	}
	_, err = tx.Exec(ctx, `INSERT INTO hauling_admin_inspections(id,truck_id,provider_id,organization_id,inspection_kind,performed_on,state,evidence_count,version,recorded_decision_id) VALUES($1,$2::uuid,$3::uuid,$4,$5,$6,'RECORDED',$7,1,$8)`,
		in.CaseRef, in.SubjectID, provider, organization, in.Params["inspection_kind"], performed, evidence, in.DecisionID)
	if isUnique(err) {
		return admincommand.Transition{}, admincommand.ErrConflict
	}
	if err != nil {
		return admincommand.Transition{}, err
	}
	if evidence == 1 {
		if _, err := tx.Exec(ctx, `INSERT INTO hauling_admin_inspection_evidence(inspection_id,media_object_id,attached_by) VALUES($1,$2::uuid,$3)`, in.CaseRef, in.Params["media_object_id"], in.DecisionID); err != nil {
			return admincommand.Transition{}, err
		}
	}
	if err := inspectionHistory(ctx, tx, in, 1, inspectionRecorded, ""); err != nil {
		return admincommand.Transition{}, err
	}
	return admincommand.Transition{PriorVersion: 0, PriorState: "NONE", ResultingState: inspectionRecorded, Outcome: "INSPECTION_RECORDED"}, nil
}

// lockInspection loads the case for the subject truck, enforces the case
// version fence and re-proves the CURRENT ownership binding: a truck that was
// transferred or whose organization link lapsed cannot be acted on through a
// case recorded under the old owner.
func lockInspection(ctx context.Context, tx pgx.Tx, in admincommand.Execution) (inspectionRow, error) {
	var r inspectionRow
	err := tx.QueryRow(ctx, `SELECT id,truck_id::text,provider_id::text,organization_id,state,performed_on,version FROM hauling_admin_inspections WHERE id=$1 FOR UPDATE`, in.CaseRef).
		Scan(&r.id, &r.truckID, &r.providerID, &r.organization, &r.state, &r.performedOn, &r.version)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && r.truckID != in.SubjectID {
		return r, admincommand.ErrNotFound
	}
	if err != nil {
		return r, err
	}
	if r.version != in.ExpectedVersion {
		return r, admincommand.ErrConflict
	}
	organization, provider, err := ownerOrganization(ctx, tx, subjectAsset, in.SubjectID)
	if err != nil {
		return r, err
	}
	if organization != r.organization || provider != r.providerID {
		return r, admincommand.ErrRejected
	}
	return r, requireTenant(in, r.organization)
}

func (e *Executor) attachEvidence(ctx context.Context, tx pgx.Tx, in admincommand.Execution) (admincommand.Transition, error) {
	r, err := lockInspection(ctx, tx, in)
	if err != nil {
		return admincommand.Transition{}, err
	}
	if r.state != inspectionRecorded {
		return admincommand.Transition{}, admincommand.ErrRejected
	}
	tag, err := tx.Exec(ctx, `INSERT INTO hauling_admin_inspection_evidence(inspection_id,media_object_id,attached_by) VALUES($1,$2::uuid,$3) ON CONFLICT DO NOTHING`, r.id, in.Params["media_object_id"], in.DecisionID)
	if err != nil {
		return admincommand.Transition{}, err
	}
	if tag.RowsAffected() != 1 {
		// The association already exists; it is immutable and never re-attested.
		return admincommand.Transition{}, admincommand.ErrRejected
	}
	if _, err := tx.Exec(ctx, `UPDATE hauling_admin_inspections SET evidence_count=evidence_count+1,version=version+1,updated_at=now() WHERE id=$1`, r.id); err != nil {
		return admincommand.Transition{}, err
	}
	if err := inspectionHistory(ctx, tx, in, r.version+1, inspectionRecorded, ""); err != nil {
		return admincommand.Transition{}, err
	}
	return admincommand.Transition{PriorVersion: r.version, PriorState: inspectionRecorded, ResultingState: inspectionRecorded, Outcome: "EVIDENCE_ATTACHED"}, nil
}

func (e *Executor) concludeInspection(ctx context.Context, tx pgx.Tx, in admincommand.Execution) (admincommand.Transition, error) {
	r, err := lockInspection(ctx, tx, in)
	if err != nil {
		return admincommand.Transition{}, err
	}
	if r.state != inspectionRecorded {
		return admincommand.Transition{}, admincommand.ErrRejected
	}
	conclusion := in.Params["conclusion"]
	var validUntil *time.Time
	if raw := in.Params["valid_until"]; raw != "" {
		value, _ := time.Parse("2006-01-02", raw)
		if conclusion == "FAIL" || value.Before(r.performedOn) {
			// A failed inspection grants no validity window, and a window
			// cannot end before the inspection was performed.
			return admincommand.Transition{}, admincommand.ErrRejected
		}
		validUntil = &value
	}
	var restriction *string
	outcome := "INSPECTION_PASSED"
	switch conclusion {
	case "CONDITIONAL":
		outcome = "INSPECTION_CONDITIONAL"
	case "FAIL":
		outcome = "INSPECTION_FAILED_TRUCK_RESTRICTED"
		id, err := applyRestriction(ctx, tx, in, r.organization, subjectAsset, r.truckID, "INSPECTION_FAILED", adminSourcePrefix+"inspection:"+r.id)
		if err != nil {
			return admincommand.Transition{}, err
		}
		restriction = &id
		if err := bumpHead(ctx, tx, "trucks", r.truckID, stateRestricted, r.organization, in.DecisionID); err != nil {
			return admincommand.Transition{}, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE hauling_admin_inspections SET state='CONCLUDED',conclusion=$2,valid_until=$3,restriction_id=$4::uuid,concluded_decision_id=$5,version=version+1,updated_at=now() WHERE id=$1`, r.id, conclusion, validUntil, restriction, in.DecisionID); err != nil {
		return admincommand.Transition{}, err
	}
	if err := inspectionHistory(ctx, tx, in, r.version+1, inspectionConcluded, conclusion); err != nil {
		return admincommand.Transition{}, err
	}
	return admincommand.Transition{PriorVersion: r.version, PriorState: inspectionRecorded, ResultingState: inspectionConcluded, Outcome: outcome}, nil
}

func inspectionHistory(ctx context.Context, tx pgx.Tx, in admincommand.Execution, version int64, state, conclusion string) error {
	var value any
	if conclusion != "" {
		value = conclusion
	}
	_, err := tx.Exec(ctx, `INSERT INTO hauling_admin_inspection_history(inspection_id,version,action,state,conclusion,decision_id) VALUES($1,$2,$3,$4,$5,$6)`, in.CaseRef, version, in.Action, state, value, in.DecisionID)
	return err
}

func isUnique(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}
