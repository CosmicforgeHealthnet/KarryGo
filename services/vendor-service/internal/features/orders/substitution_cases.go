package orders

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Store) CreateSubstitutionCase(ctx context.Context, input SubstitutionCase) (SubstitutionCase, error) {
	if s == nil || s.db == nil {
		return SubstitutionCase{}, ErrDependencyMissing
	}
	if input.ParentOrderID == "" || input.VendorOrderID == "" || input.OrderLineID == "" || input.CustomerID == "" || input.UnavailableQuantity <= 0 || !input.FallbackPolicy.Valid() || input.IdempotencyKey == "" {
		return SubstitutionCase{}, ErrOrderConflict
	}
	if len(input.OriginalLineSnapshot) == 0 {
		input.OriginalLineSnapshot = json.RawMessage(`{}`)
	}
	if len(input.PolicySnapshot) == 0 {
		input.PolicySnapshot = json.RawMessage(`{}`)
	}
	if len(input.QualitySnapshot) == 0 {
		input.QualitySnapshot = json.RawMessage(`[]`)
	}
	input.ID = "scase_" + uuid.NewString()
	now := time.Now().UTC()
	input.CreatedAt, input.UpdatedAt, input.Version = now, now, 1
	tag, err := s.db.Exec(ctx, `INSERT INTO commerce_substitution_cases(id,parent_order_id,vendor_order_id,order_line_id,customer_id,business_id,branch_id,unavailable_quantity,original_line_snapshot,policy_snapshot,quality_snapshot,deadline_at,fallback_policy,state,reason,version,idempotency_key,correlation_ref,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,'OPEN',$14,1,$15,$16,$17,$17) ON CONFLICT(idempotency_key) DO NOTHING`, input.ID, input.ParentOrderID, input.VendorOrderID, input.OrderLineID, input.CustomerID, input.BusinessID, input.BranchID, input.UnavailableQuantity, input.OriginalLineSnapshot, input.PolicySnapshot, input.QualitySnapshot, input.DeadlineAt, input.FallbackPolicy, input.Reason, input.IdempotencyKey, input.CorrelationRef, now)
	if err != nil {
		return SubstitutionCase{}, err
	}
	if tag.RowsAffected() == 0 {
		var existingID string
		if err = s.db.QueryRow(ctx, `SELECT id FROM commerce_substitution_cases WHERE idempotency_key=$1`, input.IdempotencyKey).Scan(&existingID); err != nil {
			return SubstitutionCase{}, err
		}
		return s.GetSubstitutionCaseForCustomer(ctx, input.CustomerID, existingID)
	}
	return input, nil
}

