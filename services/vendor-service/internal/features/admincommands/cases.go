package admincommands

import (
	"context"
	"cosmicforge/logistics/shared/go/admincommand"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Case transitions record investigations and appeals only. An appeal decision
// never changes a listing, verification, stock, money, mission or custody state.
func applyCase(ctx context.Context, tx pgx.Tx, in admincommand.Execution, business, branch string) (admincommand.Transition, error) {
	var out admincommand.Transition
	var b, br, kind, id, state string
	var version int64
	// Lock the real business first to serialize creation of an absent case.
	if _, err := tx.Exec(ctx, `SELECT id FROM vendor_businesses WHERE id=$1 FOR UPDATE`, business); err != nil {
		return out, err
	}
	err := tx.QueryRow(ctx, `SELECT business_id,COALESCE(branch_id,''),subject_kind,subject_id,state,version FROM vendor_admin_cases WHERE case_ref=$1 FOR UPDATE`, in.CaseRef).Scan(&b, &br, &kind, &id, &state, &version)
	exists := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	if exists && (b != business || br != branch || kind != in.SubjectKind || id != in.SubjectID) {
		return out, admincommand.ErrNotFound
	}
	if version != in.ExpectedVersion {
		return out, admincommand.ErrConflict
	}
	next := ""
	switch in.Action {
	case "OPEN_INVESTIGATION":
		if exists {
			return out, admincommand.ErrConflict
		}
		next = "OPEN"
	case "ESCALATE_INVESTIGATION":
		if state != "OPEN" && state != "REVIEWED" {
			return out, admincommand.ErrRejected
		}
		next = "ESCALATED"
	case "RECORD_FINDING":
		if state != "OPEN" && state != "ESCALATED" {
			return out, admincommand.ErrRejected
		}
		next = "REVIEWED"
	case "RECORD_SELLER_APPEAL":
		if state != "REVIEWED" {
			return out, admincommand.ErrRejected
		}
		next = "APPEALED"
	case "DECIDE_SELLER_APPEAL":
		if state != "APPEALED" {
			return out, admincommand.ErrRejected
		}
		next = "APPEAL_DECIDED"
	case "CLOSE_INVESTIGATION":
		if state != "REVIEWED" && state != "APPEAL_DECIDED" {
			return out, admincommand.ErrRejected
		}
		next = "CLOSED"
	default:
		return out, admincommand.ErrRejected
	}
	if !exists {
		_, err = tx.Exec(ctx, `INSERT INTO vendor_admin_cases(case_ref,business_id,branch_id,subject_kind,subject_id,category,severity,state,version,opened_by) VALUES($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,1,$9)`, in.CaseRef, business, branch, in.SubjectKind, in.SubjectID, in.Params["category"], in.Params["severity"], next, in.Staff)
		state = "NONE"
	} else {
		_, err = tx.Exec(ctx, `UPDATE vendor_admin_cases SET state=$2,version=version+1,updated_at=now() WHERE case_ref=$1 AND version=$3`, in.CaseRef, next, version)
	}
	if err != nil {
		return out, err
	}
	raw, err := json.Marshal(in.Params)
	if err != nil {
		return out, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO vendor_admin_case_history(id,case_ref,operation_id,action,prior_state,resulting_state,prior_version,resulting_version,staff_ref,reason_code,parameters) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, uuid.NewString(), in.CaseRef, in.OperationID, in.Action, state, next, version, version+1, in.Staff, in.ReasonCode, raw)
	return admincommand.Transition{PriorVersion: version, PriorState: state, ResultingState: next, Outcome: "OWNER_CASE_TRANSITION_RECORDED"}, err
}
