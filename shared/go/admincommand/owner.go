package admincommand

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"cosmicforge/logistics/shared/go/admincontext"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var Schema string

// SchemaSHA256 lets each owner's migration prove it embeds this exact ledger.
func SchemaSHA256() string {
	sum := sha256.Sum256([]byte(Schema))
	return hex.EncodeToString(sum[:])
}

var ErrDisabled = errors.New("administrative command capability disabled")
var ErrConflict = errors.New("administrative command identity or version conflict")
var ErrNotFound = errors.New("administrative command subject or decision not found")

// ErrRejected is a deliberate domain refusal (illegal transition, unsafe
// custody, ineligible replacement). It is distinct from a version conflict so
// HQ can tell "re-read and decide again" from "this action is not allowed".
var ErrRejected = errors.New("administrative command rejected by owner state machine")

// Execution is the authorized, validated intent handed to the owner's own
// domain executor, inside the transaction that will also hold the ledger row.
type Execution struct {
	Command
	Spec       ActionSpec
	DecisionID string
	Staff      string
	Purpose    string
	Scope      adminpolicy.Scope
	Now        time.Time
}

// Transition is the owner's report of what its state machine did.
type Transition struct {
	PriorVersion   int64
	PriorState     string
	ResultingState string
	Outcome        string
}

// Executor implements one owner's closed action dictionary. Apply must lock
// the versioned entity, compare it with ExpectedVersion (ErrConflict when
// different), perform the domain effect and write ExpectedVersion+1 in tx.
// Returning any error rolls back both the domain effect and the ledger row.
type Executor interface {
	Apply(ctx context.Context, tx pgx.Tx, in Execution) (Transition, error)
	Current(ctx context.Context, tx pgx.Tx, kind, id string) (int64, string, error)
}

