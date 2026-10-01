package main

// Section 17C adapters.
//
// Each one turns an existing authority into the narrow port the fulfilment
// feature declares. They live in the composition root so fulfilment never
// imports Food, Shop or the vendors feature, and none of those learns
// anything about fulfilment planning.
//
// Every read here is scoped by business_id. A line may never resolve against
// another vendor's catalogue, and a branch may never resolve against another
// vendor's branches — that is the structural half of "Vendor A must not
// inject another branch pickup".

import (
	"context"
	"errors"
	"math"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilment"
)

// -----------------------------------------------------------------------------
// Logistics metadata
// -----------------------------------------------------------------------------

// section17CLogisticsAuthority resolves the real physical profile of a
// checkout line from the catalogue authority that owns it.
type section17CLogisticsAuthority struct {
	db *pgxpool.Pool
}

func newSection17CLogisticsAuthority(
	db *pgxpool.Pool,
) *section17CLogisticsAuthority {
	return &section17CLogisticsAuthority{db: db}
}

// ResolveLineProfiles implements fulfilment.LogisticsAuthority.
func (a *section17CLogisticsAuthority) ResolveLineProfiles(
	ctx context.Context,
	businessID string,
	lines []fulfilment.LineLogistics,
) (map[string]fulfilment.LineProfile, error) {
	profiles := make(map[string]fulfilment.LineProfile, len(lines))

	if a == nil || a.db == nil {
		return profiles, nil
	}

	businessID = strings.TrimSpace(businessID)
	if businessID == "" {
		return profiles, nil
	}

	for _, line := range lines {
		var (
			profile fulfilment.LineProfile
			err     error
		)

		switch strings.ToUpper(strings.TrimSpace(line.Domain)) {
		case "FOOD":
			profile, err = a.resolveFood(ctx, businessID, line)
		case "SHOP":
			profile, err = a.resolveShop(ctx, businessID, line)
		default:
			continue
		}

		if err != nil {
			return nil, err
		}

		// An unknown profile is stored rather than skipped: the aggregation
		// counts it as a line without evidence, which is what makes the
		// group PARTIAL instead of silently light.
		profiles[line.CheckoutLineID] = profile
	}

	return profiles, nil
}

// resolveFood reads food_item_logistics_profiles.
//
// Food states handling metadata directly and has no sell-unit conversion, so
// the answer is a straight row read.
func (a *section17CLogisticsAuthority) resolveFood(
	ctx context.Context,
	businessID string,
	line fulfilment.LineLogistics,
) (fulfilment.LineProfile, error) {
	itemRef := strings.TrimSpace(line.ItemRef)
	if itemRef == "" {
		return fulfilment.LineProfile{}, nil
	}

	var (
		profile     fulfilment.LineProfile
		weightGrams *int32
		hints       []string
	)

	err := a.db.QueryRow(ctx, `
		SELECT temperature_class, package_size_class, spill_risk, fragile,
		       approx_weight_grams, vehicle_hints
		  FROM food_item_logistics_profiles
		 WHERE item_id = $1 AND business_id = $2`,
		itemRef, businessID,
	).Scan(
		&profile.TemperatureClass,
		&profile.PackageSizeClass,
		&profile.SpillRisk,
		&profile.Fragile,
		&weightGrams,
		&hints,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		// The vendor has stated nothing about this item. Reported as
		// unknown, never defaulted to "small and ambient".
		return fulfilment.LineProfile{}, nil
	}
	if err != nil {
		return fulfilment.LineProfile{}, err
	}

	profile.Known = true
	profile.VehicleHints = hints

	if weightGrams != nil && *weightGrams > 0 {
		grams := int64(*weightGrams)
		profile.WeightGrams = &grams
	}

	return profile, nil
}

