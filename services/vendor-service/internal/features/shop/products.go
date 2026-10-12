package shop

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

// ---------------------------------------------------------------------------
// Product master
// ---------------------------------------------------------------------------

const productCols = `id,business_id,category_id,name,COALESCE(description,''),COALESCE(brand,''),
	COALESCE(manufacturer,''),COALESCE(origin_country,''),product_class,
	COALESCE(regulatory_reference,''),COALESCE(manufacturer_code,''),status,
	created_at,updated_at,archived_at,version`

func scanProduct(row rowScanner) (Product, error) {
	var p Product
	err := row.Scan(&p.ID, &p.BusinessID, &p.CategoryID, &p.Name, &p.Description, &p.Brand,
		&p.Manufacturer, &p.OriginCountry, &p.ProductClass, &p.RegulatoryReference,
		&p.ManufacturerCode, &p.Status, &p.CreatedAt, &p.UpdatedAt, &p.ArchivedAt, &p.Version)
	return p, err
}

func loadProduct(ctx context.Context, q querier, businessID, productID string, lock bool) (Product, error) {
	sql := `SELECT ` + productCols + ` FROM shop_products WHERE business_id=$1 AND id=$2`
	if lock {
		sql += ` FOR UPDATE`
	}
	product, err := scanProduct(q.QueryRow(ctx, sql, businessID, strings.TrimSpace(productID)))
	if err != nil {
		return Product{}, mapDB(err, "Product", CodeProductNotFound)
	}
	return product, nil
}

// hydrateProduct attaches the product's ordered media and operating models.
func hydrateProduct(ctx context.Context, q querier, product *Product) error {
	rows, err := q.Query(ctx, `SELECT media_ref FROM shop_product_media
		WHERE product_id=$1 ORDER BY position, id`, product.ID)
	if err != nil {
		return err
	}
	product.MediaRefs = []string{}
	for rows.Next() {
		var ref string
		if err = rows.Scan(&ref); err != nil {
			rows.Close()
			return err
		}
		product.MediaRefs = append(product.MediaRefs, ref)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	modelRows, err := q.Query(ctx, `SELECT operating_model FROM shop_product_operating_models
		WHERE product_id=$1 ORDER BY operating_model`, product.ID)
	if err != nil {
		return err
	}
	defer modelRows.Close()
	product.OperatingModels = []string{}
	for modelRows.Next() {
		var model string
		if err = modelRows.Scan(&model); err != nil {
			return err
		}
		product.OperatingModels = append(product.OperatingModels, model)
	}
	return modelRows.Err()
}

// hydrateProducts loads media and operating-model children for a whole page
// in two bounded queries. ListProducts must not turn a 100-row page into 201
// round trips.
func hydrateProducts(ctx context.Context, q querier, products []Product) error {
	if len(products) == 0 {
		return nil
	}
	byID := make(map[string]*Product, len(products))
	ids := make([]string, 0, len(products))
	for index := range products {
		products[index].MediaRefs = []string{}
		products[index].OperatingModels = []string{}
		byID[products[index].ID] = &products[index]
		ids = append(ids, products[index].ID)
	}
	mediaRows, err := q.Query(ctx, `SELECT product_id,media_ref FROM shop_product_media
		WHERE product_id=ANY($1) ORDER BY product_id,position,id`, ids)
	if err != nil {
		return err
	}
	for mediaRows.Next() {
		var productID, mediaRef string
		if err = mediaRows.Scan(&productID, &mediaRef); err != nil {
			mediaRows.Close()
			return err
		}
		if product := byID[productID]; product != nil {
			product.MediaRefs = append(product.MediaRefs, mediaRef)
		}
	}
	mediaRows.Close()
	if err = mediaRows.Err(); err != nil {
		return err
	}
	modelRows, err := q.Query(ctx, `SELECT product_id,operating_model FROM shop_product_operating_models
		WHERE product_id=ANY($1) ORDER BY product_id,operating_model`, ids)
	if err != nil {
		return err
	}
	defer modelRows.Close()
	for modelRows.Next() {
		var productID, model string
		if err = modelRows.Scan(&productID, &model); err != nil {
			return err
		}
		if product := byID[productID]; product != nil {
			product.OperatingModels = append(product.OperatingModels, model)
		}
	}
	return modelRows.Err()
}

// CreateProductInput is the product-create command.
type CreateProductInput struct {
	CategoryID          string
	Name                string
	Description         string
	Brand               string
	Manufacturer        string
	OriginCountry       string
	ProductClass        string
	RegulatoryReference string
	ManufacturerCode    string
	OperatingModels     []string
	MediaRefs           []string
}

// CreateProduct adds a Shop product.
//
// Brand, manufacturer, origin and any identifier are optional. A market
// trader's "Fresh Tomatoes" legitimately has none of them, and requiring them
// would make the system unusable for exactly the sellers it is meant to serve.
func (s *Service) CreateProduct(ctx context.Context, actor Actor, scope Scope, in CreateProductInput) (Product, error) {
	if err := s.requireWrites(); err != nil {
		return Product{}, err
	}
	if err := requireBusinessWide(scope, "Shop products"); err != nil {
		return Product{}, err
	}
	name, violation := validateName("name", in.Name, 2, 200)
	if violation != nil {
		return Product{}, validation(*violation)
	}
	description, violation := validateOptionalText("description", in.Description, 4000)
	if violation != nil {
		return Product{}, validation(*violation)
	}
	brand, violation := validateOptionalText("brand", in.Brand, 120)
	if violation != nil {
		return Product{}, validation(*violation)
	}
	manufacturer, violation := validateOptionalText("manufacturer", in.Manufacturer, 160)
	if violation != nil {
		return Product{}, validation(*violation)
	}
	regulatory, violation := validateOptionalText("regulatory_reference", in.RegulatoryReference, 80)
	if violation != nil {
		return Product{}, validation(*violation)
	}
	manufacturerCode, violation := validateOptionalText("manufacturer_code", in.ManufacturerCode, 80)
	if violation != nil {
		return Product{}, validation(*violation)
	}
	if err := validatePharmacyMedicineExclusion("product", name, description, brand, manufacturer, regulatory, manufacturerCode); err != nil {
		return Product{}, err
	}
	origin := strings.ToUpper(strings.TrimSpace(in.OriginCountry))
	if origin != "" && !countryPattern.MatchString(origin) {
		return Product{}, fieldError("origin_country", "Use a two-letter ISO country code.")
	}
	productClass := strings.ToUpper(strings.TrimSpace(in.ProductClass))
	if !IsProductClass(productClass) {
		return Product{}, fieldError("product_class", "Choose a supported product class.")
	}
	models, err := normalizeOperatingModels(in.OperatingModels)
	if err != nil {
		return Product{}, err
	}
	mediaRefs, err := s.validateMediaRefs(ctx, actor.VendorID, in.MediaRefs)
	if err != nil {
		return Product{}, err
	}

	var created Product
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		category, err := loadCategory(ctx, tx, actor.VendorID, in.CategoryID, false)
		if err != nil {
			return err
		}
		if category.Status == StatusArchived {
			return errArchived("The category")
		}
		row, err := scanProduct(tx.QueryRow(ctx, `
			INSERT INTO shop_products
				(id,business_id,category_id,name,description,brand,manufacturer,origin_country,
				 product_class,regulatory_reference,manufacturer_code,status)
			VALUES ($1,$2,$3,$4,NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),$9,
			        NULLIF($10,''),NULLIF($11,''),'DRAFT')
			RETURNING `+productCols,
			newID("spr_"), actor.VendorID, category.ID, name, description, brand, manufacturer,
			origin, productClass, regulatory, manufacturerCode))
		if err != nil {
			return mapDB(err, "Product", CodeProductNotFound)
		}
		created = row
		if err = replaceProductModels(ctx, tx, actor.VendorID, created.ID, models); err != nil {
			return err
		}
		if err = replaceProductMedia(ctx, tx, actor.VendorID, created.ID, mediaRefs); err != nil {
			return err
		}
		if err = hydrateProduct(ctx, tx, &created); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "shop.product.created", eventType: "shop.product.changed",
			aggregateType: "shop_product", aggregateID: created.ID, version: created.Version,
			after: ProductViewOf(created), payload: ProductViewOf(created),
		})
	})
	return created, err
}

