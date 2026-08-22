package cost

// Cost policy management.
//
// A policy is the vendor's decision about how to value one material. Changing
// it changes every future calculation, so it is a high-risk, audited,
// version-checked mutation guarded by vendor.cost.policy.manage - and by
// branch scope, so a branch manager cannot rewrite how the whole business
// values its ingredients.
//
// No mutation here has any financial effect. Setting a manual cost records an
// operational assumption; it moves no money, settles nothing and writes to no
// ledger.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/features/production"
)

var (
	// ErrForbidden is authorization failure: the actor lacks the permission,
	// or holds it but not for the branch it is being used against.
	ErrForbidden = errors.New("cost: forbidden")
	// ErrVersionConflict is an optimistic-concurrency failure.
	ErrVersionConflict = errors.New("cost: version conflict")
	// ErrNotFound is a missing policy, or one belonging to another tenant -
	// deliberately the same error, so a probe cannot distinguish them.
	ErrNotFound = errors.New("cost: not found")
	// ErrInvalid is malformed input.
	ErrInvalid = errors.New("cost: invalid input")
)

// Service owns cost policy mutation and authorization.
type Service struct {
	db         *pgxpool.Pool
	production *production.Service
	engine     *Engine
	resolver   *Resolver
}

// NewService builds the Section 16D service.
func NewService(
	db *pgxpool.Pool,
	productionService *production.Service,
	engine *Engine,
	resolver *Resolver,
) *Service {
	return &Service{
		db:         db,
		production: productionService,
		engine:     engine,
		resolver:   resolver,
	}
}

// Engine exposes the calculation engine to the HTTP layer.
func (s *Service) Engine() *Engine { return s.engine }

// Authorize delegates to the Section 5 evaluator, exactly as Sections 16A-16C
// do, so cost permissions are evaluated by the same IAM that governs
// everything else rather than by a private rule this section invented.
func (s *Service) Authorize(
	ctx context.Context,
	actor production.Actor,
	permission string,
) (production.Scope, error) {
	if s == nil || s.production == nil {
		return production.Scope{}, ErrForbidden
	}
	return s.production.Authorize(ctx, actor, permission)
}

// AuthorizeBranch authorizes a permission and confirms the scope reaches the
// branch being acted on.
//
// This is the branch-escape guard. A branch-scoped actor holding
// vendor.cost.read for branch A is refused branch B's cost, and the refusal
// happens before any data is read rather than after.
func (s *Service) AuthorizeBranch(
	ctx context.Context,
	actor production.Actor,
	permission, branchID string,
) (production.Scope, error) {
	scope, err := s.Authorize(ctx, actor, permission)
	if err != nil {
		return production.Scope{}, err
	}
	if scope.Empty() {
		return production.Scope{}, ErrForbidden
	}
	if branchID != "" && !scope.Covers(branchID) {
		return production.Scope{}, ErrForbidden
	}
	return scope, nil
}

// PolicyInput is a create-or-update request.
type PolicyInput struct {
	BranchID    string `json:"branch_id"`
	SubjectType string `json:"subject_type"`
	SubjectRef  string `json:"subject_ref"`

	Strategy string `json:"strategy"`
	Currency string `json:"currency"`

	WeightedAverageReceiptLimit int `json:"weighted_average_receipt_limit"`

	ManualUnitCostMinor *int64 `json:"manual_unit_cost_minor"`
	ManualUnitCode      string `json:"manual_unit_code"`
	ManualCostReason    string `json:"manual_cost_reason"`

	Reason string `json:"reason"`
}