// resolveShop reads shop_variants, its handling flags, and — critically — the
// sell unit the customer actually ordered in.
//
// The sell-unit step is not optional. A variant may be a 50 kg sack while the
// line is "2 kg of rice"; using the variant's own net weight there would
// overstate the load by twenty-five times and put a small order in a truck.
// Section 12's conversion vocabulary is respected exactly as it is declared:
//
//   - EXACT carries the sell unit's own quantity in the canonical base of its
//     family (grams for MASS, millilitres for VOLUME, whole units for COUNT),
//     per migration 000042's fixed-base convention. It is authoritative.
//   - CALIBRATED_APPROXIMATE is explicitly approximate. Section 12 forbids
//     any caller treating it as exact, so it is used as fallback evidence
//     only — it contributes a weight but never claims a measured one.
//   - NONE admits no conversion is known, and nothing is invented for it.
func (a *section17CLogisticsAuthority) resolveShop(
	ctx context.Context,
	businessID string,
	line fulfilment.LineLogistics,
) (fulfilment.LineProfile, error) {
	variantID, err := a.resolveShopVariant(ctx, businessID, line)
	if err != nil || variantID == "" {
		return fulfilment.LineProfile{}, err
	}

	var (
		profile         fulfilment.LineProfile
		netWeightGrams  *int64
		netVolumeMl     *int64
		temperatureZone string
		consolidation   *string
		separationGroup *string
		stackable       bool
		largeLoad       bool
		lengthMm        *int32
		widthMm         *int32
		heightMm        *int32
	)

	err = a.db.QueryRow(ctx, `
		SELECT net_weight_grams, net_volume_ml, length_mm, width_mm, height_mm,
		       temperature_zone, consolidation_class, separation_group,
		       stackable, large_load
		  FROM shop_variants
		 WHERE id = $1 AND business_id = $2`,
		variantID, businessID,
	).Scan(
		&netWeightGrams, &netVolumeMl, &lengthMm, &widthMm, &heightMm,
		&temperatureZone, &consolidation, &separationGroup,
		&stackable, &largeLoad,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return fulfilment.LineProfile{}, nil
	}
	if err != nil {
		return fulfilment.LineProfile{}, err
	}

	profile.Known = true
	profile.TemperatureClass = temperatureZone
	profile.LargeLoad = largeLoad
	profile.WeightGrams = netWeightGrams
	profile.VolumeMl = netVolumeMl
	profile.LengthMm = int32PtrToInt(lengthMm)
	profile.WidthMm = int32PtrToInt(widthMm)
	profile.HeightMm = int32PtrToInt(heightMm)

	if consolidation != nil {
		profile.ConsolidationClass = *consolidation
	}

	if separationGroup != nil {
		profile.SeparationGroup = *separationGroup
	}

	// A variant that may not be stacked needs the same care a fragile one
	// does, so it raises the same carrier capability.
	if !stackable {
		profile.StackRestricted = true
	}

	if err = a.applyShopSellUnit(
		ctx, businessID, line, variantID, &profile,
	); err != nil {
		return fulfilment.LineProfile{}, err
	}

	if err = a.applyShopHandlingFlags(
		ctx, businessID, variantID, &profile,
	); err != nil {
		return fulfilment.LineProfile{}, err
	}

	return profile, nil
}

// resolveShopVariant finds the variant a line refers to.
//
// The listing is preferred because it is branch-scoped evidence; the bare
// variant reference is the fallback for a line that carries one. Both are
// scoped by business_id, so neither can reach another tenant's catalogue.
func (a *section17CLogisticsAuthority) resolveShopVariant(
	ctx context.Context,
	businessID string,
	line fulfilment.LineLogistics,
) (string, error) {
	if listingRef := strings.TrimSpace(line.ListingRef); listingRef != "" {
		var variantID string

		err := a.db.QueryRow(ctx, `
			SELECT variant_id FROM shop_branch_listings
			 WHERE id = $1 AND business_id = $2`,
			listingRef, businessID,
		).Scan(&variantID)

		if err == nil {
			return variantID, nil
		}

		if !errors.Is(err, pgx.ErrNoRows) {
			return "", err
		}
	}

	if variantRef := strings.TrimSpace(line.VariantRef); variantRef != "" {
		var variantID string

		err := a.db.QueryRow(ctx, `
			SELECT id FROM shop_variants
			 WHERE id = $1 AND business_id = $2`,
			variantRef, businessID,
		).Scan(&variantID)

		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}

		return variantID, err
	}

	return "", nil
}