type Service struct {
	Spec                      Spec
	DB                        *pgxpool.Pool
	Policy                    *adminpolicy.Engine
	Contexts                  *admincontext.Store
	Executor                  Executor
	WriteEnabled, ReadEnabled bool
	Now                       func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func (s *Service) ready(ctx context.Context, actor adminpolicy.Actor, capability, purpose string, scope adminpolicy.Scope, now time.Time) error {
	if s.DB == nil || s.Policy == nil || s.Contexts == nil || s.Contexts.Reader == nil || s.Executor == nil || s.Contexts.Authority.Owner != s.Spec.Owner {
		return adminpolicy.ErrBlocked
	}
	if err := s.Policy.AuthorizeDelegation(actor, capability, purpose, scope, now); err != nil {
		return err
	}
	return adminpolicy.PinRevision(ctx, s.DB, "admin_owner_policy_revisions", s.Policy)
}

// authorize resolves the subject's current, independently signed owner
// context. The subject must still be bound to exactly this scope through the
// owner's own resource contract; only then does the command contract apply.
func (s *Service) authorize(ctx context.Context, tx pgx.Tx, actor adminpolicy.Actor, purpose, kind, id, capability string, scope adminpolicy.Scope, now time.Time) error {
	expected, ok := s.Spec.SubjectContract(kind)
	if !ok {
		return ErrNotFound
	}
	facts, err := s.Contexts.Resolve(ctx, tx, kind, id, capability, scope, now)
	if err != nil {
		return err
	}
	if facts.SourceContract != expected || facts.Reference != id {
		return adminpolicy.ErrDenied
	}
	facts.SourceContract = s.Spec.Contract
	return s.Policy.AuthorizeOwner(actor, capability, purpose, scope, facts, now)
}

const ledgerColumns = `idempotency_key,operation_id,decision_id,owner,contract,subject_kind,subject_id,case_ref,action,version_scope,version_key,recorded_version,operation_digest,intent_digest,approval_id,approver_ref,staff_ref,purpose_code,product,market,zone,organization_ref,tenant_ref,policy_id,policy_version,policy_checksum,receipt`

// Execute runs exactly one owner transition and its immutable ledger row in
// one transaction. A replay with the identical operation returns the original
// receipt without re-executing; any differing reuse is an identity conflict.
func (s *Service) Execute(ctx context.Context, actor adminpolicy.Actor, purpose string, scope adminpolicy.Scope, c Command) (Receipt, error) {
	var empty Receipt
	if !s.WriteEnabled {
		return empty, ErrDisabled
	}
	now := s.now()
	action, err := c.Validate(s.Spec, now)
	if err != nil {
		return empty, err
	}
	if err := s.ready(ctx, actor, s.Spec.Capability, purpose, scope, now); err != nil {
		return empty, err
	}
	intent := c.IntentDigest(actor.Staff, purpose, scope)
	operation := c.OperationDigest(actor.Staff, purpose, scope)
	if c.Approval != nil && (c.Approval.ApproverRef == actor.Staff || c.Approval.IntentDigest != intent) {
		return empty, adminpolicy.ErrDenied
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback(ctx)
	// Serialize every command on one subject before any read; concurrent
	// commands then observe each other's committed version deterministically.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, s.Spec.Owner+"|"+c.SubjectKind+"|"+c.SubjectID); err != nil {
		return empty, err
	}
	var raw []byte
	var storedDigest string
	err = tx.QueryRow(ctx, `SELECT operation_digest,receipt FROM admin_owner_command_ledger WHERE idempotency_key=$1`, c.IdempotencyKey).Scan(&storedDigest, &raw)
	if err == nil {
		// Current authority is still required to learn an existing outcome.
		if err := s.authorize(ctx, tx, actor, purpose, c.SubjectKind, c.SubjectID, s.Spec.Capability, scope, now); err != nil {
			return empty, err
		}
		receipt, decodeErr := DecodeReceipt(raw)
		if decodeErr != nil || storedDigest != operation || receipt.Scope != scope {
			return empty, ErrConflict
		}
		return receipt, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return empty, err
	}
	if err := s.authorize(ctx, tx, actor, purpose, c.SubjectKind, c.SubjectID, s.Spec.Capability, scope, now); err != nil {
		return empty, err
	}
	decision := uuid.NewString()
	in := Execution{Command: c, Spec: action, DecisionID: decision, Staff: actor.Staff, Purpose: purpose, Scope: scope, Now: now}
	tr, err := s.Executor.Apply(ctx, tx, in)
	if err != nil {
		return empty, err
	}
	if tr.PriorVersion != c.ExpectedVersion || !token.MatchString(tr.PriorState) || !token.MatchString(tr.ResultingState) || !token.MatchString(tr.Outcome) {
		return empty, ErrConflict
	}
	approvalID := ""
	var approvalArg, approverArg any
	if c.Approval != nil {
		approvalID = c.Approval.ApprovalID
		approvalArg, approverArg = c.Approval.ApprovalID, c.Approval.ApproverRef
	}
	receipt := Receipt{ContractVersion: s.Spec.Contract, Owner: s.Spec.Owner, OperationID: c.OperationID, DecisionID: decision, SubjectKind: c.SubjectKind, SubjectID: c.SubjectID, CaseRef: c.CaseRef, Action: c.Action, IdempotencyKey: c.IdempotencyKey, IntentDigest: intent, ApprovalID: approvalID, PriorState: tr.PriorState, ResultingState: tr.ResultingState, RecordedVersion: c.ExpectedVersion + 1, Outcome: tr.Outcome, Scope: scope, RecordedAt: now.Truncate(time.Microsecond)}
	raw, err = json.Marshal(receipt)
	if err != nil {
		return empty, err
	}
	versionKey := ""
	if action.VersionOf == VersionCase {
		versionKey = c.CaseRef
	}
	_, err = tx.Exec(ctx, `INSERT INTO admin_owner_command_ledger(`+ledgerColumns+`) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27)`,
		c.IdempotencyKey, c.OperationID, decision, s.Spec.Owner, s.Spec.Contract, c.SubjectKind, c.SubjectID, c.CaseRef, c.Action, action.VersionOf, versionKey, receipt.RecordedVersion, operation, intent, approvalArg, approverArg, actor.Staff, purpose, scope.Product, scope.Market, scope.Zone, scope.Organization, scope.Tenant, s.Policy.ID(), s.Policy.Version(), s.Policy.Checksum(), raw)
	if err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) && (pg.Code == "23505" || pg.Code == "23514") {
			return empty, ErrConflict
		}
		return empty, err
	}
	if err := tx.Commit(ctx); err != nil {
		return empty, err
	}
	return receipt, nil
}

func (s *Service) readAudit(ctx context.Context, tx pgx.Tx, target string, actor adminpolicy.Actor, purpose string, scope adminpolicy.Scope, outcome string) error {
	_, err := tx.Exec(ctx, `INSERT INTO admin_owner_read_audit(owner,resource_kind,target_ref,staff_ref,purpose_code,market,zone,organization_ref,tenant_ref,policy_id,policy_version,policy_checksum,as_of,outcome)VALUES($1,'admin-command-decisions',$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,now(),$12)`, s.Spec.Owner, target, actor.Staff, purpose, scope.Market, scope.Zone, scope.Organization, scope.Tenant, s.Policy.ID(), s.Policy.Version(), s.Policy.Checksum(), outcome)
	return err
}

