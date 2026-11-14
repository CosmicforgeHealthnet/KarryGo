package usecases

import (
	"context"
	"encoding/json"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/compliance/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
)

func (s *Service) RegisterWorkers(w *jobs.Worker) {
	w.Register(models.JobComplianceSweep, s.handleComplianceSweep)
	w.Register(models.JobRestrictionReconcile, s.handleRestrictionReconcile)
}

func (s *Service) ScheduleMaintenance(ctx context.Context) error {
	if !s.o.Flags.Workers || s.o.Flags.KillSwitch {
		return nil
	}
	orgs, err := s.repo.ActiveOrganizations(ctx)
	if err != nil {
		return err
	}
	for _, org := range orgs {
		_, err = s.o.Queue.Enqueue(ctx, s.o.Pool, jobs.EnqueueRequest{OrganizationID: org, JobType: models.JobComplianceSweep, DedupeKey: "section19:" + org + ":" + time.Now().UTC().Format("200601021504"), Payload: map[string]string{"organizationId": org}, MaxAttempts: 8})
		if err != nil {
			return err
		}
	}
	return nil
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
	if e := s.repo.SweepStatuses(ctx, p.OrganizationID); e != nil {
		return e
	}
	due, e := s.repo.DueRestrictionIDs(ctx, p.OrganizationID)
	if e != nil {
		return e
	}
	for _, id := range due {
		if e = s.ProcessRestriction(ctx, p.OrganizationID, id); e != nil {
			return e
		}
	}
	return nil
}
