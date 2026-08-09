package proof

import (
	"context"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
	"encoding/json"
	"time"
)

const JobProofReconcile = "section19.proof_reconcile"

func (s *Service) RegisterWorkers(w *jobs.Worker) { w.Register(JobProofReconcile, s.handleReconcile) }
func (s *Service) ScheduleMaintenance(ctx context.Context) error {
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
		_, e = s.o.Queue.Enqueue(ctx, s.o.Pool, jobs.EnqueueRequest{OrganizationID: org, JobType: JobProofReconcile, DedupeKey: "section19-proof:" + org + ":" + time.Now().UTC().Format("200601021504"), Payload: map[string]string{"organizationId": org}, MaxAttempts: 8})
		if e != nil {
			return e
		}
	}
	return rows.Err()
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
	rows, e := s.o.Pool.Query(ctx, `SELECT r.id,r.media_file_id,r.source_version FROM fleet_partner_proof_records r WHERE r.organization_id=$1 AND r.media_file_id<>'' AND NOT EXISTS(SELECT 1 FROM fleet_partner_proof_verifications v WHERE v.proof_record_id=r.id AND v.status='VERIFIED') LIMIT 100`, p.OrganizationID)
	if e != nil {
		return e
	}
	defer rows.Close()
	for rows.Next() {
		var id, media string
		var version int64
		if e = rows.Scan(&id, &media, &version); e != nil {
			return e
		}
		m, x := s.o.Media.GetFile(ctx, platformclients.CallMeta{OrganizationID: p.OrganizationID}, media)
		if x != nil {
			return x
		}
		status := "REJECTED"
		reason := "[\"MEDIA_UNSAFE_OR_UNOWNED\"]"
		if m.Ready() && m.OwnerID == p.OrganizationID {
			status = "VERIFIED"
			reason = "[\"MEDIA_SAFE_AND_OWNED\"]"
		}
		_, e = s.o.Pool.Exec(ctx, `INSERT INTO fleet_partner_proof_verifications(id,organization_id,proof_record_id,status,authority_type,authority_reference,source_version,reason_codes)VALUES($1,$2,$3,$4,'MEDIA',$5,$6,$7) ON CONFLICT DO NOTHING`, ids.New("prfver"), p.OrganizationID, id, status, media, version, reason)
		if e != nil {
			return e
		}
	}
	return rows.Err()
}
