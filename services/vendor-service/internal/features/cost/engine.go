package cost

// Recipe estimated COGS.
//
// The shape of every calculation in this package is the same: gather the
// material lines, resolve each one's cost through its policy, and record both
// the numbers and the reasoning. What differs between calculation types is
// only where the quantities come from and whether expected waste has already
// been applied to them. See the package doc.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Engine calculates recipe and production cost.
type Engine struct {
	db       *pgxpool.Pool
	resolver *Resolver
}

// NewEngine builds the cost engine.
func NewEngine(db *pgxpool.Pool, resolver *Resolver) *Engine {
	return &Engine{db: db, resolver: resolver}
}

// recipeLine is one material requirement read from Section 16B.
type recipeLine struct {
	subjectType string
	subjectRef  string
	// netQuantity is the NET requirement as Section 16B persists it. The
	// expected-waste gross-up has NOT been applied.
	netQuantity Quantity
	unitCode    string
	wasteBPS    int
}

// recipeVersionHeader is the costing context of one recipe version.
type recipeVersionHeader struct {
	recipeID         string
	outputNumerator  int64
	outputDenominato int64
	outputUnitCode   string
	servingCount     int64
	expectedWasteBPS int
	status           string
}

// loadRecipeVersion reads one version under the caller's tenancy.
//
// business_id is in the WHERE clause, not merely in the join: a caller holding
// another vendor's version id must get "not found", never a costed answer.
// This is the nested-IDOR guard for every recipe cost path.
func (e *Engine) loadRecipeVersion(
	ctx context.Context,
	businessID, recipeVersionID string,
) (recipeVersionHeader, error) {
	var header recipeVersionHeader
	err := e.db.QueryRow(ctx, `
		SELECT recipe_id, expected_output_numerator, expected_output_denominator,
		       output_unit_code, serving_count, expected_waste_bps, status
		  FROM vendor_recipe_versions
		 WHERE business_id = $1 AND id = $2`,
		businessID, recipeVersionID,
	).Scan(
		&header.recipeID, &header.outputNumerator, &header.outputDenominato,
		&header.outputUnitCode, &header.servingCount,
		&header.expectedWasteBPS, &header.status,
	)
	return header, err
}

// ActiveVersionFor finds the recipe's current ACTIVE version, which is what a
// "current estimated COGS" read means.
func (e *Engine) ActiveVersionFor(
	ctx context.Context,
	businessID, recipeID string,
) (string, error) {
	var versionID string
	err := e.db.QueryRow(ctx, `
		SELECT id FROM vendor_recipe_versions
		 WHERE business_id = $1 AND recipe_id = $2 AND status = 'ACTIVE'
		 LIMIT 1`, businessID, recipeID).Scan(&versionID)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return versionID, err
}

