package ownercommandops

import (
	"context"
	"encoding/json"
	"errors"

	"cosmicforge/logistics/shared/go/admincommand"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminread"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func (s *Service) approvalAudit(ctx context.Context, tx pgx.Tx, id, actor, event, reason string, q adminread.Query) error {
	_, err := tx.Exec(ctx, `INSERT INTO admin_command_approval_audit(approval_id,actor_ref,event,reason_code,policy_id,policy_version,policy_checksum) VALUES($1,$2,$3,$4,$5,$6,$7)`, id, actor, event, reason, q.PolicyID, q.PolicyVersion, q.PolicyChecksum)
	return err
}

const approvalColumns = `id::text,owner,action,subject_kind,subject_id,case_ref,expected_version,intent_digest,status,product,market,zone,organization_ref,tenant_ref,expires_at,decided_at,created_at,requester_ref,purpose_code`

func loadApproval(row pgx.Row) (admincommand.ApprovalView, string, string, error) {
	var v admincommand.ApprovalView
	var requester, purpose string
	err := row.Scan(&v.ApprovalID, &v.Owner, &v.Action, &v.SubjectKind, &v.SubjectID, &v.CaseRef, &v.ExpectedVersion, &v.IntentDigest, &v.Status, &v.Scope.Product, &v.Scope.Market, &v.Scope.Zone, &v.Scope.Organization, &v.Scope.Tenant, &v.ExpiresAt, &v.DecidedAt, &v.CreatedAt, &requester, &purpose)
	v.ExpiresAt, v.CreatedAt = v.ExpiresAt.UTC(), v.CreatedAt.UTC()
	if v.DecidedAt != nil {
		decided := v.DecidedAt.UTC()
		v.DecidedAt = &decided
	}
	return v, requester, purpose, err
}

// Propose records one exact intent awaiting a second staff member. Only the
// requester can later execute it, and only for exactly this intent digest.
func (s *Service) Propose(ctx context.Context, actor adminpolicy.Actor, q adminread.Query, p admincommand.Proposal) (admincommand.ApprovalView, error) {
	if !s.WriteEnabled || !s.ReadEnabled {
		return admincommand.ApprovalView{}, ErrDisabled
	}
	spec := s.Owner.Spec()
	cmd := p.Command()
	action, ok := spec.Actions[cmd.Action]
	if !ok || !action.RequiresApproval {
		return admincommand.ApprovalView{}, admincommand.ErrInvalid
	}
	now := s.now()
	scope := Scope(q)
	intent := cmd.IntentDigest(actor.Staff, q.Purpose, scope)
	probe := cmd
	probe.OperationID, probe.IdempotencyKey, probe.RequestedAt = p.ApprovalID, p.ApprovalID, now
	probe.Approval = &admincommand.Approval{ApprovalID: p.ApprovalID, ApproverRef: "pending-approval", IntentDigest: intent, ApprovedAt: now}
	if _, err := probe.Validate(spec, now); err != nil {
		return admincommand.ApprovalView{}, err
	}
	if err := s.Authorize(ctx, actor, q, spec.Capability); err != nil {
		return admincommand.ApprovalView{}, err
	}
	raw, err := json.Marshal(cmd)
	if err != nil {
		return admincommand.ApprovalView{}, err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return admincommand.ApprovalView{}, err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `INSERT INTO admin_command_approvals(id,owner,contract,requester_ref,purpose_code,product,market,zone,organization_ref,tenant_ref,subject_kind,subject_id,case_ref,action,expected_version,intent_digest,intent,status,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,'PENDING',$18) ON CONFLICT(id) DO NOTHING`,
		p.ApprovalID, spec.Owner, spec.Contract, actor.Staff, q.Purpose, scope.Product, scope.Market, scope.Zone, scope.Organization, scope.Tenant, cmd.SubjectKind, cmd.SubjectID, cmd.CaseRef, cmd.Action, cmd.ExpectedVersion, intent, raw, now.Add(ApprovalWindow))
	if err != nil {
		return admincommand.ApprovalView{}, err
	}
	view, requester, purpose, err := loadApproval(tx.QueryRow(ctx, `SELECT `+approvalColumns+` FROM admin_command_approvals WHERE id=$1`, p.ApprovalID))
	if err != nil {
		return admincommand.ApprovalView{}, err
	}
	if requester != actor.Staff || purpose != q.Purpose || view.Owner != spec.Owner || view.Scope != scope || view.IntentDigest != intent {
		return admincommand.ApprovalView{}, ErrConflict
	}
	if tag.RowsAffected() == 1 {
		if err := s.approvalAudit(ctx, tx, p.ApprovalID, actor.Staff, "PROPOSED", cmd.ReasonCode, q); err != nil {
			return admincommand.ApprovalView{}, err
		}
	}
	return view, tx.Commit(ctx)
}

