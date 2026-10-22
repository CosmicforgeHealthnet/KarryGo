package adminreview

import (
	"context"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/customerreview"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func (s *Service) TransitionWorkItem(ctx context.Context, actor adminpolicy.Actor, purpose string, scope adminpolicy.Scope, c customerreview.WorkItemCommand) (customerreview.WorkItemReceipt, error) {
	var empty customerreview.WorkItemReceipt
	if !s.WorkItemsEnabled || !s.WriteEnabled {
		return empty, ErrDisabled
	}
	now := s.now()
	if err := c.Validate(now); err != nil {
		return empty, err
	}
	if err := s.ready(ctx, actor, customerreview.WorkItemCapability, purpose, scope, now); err != nil {
		return empty, err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback(ctx)
	// Same aggregate-first locking order as intake: no concurrent queue transition
	// can reopen a different customer's or a new review's aggregate.
	var aggregateVersion int64
	var aggregateState string
	if err := tx.QueryRow(ctx, `SELECT version,status FROM customer_admin_review_state WHERE customer_id=$1 FOR UPDATE`, c.CustomerID).Scan(&aggregateVersion, &aggregateState); err != nil {
		return empty, err
	}
	if err := s.authorize(ctx, tx, actor, purpose, c.CustomerID, customerreview.WorkItemCapability, scope, now); err != nil {
		return empty, err
	}
	digest := c.Digest(actor.Staff, purpose, scope)
	var raw []byte
	err = tx.QueryRow(ctx, `SELECT receipt FROM customer_admin_review_transitions WHERE idempotency_key=$1`, c.IdempotencyKey).Scan(&raw)
	if err == nil {
		receipt, e := customerreview.DecodeWorkItemReceipt(raw)
		if e != nil || receipt.IntentDigest != digest {
			return empty, ErrConflict
		}
		if err := tx.Commit(ctx); err != nil {
			return empty, err
		}
		return receipt, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return empty, err
	}
	var state string
	var version int64
	var product, market, zone, organization, tenant string
	err = tx.QueryRow(ctx, `SELECT w.status,w.version,r.product,r.market,r.zone,r.organization_ref,r.tenant_ref FROM customer_admin_review_work_items w JOIN customer_admin_review_requests r ON r.id=w.request_id AND r.customer_id=w.customer_id WHERE w.request_id=$1 AND w.customer_id=$2 FOR UPDATE OF w`, c.ReviewRequestID, c.CustomerID).Scan(&state, &version, &product, &market, &zone, &organization, &tenant)
	if err != nil {
		return empty, err
	}
	original := adminpolicy.Scope{Product: product, Market: market, Zone: zone, Organization: organization, Tenant: tenant}
	if original != scope {
		return empty, adminpolicy.ErrDenied
	}
	if aggregateState != "PENDING_REVIEW" || version != c.ExpectedVersion {
		return empty, ErrConflict
	}
	outcome := "REVIEW_ACKNOWLEDGED"
	next := "ACKNOWLEDGED"
	if c.Action == "ACKNOWLEDGE" {
		if state != "REQUESTED" {
			return empty, ErrConflict
		}
	} else {
		if state != "ACKNOWLEDGED" {
			return empty, ErrConflict
		}
		next = "CLOSED"
		outcome = "REVIEW_CLOSED_WITHOUT_ACCOUNT_CHANGE"
	}
	receipt := customerreview.WorkItemReceipt{ContractVersion: customerreview.WorkItemContract, OperationID: c.OperationID, ReviewRequestID: c.ReviewRequestID, CustomerID: c.CustomerID, IdempotencyKey: c.IdempotencyKey, IntentDigest: digest, State: next, RecordedVersion: version + 1, Outcome: outcome, Scope: scope, RecordedAt: now}
	raw, err = json.Marshal(receipt)
	if err != nil {
		return empty, err
	}
	// The immutable transition is the owner audit. Insert/audit failure rolls back
	// all queue state, and never changes the original immutable intake receipt.
	_, err = tx.Exec(ctx, `INSERT INTO customer_admin_review_transitions(idempotency_key,operation_id,request_id,customer_id,intent_digest,staff_ref,purpose_code,product,market,zone,organization_ref,tenant_ref,policy_id,policy_version,policy_checksum,receipt) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`, c.IdempotencyKey, c.OperationID, c.ReviewRequestID, c.CustomerID, digest, actor.Staff, purpose, scope.Product, scope.Market, scope.Zone, scope.Organization, scope.Tenant, s.Policy.ID(), s.Policy.Version(), s.Policy.Checksum(), raw)
	if err != nil {
		var e *pgconn.PgError
		if errors.As(err, &e) && e.Code == "23505" {
			return empty, ErrConflict
		}
		return empty, err
	}
	if _, err = tx.Exec(ctx, `UPDATE customer_admin_review_work_items SET version=version+1,status=$3,updated_at=$4 WHERE request_id=$1 AND customer_id=$2`, c.ReviewRequestID, c.CustomerID, next, now); err != nil {
		return empty, err
	}
	if next == "CLOSED" {
		if _, err = tx.Exec(ctx, `UPDATE customer_admin_review_state SET version=version+1,status='IDLE',updated_at=$2 WHERE customer_id=$1`, c.CustomerID, now); err != nil {
			return empty, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return empty, err
	}
	return receipt, nil
}
func (s *Service) WorkItemReadback(ctx context.Context, actor adminpolicy.Actor, purpose, customer, key string, scope adminpolicy.Scope) (customerreview.WorkItemReceipt, error) {
	var empty customerreview.WorkItemReceipt
	if !s.ReadEnabled {
		return empty, ErrDisabled
	}
	if !customerreview.CanonicalID(customer) || !customerreview.Segment(key) {
		return empty, customerreview.ErrInvalid
	}
	now := s.now()
	if err := s.ready(ctx, actor, customerreview.WorkItemReadCapability, purpose, scope, now); err != nil {
		return empty, err
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback(ctx)
	if err := s.authorize(ctx, tx, actor, purpose, customer, customerreview.WorkItemReadCapability, scope, now); err != nil {
		return empty, err
	}
	var raw []byte
	err = tx.QueryRow(ctx, `SELECT receipt FROM customer_admin_review_transitions WHERE idempotency_key=$1 AND customer_id=$2 AND product=$3 AND market=$4 AND zone=$5 AND organization_ref=$6 AND tenant_ref=$7`, key, customer, scope.Product, scope.Market, scope.Zone, scope.Organization, scope.Tenant).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return empty, ErrNotFound
	}
	if err != nil {
		return empty, err
	}
	out, err := customerreview.DecodeWorkItemReceipt(raw)
	if err != nil {
		return empty, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO admin_owner_read_audit(owner,resource_kind,target_ref,staff_ref,purpose_code,market,zone,organization_ref,tenant_ref,policy_id,policy_version,policy_checksum,as_of,outcome)VALUES('customer-service','review-work-item-receipts',$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,now(),'DISCLOSED')`, out.ReviewRequestID, actor.Staff, purpose, scope.Market, scope.Zone, scope.Organization, scope.Tenant, s.Policy.ID(), s.Policy.Version(), s.Policy.Checksum()); err != nil {
		return empty, err
	}
	if err := tx.Commit(ctx); err != nil {
		return empty, err
	}
	return out, nil
}
