// Package customerreviewops orchestrates the fixed Customer review-intake
// contract. It stores no Customer business state and never retries a POST.
package customerreviewops

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"cosmicforge/logistics/services/admin-backoffice-service/internal/config"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/customerreview"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrDisabled = errors.New("Customer review orchestration disabled")
var ErrConflict = errors.New("Customer review orchestration identity conflict")
var ErrNotFound = errors.New("Customer review operation not found")
var ErrProof = errors.New("Customer review original outcome proof mismatch")

type Owner interface {
	Request(context.Context, adminread.Query, customerreview.Request) (customerreview.Receipt, error)
	Readback(context.Context, adminread.Query, string, string) (customerreview.Receipt, error)
}
type Service struct {
	DB                        *pgxpool.Pool
	Owner                     Owner
	Policy                    *adminpolicy.Engine
	WriteEnabled, ReadEnabled bool
}
type Operation struct {
	OperationID string                  `json:"operation_id"`
	CustomerID  string                  `json:"customer_id"`
	Scope       adminpolicy.Scope       `json:"scope"`
	Status      string                  `json:"status"`
	CreatedAt   time.Time               `json:"created_at"`
	AsOf        time.Time               `json:"as_of"`
	Receipt     *customerreview.Receipt `json:"receipt,omitempty"`
}
type record struct {
	Operation
	key, digest, staff, purpose, claim string
	expected                           int64
}

func scope(q adminread.Query) adminpolicy.Scope {
	return adminpolicy.Scope{Product: config.Product, Market: q.Scope.Market, Zone: q.Scope.Zone, Organization: q.Scope.Organization, Tenant: q.Scope.Tenant}
}
func (s *Service) authorize(ctx context.Context, actor adminpolicy.Actor, q adminread.Query, capability string) error {
	if s.DB == nil || s.Owner == nil || s.Policy == nil {
		return adminpolicy.ErrBlocked
	}
	now := time.Now()
	if actor.Staff != q.Staff || q.Cursor != "" || q.PolicyID != s.Policy.ID() || q.PolicyVersion != s.Policy.Version() || q.PolicyChecksum != s.Policy.Checksum() {
		return adminpolicy.ErrDenied
	}
	if _, err := adminread.Parse(q.Values(), now); err != nil {
		return err
	}
	rule, err := s.Policy.Rule(capability, now)
	expected := customerreview.Contract
	if capability == customerreview.ReadCapability {
		expected = customerreview.StateContract
	}
	if err != nil || rule.Owner != customerreview.Owner || rule.SourceContract != expected {
		return adminpolicy.ErrBlocked
	}
	if !config.Contains(actor.Roles, q.Role) || !config.Contains(rule.Roles, q.Role) {
		return adminpolicy.ErrDenied
	}
	if err := s.Policy.AuthorizeDelegation(actor, capability, q.Purpose, scope(q), now); err != nil {
		return err
	}
	return adminpolicy.PinRevision(ctx, s.DB, "admin_policy_revisions", s.Policy)
}

// Submit claims durable ownership before making exactly one owner POST. Every
// error after that boundary leaves an uncertain operation for GET-only recovery.
func (s *Service) Submit(ctx context.Context, actor adminpolicy.Actor, q adminread.Query, input customerreview.Request) (Operation, error) {
	if !s.WriteEnabled || !s.ReadEnabled {
		return Operation{}, ErrDisabled
	}
	if err := input.Validate(time.Now()); err != nil {
		return Operation{}, err
	}
	if err := s.authorize(ctx, actor, q, customerreview.RequestCapability); err != nil {
		return Operation{}, err
	}
	r, won, err := s.begin(ctx, actor, q, input)
	if err != nil {
		return Operation{}, err
	}
	if !won {
		return s.Reconcile(ctx, actor, q, r.OperationID)
	}
	receipt, err := s.Owner.Request(ctx, q, input)
	if err != nil {
		// A dropped response, refusal, absent receipt or audit failure cannot prove
		// that the original action did not commit. Never offer a fresh-key retry.
		s.uncertain(ctx, r, q)
		r.Status = "UNKNOWN_OUTCOME"
		return r.Operation, nil
	}
	if err := match(r, receipt); err != nil {
		s.uncertain(ctx, r, q)
		return Operation{}, err
	}
	return s.confirm(ctx, r, q, receipt)
}

