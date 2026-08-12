package assets

import (
	"context"
	"encoding/json"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
)

const JobMaintenance = "section18.maintenance"

func (s *Service) RegisterWorkers(w *jobs.Worker) { w.Register(JobMaintenance, s.handleMaintenance) }
func (s *Service) ScheduleMaintenance(ctx context.Context) error {
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
		_, err = s.o.Queue.Enqueue(ctx, s.o.Pool, jobs.EnqueueRequest{OrganizationID: org, JobType: JobMaintenance, DedupeKey: "section18:" + org + ":" + time.Now().UTC().Format("200601021504"), Payload: map[string]string{"organizationId": org}, MaxAttempts: 8})
		if err != nil {
			return err
		}
	}
	return rows.Err()
}
func (s *Service) handleMaintenance(ctx context.Context, j jobs.Job) error {
	var p struct {
		OrganizationID string `json:"organizationId"`
	}
	if e := json.Unmarshal(j.Payload, &p); e != nil {
		return e
	}
	if p.OrganizationID != j.OrganizationID {
		return jobs.ErrTenantMismatch
	}
	if e := s.RunMaintenance(ctx, p.OrganizationID); e != nil {
		return e
	}
	return s.ReconcileSafetyDeclines(ctx)
}
