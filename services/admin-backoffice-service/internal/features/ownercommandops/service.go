// Package ownercommandops orchestrates the fixed source-owned administrative
// command contract for one owner. It stores no business state: it records the
// command identity, binds and consumes an independent two-person HQ approval,
// makes exactly one owner POST and reconciles the owner's immutable receipt by
// original identity. It never retries a POST and never presumes an uncertain
// outcome failed unless the owner's own version fence proves supersession.
package ownercommandops

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"cosmicforge/logistics/services/admin-backoffice-service/internal/config"
	"cosmicforge/logistics/shared/go/admincommand"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminread"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrDisabled = errors.New("owner command orchestration disabled")
var ErrConflict = errors.New("owner command identity or slot conflict")
var ErrNotFound = errors.New("owner command record not found")
var ErrProof = errors.New("owner command original outcome proof mismatch")
var ErrApproval = errors.New("owner command approval unusable")

// ApprovalWindow bounds both a pending proposal and an approved-but-unused
// approval. It is shorter than the owner's own approval acceptance window.
const ApprovalWindow = 12 * time.Hour

type Owner interface {
	Spec() admincommand.Spec
	Execute(context.Context, adminread.Query, admincommand.Command) (admincommand.Receipt, error)
	Readback(context.Context, adminread.Query, string, string, string) (admincommand.Receipt, error)
	History(context.Context, adminread.Query, string, string) (admincommand.History, error)
}