// Reconcile never calls Request, even for a durable EXECUTING record left by a
// crashed process. Current delegated and source-owner read access is mandatory
// on every call, including already confirmed historical receipts.
func (s *Service) Reconcile(ctx context.Context, actor adminpolicy.Actor, q adminread.Query, id string) (Operation, error) {
	if !s.ReadEnabled {
		return Operation{}, ErrDisabled
	}
	if !customerreview.CanonicalID(id) {
		return Operation{}, ErrNotFound
	}
	if err := s.authorize(ctx, actor, q, customerreview.ReadCapability); err != nil {
		return Operation{}, err
	}
	r, err := load(s.DB.QueryRow(ctx, `SELECT `+columns+` FROM admin_customer_review_operations WHERE id=$1 AND staff_ref=$2 AND purpose_code=$3 AND product=$4 AND market=$5 AND zone=$6 AND organization_ref=$7 AND tenant_ref=$8`, id, actor.Staff, q.Purpose, config.Product, q.Scope.Market, q.Scope.Zone, q.Scope.Organization, q.Scope.Tenant))
	if errors.Is(err, pgx.ErrNoRows) {
		return Operation{}, ErrNotFound
	}
	if err != nil {
		return Operation{}, err
	}
	receipt, err := s.Owner.Readback(ctx, q, r.CustomerID, r.key)
	if err != nil {
		return Operation{}, err
	}
	if err := match(r, receipt); err != nil {
		return Operation{}, err
	}
	return s.confirm(ctx, r, q, receipt)
}
func match(r record, receipt customerreview.Receipt) error {
	if receipt.ContractVersion != customerreview.Contract || receipt.Status != customerreview.Recorded || receipt.RequestID != r.OperationID || receipt.CustomerID != r.CustomerID || receipt.IdempotencyKey != r.key || receipt.IntentDigest != r.digest || receipt.RecordedReviewRevision != r.expected+1 || receipt.Scope != r.Scope || !customerreview.CanonicalID(receipt.ReviewRequestID) || receipt.RecordedAt.IsZero() {
		return ErrProof
	}
	return nil
}

const columns = `id::text,customer_id::text,product,market,zone,organization_ref,tenant_ref,status,created_at,updated_at,idempotency_key,intent_digest,staff_ref,purpose_code,claim_token::text,expected_review_revision`

