package shop

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

// ---------------------------------------------------------------------------
// Branch listings
// ---------------------------------------------------------------------------
//
// A Shop product exists for the business; it SELLS for a branch. The listing
// is that branch's commercial decision, and it carries no stock: availability
// is Part B's authority and a convenient quantity column here would be wrong
// the moment anything sold.

const listingCols = `id,business_id,branch_id,product_id,variant_id,COALESCE(operating_model,''),
	retail_enabled,wholesale_enabled,retail_price_override_minor,wholesale_price_override_minor,
	minimum_order_quantity,order_multiple,maximum_order_quantity,preorder_policy,backorder_policy,
	auto_hide_when_unavailable,logistics_available,status,created_at,updated_at,archived_at,version`

func scanListing(row rowScanner) (BranchListing, error) {
	var l BranchListing
	err := row.Scan(&l.ID, &l.BusinessID, &l.BranchID, &l.ProductID, &l.VariantID, &l.OperatingModel,
		&l.RetailEnabled, &l.WholesaleEnabled, &l.RetailOverrideMinor, &l.WholesaleOverrideMin,
		&l.MinimumOrderQty, &l.OrderMultiple, &l.MaximumOrderQty, &l.PreorderPolicy, &l.BackorderPolicy,
		&l.AutoHideUnavailable, &l.LogisticsAvailable, &l.Status, &l.CreatedAt, &l.UpdatedAt,
		&l.ArchivedAt, &l.Version)
	return l, err
}

func loadListing(ctx context.Context, q querier, businessID, listingID string, lock bool) (BranchListing, error) {
	sql := `SELECT ` + listingCols + ` FROM shop_branch_listings WHERE business_id=$1 AND id=$2`
	if lock {
		sql += ` FOR UPDATE`
	}
	listing, err := scanListing(q.QueryRow(ctx, sql, businessID, strings.TrimSpace(listingID)))
	if err != nil {
		return BranchListing{}, mapDB(err, "Listing", CodeListingNotFound)
	}
	return listing, nil
}

func (s *Service) requireListingAccess(ctx context.Context, q querier, actor Actor, scope Scope, listing BranchListing) error {
	if !scope.Covers(listing.BranchID) {
		return errForbidden()
	}
	_, err := s.requireShopBranch(ctx, q, actor.VendorID, listing.BranchID)
	return err
}

const listingSellUnitCols = `id,business_id,listing_id,sell_unit_id,variant_id,
	COALESCE(calibration_id,''),enabled,price_override_minor,wholesale_price_override_minor,
	position,created_at,updated_at,version`

func scanListingSellUnit(row rowScanner) (ListingSellUnit, error) {
	var unit ListingSellUnit
	err := row.Scan(&unit.ID, &unit.BusinessID, &unit.ListingID, &unit.SellUnitID, &unit.VariantID,
		&unit.CalibrationID, &unit.Enabled, &unit.PriceOverrideMinor, &unit.WholesaleOverrideMin,
		&unit.Position, &unit.CreatedAt, &unit.UpdatedAt, &unit.Version)
	return unit, err
}

// CreateListingInput is the listing-create command.
type CreateListingInput struct {
	VariantID           string
	OperatingModel      string
	RetailEnabled       *bool
	WholesaleEnabled    *bool
	RetailOverride      *int64
	WholesaleOverride   *int64
	MinimumOrderQty     *int64
	OrderMultiple       *int64
	MaximumOrderQty     *int64
	PreorderPolicy      string
	BackorderPolicy     string
	AutoHideUnavailable *bool
	LogisticsAvailable  *bool
	SellUnitIDs         []string
}

// CreateListing lists one variant at one branch.
func (s *Service) CreateListing(ctx context.Context, actor Actor, scope Scope, branchID string, in CreateListingInput) (BranchListing, error) {
	if err := s.requireWrites(); err != nil {
		return BranchListing{}, err
	}
	if err := s.requireBranchScope(scope, branchID); err != nil {
		return BranchListing{}, err
	}
	branch := strings.TrimSpace(branchID)
	retailEnabled := boolOrDefault(in.RetailEnabled, true)
	wholesaleEnabled := boolOrDefault(in.WholesaleEnabled, false)
	if !retailEnabled && !wholesaleEnabled {
		return BranchListing{}, fieldError("retail_enabled", "A listing must be enabled for retail, wholesale or both.")
	}
	if err := validateMoney("retail_price_override_minor", in.RetailOverride); err != nil {
		return BranchListing{}, err
	}
	if err := validateMoney("wholesale_price_override_minor", in.WholesaleOverride); err != nil {
		return BranchListing{}, err
	}
	minimum, multiple, maximum, err := normalizeOrderRules(in.MinimumOrderQty, in.OrderMultiple, in.MaximumOrderQty)
	if err != nil {
		return BranchListing{}, err
	}
	preorder, err := normalizePolicy("preorder_policy", in.PreorderPolicy)
	if err != nil {
		return BranchListing{}, err
	}
	backorder, err := normalizePolicy("backorder_policy", in.BackorderPolicy)
	if err != nil {
		return BranchListing{}, err
	}
	if in.RetailOverride != nil {
		if err = s.requirePricingAuthority(ctx, actor, branch); err != nil {
			return BranchListing{}, err
		}
	}
	if in.WholesaleOverride != nil || wholesaleEnabled || minimum > 1 || multiple > 1 || maximum != nil {
		if err = s.requireWholesaleAuthority(ctx, actor, branch); err != nil {
			return BranchListing{}, err
		}
	}

	var created BranchListing
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		capabilities, err := s.requireShopBranch(ctx, tx, actor.VendorID, branch)
		if err != nil {
			return err
		}
		operatingModel := strings.ToUpper(strings.TrimSpace(in.OperatingModel))
		if operatingModel != "" {
			if !IsOperatingModel(operatingModel) {
				return coded(apperrors.Validation("Check your details.", []apperrors.FieldViolation{
					{Field: "operating_model", Message: "Choose a supported operating model."}}),
					CodeOperatingModelInvalid)
			}
			if _, err = admitOperatingModel(operatingModel, capabilities); err != nil {
				return err
			}
			enabled, modelErr := branchModelEnabled(ctx, tx, actor.VendorID, branch, operatingModel)
			if modelErr != nil {
				return modelErr
			}
			if !enabled {
				return coded(apperrors.Conflict("This branch has not enabled the "+operatingModel+" operating model.", nil),
					CodeOperatingModelInvalid)
			}
		}
		variant, err := loadVariantByID(ctx, tx, actor.VendorID, in.VariantID, false)
		if err != nil {
			return err
		}
		if variant.Status == StatusArchived {
			return errArchived("This variant")
		}
		product, err := loadProduct(ctx, tx, actor.VendorID, variant.ProductID, false)
		if err != nil {
			return err
		}
		if product.Status == StatusArchived {
			return errArchived("This product")
		}
		if err = validateRestrictedGoodsProduct("product", product); err != nil {
			return err
		}
		row, err := scanListing(tx.QueryRow(ctx, `
			INSERT INTO shop_branch_listings
				(id,business_id,branch_id,product_id,variant_id,operating_model,retail_enabled,
				 wholesale_enabled,retail_price_override_minor,wholesale_price_override_minor,
				 minimum_order_quantity,order_multiple,maximum_order_quantity,preorder_policy,
				 backorder_policy,auto_hide_when_unavailable,logistics_available,status)
			VALUES ($1,$2,$3,$4,$5,NULLIF($6,''),$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,'ACTIVE')
			RETURNING `+listingCols,
			newID("sbl_"), actor.VendorID, branch, product.ID, variant.ID, operatingModel,
			retailEnabled, wholesaleEnabled, in.RetailOverride, in.WholesaleOverride,
			minimum, multiple, maximum, preorder, backorder,
			boolOrDefault(in.AutoHideUnavailable, true), boolOrDefault(in.LogisticsAvailable, true)))
		if err != nil {
			return mapDB(err, "Listing", CodeListingNotFound)
		}
		created = row

		if err = replaceListingSellUnits(ctx, tx, actor.VendorID, created, in.SellUnitIDs); err != nil {
			return err
		}
		currency, err := s.businessCurrency(ctx, tx, actor.VendorID)
		if err != nil {
			return err
		}
		if in.RetailOverride != nil {
			if err = s.appendPriceVersion(ctx, tx, actor, currency, priceChange{
				subjectType: SubjectListingRetail, subjectRef: created.ID, branchID: branch,
				productRef: product.ID, variantRef: variant.ID, amount: in.RetailOverride,
				reason: ReasonOverrideSet, subjectVersion: created.Version}); err != nil {
				return err
			}
		}
		if in.WholesaleOverride != nil {
			if err = s.appendPriceVersion(ctx, tx, actor, currency, priceChange{
				subjectType: SubjectListingWholesale, subjectRef: created.ID, branchID: branch,
				productRef: product.ID, variantRef: variant.ID, amount: in.WholesaleOverride,
				reason: ReasonOverrideSet, subjectVersion: created.Version}); err != nil {
				return err
			}
		}
		if _, err = bumpBranchRevision(ctx, tx, actor.VendorID, branch); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "shop.listing.created", eventType: "shop.listing.changed",
			aggregateType: "shop_branch_listing", aggregateID: created.ID, branchID: branch,
			version: created.Version, after: ListingViewOf(created), payload: ListingViewOf(created),
		})
	})
	return created, err
}