// loadRecipeLines reads components and packaging in two set-based queries
// rather than one per line.
func (e *Engine) loadRecipeLines(
	ctx context.Context,
	businessID, recipeVersionID string,
	versionWasteBPS int,
) ([]recipeLine, error) {
	var lines []recipeLine

	componentRows, err := e.db.Query(ctx, `
		SELECT ingredient_id,
		       normalized_quantity_numerator, normalized_quantity_denominator,
		       normalized_unit_code, unit_code, expected_waste_bps
		  FROM vendor_recipe_components
		 WHERE business_id = $1 AND recipe_version_id = $2
		 ORDER BY id`, businessID, recipeVersionID)
	if err != nil {
		return nil, err
	}
	for componentRows.Next() {
		var line recipeLine
		var normalizedUnit, rawUnit string
		var componentWaste int
		if err = componentRows.Scan(
			&line.subjectRef,
			&line.netQuantity.Numerator, &line.netQuantity.Denominator,
			&normalizedUnit, &rawUnit, &componentWaste,
		); err != nil {
			componentRows.Close()
			return nil, err
		}
		line.subjectType = SubjectIngredient
		line.unitCode = normalizedUnit
		if line.unitCode == "" {
			line.unitCode = rawUnit
		}
		// Section 16C's precedence: a component's own expected waste overrides
		// the version-level figure, and zero means "inherit" rather than "no
		// waste". Reproduced here so a recipe estimate and the batch it later
		// produces gross up by the same amount.
		line.wasteBPS = componentWaste
		if line.wasteBPS == 0 {
			line.wasteBPS = versionWasteBPS
		}
		lines = append(lines, line)
	}
	componentRows.Close()
	if err = componentRows.Err(); err != nil {
		return nil, err
	}

	packagingRows, err := e.db.Query(ctx, `
		SELECT packaging_material_id,
		       normalized_quantity_numerator, normalized_quantity_denominator,
		       normalized_unit_code, unit_code
		  FROM vendor_recipe_packaging_requirements
		 WHERE business_id = $1 AND recipe_version_id = $2
		 ORDER BY sequence, id`, businessID, recipeVersionID)
	if err != nil {
		return nil, err
	}
	for packagingRows.Next() {
		var line recipeLine
		var normalizedUnit, rawUnit string
		if err = packagingRows.Scan(
			&line.subjectRef,
			&line.netQuantity.Numerator, &line.netQuantity.Denominator,
			&normalizedUnit, &rawUnit,
		); err != nil {
			packagingRows.Close()
			return nil, err
		}
		line.subjectType = SubjectPackaging
		line.unitCode = normalizedUnit
		if line.unitCode == "" {
			line.unitCode = rawUnit
		}
		// Packaging is not grossed up for ingredient waste: a wrapper is not
		// consumed faster because the filling shrinks.
		line.wasteBPS = 0
		lines = append(lines, line)
	}
	packagingRows.Close()
	if err = packagingRows.Err(); err != nil {
		return nil, err
	}

	return lines, nil
}