func load(row pgx.Row) (record, error) {
	var r record
	err := row.Scan(&r.OperationID, &r.CustomerID, &r.Scope.Product, &r.Scope.Market, &r.Scope.Zone, &r.Scope.Organization, &r.Scope.Tenant, &r.Status, &r.CreatedAt, &r.AsOf, &r.key, &r.digest, &r.staff, &r.purpose, &r.claim, &r.expected)
	r.CreatedAt = r.CreatedAt.UTC()
	r.AsOf = r.AsOf.UTC()
	return r, err
}
func (s *Service) begin(ctx context.Context, actor adminpolicy.Actor, q adminread.Query, input customerreview.Request) (record, bool, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return record{}, false, err
	}
	defer tx.Rollback(ctx)
	claim := uuid.NewString()
	digest := input.Digest(actor.Staff, q.Purpose, scope(q))
	result, err := tx.Exec(ctx, `INSERT INTO admin_customer_review_operations(id,principal,staff_ref,purpose_code,product,market,zone,organization_ref,tenant_ref,customer_id,idempotency_key,intent_digest,expected_review_revision,status,claim_token) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,'EXECUTING',$14) ON CONFLICT DO NOTHING`, input.RequestID, config.HQPrincipal, actor.Staff, q.Purpose, config.Product, q.Scope.Market, q.Scope.Zone, q.Scope.Organization, q.Scope.Tenant, input.CustomerID, input.IdempotencyKey, digest, input.ExpectedReviewRevision, claim)
	if err != nil {
		return record{}, false, err
	}
	r, err := load(tx.QueryRow(ctx, `SELECT `+columns+` FROM admin_customer_review_operations WHERE idempotency_key=$1`, input.IdempotencyKey))
	if errors.Is(err, pgx.ErrNoRows) {
		return record{}, false, ErrConflict
	}
	if err != nil {
		return record{}, false, err
	}
	if r.OperationID != input.RequestID || r.digest != digest || r.staff != actor.Staff || r.purpose != q.Purpose || r.Scope != scope(q) || r.CustomerID != input.CustomerID || r.expected != input.ExpectedReviewRevision {
		return record{}, false, ErrConflict
	}
	won := result.RowsAffected() == 1
	if won {
		if err := s.audit(ctx, tx, r, q, "EXECUTING"); err != nil {
			return record{}, false, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return record{}, false, err
	}
	return r, won, nil
}
func (s *Service) audit(ctx context.Context, tx pgx.Tx, r record, q adminread.Query, outcome string) error {
	_, err := tx.Exec(ctx, `INSERT INTO admin_customer_review_operation_audit(operation_id,staff_ref,purpose_code,product,market,zone,organization_ref,tenant_ref,policy_id,policy_version,policy_checksum,outcome) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, r.OperationID, r.staff, r.purpose, r.Scope.Product, r.Scope.Market, r.Scope.Zone, r.Scope.Organization, r.Scope.Tenant, q.PolicyID, q.PolicyVersion, q.PolicyChecksum, outcome)
	return err
}
func (s *Service) uncertain(ctx context.Context, r record, q adminread.Query) {
	// The persisted EXECUTING state is itself a no-retry fence if this best-effort
	// status/audit transaction cannot commit after a caller disconnects.
	detached, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	tx, err := s.DB.Begin(detached)
	if err != nil {
		return
	}
	defer tx.Rollback(detached)
	result, err := tx.Exec(detached, `UPDATE admin_customer_review_operations SET status='UNKNOWN_OUTCOME',updated_at=now() WHERE id=$1 AND claim_token=$2 AND status='EXECUTING'`, r.OperationID, r.claim)
	if err != nil || result.RowsAffected() != 1 {
		return
	}
	if s.audit(detached, tx, r, q, "UNKNOWN_OUTCOME") != nil {
		return
	}
	_ = tx.Commit(detached)
}
func (s *Service) confirm(ctx context.Context, r record, q adminread.Query, receipt customerreview.Receipt) (Operation, error) {
	raw, err := json.Marshal(receipt)
	if err != nil {
		return Operation{}, err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Operation{}, err
	}
	defer tx.Rollback(ctx)
	// Serialize concurrent readbacks and reject any later differing proof. No
	// owner response can change the identity or original confirmed receipt.
	var previous []byte
	var status string
	if err := tx.QueryRow(ctx, `SELECT status,receipt FROM admin_customer_review_operations WHERE id=$1 AND claim_token=$2 FOR UPDATE`, r.OperationID, r.claim).Scan(&status, &previous); err != nil {
		return Operation{}, err
	}
	if status == "CONFIRMED" {
		var stored customerreview.Receipt
		if json.Unmarshal(previous, &stored) != nil || stored != receipt {
			return Operation{}, ErrProof
		}
	}
	if err := tx.QueryRow(ctx, `UPDATE admin_customer_review_operations SET status='CONFIRMED',receipt=$3,updated_at=now() WHERE id=$1 AND claim_token=$2 RETURNING updated_at`, r.OperationID, r.claim, raw).Scan(&r.AsOf); err != nil {
		return Operation{}, err
	}
	if err := s.audit(ctx, tx, r, q, "CONFIRMED"); err != nil {
		return Operation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Operation{}, err
	}
	r.Status = "CONFIRMED"
	r.AsOf = r.AsOf.UTC()
	r.Receipt = &receipt
	return r.Operation, nil
}