func normalizePolicy(field, raw string) (string, error) {
	value := strings.ToUpper(strings.TrimSpace(raw))
	if value == "" {
		return "NOT_ALLOWED", nil
	}
	if value != "NOT_ALLOWED" && value != "ALLOWED" {
		return "", fieldError(field, "Use ALLOWED or NOT_ALLOWED.")
	}
	return value, nil
}

func branchModelEnabled(ctx context.Context, q querier, businessID, branchID, model string) (bool, error) {
	var enabled bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM shop_branch_operating_model_modes
		               WHERE business_id=$1 AND branch_id=$2 AND operating_model=$3)`,
		businessID, branchID, model).Scan(&enabled)
	return enabled, err
}

// replaceListingSellUnits sets which of the variant's sell units this branch
// offers. Each must belong to the very variant being listed, which the schema
// enforces through a composite foreign key rather than trusting this code.
func replaceListingSellUnits(ctx context.Context, tx pgx.Tx, businessID string, listing BranchListing, sellUnitIDs []string) error {
	if sellUnitIDs == nil {
		return nil
	}
	if len(sellUnitIDs) > MaxSellUnitsPerVariant {
		return fieldError("sell_unit_ids", "Enable at most "+itoa(MaxSellUnitsPerVariant)+" sell units.")
	}
	// Disable first, then re-enable/upsert the submitted set. This preserves
	// branch price and calibration history when a unit is temporarily removed;
	// delete-and-reinsert would silently discard both.
	if _, err := tx.Exec(ctx, `UPDATE shop_listing_sell_units
		SET enabled=false, updated_at=now(), version=version+1
		WHERE listing_id=$1 AND enabled`, listing.ID); err != nil {
		return err
	}
	seen := map[string]bool{}
	position := 0
	for _, raw := range sellUnitIDs {
		id := strings.TrimSpace(raw)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		var unitStatus string
		var unitRetail, unitWholesale bool
		if err := tx.QueryRow(ctx, `SELECT status,retail_enabled,wholesale_enabled
			FROM shop_sell_units WHERE business_id=$1 AND id=$2 AND variant_id=$3`,
			businessID, id, listing.VariantID).Scan(&unitStatus, &unitRetail, &unitWholesale); err != nil {
			return mapDB(err, "Listing sell unit", CodeSellUnitNotFound)
		}
		if unitStatus != StatusActive {
			return coded(apperrors.Conflict("Only an active sell unit can be enabled on a branch listing.", nil),
				CodeSellUnitNotFound)
		}
		if !(listing.RetailEnabled && unitRetail) && !(listing.WholesaleEnabled && unitWholesale) {
			return coded(apperrors.Conflict("This sell unit is not enabled for either sales channel on the listing.", nil),
				CodeSellUnitNotFound)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO shop_listing_sell_units
				(id,business_id,listing_id,sell_unit_id,variant_id,enabled,position)
			VALUES ($1,$2,$3,$4,$5,true,$6)
			ON CONFLICT (listing_id,sell_unit_id) DO UPDATE
			SET enabled=true, position=EXCLUDED.position, updated_at=now(),
			    version=shop_listing_sell_units.version+1`,
			newID("slu_"), businessID, listing.ID, id, listing.VariantID, position); err != nil {
			return mapDB(err, "Listing sell unit", CodeSellUnitNotFound)
		}
		position++
	}
	return nil
}

// ListingListOptions bounds a listing read.
type ListingListOptions struct {
	ProductID       string
	VariantID       string
	Status          string
	IncludeArchived bool
	Limit, Offset   int
}

