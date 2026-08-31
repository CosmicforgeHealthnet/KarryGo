package cost

// Production cost: expected, actual, and the variance between them.
//
// THE WASTE RULE, RESTATED WHERE IT MATTERS
//
// Everything in this file reads quantities that Section 16C already froze.
// Section 16C applied the expected-waste gross-up when it created those rows,
// so every costLines call here passes applyWaste=false. Passing true would
// gross the same waste up a second time and overstate every batch. That is the
// single most likely defect in this section and it is asserted by test.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// batchHeader is the costing context of one production batch.
type batchHeader struct {
	branchID        string
	recipeID        string
	recipeVersionID string
	status          string
	outputNum       int64
	outputDen       int64
}

// loadBatch reads a batch under the caller's tenancy. business_id is in the
// WHERE clause for the same reason as elsewhere: another vendor's batch id
// must not resolve.
func (e *Engine) loadBatch(
	ctx context.Context,
	businessID, batchID string,
) (batchHeader, error) {
	var header batchHeader
	var outputNum, outputDen *int64
	err := e.db.QueryRow(ctx, `
		SELECT branch_id, recipe_id, recipe_version_id, status,
		       actual_output_numerator, actual_output_denominator
		  FROM vendor_production_batches
		 WHERE business_id = $1 AND id = $2`,
		businessID, batchID,
	).Scan(&header.branchID, &header.recipeID, &header.recipeVersionID,
		&header.status, &outputNum, &outputDen)
	if err != nil {
		return batchHeader{}, err
	}
	if outputNum != nil {
		header.outputNum = *outputNum
	}
	if outputDen != nil {
		header.outputDen = *outputDen
	}
	return header, nil
}

// loadBatchLines reads the frozen requirements for a batch.
//
// actual=false reads planned quantities (already waste-inclusive);
// actual=true reads measured consumption plus measured waste, which is what
// the vendor genuinely spent.
func (e *Engine) loadBatchLines(
	ctx context.Context,
	businessID, batchID string,
	actual bool,
) ([]recipeLine, map[string]string, error) {
	var lines []recipeLine
	inventoryByRef := map[string]string{}

	componentRows, err := e.db.Query(ctx, `
		SELECT ingredient_id, unit_code, expected_waste_bps,
		       planned_quantity_numerator, planned_quantity_denominator,
		       actual_used_numerator, actual_used_denominator,
		       actual_waste_numerator, actual_waste_denominator,
		       inventory_item_id
		  FROM vendor_production_batch_components
		 WHERE business_id = $1 AND batch_id = $2
		 ORDER BY id`, businessID, batchID)
	if err != nil {
		return nil, nil, err
	}
	for componentRows.Next() {
		var line recipeLine
		var plannedNum, plannedDen, usedNum, usedDen, wasteNum, wasteDen int64
		var inventoryItem *string
		if err = componentRows.Scan(
			&line.subjectRef, &line.unitCode, &line.wasteBPS,
			&plannedNum, &plannedDen,
			&usedNum, &usedDen, &wasteNum, &wasteDen,
			&inventoryItem,
		); err != nil {
			componentRows.Close()
			return nil, nil, err
		}
		line.subjectType = SubjectIngredient
		if inventoryItem != nil {
			inventoryByRef[line.subjectRef] = *inventoryItem
		}

		if actual {
			// Actual cost is what left the store: what was consumed plus what
			// was wasted. Both are measured evidence from Section 16C; neither
			// is inferred from the recipe.
			used := Quantity{Numerator: usedNum, Denominator: usedDen}
			waste := Quantity{Numerator: wasteNum, Denominator: wasteDen}
			combined, sumErr := addQuantity(used, waste)
			if sumErr != nil {
				componentRows.Close()
				return nil, nil, sumErr
			}
			line.netQuantity = combined
		} else {
			line.netQuantity = Quantity{Numerator: plannedNum, Denominator: plannedDen}
		}
		lines = append(lines, line)
	}
	componentRows.Close()
	if err = componentRows.Err(); err != nil {
		return nil, nil, err
	}

	packagingRows, err := e.db.Query(ctx, `
		SELECT packaging_material_id, unit_code,
		       planned_quantity_numerator, planned_quantity_denominator,
		       actual_used_numerator, actual_used_denominator,
		       inventory_item_id
		  FROM vendor_production_batch_packaging
		 WHERE business_id = $1 AND batch_id = $2
		 ORDER BY id`, businessID, batchID)
	if err != nil {
		return nil, nil, err
	}
	for packagingRows.Next() {
		var line recipeLine
		var plannedNum, plannedDen, usedNum, usedDen int64
		var inventoryItem *string
		if err = packagingRows.Scan(
			&line.subjectRef, &line.unitCode,
			&plannedNum, &plannedDen, &usedNum, &usedDen,
			&inventoryItem,
		); err != nil {
			packagingRows.Close()
			return nil, nil, err
		}
		line.subjectType = SubjectPackaging
		line.wasteBPS = 0
		if inventoryItem != nil {
			inventoryByRef[line.subjectRef] = *inventoryItem
		}
		if actual {
			line.netQuantity = Quantity{Numerator: usedNum, Denominator: usedDen}
		} else {
			line.netQuantity = Quantity{Numerator: plannedNum, Denominator: plannedDen}
		}
		lines = append(lines, line)
	}
	packagingRows.Close()
	if err = packagingRows.Err(); err != nil {
		return nil, nil, err
	}

	return lines, inventoryByRef, nil
}

