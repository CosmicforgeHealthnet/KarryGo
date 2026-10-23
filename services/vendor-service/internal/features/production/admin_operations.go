package production

import (
	"context"
	"cosmicforge/logistics/shared/go/apperrors"
	"github.com/jackc/pgx/v5"
	"strings"
)

var nativeJobTransitions = map[string]map[string]bool{"PLANNED": {"QUEUED": true, "IN_PROGRESS": true, "CANCELLED": true}, "QUEUED": {"IN_PROGRESS": true, "BLOCKED": true, "CANCELLED": true}, "IN_PROGRESS": {"BLOCKED": true, "READY": true, "CANCELLED": true}, "BLOCKED": {"IN_PROGRESS": true, "CANCELLED": true}, "READY": {"COMPLETED": true}, "COMPLETED": {}, "CANCELLED": {}}

func NativeJobTransitionAllowed(from, to string) bool { return nativeJobTransitions[from][to] }

// AdministrativeInterruptionTx supports only native blocking/resumption. No
// quantity, capacity, readiness evidence, handover or completion input exists.
func (s *Service) AdministrativeInterruptionTx(ctx context.Context, tx pgx.Tx, actor Actor, scope Scope, id string, resume bool, reason string, expected int64) (int64, string, string, error) {
	var branch, state string
	var version int64
	if err := s.requireWrites(); err != nil {
		return 0, "", "", err
	}
	if err := tx.QueryRow(ctx, `SELECT branch_id,status,version FROM production_jobs WHERE business_id=$1 AND id=$2 FOR UPDATE`, actor.VendorID, id).Scan(&branch, &state, &version); err != nil {
		return 0, "", "", err
	}
	if !scope.Covers(branch) {
		return 0, "", "", pgx.ErrNoRows
	}
	if version != expected {
		return 0, "", "", errStaleVersion()
	}
	next := "BLOCKED"
	if resume {
		next = "IN_PROGRESS"
		if state != "BLOCKED" {
			return 0, "", "", apperrors.Conflict("Production is not blocked.", nil)
		}
	}
	if !NativeJobTransitionAllowed(state, next) || strings.TrimSpace(reason) == "" {
		return 0, "", "", apperrors.Conflict("Native production transition unavailable.", nil)
	}
	if _, err := tx.Exec(ctx, `UPDATE production_jobs SET status=$3,blocked_reason=CASE WHEN $3='BLOCKED' THEN $4 ELSE '' END,actual_start=CASE WHEN $3='IN_PROGRESS' THEN COALESCE(actual_start,now()) ELSE actual_start END,version=version+1,updated_at=now() WHERE business_id=$1 AND id=$2 AND version=$5`, actor.VendorID, id, next, reason, expected); err != nil {
		return 0, "", "", err
	}
	event := "production.job." + strings.ToLower(next)
	nativeEvent := "BLOCKED"
	if resume {
		nativeEvent = "STARTED"
	}
	if _, err := tx.Exec(ctx, `INSERT INTO production_job_events(id,business_id,job_id,event_type,from_status,to_status,note,actor_id,correlation_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,''))`, newID("pje_"), actor.VendorID, id, nativeEvent, state, next, reason, actor.StaffID, actor.CorrelationID); err != nil {
		return 0, "", "", err
	}
	err := s.RecordMutation(ctx, tx, actor, event, event, "production_job", id, branch, version+1, map[string]any{"status": state}, map[string]any{"status": next}, map[string]any{"status": next}, reason)
	return version, state, next, err
}
