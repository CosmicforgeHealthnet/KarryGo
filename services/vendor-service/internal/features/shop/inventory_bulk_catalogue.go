package shop

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

// bulkCatalogueRow is a bounded structured alternative to file ingestion.
// IDs are explicit import keys, making dependent rows deterministic and the
// whole batch replayable through the job fingerprint.
type bulkCatalogueRow struct {
	EntityType          string   `json:"entity_type"`
	ID                  string   `json:"id"`
	ParentID            string   `json:"parent_id"`
	CategoryID          string   `json:"category_id"`
	ProductID           string   `json:"product_id"`
	VariantID           string   `json:"variant_id"`
	MeasureID           string   `json:"measure_id"`
	SellUnitID          string   `json:"sell_unit_id"`
	ListingID           string   `json:"listing_id"`
	BranchID            string   `json:"branch_id"`
	Name                string   `json:"name"`
	Slug                string   `json:"slug"`
	ProductClass        string   `json:"product_class"`
	PackageLevel        string   `json:"package_level"`
	MeasurementMode     string   `json:"measurement_mode"`
	DisplayName         string   `json:"display_name"`
	IdentifierType      string   `json:"identifier_type"`
	IdentifierValue     string   `json:"identifier_value"`
	OperatingModel      string   `json:"operating_model"`
	OperatingModels     []string `json:"operating_models"`
	SellUnitIDs         []string `json:"sell_unit_ids"`
	Status              string   `json:"status"`
	FractionNumerator   int      `json:"fraction_numerator"`
	FractionDenominator int      `json:"fraction_denominator"`
	PackQuantity        int      `json:"pack_quantity"`
	MinQuantity         int64    `json:"min_quantity"`
	MaxQuantity         *int64   `json:"max_quantity"`
	UnitPriceMinor      *int64   `json:"unit_price_minor"`
	RetailPriceMinor    *int64   `json:"retail_price_minor"`
	WholesalePriceMinor *int64   `json:"wholesale_price_minor"`
	RetailEnabled       *bool    `json:"retail_enabled"`
	WholesaleEnabled    *bool    `json:"wholesale_enabled"`
}

func validateBulkImportID(value string) error {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 200 || strings.ContainsAny(value, " \t\r\n") {
		return fieldError("rows.id", "Every catalogue row needs a stable non-whitespace id of at most 200 characters.")
	}
	return nil
}

func parseBulkCatalogueRows(payloads [][]byte) ([]bulkCatalogueRow, error) {
	rows := make([]bulkCatalogueRow, len(payloads))
	for i, payload := range payloads {
		if err := json.Unmarshal(payload, &rows[i]); err != nil {
			return nil, fieldError("rows", "Invalid catalogue import row.")
		}
		rows[i].EntityType = strings.ToUpper(strings.TrimSpace(rows[i].EntityType))
		if !contains([]string{"CATEGORY", "PRODUCT", "VARIANT", "SELL_UNIT", "BARCODE", "LISTING", "QUANTITY_BREAK"}, rows[i].EntityType) {
			return nil, fieldError("rows.entity_type", "Use CATEGORY, PRODUCT, VARIANT, SELL_UNIT, BARCODE, LISTING, or QUANTITY_BREAK.")
		}
		if err := validateBulkImportID(rows[i].ID); err != nil {
			return nil, err
		}
	}
	rank := map[string]int{"CATEGORY": 0, "PRODUCT": 1, "VARIANT": 2, "SELL_UNIT": 3, "BARCODE": 4, "LISTING": 5, "QUANTITY_BREAK": 6}
	sort.SliceStable(rows, func(i, j int) bool { return rank[rows[i].EntityType] < rank[rows[j].EntityType] })
	return rows, nil
}