// addQuantity sums two exact rationals.
func addQuantity(left, right Quantity) (Quantity, error) {
	if left.Denominator <= 0 {
		left.Denominator = 1
	}
	if right.Denominator <= 0 {
		right.Denominator = 1
	}
	if right.Numerator == 0 {
		return left, nil
	}
	if left.Numerator == 0 {
		return right, nil
	}
	sum := left.rat()
	sum.Add(sum, right.rat())
	if !sum.Num().IsInt64() || !sum.Denom().IsInt64() {
		return Quantity{}, ErrInvalidQuantity
	}
	return Quantity{Numerator: sum.Num().Int64(), Denominator: sum.Denom().Int64()}, nil
}

// CalculateProductionCost costs a batch from its own frozen evidence.
//
// calculationType selects expected (planned quantities) or actual (measured
// consumption and waste). Neither path consults the recipe's current version:
// a batch is costed from what was frozen for it, so a later recipe edit cannot
// retroactively change history.
func (e *Engine) CalculateProductionCost(
	ctx context.Context,
	businessID, batchID, calculationType string,
) (Snapshot, error) {
	header, err := e.loadBatch(ctx, businessID, batchID)
	if err == pgx.ErrNoRows {
		return Snapshot{}, fmt.Errorf("production batch not found")
	}
	if err != nil {
		return Snapshot{}, err
	}

	actual := calculationType == CalcProductionActual
	lines, inventoryByRef, err := e.loadBatchLines(ctx, businessID, batchID, actual)
	if err != nil {
		return Snapshot{}, err
	}

	// applyWaste=false. Section 16C already grossed these up. See the file doc.
	components, err := e.costLines(
		ctx, businessID, header.branchID, lines, false, inventoryByRef)
	if err != nil {
		return Snapshot{}, err
	}

	total, ingredient, packaging, waste, currency, strategy, completeness, err := summarise(components)
	if err != nil {
		return Snapshot{}, err
	}

	snapshot := Snapshot{
		BranchID:           header.branchID,
		CalculationType:    calculationType,
		CalculationVersion: calculationVersion,
		RecipeID:           header.recipeID,
		RecipeVersionID:    header.recipeVersionID,
		BatchID:            batchID,
		Strategy:           strategy,
		Currency:           currency,
		Completeness:       completeness,
		Components:         components,
	}

	if completeness != CompletenessUnknown {
		snapshot.TotalCostMinor = int64Ptr(total)
		snapshot.IngredientCostMinor = int64Ptr(ingredient)
		snapshot.PackagingCostMinor = int64Ptr(packaging)
		snapshot.WasteCostMinor = int64Ptr(waste)
	}

	if header.outputNum > 0 && header.outputDen > 0 {
		snapshot.OutputQuantityNum = int64Ptr(header.outputNum)
		snapshot.OutputQuantityDen = int64Ptr(header.outputDen)
		if snapshot.TotalCostMinor != nil {
			if perUnit, ok := PerUnitMinor(total, Quantity{
				Numerator:   header.outputNum,
				Denominator: header.outputDen,
			}); ok {
				snapshot.CostPerUnitMinor = int64Ptr(perUnit)
			}
		}
	}

	return snapshot, nil
}

