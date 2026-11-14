package usecases

import (
	"context"
	"encoding/json"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

const JobProofReconcile = "section19.proof_reconcile"

func (s *Service) RegisterWorkers(w *jobs.Worker) { w.Register(JobProofReconcile, s.handleReconcile) }
func (s *Service) ScheduleMaintenance(ctx context.Context) error {
	if !s.o.Flags.Workers || s.o.Flags.KillSwitch {
		return nil
	}
	orgs, e := s.repo.ActiveOrganizationIDs(ctx)
	if e != nil {
		return e
	}
	for _, org := range orgs {
		if _, e = s.o.Queue.Enqueue(ctx, s.o.Pool, jobs.EnqueueRequest{OrganizationID: org, JobType: JobProofReconcile, DedupeKey: "section19-proof:" + org + ":" + time.Now().UTC().Format("200601021504"), Payload: map[string]string{"organizationId": org}, MaxAttempts: 8}); e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) handleReconcile(ctx context.Context, j jobs.Job) error {
	var p struct {
		OrganizationID string `json:"organizationId"`
	}
	if e := json.Unmarshal(j.Payload, &p); e != nil {
		return e
	}
	if p.OrganizationID != j.OrganizationID {
		return jobs.ErrTenantMismatch
	}
	if s.o.Media == nil || !s.o.Media.Configured() {
		return platformclients.ErrNotConfigured
	}
	pending, e := s.repo.ListPendingMediaRecords(ctx, p.OrganizationID)
	if e != nil {
		return e
	}
	for _, r := range pending {
		m, x := s.o.Media.GetFile(ctx, platformclients.CallMeta{OrganizationID: p.OrganizationID}, r.MediaFileID)
		if x != nil {
			return x
		}
		status := "REJECTED"
		reason := "[\"MEDIA_UNSAFE_OR_UNOWNED\"]"
		if m.Ready() && m.OwnerID == p.OrganizationID {
			status = "VERIFIED"
			reason = "[\"MEDIA_SAFE_AND_OWNED\"]"
		}
		if e = s.repo.InsertMediaReconciliation(ctx, p.OrganizationID, r.ID, status, r.MediaFileID, r.SourceVersion, reason); e != nil {
			return e
		}
	}
	return nil
}