func normalizeOperatingModels(raw []string, target ...string) ([]string, error) {
	if len(raw) > MaxOperatingModels {
		return nil, fieldError("operating_models", "Supply at most "+itoa(MaxOperatingModels)+" operating models.")
	}
	seen := map[string]bool{}
	models := []string{}
	for _, entry := range raw {
		value := strings.ToUpper(strings.TrimSpace(entry))
		if value == "" {
			continue
		}
		if !IsOperatingModel(value) {
			return nil, coded(apperrors.Validation("Check your details.", []apperrors.FieldViolation{
				{Field: "operating_models", Message: "Choose supported operating models."}}), CodeOperatingModelInvalid)
		}
		if seen[value] {
			continue
		}
		seen[value] = true
		models = append(models, value)
	}
	return models, nil
}

func replaceProductModels(ctx context.Context, tx pgx.Tx, businessID, productID string, models []string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM shop_product_operating_models WHERE product_id=$1`, productID); err != nil {
		return err
	}
	for _, model := range models {
		if _, err := tx.Exec(ctx, `
			INSERT INTO shop_product_operating_models (id,business_id,product_id,operating_model)
			VALUES ($1,$2,$3,$4)`, newID("spm_"), businessID, productID, model); err != nil {
			return mapDB(err, "Product operating model", CodeOperatingModelInvalid)
		}
	}
	return nil
}

func replaceProductMedia(ctx context.Context, tx pgx.Tx, businessID, productID string, refs []string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM shop_product_media WHERE product_id=$1`, productID); err != nil {
		return err
	}
	for index, ref := range refs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO shop_product_media (id,business_id,product_id,media_ref,position)
			VALUES ($1,$2,$3,$4,$5)`, newID("spd_"), businessID, productID, ref, index); err != nil {
			return mapDB(err, "Product media", CodeMediaUnavailable)
		}
	}
	return nil
}

// validateMediaRefs proves every reference with the Media authority BEFORE the
// mutation transaction opens.
//
// No network call happens inside a locked transaction: a slow media read must
// never hold catalogue rows. A nil authority refuses attachment outright,
// because an unverified reference is worse than no image.
func (s *Service) validateMediaRefs(ctx context.Context, businessID string, raw []string, target ...string) ([]string, error) {
	refs := []string{}
	seen := map[string]bool{}
	for _, entry := range raw {
		ref := strings.TrimSpace(entry)
		if ref == "" || seen[ref] {
			continue
		}
		seen[ref] = true
		refs = append(refs, ref)
	}
	if len(refs) == 0 {
		return refs, nil
	}
	if len(refs) > MaxProductMedia {
		return nil, fieldError("media_refs", "Attach at most "+itoa(MaxProductMedia)+" media references.")
	}
	if s.media == nil {
		return nil, coded(apperrors.Unavailable("Media references cannot be verified right now.", nil),
			CodeMediaUnavailable)
	}
	for _, ref := range refs {
		object, err := s.media.GetObject(ctx, businessID, ref)
		if err != nil {
			return nil, coded(apperrors.Unavailable("Media references cannot be verified right now.", err),
				CodeMediaUnavailable)
		}
		if object.UploadPurpose == "PRODUCT_IMAGE" && (len(target) != 1 || object.OwnerService != "vendor-service" || object.OwnerReference != businessID || object.BindingType != "ASSET" || object.BindingID != "SHOP_PRODUCT:"+target[0] || object.Visibility != "PRIVATE" || object.SafetyStatus != "SAFE") {
			return nil, fieldError("media_refs", "Choose a safe image uploaded for this product.")
		}
		if object.UploadStatus != "COMPLETED" || object.DeletionState != "ACTIVE" {
			return nil, coded(apperrors.Validation("Check your details.", []apperrors.FieldViolation{
				{Field: "media_refs", Message: "One or more media references are not an active, completed upload."}}),
				CodeMediaUnavailable)
		}
	}
	return refs, nil
}

// ProductListOptions bounds a catalogue read.
type ProductListOptions struct {
	CategoryID      string
	ProductClass    string
	Brand           string
	Status          string
	Search          string
	IncludeArchived bool
	Limit, Offset   int
}

// ListProducts reads the business's Shop products.
//
// The Shop/Food boundary is a query-scope fact, not a filter applied
// afterwards: this reads shop_products and nothing else, so a cooked
// restaurant dish cannot appear here however similar its name.
func (s *Service) ListProducts(ctx context.Context, actor Actor, scope Scope, in ProductListOptions) ([]Product, bool, error) {
	if scope.Empty() {
		return nil, false, errForbidden()
	}
	limit, offset := boundedPage(in.Limit, in.Offset)
	sql := `SELECT ` + productCols + ` FROM shop_products WHERE business_id=$1`
	args := []any{actor.VendorID}
	if categoryID := strings.TrimSpace(in.CategoryID); categoryID != "" {
		args = append(args, categoryID)
		sql += ` AND category_id=$` + itoa(len(args))
	}
	if productClass := strings.ToUpper(strings.TrimSpace(in.ProductClass)); productClass != "" {
		if !IsProductClass(productClass) {
			return nil, false, fieldError("product_class", "Choose a supported product class.")
		}
		args = append(args, productClass)
		sql += ` AND product_class=$` + itoa(len(args))
	}
	if brand := strings.TrimSpace(in.Brand); brand != "" {
		args = append(args, strings.ToLower(brand))
		sql += ` AND lower(brand)=$` + itoa(len(args))
	}
	if status := strings.ToUpper(strings.TrimSpace(in.Status)); status != "" {
		args = append(args, status)
		sql += ` AND status=$` + itoa(len(args))
	} else if !in.IncludeArchived {
		sql += ` AND status <> 'ARCHIVED'`
	}
	if search := strings.TrimSpace(in.Search); search != "" {
		args = append(args, "%"+strings.ToLower(search)+"%")
		sql += ` AND lower(name) LIKE $` + itoa(len(args))
	}
	args = append(args, limit+1, offset)
	sql += ` ORDER BY created_at DESC, id LIMIT $` + itoa(len(args)-1) + ` OFFSET $` + itoa(len(args))

	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, false, err
	}
	var products []Product
	for rows.Next() {
		product, scanErr := scanProduct(rows)
		if scanErr != nil {
			rows.Close()
			return nil, false, scanErr
		}
		products = append(products, product)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(products) > limit
	if hasMore {
		products = products[:limit]
	}
	if err = hydrateProducts(ctx, s.db, products); err != nil {
		return nil, false, err
	}
	return products, hasMore, nil
}

// GetProduct reads one product with its media and operating models.
func (s *Service) GetProduct(ctx context.Context, actor Actor, scope Scope, productID string) (Product, error) {
	if scope.Empty() {
		return Product{}, errForbidden()
	}
	product, err := loadProduct(ctx, s.db, actor.VendorID, productID, false)
	if err != nil {
		return Product{}, err
	}
	return product, hydrateProduct(ctx, s.db, &product)
}

// UpdateProductInput edits the product master.
type UpdateProductInput struct {
	CategoryID          *string
	Name                *string
	Description         *string
	Brand               *string
	Manufacturer        *string
	OriginCountry       *string
	ProductClass        *string
	RegulatoryReference *string
	ManufacturerCode    *string
	OperatingModels     *[]string
	MediaRefs           *[]string
	Status              *string
	ExpectedVersion     int64
}

// UpdateProduct edits a product.
func (s *Service) UpdateProduct(ctx context.Context, actor Actor, scope Scope, productID string, in UpdateProductInput) (Product, error) {
	if err := s.requireWrites(); err != nil {
		return Product{}, err
	}
	if err := requireBusinessWide(scope, "Shop products"); err != nil {
		return Product{}, err
	}
	var mediaRefs []string
	if in.MediaRefs != nil {
		validated, err := s.validateMediaRefs(ctx, actor.VendorID, *in.MediaRefs, productID)
		if err != nil {
			return Product{}, err
		}
		mediaRefs = validated
	}
	var models []string
	if in.OperatingModels != nil {
		normalized, err := normalizeOperatingModels(*in.OperatingModels)
		if err != nil {
			return Product{}, err
		}
		models = normalized
	}

	var updated Product
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		current, err := loadProduct(ctx, tx, actor.VendorID, productID, true)
		if err != nil {
			return err
		}
		if err = expectedVersionMatches(current.Version, in.ExpectedVersion); err != nil {
			return err
		}
		if current.Status == StatusArchived {
			return errArchived("This product")
		}
		if err = hydrateProduct(ctx, tx, &current); err != nil {
			return err
		}

		categoryID, name, description := current.CategoryID, current.Name, current.Description
		brand, manufacturer, origin := current.Brand, current.Manufacturer, current.OriginCountry
		productClass, regulatory := current.ProductClass, current.RegulatoryReference
		manufacturerCode, status := current.ManufacturerCode, current.Status

		if in.CategoryID != nil {
			category, categoryErr := loadCategory(ctx, tx, actor.VendorID, *in.CategoryID, false)
			if categoryErr != nil {
				return categoryErr
			}
			if category.Status == StatusArchived {
				return errArchived("The category")
			}
			categoryID = category.ID
		}
		if in.Name != nil {
			value, violation := validateName("name", *in.Name, 2, 200)
			if violation != nil {
				return validation(*violation)
			}
			name = value
		}
		if in.Description != nil {
			value, violation := validateOptionalText("description", *in.Description, 4000)
			if violation != nil {
				return validation(*violation)
			}
			description = value
		}
		if in.Brand != nil {
			value, violation := validateOptionalText("brand", *in.Brand, 120)
			if violation != nil {
				return validation(*violation)
			}
			brand = value
		}
		if in.Manufacturer != nil {
			value, violation := validateOptionalText("manufacturer", *in.Manufacturer, 160)
			if violation != nil {
				return validation(*violation)
			}
			manufacturer = value
		}
		if in.OriginCountry != nil {
			value := strings.ToUpper(strings.TrimSpace(*in.OriginCountry))
			if value != "" && !countryPattern.MatchString(value) {
				return fieldError("origin_country", "Use a two-letter ISO country code.")
			}
			origin = value
		}
		if in.ProductClass != nil {
			value := strings.ToUpper(strings.TrimSpace(*in.ProductClass))
			if !IsProductClass(value) {
				return fieldError("product_class", "Choose a supported product class.")
			}
			// Changing the class changes which fresh attributes are legal, so
			// a class change with incompatible attributes attached is refused
			// rather than silently leaving a fish's descaling option on rice.
			if value != current.ProductClass {
				if err = ensureAttributesFitClass(ctx, tx, actor.VendorID, current.ID, value); err != nil {
					return err
				}
			}
			productClass = value
		}
		if in.RegulatoryReference != nil {
			value, violation := validateOptionalText("regulatory_reference", *in.RegulatoryReference, 80)
			if violation != nil {
				return validation(*violation)
			}
			regulatory = value
		}
		if in.ManufacturerCode != nil {
			value, violation := validateOptionalText("manufacturer_code", *in.ManufacturerCode, 80)
			if violation != nil {
				return validation(*violation)
			}
			manufacturerCode = value
		}
		if in.Status != nil {
			value := strings.ToUpper(strings.TrimSpace(*in.Status))
			if value != StatusDraft && value != StatusActive && value != StatusPaused {
				return fieldError("status", "Use DRAFT, ACTIVE or PAUSED. Archive with the archive command.")
			}
			status = value
		}
		if err = validatePharmacyMedicineExclusion("product", name, description, brand, manufacturer, regulatory, manufacturerCode); err != nil {
			return err
		}

		row, err := scanProduct(tx.QueryRow(ctx, `
			UPDATE shop_products
			SET category_id=$3, name=$4, description=NULLIF($5,''), brand=NULLIF($6,''),
			    manufacturer=NULLIF($7,''), origin_country=NULLIF($8,''), product_class=$9,
			    regulatory_reference=NULLIF($10,''), manufacturer_code=NULLIF($11,''), status=$12,
			    updated_at=now(), version=version+1
			WHERE business_id=$1 AND id=$2 RETURNING `+productCols,
			actor.VendorID, current.ID, categoryID, name, description, brand, manufacturer, origin,
			productClass, regulatory, manufacturerCode, status))
		if err != nil {
			return mapDB(err, "Product", CodeProductNotFound)
		}
		updated = row
		if in.OperatingModels != nil {
			if err = replaceProductModels(ctx, tx, actor.VendorID, updated.ID, models); err != nil {
				return err
			}
		}
		if in.MediaRefs != nil {
			if err = replaceProductMedia(ctx, tx, actor.VendorID, updated.ID, mediaRefs); err != nil {
				return err
			}
		}
		if err = hydrateProduct(ctx, tx, &updated); err != nil {
			return err
		}
		if _, err = bumpRevisionsForProduct(ctx, tx, actor.VendorID, updated.ID); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "shop.product.updated", eventType: "shop.product.changed",
			aggregateType: "shop_product", aggregateID: updated.ID, version: updated.Version,
			before: ProductViewOf(current), after: ProductViewOf(updated), payload: ProductViewOf(updated),
		})
	})
	return updated, err
}

// ArchiveProduct retires a product and deactivates its branch listings, so a
// retired product stops being sellable everywhere at once.
func (s *Service) ArchiveProduct(ctx context.Context, actor Actor, scope Scope, productID string, expectedVersion int64) (Product, error) {
	if err := s.requireWrites(); err != nil {
		return Product{}, err
	}
	if err := requireBusinessWide(scope, "Shop products"); err != nil {
		return Product{}, err
	}
	var archived Product
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		current, err := loadProduct(ctx, tx, actor.VendorID, productID, true)
		if err != nil {
			return err
		}
		if err = expectedVersionMatches(current.Version, expectedVersion); err != nil {
			return err
		}
		if current.Status == StatusArchived {
			archived = current
			return hydrateProduct(ctx, tx, &archived)
		}
		row, err := scanProduct(tx.QueryRow(ctx, `
			UPDATE shop_products SET status='ARCHIVED', archived_at=now(), updated_at=now(), version=version+1
			WHERE business_id=$1 AND id=$2 RETURNING `+productCols, actor.VendorID, current.ID))
		if err != nil {
			return mapDB(err, "Product", CodeProductNotFound)
		}
		archived = row
		if _, err = tx.Exec(ctx, `
			UPDATE shop_branch_listings SET status='INACTIVE', updated_at=now(), version=version+1
			WHERE business_id=$1 AND product_id=$2 AND status='ACTIVE'`, actor.VendorID, current.ID); err != nil {
			return err
		}
		if _, err = bumpRevisionsForProduct(ctx, tx, actor.VendorID, current.ID); err != nil {
			return err
		}
		if err = hydrateProduct(ctx, tx, &archived); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "shop.product.archived", eventType: "shop.product.changed",
			aggregateType: "shop_product", aggregateID: archived.ID, version: archived.Version,
			before: ProductViewOf(current), after: ProductViewOf(archived), payload: ProductViewOf(archived),
		})
	})
	return archived, err
}

// ---------------------------------------------------------------------------
// Variants
// ---------------------------------------------------------------------------

const variantCols = `id,business_id,product_id,COALESCE(sku,''),name,package_level,pack_quantity,
	net_weight_grams,net_volume_ml,length_mm,width_mm,height_mm,retail_price_minor,wholesale_price_minor,
	temperature_zone,COALESCE(consolidation_class,''),COALESCE(separation_group,''),stackable,large_load,
	status,position,created_at,updated_at,archived_at,version`

func scanVariant(row rowScanner) (Variant, error) {
	var v Variant
	err := row.Scan(&v.ID, &v.BusinessID, &v.ProductID, &v.SKU, &v.Name, &v.PackageLevel, &v.PackQuantity,
		&v.NetWeightGrams, &v.NetVolumeML, &v.LengthMM, &v.WidthMM, &v.HeightMM,
		&v.RetailPriceMinor, &v.WholesalePriceMinor, &v.TemperatureZone, &v.ConsolidationClass,
		&v.SeparationGroup, &v.Stackable, &v.LargeLoad, &v.Status, &v.Position,
		&v.CreatedAt, &v.UpdatedAt, &v.ArchivedAt, &v.Version)
	return v, err
}

func loadVariant(ctx context.Context, q querier, businessID, productID, variantID string, lock bool) (Variant, error) {
	sql := `SELECT ` + variantCols + ` FROM shop_variants WHERE business_id=$1 AND product_id=$2 AND id=$3`
	if lock {
		sql += ` FOR UPDATE`
	}
	variant, err := scanVariant(q.QueryRow(ctx, sql, businessID,
		strings.TrimSpace(productID), strings.TrimSpace(variantID)))
	if err != nil {
		return Variant{}, mapDB(err, "Variant", CodeVariantNotFound)
	}
	return variant, nil
}

// loadVariantByID reads a variant without knowing its product, still scoped to
// the tenant.
func loadVariantByID(ctx context.Context, q querier, businessID, variantID string, lock bool) (Variant, error) {
	sql := `SELECT ` + variantCols + ` FROM shop_variants WHERE business_id=$1 AND id=$2`
	if lock {
		sql += ` FOR UPDATE`
	}
	variant, err := scanVariant(q.QueryRow(ctx, sql, businessID, strings.TrimSpace(variantID)))
	if err != nil {
		return Variant{}, mapDB(err, "Variant", CodeVariantNotFound)
	}
	return variant, nil
}

func hydrateVariant(ctx context.Context, q querier, variant *Variant) error {
	rows, err := q.Query(ctx, `SELECT handling_flag FROM shop_variant_handling_flags
		WHERE variant_id=$1 ORDER BY handling_flag`, variant.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	variant.HandlingFlags = []string{}
	for rows.Next() {
		var flag string
		if err = rows.Scan(&flag); err != nil {
			return err
		}
		variant.HandlingFlags = append(variant.HandlingFlags, flag)
	}
	return rows.Err()
}

// hydrateVariants loads handling flags for a whole page in one query.
func hydrateVariants(ctx context.Context, q querier, variants []Variant) error {
	if len(variants) == 0 {
		return nil
	}
	byID := make(map[string]*Variant, len(variants))
	ids := make([]string, 0, len(variants))
	for index := range variants {
		variants[index].HandlingFlags = []string{}
		byID[variants[index].ID] = &variants[index]
		ids = append(ids, variants[index].ID)
	}
	rows, err := q.Query(ctx, `SELECT variant_id,handling_flag FROM shop_variant_handling_flags
		WHERE variant_id=ANY($1) ORDER BY variant_id,handling_flag`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var variantID, flag string
		if err = rows.Scan(&variantID, &flag); err != nil {
			return err
		}
		if variant := byID[variantID]; variant != nil {
			variant.HandlingFlags = append(variant.HandlingFlags, flag)
		}
	}
	return rows.Err()
}

// CreateVariantInput is the variant-create command.
type CreateVariantInput struct {
	SKU                 string
	Name                string
	PackageLevel        string
	PackQuantity        *int
	NetWeightGrams      *int64
	NetVolumeML         *int64
	LengthMM            *int
	WidthMM             *int
	HeightMM            *int
	RetailPriceMinor    *int64
	WholesalePriceMinor *int64
	TemperatureZone     string
	ConsolidationClass  string
	SeparationGroup     string
	Stackable           *bool
	LargeLoad           *bool
	HandlingFlags       []string
}

// CreateVariant adds a SKU to a product.
func (s *Service) CreateVariant(ctx context.Context, actor Actor, scope Scope, productID string, in CreateVariantInput) (Variant, error) {
	if err := s.requireWrites(); err != nil {
		return Variant{}, err
	}
	if err := requireBusinessWide(scope, "Shop variants"); err != nil {
		return Variant{}, err
	}
	name, violation := validateName("name", in.Name, 1, 200)
	if violation != nil {
		return Variant{}, validation(*violation)
	}
	sku := strings.TrimSpace(in.SKU)
	if sku != "" && !skuPattern.MatchString(sku) {
		return Variant{}, fieldError("sku", "Use up to 64 letters, digits, dot, dash, slash or underscore.")
	}
	packageLevel := strings.ToUpper(strings.TrimSpace(in.PackageLevel))
	if packageLevel == "" {
		packageLevel = "EACH"
	}
	if !IsPackageLevel(packageLevel) {
		return Variant{}, fieldError("package_level", "Choose a supported package level.")
	}
	physical, err := validatePhysical(in.NetWeightGrams, in.NetVolumeML, in.LengthMM, in.WidthMM, in.HeightMM)
	if err != nil {
		return Variant{}, err
	}
	if err = validateMoney("retail_price_minor", in.RetailPriceMinor); err != nil {
		return Variant{}, err
	}
	if err = validateMoney("wholesale_price_minor", in.WholesalePriceMinor); err != nil {
		return Variant{}, err
	}
	zone := strings.ToUpper(strings.TrimSpace(in.TemperatureZone))
	if zone == "" {
		zone = "AMBIENT"
	}
	if !contains(TemperatureZones, zone) {
		return Variant{}, fieldError("temperature_zone", "Choose a supported temperature zone.")
	}
	consolidation := strings.ToUpper(strings.TrimSpace(in.ConsolidationClass))
	if consolidation != "" && !contains(ConsolidationClasses, consolidation) {
		return Variant{}, fieldError("consolidation_class", "Choose a supported consolidation class.")
	}
	separation, violation := validateOptionalText("separation_group", in.SeparationGroup, 60)
	if violation != nil {
		return Variant{}, validation(*violation)
	}
	flags, err := normalizeHandlingFlags(in.HandlingFlags)
	if err != nil {
		return Variant{}, err
	}
	packQuantity := 1
	if in.PackQuantity != nil {
		if *in.PackQuantity < 1 || *in.PackQuantity > 100000 {
			return Variant{}, fieldError("pack_quantity", "Pack quantity must be between 1 and 100000.")
		}
		packQuantity = *in.PackQuantity
	}

	// Establishing a price — including an explicit zero — is price authority,
	// checked per field so creating a SKU with no price needs none.
	if in.RetailPriceMinor != nil {
		if err = s.requirePricingAuthority(ctx, actor, ""); err != nil {
			return Variant{}, err
		}
	}
	if in.WholesalePriceMinor != nil {
		if err = s.requireWholesaleAuthority(ctx, actor, ""); err != nil {
			return Variant{}, err
		}
	}

	var created Variant
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		product, err := loadProduct(ctx, tx, actor.VendorID, productID, true)
		if err != nil {
			return err
		}
		if product.Status == StatusArchived {
			return errArchived("This product")
		}
		var position int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM shop_variants WHERE product_id=$1`,
			product.ID).Scan(&position); err != nil {
			return err
		}
		row, err := scanVariant(tx.QueryRow(ctx, `
			INSERT INTO shop_variants
				(id,business_id,product_id,sku,name,package_level,pack_quantity,net_weight_grams,net_volume_ml,
				 length_mm,width_mm,height_mm,retail_price_minor,wholesale_price_minor,temperature_zone,
				 consolidation_class,separation_group,stackable,large_load,status,position)
			VALUES ($1,$2,$3,NULLIF($4,''),$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,NULLIF($16,''),NULLIF($17,''),
			        $18,$19,'ACTIVE',$20)
			RETURNING `+variantCols,
			newID("svr_"), actor.VendorID, product.ID, sku, name, packageLevel, packQuantity,
			physical.weight, physical.volume, physical.length, physical.width, physical.height,
			in.RetailPriceMinor, in.WholesalePriceMinor, zone, consolidation, separation,
			boolOrDefault(in.Stackable, true), boolOrDefault(in.LargeLoad, false), position))
		if err != nil {
			return mapDB(err, "Variant", CodeVariantNotFound)
		}
		created = row
		if err = replaceHandlingFlags(ctx, tx, actor.VendorID, created.ID, flags); err != nil {
			return err
		}
		if err = hydrateVariant(ctx, tx, &created); err != nil {
			return err
		}
		currency, err := s.businessCurrency(ctx, tx, actor.VendorID)
		if err != nil {
			return err
		}
		if in.RetailPriceMinor != nil {
			if err = s.appendPriceVersion(ctx, tx, actor, currency, priceChange{
				subjectType: SubjectVariantRetail, subjectRef: created.ID, productRef: product.ID,
				variantRef: created.ID, amount: in.RetailPriceMinor, reason: ReasonCreated,
				subjectVersion: created.Version}); err != nil {
				return err
			}
		}
		if in.WholesalePriceMinor != nil {
			if err = s.appendPriceVersion(ctx, tx, actor, currency, priceChange{
				subjectType: SubjectVariantWholesale, subjectRef: created.ID, productRef: product.ID,
				variantRef: created.ID, amount: in.WholesalePriceMinor, reason: ReasonCreated,
				subjectVersion: created.Version}); err != nil {
				return err
			}
		}
		return s.record(ctx, tx, actor, evidence{
			action: "shop.variant.created", eventType: "shop.variant.changed",
			aggregateType: "shop_variant", aggregateID: created.ID, version: created.Version,
			after: VariantViewOf(created), payload: VariantViewOf(created),
		})
	})
	return created, err
}