// CalculateVariance compares a batch's persisted expected and actual snapshots.
//
// It reads the two immutable snapshots rather than recalculating either, which
// is what makes a variance reproducible: recomputing at read time would show a
// different number every time purchase prices moved.
//
// This produces an observation and nothing else. No recipe is edited, no price
// changes, no supplier is adjusted, no inventory moves and no financial record
// of any kind is written.
func (e *Engine) CalculateVariance(
	ctx context.Context,
	businessID, batchID string,
) (Variance, error) {
	variance := Variance{BatchID: batchID, Completeness: CompletenessIncomplete}

	expected, expectedOK, err := e.LoadProductionSnapshot(
		ctx, businessID, batchID, CalcProductionExpected)
	if err != nil {
		return Variance{}, err
	}
	actual, actualOK, err := e.LoadProductionSnapshot(
		ctx, businessID, batchID, CalcProductionActual)
	if err != nil {
		return Variance{}, err
	}

	if expectedOK {
		variance.ExpectedTotalMinor = expected.TotalCostMinor
		variance.ExpectedIngredientMinor = expected.IngredientCostMinor
		variance.ExpectedPackagingMinor = expected.PackagingCostMinor
		variance.ExpectedWasteMinor = expected.WasteCostMinor
		variance.Currency = expected.Currency
	}
	if actualOK {
		variance.ActualTotalMinor = actual.TotalCostMinor
		variance.ActualIngredientMinor = actual.IngredientCostMinor
		variance.ActualPackagingMinor = actual.PackagingCostMinor
		variance.ActualWasteMinor = actual.WasteCostMinor
		if variance.Currency == "" {
			variance.Currency = actual.Currency
		}
	}

	if !expectedOK || !actualOK {
		// Half a comparison is not a variance. Saying so is more useful than
		// presenting the half we have as if it were the whole.
		return variance, nil
	}

	if expected.Currency != "" && actual.Currency != "" &&
		expected.Currency != actual.Currency {
		variance.Completeness = CompletenessIncomplete
		return variance, nil
	}

	if expected.TotalCostMinor != nil && actual.TotalCostMinor != nil {
		variance.VarianceMinor = int64Ptr(
			VarianceMinor(*actual.TotalCostMinor, *expected.TotalCostMinor))
	}

	if expected.Completeness == CompletenessComplete &&
		actual.Completeness == CompletenessComplete {
		variance.Completeness = CompletenessComplete
	}

	variance.Components = componentVariance(expected.Components, actual.Components)
	return variance, nil
}

// componentVariance pairs expected and actual lines by material.
func componentVariance(expected, actual []Component) []ComponentVariance {
	actualByKey := map[string]Component{}
	for _, component := range actual {
		actualByKey[component.SubjectType+":"+component.SubjectRef] = component
	}

	result := make([]ComponentVariance, 0, len(expected))
	for _, expectedComponent := range expected {
		key := expectedComponent.SubjectType + ":" + expectedComponent.SubjectRef
		entry := ComponentVariance{
			SubjectType:         expectedComponent.SubjectType,
			SubjectRef:          expectedComponent.SubjectRef,
			ExpectedQuantityNum: expectedComponent.QuantityNum,
			ExpectedQuantityDen: expectedComponent.QuantityDen,
			ExpectedCostMinor:   expectedComponent.ComponentCostMinor,
		}
		if actualComponent, ok := actualByKey[key]; ok {
			entry.ActualQuantityNum = actualComponent.QuantityNum
			entry.ActualQuantityDen = actualComponent.QuantityDen
			entry.ActualCostMinor = actualComponent.ComponentCostMinor
			if expectedComponent.ComponentCostMinor != nil &&
				actualComponent.ComponentCostMinor != nil {
				entry.VarianceMinor = int64Ptr(VarianceMinor(
					*actualComponent.ComponentCostMinor,
					*expectedComponent.ComponentCostMinor))
			}
			delete(actualByKey, key)
		}
		result = append(result, entry)
	}

	// A material consumed but never planned is a real operational fact and
	// must not be dropped from the comparison.
	for _, actualComponent := range actualByKey {
		result = append(result, ComponentVariance{
			SubjectType:       actualComponent.SubjectType,
			SubjectRef:        actualComponent.SubjectRef,
			ActualQuantityNum: actualComponent.QuantityNum,
			ActualQuantityDen: actualComponent.QuantityDen,
			ActualCostMinor:   actualComponent.ComponentCostMinor,
			VarianceMinor:     actualComponent.ComponentCostMinor,
		})
	}
	return result
}
