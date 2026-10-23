package production

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var stepTransitions = map[string]map[string]bool{
	"PENDING":     {"READY": true, "CANCELLED": true},
	"READY":       {"IN_PROGRESS": true, "SKIPPED": true, "CANCELLED": true},
	"IN_PROGRESS": {"BLOCKED": true, "COMPLETED": true, "CANCELLED": true},
	"BLOCKED":     {"IN_PROGRESS": true, "CANCELLED": true},
	"COMPLETED":   {}, "SKIPPED": {}, "CANCELLED": {},
}

type StepInput struct {
	ProductionLineID   *string `json:"production_line_id"`
	SequenceNo         int     `json:"sequence_no"`
	Phase              int     `json:"phase"`
	Name               string  `json:"name"`
	Required           bool    `json:"required"`
	ExpectedJobVersion int64   `json:"expected_job_version"`
}

type StepChange struct {
	ExpectedVersion int64  `json:"expected_version"`
	Status          string `json:"status"`
	BlockedReason   string `json:"blocked_reason"`
	Note            string `json:"note"`
}

// CreateStep snapshots one station step onto a job. Production lines remain
// the station authority; a step only references an existing line.
func (s *Service) CreateStep(ctx context.Context, actor Actor, jobID string, in StepInput) (string, error) {
	if strings.TrimSpace(in.Name) == "" || in.SequenceNo < 0 || in.Phase < 0 {
		return "", validation()
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var branch string
	var jobVersion int64
	if err = tx.QueryRow(ctx, `SELECT branch_id,version FROM production_jobs WHERE business_id=$1 AND id=$2 FOR UPDATE`, actor.VendorID, jobID).Scan(&branch, &jobVersion); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", errNotFound("Job", CodeJobNotFound)
		}
		return "", err
	}
	if in.ExpectedJobVersion > 0 && in.ExpectedJobVersion != jobVersion {
		return "", errStaleVersion()
	}
	id := newID("pstep_")
	_, err = tx.Exec(ctx, `INSERT INTO production_job_steps(id,business_id,job_id,production_line_id,sequence_no,phase,name,required) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, id, actor.VendorID, jobID, in.ProductionLineID, in.SequenceNo, in.Phase, strings.TrimSpace(in.Name), in.Required)
	if err != nil {
		return "", err
	}
	if err = s.RecordMutation(ctx, tx, actor, "production.step.created", "production.step.created", "production_job_step", id, branch, 1, nil, in, map[string]any{"job_id": jobID}, ""); err != nil {
		return "", err
	}
	if err = tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}

// ChangeStep applies OCC to a station step and derives the top-level job's
// operational status without touching the commercial VendorOrder.
func (s *Service) ChangeStep(ctx context.Context, actor Actor, jobID, stepID string, in StepChange) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var branch, current string
	var sequenceNo int
	var version int64
	if err = tx.QueryRow(ctx, `SELECT j.branch_id,s.status,s.version,s.sequence_no FROM production_job_steps s JOIN production_jobs j ON j.id=s.job_id AND j.business_id=s.business_id WHERE s.business_id=$1 AND s.job_id=$2 AND s.id=$3 FOR UPDATE`, actor.VendorID, jobID, stepID).Scan(&branch, &current, &version, &sequenceNo); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errNotFound("Step", CodeJobNotFound)
		}
		return err
	}
	target := strings.ToUpper(strings.TrimSpace(in.Status))
	if in.ExpectedVersion <= 0 || in.ExpectedVersion != version || !stepTransitions[current][target] {
		return errStaleVersion()
	}
	if target == "READY" {
		var prior int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM production_job_steps WHERE business_id=$1 AND job_id=$2 AND required AND sequence_no<$3 AND status NOT IN ('COMPLETED','SKIPPED')`, actor.VendorID, jobID, sequenceNo).Scan(&prior); err != nil {
			return err
		}
		if prior > 0 {
			return coded(apperrors.Conflict("Prior required production steps are not complete.", nil), CodeInvalidTransition)
		}
	}
	if target == "BLOCKED" && strings.TrimSpace(in.BlockedReason) == "" {
		return validation()
	}
	var tag pgconn.CommandTag
	tag, err = tx.Exec(ctx, `UPDATE production_job_steps SET status=$1,blocked_reason=CASE WHEN $1='BLOCKED' THEN $2 WHEN $1='IN_PROGRESS' THEN '' ELSE blocked_reason END,blocked_at=CASE WHEN $1='BLOCKED' THEN now() ELSE blocked_at END,started_at=CASE WHEN $1='IN_PROGRESS' THEN COALESCE(started_at,now()) ELSE started_at END,completed_at=CASE WHEN $1 IN ('COMPLETED','SKIPPED','CANCELLED') THEN COALESCE(completed_at,now()) ELSE completed_at END,version=version+1,updated_at=now() WHERE business_id=$3 AND job_id=$4 AND id=$5 AND version=$6`, target, in.BlockedReason, actor.VendorID, jobID, stepID, version)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errStaleVersion()
	}
	jobStatus := ""
	if target == "BLOCKED" {
		jobStatus = "BLOCKED"
	} else if target == "IN_PROGRESS" {
		jobStatus = "IN_PROGRESS"
	} else if target == "READY" {
		jobStatus = "IN_PROGRESS"
	}
	if jobStatus != "" {
		_, err = tx.Exec(ctx, `UPDATE production_jobs SET status=$1,blocked_reason=CASE WHEN $1='BLOCKED' THEN $2 ELSE '' END,updated_at=now(),version=version+1 WHERE business_id=$3 AND id=$4 AND status NOT IN ('COMPLETED','CANCELLED')`, jobStatus, in.BlockedReason, actor.VendorID, jobID)
	}
	if err == nil {
		err = s.RecordMutation(ctx, tx, actor, "production.step."+strings.ToLower(target), "production.step."+strings.ToLower(target), "production_job_step", stepID, branch, version+1, map[string]any{"status": current}, map[string]any{"status": target}, map[string]any{"job_id": jobID, "note": in.Note}, "")
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	return err
}