func (s *Service) applyBulkCatalogueRows(ctx context.Context, tx pgx.Tx, actor Actor, scope Scope, payloads [][]byte) error {
	if err := requireBusinessWide(scope, "Catalogue imports"); err != nil {
		return err
	}
	rows, err := parseBulkCatalogueRows(payloads)
	if err != nil {
		return err
	}
	currency, err := s.businessCurrency(ctx, tx, actor.VendorID)
	if err != nil {
		return err
	}
	for _, row := range rows {
		switch row.EntityType {
		case "CATEGORY":
			name := strings.TrimSpace(row.Name)
			if len(name) < 2 || len(name) > 120 {
				return fieldError("rows.name", "Category names must contain 2 to 120 characters.")
			}
			slug := strings.TrimSpace(row.Slug)
			if slug == "" {
				slug = slugify(name)
			}
			depth := 0
			if row.ParentID != "" {
				if err = tx.QueryRow(ctx, `SELECT depth+1 FROM shop_categories WHERE id=$1 AND business_id=$2 AND status='ACTIVE'`, row.ParentID, actor.VendorID).Scan(&depth); err != nil {
					return mapDB(err, "Parent category", CodeCategoryNotFound)
				}
			}
			if depth > 5 {
				return fieldError("rows.parent_id", "Category depth cannot exceed five.")
			}
			if _, err = tx.Exec(ctx, `INSERT INTO shop_categories(id,business_id,parent_id,name,slug,depth,status) VALUES($1,$2,NULLIF($3,''),$4,$5,$6,'ACTIVE')`, row.ID, actor.VendorID, row.ParentID, name, slug, depth); err != nil {
				return mapDB(err, "Category import row", CodeCategoryNotFound)
			}
		case "PRODUCT":
			class := strings.ToUpper(strings.TrimSpace(row.ProductClass))
			if !IsProductClass(class) {
				return fieldError("rows.product_class", "Unknown product class.")
			}
			name := strings.TrimSpace(row.Name)
			if len(name) < 2 || len(name) > 200 {
				return fieldError("rows.name", "Product names must contain 2 to 200 characters.")
			}
			status := strings.ToUpper(strings.TrimSpace(row.Status))
			if status == "" {
				status = StatusActive
			}
			if !contains([]string{StatusDraft, StatusActive, StatusPaused}, status) {
				return fieldError("rows.status", "Invalid product status.")
			}
			if _, err = tx.Exec(ctx, `INSERT INTO shop_products(id,business_id,category_id,name,product_class,status) VALUES($1,$2,$3,$4,$5,$6)`, row.ID, actor.VendorID, row.CategoryID, name, class, status); err != nil {
				return mapDB(err, "Product import row", CodeProductNotFound)
			}
			for _, model := range row.OperatingModels {
				model = strings.ToUpper(strings.TrimSpace(model))
				if !IsOperatingModel(model) {
					return fieldError("rows.operating_models", "Unknown operating model.")
				}
				if _, err = tx.Exec(ctx, `INSERT INTO shop_product_operating_models(id,business_id,product_id,operating_model) VALUES($1,$2,$3,$4)`, newID("spom_"), actor.VendorID, row.ID, model); err != nil {
					return err
				}
			}
		case "VARIANT":
			packageLevel := strings.ToUpper(strings.TrimSpace(row.PackageLevel))
			if packageLevel == "" {
				packageLevel = "EACH"
			}
			packQuantity := row.PackQuantity
			if packQuantity == 0 {
				packQuantity = 1
			}
			name := strings.TrimSpace(row.Name)
			if name == "" || len(name) > 200 {
				return fieldError("rows.name", "Variant names must contain 1 to 200 characters.")
			}
			if _, err = tx.Exec(ctx, `INSERT INTO shop_variants(id,business_id,product_id,name,package_level,pack_quantity,retail_price_minor,wholesale_price_minor,status) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'ACTIVE')`, row.ID, actor.VendorID, row.ProductID, name, packageLevel, packQuantity, row.RetailPriceMinor, row.WholesalePriceMinor); err != nil {
				return mapDB(err, "Variant import row", CodeVariantNotFound)
			}
			if row.RetailPriceMinor != nil {
				if err = s.appendPriceVersion(ctx, tx, actor, currency, priceChange{subjectType: "VARIANT_RETAIL", subjectRef: row.ID, productRef: row.ProductID, variantRef: row.ID, amount: row.RetailPriceMinor, reason: ReasonBaseline, subjectVersion: 1}); err != nil {
					return err
				}
			}
			if row.WholesalePriceMinor != nil {
				if err = s.appendPriceVersion(ctx, tx, actor, currency, priceChange{subjectType: "VARIANT_WHOLESALE", subjectRef: row.ID, productRef: row.ProductID, variantRef: row.ID, amount: row.WholesalePriceMinor, reason: ReasonBaseline, subjectVersion: 1}); err != nil {
					return err
				}
			}
		case "SELL_UNIT":
			var owner, family string
			var base *int64
			if err = tx.QueryRow(ctx, `SELECT owner_key,measurement_family,base_quantity FROM shop_measure_definitions WHERE id=$1 AND status='ACTIVE' AND owner_key IN ('PLATFORM',$2)`, row.MeasureID, actor.VendorID).Scan(&owner, &family, &base); err != nil {
				return mapDB(err, "Measure", CodeMeasureNotFound)
			}
			numerator, denominator := row.FractionNumerator, row.FractionDenominator
			if numerator == 0 {
				numerator = 1
			}
			if denominator == 0 {
				denominator = 1
			}
			conversion := "NONE"
			var exactFamily *string
			var exactQuantity *int64
			if base != nil && (*base*int64(numerator))%int64(denominator) == 0 {
				conversion = "EXACT"
				familyCopy := family
				quantity := *base * int64(numerator) / int64(denominator)
				exactFamily, exactQuantity = &familyCopy, &quantity
			}
			mode := strings.ToUpper(strings.TrimSpace(row.MeasurementMode))
			if mode == "" {
				mode = ModeFixed
			}
			display := strings.TrimSpace(row.DisplayName)
			if display == "" {
				display = strings.TrimSpace(row.Name)
			}
			if display == "" || !contains(MeasurementModes, mode) {
				return fieldError("rows.measurement_mode", "Sell units need a display name and supported measurement mode.")
			}
			actualRequired := mode == ModeVariableWeight || mode == ModeVariableCount || mode == ModeLocalContainer || mode == ModePricePortion
			if _, err = tx.Exec(ctx, `INSERT INTO shop_sell_units(id,business_id,product_id,variant_id,measure_definition_id,measure_owner_key,fraction_numerator,fraction_denominator,display_name,measurement_mode,conversion_type,exact_base_family,exact_base_quantity,actual_measurement_required,price_minor,wholesale_price_minor,retail_enabled,wholesale_enabled,status)
				VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,COALESCE($17,true),COALESCE($18,false),'ACTIVE')`, row.ID, actor.VendorID, row.ProductID, row.VariantID, row.MeasureID, owner, numerator, denominator, display, mode, conversion, exactFamily, exactQuantity, actualRequired, row.RetailPriceMinor, row.WholesalePriceMinor, row.RetailEnabled, row.WholesaleEnabled); err != nil {
				return mapDB(err, "Sell unit import row", CodeSellUnitNotFound)
			}
			if row.RetailPriceMinor != nil {
				if err = s.appendPriceVersion(ctx, tx, actor, currency, priceChange{subjectType: "SELL_UNIT_RETAIL", subjectRef: row.ID, productRef: row.ProductID, variantRef: row.VariantID, amount: row.RetailPriceMinor, reason: ReasonBaseline, subjectVersion: 1}); err != nil {
					return err
				}
			}
		case "BARCODE":
			identifierType := strings.ToUpper(strings.TrimSpace(row.IdentifierType))
			if identifierType == "" {
				identifierType = "BARCODE"
			}
			if _, err = tx.Exec(ctx, `INSERT INTO shop_product_identifiers(id,business_id,product_id,variant_id,identifier_type,identifier_value,status) VALUES($1,$2,$3,$4,$5,$6,'ACTIVE')`, row.ID, actor.VendorID, row.ProductID, row.VariantID, identifierType, strings.TrimSpace(row.IdentifierValue)); err != nil {
				return mapDB(err, "Barcode import row", CodeIdentifierNotFound)
			}
		case "LISTING":
			if err = s.requireBranchScope(scope, row.BranchID); err != nil {
				return err
			}
			var productID string
			if err = tx.QueryRow(ctx, `SELECT product_id FROM shop_variants WHERE id=$1 AND business_id=$2 AND status='ACTIVE'`, row.VariantID, actor.VendorID).Scan(&productID); err != nil {
				return mapDB(err, "Variant", CodeVariantNotFound)
			}
			model := strings.ToUpper(strings.TrimSpace(row.OperatingModel))
			if model != "" && !IsOperatingModel(model) {
				return fieldError("rows.operating_model", "Unknown listing operating model.")
			}
			if _, err = tx.Exec(ctx, `INSERT INTO shop_branch_listings(id,business_id,branch_id,product_id,variant_id,operating_model,retail_enabled,wholesale_enabled,status) VALUES($1,$2,$3,$4,$5,NULLIF($6,''),COALESCE($7,true),COALESCE($8,false),'ACTIVE')`, row.ID, actor.VendorID, row.BranchID, productID, row.VariantID, model, row.RetailEnabled, row.WholesaleEnabled); err != nil {
				return mapDB(err, "Listing import row", CodeListingNotFound)
			}
			for index, sellUnitID := range row.SellUnitIDs {
				if _, err = tx.Exec(ctx, `INSERT INTO shop_listing_sell_units(id,business_id,listing_id,sell_unit_id,variant_id,position) VALUES($1,$2,$3,$4,$5,$6)`, newID("slsu_"), actor.VendorID, row.ID, sellUnitID, row.VariantID, index); err != nil {
					return mapDB(err, "Listing sell unit", CodeSellUnitNotFound)
				}
			}
			if _, err = bumpBranchRevision(ctx, tx, actor.VendorID, row.BranchID); err != nil {
				return err
			}
		case "QUANTITY_BREAK":
			if row.MinQuantity <= 0 || row.UnitPriceMinor == nil {
				return fieldError("rows.min_quantity", "Quantity breaks need a positive minimum and unit price.")
			}
			var branchID, productID, variantID string
			if err = tx.QueryRow(ctx, `SELECT branch_id,product_id,variant_id FROM shop_branch_listings WHERE id=$1 AND business_id=$2`, row.ListingID, actor.VendorID).Scan(&branchID, &productID, &variantID); err != nil {
				return mapDB(err, "Listing", CodeListingNotFound)
			}
			if _, err = tx.Exec(ctx, `INSERT INTO shop_quantity_breaks(id,business_id,listing_id,sell_unit_id,min_quantity,max_quantity,unit_price_minor) VALUES($1,$2,$3,NULLIF($4,''),$5,$6,$7)`, row.ID, actor.VendorID, row.ListingID, row.SellUnitID, row.MinQuantity, row.MaxQuantity, row.UnitPriceMinor); err != nil {
				return mapDB(err, "Quantity break import row", CodeQuantityBreakNotFound)
			}
			if err = s.appendPriceVersion(ctx, tx, actor, currency, priceChange{subjectType: "QUANTITY_BREAK", subjectRef: row.ID, branchID: branchID, productRef: productID, variantRef: variantID, amount: row.UnitPriceMinor, reason: ReasonBaseline, subjectVersion: 1}); err != nil {
				return err
			}
			if _, err = bumpBranchRevision(ctx, tx, actor.VendorID, branchID); err != nil {
				return err
			}
		}
	}
	return nil
}
