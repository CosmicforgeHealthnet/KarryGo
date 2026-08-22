package cost

// Snapshot persistence and cost read models.
//
// IMMUTABILITY
//
// Everything except RECIPE_PREVIEW is written once and never updated. This
// package does not contain an UPDATE or DELETE against vendor_cost_snapshots
// for those types, and migration 000074 installs triggers that raise if anyone
// ever adds one. Two layers, because a service-only guarantee lasts exactly
// until the next handler is written by someone who has not read this file.
//
// IDEMPOTENCY
//
// Two constraints do the work, so a retry cannot create a second history:
//
//   - vendor_cost_snapshots_production_uq: one PRODUCTION_EXPECTED and one
//     PRODUCTION_ACTUAL per batch, ever.
//   - vendor_cost_snapshots_command_uq: one snapshot per command_ref.
//
// A conflicting insert returns the row that already exists rather than an
// error, so a retried command is indistinguishable from the first attempt.

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrSnapshotImmutable is returned when a caller attempts to change settled
// historical evidence.
var ErrSnapshotImmutable = errors.New("cost: historical cost snapshot is immutable")

func newCostID(prefix string) string { return prefix + uuid.NewString() }

// PersistSnapshot writes a snapshot and its component evidence in one
// transaction.
//
// commandRef, when supplied, makes the write idempotent. If a snapshot already
// exists for this command or for this batch and calculation type, the existing
// one is returned unchanged - which is the behaviour a retried request needs
// and the behaviour a second concurrent finalization must get.
func PersistSnapshot(
	ctx context.Context,
	db *pgxpool.Pool,
	businessID string,
	snapshot Snapshot,
	staffID, correlationID, commandRef string,
) (Snapshot, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	stored, err := persistSnapshotTx(
		ctx, tx, businessID, snapshot, staffID, correlationID, commandRef)
	if err != nil {
		return Snapshot{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Snapshot{}, err
	}
	return stored, nil
}

func persistSnapshotTx(
	ctx context.Context,
	tx pgx.Tx,
	businessID string,
	snapshot Snapshot,
	staffID, correlationID, commandRef string,
) (Snapshot, error) {
	snapshot.ID = newCostID("cs_")
	snapshot.CalculatedBy = staffID
	snapshot.CalculationVersion = calculationVersion

	var branch, recipe, recipeVersion, batch, command *string
	if snapshot.BranchID != "" {
		branch = &snapshot.BranchID
	}
	if snapshot.RecipeID != "" {
		recipe = &snapshot.RecipeID
	}
	if snapshot.RecipeVersionID != "" {
		recipeVersion = &snapshot.RecipeVersionID
	}
	if snapshot.BatchID != "" {
		batch = &snapshot.BatchID
	}
	if commandRef != "" {
		command = &commandRef
	}

	var storedID string
	err := tx.QueryRow(ctx, `
		INSERT INTO vendor_cost_snapshots(
			id, business_id, branch_id, calculation_type, calculation_version,
			recipe_id, recipe_version_id, batch_id,
			strategy, currency, completeness,
			total_cost_minor, ingredient_cost_minor, packaging_cost_minor,
			waste_cost_minor,
			output_quantity_numerator, output_quantity_denominator, serving_count,
			expected_waste_bps, calculated_by_staff_id, correlation_id, command_ref
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22
		)
		ON CONFLICT DO NOTHING
		RETURNING id`,
		snapshot.ID, businessID, branch,
		snapshot.CalculationType, snapshot.CalculationVersion,
		recipe, recipeVersion, batch,
		snapshot.Strategy, snapshot.Currency, snapshot.Completeness,
		snapshot.TotalCostMinor, snapshot.IngredientCostMinor,
		snapshot.PackagingCostMinor, snapshot.WasteCostMinor,
		snapshot.OutputQuantityNum, snapshot.OutputQuantityDen,
		snapshot.ServingCount,
		snapshot.ExpectedWasteBPS, staffID, correlationID, command,
	).Scan(&storedID)

	if errors.Is(err, pgx.ErrNoRows) {
		// A snapshot already satisfied one of the uniqueness constraints. The
		// caller retried, or two finalizations raced. Return what exists; do
		// not write a competing history.
		return loadExistingSnapshotTx(ctx, tx, businessID, snapshot, commandRef)
	}
	if err != nil {
		return Snapshot{}, err
	}

	for _, component := range snapshot.Components {
		if err = insertComponentTx(ctx, tx, businessID, snapshot.ID, component); err != nil {
			return Snapshot{}, err
		}
	}

	return snapshot, nil
}

func insertComponentTx(
	ctx context.Context,
	tx pgx.Tx,
	businessID, snapshotID string,
	component Component,
) error {
	var inventoryItem, evidenceRef, reason *string
	var receiptCount *int
	if component.InventoryItemID != "" {
		inventoryItem = &component.InventoryItemID
	}
	if component.SourceEvidenceRef != "" {
		evidenceRef = &component.SourceEvidenceRef
	}
	if component.IncompleteReason != "" {
		reason = &component.IncompleteReason
	}
	if component.SourceReceiptCount > 0 {
		receiptCount = &component.SourceReceiptCount
	}

	_, err := tx.Exec(ctx, `
		INSERT INTO vendor_cost_snapshot_components(
			id, business_id, snapshot_id, line_number,
			subject_type, subject_ref, inventory_item_id,
			quantity_numerator, quantity_denominator, quantity_unit_code,
			cost_source, source_evidence_ref, source_receipt_count,
			unit_cost_minor, currency, component_cost_minor,
			expected_waste_bps, waste_cost_minor, incomplete_reason
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19
		)`,
		newCostID("csc_"), businessID, snapshotID, component.LineNumber,
		component.SubjectType, component.SubjectRef, inventoryItem,
		component.QuantityNum, component.QuantityDen, component.QuantityUnit,
		component.CostSource, evidenceRef, receiptCount,
		component.UnitCostMinor, component.Currency, component.ComponentCostMinor,
		component.ExpectedWasteBPS, component.WasteCostMinor, reason,
	)
	return err
}

func loadExistingSnapshotTx(
	ctx context.Context,
	tx pgx.Tx,
	businessID string,
	attempted Snapshot,
	commandRef string,
) (Snapshot, error) {
	var id string
	var err error

	if commandRef != "" {
		err = tx.QueryRow(ctx, `
			SELECT id FROM vendor_cost_snapshots
			 WHERE business_id = $1 AND command_ref = $2`,
			businessID, commandRef).Scan(&id)
	} else {
		err = tx.QueryRow(ctx, `
			SELECT id FROM vendor_cost_snapshots
			 WHERE business_id = $1 AND batch_id = $2 AND calculation_type = $3`,
			businessID, attempted.BatchID, attempted.CalculationType).Scan(&id)
	}
	if err != nil {
		return Snapshot{}, err
	}

	stored := attempted
	stored.ID = id
	return stored, nil
}

const snapshotColumns = `
	id, branch_id, calculation_type, calculation_version,
	recipe_id, recipe_version_id, batch_id,
	strategy, currency, completeness,
	total_cost_minor, ingredient_cost_minor, packaging_cost_minor, waste_cost_minor,
	output_quantity_numerator, output_quantity_denominator, serving_count,
	expected_waste_bps, calculated_at, calculated_by_staff_id`

func scanSnapshot(row scannable) (Snapshot, error) {
	var snapshot Snapshot
	var branch, recipe, recipeVersion, batch *string
	err := row.Scan(
		&snapshot.ID, &branch, &snapshot.CalculationType, &snapshot.CalculationVersion,
		&recipe, &recipeVersion, &batch,
		&snapshot.Strategy, &snapshot.Currency, &snapshot.Completeness,
		&snapshot.TotalCostMinor, &snapshot.IngredientCostMinor,
		&snapshot.PackagingCostMinor, &snapshot.WasteCostMinor,
		&snapshot.OutputQuantityNum, &snapshot.OutputQuantityDen,
		&snapshot.ServingCount,
		&snapshot.ExpectedWasteBPS, &snapshot.CalculatedAt, &snapshot.CalculatedBy,
	)
	if err != nil {
		return Snapshot{}, err
	}
	if branch != nil {
		snapshot.BranchID = *branch
	}
	if recipe != nil {
		snapshot.RecipeID = *recipe
	}
	if recipeVersion != nil {
		snapshot.RecipeVersionID = *recipeVersion
	}
	if batch != nil {
		snapshot.BatchID = *batch
	}
	snapshot.derivePerUnit()
	return snapshot, nil
}

// derivePerUnit recomputes the per-unit and per-serving figures from the
// persisted total. They are derived rather than stored so they can never
// disagree with the total they came from.
func (s *Snapshot) derivePerUnit() {
	if s.TotalCostMinor == nil {
		return
	}
	if s.OutputQuantityNum != nil && s.OutputQuantityDen != nil {
		if perUnit, ok := PerUnitMinor(*s.TotalCostMinor, Quantity{
			Numerator:   *s.OutputQuantityNum,
			Denominator: *s.OutputQuantityDen,
		}); ok {
			s.CostPerUnitMinor = int64Ptr(perUnit)
		}
	}
	if s.ServingCount != nil && *s.ServingCount > 0 {
		if perServing, ok := PerUnitMinor(*s.TotalCostMinor, Quantity{
			Numerator:   *s.ServingCount,
			Denominator: 1,
		}); ok {
			s.CostPerServingMinor = int64Ptr(perServing)
		}
	}
}

// LoadProductionSnapshot reads one batch's stored expected or actual cost.
func (e *Engine) LoadProductionSnapshot(
	ctx context.Context,
	businessID, batchID, calculationType string,
) (Snapshot, bool, error) {
	rows, err := e.db.Query(ctx, `
		SELECT`+snapshotColumns+`
		  FROM vendor_cost_snapshots
		 WHERE business_id = $1 AND batch_id = $2 AND calculation_type = $3`,
		businessID, batchID, calculationType)
	if err != nil {
		return Snapshot{}, false, err
	}
	defer rows.Close()

	if !rows.Next() {
		return Snapshot{}, false, rows.Err()
	}
	snapshot, err := scanSnapshot(rows)
	if err != nil {
		return Snapshot{}, false, err
	}
	rows.Close()

	snapshot.Components, err = e.LoadComponents(ctx, businessID, snapshot.ID)
	if err != nil {
		return Snapshot{}, false, err
	}
	return snapshot, true, nil
}

// LoadComponents reads a snapshot's component evidence.
//
// business_id is filtered here too, not only on the parent: a caller holding
// another vendor's snapshot id must not read its component breakdown.
func (e *Engine) LoadComponents(
	ctx context.Context,
	businessID, snapshotID string,
) ([]Component, error) {
	rows, err := e.db.Query(ctx, `
		SELECT line_number, subject_type, subject_ref, inventory_item_id,
		       quantity_numerator, quantity_denominator, quantity_unit_code,
		       cost_source, source_evidence_ref, source_receipt_count,
		       unit_cost_minor, currency, component_cost_minor,
		       expected_waste_bps, waste_cost_minor, incomplete_reason
		  FROM vendor_cost_snapshot_components
		 WHERE business_id = $1 AND snapshot_id = $2
		 ORDER BY line_number`, businessID, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var components []Component
	for rows.Next() {
		var component Component
		var inventoryItem, evidenceRef, reason *string
		var receiptCount *int
		if err = rows.Scan(
			&component.LineNumber, &component.SubjectType, &component.SubjectRef,
			&inventoryItem,
			&component.QuantityNum, &component.QuantityDen, &component.QuantityUnit,
			&component.CostSource, &evidenceRef, &receiptCount,
			&component.UnitCostMinor, &component.Currency,
			&component.ComponentCostMinor,
			&component.ExpectedWasteBPS, &component.WasteCostMinor, &reason,
		); err != nil {
			return nil, err
		}
		if inventoryItem != nil {
			component.InventoryItemID = *inventoryItem
		}
		if evidenceRef != nil {
			component.SourceEvidenceRef = *evidenceRef
		}
		if reason != nil {
			component.IncompleteReason = *reason
		}
		if receiptCount != nil {
			component.SourceReceiptCount = *receiptCount
		}
		component.Quantity = Quantity{
			Numerator:   component.QuantityNum,
			Denominator: component.QuantityDen,
		}
		components = append(components, component)
	}
	return components, rows.Err()
}

// LatestRecipeSnapshot returns the most recent immutable snapshot for a recipe
// version, which is what "the recipe's recorded COGS" means.
func (e *Engine) LatestRecipeSnapshot(
	ctx context.Context,
	businessID, recipeVersionID string,
) (Snapshot, bool, error) {
	rows, err := e.db.Query(ctx, `
		SELECT`+snapshotColumns+`
		  FROM vendor_cost_snapshots
		 WHERE business_id = $1 AND recipe_version_id = $2
		   AND calculation_type = $3
		 ORDER BY calculated_at DESC, id DESC
		 LIMIT 1`, businessID, recipeVersionID, CalcRecipeSnapshot)
	if err != nil {
		return Snapshot{}, false, err
	}
	defer rows.Close()

	if !rows.Next() {
		return Snapshot{}, false, rows.Err()
	}
	snapshot, err := scanSnapshot(rows)
	if err != nil {
		return Snapshot{}, false, err
	}
	rows.Close()

	snapshot.Components, err = e.LoadComponents(ctx, businessID, snapshot.ID)
	if err != nil {
		return Snapshot{}, false, err
	}
	return snapshot, true, nil
}

// RecipeCostHistory returns a recipe's snapshots newest-first, bounded.
//
// Old snapshots are returned exactly as they were written: a v1 COGS stays at
// the v1 figure however much prices have moved since.
func (e *Engine) RecipeCostHistory(
	ctx context.Context,
	businessID, recipeID, recipeVersionID string,
	limit, offset int,
) ([]Snapshot, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := e.db.Query(ctx, `
		SELECT`+snapshotColumns+`
		  FROM vendor_cost_snapshots
		 WHERE business_id = $1
		   AND ($2 = '' OR recipe_id = $2)
		   AND ($3 = '' OR recipe_version_id = $3)
		   AND calculation_type = ANY(ARRAY['RECIPE_SNAPSHOT','RECIPE_PREVIEW'])
		 ORDER BY calculated_at DESC, id DESC
		 LIMIT $4 OFFSET $5`,
		businessID, recipeID, recipeVersionID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var snapshots []Snapshot
	for rows.Next() {
		snapshot, scanErr := scanSnapshot(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, rows.Err()
}

// ProductionCostHistory returns production cost snapshots for a branch,
// bounded and newest-first.
func (e *Engine) ProductionCostHistory(
	ctx context.Context,
	businessID, branchID string,
	limit, offset int,
) ([]Snapshot, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := e.db.Query(ctx, `
		SELECT`+snapshotColumns+`
		  FROM vendor_cost_snapshots
		 WHERE business_id = $1
		   AND ($2 = '' OR branch_id = $2)
		   AND calculation_type = ANY(ARRAY['PRODUCTION_EXPECTED','PRODUCTION_ACTUAL'])
		 ORDER BY calculated_at DESC, id DESC
		 LIMIT $3 OFFSET $4`, businessID, branchID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var snapshots []Snapshot
	for rows.Next() {
		snapshot, scanErr := scanSnapshot(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, rows.Err()
}

// HighestCostSubject is one row of the highest-cost materials read model.
type HighestCostSubject struct {
	SubjectType    string `json:"subject_type"`
	SubjectRef     string `json:"subject_ref"`
	TotalCostMinor int64  `json:"total_cost_minor"`
	Currency       string `json:"currency"`
	Observations   int    `json:"observations"`
}

// HighestCostSubjects ranks materials by contributed cost over recent
// snapshots. Bounded on both sides: a capped snapshot window and a capped
// result set. This is an operational read, not company-wide BI - that belongs
// to the Analytics Service.
//
// It is deliberately TWO queries rather than one.
//
// Expressed as a single statement with the snapshot window as a CTE, the
// planner mis-costs the join: it cannot see that the window is small, hash
// joins it against the whole component table and reads every evidence row. On
// an 80,000-row corpus that measured 36ms versus 4.9ms for the index nested
// loop - and the sequential scan grows with total evidence, while the nested
// loop grows only with the window. Marking the CTE MATERIALIZED did not change
// the choice.
//
// Passing an explicit array of snapshot ids gives the planner an accurate
// estimate, so it uses vendor_cost_snapshot_components_snapshot_idx. Both
// queries stay bounded, so this is two round trips regardless of data size -
// not an N+1.
func (e *Engine) HighestCostSubjects(
	ctx context.Context,
	businessID, branchID string,
	limit int,
) ([]HighestCostSubject, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	const snapshotWindow = 500

	windowRows, err := e.db.Query(ctx, `
		SELECT id FROM vendor_cost_snapshots
		 WHERE business_id = $1 AND ($2 = '' OR branch_id = $2)
		 ORDER BY calculated_at DESC, id DESC
		 LIMIT $3`, businessID, branchID, snapshotWindow)
	if err != nil {
		return nil, err
	}
	snapshotIDs := make([]string, 0, snapshotWindow)
	for windowRows.Next() {
		var id string
		if err = windowRows.Scan(&id); err != nil {
			windowRows.Close()
			return nil, err
		}
		snapshotIDs = append(snapshotIDs, id)
	}
	windowRows.Close()
	if err = windowRows.Err(); err != nil {
		return nil, err
	}
	if len(snapshotIDs) == 0 {
		return nil, nil
	}

	rows, err := e.db.Query(ctx, `
		SELECT c.subject_type, c.subject_ref,
		       sum(c.component_cost_minor)::bigint,
		       coalesce(max(c.currency),''),
		       count(*)::int
		  FROM vendor_cost_snapshot_components c
		 WHERE c.business_id = $1
		   AND c.snapshot_id = ANY($2)
		   AND c.component_cost_minor IS NOT NULL
		 GROUP BY c.subject_type, c.subject_ref
		 ORDER BY sum(c.component_cost_minor) DESC
		 LIMIT $3`, businessID, snapshotIDs, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []HighestCostSubject
	for rows.Next() {
		var entry HighestCostSubject
		if err = rows.Scan(&entry.SubjectType, &entry.SubjectRef,
			&entry.TotalCostMinor, &entry.Currency, &entry.Observations); err != nil {
			return nil, err
		}
		result = append(result, entry)
	}
	return result, rows.Err()
}