// ListListings reads one branch's listings.
func (s *Service) ListListings(ctx context.Context, actor Actor, scope Scope, branchID string, in ListingListOptions) ([]BranchListing, bool, error) {
	if err := s.requireBranchScope(scope, branchID); err != nil {
		return nil, false, err
	}
	if _, err := s.requireShopBranch(ctx, s.db, actor.VendorID, branchID); err != nil {
		return nil, false, err
	}
	limit, offset := boundedPage(in.Limit, in.Offset)
	sql := `SELECT ` + listingCols + ` FROM shop_branch_listings WHERE business_id=$1 AND branch_id=$2`
	args := []any{actor.VendorID, strings.TrimSpace(branchID)}
	if productID := strings.TrimSpace(in.ProductID); productID != "" {
		args = append(args, productID)
		sql += ` AND product_id=$` + itoa(len(args))
	}
	if variantID := strings.TrimSpace(in.VariantID); variantID != "" {
		args = append(args, variantID)
		sql += ` AND variant_id=$` + itoa(len(args))
	}
	if status := strings.ToUpper(strings.TrimSpace(in.Status)); status != "" {
		args = append(args, status)
		sql += ` AND status=$` + itoa(len(args))
	} else if !in.IncludeArchived {
		sql += ` AND status <> 'ARCHIVED'`
	}
	args = append(args, limit+1, offset)
	sql += ` ORDER BY created_at DESC, id LIMIT $` + itoa(len(args)-1) + ` OFFSET $` + itoa(len(args))

	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var listings []BranchListing
	for rows.Next() {
		listing, scanErr := scanListing(rows)
		if scanErr != nil {
			return nil, false, scanErr
		}
		listings = append(listings, listing)
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(listings) > limit
	if hasMore {
		listings = listings[:limit]
	}
	return listings, hasMore, nil
}

// GetListing reads one listing with its enabled sell units.
func (s *Service) GetListing(ctx context.Context, actor Actor, scope Scope, listingID string) (BranchListing, []ListingSellUnit, error) {
	if scope.Empty() {
		return BranchListing{}, nil, errForbidden()
	}
	listing, err := loadListing(ctx, s.db, actor.VendorID, listingID, false)
	if err != nil {
		return BranchListing{}, nil, err
	}
	if err = s.requireListingAccess(ctx, s.db, actor, scope, listing); err != nil {
		return BranchListing{}, nil, err
	}
	units, err := listListingSellUnits(ctx, s.db, listing.ID)
	return listing, units, err
}

func listListingSellUnits(ctx context.Context, q querier, listingID string) ([]ListingSellUnit, error) {
	rows, err := q.Query(ctx, `SELECT `+listingSellUnitCols+`
		FROM shop_listing_sell_units WHERE listing_id=$1 ORDER BY position, id`, listingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	units := []ListingSellUnit{}
	for rows.Next() {
		unit, scanErr := scanListingSellUnit(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		units = append(units, unit)
	}
	return units, rows.Err()
}

// UpdateListingInput edits a listing.
type UpdateListingInput struct {
	OperatingModel      *string
	RetailEnabled       *bool
	WholesaleEnabled    *bool
	RetailOverride      **int64
	WholesaleOverride   **int64
	MinimumOrderQty     *int64
	OrderMultiple       *int64
	MaximumOrderQty     **int64
	PreorderPolicy      *string
	BackorderPolicy     *string
	AutoHideUnavailable *bool
	LogisticsAvailable  *bool
	SellUnitIDs         *[]string
	Status              *string
	ExpectedVersion     int64
}

// UpdateListing edits a branch listing.
func (s *Service) UpdateListing(ctx context.Context, actor Actor, scope Scope, listingID string, in UpdateListingInput) (BranchListing, error) {
	if err := s.requireWrites(); err != nil {
		return BranchListing{}, err
	}
	if scope.Empty() {
		return BranchListing{}, errForbidden()
	}
	var updated BranchListing
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		current, err := loadListing(ctx, tx, actor.VendorID, listingID, true)
		if err != nil {
			return err
		}
		if err = s.requireListingAccess(ctx, tx, actor, scope, current); err != nil {
			return err
		}
		if err = expectedVersionMatches(current.Version, in.ExpectedVersion); err != nil {
			return err
		}
		if current.Status == StatusArchived {
			return errArchived("This listing")
		}
		if in.RetailOverride != nil {
			if err = s.requirePricingAuthority(ctx, actor, current.BranchID); err != nil {
				return err
			}
			if err = validateMoney("retail_price_override_minor", *in.RetailOverride); err != nil {
				return err
			}
		}
		if in.WholesaleOverride != nil || in.WholesaleEnabled != nil ||
			in.MinimumOrderQty != nil || in.OrderMultiple != nil || in.MaximumOrderQty != nil {
			if err = s.requireWholesaleAuthority(ctx, actor, current.BranchID); err != nil {
				return err
			}
		}
		if in.WholesaleOverride != nil {
			if err = validateMoney("wholesale_price_override_minor", *in.WholesaleOverride); err != nil {
				return err
			}
		}

		operatingModel := current.OperatingModel
		retailEnabled, wholesaleEnabled := current.RetailEnabled, current.WholesaleEnabled
		retailOverride, wholesaleOverride := current.RetailOverrideMinor, current.WholesaleOverrideMin
		minimum, multiple, maximum := current.MinimumOrderQty, current.OrderMultiple, current.MaximumOrderQty
		preorder, backorder := current.PreorderPolicy, current.BackorderPolicy
		autoHide, logistics, status := current.AutoHideUnavailable, current.LogisticsAvailable, current.Status

		if in.OperatingModel != nil {
			value := strings.ToUpper(strings.TrimSpace(*in.OperatingModel))
			if value != "" {
				if !IsOperatingModel(value) {
					return coded(apperrors.Validation("Check your details.", []apperrors.FieldViolation{
						{Field: "operating_model", Message: "Choose a supported operating model."}}),
						CodeOperatingModelInvalid)
				}
				enabled, modelErr := branchModelEnabled(ctx, tx, actor.VendorID, current.BranchID, value)
				if modelErr != nil {
					return modelErr
				}
				if !enabled {
					return coded(apperrors.Conflict("This branch has not enabled the "+value+" operating model.", nil),
						CodeOperatingModelInvalid)
				}
			}
			operatingModel = value
		}
		if in.RetailEnabled != nil {
			retailEnabled = *in.RetailEnabled
		}
		if in.WholesaleEnabled != nil {
			wholesaleEnabled = *in.WholesaleEnabled
		}
		if !retailEnabled && !wholesaleEnabled {
			return fieldError("retail_enabled", "A listing must be enabled for retail, wholesale or both.")
		}
		if in.RetailOverride != nil {
			retailOverride = *in.RetailOverride
		}
		if in.WholesaleOverride != nil {
			wholesaleOverride = *in.WholesaleOverride
		}
		if in.MinimumOrderQty != nil || in.OrderMultiple != nil || in.MaximumOrderQty != nil {
			minimumCandidate, multipleCandidate, maximumCandidate := minimum, multiple, maximum
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
		if in.PreorderPolicy != nil {
			value, policyErr := normalizePolicy("preorder_policy", *in.PreorderPolicy)
			if policyErr != nil {
				return policyErr
			}
			preorder = value
		}
		if in.BackorderPolicy != nil {
			value, policyErr := normalizePolicy("backorder_policy", *in.BackorderPolicy)
			if policyErr != nil {
				return policyErr
			}
			backorder = value
		}
		if in.AutoHideUnavailable != nil {
			autoHide = *in.AutoHideUnavailable
		}
		if in.LogisticsAvailable != nil {
			logistics = *in.LogisticsAvailable
		}
		if in.Status != nil {
			value := strings.ToUpper(strings.TrimSpace(*in.Status))
			if value != StatusActive && value != StatusInactive {
				return fieldError("status", "Use ACTIVE or INACTIVE. Archive with the archive command.")
			}
			status = value
		}
		if status == StatusActive {
			product, productErr := loadProduct(ctx, tx, actor.VendorID, current.ProductID, false)
			if productErr != nil {
				return productErr
			}
			if productErr = validateRestrictedGoodsProduct("product", product); productErr != nil {
				return productErr
			}
		}

		row, err := scanListing(tx.QueryRow(ctx, `
			UPDATE shop_branch_listings
			SET operating_model=NULLIF($3,''), retail_enabled=$4, wholesale_enabled=$5,
			    retail_price_override_minor=$6, wholesale_price_override_minor=$7,
			    minimum_order_quantity=$8, order_multiple=$9, maximum_order_quantity=$10,
			    preorder_policy=$11, backorder_policy=$12, auto_hide_when_unavailable=$13,
			    logistics_available=$14, status=$15, updated_at=now(), version=version+1
			WHERE business_id=$1 AND id=$2 RETURNING `+listingCols,
			actor.VendorID, current.ID, operatingModel, retailEnabled, wholesaleEnabled,
			retailOverride, wholesaleOverride, minimum, multiple, maximum, preorder, backorder,
			autoHide, logistics, status))
		if err != nil {
			return mapDB(err, "Listing", CodeListingNotFound)
		}
		updated = row
		if in.SellUnitIDs != nil {
			if err = replaceListingSellUnits(ctx, tx, actor.VendorID, updated, *in.SellUnitIDs); err != nil {
				return err
			}
		}
		currency, err := s.businessCurrency(ctx, tx, actor.VendorID)
		if err != nil {
			return err
		}
		if moneyChanged(current.RetailOverrideMinor, updated.RetailOverrideMinor) {
			if err = s.appendPriceVersion(ctx, tx, actor, currency, priceChange{
				subjectType: SubjectListingRetail, subjectRef: updated.ID, branchID: updated.BranchID,
				productRef: updated.ProductID, variantRef: updated.VariantID,
				previousAmount: current.RetailOverrideMinor, amount: updated.RetailOverrideMinor,
				subjectVersion: updated.Version,
				reason:         overrideReason(current.RetailOverrideMinor, updated.RetailOverrideMinor)}); err != nil {
				return err
			}
		}
		if moneyChanged(current.WholesaleOverrideMin, updated.WholesaleOverrideMin) {
			if err = s.appendPriceVersion(ctx, tx, actor, currency, priceChange{
				subjectType: SubjectListingWholesale, subjectRef: updated.ID, branchID: updated.BranchID,
				productRef: updated.ProductID, variantRef: updated.VariantID,
				previousAmount: current.WholesaleOverrideMin, amount: updated.WholesaleOverrideMin,
				subjectVersion: updated.Version,
				reason:         overrideReason(current.WholesaleOverrideMin, updated.WholesaleOverrideMin)}); err != nil {
				return err
			}
		}
		if _, err = bumpBranchRevision(ctx, tx, actor.VendorID, updated.BranchID); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "shop.listing.updated", eventType: "shop.listing.changed",
			aggregateType: "shop_branch_listing", aggregateID: updated.ID, branchID: updated.BranchID,
			version: updated.Version, before: ListingViewOf(current), after: ListingViewOf(updated),
			payload: ListingViewOf(updated),
		})
	})
	return updated, err
}