func (s *Store) RespondSubstitutionCase(ctx context.Context, customerID, caseID string, expectedVersion int64, accept bool, responseRef string, now time.Time) (SubstitutionCase, error) {
	if s == nil || s.db == nil {
		return SubstitutionCase{}, ErrDependencyMissing
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return SubstitutionCase{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var state string
	var currentVersion int64
	if err = tx.QueryRow(ctx, `SELECT state,version FROM commerce_substitution_cases WHERE id=$1 AND customer_id=$2 FOR UPDATE`, caseID, customerID).Scan(&state, &currentVersion); errors.Is(err, pgx.ErrNoRows) {
		return SubstitutionCase{}, ErrOrderNotFound
	} else if err != nil {
		return SubstitutionCase{}, err
	}
	if currentVersion != expectedVersion || state != string(CaseAwaitingCustomer) && state != string(CaseOpen) {
		return SubstitutionCase{}, ErrVersionConflict
	}
	next := CaseRejected
	if accept {
		next = CaseApproved
	}
	if _, err = tx.Exec(ctx, `UPDATE commerce_substitution_cases SET state=$1,version=version+1,updated_at=$2,resolved_at=$2,reason=CASE WHEN $3<>'' THEN $3 ELSE reason END WHERE id=$4 AND customer_id=$5 AND version=$6`, next, now.UTC(), responseRef, caseID, customerID, expectedVersion); err != nil {
		return SubstitutionCase{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return SubstitutionCase{}, err
	}
	return SubstitutionCase{ID: caseID, CustomerID: customerID, State: next, Version: expectedVersion + 1, UpdatedAt: now.UTC(), ResolvedAt: &now}, nil
}

func (s *Store) GetSubstitutionCaseForCustomer(ctx context.Context, customerID, caseID string) (SubstitutionCase, error) {
	if s == nil || s.db == nil {
		return SubstitutionCase{}, ErrDependencyMissing
	}
	var c SubstitutionCase
	var state, fallback string
	err := s.db.QueryRow(ctx, `SELECT id,parent_order_id,vendor_order_id,order_line_id,customer_id,business_id,branch_id,unavailable_quantity,original_line_snapshot,policy_snapshot,quality_snapshot,deadline_at,fallback_policy,state,reason,version,idempotency_key,correlation_ref,created_at,updated_at,resolved_at FROM commerce_substitution_cases WHERE id=$1 AND customer_id=$2`, caseID, customerID).Scan(&c.ID, &c.ParentOrderID, &c.VendorOrderID, &c.OrderLineID, &c.CustomerID, &c.BusinessID, &c.BranchID, &c.UnavailableQuantity, &c.OriginalLineSnapshot, &c.PolicySnapshot, &c.QualitySnapshot, &c.DeadlineAt, &fallback, &state, &c.Reason, &c.Version, &c.IdempotencyKey, &c.CorrelationRef, &c.CreatedAt, &c.UpdatedAt, &c.ResolvedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return SubstitutionCase{}, ErrOrderNotFound
	}
	if err != nil {
		return SubstitutionCase{}, err
	}
	c.FallbackPolicy = SubstitutionPolicy(fallback)
	c.State = SubstitutionCaseState(state)
	return c, nil
}

func (s *Store) ListSubstitutionCasesForCustomer(ctx context.Context, customerID string, limit int) ([]SubstitutionCase, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `SELECT id,parent_order_id,vendor_order_id,order_line_id,customer_id,business_id,branch_id,unavailable_quantity,original_line_snapshot,policy_snapshot,quality_snapshot,deadline_at,fallback_policy,state,reason,version,idempotency_key,correlation_ref,created_at,updated_at,resolved_at FROM commerce_substitution_cases WHERE customer_id=$1 AND state IN ('OPEN','AWAITING_CUSTOMER') ORDER BY created_at,id LIMIT $2`, customerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]SubstitutionCase, 0)
	for rows.Next() {
		var c SubstitutionCase
		var fallback, state string
		if err = rows.Scan(&c.ID, &c.ParentOrderID, &c.VendorOrderID, &c.OrderLineID, &c.CustomerID, &c.BusinessID, &c.BranchID, &c.UnavailableQuantity, &c.OriginalLineSnapshot, &c.PolicySnapshot, &c.QualitySnapshot, &c.DeadlineAt, &fallback, &state, &c.Reason, &c.Version, &c.IdempotencyKey, &c.CorrelationRef, &c.CreatedAt, &c.UpdatedAt, &c.ResolvedAt); err != nil {
			return nil, err
		}
		c.FallbackPolicy = SubstitutionPolicy(fallback)
		c.State = SubstitutionCaseState(state)
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) CancelSubstitutionCases(ctx context.Context, vendorOrderID string, now time.Time) error {
	if s == nil || s.db == nil {
		return ErrDependencyMissing
	}
	_, err := s.db.Exec(ctx, `UPDATE commerce_substitution_cases SET state='CANCELLED',version=version+1,updated_at=$1,resolved_at=$1 WHERE vendor_order_id=$2 AND state IN ('OPEN','AWAITING_CUSTOMER','AUTO_REFUND_PENDING')`, now.UTC(), vendorOrderID)
	return err
}

func normalizeCaseCustomer(value string) string { return strings.TrimSpace(value) }
