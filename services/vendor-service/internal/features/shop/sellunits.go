package shop

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

// ---------------------------------------------------------------------------
// Sell units
// ---------------------------------------------------------------------------
//
// The first-class answer to "in what units may a customer buy this". Rice sells
// by Derica, by half Paint, by Paint, by 25 kg bag and by 50 kg bag from the
// same SKU; eggs by the piece, the six-pack, the dozen and the tray; tomatoes
// by the kilogram, the basket, the half basket and by a ₦500 portion.
//
// Fractions are exact rationals over a canonical measure, not new units. That
// is why the vocabulary stays finite: HALF_PAINT, QUARTER_PAINT, HALF_BAG and
// HALF_CARTON never need to exist.

const sellUnitCols = `id,business_id,product_id,variant_id,measure_definition_id,measure_owner_key,
	fraction_numerator,fraction_denominator,display_name,measurement_mode,conversion_type,
	COALESCE(exact_base_family,''),exact_base_quantity,COALESCE(calibration_id,''),estimated_base_quantity,
	underfill_tolerance_bps,overfill_tolerance_bps,actual_measurement_required,display_precision,
	price_minor,wholesale_price_minor,retail_enabled,wholesale_enabled,minimum_order_quantity,
	order_multiple,maximum_order_quantity,status,position,created_at,updated_at,archived_at,version`

func scanSellUnit(row rowScanner) (SellUnit, error) {
	var u SellUnit
	err := row.Scan(&u.ID, &u.BusinessID, &u.ProductID, &u.VariantID, &u.MeasureID, &u.MeasureOwnerKey,
		&u.FractionNumerator, &u.FractionDenominator, &u.DisplayName, &u.MeasurementMode, &u.ConversionType,
		&u.ExactBaseFamily, &u.ExactBaseQuantity, &u.CalibrationID, &u.EstimatedBaseQty,
		&u.UnderfillBps, &u.OverfillBps, &u.MeasurementRequired, &u.DisplayPrecision,
		&u.PriceMinor, &u.WholesalePriceMinor, &u.RetailEnabled, &u.WholesaleEnabled,
		&u.MinimumOrderQty, &u.OrderMultiple, &u.MaximumOrderQty, &u.Status, &u.Position,
		&u.CreatedAt, &u.UpdatedAt, &u.ArchivedAt, &u.Version)
	return u, err
}

func loadSellUnit(ctx context.Context, q querier, businessID, sellUnitID string, lock bool) (SellUnit, error) {
	sql := `SELECT ` + sellUnitCols + ` FROM shop_sell_units WHERE business_id=$1 AND id=$2`
	if lock {
		sql += ` FOR UPDATE`
	}
	unit, err := scanSellUnit(q.QueryRow(ctx, sql, businessID, strings.TrimSpace(sellUnitID)))
	if err != nil {
		return SellUnit{}, mapDB(err, "Sell unit", CodeSellUnitNotFound)
	}
	return unit, nil
}

// modeFamilies constrains which measurement mode may be used with which
// measurement family. A price portion over a kilogram, or a package count over
// a bunch, would each mean something the fulfilment side cannot honour.
var modeFamilies = map[string][]string{
	ModeFixed: {FamilyMass, FamilyVolume, FamilyCount, FamilyLocalContainer, FamilyPackage,
		FamilyGroupedCount, FamilyLoad, FamilyOther},
	ModeVariableWeight: {FamilyMass, FamilyVariableWeight},
	ModeVariableCount:  {FamilyCount, FamilyGroupedCount, FamilyVariableCount},
	ModeLocalContainer: {FamilyLocalContainer, FamilyLoad},
	ModePackageCount:   {FamilyPackage},
	ModePricePortion:   {FamilyPricePortion},
}