func overrideReason(previous, next *int64) string {
	switch {
	case next == nil:
		return ReasonOverrideCleared
	case previous == nil:
		return ReasonOverrideSet
	default:
		return ReasonPriceChanged
	}
}

// ArchiveListing retires a listing and the tiers priced against it.
func (s *Service) ArchiveListing(ctx context.Context, actor Actor, scope Scope, listingID string, expectedVersion int64) (BranchListing, error) {
	if err := s.requireWrites(); err != nil {
		return BranchListing{}, err
	}
	if scope.Empty() {
		return BranchListing{}, errForbidden()
	}
	var archived BranchListing
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		current, err := loadListing(ctx, tx, actor.VendorID, listingID, true)
		if err != nil {
			return err
		}
		if err = s.requireListingAccess(ctx, tx, actor, scope, current); err != nil {
			return err
		}
		if err = expectedVersionMatches(current.Version, expectedVersion); err != nil {
			return err
		}
		if current.Status == StatusArchived {
			archived = current
			return nil
		}
		row, err := scanListing(tx.QueryRow(ctx, `
			UPDATE shop_branch_listings SET status='ARCHIVED', archived_at=now(), updated_at=now(), version=version+1
			WHERE business_id=$1 AND id=$2 RETURNING `+listingCols, actor.VendorID, current.ID))
		if err != nil {
			return mapDB(err, "Listing", CodeListingNotFound)
		}
		archived = row
		if _, err = tx.Exec(ctx, `
			UPDATE shop_quantity_breaks SET status='ARCHIVED', archived_at=now(), updated_at=now(), version=version+1
			WHERE business_id=$1 AND listing_id=$2 AND status='ACTIVE'`, actor.VendorID, current.ID); err != nil {
			return err
		}
		if _, err = bumpBranchRevision(ctx, tx, actor.VendorID, current.BranchID); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "shop.listing.archived", eventType: "shop.listing.changed",
			aggregateType: "shop_branch_listing", aggregateID: archived.ID, branchID: archived.BranchID,
			version: archived.Version, before: ListingViewOf(current), after: ListingViewOf(archived),
			payload: ListingViewOf(archived),
		})
	})
	return archived, err
}

// SetListingSellUnitPriceInput overrides one sell unit's price at one branch.
type SetListingSellUnitPriceInput struct {
	Enabled           *bool
	CalibrationID     **string
	PriceOverride     **int64
	WholesaleOverride **int64
	ExpectedVersion   int64
}

// SetListingSellUnitPrice sets a branch's price for one sell unit — the most
// specific retail price the resolver knows.
func (s *Service) SetListingSellUnitPrice(ctx context.Context, actor Actor, scope Scope, listingID, listingUnitID string, in SetListingSellUnitPriceInput) (ListingSellUnit, error) {
	if err := s.requireWrites(); err != nil {
		return ListingSellUnit{}, err
	}
	if scope.Empty() {
		return ListingSellUnit{}, errForbidden()
	}
	var updated ListingSellUnit
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		listing, err := loadListing(ctx, tx, actor.VendorID, listingID, false)
		if err != nil {
			return err
		}
		if err = s.requireListingAccess(ctx, tx, actor, scope, listing); err != nil {
			return err
		}
		if listing.Status == StatusArchived {
			return errArchived("This listing")
		}
		current, err := scanListingSellUnit(tx.QueryRow(ctx, `SELECT `+listingSellUnitCols+`
			FROM shop_listing_sell_units WHERE business_id=$1 AND listing_id=$2 AND id=$3 FOR UPDATE`,
			actor.VendorID, listing.ID, strings.TrimSpace(listingUnitID)))
		if err != nil {
			return mapDB(err, "Listing sell unit", CodeSellUnitNotFound)
		}
		if err = expectedVersionMatches(current.Version, in.ExpectedVersion); err != nil {
			return err
		}
		if in.PriceOverride != nil {
			if err = s.requirePricingAuthority(ctx, actor, listing.BranchID); err != nil {
				return err
			}
			if err = validateMoney("price_override_minor", *in.PriceOverride); err != nil {
				return err
			}
		}
		if in.WholesaleOverride != nil {
			if err = s.requireWholesaleAuthority(ctx, actor, listing.BranchID); err != nil {
				return err
			}
			if err = validateMoney("wholesale_price_override_minor", *in.WholesaleOverride); err != nil {
				return err
			}
		}
		enabled, calibrationID := current.Enabled, current.CalibrationID
		price, wholesale := current.PriceOverrideMinor, current.WholesaleOverrideMin
		if in.Enabled != nil {
			enabled = *in.Enabled
		}
		unit, err := loadSellUnit(ctx, tx, actor.VendorID, current.SellUnitID, false)
		if err != nil {
			return err
		}
		if enabled {
			if unit.Status != StatusActive {
				return coded(apperrors.Conflict("Only an active sell unit can be enabled on a branch listing.", nil),
					CodeSellUnitNotFound)
			}
			if !(listing.RetailEnabled && unit.RetailEnabled) && !(listing.WholesaleEnabled && unit.WholesaleEnabled) {
				return coded(apperrors.Conflict("This sell unit is not enabled for either sales channel on the listing.", nil),
					CodeSellUnitNotFound)
			}
		}
		if in.CalibrationID != nil {
			calibrationID = strings.TrimSpace(derefString(*in.CalibrationID))
			if calibrationID != "" {
				if unit.ConversionType == ConversionExact {
					return coded(apperrors.Validation("Check your details.", []apperrors.FieldViolation{{
						Field: "calibration_id", Message: "An exact sell unit cannot be replaced by an approximate branch calibration."}}),
						CodeConversionInvalid)
				}
				calibration, calibrationErr := loadCalibration(ctx, tx, actor.VendorID, listing.ProductID, calibrationID, false)
				if calibrationErr != nil {
					return calibrationErr
				}
				if calibration.Status == StatusArchived || calibration.BranchID != listing.BranchID ||
					(calibration.VariantID != "" && calibration.VariantID != listing.VariantID) ||
					calibration.MeasureID != unit.MeasureID {
					return coded(apperrors.Validation("Check your details.", []apperrors.FieldViolation{{
						Field: "calibration_id", Message: "Use an active calibration for this branch, variant and measure."}}),
						CodeConversionInvalid)
				}
			}
		}
		if in.PriceOverride != nil {
			price = *in.PriceOverride
		}
		if in.WholesaleOverride != nil {
			wholesale = *in.WholesaleOverride
		}
		updated, err = scanListingSellUnit(tx.QueryRow(ctx, `
			UPDATE shop_listing_sell_units
			SET enabled=$3, calibration_id=NULLIF($4,''), price_override_minor=$5, wholesale_price_override_minor=$6,
			    updated_at=now(), version=version+1
			WHERE business_id=$1 AND id=$2
			RETURNING `+listingSellUnitCols,
			actor.VendorID, current.ID, enabled, calibrationID, price, wholesale))
		if err != nil {
			return mapDB(err, "Listing sell unit", CodeSellUnitNotFound)
		}
		currency, err := s.businessCurrency(ctx, tx, actor.VendorID)
		if err != nil {
			return err
		}
		if moneyChanged(current.PriceOverrideMinor, updated.PriceOverrideMinor) {
			if err = s.appendPriceVersion(ctx, tx, actor, currency, priceChange{
				subjectType: SubjectListingUnitRetail, subjectRef: updated.ID, branchID: listing.BranchID,
				productRef: listing.ProductID, variantRef: listing.VariantID,
				previousAmount: current.PriceOverrideMinor, amount: updated.PriceOverrideMinor,
				subjectVersion: updated.Version,
				reason:         overrideReason(current.PriceOverrideMinor, updated.PriceOverrideMinor)}); err != nil {
				return err
			}
		}
		if moneyChanged(current.WholesaleOverrideMin, updated.WholesaleOverrideMin) {
			if err = s.appendPriceVersion(ctx, tx, actor, currency, priceChange{
				subjectType: SubjectListingUnitWholesa, subjectRef: updated.ID, branchID: listing.BranchID,
				productRef: listing.ProductID, variantRef: listing.VariantID,
				previousAmount: current.WholesaleOverrideMin, amount: updated.WholesaleOverrideMin,
				subjectVersion: updated.Version,
				reason:         overrideReason(current.WholesaleOverrideMin, updated.WholesaleOverrideMin)}); err != nil {
				return err
			}
		}
		if _, err = bumpBranchRevision(ctx, tx, actor.VendorID, listing.BranchID); err != nil {
			return err
		}
		eventType := "shop.listing.changed"
		if moneyChanged(current.PriceOverrideMinor, updated.PriceOverrideMinor) ||
			moneyChanged(current.WholesaleOverrideMin, updated.WholesaleOverrideMin) {
			eventType = "shop.price.changed"
		}
		return s.record(ctx, tx, actor, evidence{
			action: "shop.listing.sell_unit_updated", eventType: eventType,
			aggregateType: "shop_branch_listing", aggregateID: listing.ID, branchID: listing.BranchID,
			version: updated.Version, before: ListingSellUnitViewOf(current), after: ListingSellUnitViewOf(updated),
			payload: ListingSellUnitViewOf(updated),
		})
	})
	return updated, err
}