type physicalMetrics struct {
	weight, volume        *int64
	length, width, height *int
}

func validatePhysical(weight, volume *int64, length, width, height *int) (physicalMetrics, error) {
	if weight != nil && (*weight <= 0 || *weight > 100000000) {
		return physicalMetrics{}, fieldError("net_weight_grams", "Weight must be a positive number of grams.")
	}
	if volume != nil && (*volume <= 0 || *volume > 100000000) {
		return physicalMetrics{}, fieldError("net_volume_ml", "Volume must be a positive number of millilitres.")
	}
	for field, value := range map[string]*int{"length_mm": length, "width_mm": width, "height_mm": height} {
		if value != nil && (*value <= 0 || *value > 100000) {
			return physicalMetrics{}, fieldError(field, "Dimensions must be a positive number of millimetres.")
		}
	}
	return physicalMetrics{weight: weight, volume: volume, length: length, width: width, height: height}, nil
}

func validateMoney(field string, amount *int64) error {
	if amount == nil {
		return nil
	}
	if *amount < 0 || *amount > MaxMoneyMinor {
		return fieldError(field, "Amounts are whole minor units and must be zero or greater.")
	}
	return nil
}

func normalizeHandlingFlags(raw []string) ([]string, error) {
	if len(raw) > MaxHandlingFlags {
		return nil, fieldError("handling_flags", "Supply at most "+itoa(MaxHandlingFlags)+" handling flags.")
	}
	seen := map[string]bool{}
	flags := []string{}
	for _, entry := range raw {
		value := strings.ToUpper(strings.TrimSpace(entry))
		if value == "" {
			continue
		}
		if !contains(HandlingFlags, value) {
			return nil, fieldError("handling_flags", "Choose supported handling flags.")
		}
		if seen[value] {
			continue
		}
		seen[value] = true
		flags = append(flags, value)
	}
	return flags, nil
}

