package crm

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"time"
)

const (
	TriggerUnresolvedComplaint = "UNRESOLVED_COMPLAINT"
	TriggerRejectedRFQ         = "REJECTED_RFQ"
	TriggerHighValueRecovery   = "HIGH_VALUE_RECOVERY"
	TriggerLapsedCustomer      = "LAPSED_CUSTOMER"
	TriggerCorporateLead       = "CORPORATE_LEAD"
)

type TaskTriggerConfig struct {
	Key                  string `json:"trigger_key"`
	Enabled              bool   `json:"enabled"`
	DueOffsetHours       int    `json:"due_offset_hours"`
	Priority             string `json:"priority"`
	UnresolvedAfterHours int    `json:"unresolved_after_hours"`
	Version              int64  `json:"version"`
}

func (s *Service) UpsertTaskTriggerConfig(ctx context.Context, actor Actor, cfg TaskTriggerConfig) (TaskTriggerConfig, error) {
	if cfg.DueOffsetHours < 0 {
		cfg.DueOffsetHours = 0
	}
	if cfg.Priority == "" {
		cfg.Priority = PriorityNormal
	}
	if cfg.UnresolvedAfterHours < 0 {
		cfg.UnresolvedAfterHours = 0
	}
	if cfg.Version < 1 {
		cfg.Version = 1
	}
	_, err := s.db.Exec(ctx, `INSERT INTO crm_task_trigger_configs (business_id,trigger_key,enabled,due_offset_hours,priority,unresolved_after_hours,version) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (business_id,trigger_key) DO UPDATE SET enabled=EXCLUDED.enabled,due_offset_hours=EXCLUDED.due_offset_hours,priority=EXCLUDED.priority,unresolved_after_hours=EXCLUDED.unresolved_after_hours,updated_at=now(),version=crm_task_trigger_configs.version+1`, actor.VendorID, cfg.Key, cfg.Enabled, cfg.DueOffsetHours, cfg.Priority, cfg.UnresolvedAfterHours, cfg.Version)
	if err != nil {
		return cfg, err
	}
	return cfg, nil
}