func (in PolicyInput) validate() error {
	switch in.SubjectType {
	case SubjectIngredient, SubjectPackaging:
	default:
		return fmt.Errorf("%w: subject_type", ErrInvalid)
	}
	if in.SubjectRef == "" {
		return fmt.Errorf("%w: subject_ref", ErrInvalid)
	}
	switch in.Strategy {
	case StrategyLatest, StrategyWeightedAverage, StrategyManual:
	default:
		return fmt.Errorf("%w: strategy", ErrInvalid)
	}
	if len(in.Currency) != 3 {
		return fmt.Errorf("%w: currency", ErrInvalid)
	}
	// A manual strategy without an amount would resolve to nothing and be
	// reported incomplete forever; rejecting it at the boundary is clearer.
	if in.Strategy == StrategyManual {
		if in.ManualUnitCostMinor == nil || *in.ManualUnitCostMinor < 0 {
			return fmt.Errorf("%w: manual_unit_cost_minor", ErrInvalid)
		}
		if in.ManualUnitCode == "" {
			return fmt.Errorf("%w: manual_unit_code", ErrInvalid)
		}
	}
	if in.WeightedAverageReceiptLimit < 0 || in.WeightedAverageReceiptLimit > 200 {
		return fmt.Errorf("%w: weighted_average_receipt_limit", ErrInvalid)
	}
	return nil
}

