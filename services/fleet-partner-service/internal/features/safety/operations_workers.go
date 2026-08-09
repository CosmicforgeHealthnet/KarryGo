package safety

import (
	"context"
	"encoding/json"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
)

const JobSafetySweep = "section19.safety_sweep"

func (s *OperationsService) RegisterWorkers(w *jobs.Worker) {
	w.Register(JobSafetySweep, s.handleSafetySweep)
}
func (s *OperationsService) ScheduleMaintenance(ctx context.Context) error {
	if !s.o.Flags.Workers || s.o.Flags.KillSwitch {
		return nil
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT id FROM fleet_partner_organizations WHERE status='ACTIVE'`)
	if e != nil {
		return e
	}
	defer rows.Close()
	for rows.Next() {
		var org string
		if e = rows.Scan(&org); e != nil {
			return e
		}
		_, e = s.o.Queue.Enqueue(ctx, s.o.Pool, jobs.EnqueueRequest{OrganizationID: org, JobType: JobSafetySweep, DedupeKey: "section19-safety:" + org + ":" + time.Now().UTC().Format("200601021504"), Payload: map[string]string{"organizationId": org}, MaxAttempts: 8})
		if e != nil {
			return e
		}
	}
	return rows.Err()
}
func (s *OperationsService) handleSafetySweep(ctx context.Context, j jobs.Job) error {
	var p struct {
		OrganizationID string `json:"organizationId"`
	}
	if e := json.Unmarshal(j.Payload, &p); e != nil {
		return e
	}
	if p.OrganizationID != j.OrganizationID {
		return jobs.ErrTenantMismatch
	}
	_, e := s.o.Pool.Exec(ctx, `UPDATE fleet_partner_corrective_actions SET status='OVERDUE',version=version+1,updated_at=now() WHERE organization_id=$1 AND status IN('OPEN','IN_PROGRESS') AND due_at<now()`, p.OrganizationID)
	return e
}