// applyShopSellUnit replaces the variant's own physical quantity with the
// quantity the ordered sell unit actually represents.
func (a *section17CLogisticsAuthority) applyShopSellUnit(
	ctx context.Context,
	businessID string,
	line fulfilment.LineLogistics,
	variantID string,
	profile *fulfilment.LineProfile,
) error {
	sellUnitRef := strings.TrimSpace(line.SellUnitRef)
	if sellUnitRef == "" {
		return nil
	}

	var (
		conversionType string
		baseFamily     *string
		baseQuantity   *int64
		estimated      *int64
		overfillBPS    int64
		calibratedGram *int64
		toleranceBPS   *int64
	)

	err := a.db.QueryRow(ctx, `
		SELECT su.conversion_type, su.exact_base_family, su.exact_base_quantity,
		       su.estimated_base_quantity, COALESCE(su.overfill_tolerance_bps,0),
		       c.approximate_weight_grams, c.tolerance_bps
		  FROM shop_sell_units su
		  LEFT JOIN shop_measure_calibrations c
		    ON c.id = su.calibration_id AND c.business_id = su.business_id AND c.status = 'ACTIVE'
		 WHERE su.id = $1 AND su.business_id = $2 AND su.variant_id = $3`,
		sellUnitRef, businessID, variantID,
	).Scan(&conversionType, &baseFamily, &baseQuantity, &estimated, &overfillBPS, &calibratedGram, &toleranceBPS)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}

	variantWeight := profile.WeightGrams
	variantVolume := profile.VolumeMl

	switch strings.ToUpper(strings.TrimSpace(conversionType)) {
	case "EXACT":
		if baseFamily == nil || baseQuantity == nil || *baseQuantity <= 0 {
			return nil
		}

		switch strings.ToUpper(strings.TrimSpace(*baseFamily)) {
		case "MASS":
			grams := *baseQuantity
			if line.MeasureQuantity > 0 {
				if line.Quantity <= 0 || grams > math.MaxInt64/line.MeasureQuantity {
					return errors.New("measured logistics weight overflow")
				}
				total := grams * line.MeasureQuantity
				grams = total / line.Quantity
				if total%line.Quantity != 0 {
					grams++
				}
			}
			profile.WeightGrams = &grams

			// The variant's own volume describes a whole variant unit, not
			// this portion of it, so it is dropped rather than reused.
			profile.VolumeMl = nil

		case "VOLUME":
			millilitres := *baseQuantity
			profile.VolumeMl = &millilitres
			profile.WeightGrams = nil

		case "COUNT":
			// A count sell unit is N whole variant units, so both of the
			// variant's own measurements scale by N.
			profile.WeightGrams = scaleOptional(variantWeight, *baseQuantity)
			profile.VolumeMl = scaleOptional(variantVolume, *baseQuantity)
		}

	case "CALIBRATED_APPROXIMATE":
		// Section 12 forbids treating a calibration as exact. It is carried
		// as the Food size-class fallback is carried: it contributes to the
		// total but must not be reported as a measured value, so the
		// authoritative measurements are cleared and the approximate figure
		// travels as a size class instead.
		profile.WeightGrams = nil
		profile.VolumeMl = nil

		// Variable-measurement goods still need a safe PRE-PICK planning
		// weight (Karryam V4 §14.6). The vendor's own calibration states an
		// approximate weight and a tolerance; planning uses the tolerance-
		// inflated UPPER bound, so an approximation can only ever size the
		// vehicle up, never down. The measured weight later drives post-pick
		// re-evaluation. Without a calibrated weight nothing is invented.
		if calibratedGram != nil && *calibratedGram > 0 {
			bps := overfillBPS
			if toleranceBPS != nil && *toleranceBPS > bps {
				bps = *toleranceBPS
			}
			upper := (*calibratedGram*(10_000+bps) + 9_999) / 10_000
			profile.WeightGrams = &upper
		} else if estimated != nil && *estimated > 0 {
			profile.PackageSizeClass = approximateSizeClass(*estimated)
		}

	case "NONE":
		// No conversion is known. Nothing is invented; the variant's own
		// figures are the only honest evidence available and they stay.
	}

	return nil
}

// approximateSizeClass buckets an approximate base quantity into the same
// SMALL/MEDIUM/LARGE vocabulary the Food fallback already uses, so both
// approximations flow through one clearly-marked fallback path instead of two.
func approximateSizeClass(baseQuantity int64) string {
	switch {
	case baseQuantity <= 2000:
		return "SMALL"
	case baseQuantity <= 8000:
		return "MEDIUM"
	default:
		return "LARGE"
	}
}

