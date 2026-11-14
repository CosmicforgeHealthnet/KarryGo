package usecases

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/workforce/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

const JobMaintenance = "section17.maintenance"

func (s *Service) RegisterWorkers(w *jobs.Worker) { w.Register(JobMaintenance, s.handleMaintenance) }
func (s *Service) ScheduleMaintenance(ctx context.Context) error {
	orgs, err := s.repo.ActiveOrganizationIDs(ctx, s.o.Pool)
	if err != nil {
		return err
	}
	for _, org := range orgs {
		_, err = s.o.Queue.Enqueue(ctx, s.o.Pool, jobs.EnqueueRequest{OrganizationID: org, JobType: JobMaintenance, DedupeKey: "section17:" + org + ":" + time.Now().UTC().Format("200601021504"), Payload: map[string]string{"organizationId": org}, MaxAttempts: 8})
		if err != nil {
			return err
		}
	}
	return nil
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
	return s.GenerateDuePerformance(ctx, p.OrganizationID)
}
func (s *Service) GenerateDuePerformance(ctx context.Context, org string) error {
	if !s.o.Flags.Performance || s.o.Flags.KillSwitch {
		return nil
	}
	end := s.o.Now().UTC().Truncate(24 * time.Hour)
	start := end.Add(-24 * time.Hour)
	members, e := s.repo.ActiveProfileMemberships(ctx, s.o.Pool, org)
	if e != nil {
		return e
	}
	scope := tenant.Context{OrganizationID: org, ActorType: tenant.ActorTypeSystem, ActorID: "performance-snapshot-worker", CorrelationID: end.Format("2006-01-02")}
	for _, id := range members {
		if _, e = s.GeneratePerformance(ctx, scope, id, start, end); e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) RunMaintenance(ctx context.Context, org string) error {
	if !s.o.Flags.Workers || s.o.Flags.KillSwitch {
		return nil
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		if e := s.repo.AdvanceSchedules(ctx, tx, org); e != nil {
			return e
		}
		scope := tenant.Context{OrganizationID: org, ActorType: tenant.ActorTypeSystem, ActorID: "section17-maintenance-worker", CorrelationID: "section17-maintenance"}
		changes := []struct {
			apply              func(context.Context, database.Querier, string) ([]models.VersionedID, error)
			event, kind, state string
		}{
			{s.repo.MarkNoShows, `shift_assignment.no_show.v1`, `shift_assignment`, `NO_SHOW`},
			{s.repo.ExpireProviderQualifications, `provider_qualification.expired.v1`, `provider_qualification`, `EXPIRED`},
			{s.repo.ExpireTrainingCompletions, `training.expired.v1`, `training_completion`, `EXPIRED`},
		}
		for _, change := range changes {
			updated, e := change.apply(ctx, tx, org)
			if e != nil {
				return e
			}
			for _, item := range updated {
				if e = s.record(ctx, tx, scope, change.event, change.kind, item.ID, item.Version, map[string]any{"id": item.ID, "status": change.state}); e != nil {
					return e
				}
			}
		}
		return nil
	})
}