// ---------------------------------------------------------------------------
// Quantity breaks
// ---------------------------------------------------------------------------

const breakCols = `id,business_id,listing_id,COALESCE(sell_unit_id,''),min_quantity,max_quantity,
	unit_price_minor,wholesale_only,lead_time_days,status,created_at,updated_at,archived_at,version`

func scanBreak(row rowScanner) (QuantityBreak, error) {
	var b QuantityBreak
	err := row.Scan(&b.ID, &b.BusinessID, &b.ListingID, &b.SellUnitID, &b.MinQuantity, &b.MaxQuantity,
		&b.UnitPriceMinor, &b.WholesaleOnly, &b.LeadTimeDays, &b.Status, &b.CreatedAt, &b.UpdatedAt,
		&b.ArchivedAt, &b.Version)
	return b, err
}

// CreateQuantityBreakInput is the tier-create command.
type CreateQuantityBreakInput struct {
	SellUnitID     string
	MinQuantity    int64
	MaxQuantity    *int64
	UnitPriceMinor int64
	WholesaleOnly  *bool
	LeadTimeDays   *int
}

// CreateQuantityBreak adds one wholesale tier.
//
// Bands may not overlap, and only one may be open-ended. Both rules are
// enforced in this transaction AND by the database, because an ambiguous tier
// table means two defensible answers to "what does this cost".
func (s *Service) CreateQuantityBreak(ctx context.Context, actor Actor, scope Scope, listingID string, in CreateQuantityBreakInput) (QuantityBreak, error) {
	if err := s.requireWrites(); err != nil {
		return QuantityBreak{}, err
	}
	if scope.Empty() {
		return QuantityBreak{}, errForbidden()
	}
	if in.MinQuantity < 1 || in.MinQuantity > 1000000000 {
		return QuantityBreak{}, fieldError("min_quantity", "The minimum quantity must be at least one.")
	}
	if in.MaxQuantity != nil && (*in.MaxQuantity < in.MinQuantity || *in.MaxQuantity > 1000000000) {
		return QuantityBreak{}, fieldError("max_quantity", "The maximum quantity must be at least the minimum.")
	}
	if in.UnitPriceMinor < 0 || in.UnitPriceMinor > MaxMoneyMinor {
		return QuantityBreak{}, fieldError("unit_price_minor", "Amounts are whole minor units and must be zero or greater.")
	}
	if in.LeadTimeDays != nil && (*in.LeadTimeDays < 0 || *in.LeadTimeDays > 365) {
		return QuantityBreak{}, fieldError("lead_time_days", "Lead time must be between 0 and 365 days.")
	}

	var created QuantityBreak
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		listing, err := loadListing(ctx, tx, actor.VendorID, listingID, true)
		if err != nil {
			return err
		}
		if err = s.requireListingAccess(ctx, tx, actor, scope, listing); err != nil {
			return err
		}
		if err = s.requireWholesaleAuthority(ctx, actor, listing.BranchID); err != nil {
			return err
		}
		if listing.Status == StatusArchived {
			return errArchived("This listing")
		}
		sellUnitID := strings.TrimSpace(in.SellUnitID)
		if sellUnitID != "" {
			unit, unitErr := loadSellUnit(ctx, tx, actor.VendorID, sellUnitID, false)
			if unitErr != nil {
				return unitErr
			}
			if unit.VariantID != listing.VariantID {
				return errNotFound("Sell unit", CodeSellUnitNotFound)
			}
			var selected bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM shop_listing_sell_units
				WHERE business_id=$1 AND listing_id=$2 AND sell_unit_id=$3 AND enabled)`,
				actor.VendorID, listing.ID, sellUnitID).Scan(&selected); err != nil {
				return err
			}
			if !selected {
				return coded(apperrors.Conflict("Enable this sell unit on the branch listing before pricing quantity tiers.", nil),
					CodeSellUnitNotFound)
			}
		}
		var count int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM shop_quantity_breaks
			WHERE listing_id=$1 AND status='ACTIVE'`, listing.ID).Scan(&count); err != nil {
			return err
		}
		if count >= MaxQuantityBreaks {
			return fieldError("quantity_break", "A listing may have at most "+itoa(MaxQuantityBreaks)+" active tiers.")
		}
		if err = ensureNoOverlap(ctx, tx, listing.ID, sellUnitID, "", in.MinQuantity, in.MaxQuantity); err != nil {
			return err
		}
		row, err := scanBreak(tx.QueryRow(ctx, `
			INSERT INTO shop_quantity_breaks
				(id,business_id,listing_id,sell_unit_id,min_quantity,max_quantity,unit_price_minor,
				 wholesale_only,lead_time_days,status)
			VALUES ($1,$2,$3,NULLIF($4,''),$5,$6,$7,$8,$9,'ACTIVE') RETURNING `+breakCols,
			newID("sqb_"), actor.VendorID, listing.ID, sellUnitID, in.MinQuantity, in.MaxQuantity,
			in.UnitPriceMinor, boolOrDefault(in.WholesaleOnly, true), in.LeadTimeDays))
		if err != nil {
			return mapDB(err, "Quantity break", CodeQuantityBreakNotFound)
		}
		created = row
		currency, err := s.businessCurrency(ctx, tx, actor.VendorID)
		if err != nil {
			return err
		}
		price := created.UnitPriceMinor
		if err = s.appendPriceVersion(ctx, tx, actor, currency, priceChange{
			subjectType: SubjectQuantityBreak, subjectRef: created.ID, branchID: listing.BranchID,
			productRef: listing.ProductID, variantRef: listing.VariantID, amount: &price,
			reason: ReasonCreated, subjectVersion: created.Version}); err != nil {
			return err
		}
		if _, err = bumpBranchRevision(ctx, tx, actor.VendorID, listing.BranchID); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "shop.quantity_break.created", eventType: "shop.quantity_break.changed",
			aggregateType: "shop_quantity_break", aggregateID: created.ID, branchID: listing.BranchID,
			version: created.Version, after: QuantityBreakViewOf(created), payload: QuantityBreakViewOf(created),
		})
	})
	return created, err
}