func (a *section17CLogisticsAuthority) applyShopHandlingFlags(
	ctx context.Context,
	businessID, variantID string,
	profile *fulfilment.LineProfile,
) error {
	rows, err := a.db.Query(ctx, `
		SELECT handling_flag FROM shop_variant_handling_flags
		 WHERE variant_id = $1 AND business_id = $2`,
		variantID, businessID,
	)
	if err != nil {
		return err
	}

	defer rows.Close()

	for rows.Next() {
		var flag string

		if err = rows.Scan(&flag); err != nil {
			return err
		}

		switch strings.ToUpper(strings.TrimSpace(flag)) {
		case "FRAGILE", "CRUSH_SENSITIVE":
			profile.Fragile = true

		case "STACK_RESTRICTED":
			profile.StackRestricted = true

		case "UPRIGHT_REQUIRED":
			profile.UprightRequired = true

		case "LEAK_RISK", "LIQUID":
			profile.SpillRisk = true

		case "KEEP_COLD":
			// A handling flag may only ever RAISE the requirement. A variant
			// already declared FROZEN must not be demoted to chilled by a
			// KEEP_COLD flag.
			if strings.EqualFold(profile.TemperatureClass, "AMBIENT") ||
				strings.EqualFold(profile.TemperatureClass, "FRESH") {
				profile.TemperatureClass = "CHILLED"
			}
		}
	}

	return rows.Err()
}

func scaleOptional(value *int64, factor int64) *int64 {
	if value == nil || factor <= 0 {
		return nil
	}

	scaled := *value * factor

	return &scaled
}

func int32PtrToInt(value *int32) *int {
	if value == nil {
		return nil
	}

	converted := int(*value)

	return &converted
}

// -----------------------------------------------------------------------------
// Branch pickup and reach
// -----------------------------------------------------------------------------

// section17CBranchAuthority resolves the pickup point and reach of one branch.
type section17CBranchAuthority struct {
	db *pgxpool.Pool
}

func newSection17CBranchAuthority(
	db *pgxpool.Pool,
) *section17CBranchAuthority {
	return &section17CBranchAuthority{db: db}
}

// ResolveBranch implements fulfilment.BranchAuthority.
//
// The composite (branch_id, business_id) predicate is the tenant guard: a
// branch id borrowed from another business resolves to nothing, so it cannot
// be used as a pickup point.
func (a *section17CBranchAuthority) ResolveBranch(
	ctx context.Context,
	businessID, branchID string,
) (fulfilment.Branch, bool, error) {
	if a == nil || a.db == nil {
		return fulfilment.Branch{}, false, nil
	}

	businessID = strings.TrimSpace(businessID)
	branchID = strings.TrimSpace(branchID)

	if businessID == "" || branchID == "" {
		return fulfilment.Branch{}, false, nil
	}

	branch := fulfilment.Branch{
		BusinessID: businessID,
		BranchID:   branchID,
	}

	var (
		areaLatitude  *float64
		areaLongitude *float64
		areaRadius    *int32
		accepting     *bool
	)

	err := a.db.QueryRow(ctx, `
		SELECT b.status, b.latitude, b.longitude,
		       a.center_latitude, a.center_longitude, a.radius_meters,
		       s.accepting_new_orders
		  FROM vendor_branches b
		  LEFT JOIN vendor_branch_service_areas a
		         ON a.branch_id = b.id AND a.business_id = b.business_id
		  LEFT JOIN vendor_branch_settings s
		         ON s.branch_id = b.id AND s.business_id = b.business_id
		 WHERE b.id = $1 AND b.business_id = $2`,
		branchID, businessID,
	).Scan(
		&branch.Status,
		&branch.Latitude,
		&branch.Longitude,
		&areaLatitude,
		&areaLongitude,
		&areaRadius,
		&accepting,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return fulfilment.Branch{}, false, nil
	}
	if err != nil {
		return fulfilment.Branch{}, false, err
	}

	if areaLatitude != nil && areaLongitude != nil &&
		areaRadius != nil && *areaRadius > 0 {
		branch.HasServiceArea = true
		branch.AreaCenterLatitude = *areaLatitude
		branch.AreaCenterLongitude = *areaLongitude
		branch.AreaRadiusMeters = int(*areaRadius)
	}

	if accepting != nil {
		branch.HasOperationalSetting = true
		branch.AcceptingNewOrders = *accepting
	}

	return branch, true, nil
}