type Service struct {
	DB                        *pgxpool.Pool
	Owner                     Owner
	Policy                    *adminpolicy.Engine
	WriteEnabled, ReadEnabled bool
	Now                       func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func Scope(q adminread.Query) adminpolicy.Scope {
	return adminpolicy.Scope{Product: config.Product, Market: q.Scope.Market, Zone: q.Scope.Zone, Organization: q.Scope.Organization, Tenant: q.Scope.Tenant}
}

// Authorize checks the signed governance policy, its owner/contract binding,
// the role selected from the staff assertion and full five-scope delegation.
func (s *Service) Authorize(ctx context.Context, actor adminpolicy.Actor, q adminread.Query, capability string) error {
	if s.DB == nil || s.Owner == nil || s.Policy == nil {
		return adminpolicy.ErrBlocked
	}
	now := s.now()
	if actor.Staff != q.Staff || q.Cursor != "" || q.PolicyID != s.Policy.ID() || q.PolicyVersion != s.Policy.Version() || q.PolicyChecksum != s.Policy.Checksum() {
		return adminpolicy.ErrDenied
	}
	if _, err := adminread.Parse(q.Values(), now); err != nil {
		return err
	}
	spec := s.Owner.Spec()
	rule, err := s.Policy.Rule(capability, now)
	if err != nil || rule.Owner != spec.Owner || rule.SourceContract != spec.Contract {
		return adminpolicy.ErrBlocked
	}
	if !config.Contains(actor.Roles, q.Role) || !config.Contains(rule.Roles, q.Role) {
		return adminpolicy.ErrDenied
	}
	if err := s.Policy.AuthorizeDelegation(actor, capability, q.Purpose, Scope(q), now); err != nil {
		return err
	}
	return adminpolicy.PinRevision(ctx, s.DB, "admin_policy_revisions", s.Policy)
}

type record struct {
	admincommand.Operation
	key, intent, digest, staff, purpose, claim, versionKey string
	command                                                admincommand.Command
}

const columns = `id::text,owner,action,subject_kind,subject_id,case_ref,expected_version,COALESCE(approval_id::text,''),product,market,zone,organization_ref,tenant_ref,status,COALESCE(rejection_code,''),created_at,updated_at,idempotency_key,intent_digest,operation_digest,staff_ref,purpose_code,claim_token::text,version_key,command,receipt`

func load(row pgx.Row) (record, error) {
	var r record
	var command, receipt []byte
	err := row.Scan(&r.OperationID, &r.Owner, &r.Action, &r.SubjectKind, &r.SubjectID, &r.CaseRef, &r.ExpectedVersion, &r.ApprovalID, &r.Scope.Product, &r.Scope.Market, &r.Scope.Zone, &r.Scope.Organization, &r.Scope.Tenant, &r.Status, &r.RejectionCode, &r.CreatedAt, &r.AsOf, &r.key, &r.intent, &r.digest, &r.staff, &r.purpose, &r.claim, &r.versionKey, &command, &receipt)
	if err != nil {
		return r, err
	}
	r.CreatedAt, r.AsOf = r.CreatedAt.UTC(), r.AsOf.UTC()
	if json.Unmarshal(command, &r.command) != nil {
		return r, ErrProof
	}
	if receipt != nil {
		out, err := admincommand.DecodeReceipt(receipt)
		if err != nil {
			return r, ErrProof
		}
		r.Receipt = &out
	}
	return r, nil
}

func (s *Service) audit(ctx context.Context, tx pgx.Tx, r record, q adminread.Query, outcome string) error {
	_, err := tx.Exec(ctx, `INSERT INTO admin_owner_command_operation_audit(operation_id,staff_ref,purpose_code,product,market,zone,organization_ref,tenant_ref,policy_id,policy_version,policy_checksum,outcome) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, r.OperationID, r.staff, r.purpose, r.Scope.Product, r.Scope.Market, r.Scope.Zone, r.Scope.Organization, r.Scope.Tenant, q.PolicyID, q.PolicyVersion, q.PolicyChecksum, outcome)
	return err
}

// Submit claims durable ownership of the operation (consuming its approval in
// the same transaction) before exactly one owner POST. A replay of an existing
// identity only reconciles; it never sends a second POST.
func (s *Service) Submit(ctx context.Context, actor adminpolicy.Actor, q adminread.Query, in admincommand.Submission) (admincommand.Operation, error) {
	if !s.WriteEnabled || !s.ReadEnabled {
		return admincommand.Operation{}, ErrDisabled
	}
	spec := s.Owner.Spec()
	cmd := in.Command()
	action, ok := spec.Actions[cmd.Action]
	if !ok {
		return admincommand.Operation{}, admincommand.ErrInvalid
	}
	if action.RequiresApproval != (in.ApprovalID != "") {
		return admincommand.Operation{}, admincommand.ErrInvalid
	}
	// Validate everything except approval presence before touching storage.
	probe := cmd
	if action.RequiresApproval {
		probe.Approval = &admincommand.Approval{ApprovalID: in.ApprovalID, ApproverRef: "pending-approval", IntentDigest: cmd.IntentDigest(actor.Staff, q.Purpose, Scope(q)), ApprovedAt: s.now()}
	}
	if _, err := probe.Validate(spec, s.now()); err != nil {
		return admincommand.Operation{}, err
	}
	if err := s.Authorize(ctx, actor, q, spec.Capability); err != nil {
		return admincommand.Operation{}, err
	}
	if err := s.walletRemedyConfirmed(ctx, q, cmd); err != nil {
		return admincommand.Operation{}, err
	}
	r, won, err := s.begin(ctx, actor, q, cmd, action, in.ApprovalID)
	if err != nil {
		return admincommand.Operation{}, err
	}
	if !won {
		return s.Reconcile(ctx, actor, q, r.OperationID)
	}
	receipt, err := s.Owner.Execute(ctx, q, r.command)
	if errors.Is(err, admincommand.ErrRefused) {
		// The owner answered definitively before committing anything: its
		// transaction rolled back, so the versioned slot is released.
		return s.settleRejected(ctx, r, q, "owner_refused")
	}
	if err != nil {
		s.uncertain(ctx, r, q)
		r.Status = "UNKNOWN_OUTCOME"
		return r.Operation, nil
	}
	if !admincommand.Matches(receipt, r.command, q) || receipt.Owner != spec.Owner {
		s.uncertain(ctx, r, q)
		return admincommand.Operation{}, ErrProof
	}
	return s.confirm(ctx, r, q, receipt)
}

func (s *Service) begin(ctx context.Context, actor adminpolicy.Actor, q adminread.Query, cmd admincommand.Command, action admincommand.ActionSpec, approvalID string) (record, bool, error) {
	spec := s.Owner.Spec()
	scope := Scope(q)
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return record{}, false, err
	}
	defer tx.Rollback(ctx)
	intent := cmd.IntentDigest(actor.Staff, q.Purpose, scope)
	// An existing identity is reconciled, never re-executed or re-approved.
	existing, err := load(tx.QueryRow(ctx, `SELECT `+columns+` FROM admin_owner_command_operations WHERE idempotency_key=$1`, cmd.IdempotencyKey))
	if err == nil {
		stored := existing.command
		stored.Approval = nil
		if existing.OperationID != cmd.OperationID || existing.Owner != spec.Owner || existing.staff != actor.Staff || existing.purpose != q.Purpose || existing.Scope != scope || existing.intent != intent || existing.ApprovalID != approvalID || stored.OperationDigest(actor.Staff, q.Purpose, scope) != cmd.OperationDigest(actor.Staff, q.Purpose, scope) {
			return record{}, false, ErrConflict
		}
		return existing, false, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return record{}, false, err
	}
	now := s.now()
	if approvalID != "" {
		var status, owner, requester, approver, digest, purpose string
		var product, market, zone, organization, tenant string
		var expires time.Time
		var decided *time.Time
		err := tx.QueryRow(ctx, `SELECT status,owner,requester_ref,COALESCE(approver_ref,''),intent_digest,purpose_code,product,market,zone,organization_ref,tenant_ref,expires_at,decided_at FROM admin_command_approvals WHERE id=$1 FOR UPDATE`, approvalID).Scan(&status, &owner, &requester, &approver, &digest, &purpose, &product, &market, &zone, &organization, &tenant, &expires, &decided)
		if errors.Is(err, pgx.ErrNoRows) {
			return record{}, false, ErrApproval
		}
		if err != nil {
			return record{}, false, err
		}
		if status != "APPROVED" || owner != spec.Owner || requester != actor.Staff || approver == "" || approver == actor.Staff || digest != intent || purpose != q.Purpose || (adminpolicy.Scope{Product: product, Market: market, Zone: zone, Organization: organization, Tenant: tenant}) != scope || !now.Before(expires) || decided == nil {
			return record{}, false, ErrApproval
		}
		cmd.Approval = &admincommand.Approval{ApprovalID: approvalID, ApproverRef: approver, IntentDigest: digest, ApprovedAt: decided.UTC().Truncate(time.Microsecond)}
	}
	if _, err := cmd.Validate(spec, now); err != nil {
		return record{}, false, err
	}
	raw, err := json.Marshal(cmd)
	if err != nil {
		return record{}, false, err
	}
	versionKey := ""
	if action.VersionOf == admincommand.VersionCase {
		versionKey = cmd.CaseRef
	}
	var approvalArg any
	if approvalID != "" {
		approvalArg = approvalID
	}
	claim := uuid.NewString()
	_, err = tx.Exec(ctx, `INSERT INTO admin_owner_command_operations(id,principal,owner,contract,staff_ref,purpose_code,product,market,zone,organization_ref,tenant_ref,subject_kind,subject_id,case_ref,action,version_key,expected_version,idempotency_key,intent_digest,operation_digest,approval_id,command,status,claim_token) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,'EXECUTING',$23)`,
		cmd.OperationID, config.HQPrincipal, spec.Owner, spec.Contract, actor.Staff, q.Purpose, scope.Product, scope.Market, scope.Zone, scope.Organization, scope.Tenant, cmd.SubjectKind, cmd.SubjectID, cmd.CaseRef, cmd.Action, versionKey, cmd.ExpectedVersion, cmd.IdempotencyKey, intent, cmd.OperationDigest(actor.Staff, q.Purpose, scope), approvalArg, raw, claim)
	if err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23505" {
			// A live operation already holds this versioned slot (or the id/approval).
			return record{}, false, ErrConflict
		}
		return record{}, false, err
	}
	r, err := load(tx.QueryRow(ctx, `SELECT `+columns+` FROM admin_owner_command_operations WHERE id=$1`, cmd.OperationID))
	if err != nil {
		return record{}, false, err
	}
	if approvalID != "" {
		if _, err := tx.Exec(ctx, `UPDATE admin_command_approvals SET status='CONSUMED',consumed_operation_id=$2,updated_at=now() WHERE id=$1 AND status='APPROVED'`, approvalID, cmd.OperationID); err != nil {
			return record{}, false, err
		}
		if err := s.approvalAudit(ctx, tx, approvalID, actor.Staff, "CONSUMED", cmd.ReasonCode, q); err != nil {
			return record{}, false, err
		}
	}
	if err := s.audit(ctx, tx, r, q, "EXECUTING"); err != nil {
		return record{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return record{}, false, err
	}
	return r, true, nil
}

func (s *Service) uncertain(ctx context.Context, r record, q adminread.Query) {
	// The persisted EXECUTING state is itself a no-retry fence if this
	// best-effort transition cannot commit after the caller disconnects.
	detached, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	tx, err := s.DB.Begin(detached)
	if err != nil {
		return
	}
	defer tx.Rollback(detached)
	result, err := tx.Exec(detached, `UPDATE admin_owner_command_operations SET status='UNKNOWN_OUTCOME',updated_at=now() WHERE id=$1 AND claim_token=$2 AND status='EXECUTING'`, r.OperationID, r.claim)
	if err != nil || result.RowsAffected() != 1 || s.audit(detached, tx, r, q, "UNKNOWN_OUTCOME") != nil {
		return
	}
	_ = tx.Commit(detached)
}

func (s *Service) settleRejected(ctx context.Context, r record, q adminread.Query, code string) (admincommand.Operation, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return admincommand.Operation{}, err
	}
	defer tx.Rollback(ctx)
	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM admin_owner_command_operations WHERE id=$1 AND claim_token=$2 FOR UPDATE`, r.OperationID, r.claim).Scan(&status); err != nil {
		return admincommand.Operation{}, err
	}
	if status == "CONFIRMED" {
		return admincommand.Operation{}, ErrProof
	}
	if status != "REJECTED" {
		if err := tx.QueryRow(ctx, `UPDATE admin_owner_command_operations SET status='REJECTED',rejection_code=$3,updated_at=now() WHERE id=$1 AND claim_token=$2 RETURNING updated_at`, r.OperationID, r.claim, code).Scan(&r.AsOf); err != nil {
			return admincommand.Operation{}, err
		}
		if err := s.audit(ctx, tx, r, q, "REJECTED"); err != nil {
			return admincommand.Operation{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return admincommand.Operation{}, err
	}
	r.Status, r.RejectionCode, r.AsOf = "REJECTED", code, r.AsOf.UTC()
	return r.Operation, nil
}

func (s *Service) confirm(ctx context.Context, r record, q adminread.Query, receipt admincommand.Receipt) (admincommand.Operation, error) {
	raw, err := json.Marshal(receipt)
	if err != nil {
		return admincommand.Operation{}, err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return admincommand.Operation{}, err
	}
	defer tx.Rollback(ctx)
	var status string
	var previous []byte
	if err := tx.QueryRow(ctx, `SELECT status,receipt FROM admin_owner_command_operations WHERE id=$1 AND claim_token=$2 FOR UPDATE`, r.OperationID, r.claim).Scan(&status, &previous); err != nil {
		return admincommand.Operation{}, err
	}
	switch status {
	case "REJECTED":
		// A committed owner receipt after a recorded refusal is a proof failure.
		return admincommand.Operation{}, ErrProof
	case "CONFIRMED":
		stored, err := admincommand.DecodeReceipt(previous)
		if err != nil || stored != receipt {
			return admincommand.Operation{}, ErrProof
		}
		if err := s.audit(ctx, tx, r, q, "RECONCILED"); err != nil {
			return admincommand.Operation{}, err
		}
		r.AsOf = s.now()
	default:
		if err := tx.QueryRow(ctx, `UPDATE admin_owner_command_operations SET status='CONFIRMED',receipt=$3,updated_at=now() WHERE id=$1 AND claim_token=$2 RETURNING updated_at`, r.OperationID, r.claim, raw).Scan(&r.AsOf); err != nil {
			return admincommand.Operation{}, err
		}
		if err := s.audit(ctx, tx, r, q, "CONFIRMED"); err != nil {
			return admincommand.Operation{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return admincommand.Operation{}, err
	}
	r.Status, r.AsOf, r.Receipt = "CONFIRMED", r.AsOf.UTC(), &receipt
	return r.Operation, nil
}

// Reconcile never calls Execute. Current delegated and owner read authority is
// required on every call. For an unresolved operation, the subject's history
// is read BEFORE the original-identity readback: if the owner's version has
// already moved past the expected version and the original identity is still
// absent afterwards, the owner's version fence proves it can never commit.
func (s *Service) Reconcile(ctx context.Context, actor adminpolicy.Actor, q adminread.Query, id string) (admincommand.Operation, error) {
	if !s.ReadEnabled {
		return admincommand.Operation{}, ErrDisabled
	}
	if !admincommand.CanonicalID(id) {
		return admincommand.Operation{}, ErrNotFound
	}
	spec := s.Owner.Spec()
	if err := s.Authorize(ctx, actor, q, spec.ReadCapability); err != nil {
		return admincommand.Operation{}, err
	}
	scope := Scope(q)
	r, err := load(s.DB.QueryRow(ctx, `SELECT `+columns+` FROM admin_owner_command_operations WHERE id=$1 AND owner=$2 AND staff_ref=$3 AND purpose_code=$4 AND product=$5 AND market=$6 AND zone=$7 AND organization_ref=$8 AND tenant_ref=$9`, id, spec.Owner, actor.Staff, q.Purpose, scope.Product, scope.Market, scope.Zone, scope.Organization, scope.Tenant))
	if errors.Is(err, pgx.ErrNoRows) {
		return admincommand.Operation{}, ErrNotFound
	}
	if err != nil {
		return admincommand.Operation{}, err
	}
	if r.Status == "REJECTED" {
		return r.Operation, nil
	}
	var history admincommand.History
	var historyErr error
	if r.Status != "CONFIRMED" {
		history, historyErr = s.Owner.History(ctx, q, r.SubjectKind, r.SubjectID)
	}
	receipt, err := s.Owner.Readback(ctx, q, r.SubjectKind, r.SubjectID, r.key)
	if errors.Is(err, admincommand.ErrAbsent) && r.Status != "CONFIRMED" && historyErr == nil && superseded(history, r) {
		return s.settleRejected(ctx, r, q, "superseded_by_owner_version")
	}
	if errors.Is(err, admincommand.ErrAbsent) && r.Status != "CONFIRMED" {
		return r.Operation, nil
	}
	if err != nil {
		return admincommand.Operation{}, err
	}
	if !admincommand.Matches(receipt, r.command, q) || receipt.Owner != spec.Owner {
		return admincommand.Operation{}, ErrProof
	}
	return s.confirm(ctx, r, q, receipt)
}

func superseded(h admincommand.History, r record) bool {
	if r.versionKey == "" {
		return h.CurrentVersion > r.ExpectedVersion
	}
	for _, d := range h.Decisions {
		if d.CaseRef == r.versionKey && d.RecordedVersion > r.ExpectedVersion && d.IdempotencyKey != r.key {
			if action, ok := spec(r.Owner).Actions[d.Action]; ok && action.VersionOf == admincommand.VersionCase {
				return true
			}
		}
	}
	return false
}
func spec(owner string) admincommand.Spec {
	s, _ := admincommand.ForOwner(owner)
	return s
}

// History is a pass-through of the owner's own immutable decision history.
func (s *Service) History(ctx context.Context, actor adminpolicy.Actor, q adminread.Query, kind, id string) (admincommand.History, error) {
	if !s.ReadEnabled {
		return admincommand.History{}, ErrDisabled
	}
	if err := s.Authorize(ctx, actor, q, s.Owner.Spec().ReadCapability); err != nil {
		return admincommand.History{}, err
	}
	return s.Owner.History(ctx, q, kind, id)
}
