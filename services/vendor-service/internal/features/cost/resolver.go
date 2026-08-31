package cost

// Strategy resolution: turning a policy plus Section 15 evidence into a unit
// cost, or into an honest statement that there is none.

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Resolver resolves policies and unit costs.
type Resolver struct {
	db        *pgxpool.Pool
	purchases PurchasingCostReader
}

// NewResolver builds the resolver over the Section 15 read port.
func NewResolver(db *pgxpool.Pool, purchases PurchasingCostReader) *Resolver {
	return &Resolver{db: db, purchases: purchases}
}

// ResolvePolicy finds the policy governing one material in one branch.
//
// Branch override wins over the business default, and only within its own
// branch: the ORDER BY puts a non-NULL branch_id first and the WHERE only ever
// admits this branch's row, so a policy from another branch can never be
// selected. That is the branch-scope-escape guard, and it is asserted directly.
func (r *Resolver) ResolvePolicy(
	ctx context.Context,
	businessID, branchID, subjectType, subjectRef string,
) (Policy, bool, error) {
	var policy Policy
	var branch *string
	var manualCost *int64
	var manualUnit, manualReason *string

	err := r.db.QueryRow(ctx, `
		SELECT id, branch_id, subject_type, subject_ref, strategy, currency,
		       weighted_average_receipt_limit,
		       manual_unit_cost_minor, manual_unit_code, manual_cost_reason,
		       status, version, created_at, updated_at
		  FROM vendor_cost_policies
		 WHERE business_id = $1
		   AND subject_type = $3
		   AND subject_ref = $4
		   AND status = 'ACTIVE'
		   AND (branch_id IS NULL OR branch_id = $2)
		 ORDER BY (branch_id IS NOT NULL) DESC
		 LIMIT 1`,
		businessID, branchID, subjectType, subjectRef,
	).Scan(
		&policy.ID, &branch, &policy.SubjectType, &policy.SubjectRef,
		&policy.Strategy, &policy.Currency,
		&policy.WeightedAverageReceiptLimit,
		&manualCost, &manualUnit, &manualReason,
		&policy.Status, &policy.Version, &policy.CreatedAt, &policy.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return Policy{}, false, nil
	}
	if err != nil {
		return Policy{}, false, err
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
	return policy, true, nil
}

// ResolveUnitCost applies one policy to Section 15 evidence.
//
// Every failure path returns Resolved=false with a Reason. None of them
// returns zero: a component that cannot be costed must be visibly uncosted,
// because a fabricated zero silently understates a recipe and nobody ever
// finds out.
func (r *Resolver) ResolveUnitCost(
	ctx context.Context,
	businessID, branchID, inventoryItemID string,
	policy Policy,
	prefetched map[string]PurchaseObservation,
) (ResolvedCost, error) {
	switch policy.Strategy {

	case StrategyManual:
		// Manual cost is consulted ONLY here, when the active policy selects
		// it. It can never quietly outrank purchase evidence, because no other
		// branch of this switch reads it.
		if policy.ManualUnitCostMinor == nil {
			return ResolvedCost{Reason: ReasonNoManualCost}, nil
		}
		return ResolvedCost{
			Resolved:      true,
			Source:        StrategyManual,
			UnitCostMinor: *policy.ManualUnitCostMinor,
			Currency:      policy.Currency,
			EvidenceRef:   policy.ID,
		}, nil

	case StrategyLatest:
		if inventoryItemID == "" {
			return ResolvedCost{Reason: ReasonNoInventoryMapping}, nil
		}

		observation, ok := prefetched[inventoryItemID]
		if !ok {
			var err error
			observation, ok, err = r.purchases.LatestAccepted(
				ctx, businessID, branchID, inventoryItemID)
			if err != nil {
				return ResolvedCost{}, err
			}
		}
		if !ok {
			return ResolvedCost{Reason: ReasonNoPurchaseEvidence}, nil
		}
		if observation.Currency != policy.Currency {
			// No FX exists on this platform. Excluding the evidence and
			// saying so is the only honest option; converting it at an
			// invented rate would be worse than reporting nothing.
			return ResolvedCost{Reason: ReasonCurrencyMismatch}, nil
		}
		return ResolvedCost{
			Resolved:      true,
			Source:        StrategyLatest,
			UnitCostMinor: observation.InventoryUnitCostMinor,
			Currency:      observation.Currency,
			EvidenceRef:   observation.EvidenceRef,
			ReceiptCount:  1,
		}, nil

	case StrategyWeightedAverage:
		if inventoryItemID == "" {
			return ResolvedCost{Reason: ReasonNoInventoryMapping}, nil
		}

		limit := policy.WeightedAverageReceiptLimit
		if limit <= 0 {
			limit = 10
		}

		observations, err := r.purchases.RecentAccepted(
			ctx, businessID, branchID, inventoryItemID, limit)
		if err != nil {
			return ResolvedCost{}, err
		}
		if len(observations) == 0 {
			return ResolvedCost{Reason: ReasonNoPurchaseEvidence}, nil
		}

		// Same-currency only. Mixed-currency evidence is not averaged into a
		// meaningless number; the caller is told the basis is incompatible.
		quantities := make([]int64, 0, len(observations))
		unitCosts := make([]int64, 0, len(observations))
		for _, observation := range observations {
			if observation.Currency != policy.Currency {
				return ResolvedCost{Reason: ReasonCurrencyMismatch}, nil
			}
			quantities = append(quantities, observation.InventoryQuantity)
			unitCosts = append(unitCosts, observation.InventoryUnitCostMinor)
		}

		average, ok := WeightedAverageMinor(quantities, unitCosts)
		if !ok {
			return ResolvedCost{Reason: ReasonNoPurchaseEvidence}, nil
		}
		return ResolvedCost{
			Resolved:      true,
			Source:        StrategyWeightedAverage,
			UnitCostMinor: average,
			Currency:      policy.Currency,
			// The newest row in the window identifies the basis; the count
			// records how much evidence went into it.
			EvidenceRef:  observations[0].EvidenceRef,
			ReceiptCount: len(observations),
		}, nil
	}

	return ResolvedCost{Reason: ReasonNoPolicy}, nil
}

// InventoryItemFor maps a Section 16A material to the branch inventory item
// whose purchase history carries its cost.
//
// The mapping lives on Section 16A's branch config, which is why cost is a
// branch-scoped question even when the policy is business-wide: the same
// ingredient can map to different inventory items in different branches.
func (r *Resolver) InventoryItemFor(
	ctx context.Context,
	businessID, branchID, subjectType, subjectRef string,
) (string, error) {
	table := "vendor_ingredient_branch_configs"
	column := "ingredient_id"
	if subjectType == SubjectPackaging {
		table = "vendor_packaging_branch_configs"
		column = "packaging_material_id"
	}

	var inventoryItemID *string
	err := r.db.QueryRow(ctx, `
		SELECT inventory_item_id
		  FROM `+table+`
		 WHERE business_id = $1 AND branch_id = $2 AND `+column+` = $3`,
		businessID, branchID, subjectRef,
	).Scan(&inventoryItemID)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if inventoryItemID == nil {
		return "", nil
	}
	return *inventoryItemID, nil
}

// InventoryItemsFor resolves many materials in one query, so a recipe's
// components do not each cost a round trip.
func (r *Resolver) InventoryItemsFor(
	ctx context.Context,
	businessID, branchID, subjectType string,
	subjectRefs []string,
) (map[string]string, error) {
	result := map[string]string{}
	if len(subjectRefs) == 0 {
		return result, nil
	}

	table := "vendor_ingredient_branch_configs"
	column := "ingredient_id"
	if subjectType == SubjectPackaging {
		table = "vendor_packaging_branch_configs"
		column = "packaging_material_id"
	}

	rows, err := r.db.Query(ctx, `
		SELECT `+column+`, inventory_item_id
		  FROM `+table+`
		 WHERE business_id = $1 AND branch_id = $2 AND `+column+` = ANY($3)
		   AND inventory_item_id IS NOT NULL`,
		businessID, branchID, subjectRefs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var ref, itemID string
		if err = rows.Scan(&ref, &itemID); err != nil {
			return nil, err
		}
		result[ref] = itemID
	}
	return result, rows.Err()
}

// ResolvePolicies loads the governing policy for many materials at once,
// applying the same branch-override-then-business-default precedence as
// ResolvePolicy. DISTINCT ON does the precedence in the database rather than
// issuing one query per component.
func (r *Resolver) ResolvePolicies(
	ctx context.Context,
	businessID, branchID, subjectType string,
	subjectRefs []string,
) (map[string]Policy, error) {
	result := map[string]Policy{}
	if len(subjectRefs) == 0 {
		return result, nil
	}

	rows, err := r.db.Query(ctx, `
		SELECT DISTINCT ON (subject_ref)
		       id, branch_id, subject_type, subject_ref, strategy, currency,
		       weighted_average_receipt_limit,
		       manual_unit_cost_minor, manual_unit_code, manual_cost_reason,
		       status, version, created_at, updated_at
		  FROM vendor_cost_policies
		 WHERE business_id = $1
		   AND subject_type = $3
		   AND subject_ref = ANY($4)
		   AND status = 'ACTIVE'
		   AND (branch_id IS NULL OR branch_id = $2)
		 ORDER BY subject_ref, (branch_id IS NOT NULL) DESC`,
		businessID, branchID, subjectType, subjectRefs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

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
		result[policy.SubjectRef] = policy
	}
	return result, rows.Err()
}