// EvaluateCustomerTriggers is idempotent: the unique identity is claimed before
// the task is created, so two workers or a replay can produce at most one task.
func (s *Service) EvaluateCustomerTriggers(ctx context.Context, actor Actor, profileID string, asOf time.Time) ([]Task, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var cfgs []TaskTriggerConfig
	rows, err := tx.Query(ctx, `SELECT trigger_key,enabled,due_offset_hours,priority,unresolved_after_hours,version FROM crm_task_trigger_configs WHERE business_id=$1 AND enabled=true`, actor.VendorID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var c TaskTriggerConfig
		if err := rows.Scan(&c.Key, &c.Enabled, &c.DueOffsetHours, &c.Priority, &c.UnresolvedAfterHours, &c.Version); err != nil {
			rows.Close()
			return nil, err
		}
		cfgs = append(cfgs, c)
	}
	rows.Close()
	var out []Task
	for _, cfg := range cfgs {
		qualifies, source := s.triggerQualifies(ctx, tx, actor.VendorID, profileID, cfg, asOf)
		if !qualifies {
			continue
		}
		identity := fmt.Sprintf("%s:%s:%s", cfg.Key, profileID, source)
		var evaluationID string
		err = tx.QueryRow(ctx, `INSERT INTO crm_task_trigger_evaluations (id,business_id,trigger_key,trigger_version,subject_type,customer_profile_id,dedupe_identity,source_ref,status,evaluated_at,last_evaluated_at) VALUES($1,$2,$3,$4,'CUSTOMER',$5,$6,$7,'FIRED',$8,$8) ON CONFLICT (business_id,trigger_key,dedupe_identity) DO UPDATE SET last_evaluated_at=EXCLUDED.last_evaluated_at RETURNING id`, newID("crmeval_"), actor.VendorID, cfg.Key, cfg.Version, profileID, identity, source, asOf).Scan(&evaluationID)
		if err != nil {
			return nil, err
		}
		var existing string
		_ = tx.QueryRow(ctx, `SELECT COALESCE(task_id,'') FROM crm_task_trigger_evaluations WHERE id=$1`, evaluationID).Scan(&existing)
		if existing != "" {
			continue
		}
		due := asOf.Add(time.Duration(cfg.DueOffsetHours) * time.Hour)
		trigger := cfg.Key
		taskSQL := `INSERT INTO crm_tasks (id,business_id,customer_profile_id,original_profile_id,visibility_scope,title,description,status,priority,created_by_staff_id,due_at,trigger_key,trigger_version,trigger_source_ref,trigger_evaluated_at) VALUES($1,$2,$3,$3,'BUSINESS',$4,$5,'OPEN',$6,NULL,$7,$8,$9,$10,$11) RETURNING ` + taskCols
		task, err := scanTask(tx.QueryRow(ctx, taskSQL, newID("crmt_"), actor.VendorID, profileID, trigger, triggerDescription(cfg.Key), cfg.Priority, due, cfg.Key, cfg.Version, source, asOf))
		if err != nil {
			return nil, err
		}
		if err = s.appendTaskHistory(ctx, tx, actor, task, "TRIGGERED"); err != nil {
			return nil, err
		}
		if _, err = tx.Exec(ctx, `UPDATE crm_task_trigger_evaluations SET task_id=$1 WHERE id=$2`, task.ID, evaluationID); err != nil {
			return nil, err
		}
		out = append(out, task)
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) triggerQualifies(ctx context.Context, tx pgx.Tx, businessID, profileID string, cfg TaskTriggerConfig, asOf time.Time) (bool, string) {
	switch cfg.Key {
	case TriggerUnresolvedComplaint:
		var ref string
		var at time.Time
		err := tx.QueryRow(ctx, `SELECT source_case_ref,occurred_at FROM crm_customer_support_facts WHERE business_id=$1 AND profile_id=$2 AND case_state IN ('OPEN','IN_REVIEW') ORDER BY occurred_at LIMIT 1`, businessID, profileID).Scan(&ref, &at)
		return err == nil && asOf.Sub(at) >= time.Duration(cfg.UnresolvedAfterHours)*time.Hour, ref
	case TriggerHighValueRecovery:
		var ref string
		err := tx.QueryRow(ctx, `SELECT COALESCE(r.latest_case_ref,'') FROM crm_customer_service_recovery_projections r JOIN crm_customer_value_projections v ON v.business_id=r.business_id AND v.profile_id=r.profile_id AND v.scope='BUSINESS' AND v.state='CURRENT' JOIN crm_customer_value_band_definitions d ON d.business_id=v.business_id AND d.band_key=v.band WHERE r.business_id=$1 AND r.profile_id=$2 AND r.unresolved_complaint_count>0 AND d.rank >= COALESCE((SELECT high_value_minimum_rank FROM crm_relationship_signal_configs WHERE business_id=$1),3)`, businessID, profileID).Scan(&ref)
		return err == nil, ref
	case TriggerLapsedCustomer:
		var lapsed bool
		var last *time.Time
		err := tx.QueryRow(ctx, `SELECT lapsed_signal,last_completed_order_at FROM crm_relationship_signals WHERE business_id=$1 AND profile_id=$2 AND scope='BUSINESS'`, businessID, profileID).Scan(&lapsed, &last)
		if last == nil {
			last = &asOf
		}
		return err == nil && lapsed, last.Format(time.RFC3339)
	case TriggerRejectedRFQ:
		var ref string
		err := tx.QueryRow(ctx, `SELECT source_rfq_ref FROM crm_corporate_rfq_facts WHERE business_id=$1 AND profile_id=$2 AND (quote_outcome='REJECTED' OR rfq_state='REJECTED') ORDER BY occurred_at DESC LIMIT 1`, businessID, profileID).Scan(&ref)
		return err == nil, ref
	default:
		return false, ""
	}
}
func triggerDescription(key string) string {
	switch key {
	case TriggerUnresolvedComplaint:
		return "Follow up on an unresolved complaint"
	case TriggerRejectedRFQ:
		return "Follow up on a rejected RFQ"
	case TriggerHighValueRecovery:
		return "Recover a high-value customer relationship"
	case TriggerLapsedCustomer:
		return "Re-engage a lapsed customer"
	case TriggerCorporateLead:
		return "Follow up on a corporate lead"
	}
	return "CRM follow-up"
}

// RunAutomaticFollowUpPass is the bounded worker entry point. Deployment may
// schedule it with the existing jobs package; the pass itself never scans an
// unbounded tenant and each evaluation remains protected by the dedupe fence.
func (s *Service) RunAutomaticFollowUpPass(ctx context.Context, actor Actor, limit int) (int, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `SELECT id FROM vendor_customer_profiles WHERE business_id=$1 AND privacy_state NOT IN ('REDACTED','MERGED') AND merge_state <> 'MERGED_SOURCE' ORDER BY id LIMIT $2`, actor.VendorID, limit)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	total := 0
	for _, id := range ids {
		tasks, err := s.EvaluateCustomerTriggers(ctx, actor, id, s.now())
		if err != nil {
			return total, err
		}
		total += len(tasks)
	}
	return total, nil
}