// ensureNoOverlap refuses a band that intersects an existing active one.
func ensureNoOverlap(ctx context.Context, q querier, listingID, sellUnitID, excludeID string, minimum int64, maximum *int64) error {
	var overlaps bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM shop_quantity_breaks
			WHERE listing_id=$1 AND sell_unit_key=$2 AND status='ACTIVE' AND id <> $3
			  AND $4 <= COALESCE(max_quantity, 9223372036854775807)
			  AND min_quantity <= COALESCE($5::bigint, 9223372036854775807))`,
		listingID, sellUnitID, excludeID, minimum, maximum).Scan(&overlaps)
	if err != nil {
		return err
	}
	if overlaps {
		return coded(apperrors.Conflict("This quantity band overlaps an existing active band.", nil),
			CodeQuantityBreakOverlap)
	}
	return nil
}

// ListQuantityBreaks reads a listing's tiers in ascending order.
func (s *Service) ListQuantityBreaks(ctx context.Context, actor Actor, scope Scope, listingID string, includeArchived bool) ([]QuantityBreak, error) {
	if scope.Empty() {
		return nil, errForbidden()
	}
	listing, err := loadListing(ctx, s.db, actor.VendorID, listingID, false)
	if err != nil {
		return nil, err
	}
	if err = s.requireListingAccess(ctx, s.db, actor, scope, listing); err != nil {
		return nil, err
	}
	sql := `SELECT ` + breakCols + ` FROM shop_quantity_breaks WHERE business_id=$1 AND listing_id=$2`
	if !includeArchived {
		sql += ` AND status <> 'ARCHIVED'`
	}
	sql += ` ORDER BY sell_unit_key, min_quantity LIMIT 100`
	rows, err := s.db.Query(ctx, sql, actor.VendorID, listing.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	breaks := []QuantityBreak{}
	for rows.Next() {
		band, scanErr := scanBreak(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		breaks = append(breaks, band)
	}
	return breaks, rows.Err()
}

// UpdateQuantityBreakInput edits a tier.
type UpdateQuantityBreakInput struct {
	MinQuantity     *int64
	MaxQuantity     **int64
	UnitPriceMinor  *int64
	WholesaleOnly   *bool
	LeadTimeDays    **int
	ExpectedVersion int64
}

// UpdateQuantityBreak revises a tier and appends price history.
func (s *Service) UpdateQuantityBreak(ctx context.Context, actor Actor, scope Scope, listingID, breakID string, in UpdateQuantityBreakInput) (QuantityBreak, error) {
	if err := s.requireWrites(); err != nil {
		return QuantityBreak{}, err
	}
	if scope.Empty() {
		return QuantityBreak{}, errForbidden()
	}
	var updated QuantityBreak
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		listing, err := loadListing(ctx, tx, actor.VendorID, listingID, true)
		if err != nil {
			return err
		}
		if err = s.requireListingAccess(ctx, tx, actor, scope, listing); err != nil {
			return err
		}
		if err = s.requireWholesaleAuthority(ctx, actor, listing.BranchID); err != nil {
			return err
		}
		current, err := scanBreak(tx.QueryRow(ctx, `SELECT `+breakCols+` FROM shop_quantity_breaks
			WHERE business_id=$1 AND listing_id=$2 AND id=$3 FOR UPDATE`,
			actor.VendorID, listing.ID, strings.TrimSpace(breakID)))
		if err != nil {
			return mapDB(err, "Quantity break", CodeQuantityBreakNotFound)
		}
		if err = expectedVersionMatches(current.Version, in.ExpectedVersion); err != nil {
			return err
		}
		if current.Status == StatusArchived {
			return errArchived("This quantity break")
		}
		minimum, maximum := current.MinQuantity, current.MaxQuantity
		price, wholesaleOnly, leadTime := current.UnitPriceMinor, current.WholesaleOnly, current.LeadTimeDays
		if in.MinQuantity != nil {
			if *in.MinQuantity < 1 || *in.MinQuantity > 1000000000 {
				return fieldError("min_quantity", "The minimum quantity must be at least one.")
			}
			minimum = *in.MinQuantity
		}
		if in.MaxQuantity != nil {
			maximum = *in.MaxQuantity
		}
		if maximum != nil && *maximum < minimum {
			return fieldError("max_quantity", "The maximum quantity must be at least the minimum.")
		}
		if in.UnitPriceMinor != nil {
			if *in.UnitPriceMinor < 0 || *in.UnitPriceMinor > MaxMoneyMinor {
				return fieldError("unit_price_minor", "Amounts are whole minor units and must be zero or greater.")
			}
			price = *in.UnitPriceMinor
		}
		if in.WholesaleOnly != nil {
			wholesaleOnly = *in.WholesaleOnly
		}
		if in.LeadTimeDays != nil {
			if *in.LeadTimeDays != nil && (**in.LeadTimeDays < 0 || **in.LeadTimeDays > 365) {
				return fieldError("lead_time_days", "Lead time must be between 0 and 365 days.")
			}
			leadTime = *in.LeadTimeDays
		}
		if err = ensureNoOverlap(ctx, tx, listing.ID, current.SellUnitID, current.ID, minimum, maximum); err != nil {
			return err
		}
		row, err := scanBreak(tx.QueryRow(ctx, `
			UPDATE shop_quantity_breaks
			SET min_quantity=$3, max_quantity=$4, unit_price_minor=$5, wholesale_only=$6,
			    lead_time_days=$7, updated_at=now(), version=version+1
			WHERE business_id=$1 AND id=$2 RETURNING `+breakCols,
			actor.VendorID, current.ID, minimum, maximum, price, wholesaleOnly, leadTime))
		if err != nil {
			return mapDB(err, "Quantity break", CodeQuantityBreakNotFound)
		}
		updated = row
		if current.UnitPriceMinor != updated.UnitPriceMinor {
			currency, currencyErr := s.businessCurrency(ctx, tx, actor.VendorID)
			if currencyErr != nil {
				return currencyErr
			}
			previous, next := current.UnitPriceMinor, updated.UnitPriceMinor
			if err = s.appendPriceVersion(ctx, tx, actor, currency, priceChange{
				subjectType: SubjectQuantityBreak, subjectRef: updated.ID, branchID: listing.BranchID,
				productRef: listing.ProductID, variantRef: listing.VariantID,
				previousAmount: &previous, amount: &next, reason: ReasonPriceChanged,
				subjectVersion: updated.Version}); err != nil {
				return err
			}
		}
		if _, err = bumpBranchRevision(ctx, tx, actor.VendorID, listing.BranchID); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "shop.quantity_break.updated", eventType: "shop.quantity_break.changed",
			aggregateType: "shop_quantity_break", aggregateID: updated.ID, branchID: listing.BranchID,
			version: updated.Version, before: QuantityBreakViewOf(current), after: QuantityBreakViewOf(updated),
			payload: QuantityBreakViewOf(updated),
		})
	})
	return updated, err
}

// ArchiveQuantityBreak retires a tier.
func (s *Service) ArchiveQuantityBreak(ctx context.Context, actor Actor, scope Scope, listingID, breakID string, expectedVersion int64) (QuantityBreak, error) {
	if err := s.requireWrites(); err != nil {
		return QuantityBreak{}, err
	}
	if scope.Empty() {
		return QuantityBreak{}, errForbidden()
	}
	var archived QuantityBreak
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		listing, err := loadListing(ctx, tx, actor.VendorID, listingID, true)
		if err != nil {
			return err
		}
		if err = s.requireListingAccess(ctx, tx, actor, scope, listing); err != nil {
			return err
		}
		if err = s.requireWholesaleAuthority(ctx, actor, listing.BranchID); err != nil {
			return err
		}
		current, err := scanBreak(tx.QueryRow(ctx, `SELECT `+breakCols+` FROM shop_quantity_breaks
			WHERE business_id=$1 AND listing_id=$2 AND id=$3 FOR UPDATE`,
			actor.VendorID, listing.ID, strings.TrimSpace(breakID)))
		if err != nil {
			return mapDB(err, "Quantity break", CodeQuantityBreakNotFound)
		}
		if err = expectedVersionMatches(current.Version, expectedVersion); err != nil {
			return err
		}
		if current.Status == StatusArchived {
			archived = current
			return nil
		}
		row, err := scanBreak(tx.QueryRow(ctx, `
			UPDATE shop_quantity_breaks SET status='ARCHIVED', archived_at=now(), updated_at=now(), version=version+1
			WHERE business_id=$1 AND id=$2 RETURNING `+breakCols, actor.VendorID, current.ID))
		if err != nil {
			return mapDB(err, "Quantity break", CodeQuantityBreakNotFound)
		}
		archived = row
		currency, err := s.businessCurrency(ctx, tx, actor.VendorID)
		if err != nil {
			return err
		}
		previous := current.UnitPriceMinor
		if err = s.appendPriceVersion(ctx, tx, actor, currency, priceChange{
			subjectType: SubjectQuantityBreak, subjectRef: archived.ID, branchID: listing.BranchID,
			productRef: listing.ProductID, variantRef: listing.VariantID, previousAmount: &previous,
			reason: ReasonArchived, subjectVersion: archived.Version}); err != nil {
			return err
		}
		if _, err = bumpBranchRevision(ctx, tx, actor.VendorID, listing.BranchID); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "shop.quantity_break.archived", eventType: "shop.quantity_break.changed",
			aggregateType: "shop_quantity_break", aggregateID: archived.ID, branchID: listing.BranchID,
			version: archived.Version, before: QuantityBreakViewOf(current), after: QuantityBreakViewOf(archived),
			payload: QuantityBreakViewOf(archived),
		})
	})
	return archived, err
}

// ---------------------------------------------------------------------------
// Price resolution
// ---------------------------------------------------------------------------

// Price sources, in the order the resolver considers them.
const (
	SourceListingSellUnitOverride = "LISTING_SELL_UNIT_OVERRIDE"
	SourceListingOverride         = "LISTING_OVERRIDE"
	SourceSellUnitPrice           = "SELL_UNIT_PRICE"
	SourceVariantBase             = "VARIANT_BASE"
	SourceQuantityBreak           = "QUANTITY_BREAK"
	SourceUnpriced                = "UNPRICED"
)

// ResolvedPrice is the deterministic commercial answer for one listing, sell
// unit and quantity. It is configuration truth, not a charge: nothing here
// touches a wallet, a cart or an order.
type ResolvedPrice struct {
	ListingRef      string
	BranchRef       string
	VariantRef      string
	SellUnitRef     string
	Currency        string
	Quantity        int64
	Mode            string
	UnitPriceMinor  *int64
	TotalPriceMinor *int64
	Source          string
	QuantityBreakID string
	MinimumOrderQty int64
	OrderMultiple   int64
	MaximumOrderQty *int64
	Satisfied       bool
	Reason          string
}

// ResolvePrice applies the documented precedence.
//
// RETAIL:   branch sell-unit override → branch listing override →
//
//	sell-unit price → variant retail base.
//
// WHOLESALE: matching quantity break → branch sell-unit wholesale override →
//
//	branch listing wholesale override → sell-unit wholesale price →
//	variant wholesale base.
//
// A price that resolves nowhere is reported as UNPRICED rather than as zero:
// free and unpriced are not the same commercial statement.
func (s *Service) ResolvePrice(ctx context.Context, actor Actor, scope Scope, listingID, sellUnitID string, quantity int64, wholesale bool) (ResolvedPrice, error) {
	if scope.Empty() {
		return ResolvedPrice{}, errForbidden()
	}
	if quantity < 1 {
		quantity = 1
	}
	listing, err := loadListing(ctx, s.db, actor.VendorID, listingID, false)
	if err != nil {
		return ResolvedPrice{}, err
	}
	if err = s.requireListingAccess(ctx, s.db, actor, scope, listing); err != nil {
		return ResolvedPrice{}, err
	}
	currency, err := s.businessCurrency(ctx, s.db, actor.VendorID)
	if err != nil {
		return ResolvedPrice{}, err
	}
	variant, err := loadVariantByID(ctx, s.db, actor.VendorID, listing.VariantID, false)
	if err != nil {
		return ResolvedPrice{}, err
	}

	resolved := ResolvedPrice{
		ListingRef: listing.ID, BranchRef: listing.BranchID, VariantRef: listing.VariantID,
		SellUnitRef: strings.TrimSpace(sellUnitID), Currency: currency, Quantity: quantity,
		Mode: "RETAIL", Source: SourceUnpriced,
		MinimumOrderQty: listing.MinimumOrderQty, OrderMultiple: listing.OrderMultiple,
		MaximumOrderQty: listing.MaximumOrderQty,
	}
	if wholesale {
		resolved.Mode = "WHOLESALE"
	}
	if wholesale && !listing.WholesaleEnabled {
		resolved.Reason = "This listing is not enabled for wholesale."
		return resolved, nil
	}
	if !wholesale && !listing.RetailEnabled {
		resolved.Reason = "This listing is not enabled for retail."
		return resolved, nil
	}

	var unit *SellUnit
	var listingUnit *ListingSellUnit
	if resolved.SellUnitRef != "" {
		loaded, unitErr := loadSellUnit(ctx, s.db, actor.VendorID, resolved.SellUnitRef, false)
		if unitErr != nil {
			return ResolvedPrice{}, unitErr
		}
		if loaded.VariantID != listing.VariantID {
			return ResolvedPrice{}, errNotFound("Sell unit", CodeSellUnitNotFound)
		}
		if loaded.Status != StatusActive || (wholesale && !loaded.WholesaleEnabled) || (!wholesale && !loaded.RetailEnabled) {
			resolved.Reason = "This sell unit is not enabled for the requested sales channel."
			return resolved, nil
		}
		unit = &loaded
		units, listErr := listListingSellUnits(ctx, s.db, listing.ID)
		if listErr != nil {
			return ResolvedPrice{}, listErr
		}
		for index := range units {
			if units[index].SellUnitID == loaded.ID && units[index].Enabled {
				listingUnit = &units[index]
				break
			}
		}
		if listingUnit == nil {
			resolved.Reason = "This sell unit is not enabled for the branch listing."
			return resolved, nil
		}
		// Sell-unit order rules are tighter than the listing's where present.
		if loaded.MinimumOrderQty > resolved.MinimumOrderQty {
			resolved.MinimumOrderQty = loaded.MinimumOrderQty
		}
		if loaded.OrderMultiple > resolved.OrderMultiple {
			resolved.OrderMultiple = loaded.OrderMultiple
		}
		if loaded.MaximumOrderQty != nil &&
			(resolved.MaximumOrderQty == nil || *loaded.MaximumOrderQty < *resolved.MaximumOrderQty) {
			resolved.MaximumOrderQty = loaded.MaximumOrderQty
		}
	}

	if wholesale {
		band, bandErr := s.matchQuantityBreak(ctx, actor.VendorID, listing.ID, resolved.SellUnitRef, quantity)
		if bandErr != nil {
			return ResolvedPrice{}, bandErr
		}
		if band != nil {
			price := band.UnitPriceMinor
			resolved.UnitPriceMinor = &price
			resolved.Source = SourceQuantityBreak
			resolved.QuantityBreakID = band.ID
		} else if listingUnit != nil && listingUnit.WholesaleOverrideMin != nil {
			resolved.UnitPriceMinor = listingUnit.WholesaleOverrideMin
			resolved.Source = SourceListingSellUnitOverride
		} else if listing.WholesaleOverrideMin != nil {
			resolved.UnitPriceMinor = listing.WholesaleOverrideMin
			resolved.Source = SourceListingOverride
		} else if unit != nil && unit.WholesalePriceMinor != nil {
			resolved.UnitPriceMinor = unit.WholesalePriceMinor
			resolved.Source = SourceSellUnitPrice
		} else if variant.WholesalePriceMinor != nil {
			resolved.UnitPriceMinor = variant.WholesalePriceMinor
			resolved.Source = SourceVariantBase
		}
	} else {
		switch {
		case listingUnit != nil && listingUnit.PriceOverrideMinor != nil:
			resolved.UnitPriceMinor = listingUnit.PriceOverrideMinor
			resolved.Source = SourceListingSellUnitOverride
		case listing.RetailOverrideMinor != nil:
			resolved.UnitPriceMinor = listing.RetailOverrideMinor
			resolved.Source = SourceListingOverride
		case unit != nil && unit.PriceMinor != nil:
			resolved.UnitPriceMinor = unit.PriceMinor
			resolved.Source = SourceSellUnitPrice
		case variant.RetailPriceMinor != nil:
			resolved.UnitPriceMinor = variant.RetailPriceMinor
			resolved.Source = SourceVariantBase
		}
	}

	if resolved.UnitPriceMinor == nil {
		resolved.Reason = "No price is configured for this listing and unit."
		return resolved, nil
	}
	// Integer money throughout, with an explicit overflow guard rather than a
	// silent wrap.
	if *resolved.UnitPriceMinor != 0 && quantity > MaxMoneyMinor / *resolved.UnitPriceMinor {
		resolved.Reason = "The requested quantity exceeds the supported total."
		return resolved, nil
	}
	total := *resolved.UnitPriceMinor * quantity
	resolved.TotalPriceMinor = &total
	resolved.Satisfied = quantity >= resolved.MinimumOrderQty &&
		quantity%resolved.OrderMultiple == 0 &&
		(resolved.MaximumOrderQty == nil || quantity <= *resolved.MaximumOrderQty)
	if !resolved.Satisfied {
		resolved.Reason = "The requested quantity does not satisfy the configured order rules."
	}
	return resolved, nil
}

// matchQuantityBreak finds the band covering a quantity. Bands never overlap,
// so at most one can match and the boundary behaviour is unambiguous.
func (s *Service) matchQuantityBreak(ctx context.Context, businessID, listingID, sellUnitID string, quantity int64) (*QuantityBreak, error) {
	band, err := scanBreak(s.db.QueryRow(ctx, `SELECT `+breakCols+` FROM shop_quantity_breaks
		WHERE business_id=$1 AND listing_id=$2 AND sell_unit_key=$3 AND status='ACTIVE'
		  AND min_quantity <= $4 AND (max_quantity IS NULL OR max_quantity >= $4)
		ORDER BY min_quantity DESC LIMIT 1`, businessID, listingID, sellUnitID, quantity))
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return nil, nil
		}
		var notFound = mapDB(err, "Quantity break", CodeQuantityBreakNotFound)
		if appErr, ok := notFound.(*apperrors.Error); ok && appErr.Status == 404 {
			return nil, nil
		}
		return nil, err
	}
	return &band, nil
}

// ---------------------------------------------------------------------------
// Price history
// ---------------------------------------------------------------------------

// PriceHistoryOptions bounds a history read.
type PriceHistoryOptions struct {
	SubjectType   string
	SubjectRef    string
	VariantRef    string
	ProductRef    string
	Limit, Offset int
}

// ListPriceHistory reads append-only commercial evidence.
func (s *Service) ListPriceHistory(ctx context.Context, actor Actor, scope Scope, in PriceHistoryOptions) ([]PriceVersion, bool, error) {
	if scope.Empty() {
		return nil, false, errForbidden()
	}
	limit, offset := boundedPage(in.Limit, in.Offset)
	sql := `SELECT id,business_id,COALESCE(branch_id,''),subject_type,subject_ref,COALESCE(product_ref,''),
		COALESCE(variant_ref,''),currency,previous_amount_minor,amount_minor,previous_inherited,inherited,
		change_reason,subject_version,revision,actor_type,COALESCE(actor_id,''),COALESCE(correlation_id,''),
		COALESCE(command_ref,''),recorded_at,effective_at
		FROM shop_price_versions WHERE business_id=$1`
	args := []any{actor.VendorID}
	if !scope.Global {
		args = append(args, scope.BranchIDs)
		// Branch-specific commercial evidence is scoped in SQL before
		// pagination. Business-level base prices remain visible because they
		// are shared catalogue truth for every branch.
		sql += ` AND (branch_id IS NULL OR branch_id=ANY($2))`
	}
	if subjectType := strings.ToUpper(strings.TrimSpace(in.SubjectType)); subjectType != "" {
		args = append(args, subjectType)
		sql += ` AND subject_type=$` + itoa(len(args))
	}
	if subjectRef := strings.TrimSpace(in.SubjectRef); subjectRef != "" {
		args = append(args, subjectRef)
		sql += ` AND subject_ref=$` + itoa(len(args))
	}
	if variantRef := strings.TrimSpace(in.VariantRef); variantRef != "" {
		args = append(args, variantRef)
		sql += ` AND variant_ref=$` + itoa(len(args))
	}
	if productRef := strings.TrimSpace(in.ProductRef); productRef != "" {
		args = append(args, productRef)
		sql += ` AND product_ref=$` + itoa(len(args))
	}
	args = append(args, limit+1, offset)
	sql += ` ORDER BY recorded_at DESC, id DESC LIMIT $` + itoa(len(args)-1) + ` OFFSET $` + itoa(len(args))

	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var versions []PriceVersion
	for rows.Next() {
		var v PriceVersion
		if err = rows.Scan(&v.ID, &v.BusinessID, &v.BranchID, &v.SubjectType, &v.SubjectRef, &v.ProductRef,
			&v.VariantRef, &v.Currency, &v.PreviousAmount, &v.Amount, &v.PreviousInherited, &v.Inherited,
			&v.ChangeReason, &v.SubjectVersion, &v.Revision, &v.ActorType, &v.ActorID, &v.CorrelationID,
			&v.CommandRef, &v.RecordedAt, &v.EffectiveAt); err != nil {
			return nil, false, err
		}
		versions = append(versions, v)
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(versions) > limit
	if hasMore {
		versions = versions[:limit]
	}
	return versions, hasMore, nil
}