// costLines resolves and costs a set of material lines.
//
// applyWaste selects the semantics: true for a recipe estimate reading NET
// Section 16B quantities, false for production costs whose quantities already
// carry the gross-up. That single flag is the double-count boundary.
func (e *Engine) costLines(
	ctx context.Context,
	businessID, branchID string,
	lines []recipeLine,
	applyWaste bool,
	inventoryOverride map[string]string,
) ([]Component, error) {
	// Batch the two lookups that would otherwise be per-line round trips.
	ingredientRefs := make([]string, 0, len(lines))
	packagingRefs := make([]string, 0, len(lines))
	for _, line := range lines {
		if line.subjectType == SubjectIngredient {
			ingredientRefs = append(ingredientRefs, line.subjectRef)
		} else {
			packagingRefs = append(packagingRefs, line.subjectRef)
		}
	}

	inventoryByRef := map[string]string{}
	for ref, item := range inventoryOverride {
		inventoryByRef[ref] = item
	}

	if len(inventoryOverride) == 0 {
		ingredientItems, err := e.resolver.InventoryItemsFor(
			ctx, businessID, branchID, SubjectIngredient, ingredientRefs)
		if err != nil {
			return nil, err
		}
		for ref, item := range ingredientItems {
			inventoryByRef[ref] = item
		}
		packagingItems, err := e.resolver.InventoryItemsFor(
			ctx, businessID, branchID, SubjectPackaging, packagingRefs)
		if err != nil {
			return nil, err
		}
		for ref, item := range packagingItems {
			inventoryByRef[ref] = item
		}
	}

	ingredientPolicies, err := e.resolver.ResolvePolicies(
		ctx, businessID, branchID, SubjectIngredient, ingredientRefs)
	if err != nil {
		return nil, err
	}
	packagingPolicies, err := e.resolver.ResolvePolicies(
		ctx, businessID, branchID, SubjectPackaging, packagingRefs)
	if err != nil {
		return nil, err
	}

	// One purchase-evidence read for every item that will use the latest
	// strategy, instead of one per component.
	latestItems := make([]string, 0, len(lines))
	for _, line := range lines {
		policies := ingredientPolicies
		if line.subjectType == SubjectPackaging {
			policies = packagingPolicies
		}
		if policy, ok := policies[line.subjectRef]; ok && policy.Strategy == StrategyLatest {
			if item := inventoryByRef[line.subjectRef]; item != "" {
				latestItems = append(latestItems, item)
			}
		}
	}
	prefetched, err := e.resolver.purchases.LatestAcceptedBatch(
		ctx, businessID, branchID, latestItems)
	if err != nil {
		return nil, err
	}

	components := make([]Component, 0, len(lines))
	for index, line := range lines {
		component := Component{
			LineNumber:       index + 1,
			SubjectType:      line.subjectType,
			SubjectRef:       line.subjectRef,
			InventoryItemID:  inventoryByRef[line.subjectRef],
			QuantityUnit:     line.unitCode,
			ExpectedWasteBPS: line.wasteBPS,
		}

		quantity := line.netQuantity
		if quantity.Denominator <= 0 {
			quantity.Denominator = 1
		}

		// The gross-up, applied here and nowhere else on this path.
		costedQuantity := quantity
		if applyWaste && line.wasteBPS > 0 {
			grossed, wasteErr := ApplyExpectedWaste(quantity, line.wasteBPS)
			if wasteErr != nil {
				return nil, wasteErr
			}
			costedQuantity = grossed
		}
		component.Quantity = costedQuantity
		component.QuantityNum = costedQuantity.Numerator
		component.QuantityDen = costedQuantity.Denominator

		policies := ingredientPolicies
		if line.subjectType == SubjectPackaging {
			policies = packagingPolicies
		}
		policy, hasPolicy := policies[line.subjectRef]
		if !hasPolicy {
			component.CostSource = SourceUnresolved
			component.IncompleteReason = ReasonNoPolicy
			components = append(components, component)
			continue
		}

		resolved, resolveErr := e.resolver.ResolveUnitCost(
			ctx, businessID, branchID,
			inventoryByRef[line.subjectRef], policy, prefetched)
		if resolveErr != nil {
			return nil, resolveErr
		}
		if !resolved.Resolved {
			component.CostSource = SourceUnresolved
			component.IncompleteReason = resolved.Reason
			components = append(components, component)
			continue
		}

		amount, costErr := CostOf(costedQuantity, resolved.UnitCostMinor)
		if costErr != nil {
			return nil, costErr
		}

		component.CostSource = resolved.Source
		component.SourceEvidenceRef = resolved.EvidenceRef
		component.SourceReceiptCount = resolved.ReceiptCount
		component.UnitCostMinor = int64Ptr(resolved.UnitCostMinor)
		component.Currency = resolved.Currency
		component.ComponentCostMinor = int64Ptr(amount)

		// The waste contribution is the difference between the grossed-up cost
		// and what the net quantity alone would have cost, so waste is visible
		// as a number rather than buried inside the component total.
		if applyWaste && line.wasteBPS > 0 {
			netAmount, netErr := CostOf(quantity, resolved.UnitCostMinor)
			if netErr != nil {
				return nil, netErr
			}
			component.WasteCostMinor = int64Ptr(amount - netAmount)
		}

		components = append(components, component)
	}

	return components, nil
}

