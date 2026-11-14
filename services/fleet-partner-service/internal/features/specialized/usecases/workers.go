package usecases

import (
	"context"
	"encoding/json"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/specialized/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
)

func (s *Service) RegisterWorkers(w *jobs.Worker) {
	w.Register(models.JobSpecializedSweep, s.handleSweep)
}
func (s *Service) ScheduleMaintenance(ctx context.Context) error {
	if !s.o.Flags.Workers || s.o.Flags.KillSwitch {
		return nil
	}
	orgs, e := s.repo.EnabledOrganizations(ctx)
	if e != nil {
		return e
	}
	for _, org := range orgs {
		_, e = s.o.Queue.Enqueue(ctx, s.o.Pool, jobs.EnqueueRequest{OrganizationID: org, JobType: models.JobSpecializedSweep, DedupeKey: "section20:" + org + ":" + time.Now().UTC().Format("200601021504"), Payload: map[string]string{"organizationId": org}, MaxAttempts: 8})
		if e != nil {
			return e
		}
	}
	return nil
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
	return s.repo.Sweep(ctx, p.OrganizationID)
}
