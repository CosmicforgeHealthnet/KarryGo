package specialized

import (
	"context"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"encoding/json"
	"time"
)

const JobSpecializedSweep = "section20.specialized_sweep"

func (s *Service) RegisterWorkers(w *jobs.Worker) { w.Register(JobSpecializedSweep, s.handleSweep) }
func (s *Service) ScheduleMaintenance(ctx context.Context) error {
	if !s.o.Flags.Workers || s.o.Flags.KillSwitch {
		return nil
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT organization_id FROM fleet_partner_organization_capabilities WHERE status='ENABLED' GROUP BY organization_id`)
	if e != nil {
		return e
	}
	defer rows.Close()
	for rows.Next() {
		var org string
		if e = rows.Scan(&org); e != nil {
			return e
		}
		_, e = s.o.Queue.Enqueue(ctx, s.o.Pool, jobs.EnqueueRequest{OrganizationID: org, JobType: JobSpecializedSweep, DedupeKey: "section20:" + org + ":" + time.Now().UTC().Format("200601021504"), Payload: map[string]string{"organizationId": org}, MaxAttempts: 8})
		if e != nil {
			return e
		}
	}
	return rows.Err()
}
func (s *Service) handleSweep(ctx context.Context, j jobs.Job) error {
	var p struct {
		OrganizationID string `json:"organizationId"`
	}
	if e := json.Unmarshal(j.Payload, &p); e != nil {
		return e
	}
	if p.OrganizationID != j.OrganizationID {
		return jobs.ErrTenantMismatch
	}
	_, e := s.o.Pool.Exec(ctx, `UPDATE fleet_partner_specialized_records SET status='EXPIRED',version=version+1,updated_at=now() WHERE organization_id=$1 AND record_type='PERMIT' AND status NOT IN('EXPIRED','COMPLETED') AND NULLIF(payload->>'validUntil','')::timestamptz<=now()`, p.OrganizationID)
	if e != nil {
		return e
	}
	_, e = s.o.Pool.Exec(ctx, `UPDATE fleet_partner_specialized_records r SET status='QUARANTINED',version=version+1,updated_at=now() WHERE organization_id=$1 AND capability_code='COLD_CHAIN' AND EXISTS(SELECT 1 FROM fleet_partner_temperature_events t WHERE t.record_id=r.id AND t.excursion) AND status NOT IN('QUARANTINED','RELEASED')`, p.OrganizationID)
	return e
}
