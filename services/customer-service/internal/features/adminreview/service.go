// Package adminreview owns Customer review intake and immutable outcomes.
// It does not change identity, verification, restrictions, consent or bookings.
package adminreview

import (
	"context"
	"errors"
	"time"

	"cosmicforge/logistics/shared/go/admincontext"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/customerreview"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrDisabled = errors.New("Customer review capability disabled")
var ErrConflict = errors.New("Customer review identity or revision conflict")
var ErrNotFound = errors.New("Customer review request not found")

type Service struct {
	DB                        *pgxpool.Pool
	Policy                    *adminpolicy.Engine
	Contexts                  *admincontext.Store
	WriteEnabled, ReadEnabled bool
	Now                       func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func (s *Service) authorize(ctx context.Context, tx pgx.Tx, actor adminpolicy.Actor, purpose, customer, capability string, scope adminpolicy.Scope, now time.Time) error {
	if s.Contexts == nil || s.Contexts.Reader == nil || s.Contexts.Authority.Owner != customerreview.Owner {
		return adminpolicy.ErrBlocked
	}
	facts, err := s.Contexts.Resolve(ctx, tx, "review-states", customer, capability, scope, now)
	if err != nil {
		return err
	}
	if facts.SourceContract != customerreview.StateContract {
		return adminpolicy.ErrDenied
	}
	// The source-owned request operation defines its own eligibility contract;
	// the independently signed allocation above still binds the actual review
	// aggregate and customer. No body tenant or read-model ETag supplies facts.
	if capability == customerreview.RequestCapability {
		facts.SourceContract = customerreview.Contract
	}
	return s.Policy.AuthorizeOwner(actor, capability, purpose, scope, facts, now)
}
func (s *Service) ready(ctx context.Context, actor adminpolicy.Actor, capability, purpose string, scope adminpolicy.Scope, now time.Time) error {
	if s.DB == nil || s.Policy == nil || s.Contexts == nil {
		return adminpolicy.ErrBlocked
	}
	if err := s.Policy.AuthorizeDelegation(actor, capability, purpose, scope, now); err != nil {
		return err
	}
	return adminpolicy.PinRevision(ctx, s.DB, "admin_owner_policy_revisions", s.Policy)
}

const receiptColumns = `id::text,request_id::text,customer_id::text,idempotency_key,intent_digest,recorded_revision,product,market,zone,organization_ref,tenant_ref,created_at`

func scanReceipt(row pgx.Row) (customerreview.Receipt, error) {
	out := customerreview.Receipt{ContractVersion: customerreview.Contract, Status: customerreview.Recorded}
	err := row.Scan(&out.ReviewRequestID, &out.RequestID, &out.CustomerID, &out.IdempotencyKey, &out.IntentDigest, &out.RecordedReviewRevision, &out.Scope.Product, &out.Scope.Market, &out.Scope.Zone, &out.Scope.Organization, &out.Scope.Tenant, &out.RecordedAt)
	out.RecordedAt = out.RecordedAt.UTC()
	return out, err
}
func (s *Service) Request(ctx context.Context, actor adminpolicy.Actor, purpose string, scope adminpolicy.Scope, input customerreview.Request) (customerreview.Receipt, error) {
	var empty customerreview.Receipt
	if !s.WriteEnabled {
		return empty, ErrDisabled
	}
	now := s.now()
	if err := input.Validate(now); err != nil {
		return empty, err
	}
	if err := s.ready(ctx, actor, customerreview.RequestCapability, purpose, scope, now); err != nil {
		return empty, err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback(ctx)
	if err := s.authorize(ctx, tx, actor, purpose, input.CustomerID, customerreview.RequestCapability, scope, now); err != nil {
		return empty, err
	}
	var revision int64
	var state string
	if err := tx.QueryRow(ctx, `SELECT version,status FROM customer_admin_review_state WHERE customer_id=$1 FOR UPDATE`, input.CustomerID).Scan(&revision, &state); err != nil {
		return empty, err
	}
	digest := input.Digest(actor.Staff, purpose, scope)
	previous, err := scanReceipt(tx.QueryRow(ctx, `SELECT `+receiptColumns+` FROM customer_admin_review_requests WHERE idempotency_key=$1`, input.IdempotencyKey))
	if err == nil {
		if previous.IntentDigest != digest || previous.Scope != scope || previous.CustomerID != input.CustomerID {
			return empty, ErrConflict
		}
		if err := tx.Commit(ctx); err != nil {
			return empty, err
		}
		return previous, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return empty, err
	}
	if revision != input.ExpectedReviewRevision || state != "IDLE" {
		return empty, ErrConflict
	}
	id := uuid.NewString()
	receipt, err := scanReceipt(tx.QueryRow(ctx, `INSERT INTO customer_admin_review_requests(id,customer_id,request_id,idempotency_key,intent_digest,requested_revision,recorded_revision,staff_ref,purpose_code,reason_code,product,market,zone,organization_ref,tenant_ref,policy_id,policy_version,policy_checksum) VALUES($1,$2,$3,$4,$5,$6::bigint,$6::bigint+1,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17) RETURNING `+receiptColumns, id, input.CustomerID, input.RequestID, input.IdempotencyKey, digest, revision, actor.Staff, purpose, input.ReasonCode, scope.Product, scope.Market, scope.Zone, scope.Organization, scope.Tenant, s.Policy.ID(), s.Policy.Version(), s.Policy.Checksum()))
	if err != nil {
		var pgerr *pgconn.PgError
		if errors.As(err, &pgerr) && pgerr.Code == "23505" {
			return empty, ErrConflict
		}
		return empty, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO customer_admin_review_work_items(request_id,customer_id) VALUES($1,$2)`, id, input.CustomerID); err != nil {
		return empty, err
	}
	if _, err := tx.Exec(ctx, `UPDATE customer_admin_review_state SET version=version+1,status='PENDING_REVIEW',updated_at=now() WHERE customer_id=$1`, input.CustomerID); err != nil {
		return empty, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO customer_admin_review_audit(request_id,staff_ref,purpose_code,outcome) VALUES($1,$2,$3,'REVIEW_REQUEST_RECORDED')`, id, actor.Staff, purpose); err != nil {
		return empty, err
	}
	if err := tx.Commit(ctx); err != nil {
		return empty, err
	}
	return receipt, nil
}
func (s *Service) Receipt(ctx context.Context, actor adminpolicy.Actor, purpose, customer, key string, scope adminpolicy.Scope) (customerreview.Receipt, error) {
	var empty customerreview.Receipt
	if !s.ReadEnabled {
		return empty, ErrDisabled
	}
	if !customerreview.CanonicalID(customer) || !customerreview.Segment(key) {
		return empty, customerreview.ErrInvalid
	}
	now := s.now()
	if err := s.ready(ctx, actor, customerreview.ReadCapability, purpose, scope, now); err != nil {
		return empty, err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback(ctx)
	if err := s.authorize(ctx, tx, actor, purpose, customer, customerreview.ReadCapability, scope, now); err != nil {
		return empty, err
	}
	out, err := scanReceipt(tx.QueryRow(ctx, `SELECT `+receiptColumns+` FROM customer_admin_review_requests WHERE customer_id=$1 AND idempotency_key=$2 AND product=$3 AND market=$4 AND zone=$5 AND organization_ref=$6 AND tenant_ref=$7`, customer, key, scope.Product, scope.Market, scope.Zone, scope.Organization, scope.Tenant))
	if errors.Is(err, pgx.ErrNoRows) {
		return empty, ErrNotFound
	}
	if err != nil {
		return empty, err
	}
	// Persist disclosure evidence before returning immutable historical proof.
	_, err = tx.Exec(ctx, `INSERT INTO admin_owner_read_audit(owner,resource_kind,target_ref,staff_ref,purpose_code,market,zone,organization_ref,tenant_ref,policy_id,policy_version,policy_checksum,as_of,outcome) VALUES('customer-service','review-receipts',$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,now(),'DISCLOSED')`, out.ReviewRequestID, actor.Staff, purpose, scope.Market, scope.Zone, scope.Organization, scope.Tenant, s.Policy.ID(), s.Policy.Version(), s.Policy.Checksum())
	if err != nil {
		return empty, err
	}
	if err := tx.Commit(ctx); err != nil {
		return empty, err
	}
	return out, nil
}