// CreateSellUnitInput is the sell-unit create command.
type CreateSellUnitInput struct {
	VariantID           string
	MeasureID           string
	FractionNumerator   *int
	FractionDenominator *int
	DisplayName         string
	MeasurementMode     string
	// CalibrationID makes the conversion explicitly approximate. It is refused
	// together with an exact conversion: a unit cannot be both.
	CalibrationID       string
	EstimatedBaseQty    *int64
	UnderfillBps        *int
	OverfillBps         *int
	MeasurementRequired *bool
	DisplayPrecision    *int
	PriceMinor          *int64
	WholesalePriceMinor *int64
	RetailEnabled       *bool
	WholesaleEnabled    *bool
	MinimumOrderQty     *int64
	OrderMultiple       *int64
	MaximumOrderQty     *int64
}

// CreateSellUnit adds a way to buy a variant.
func (s *Service) CreateSellUnit(ctx context.Context, actor Actor, scope Scope, productID string, in CreateSellUnitInput) (SellUnit, error) {
	if err := s.requireWrites(); err != nil {
		return SellUnit{}, err
	}
	if err := requireBusinessWide(scope, "Sell units"); err != nil {
		return SellUnit{}, err
	}
	numerator, denominator, err := normalizeFraction(in.FractionNumerator, in.FractionDenominator)
	if err != nil {
		return SellUnit{}, err
	}
	mode := strings.ToUpper(strings.TrimSpace(in.MeasurementMode))
	if mode == "" {
		mode = ModeFixed
	}
	if !contains(MeasurementModes, mode) {
		return SellUnit{}, fieldError("measurement_mode", "Choose a supported measurement mode.")
	}
	if err = validateMoney("price_minor", in.PriceMinor); err != nil {
		return SellUnit{}, err
	}
	if err = validateMoney("wholesale_price_minor", in.WholesalePriceMinor); err != nil {
		return SellUnit{}, err
	}
	underfill, overfill, err := normalizeTolerances(in.UnderfillBps, in.OverfillBps)
	if err != nil {
		return SellUnit{}, err
	}
	precision := 0
	if in.DisplayPrecision != nil {
		if *in.DisplayPrecision < 0 || *in.DisplayPrecision > 3 {
			return SellUnit{}, fieldError("display_precision", "Display precision must be between 0 and 3.")
		}
		precision = *in.DisplayPrecision
	}
	minimum, multiple, maximum, err := normalizeOrderRules(in.MinimumOrderQty, in.OrderMultiple, in.MaximumOrderQty)
	if err != nil {
		return SellUnit{}, err
	}
	if in.PriceMinor != nil {
		if err = s.requirePricingAuthority(ctx, actor, ""); err != nil {
			return SellUnit{}, err
		}
	}
	if in.WholesalePriceMinor != nil {
		if err = s.requireWholesaleAuthority(ctx, actor, ""); err != nil {
			return SellUnit{}, err
		}
	}
	// Order minimums and multiples are wholesale terms wherever they bind.
	if (minimum > 1 || multiple > 1 || maximum != nil) && in.WholesalePriceMinor == nil {
		if err = s.requireWholesaleAuthority(ctx, actor, ""); err != nil {
			return SellUnit{}, err
		}
	}

	var created SellUnit
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		product, err := loadProduct(ctx, tx, actor.VendorID, productID, false)
		if err != nil {
			return err
		}
		variant, err := loadVariant(ctx, tx, actor.VendorID, product.ID, in.VariantID, true)
		if err != nil {
			return err
		}
		if variant.Status == StatusArchived {
			return errArchived("This variant")
		}
		measure, err := loadMeasure(ctx, tx, actor.VendorID, in.MeasureID, false)
		if err != nil {
			return err
		}
		if measure.Status == StatusArchived {
			return errArchived("This measure definition")
		}
		if allowed, known := modeFamilies[mode]; known && !contains(allowed, measure.Family) {
			return coded(apperrors.Validation("Check your details.", []apperrors.FieldViolation{
				{Field: "measurement_mode", Message: "A " + mode + " sell unit cannot be measured in a " +
					measure.Family + " unit."}}), CodeConversionInvalid)
		}
		if mode == ModePricePortion && in.PriceMinor == nil {
			return coded(apperrors.Validation("Check your details.", []apperrors.FieldViolation{
				{Field: "price_minor", Message: "A price portion needs the price that defines it."}}),
				CodeConversionInvalid)
		}

		conversion, err := s.resolveConversion(ctx, tx, actor.VendorID, product.ID, variant.ID, measure,
			numerator, denominator, in.CalibrationID)
		if err != nil {
			return err
		}

		var count int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM shop_sell_units
			WHERE variant_id=$1 AND status <> 'ARCHIVED'`, variant.ID).Scan(&count); err != nil {
			return err
		}
		if count >= MaxSellUnitsPerVariant {
			return fieldError("sell_unit", "A variant may have at most "+itoa(MaxSellUnitsPerVariant)+" sell units.")
		}

		displayName := strings.TrimSpace(in.DisplayName)
		if displayName == "" {
			displayName = defaultSellUnitName(numerator, denominator, measure.DisplayName)
		}
		if len([]rune(displayName)) > 120 {
			return fieldError("display_name", "Use at most 120 characters.")
		}

		row, err := scanSellUnit(tx.QueryRow(ctx, `
			INSERT INTO shop_sell_units
				(id,business_id,product_id,variant_id,measure_definition_id,measure_owner_key,
				 fraction_numerator,fraction_denominator,display_name,measurement_mode,conversion_type,
				 exact_base_family,exact_base_quantity,calibration_id,estimated_base_quantity,
				 underfill_tolerance_bps,overfill_tolerance_bps,actual_measurement_required,display_precision,
				 price_minor,wholesale_price_minor,retail_enabled,wholesale_enabled,
				 minimum_order_quantity,order_multiple,maximum_order_quantity,status,position)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NULLIF($12,''),$13,NULLIF($14,''),$15,$16,$17,$18,$19,
			        $20,$21,$22,$23,$24,$25,$26,'ACTIVE',$27)
			RETURNING `+sellUnitCols,
			newID("ssu_"), actor.VendorID, product.ID, variant.ID, measure.ID, measure.OwnerKey,
			numerator, denominator, displayName, mode, conversion.conversionType,
			conversion.baseFamily, conversion.baseQuantity, conversion.calibrationID, in.EstimatedBaseQty,
			underfill, overfill, boolOrDefault(in.MeasurementRequired, mode == ModeVariableWeight || mode == ModeVariableCount),
			precision, in.PriceMinor, in.WholesalePriceMinor,
			boolOrDefault(in.RetailEnabled, true), boolOrDefault(in.WholesaleEnabled, false),
			minimum, multiple, maximum, count))
		if err != nil {
			return mapDB(err, "Sell unit", CodeSellUnitNotFound)
		}
		created = row

		currency, err := s.businessCurrency(ctx, tx, actor.VendorID)
		if err != nil {
			return err
		}
		if in.PriceMinor != nil {
			if err = s.appendPriceVersion(ctx, tx, actor, currency, priceChange{
				subjectType: SubjectSellUnitRetail, subjectRef: created.ID, productRef: product.ID,
				variantRef: variant.ID, amount: in.PriceMinor, reason: ReasonCreated,
				subjectVersion: created.Version}); err != nil {
				return err
			}
		}
		if in.WholesalePriceMinor != nil {
			if err = s.appendPriceVersion(ctx, tx, actor, currency, priceChange{
				subjectType: SubjectSellUnitWholesale, subjectRef: created.ID, productRef: product.ID,
				variantRef: variant.ID, amount: in.WholesalePriceMinor, reason: ReasonCreated,
				subjectVersion: created.Version}); err != nil {
				return err
			}
		}
		if _, err = bumpRevisionsForVariant(ctx, tx, actor.VendorID, variant.ID); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "shop.sell_unit.created", eventType: "shop.sell_unit.changed",
			aggregateType: "shop_sell_unit", aggregateID: created.ID, version: created.Version,
			after: SellUnitViewOf(created), payload: SellUnitViewOf(created),
		})
	})
	return created, err
}

type conversionResult struct {
	conversionType string
	baseFamily     string
	baseQuantity   *int64
	calibrationID  string
}

// resolveConversion decides, explicitly, what is known about this sell unit's
// relationship to a canonical base.
//
// EXACT is only possible when the measure itself has a universal base — a
// kilogram is a thousand grams everywhere — AND the fraction divides that base
// into whole units. Anything derived from a vendor's own container is
// CALIBRATED_APPROXIMATE and is labelled as such forever. A container with no
// calibration is honestly NONE rather than a guess.
func (s *Service) resolveConversion(ctx context.Context, q querier, businessID, productID, variantID string,
	measure MeasureDefinition, numerator, denominator int, calibrationID string) (conversionResult, error) {

	calibrationID = strings.TrimSpace(calibrationID)
	if calibrationID != "" {
		if measure.BaseQuantity != nil {
			return conversionResult{}, coded(apperrors.Validation("Check your details.", []apperrors.FieldViolation{
				{Field: "calibration_id", Message: "This measure already converts exactly and needs no calibration."}}),
				CodeConversionInvalid)
		}
		calibration, err := loadCalibration(ctx, q, businessID, productID, calibrationID, false)
		if err != nil {
			return conversionResult{}, err
		}
		if calibration.Status == StatusArchived {
			return conversionResult{}, errArchived("This calibration")
		}
		if calibration.BranchID != "" {
			return conversionResult{}, coded(apperrors.Validation("Check your details.", []apperrors.FieldViolation{
				{Field: "calibration_id", Message: "A branch calibration must be assigned on that branch's listing sell unit."}}),
				CodeConversionInvalid)
		}
		if calibration.MeasureID != measure.ID {
			return conversionResult{}, coded(apperrors.Validation("Check your details.", []apperrors.FieldViolation{
				{Field: "calibration_id", Message: "That calibration is for a different measure."}}),
				CodeConversionInvalid)
		}
		if calibration.VariantID != "" && calibration.VariantID != variantID {
			return conversionResult{}, coded(apperrors.Validation("Check your details.", []apperrors.FieldViolation{
				{Field: "calibration_id", Message: "That calibration is for a different variant."}}),
				CodeConversionInvalid)
		}
		return conversionResult{conversionType: ConversionCalibrated, calibrationID: calibration.ID}, nil
	}

	if measure.BaseQuantity == nil {
		// A Paint, a basket, a bag: real measures with no universal content.
		return conversionResult{conversionType: ConversionNone}, nil
	}
	total := *measure.BaseQuantity * int64(numerator)
	if total%int64(denominator) != 0 {
		return conversionResult{}, coded(apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "fraction_denominator",
				Message: "That fraction does not divide this measure into whole base units."}}),
			CodeConversionInvalid)
	}
	exact := total / int64(denominator)
	if exact <= 0 || exact > 100000000000 {
		return conversionResult{}, fieldError("fraction_numerator", "The resulting quantity is out of range.")
	}
	return conversionResult{conversionType: ConversionExact, baseFamily: measure.Family, baseQuantity: &exact}, nil
}

func normalizeFraction(numeratorIn, denominatorIn *int) (int, int, error) {
	numerator, denominator := 1, 1
	if numeratorIn != nil {
		numerator = *numeratorIn
	}
	if denominatorIn != nil {
		denominator = *denominatorIn
	}
	if numerator <= 0 || numerator > MaxFractionNumerator {
		return 0, 0, fieldError("fraction_numerator", "The numerator must be a positive whole number.")
	}
	if denominator <= 0 || denominator > MaxFractionDenominator {
		return 0, 0, fieldError("fraction_denominator", "The denominator must be a positive whole number.")
	}
	divisor := gcd(numerator, denominator)
	return numerator / divisor, denominator / divisor, nil
}

func normalizeTolerances(underfillIn, overfillIn *int) (int, int, error) {
	underfill, overfill := 0, 0
	if underfillIn != nil {
		underfill = *underfillIn
	}
	if overfillIn != nil {
		overfill = *overfillIn
	}
	if underfill < 0 || underfill > 10000 {
		return 0, 0, fieldError("underfill_tolerance_bps", "Tolerance must be between 0 and 10000 basis points.")
	}
	if overfill < 0 || overfill > 10000 {
		return 0, 0, fieldError("overfill_tolerance_bps", "Tolerance must be between 0 and 10000 basis points.")
	}
	return underfill, overfill, nil
}

func normalizeOrderRules(minimumIn, multipleIn, maximumIn *int64) (int64, int64, *int64, error) {
	minimum, multiple := int64(1), int64(1)
	if minimumIn != nil {
		minimum = *minimumIn
	}
	if multipleIn != nil {
		multiple = *multipleIn
	}
	if minimum < 1 || minimum > 1000000000 {
		return 0, 0, nil, fieldError("minimum_order_quantity", "The minimum order quantity must be at least one.")
	}
	if multiple < 1 || multiple > 1000000000 {
		return 0, 0, nil, fieldError("order_multiple", "The order multiple must be at least one.")
	}
	if maximumIn != nil {
		if *maximumIn < minimum || *maximumIn > 1000000000 {
			return 0, 0, nil, fieldError("maximum_order_quantity",
				"The maximum order quantity must be at least the minimum.")
		}
	}
	return minimum, multiple, maximumIn, nil
}

// defaultSellUnitName renders "Half Paint", "Quarter Basket" or "3/4 Bag"
// without ever creating a HALF_PAINT measure to do it.
func defaultSellUnitName(numerator, denominator int, measureName string) string {
	if denominator == 1 {
		if numerator == 1 {
			return measureName
		}
		return itoa(numerator) + " " + measureName
	}
	if numerator == 1 {
		switch denominator {
		case 2:
			return "Half " + measureName
		case 4:
			return "Quarter " + measureName
		}
	}
	return itoa(numerator) + "/" + itoa(denominator) + " " + measureName
}

// ListSellUnits reads a product's sell units.
func (s *Service) ListSellUnits(ctx context.Context, actor Actor, scope Scope, productID, variantID string, includeArchived bool) ([]SellUnit, error) {
	if scope.Empty() {
		return nil, errForbidden()
	}
	sql := `SELECT ` + sellUnitCols + ` FROM shop_sell_units WHERE business_id=$1 AND product_id=$2`
	args := []any{actor.VendorID, strings.TrimSpace(productID)}
	if variant := strings.TrimSpace(variantID); variant != "" {
		args = append(args, variant)
		sql += ` AND variant_id=$3`
	}
	if !includeArchived {
		sql += ` AND status <> 'ARCHIVED'`
	}
	sql += ` ORDER BY variant_id, position, id LIMIT 200`
	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	units := []SellUnit{}
	for rows.Next() {
		unit, scanErr := scanSellUnit(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		units = append(units, unit)
	}
	return units, rows.Err()
}

// UpdateSellUnitInput edits a sell unit.
type UpdateSellUnitInput struct {
	DisplayName         *string
	MeasurementMode     *string
	CalibrationID       **string
	EstimatedBaseQty    **int64
	UnderfillBps        *int
	OverfillBps         *int
	MeasurementRequired *bool
	DisplayPrecision    *int
	PriceMinor          **int64
	WholesalePriceMinor **int64
	RetailEnabled       *bool
	WholesaleEnabled    *bool
	MinimumOrderQty     *int64
	OrderMultiple       *int64
	MaximumOrderQty     **int64
	Status              *string
	ExpectedVersion     int64
}

// UpdateSellUnit edits a sell unit, appending price history for money changes.
//
// The measure and its fraction are deliberately immutable: changing what a
// unit MEANS would silently reinterpret every past order that named it. A
// different shape is a different sell unit.
func (s *Service) UpdateSellUnit(ctx context.Context, actor Actor, scope Scope, productID, sellUnitID string, in UpdateSellUnitInput) (SellUnit, error) {
	if err := s.requireWrites(); err != nil {
		return SellUnit{}, err
	}
	if err := requireBusinessWide(scope, "Sell units"); err != nil {
		return SellUnit{}, err
	}
	if in.PriceMinor != nil {
		if err := s.requirePricingAuthority(ctx, actor, ""); err != nil {
			return SellUnit{}, err
		}
		if err := validateMoney("price_minor", *in.PriceMinor); err != nil {
			return SellUnit{}, err
		}
	}
	if in.WholesalePriceMinor != nil {
		if err := s.requireWholesaleAuthority(ctx, actor, ""); err != nil {
			return SellUnit{}, err
		}
		if err := validateMoney("wholesale_price_minor", *in.WholesalePriceMinor); err != nil {
			return SellUnit{}, err
		}
	}
	if in.MinimumOrderQty != nil || in.OrderMultiple != nil || in.MaximumOrderQty != nil {
		if err := s.requireWholesaleAuthority(ctx, actor, ""); err != nil {
			return SellUnit{}, err
		}
	}

	var updated SellUnit
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		current, err := loadSellUnit(ctx, tx, actor.VendorID, sellUnitID, true)
		if err != nil {
			return err
		}
		if current.ProductID != strings.TrimSpace(productID) {
			return errNotFound("Sell unit", CodeSellUnitNotFound)
		}
		if err = expectedVersionMatches(current.Version, in.ExpectedVersion); err != nil {
			return err
		}
		if current.Status == StatusArchived {
			return errArchived("This sell unit")
		}

		displayName, mode := current.DisplayName, current.MeasurementMode
		conversionType, baseFamily := current.ConversionType, current.ExactBaseFamily
		baseQuantity, calibrationID := current.ExactBaseQuantity, current.CalibrationID
		estimated := current.EstimatedBaseQty
		underfill, overfill := current.UnderfillBps, current.OverfillBps
		measurementRequired, precision := current.MeasurementRequired, current.DisplayPrecision
		price, wholesale := current.PriceMinor, current.WholesalePriceMinor
		retailEnabled, wholesaleEnabled := current.RetailEnabled, current.WholesaleEnabled
		minimum, multiple, maximum := current.MinimumOrderQty, current.OrderMultiple, current.MaximumOrderQty
		status := current.Status

		if in.DisplayName != nil {
			value, violation := validateName("display_name", *in.DisplayName, 1, 120)
			if violation != nil {
				return validation(*violation)
			}
			displayName = value
		}
		measure, err := loadMeasure(ctx, tx, actor.VendorID, current.MeasureID, false)
		if err != nil {
			return err
		}
		if in.MeasurementMode != nil {
			value := strings.ToUpper(strings.TrimSpace(*in.MeasurementMode))
			if !contains(MeasurementModes, value) {
				return fieldError("measurement_mode", "Choose a supported measurement mode.")
			}
			if allowed, known := modeFamilies[value]; known && !contains(allowed, measure.Family) {
				return coded(apperrors.Validation("Check your details.", []apperrors.FieldViolation{
					{Field: "measurement_mode", Message: "A " + value + " sell unit cannot be measured in a " +
						measure.Family + " unit."}}), CodeConversionInvalid)
			}
			mode = value
		}
		if in.CalibrationID != nil {
			candidate := ""
			if *in.CalibrationID != nil {
				candidate = strings.TrimSpace(**in.CalibrationID)
			}
			conversion, convErr := s.resolveConversion(ctx, tx, actor.VendorID, current.ProductID,
				current.VariantID, measure, current.FractionNumerator, current.FractionDenominator, candidate)
			if convErr != nil {
				return convErr
			}
			conversionType, baseFamily, baseQuantity, calibrationID =
				conversion.conversionType, conversion.baseFamily, conversion.baseQuantity, conversion.calibrationID
		}
		if in.EstimatedBaseQty != nil {
			if *in.EstimatedBaseQty != nil && (**in.EstimatedBaseQty <= 0 || **in.EstimatedBaseQty > 100000000000) {
				return fieldError("estimated_base_quantity", "The estimate must be a positive whole number.")
			}
			estimated = *in.EstimatedBaseQty
		}
		if in.UnderfillBps != nil || in.OverfillBps != nil {
			under, over, tolErr := normalizeTolerances(
				intOrExisting(in.UnderfillBps, underfill), intOrExisting(in.OverfillBps, overfill))
			if tolErr != nil {
				return tolErr
			}
			underfill, overfill = under, over
		}
		if in.MeasurementRequired != nil {
			measurementRequired = *in.MeasurementRequired
		}
		if in.DisplayPrecision != nil {
			if *in.DisplayPrecision < 0 || *in.DisplayPrecision > 3 {
				return fieldError("display_precision", "Display precision must be between 0 and 3.")
			}
			precision = *in.DisplayPrecision
		}
		if in.PriceMinor != nil {
			price = *in.PriceMinor
		}
		if in.WholesalePriceMinor != nil {
			wholesale = *in.WholesalePriceMinor
		}
		if mode == ModePricePortion && price == nil {
			return coded(apperrors.Validation("Check your details.", []apperrors.FieldViolation{
				{Field: "price_minor", Message: "A price portion needs the price that defines it."}}),
				CodeConversionInvalid)
		}
		if in.RetailEnabled != nil {
			retailEnabled = *in.RetailEnabled
		}
		if in.WholesaleEnabled != nil {
			wholesaleEnabled = *in.WholesaleEnabled
		}
		if in.MinimumOrderQty != nil || in.OrderMultiple != nil || in.MaximumOrderQty != nil {
			minimumCandidate, multipleCandidate := minimum, multiple
			maximumCandidate := maximum
			if in.MinimumOrderQty != nil {
				minimumCandidate = *in.MinimumOrderQty
			}
			if in.OrderMultiple != nil {
				multipleCandidate = *in.OrderMultiple
			}
			if in.MaximumOrderQty != nil {
				maximumCandidate = *in.MaximumOrderQty
			}
			normalizedMin, normalizedMultiple, normalizedMax, ruleErr := normalizeOrderRules(
				&minimumCandidate, &multipleCandidate, maximumCandidate)
			if ruleErr != nil {
				return ruleErr
			}
			minimum, multiple, maximum = normalizedMin, normalizedMultiple, normalizedMax
		}
		if in.Status != nil {
			value := strings.ToUpper(strings.TrimSpace(*in.Status))
			if value != StatusActive && value != StatusPaused {
				return fieldError("status", "Use ACTIVE or PAUSED. Archive with the archive command.")
			}
			status = value
		}

		row, err := scanSellUnit(tx.QueryRow(ctx, `
			UPDATE shop_sell_units
			SET display_name=$3, measurement_mode=$4, conversion_type=$5, exact_base_family=NULLIF($6,''),
			    exact_base_quantity=$7, calibration_id=NULLIF($8,''), estimated_base_quantity=$9,
			    underfill_tolerance_bps=$10, overfill_tolerance_bps=$11, actual_measurement_required=$12,
			    display_precision=$13, price_minor=$14, wholesale_price_minor=$15, retail_enabled=$16,
			    wholesale_enabled=$17, minimum_order_quantity=$18, order_multiple=$19,
			    maximum_order_quantity=$20, status=$21, updated_at=now(), version=version+1
			WHERE business_id=$1 AND id=$2 RETURNING `+sellUnitCols,
			actor.VendorID, current.ID, displayName, mode, conversionType, baseFamily, baseQuantity,
			calibrationID, estimated, underfill, overfill, measurementRequired, precision,
			price, wholesale, retailEnabled, wholesaleEnabled, minimum, multiple, maximum, status))
		if err != nil {
			return mapDB(err, "Sell unit", CodeSellUnitNotFound)
		}
		updated = row

		currency, err := s.businessCurrency(ctx, tx, actor.VendorID)
		if err != nil {
			return err
		}
		if moneyChanged(current.PriceMinor, updated.PriceMinor) {
			if err = s.appendPriceVersion(ctx, tx, actor, currency, priceChange{
				subjectType: SubjectSellUnitRetail, subjectRef: updated.ID, productRef: updated.ProductID,
				variantRef: updated.VariantID, previousAmount: current.PriceMinor, amount: updated.PriceMinor,
				subjectVersion: updated.Version,
				reason:         priceReason(current.PriceMinor, updated.PriceMinor)}); err != nil {
				return err
			}
		}
		if moneyChanged(current.WholesalePriceMinor, updated.WholesalePriceMinor) {
			if err = s.appendPriceVersion(ctx, tx, actor, currency, priceChange{
				subjectType: SubjectSellUnitWholesale, subjectRef: updated.ID, productRef: updated.ProductID,
				variantRef: updated.VariantID, previousAmount: current.WholesalePriceMinor,
				amount: updated.WholesalePriceMinor, subjectVersion: updated.Version,
				reason: priceReason(current.WholesalePriceMinor, updated.WholesalePriceMinor)}); err != nil {
				return err
			}
		}
		if _, err = bumpRevisionsForVariant(ctx, tx, actor.VendorID, updated.VariantID); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "shop.sell_unit.updated", eventType: "shop.sell_unit.changed",
			aggregateType: "shop_sell_unit", aggregateID: updated.ID, version: updated.Version,
			before: SellUnitViewOf(current), after: SellUnitViewOf(updated), payload: SellUnitViewOf(updated),
		})
	})
	return updated, err
}

func intOrExisting(candidate *int, existing int) *int {
	if candidate != nil {
		return candidate
	}
	value := existing
	return &value
}

// ArchiveSellUnit retires a sell unit and disables it on every listing.
func (s *Service) ArchiveSellUnit(ctx context.Context, actor Actor, scope Scope, productID, sellUnitID string, expectedVersion int64) (SellUnit, error) {
	if err := s.requireWrites(); err != nil {
		return SellUnit{}, err
	}
	if err := requireBusinessWide(scope, "Sell units"); err != nil {
		return SellUnit{}, err
	}
	var archived SellUnit
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		current, err := loadSellUnit(ctx, tx, actor.VendorID, sellUnitID, true)
		if err != nil {
			return err
		}
		if current.ProductID != strings.TrimSpace(productID) {
			return errNotFound("Sell unit", CodeSellUnitNotFound)
		}
		if err = expectedVersionMatches(current.Version, expectedVersion); err != nil {
			return err
		}
		if current.Status == StatusArchived {
			archived = current
			return nil
		}
		row, err := scanSellUnit(tx.QueryRow(ctx, `
			UPDATE shop_sell_units SET status='ARCHIVED', archived_at=now(), updated_at=now(), version=version+1
			WHERE business_id=$1 AND id=$2 RETURNING `+sellUnitCols, actor.VendorID, current.ID))
		if err != nil {
			return mapDB(err, "Sell unit", CodeSellUnitNotFound)
		}
		archived = row
		if _, err = tx.Exec(ctx, `
			UPDATE shop_listing_sell_units SET enabled=false, updated_at=now(), version=version+1
			WHERE business_id=$1 AND sell_unit_id=$2 AND enabled`, actor.VendorID, current.ID); err != nil {
			return err
		}
		// A retired sell unit also retires the tiers priced against it: a band
		// pointing at a unit nobody can buy is a price with no meaning.
		if _, err = tx.Exec(ctx, `
			UPDATE shop_quantity_breaks SET status='ARCHIVED', archived_at=now(), updated_at=now(), version=version+1
			WHERE business_id=$1 AND sell_unit_id=$2 AND status='ACTIVE'`, actor.VendorID, current.ID); err != nil {
			return err
		}
		if _, err = bumpRevisionsForVariant(ctx, tx, actor.VendorID, current.VariantID); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "shop.sell_unit.archived", eventType: "shop.sell_unit.changed",
			aggregateType: "shop_sell_unit", aggregateID: archived.ID, version: archived.Version,
			before: SellUnitViewOf(current), after: SellUnitViewOf(archived), payload: SellUnitViewOf(archived),
		})
	})
	return archived, err
}