// Readback returns the original immutable receipt by original identity.
// Absence is never proof of rejection; current read authority is mandatory.
func (s *Service) Readback(ctx context.Context, actor adminpolicy.Actor, purpose string, scope adminpolicy.Scope, kind, id, key string) (Receipt, error) {
	var empty Receipt
	if !s.ReadEnabled {
		return empty, ErrDisabled
	}
	if !Segment(id) || !Segment(key) || !Segment(kind) {
		return empty, ErrInvalid
	}
	now := s.now()
	if err := s.ready(ctx, actor, s.Spec.ReadCapability, purpose, scope, now); err != nil {
		return empty, err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback(ctx)
	if err := s.authorize(ctx, tx, actor, purpose, kind, id, s.Spec.ReadCapability, scope, now); err != nil {
		return empty, err
	}
	var raw []byte
	err = tx.QueryRow(ctx, `SELECT receipt FROM admin_owner_command_ledger WHERE idempotency_key=$1 AND owner=$2 AND subject_kind=$3 AND subject_id=$4 AND product=$5 AND market=$6 AND zone=$7 AND organization_ref=$8 AND tenant_ref=$9`, key, s.Spec.Owner, kind, id, scope.Product, scope.Market, scope.Zone, scope.Organization, scope.Tenant).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return empty, ErrNotFound
	}
	if err != nil {
		return empty, err
	}
	out, err := DecodeReceipt(raw)
	if err != nil {
		return empty, err
	}
	if err := s.readAudit(ctx, tx, id, actor, purpose, scope, "DISCLOSED"); err != nil {
		return empty, err
	}
	return out, tx.Commit(ctx)
}

// History lists the subject's immutable decisions newest first, together with
// the owner's current administrative version and state for the subject.
func (s *Service) History(ctx context.Context, actor adminpolicy.Actor, purpose string, scope adminpolicy.Scope, kind, id string) (History, error) {
	var empty History
	if !s.ReadEnabled {
		return empty, ErrDisabled
	}
	if !Segment(id) || !Segment(kind) {
		return empty, ErrInvalid
	}
	now := s.now()
	if err := s.ready(ctx, actor, s.Spec.ReadCapability, purpose, scope, now); err != nil {
		return empty, err
	}
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return empty, err
	}
	defer tx.Rollback(ctx)
	if err := s.authorize(ctx, tx, actor, purpose, kind, id, s.Spec.ReadCapability, scope, now); err != nil {
		return empty, err
	}
	version, state, err := s.Executor.Current(ctx, tx, kind, id)
	if err != nil {
		return empty, err
	}
	rows, err := tx.Query(ctx, `SELECT receipt FROM admin_owner_command_ledger WHERE owner=$1 AND subject_kind=$2 AND subject_id=$3 AND product=$4 AND market=$5 AND zone=$6 AND organization_ref=$7 AND tenant_ref=$8 ORDER BY created_at DESC, decision_id DESC LIMIT 51`, s.Spec.Owner, kind, id, scope.Product, scope.Market, scope.Zone, scope.Organization, scope.Tenant)
	if err != nil {
		return empty, err
	}
	out := History{ContractVersion: s.Spec.Contract, Owner: s.Spec.Owner, SubjectKind: kind, SubjectID: id, CurrentVersion: version, CurrentState: state, Decisions: []Receipt{}, Scope: scope, AsOf: now}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			rows.Close()
			return empty, err
		}
		receipt, err := DecodeReceipt(raw)
		if err != nil {
			rows.Close()
			return empty, err
		}
		out.Decisions = append(out.Decisions, receipt)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return empty, err
	}
	if len(out.Decisions) > 50 {
		out.Decisions, out.Truncated = out.Decisions[:50], true
	}
	// The read-only snapshot cannot write the disclosure audit, so a separate
	// short transaction records it before anything is returned.
	if err := tx.Commit(ctx); err != nil {
		return empty, err
	}
	audit, err := s.DB.Begin(ctx)
	if err != nil {
		return empty, err
	}
	defer audit.Rollback(ctx)
	if err := s.readAudit(ctx, audit, id, actor, purpose, scope, "DISCLOSED"); err != nil {
		return empty, err
	}
	return out, audit.Commit(ctx)
}