// EvaluateCorporateTriggers handles the two triggers whose subject is the
// existing corporate CRM account rather than an individual profile.
func (s *Service) EvaluateCorporateTriggers(ctx context.Context, actor Actor, corporateID string, asOf time.Time) ([]Task, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, `SELECT trigger_key,enabled,due_offset_hours,priority,unresolved_after_hours,version FROM crm_task_trigger_configs WHERE business_id=$1 AND enabled=true AND trigger_key IN ('REJECTED_RFQ','CORPORATE_LEAD')`, actor.VendorID)
	if err != nil {
		return nil, err
	}
	var cfgs []TaskTriggerConfig
	for rows.Next() {
		var c TaskTriggerConfig
		if err := rows.Scan(&c.Key, &c.Enabled, &c.DueOffsetHours, &c.Priority, &c.UnresolvedAfterHours, &c.Version); err != nil {
			rows.Close()
			return nil, err
		}
		cfgs = append(cfgs, c)
	}
	rows.Close()
	var out []Task
	for _, c := range cfgs {
		var source string
		var qualifies bool
		if c.Key == TriggerRejectedRFQ {
			err := tx.QueryRow(ctx, `SELECT source_rfq_ref FROM crm_corporate_rfq_facts WHERE business_id=$1 AND corporate_customer_id=$2 AND (quote_outcome='REJECTED' OR rfq_state='REJECTED') ORDER BY occurred_at DESC LIMIT 1`, actor.VendorID, corporateID).Scan(&source)
			qualifies = err == nil
		} else {
			var status string
			err := tx.QueryRow(ctx, `SELECT status FROM corporate_customer_profiles WHERE business_id=$1 AND id=$2`, actor.VendorID, corporateID).Scan(&status)
			qualifies = err == nil && (status == "LEAD" || status == "PROSPECT")
			source = corporateID
		}
		if !qualifies {
			continue
		}
		identity := fmt.Sprintf("%s:%s:%s", c.Key, corporateID, source)
		var evalID string
		if err := tx.QueryRow(ctx, `INSERT INTO crm_task_trigger_evaluations (id,business_id,trigger_key,trigger_version,subject_type,corporate_customer_id,dedupe_identity,source_ref,status,evaluated_at,last_evaluated_at) VALUES($1,$2,$3,$4,'CORPORATE',$5,$6,$7,'FIRED',$8,$8) ON CONFLICT (business_id,trigger_key,dedupe_identity) DO UPDATE SET last_evaluated_at=EXCLUDED.last_evaluated_at RETURNING id`, newID("crmeval_"), actor.VendorID, c.Key, c.Version, corporateID, identity, source, asOf).Scan(&evalID); err != nil {
			return nil, err
		}
		var existing string
		_ = tx.QueryRow(ctx, `SELECT COALESCE(task_id,'') FROM crm_task_trigger_evaluations WHERE id=$1`, evalID).Scan(&existing)
		if existing != "" {
			continue
		}
		due := asOf.Add(time.Duration(c.DueOffsetHours) * time.Hour)
		sql := `INSERT INTO crm_tasks (id,business_id,corporate_customer_id,visibility_scope,title,description,status,priority,created_by_staff_id,due_at,trigger_key,trigger_version,trigger_source_ref,trigger_evaluated_at) VALUES($1,$2,$3,'BUSINESS',$4,$5,'OPEN',$6,NULL,$7,$8,$9,$10,$11) RETURNING ` + taskCols
		task, err := scanTask(tx.QueryRow(ctx, sql, newID("crmt_"), actor.VendorID, corporateID, c.Key, triggerDescription(c.Key), c.Priority, due, c.Key, c.Version, source, asOf))
		if err != nil {
			return nil, err
		}
		if err = s.appendTaskHistory(ctx, tx, actor, task, "TRIGGERED"); err != nil {
			return nil, err
		}
		if _, err = tx.Exec(ctx, `UPDATE crm_task_trigger_evaluations SET task_id=$1 WHERE id=$2`, task.ID, evalID); err != nil {
			return nil, err
		}
		out = append(out, task)
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

var _ = pgx.ErrNoRows