// summarise folds costed components into snapshot totals.
//
// A snapshot is INCOMPLETE if any line failed to resolve, and UNKNOWN if none
// resolved at all. The total of a partial calculation is still reported,
// because "we know at least this much" is more useful than nothing - but it is
// never labelled COMPLETE.
func summarise(components []Component) (
	total, ingredient, packaging, waste int64,
	currency, strategy, completeness string,
	err error,
) {
	resolvedCount := 0
	strategies := map[string]bool{}
	currencies := map[string]bool{}

	for _, component := range components {
		if component.ComponentCostMinor == nil {
			continue
		}
		resolvedCount++
		strategies[component.CostSource] = true
		if component.Currency != "" {
			currencies[component.Currency] = true
		}

		amount := *component.ComponentCostMinor
		total, err = SumMinor(total, amount)
		if err != nil {
			return
		}
		if component.SubjectType == SubjectPackaging {
			packaging, err = SumMinor(packaging, amount)
		} else {
			ingredient, err = SumMinor(ingredient, amount)
		}
		if err != nil {
			return
		}
		if component.WasteCostMinor != nil {
			waste, err = SumMinor(waste, *component.WasteCostMinor)
			if err != nil {
				return
			}
		}
	}

	switch {
	case resolvedCount == 0:
		completeness = CompletenessUnknown
	case resolvedCount < len(components):
		completeness = CompletenessIncomplete
	default:
		completeness = CompletenessComplete
	}

	switch len(strategies) {
	case 0:
		strategy = StrategyMixed
	case 1:
		for value := range strategies {
			strategy = value
		}
	default:
		strategy = StrategyMixed
	}

	// More than one currency among resolved components would make the total
	// meaningless. Policy resolution already excludes mismatched evidence per
	// component, so this is a belt-and-braces guard on the aggregate.
	if len(currencies) > 1 {
		completeness = CompletenessIncomplete
	}
	for value := range currencies {
		currency = value
	}

	return
}

// CalculateRecipeCost produces a recipe version's estimated COGS.
//
// It never writes to the recipe. The returned snapshot is not persisted here;
// the caller decides whether this is a recalculable preview or immutable
// evidence.
func (e *Engine) CalculateRecipeCost(
	ctx context.Context,
	businessID, branchID, recipeVersionID string,
) (Snapshot, error) {
	header, err := e.loadRecipeVersion(ctx, businessID, recipeVersionID)
	if err == pgx.ErrNoRows {
		return Snapshot{}, fmt.Errorf("recipe version not found")
	}
	if err != nil {
		return Snapshot{}, err
	}

	lines, err := e.loadRecipeLines(ctx, businessID, recipeVersionID, header.expectedWasteBPS)
	if err != nil {
		return Snapshot{}, err
	}

	// applyWaste=true: Section 16B quantities are NET.
	components, err := e.costLines(ctx, businessID, branchID, lines, true, nil)
	if err != nil {
		return Snapshot{}, err
	}

	total, ingredient, packaging, waste, currency, strategy, completeness, err := summarise(components)
	if err != nil {
		return Snapshot{}, err
	}

	snapshot := Snapshot{
		BranchID:           branchID,
		CalculationType:    CalcRecipePreview,
		CalculationVersion: calculationVersion,
		RecipeID:           header.recipeID,
		RecipeVersionID:    recipeVersionID,
		Strategy:           strategy,
		Currency:           currency,
		Completeness:       completeness,
		ExpectedWasteBPS:   header.expectedWasteBPS,
		Components:         components,
	}

	if completeness != CompletenessUnknown {
		snapshot.TotalCostMinor = int64Ptr(total)
		snapshot.IngredientCostMinor = int64Ptr(ingredient)
		snapshot.PackagingCostMinor = int64Ptr(packaging)
		snapshot.WasteCostMinor = int64Ptr(waste)
	}

	if header.outputNumerator > 0 && header.outputDenominato > 0 {
		snapshot.OutputQuantityNum = int64Ptr(header.outputNumerator)
		snapshot.OutputQuantityDen = int64Ptr(header.outputDenominato)
		if snapshot.TotalCostMinor != nil {
			if perUnit, ok := PerUnitMinor(total, Quantity{
				Numerator:   header.outputNumerator,
				Denominator: header.outputDenominato,
			}); ok {
				snapshot.CostPerUnitMinor = int64Ptr(perUnit)
			}
		}
	}
	if header.servingCount > 0 {
		snapshot.ServingCount = int64Ptr(header.servingCount)
		if snapshot.TotalCostMinor != nil {
			if perServing, ok := PerUnitMinor(total, Quantity{
				Numerator:   header.servingCount,
				Denominator: 1,
			}); ok {
				snapshot.CostPerServingMinor = int64Ptr(perServing)
			}
		}
	}

	return snapshot, nil
}