// Decide is performed by a different staff member who independently holds
// the owner's approve capability for the same five-scope. A decision is final.
func (s *Service) Decide(ctx context.Context, actor adminpolicy.Actor, q adminread.Query, id string, d admincommand.ApprovalDecision) (admincommand.ApprovalView, error) {
	if !s.WriteEnabled || !s.ReadEnabled {
		return admincommand.ApprovalView{}, ErrDisabled
	}
	if !admincommand.CanonicalID(id) {
		return admincommand.ApprovalView{}, ErrNotFound
	}
	spec := s.Owner.Spec()
	if err := s.Authorize(ctx, actor, q, spec.ApproveCapability); err != nil {
		return admincommand.ApprovalView{}, err
	}
	scope := Scope(q)
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return admincommand.ApprovalView{}, err
	}
	defer tx.Rollback(ctx)
	view, requester, _, err := loadApproval(tx.QueryRow(ctx, `SELECT `+approvalColumns+` FROM admin_command_approvals WHERE id=$1 AND owner=$2 AND product=$3 AND market=$4 AND zone=$5 AND organization_ref=$6 AND tenant_ref=$7 FOR UPDATE`, id, spec.Owner, scope.Product, scope.Market, scope.Zone, scope.Organization, scope.Tenant))
	if errors.Is(err, pgx.ErrNoRows) {
		return admincommand.ApprovalView{}, ErrNotFound
	}
	if err != nil {
		return admincommand.ApprovalView{}, err
	}
	if requester == actor.Staff {
		return admincommand.ApprovalView{}, adminpolicy.ErrDenied
	}
	now := s.now()
	if view.Status != "PENDING" || !now.Before(view.ExpiresAt) {
		return admincommand.ApprovalView{}, ErrApproval
	}
	status, event := "APPROVED", "APPROVED"
	if d.Decision == "REJECT" {
		status, event = "REJECTED", "REJECTED"
	}
	if _, err := tx.Exec(ctx, `UPDATE admin_command_approvals SET status=$2,approver_ref=$3,decided_at=$4,updated_at=now() WHERE id=$1 AND status='PENDING'`, id, status, actor.Staff, now); err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23514" {
			return admincommand.ApprovalView{}, adminpolicy.ErrDenied
		}
		return admincommand.ApprovalView{}, err
	}
	if err := s.approvalAudit(ctx, tx, id, actor.Staff, event, d.ReasonCode, q); err != nil {
		return admincommand.ApprovalView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return admincommand.ApprovalView{}, err
	}
	view.Status = status
	decided := now.UTC()
	view.DecidedAt = &decided
	return view, nil
}

// Approval returns an approval's current state to any staff member who holds
// the owner's read capability in exactly the same five-scope.
func (s *Service) Approval(ctx context.Context, actor adminpolicy.Actor, q adminread.Query, id string) (admincommand.ApprovalView, error) {
	if !s.ReadEnabled {
		return admincommand.ApprovalView{}, ErrDisabled
	}
	if !admincommand.CanonicalID(id) {
		return admincommand.ApprovalView{}, ErrNotFound
	}
	spec := s.Owner.Spec()
	if err := s.Authorize(ctx, actor, q, spec.ReadCapability); err != nil {
		return admincommand.ApprovalView{}, err
	}
	scope := Scope(q)
	view, _, _, err := loadApproval(s.DB.QueryRow(ctx, `SELECT `+approvalColumns+` FROM admin_command_approvals WHERE id=$1 AND owner=$2 AND product=$3 AND market=$4 AND zone=$5 AND organization_ref=$6 AND tenant_ref=$7`, id, spec.Owner, scope.Product, scope.Market, scope.Zone, scope.Organization, scope.Tenant))
	if errors.Is(err, pgx.ErrNoRows) {
		return admincommand.ApprovalView{}, ErrNotFound
	}
	if err == nil && view.Status == "PENDING" && !s.now().Before(view.ExpiresAt) {
		view.Status = "EXPIRED"
	}
	return view, err
}