func replaceHandlingFlags(ctx context.Context, tx pgx.Tx, businessID, variantID string, flags []string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM shop_variant_handling_flags WHERE variant_id=$1`, variantID); err != nil {
		return err
	}
	for _, flag := range flags {
		if _, err := tx.Exec(ctx, `
			INSERT INTO shop_variant_handling_flags (id,business_id,variant_id,handling_flag)
			VALUES ($1,$2,$3,$4)`, newID("svh_"), businessID, variantID, flag); err != nil {
			return mapDB(err, "Handling flag", CodeVariantNotFound)
		}
	}
	return nil
}

func boolOrDefault(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

// ListVariants reads a product's SKUs.
func (s *Service) ListVariants(ctx context.Context, actor Actor, scope Scope, productID string, includeArchived bool) ([]Variant, error) {
	if scope.Empty() {
		return nil, errForbidden()
	}
	sql := `SELECT ` + variantCols + ` FROM shop_variants WHERE business_id=$1 AND product_id=$2`
	if !includeArchived {
		sql += ` AND status <> 'ARCHIVED'`
	}
	sql += ` ORDER BY position, id LIMIT 200`
	rows, err := s.db.Query(ctx, sql, actor.VendorID, strings.TrimSpace(productID))
	if err != nil {
		return nil, err
	}
	var variants []Variant
	for rows.Next() {
		variant, scanErr := scanVariant(rows)
		if scanErr != nil {
			rows.Close()
			return nil, scanErr
		}
		variants = append(variants, variant)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if err = hydrateVariants(ctx, s.db, variants); err != nil {
		return nil, err
	}
	if variants == nil {
		variants = []Variant{}
	}
	return variants, nil
}

// GetVariant reads one SKU.
func (s *Service) GetVariant(ctx context.Context, actor Actor, scope Scope, variantID string) (Variant, error) {
	if scope.Empty() {
		return Variant{}, errForbidden()
	}
	variant, err := loadVariantByID(ctx, s.db, actor.VendorID, variantID, false)
	if err != nil {
		return Variant{}, err
	}
	return variant, hydrateVariant(ctx, s.db, &variant)
}

// UpdateVariantInput edits a SKU.
type UpdateVariantInput struct {
	SKU                 *string
	Name                *string
	PackageLevel        *string
	PackQuantity        *int
	NetWeightGrams      *int64
	NetVolumeML         *int64
	LengthMM            *int
	WidthMM             *int
	HeightMM            *int
	RetailPriceMinor    **int64
	WholesalePriceMinor **int64
	TemperatureZone     *string
	ConsolidationClass  *string
	SeparationGroup     *string
	Stackable           *bool
	LargeLoad           *bool
	HandlingFlags       *[]string
	Status              *string
	ExpectedVersion     int64
}

// UpdateVariant edits a SKU, appending price history for any money that moves.
func (s *Service) UpdateVariant(ctx context.Context, actor Actor, scope Scope, productID, variantID string, in UpdateVariantInput) (Variant, error) {
	if err := s.requireWrites(); err != nil {
		return Variant{}, err
	}
	if err := requireBusinessWide(scope, "Shop variants"); err != nil {
		return Variant{}, err
	}
	if in.RetailPriceMinor != nil {
		if err := s.requirePricingAuthority(ctx, actor, ""); err != nil {
			return Variant{}, err
		}
		if err := validateMoney("retail_price_minor", *in.RetailPriceMinor); err != nil {
			return Variant{}, err
		}
	}
	if in.WholesalePriceMinor != nil {
		if err := s.requireWholesaleAuthority(ctx, actor, ""); err != nil {
			return Variant{}, err
		}
		if err := validateMoney("wholesale_price_minor", *in.WholesalePriceMinor); err != nil {
			return Variant{}, err
		}
	}
	var flags []string
	if in.HandlingFlags != nil {
		normalized, err := normalizeHandlingFlags(*in.HandlingFlags)
		if err != nil {
			return Variant{}, err
		}
		flags = normalized
	}

	var updated Variant
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		current, err := loadVariant(ctx, tx, actor.VendorID, productID, variantID, true)
		if err != nil {
			return err
		}
		if err = expectedVersionMatches(current.Version, in.ExpectedVersion); err != nil {
			return err
		}
		if current.Status == StatusArchived {
			return errArchived("This variant")
		}
		if err = hydrateVariant(ctx, tx, &current); err != nil {
			return err
		}

		sku, name, packageLevel, packQuantity := current.SKU, current.Name, current.PackageLevel, current.PackQuantity
		weight, volume := current.NetWeightGrams, current.NetVolumeML
		length, width, height := current.LengthMM, current.WidthMM, current.HeightMM
		retail, wholesale := current.RetailPriceMinor, current.WholesalePriceMinor
		zone, consolidation, separation := current.TemperatureZone, current.ConsolidationClass, current.SeparationGroup
		stackable, largeLoad, status := current.Stackable, current.LargeLoad, current.Status

		if in.SKU != nil {
			value := strings.TrimSpace(*in.SKU)
			if value != "" && !skuPattern.MatchString(value) {
				return fieldError("sku", "Use up to 64 letters, digits, dot, dash, slash or underscore.")
			}
			sku = value
		}
		if in.Name != nil {
			value, violation := validateName("name", *in.Name, 1, 200)
			if violation != nil {
				return validation(*violation)
			}
			name = value
		}
		if in.PackageLevel != nil {
			value := strings.ToUpper(strings.TrimSpace(*in.PackageLevel))
			if !IsPackageLevel(value) {
				return fieldError("package_level", "Choose a supported package level.")
			}
			packageLevel = value
		}
		if in.PackQuantity != nil {
			if *in.PackQuantity < 1 || *in.PackQuantity > 100000 {
				return fieldError("pack_quantity", "Pack quantity must be between 1 and 100000.")
			}
			packQuantity = *in.PackQuantity
		}
		if in.NetWeightGrams != nil {
			weight = in.NetWeightGrams
		}
		if in.NetVolumeML != nil {
			volume = in.NetVolumeML
		}
		if in.LengthMM != nil {
			length = in.LengthMM
		}
		if in.WidthMM != nil {
			width = in.WidthMM
		}
		if in.HeightMM != nil {
			height = in.HeightMM
		}
		if _, err = validatePhysical(weight, volume, length, width, height); err != nil {
			return err
		}
		if in.RetailPriceMinor != nil {
			retail = *in.RetailPriceMinor
		}
		if in.WholesalePriceMinor != nil {
			wholesale = *in.WholesalePriceMinor
		}
		if in.TemperatureZone != nil {
			value := strings.ToUpper(strings.TrimSpace(*in.TemperatureZone))
			if !contains(TemperatureZones, value) {
				return fieldError("temperature_zone", "Choose a supported temperature zone.")
			}
			zone = value
		}
		if in.ConsolidationClass != nil {
			value := strings.ToUpper(strings.TrimSpace(*in.ConsolidationClass))
			if value != "" && !contains(ConsolidationClasses, value) {
				return fieldError("consolidation_class", "Choose a supported consolidation class.")
			}
			consolidation = value
		}
		if in.SeparationGroup != nil {
			value, violation := validateOptionalText("separation_group", *in.SeparationGroup, 60)
			if violation != nil {
				return validation(*violation)
			}
			separation = value
		}
		if in.Stackable != nil {
			stackable = *in.Stackable
		}
		if in.LargeLoad != nil {
			largeLoad = *in.LargeLoad
		}
		if in.Status != nil {
			value := strings.ToUpper(strings.TrimSpace(*in.Status))
			if value != StatusActive && value != StatusPaused {
				return fieldError("status", "Use ACTIVE or PAUSED. Archive with the archive command.")
			}
			status = value
		}

		row, err := scanVariant(tx.QueryRow(ctx, `
			UPDATE shop_variants
			SET sku=NULLIF($3,''), name=$4, package_level=$5, pack_quantity=$6, net_weight_grams=$7,
			    net_volume_ml=$8, length_mm=$9, width_mm=$10, height_mm=$11, retail_price_minor=$12,
			    wholesale_price_minor=$13, temperature_zone=$14, consolidation_class=NULLIF($15,''),
			    separation_group=NULLIF($16,''), stackable=$17, large_load=$18, status=$19,
			    updated_at=now(), version=version+1
			WHERE business_id=$1 AND id=$2 RETURNING `+variantCols,
			actor.VendorID, current.ID, sku, name, packageLevel, packQuantity, weight, volume,
			length, width, height, retail, wholesale, zone, consolidation, separation,
			stackable, largeLoad, status))
		if err != nil {
			return mapDB(err, "Variant", CodeVariantNotFound)
		}
		updated = row
		if in.HandlingFlags != nil {
			if err = replaceHandlingFlags(ctx, tx, actor.VendorID, updated.ID, flags); err != nil {
				return err
			}
		}
		if err = hydrateVariant(ctx, tx, &updated); err != nil {
			return err
		}
		currency, err := s.businessCurrency(ctx, tx, actor.VendorID)
		if err != nil {
			return err
		}
		if moneyChanged(current.RetailPriceMinor, updated.RetailPriceMinor) {
			if err = s.appendPriceVersion(ctx, tx, actor, currency, priceChange{
				subjectType: SubjectVariantRetail, subjectRef: updated.ID, productRef: updated.ProductID,
				variantRef: updated.ID, previousAmount: current.RetailPriceMinor,
				amount: updated.RetailPriceMinor, subjectVersion: updated.Version,
				reason: priceReason(current.RetailPriceMinor, updated.RetailPriceMinor)}); err != nil {
				return err
			}
		}
		if moneyChanged(current.WholesalePriceMinor, updated.WholesalePriceMinor) {
			if err = s.appendPriceVersion(ctx, tx, actor, currency, priceChange{
				subjectType: SubjectVariantWholesale, subjectRef: updated.ID, productRef: updated.ProductID,
				variantRef: updated.ID, previousAmount: current.WholesalePriceMinor,
				amount: updated.WholesalePriceMinor, subjectVersion: updated.Version,
				reason: priceReason(current.WholesalePriceMinor, updated.WholesalePriceMinor)}); err != nil {
				return err
			}
		}
		if _, err = bumpRevisionsForVariant(ctx, tx, actor.VendorID, updated.ID); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "shop.variant.updated", eventType: "shop.variant.changed",
			aggregateType: "shop_variant", aggregateID: updated.ID, version: updated.Version,
			before: VariantViewOf(current), after: VariantViewOf(updated), payload: VariantViewOf(updated),
		})
	})
	return updated, err
}

// ArchiveVariant retires a SKU and deactivates the listings that sell it.
func (s *Service) ArchiveVariant(ctx context.Context, actor Actor, scope Scope, productID, variantID string, expectedVersion int64) (Variant, error) {
	if err := s.requireWrites(); err != nil {
		return Variant{}, err
	}
	if err := requireBusinessWide(scope, "Shop variants"); err != nil {
		return Variant{}, err
	}
	var archived Variant
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		current, err := loadVariant(ctx, tx, actor.VendorID, productID, variantID, true)
		if err != nil {
			return err
		}
		if err = expectedVersionMatches(current.Version, expectedVersion); err != nil {
			return err
		}
		if current.Status == StatusArchived {
			archived = current
			return hydrateVariant(ctx, tx, &archived)
		}
		row, err := scanVariant(tx.QueryRow(ctx, `
			UPDATE shop_variants SET status='ARCHIVED', archived_at=now(), updated_at=now(), version=version+1
			WHERE business_id=$1 AND id=$2 RETURNING `+variantCols, actor.VendorID, current.ID))
		if err != nil {
			return mapDB(err, "Variant", CodeVariantNotFound)
		}
		archived = row
		if _, err = tx.Exec(ctx, `
			UPDATE shop_branch_listings SET status='INACTIVE', updated_at=now(), version=version+1
			WHERE business_id=$1 AND variant_id=$2 AND status='ACTIVE'`, actor.VendorID, current.ID); err != nil {
			return err
		}
		if _, err = bumpRevisionsForVariant(ctx, tx, actor.VendorID, current.ID); err != nil {
			return err
		}
		if err = hydrateVariant(ctx, tx, &archived); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "shop.variant.archived", eventType: "shop.variant.changed",
			aggregateType: "shop_variant", aggregateID: archived.ID, version: archived.Version,
			before: VariantViewOf(current), after: VariantViewOf(archived), payload: VariantViewOf(archived),
		})
	})
	return archived, err
}

// ---------------------------------------------------------------------------
// Trade identifiers
// ---------------------------------------------------------------------------

const identifierCols = `id,business_id,product_id,variant_id,identifier_type,identifier_value,
	COALESCE(package_level,''),status,created_at,updated_at,archived_at,version`

func scanIdentifier(row rowScanner) (ProductIdentifier, error) {
	var i ProductIdentifier
	err := row.Scan(&i.ID, &i.BusinessID, &i.ProductID, &i.VariantID, &i.Type, &i.Value,
		&i.PackageLevel, &i.Status, &i.CreatedAt, &i.UpdatedAt, &i.ArchivedAt, &i.Version)
	return i, err
}

// AddIdentifier attaches a trade identifier to a variant.
//
// Different package levels legitimately carry different identifiers — the
// single tin and the carton of 24 have different barcodes — which is why the
// level is part of the record rather than assumed.
func (s *Service) AddIdentifier(ctx context.Context, actor Actor, scope Scope, variantID, identifierType, value, packageLevel string) (ProductIdentifier, error) {
	if err := s.requireWrites(); err != nil {
		return ProductIdentifier{}, err
	}
	if err := requireBusinessWide(scope, "Product identifiers"); err != nil {
		return ProductIdentifier{}, err
	}
	kind := strings.ToUpper(strings.TrimSpace(identifierType))
	if !IsIdentifierType(kind) {
		return ProductIdentifier{}, fieldError("identifier_type", "Choose a supported identifier type.")
	}
	trimmed := strings.TrimSpace(value)
	if !identifierPattern.MatchString(trimmed) {
		return ProductIdentifier{}, fieldError("identifier_value", "Use up to 64 letters, digits, dot, dash, slash or underscore.")
	}
	if lengths, bounded := identifierLengths[kind]; bounded {
		if !isDigits(trimmed) || !containsInt(lengths, len(trimmed)) {
			return ProductIdentifier{}, fieldError("identifier_value",
				"A "+kind+" is all digits and must be one of the standard lengths.")
		}
	}
	level := strings.ToUpper(strings.TrimSpace(packageLevel))
	if level != "" && !IsPackageLevel(level) {
		return ProductIdentifier{}, fieldError("package_level", "Choose a supported package level.")
	}

	var created ProductIdentifier
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		variant, err := loadVariantByID(ctx, tx, actor.VendorID, variantID, false)
		if err != nil {
			return err
		}
		if variant.Status == StatusArchived {
			return errArchived("This variant")
		}
		row, err := scanIdentifier(tx.QueryRow(ctx, `
			INSERT INTO shop_product_identifiers
				(id,business_id,product_id,variant_id,identifier_type,identifier_value,package_level,status)
			VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7,''),'ACTIVE') RETURNING `+identifierCols,
			newID("sid_"), actor.VendorID, variant.ProductID, variant.ID, kind, trimmed, level))
		if err != nil {
			return mapDB(err, "Identifier", CodeIdentifierNotFound)
		}
		created = row
		if _, err = bumpRevisionsForVariant(ctx, tx, actor.VendorID, variant.ID); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "shop.identifier.added", eventType: "shop.variant.changed",
			aggregateType: "shop_variant", aggregateID: variant.ID, version: variant.Version,
			after: IdentifierViewOf(created), payload: IdentifierViewOf(created),
		})
	})
	return created, err
}

func isDigits(value string) bool {
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return value != ""
}

func containsInt(set []int, value int) bool {
	for _, entry := range set {
		if entry == value {
			return true
		}
	}
	return false
}

// ListIdentifiers reads a variant's identifiers.
func (s *Service) ListIdentifiers(ctx context.Context, actor Actor, scope Scope, variantID string, includeArchived bool) ([]ProductIdentifier, error) {
	if scope.Empty() {
		return nil, errForbidden()
	}
	sql := `SELECT ` + identifierCols + ` FROM shop_product_identifiers
		WHERE business_id=$1 AND variant_id=$2`
	if !includeArchived {
		sql += ` AND status <> 'ARCHIVED'`
	}
	sql += ` ORDER BY identifier_type, identifier_value LIMIT 100`
	rows, err := s.db.Query(ctx, sql, actor.VendorID, strings.TrimSpace(variantID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	identifiers := []ProductIdentifier{}
	for rows.Next() {
		identifier, scanErr := scanIdentifier(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		identifiers = append(identifiers, identifier)
	}
	return identifiers, rows.Err()
}

// ArchiveIdentifier retires an identifier. It is archived rather than deleted
// because a historical order may name it.
func (s *Service) ArchiveIdentifier(ctx context.Context, actor Actor, scope Scope, variantID, identifierID string, expectedVersion int64) (ProductIdentifier, error) {
	if err := s.requireWrites(); err != nil {
		return ProductIdentifier{}, err
	}
	if err := requireBusinessWide(scope, "Product identifiers"); err != nil {
		return ProductIdentifier{}, err
	}
	var archived ProductIdentifier
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		current, err := scanIdentifier(tx.QueryRow(ctx, `SELECT `+identifierCols+`
			FROM shop_product_identifiers WHERE business_id=$1 AND variant_id=$2 AND id=$3 FOR UPDATE`,
			actor.VendorID, strings.TrimSpace(variantID), strings.TrimSpace(identifierID)))
		if err != nil {
			return mapDB(err, "Identifier", CodeIdentifierNotFound)
		}
		if err = expectedVersionMatches(current.Version, expectedVersion); err != nil {
			return err
		}
		if current.Status == StatusArchived {
			archived = current
			return nil
		}
		row, err := scanIdentifier(tx.QueryRow(ctx, `
			UPDATE shop_product_identifiers
			SET status='ARCHIVED', archived_at=now(), updated_at=now(), version=version+1
			WHERE business_id=$1 AND id=$2 RETURNING `+identifierCols, actor.VendorID, current.ID))
		if err != nil {
			return mapDB(err, "Identifier", CodeIdentifierNotFound)
		}
		archived = row
		if _, err = bumpRevisionsForVariant(ctx, tx, actor.VendorID, archived.VariantID); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "shop.identifier.archived", eventType: "shop.variant.changed",
			aggregateType: "shop_variant", aggregateID: archived.VariantID, version: archived.Version,
			before: IdentifierViewOf(current), after: IdentifierViewOf(archived), payload: IdentifierViewOf(archived),
		})
	})
	return archived, err
}

// LookupByIdentifier resolves a scanned barcode to its variant and product.
//
// Part B consumes this for receiving and stocktakes; Part A exposes only the
// management-side lookup, and it is tenant-scoped, so a scan never resolves
// into someone else's catalogue.
func (s *Service) LookupByIdentifier(ctx context.Context, actor Actor, scope Scope, value, identifierType string) (ProductIdentifier, Variant, Product, error) {
	if scope.Empty() {
		return ProductIdentifier{}, Variant{}, Product{}, errForbidden()
	}
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ProductIdentifier{}, Variant{}, Product{}, fieldError("value", "Provide an identifier value to look up.")
	}
	sql := `SELECT ` + identifierCols + ` FROM shop_product_identifiers
		WHERE business_id=$1 AND upper(identifier_value)=upper($2) AND status='ACTIVE'`
	args := []any{actor.VendorID, trimmed}
	if kind := strings.ToUpper(strings.TrimSpace(identifierType)); kind != "" {
		if !IsIdentifierType(kind) {
			return ProductIdentifier{}, Variant{}, Product{}, fieldError("identifier_type", "Choose a supported identifier type.")
		}
		args = append(args, kind)
		sql += ` AND identifier_type=$3`
	}
	sql += ` ORDER BY identifier_type LIMIT 1`

	identifier, err := scanIdentifier(s.db.QueryRow(ctx, sql, args...))
	if err != nil {
		return ProductIdentifier{}, Variant{}, Product{}, mapDB(err, "Identifier", CodeIdentifierNotFound)
	}
	variant, err := loadVariantByID(ctx, s.db, actor.VendorID, identifier.VariantID, false)
	if err != nil {
		return ProductIdentifier{}, Variant{}, Product{}, err
	}
	if err = hydrateVariant(ctx, s.db, &variant); err != nil {
		return ProductIdentifier{}, Variant{}, Product{}, err
	}
	product, err := loadProduct(ctx, s.db, actor.VendorID, identifier.ProductID, false)
	if err != nil {
		return ProductIdentifier{}, Variant{}, Product{}, err
	}
	if err = hydrateProduct(ctx, s.db, &product); err != nil {
		return ProductIdentifier{}, Variant{}, Product{}, err
	}
	return identifier, variant, product, nil
}

// ---------------------------------------------------------------------------
// Packaging hierarchy
// ---------------------------------------------------------------------------

const packageCols = `id,business_id,product_id,parent_variant_id,child_variant_id,units_per_parent,
	status,created_at,updated_at,archived_at,version`

func scanPackageRelation(row rowScanner) (PackageRelation, error) {
	var p PackageRelation
	err := row.Scan(&p.ID, &p.BusinessID, &p.ProductID, &p.ParentVariant, &p.ChildVariant,
		&p.UnitsPerParent, &p.Status, &p.CreatedAt, &p.UpdatedAt, &p.ArchivedAt, &p.Version)
	return p, err
}

// AddPackageRelation configures "one parent contains N children".
//
// The ratio is always explicit. Nothing infers that a carton holds 24 packs
// from the words "carton" and "pack" — a wrong inference here would misprice
// every wholesale order that used it.
func (s *Service) AddPackageRelation(ctx context.Context, actor Actor, scope Scope, productID, parentVariantID, childVariantID string, unitsPerParent int) (PackageRelation, error) {
	if err := s.requireWrites(); err != nil {
		return PackageRelation{}, err
	}
	if err := requireBusinessWide(scope, "Packaging relationships"); err != nil {
		return PackageRelation{}, err
	}
	if unitsPerParent < 1 || unitsPerParent > 1000000 {
		return PackageRelation{}, fieldError("units_per_parent", "Units per parent must be a positive whole number.")
	}
	parentID, childID := strings.TrimSpace(parentVariantID), strings.TrimSpace(childVariantID)
	if parentID == childID {
		return PackageRelation{}, coded(apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "child_variant_id", Message: "A package cannot contain itself."}}), CodePackageCycle)
	}

	var created PackageRelation
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		product, err := loadProduct(ctx, tx, actor.VendorID, productID, true)
		if err != nil {
			return err
		}
		if _, err = loadVariant(ctx, tx, actor.VendorID, product.ID, parentID, false); err != nil {
			return err
		}
		if _, err = loadVariant(ctx, tx, actor.VendorID, product.ID, childID, false); err != nil {
			return err
		}
		// If the proposed child already contains the proposed parent, adding
		// this edge would close a containment loop and make any expansion
		// non-terminating.
		cycle, err := packageContains(ctx, tx, actor.VendorID, childID, parentID)
		if err != nil {
			return err
		}
		if cycle {
			return coded(apperrors.Conflict("That relationship would create a packaging cycle.", nil), CodePackageCycle)
		}
		row, err := scanPackageRelation(tx.QueryRow(ctx, `
			INSERT INTO shop_package_relations
				(id,business_id,product_id,parent_variant_id,child_variant_id,units_per_parent,status)
			VALUES ($1,$2,$3,$4,$5,$6,'ACTIVE') RETURNING `+packageCols,
			newID("spk_"), actor.VendorID, product.ID, parentID, childID, unitsPerParent))
		if err != nil {
			return mapDB(err, "Packaging relationship", CodePackageNotFound)
		}
		created = row
		if _, err = bumpRevisionsForProduct(ctx, tx, actor.VendorID, product.ID); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "shop.packaging.added", eventType: "shop.product.changed",
			aggregateType: "shop_product", aggregateID: product.ID, version: created.Version,
			after: PackageRelationViewOf(created), payload: PackageRelationViewOf(created),
		})
	})
	return created, err
}

// packageContains reports whether needle is reachable below start.
func packageContains(ctx context.Context, q querier, businessID, start, needle string) (bool, error) {
	var found bool
	err := q.QueryRow(ctx, `
		WITH RECURSIVE down AS (
			SELECT child_variant_id AS id, 1 AS level FROM shop_package_relations
			WHERE business_id=$1 AND parent_variant_id=$2 AND status='ACTIVE'
			UNION ALL
			SELECT r.child_variant_id, down.level+1 FROM shop_package_relations r
			JOIN down ON r.parent_variant_id = down.id AND r.business_id=$1 AND r.status='ACTIVE'
			WHERE down.level < 16
		)
		SELECT EXISTS (SELECT 1 FROM down WHERE id=$3) OR $2=$3`,
		businessID, start, needle).Scan(&found)
	return found, err
}

// ListPackageRelations reads a product's packaging hierarchy.
func (s *Service) ListPackageRelations(ctx context.Context, actor Actor, scope Scope, productID string, includeArchived bool) ([]PackageRelation, error) {
	if scope.Empty() {
		return nil, errForbidden()
	}
	sql := `SELECT ` + packageCols + ` FROM shop_package_relations WHERE business_id=$1 AND product_id=$2`
	if !includeArchived {
		sql += ` AND status <> 'ARCHIVED'`
	}
	sql += ` ORDER BY parent_variant_id, child_variant_id LIMIT 200`
	rows, err := s.db.Query(ctx, sql, actor.VendorID, strings.TrimSpace(productID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	relations := []PackageRelation{}
	for rows.Next() {
		relation, scanErr := scanPackageRelation(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		relations = append(relations, relation)
	}
	return relations, rows.Err()
}

// ArchivePackageRelation retires a packaging relationship.
func (s *Service) ArchivePackageRelation(ctx context.Context, actor Actor, scope Scope, productID, relationID string, expectedVersion int64) (PackageRelation, error) {
	if err := s.requireWrites(); err != nil {
		return PackageRelation{}, err
	}
	if err := requireBusinessWide(scope, "Packaging relationships"); err != nil {
		return PackageRelation{}, err
	}
	var archived PackageRelation
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		current, err := scanPackageRelation(tx.QueryRow(ctx, `SELECT `+packageCols+`
			FROM shop_package_relations WHERE business_id=$1 AND product_id=$2 AND id=$3 FOR UPDATE`,
			actor.VendorID, strings.TrimSpace(productID), strings.TrimSpace(relationID)))
		if err != nil {
			return mapDB(err, "Packaging relationship", CodePackageNotFound)
		}
		if err = expectedVersionMatches(current.Version, expectedVersion); err != nil {
			return err
		}
		if current.Status == StatusArchived {
			archived = current
			return nil
		}
		row, err := scanPackageRelation(tx.QueryRow(ctx, `
			UPDATE shop_package_relations
			SET status='ARCHIVED', archived_at=now(), updated_at=now(), version=version+1
			WHERE business_id=$1 AND id=$2 RETURNING `+packageCols, actor.VendorID, current.ID))
		if err != nil {
			return mapDB(err, "Packaging relationship", CodePackageNotFound)
		}
		archived = row
		if _, err = bumpRevisionsForProduct(ctx, tx, actor.VendorID, current.ProductID); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "shop.packaging.archived", eventType: "shop.product.changed",
			aggregateType: "shop_product", aggregateID: current.ProductID, version: archived.Version,
			before: PackageRelationViewOf(current), after: PackageRelationViewOf(archived),
			payload: PackageRelationViewOf(archived),
		})
	})
	return archived, err
}