type DelayInput struct {
	ReasonCode      string     `json:"reason_code"`
	Note            string     `json:"note"`
	NewWindowStart  *time.Time `json:"new_window_start"`
	NewWindowEnd    *time.Time `json:"new_window_end"`
	IdempotencyKey  string     `json:"idempotency_key"`
	ExpectedVersion int64      `json:"expected_version"`
}

// ReportDelay appends immutable delay evidence and revises only the
// operational expected-ready window. promised_ready_at is never changed.
// ReportDelay retains the native API transaction boundary.
func (s *Service) ReportDelay(ctx context.Context, actor Actor, jobID string, in DelayInput) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = s.ReportDelayInTx(ctx, tx, actor, jobID, in); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ReportDelayInTx lets source-owned commands commit delay/readiness/ETA evidence
// atomically with their immutable command receipt. It never changes the promise.
func (s *Service) ReportDelayInTx(ctx context.Context, tx pgx.Tx, actor Actor, jobID string, in DelayInput) error {
	if err := s.requireWrites(); err != nil {
		return err
	}
	if !inVocabulary([]string{"STAFF_SHORTAGE", "EQUIPMENT_FAILURE", "INGREDIENT_SHORTAGE", "QUALITY_REWORK", "CAPACITY_CONFLICT", "LOGISTICS_CONSTRAINT", "OTHER"}, strings.ToUpper(strings.TrimSpace(in.ReasonCode))) || len(in.Note) > 280 || in.NewWindowStart == nil || in.NewWindowEnd == nil || !in.NewWindowEnd.After(*in.NewWindowStart) {
		return validation()
	}
	key := strings.TrimSpace(in.IdempotencyKey)
	if key == "" {
		key = actor.CommandRef
	}
	if key == "" {
		return validation()
	}
	var err error
	var branch string
	var promised, oldStart, oldEnd *time.Time
	var version int64
	if err = tx.QueryRow(ctx, `SELECT branch_id,promised_ready_at,expected_ready_window_start,expected_ready_window_end,version FROM production_jobs WHERE business_id=$1 AND id=$2 FOR UPDATE`, actor.VendorID, jobID).Scan(&branch, &promised, &oldStart, &oldEnd, &version); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errNotFound("Job", CodeJobNotFound)
		}
		return err
	}
	if in.ExpectedVersion <= 0 || in.ExpectedVersion != version {
		var already bool
		if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM production_job_delay_events WHERE business_id=$1 AND idempotency_key=$2)`, actor.VendorID, key).Scan(&already); e != nil {
			return e
		}
		if already {
			return nil
		}
		return errStaleVersion()
	}
	var priorJob string
	var priorStart, priorEnd *time.Time
	lookupErr := tx.QueryRow(ctx, `SELECT job_id,new_window_start,new_window_end FROM production_job_delay_events WHERE business_id=$1 AND idempotency_key=$2`, actor.VendorID, key).Scan(&priorJob, &priorStart, &priorEnd)
	if lookupErr == nil {
		if priorJob != jobID || priorStart == nil || priorEnd == nil || !priorStart.Equal(*in.NewWindowStart) || !priorEnd.Equal(*in.NewWindowEnd) {
			return coded(apperrors.Conflict("This delay command key was already used with different evidence.", nil), CodeIdempotencyConflict)
		}
		return nil
	}
	if !errors.Is(lookupErr, pgx.ErrNoRows) {
		return lookupErr
	}
	var priorETAStatus string
	var priorETAStart, priorETAEnd *time.Time
	var vendorOrderID, parentOrderID, customerRef string
	var priorEvidence []byte
	readinessErr := tx.QueryRow(ctx, `SELECT vendor_order_id,parent_order_id,COALESCE(customer_eta_status,'NOT_AVAILABLE'),customer_eta_start,customer_eta_end,timing_source_evidence FROM commerce_vendor_order_readiness WHERE business_id=$1 AND production_job_id=$2 FOR UPDATE`, actor.VendorID, jobID).Scan(&vendorOrderID, &parentOrderID, &priorETAStatus, &priorETAStart, &priorETAEnd, &priorEvidence)
	if errors.Is(readinessErr, pgx.ErrNoRows) {
		priorETAStatus = "NOT_AVAILABLE"
		readinessErr = nil
	}
	if readinessErr != nil {
		return readinessErr
	}
	if parentOrderID != "" {
		if err = tx.QueryRow(ctx, `SELECT customer_id FROM commerce_parent_orders WHERE id=$1`, parentOrderID).Scan(&customerRef); err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO production_job_delay_events(id,business_id,job_id,reason_code,note,promised_ready_at,previous_window_start,previous_window_end,new_window_start,new_window_end,actor_id,correlation_id,idempotency_key) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NULLIF($11,''),NULLIF($12,''),$13)`, newID("pdelay_"), actor.VendorID, jobID, strings.ToUpper(in.ReasonCode), strings.TrimSpace(in.Note), promised, oldStart, oldEnd, in.NewWindowStart, in.NewWindowEnd, actor.StaffID, actor.CorrelationID, key)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE production_jobs SET expected_ready_window_start=$1,expected_ready_window_end=$2,updated_at=now(),version=version+1 WHERE business_id=$3 AND id=$4`, in.NewWindowStart, in.NewWindowEnd, actor.VendorID, jobID)
	if err == nil {
		// Until the logistics authority returns a current delivery window, an
		// updated production window is not a customer ETA. Marking it honestly
		// as unavailable prevents a stale promise from being shown as current.
		_, err = tx.Exec(ctx, `UPDATE commerce_vendor_order_readiness SET expected_window_start=$1,expected_window_end=$2,customer_eta_status='NOT_AVAILABLE',customer_eta_start=NULL,customer_eta_end=NULL,timing_revision=timing_revision+1,timing_source_evidence=jsonb_build_object('production_job_id',$4::text,'production_window_start',$1::timestamptz,'production_window_end',$2::timestamptz,'logistics_timing','NOT_AVAILABLE'),source_version=source_version+1,updated_at=now() WHERE business_id=$3 AND production_job_id=$4`, in.NewWindowStart, in.NewWindowEnd, actor.VendorID, jobID)
	}
	if err == nil {
		// Platform logistics owns the mission, but its pickup timing projection
		// may consume this production readiness evidence. Fare and mission state
		// are never changed here.
		_, err = tx.Exec(ctx, `UPDATE commerce_order_missions m SET production_ready_window_start=$1,production_ready_window_end=$2,production_timing_source_job_id=$3,production_timing_version=production_timing_version+1,logistics_timing_status=CASE WHEN m.status IN ('PENDING','ACCEPTED') THEN 'UNKNOWN' ELSE m.logistics_timing_status END,updated_at=now() FROM commerce_order_mission_allocations a JOIN commerce_vendor_orders v ON v.id=a.vendor_order_id JOIN commerce_vendor_order_readiness r ON r.vendor_order_id=v.id WHERE a.mission_id=m.id AND r.production_job_id=$3 AND v.business_id=$4`, in.NewWindowStart, in.NewWindowEnd, jobID, actor.VendorID)
	}
	if err == nil {
		err = s.RecordMutation(ctx, tx, actor, "production.job.delay_reported", "production.job.delay_reported", "production_job", jobID, branch, version+1, map[string]any{"expected_ready_window_start": oldStart, "expected_ready_window_end": oldEnd, "promised_ready_at": promised}, map[string]any{"expected_ready_window_start": in.NewWindowStart, "expected_ready_window_end": in.NewWindowEnd, "promised_ready_at": promised}, json.RawMessage(`{"reason_code":"`+strings.ToUpper(in.ReasonCode)+`"}`), "")
	}
	if err == nil && vendorOrderID != "" && s.eta != nil {
		err = s.eta.ProjectReadinessETA(ctx, tx, vendorOrderID, parentOrderID,
			in.NewWindowStart, in.NewWindowEnd, true, false, "production-delay:"+jobID+":"+key,
			version+1, s.now().UTC())
	}
	if err == nil && vendorOrderID != "" && s.eta == nil {
		material := priorETAStatus != "NOT_AVAILABLE" || priorETAStart != nil || priorETAEnd != nil
		if material {
			payload := map[string]any{
				"customer_ref":            customerRef,
				"vendor_order_ref":        vendorOrderID,
				"parent_order_ref":        parentOrderID,
				"production_job_ref":      jobID,
				"customer_eta_status":     "NOT_AVAILABLE",
				"previous_eta_start":      priorETAStart,
				"previous_eta_end":        priorETAEnd,
				"production_window_start": in.NewWindowStart,
				"production_window_end":   in.NewWindowEnd,
				"source_evidence":         json.RawMessage(priorEvidence),
			}
			err = s.publishTx(ctx, tx, actor, "commerce.customer_eta.changed", "VendorOrder", vendorOrderID, version+1, payload)
		}
	}
	return err
}
