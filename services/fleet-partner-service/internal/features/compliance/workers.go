package compliance

import (
	"context"
	"encoding/json"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
)

const (
	JobComplianceSweep      = "section19.compliance_sweep"
	JobRestrictionReconcile = "section19.restriction_reconcile"
)

func (s *Service) RegisterWorkers(w *jobs.Worker) {
	w.Register(JobComplianceSweep, s.handleComplianceSweep)
	w.Register(JobRestrictionReconcile, s.handleRestrictionReconcile)
}

func (s *Service) ScheduleMaintenance(ctx context.Context) error {
	if !s.o.Flags.Workers || s.o.Flags.KillSwitch {
		return nil
	}
	rows, err := s.o.Pool.Query(ctx, `SELECT id FROM fleet_partner_organizations WHERE status='ACTIVE'`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var org string
		if err = rows.Scan(&org); err != nil {
			return err
		}
		_, err = s.o.Queue.Enqueue(ctx, s.o.Pool, jobs.EnqueueRequest{OrganizationID: org, JobType: JobComplianceSweep, DedupeKey: "section19:" + org + ":" + time.Now().UTC().Format("200601021504"), Payload: map[string]string{"organizationId": org}, MaxAttempts: 8})
		if err != nil {
			return err
		}
	}
	return rows.Err()
}
func (s *Service) handleRestrictionReconcile(ctx context.Context, j jobs.Job) error {
	var p struct {
		OrganizationID string `json:"organizationId"`
		RestrictionID  string `json:"restrictionId"`
	}
	if e := json.Unmarshal(j.Payload, &p); e != nil {
		return e
	}
	if p.OrganizationID != j.OrganizationID {
		return jobs.ErrTenantMismatch
	}
	return s.ProcessRestriction(ctx, p.OrganizationID, p.RestrictionID)
}
func (s *Service) handleComplianceSweep(ctx context.Context, j jobs.Job) error {
	var p struct {
		OrganizationID string `json:"organizationId"`
	}
	if e := json.Unmarshal(j.Payload, &p); e != nil {
		return e
	}
	if p.OrganizationID != j.OrganizationID {
		return jobs.ErrTenantMismatch
	}
	_, e := s.o.Pool.Exec(ctx, `UPDATE fleet_partner_compliance_statuses SET state=CASE WHEN expires_at<=now() THEN 'EXPIRED' WHEN expires_at<=now()+interval '30 days' AND state='COMPLIANT' THEN 'DUE_SOON' ELSE state END,version=version+1,updated_at=now() WHERE organization_id=$1 AND expires_at IS NOT NULL AND((expires_at<=now() AND state NOT IN('EXPIRED','RESTRICTED'))OR(expires_at<=now()+interval '30 days' AND state='COMPLIANT'))`, p.OrganizationID)
	if e != nil {
		return e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT DISTINCT restriction_id FROM fleet_partner_restriction_operations WHERE organization_id=$1 AND status IN('OUTCOME_UNKNOWN','RETRY_SCHEDULED') AND next_attempt_at<=now()`, p.OrganizationID)
	if e != nil {
		return e
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			return e
		}
		if e = s.ProcessRestriction(ctx, p.OrganizationID, id); e != nil {
			return e
		}
	}
	return rows.Err()
}
