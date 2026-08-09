package workforce

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

const JobMaintenance = "section17.maintenance"

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
		_, err = s.o.Queue.Enqueue(ctx, s.o.Pool, jobs.EnqueueRequest{OrganizationID: org, JobType: JobMaintenance, DedupeKey: "section17:" + org + ":" + time.Now().UTC().Format("200601021504"), Payload: map[string]string{"organizationId": org}, MaxAttempts: 8})
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
	return s.GenerateDuePerformance(ctx, p.OrganizationID)
}
func (s *Service) GenerateDuePerformance(ctx context.Context, org string) error {
	if !s.o.Flags.Performance || s.o.Flags.KillSwitch {
		return nil
	}
	end := s.o.Now().UTC().Truncate(24 * time.Hour)
	start := end.Add(-24 * time.Hour)
	rows, e := s.o.Pool.Query(ctx, `SELECT membership_id FROM fleet_partner_workforce_profiles WHERE organization_id=$1 AND status='ACTIVE'`, org)
	if e != nil {
		return e
	}
	members := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		members = append(members, id)
	}
	rows.Close()
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
	statements := []string{
		`UPDATE fleet_partner_shifts SET status='IN_PROGRESS',version=version+1,updated_at=now() WHERE organization_id=$1 AND status='PUBLISHED' AND start_at<=now() AND end_at>now()`,
		`UPDATE fleet_partner_shifts SET status='COMPLETED',version=version+1,updated_at=now() WHERE organization_id=$1 AND status IN('PUBLISHED','IN_PROGRESS') AND end_at<=now()`,
		`UPDATE fleet_partner_leave_records SET status='COMPLETED',version=version+1,updated_at=now() WHERE organization_id=$1 AND status='APPROVED' AND end_at<=now()`,
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		for _, statement := range statements {
			if _, e := tx.Exec(ctx, statement, org); e != nil {
				return e
			}
		}
		scope := tenant.Context{OrganizationID: org, ActorType: tenant.ActorTypeSystem, ActorID: "section17-maintenance-worker", CorrelationID: "section17-maintenance"}
		changes := []struct{ sql, event, kind, status string }{{`UPDATE fleet_partner_shift_assignments SET status='NO_SHOW',version=version+1,updated_at=now() WHERE organization_id=$1 AND status='CONFIRMED' AND expected_end<=now() RETURNING id,version`, `shift_assignment.no_show.v1`, `shift_assignment`, `NO_SHOW`}, {`UPDATE fleet_partner_provider_qualifications SET status='EXPIRED',version=version+1,updated_at=now() WHERE organization_id=$1 AND status='VERIFIED' AND expires_at<=now() RETURNING id,version`, `provider_qualification.expired.v1`, `provider_qualification`, `EXPIRED`}, {`UPDATE fleet_partner_training_completions SET status='EXPIRED',version=version+1,updated_at=now() WHERE organization_id=$1 AND status='PASSED' AND expires_at<=now() RETURNING id,version`, `training.expired.v1`, `training_completion`, `EXPIRED`}}
		for _, change := range changes {
			rows, e := tx.Query(ctx, change.sql, org)
			if e != nil {
				return e
			}
			var updated []struct {
				id      string
				version int64
			}
			for rows.Next() {
				var item struct {
					id      string
					version int64
				}
				if e = rows.Scan(&item.id, &item.version); e != nil {
					rows.Close()
					return e
				}
				updated = append(updated, item)
			}
			rows.Close()
			if e = rows.Err(); e != nil {
				return e
			}
			for _, item := range updated {
				if e = s.record(ctx, tx, scope, change.event, change.kind, item.id, item.version, map[string]any{"id": item.id, "status": change.status}); e != nil {
					return e
				}
			}
		}
		return nil
	})
}

var _ = time.Second
