package usecases

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
)

const JobWarehouseSweep = "section21.operations_sweep"

func (s *Service) RegisterWorkers(w *jobs.Worker) { w.Register(JobWarehouseSweep, s.handleSweep) }
func (s *Service) ScheduleMaintenance(ctx context.Context) error {
	if !s.o.Flags.Workers || s.o.Flags.KillSwitch {
		return nil
	}
	orgs, e := s.repo.OrganizationsWithActiveWarehouses(ctx)
	if e != nil {
		return e
	}
	for _, org := range orgs {
		if _, e = s.o.Queue.Enqueue(ctx, s.o.Pool, jobs.EnqueueRequest{OrganizationID: org, JobType: JobWarehouseSweep, DedupeKey: "section21:" + org + ":" + time.Now().UTC().Format("200601021504"), Payload: map[string]string{"organizationId": org}, MaxAttempts: 8}); e != nil {
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
	return s.tx(ctx, func(tx pgx.Tx) error {
		expired, e := s.repo.LockExpiredReservations(ctx, tx, p.OrganizationID)
		if e != nil {
			return e
		}
		for _, v := range expired {
			if e = s.repo.ExpireReservation(ctx, tx, v); e != nil {
				return e
			}
		}
		if e = s.repo.SweepStaleOperations(ctx, tx, p.OrganizationID); e != nil {
			return e
		}
		if ops := s.ops(); ops != nil {
			if e = ops.ReleaseExpiredTaskLeases(ctx, tx, p.OrganizationID); e != nil {
				return e
			}
		}
		for _, v := range expired {
			s.repo.InsertReservationExpiryMovement(ctx, tx, p.OrganizationID, v)
		}
		return nil
	})
}