// CreatePolicy records a new cost policy.
func (s *Service) CreatePolicy(
	ctx context.Context,
	actor production.Actor,
	in PolicyInput,
) (Policy, error) {
	if err := in.validate(); err != nil {
		return Policy{}, err
	}
	if _, err := s.AuthorizeBranch(
		ctx, actor, PermCostPolicyManage, in.BranchID); err != nil {
		return Policy{}, err
	}

	limit := in.WeightedAverageReceiptLimit
	if limit == 0 {
		limit = 10
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Policy{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var branch *string
	if in.BranchID != "" {
		branch = &in.BranchID
	}

	policy := Policy{
		ID:                          newCostID("cp_"),
		BranchID:                    in.BranchID,
		SubjectType:                 in.SubjectType,
		SubjectRef:                  in.SubjectRef,
		Strategy:                    in.Strategy,
		Currency:                    in.Currency,
		WeightedAverageReceiptLimit: limit,
		ManualUnitCostMinor:         in.ManualUnitCostMinor,
		ManualUnitCode:              in.ManualUnitCode,
		ManualCostReason:            in.ManualCostReason,
		Status:                      StatusActive,
		Version:                     1,
	}

	if _, err = tx.Exec(ctx, `
		INSERT INTO vendor_cost_policies(
			id, business_id, branch_id, subject_type, subject_ref,
			strategy, currency, weighted_average_receipt_limit,
			manual_unit_cost_minor, manual_unit_code, manual_cost_reason,
			status, version, created_by_staff_id, updated_by_staff_id
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'ACTIVE',1,$12,$12)`,
		policy.ID, actor.VendorID, branch, policy.SubjectType, policy.SubjectRef,
		policy.Strategy, policy.Currency, policy.WeightedAverageReceiptLimit,
		policy.ManualUnitCostMinor, nullIfEmpty(policy.ManualUnitCode),
		nullIfEmpty(policy.ManualCostReason), actor.StaffID,
	); err != nil {
		return Policy{}, err
	}

	// Audit and outbox in the same transaction as the mutation, so a policy
	// change can never exist without the record of who made it.
	if err = s.production.RecordMutation(
		ctx, tx, actor,
		"cost.policy.created", EventCostPolicyChanged,
		"vendor_cost_policy", policy.ID, in.BranchID, 1,
		nil, policy, policy, in.Reason,
	); err != nil {
		return Policy{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return Policy{}, err
	}
	return policy, nil
}

// UpdatePolicy changes a policy under optimistic concurrency.
//
// expectedVersion comes from If-Match. The UPDATE carries it in the WHERE
// clause, so two concurrent edits cannot both succeed: the loser matches no
// row and is told the version conflicted rather than silently overwriting.
func (s *Service) UpdatePolicy(
	ctx context.Context,
	actor production.Actor,
	policyID string,
	expectedVersion int64,
	in PolicyInput,
) (Policy, error) {
	if err := in.validate(); err != nil {
		return Policy{}, err
	}

	existing, err := s.GetPolicy(ctx, actor, policyID)
	if err != nil {
		return Policy{}, err
	}
	if _, err = s.AuthorizeBranch(
		ctx, actor, PermCostPolicyManage, existing.BranchID); err != nil {
		return Policy{}, err
	}
	// The target branch must also be reachable, so an update cannot move a
	// policy into a branch the actor does not hold.
	if _, err = s.AuthorizeBranch(
		ctx, actor, PermCostPolicyManage, in.BranchID); err != nil {
		return Policy{}, err
	}

	limit := in.WeightedAverageReceiptLimit
	if limit == 0 {
		limit = existing.WeightedAverageReceiptLimit
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Policy{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var newVersion int64
	err = tx.QueryRow(ctx, `
		UPDATE vendor_cost_policies
		   SET strategy=$4, currency=$5, weighted_average_receipt_limit=$6,
		       manual_unit_cost_minor=$7, manual_unit_code=$8,
		       manual_cost_reason=$9,
		       version=version+1, updated_at=now(), updated_by_staff_id=$10
		 WHERE business_id=$1 AND id=$2 AND version=$3
		RETURNING version`,
		actor.VendorID, policyID, expectedVersion,
		in.Strategy, in.Currency, limit,
		in.ManualUnitCostMinor, nullIfEmpty(in.ManualUnitCode),
		nullIfEmpty(in.ManualCostReason), actor.StaffID,
	).Scan(&newVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return Policy{}, ErrVersionConflict
	}
	if err != nil {
		return Policy{}, err
	}

	updated := existing
	updated.Strategy = in.Strategy
	updated.Currency = in.Currency
	updated.WeightedAverageReceiptLimit = limit
	updated.ManualUnitCostMinor = in.ManualUnitCostMinor
	updated.ManualUnitCode = in.ManualUnitCode
	updated.ManualCostReason = in.ManualCostReason
	updated.Version = newVersion

	// before/after are both recorded: a manual cost change must be auditable
	// as "from X to Y", not merely "changed".
	if err = s.production.RecordMutation(
		ctx, tx, actor,
		"cost.policy.updated", EventCostPolicyChanged,
		"vendor_cost_policy", policyID, updated.BranchID, newVersion,
		existing, updated, updated, in.Reason,
	); err != nil {
		return Policy{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return Policy{}, err
	}
	return updated, nil
}

// ArchivePolicy deactivates a policy. Archiving frees the ACTIVE uniqueness
// slot so a replacement can be created, and leaves every snapshot that already
// used it untouched.
func (s *Service) ArchivePolicy(
	ctx context.Context,
	actor production.Actor,
	policyID string,
	expectedVersion int64,
	reason string,
) (Policy, error) {
	existing, err := s.GetPolicy(ctx, actor, policyID)
	if err != nil {
		return Policy{}, err
	}
	if _, err = s.AuthorizeBranch(
		ctx, actor, PermCostPolicyManage, existing.BranchID); err != nil {
		return Policy{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Policy{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var newVersion int64
	err = tx.QueryRow(ctx, `
		UPDATE vendor_cost_policies
		   SET status='ARCHIVED', version=version+1,
		       updated_at=now(), updated_by_staff_id=$4
		 WHERE business_id=$1 AND id=$2 AND version=$3 AND status='ACTIVE'
		RETURNING version`,
		actor.VendorID, policyID, expectedVersion, actor.StaffID,
	).Scan(&newVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return Policy{}, ErrVersionConflict
	}
	if err != nil {
		return Policy{}, err
	}

	archived := existing
	archived.Status = StatusArchived
	archived.Version = newVersion

	if err = s.production.RecordMutation(
		ctx, tx, actor,
		"cost.policy.archived", EventCostPolicyChanged,
		"vendor_cost_policy", policyID, archived.BranchID, newVersion,
		existing, archived, archived, reason,
	); err != nil {
		return Policy{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return Policy{}, err
	}
	return archived, nil
}

// GetPolicy reads one policy under the caller's tenancy.
//
// business_id is in the WHERE clause, so another vendor's policy id returns
// ErrNotFound rather than their data.
func (s *Service) GetPolicy(
	ctx context.Context,
	actor production.Actor,
	policyID string,
) (Policy, error) {
	var policy Policy
	var branch *string
	var manualCost *int64
	var manualUnit, manualReason *string

	err := s.db.QueryRow(ctx, `
		SELECT id, branch_id, subject_type, subject_ref, strategy, currency,
		       weighted_average_receipt_limit,
		       manual_unit_cost_minor, manual_unit_code, manual_cost_reason,
		       status, version, created_at, updated_at
		  FROM vendor_cost_policies
		 WHERE business_id = $1 AND id = $2`,
		actor.VendorID, policyID,
	).Scan(
		&policy.ID, &branch, &policy.SubjectType, &policy.SubjectRef,
		&policy.Strategy, &policy.Currency, &policy.WeightedAverageReceiptLimit,
		&manualCost, &manualUnit, &manualReason,
		&policy.Status, &policy.Version, &policy.CreatedAt, &policy.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Policy{}, ErrNotFound
	}
	if err != nil {
		return Policy{}, err
	}
	if branch != nil {
		policy.BranchID = *branch
	}
	policy.ManualUnitCostMinor = manualCost
	if manualUnit != nil {
		policy.ManualUnitCode = *manualUnit
	}
	if manualReason != nil {
		policy.ManualCostReason = *manualReason
	}
	return policy, nil
}

// ListPolicies returns a bounded page of policies for the caller's tenant,
// restricted to branches the actor's scope reaches.
func (s *Service) ListPolicies(
	ctx context.Context,
	actor production.Actor,
	scope production.Scope,
	subjectType string,
	limit, offset int,
) ([]Policy, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	// A non-global actor sees only its own branches' overrides plus the
	// business-level defaults; it never sees another branch's policy.
	restricted := !scope.Global
	branches := scope.BranchIDs
	if branches == nil {
		branches = []string{}
	}

	rows, err := s.db.Query(ctx, `
		SELECT id, branch_id, subject_type, subject_ref, strategy, currency,
		       weighted_average_receipt_limit,
		       manual_unit_cost_minor, manual_unit_code, manual_cost_reason,
		       status, version, created_at, updated_at
		  FROM vendor_cost_policies
		 WHERE business_id = $1
		   AND ($2 = '' OR subject_type = $2)
		   AND (NOT $3::boolean OR branch_id IS NULL OR branch_id = ANY($4::text[]))
		 ORDER BY subject_type, subject_ref, branch_id NULLS FIRST
		 LIMIT $5 OFFSET $6`,
		actor.VendorID, subjectType, restricted, branches, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var policies []Policy
	for rows.Next() {
		var policy Policy
		var branch *string
		var manualCost *int64
		var manualUnit, manualReason *string
		if err = rows.Scan(
			&policy.ID, &branch, &policy.SubjectType, &policy.SubjectRef,
			&policy.Strategy, &policy.Currency,
			&policy.WeightedAverageReceiptLimit,
			&manualCost, &manualUnit, &manualReason,
			&policy.Status, &policy.Version, &policy.CreatedAt, &policy.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if branch != nil {
			policy.BranchID = *branch
		}
		policy.ManualUnitCostMinor = manualCost
		if manualUnit != nil {
			policy.ManualUnitCode = *manualUnit
		}
		if manualReason != nil {
			policy.ManualCostReason = *manualReason
		}
		policies = append(policies, policy)
	}
	return policies, rows.Err()
}

func nullIfEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
