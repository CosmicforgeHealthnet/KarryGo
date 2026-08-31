package shop

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

// ---------------------------------------------------------------------------
// Measure definitions
// ---------------------------------------------------------------------------
//
// Trade measurement is a domain, not a string column. The seeded platform
// vocabulary covers standard units and the Nigerian market measures a trader
// actually uses — Paint, Derica, Mudu, Congo, Tiya, Kobiowu, basket, bunch,
// bag, sack, crate — and a business may add its own, visible only to itself.
//
// NO LOCAL CONTAINER CARRIES A WEIGHT. A Paint of rice, a Paint of beans, a
// Paint of garri and a Paint of crayfish are four different masses, so no
// definition here may claim one; a vendor calibrates its own containers per
// product instead.

const measureCols = `id,COALESCE(business_id,''),owner_key,code,display_name,measurement_family,
	measure_origin,COALESCE(region_code,''),base_quantity,status,created_at,updated_at,archived_at,version`

func scanMeasure(row rowScanner) (MeasureDefinition, error) {
	var m MeasureDefinition
	err := row.Scan(&m.ID, &m.BusinessID, &m.OwnerKey, &m.Code, &m.DisplayName, &m.Family,
		&m.Origin, &m.RegionCode, &m.BaseQuantity, &m.Status, &m.CreatedAt, &m.UpdatedAt,
		&m.ArchivedAt, &m.Version)
	return m, err
}

// loadMeasure reads a definition the business may legitimately see: a platform
// definition, or its own. Another tenant's custom measure simply does not
// exist as far as this query is concerned.
func loadMeasure(ctx context.Context, q querier, businessID, measureID string, lock bool) (MeasureDefinition, error) {
	sql := `SELECT ` + measureCols + ` FROM shop_measure_definitions
		WHERE id=$2 AND owner_key IN ('PLATFORM',$1)`
	if lock {
		sql += ` FOR UPDATE`
	}
	measure, err := scanMeasure(q.QueryRow(ctx, sql, businessID, strings.TrimSpace(measureID)))
	if err != nil {
		return MeasureDefinition{}, mapDB(err, "Measure definition", CodeMeasureNotFound)
	}
	return measure, nil
}

// CreateMeasureInput defines a vendor-owned measure.
type CreateMeasureInput struct {
	Code        string
	DisplayName string
	Family      string
	RegionCode  string
	// BaseQuantity is only meaningful for MASS, VOLUME and COUNT. Supplying it
	// for a container measure is refused rather than quietly dropped.
	BaseQuantity *int64
}

// CreateMeasure adds a business-scoped measure definition.
func (s *Service) CreateMeasure(ctx context.Context, actor Actor, scope Scope, in CreateMeasureInput) (MeasureDefinition, error) {
	if err := s.requireWrites(); err != nil {
		return MeasureDefinition{}, err
	}
	if err := requireBusinessWide(scope, "Measure definitions"); err != nil {
		return MeasureDefinition{}, err
	}
	code := strings.ToUpper(strings.TrimSpace(in.Code))
	if !measureCodePattern.MatchString(code) {
		return MeasureDefinition{}, fieldError("code", "Use 2–40 uppercase letters, digits or underscores.")
	}
	displayName, violation := validateName("display_name", in.DisplayName, 1, 80)
	if violation != nil {
		return MeasureDefinition{}, validation(*violation)
	}
	family := strings.ToUpper(strings.TrimSpace(in.Family))
	if !IsMeasurementFamily(family) {
		return MeasureDefinition{}, fieldError("measurement_family", "Choose a supported measurement family.")
	}
	region := strings.ToUpper(strings.TrimSpace(in.RegionCode))
	if region != "" && !regionPattern.MatchString(region) {
		return MeasureDefinition{}, fieldError("region_code", "Use an ISO country code, optionally with a region suffix.")
	}
	if in.BaseQuantity != nil {
		if !IsConvertibleFamily(family) {
			return MeasureDefinition{}, coded(apperrors.Validation("Check your details.", []apperrors.FieldViolation{
				{Field: "base_quantity", Message: "Only mass, volume and count measures have an exact base quantity. " +
					"Container and package measures are calibrated per product instead."}}), CodeConversionInvalid)
		}
		if *in.BaseQuantity <= 0 || *in.BaseQuantity > 1000000000 {
			return MeasureDefinition{}, fieldError("base_quantity", "Base quantity must be a positive integer.")
		}
	}

	var created MeasureDefinition
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		row, err := scanMeasure(tx.QueryRow(ctx, `
			INSERT INTO shop_measure_definitions
				(id,business_id,code,display_name,measurement_family,measure_origin,region_code,base_quantity,status)
			VALUES ($1,$2,$3,$4,$5,'VENDOR_DEFINED',NULLIF($6,''),$7,'ACTIVE')
			RETURNING `+measureCols,
			newID("smd_"), actor.VendorID, code, displayName, family, region, in.BaseQuantity))
		if err != nil {
			return mapDB(err, "Measure definition", CodeMeasureNotFound)
		}
		created = row
		return s.record(ctx, tx, actor, evidence{
			action: "shop.measurement.created", eventType: "shop.measurement.changed",
			aggregateType: "shop_measure_definition", aggregateID: created.ID, version: created.Version,
			after: MeasureViewOf(created), payload: MeasureViewOf(created),
		})
	})
	return created, err
}

// MeasureListOptions bounds a measure read.
type MeasureListOptions struct {
	Family          string
	Origin          string
	Search          string
	IncludeArchived bool
	Limit, Offset   int
}

// ListMeasures reads the definitions this business may use: platform-seeded
// plus its own, never another tenant's.
func (s *Service) ListMeasures(ctx context.Context, actor Actor, scope Scope, in MeasureListOptions) ([]MeasureDefinition, bool, error) {
	if scope.Empty() {
		return nil, false, errForbidden()
	}
	limit, offset := boundedPage(in.Limit, in.Offset)
	sql := `SELECT ` + measureCols + ` FROM shop_measure_definitions WHERE owner_key IN ('PLATFORM',$1)`
	args := []any{actor.VendorID}
	if family := strings.ToUpper(strings.TrimSpace(in.Family)); family != "" {
		if !IsMeasurementFamily(family) {
			return nil, false, fieldError("measurement_family", "Choose a supported measurement family.")
		}
		args = append(args, family)
		sql += ` AND measurement_family=$` + itoa(len(args))
	}
	if origin := strings.ToUpper(strings.TrimSpace(in.Origin)); origin != "" {
		args = append(args, origin)
		sql += ` AND measure_origin=$` + itoa(len(args))
	}
	if search := strings.TrimSpace(in.Search); search != "" {
		// Aliases are searchable because a trader looks for "Paint Bucket",
		// not for PAINT — but the canonical code remains the identity every
		// calculation uses.
		args = append(args, "%"+strings.ToLower(search)+"%")
		sql += ` AND (lower(display_name) LIKE $` + itoa(len(args)) +
			` OR lower(code) LIKE $` + itoa(len(args)) +
			` OR EXISTS (SELECT 1 FROM shop_measure_aliases a
			              WHERE a.measure_definition_id = shop_measure_definitions.id
			                AND (a.business_id IS NULL OR a.business_id = $1)
			                AND lower(a.alias) LIKE $` + itoa(len(args)) + `))`
	}
	if !in.IncludeArchived {
		sql += ` AND status <> 'ARCHIVED'`
	}
	args = append(args, limit+1, offset)
	sql += ` ORDER BY measurement_family, code LIMIT $` + itoa(len(args)-1) + ` OFFSET $` + itoa(len(args))

	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var measures []MeasureDefinition
	for rows.Next() {
		measure, scanErr := scanMeasure(rows)
		if scanErr != nil {
			return nil, false, scanErr
		}
		measures = append(measures, measure)
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(measures) > limit
	if hasMore {
		measures = measures[:limit]
	}
	return measures, hasMore, nil
}

// GetMeasure reads one visible definition together with its aliases.
func (s *Service) GetMeasure(ctx context.Context, actor Actor, scope Scope, measureID string) (MeasureDefinition, []MeasureAlias, error) {
	if scope.Empty() {
		return MeasureDefinition{}, nil, errForbidden()
	}
	measure, err := loadMeasure(ctx, s.db, actor.VendorID, measureID, false)
	if err != nil {
		return MeasureDefinition{}, nil, err
	}
	aliases, err := listAliases(ctx, s.db, actor.VendorID, measure.ID)
	return measure, aliases, err
}

// UpdateMeasureInput edits a vendor-owned definition.
type UpdateMeasureInput struct {
	DisplayName     *string
	RegionCode      *string
	ExpectedVersion int64
}

// UpdateMeasure edits a business-owned definition. Platform definitions are
// shared vocabulary and are never editable by a tenant.
func (s *Service) UpdateMeasure(ctx context.Context, actor Actor, scope Scope, measureID string, in UpdateMeasureInput) (MeasureDefinition, error) {
	if err := s.requireWrites(); err != nil {
		return MeasureDefinition{}, err
	}
	if err := requireBusinessWide(scope, "Measure definitions"); err != nil {
		return MeasureDefinition{}, err
	}
	var updated MeasureDefinition
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		current, err := loadMeasure(ctx, tx, actor.VendorID, measureID, true)
		if err != nil {
			return err
		}
		if current.BusinessID == "" {
			return apperrors.Forbidden("Platform measure definitions are shared and cannot be edited.", nil)
		}
		if err = expectedVersionMatches(current.Version, in.ExpectedVersion); err != nil {
			return err
		}
		if current.Status == StatusArchived {
			return errArchived("This measure definition")
		}
		displayName, region := current.DisplayName, current.RegionCode
		if in.DisplayName != nil {
			value, violation := validateName("display_name", *in.DisplayName, 1, 80)
			if violation != nil {
				return validation(*violation)
			}
			displayName = value
		}
		if in.RegionCode != nil {
			value := strings.ToUpper(strings.TrimSpace(*in.RegionCode))
			if value != "" && !regionPattern.MatchString(value) {
				return fieldError("region_code", "Use an ISO country code, optionally with a region suffix.")
			}
			region = value
		}
		row, err := scanMeasure(tx.QueryRow(ctx, `
			UPDATE shop_measure_definitions
			SET display_name=$3, region_code=NULLIF($4,''), updated_at=now(), version=version+1
			WHERE id=$2 AND business_id=$1 RETURNING `+measureCols,
			actor.VendorID, current.ID, displayName, region))
		if err != nil {
			return mapDB(err, "Measure definition", CodeMeasureNotFound)
		}
		updated = row
		if _, err = bumpRevisionsForMeasure(ctx, tx, actor.VendorID, updated.ID); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "shop.measurement.updated", eventType: "shop.measurement.changed",
			aggregateType: "shop_measure_definition", aggregateID: updated.ID, version: updated.Version,
			before: MeasureViewOf(current), after: MeasureViewOf(updated), payload: MeasureViewOf(updated),
		})
	})
	return updated, err
}

// ArchiveMeasure retires a business-owned definition. It is refused while a
// live sell unit or calibration still names it, because a retired measure
// would leave those rows pointing at a unit nobody can explain.
func (s *Service) ArchiveMeasure(ctx context.Context, actor Actor, scope Scope, measureID string, expectedVersion int64) (MeasureDefinition, error) {
	if err := s.requireWrites(); err != nil {
		return MeasureDefinition{}, err
	}
	if err := requireBusinessWide(scope, "Measure definitions"); err != nil {
		return MeasureDefinition{}, err
	}
	var archived MeasureDefinition
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		current, err := loadMeasure(ctx, tx, actor.VendorID, measureID, true)
		if err != nil {
			return err
		}
		if current.BusinessID == "" {
			return apperrors.Forbidden("Platform measure definitions are shared and cannot be archived.", nil)
		}
		if err = expectedVersionMatches(current.Version, expectedVersion); err != nil {
			return err
		}
		if current.Status == StatusArchived {
			archived = current
			return nil
		}
		var inUse int
		if err = tx.QueryRow(ctx, `
			SELECT (SELECT count(*) FROM shop_sell_units
			        WHERE business_id=$1 AND measure_definition_id=$2 AND status <> 'ARCHIVED')
			     + (SELECT count(*) FROM shop_measure_calibrations
			        WHERE business_id=$1 AND measure_definition_id=$2 AND status <> 'ARCHIVED')`,
			actor.VendorID, current.ID).Scan(&inUse); err != nil {
			return err
		}
		if inUse > 0 {
			return apperrors.Conflict("This measure is still used by a sell unit or calibration.", nil)
		}
		row, err := scanMeasure(tx.QueryRow(ctx, `
			UPDATE shop_measure_definitions
			SET status='ARCHIVED', archived_at=now(), updated_at=now(), version=version+1
			WHERE id=$2 AND business_id=$1 RETURNING `+measureCols, actor.VendorID, current.ID))
		if err != nil {
			return mapDB(err, "Measure definition", CodeMeasureNotFound)
		}
		archived = row
		return s.record(ctx, tx, actor, evidence{
			action: "shop.measurement.archived", eventType: "shop.measurement.changed",
			aggregateType: "shop_measure_definition", aggregateID: archived.ID, version: archived.Version,
			before: MeasureViewOf(current), after: MeasureViewOf(archived), payload: MeasureViewOf(archived),
		})
	})
	return archived, err
}

// ---------------------------------------------------------------------------
// Aliases
// ---------------------------------------------------------------------------

func listAliases(ctx context.Context, q querier, businessID, measureID string) ([]MeasureAlias, error) {
	rows, err := q.Query(ctx, `
		SELECT id,measure_definition_id,owner_key,COALESCE(business_id,''),alias,COALESCE(region_code,''),created_at
		FROM shop_measure_aliases
		WHERE measure_definition_id=$1 AND (business_id IS NULL OR business_id=$2)
		ORDER BY lower(alias)`, measureID, businessID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	aliases := []MeasureAlias{}
	for rows.Next() {
		var alias MeasureAlias
		if err = rows.Scan(&alias.ID, &alias.MeasureID, &alias.OwnerKey, &alias.BusinessID,
			&alias.Alias, &alias.RegionCode, &alias.CreatedAt); err != nil {
			return nil, err
		}
		aliases = append(aliases, alias)
	}
	return aliases, rows.Err()
}

// ListMeasureAliases reads one definition's aliases.
func (s *Service) ListMeasureAliases(ctx context.Context, actor Actor, scope Scope, measureID string) ([]MeasureAlias, error) {
	if scope.Empty() {
		return nil, errForbidden()
	}
	measure, err := loadMeasure(ctx, s.db, actor.VendorID, measureID, false)
	if err != nil {
		return nil, err
	}
	return listAliases(ctx, s.db, actor.VendorID, measure.ID)
}

// AddMeasureAlias records a vendor-scoped presentation name.
//
// An alias on a PLATFORM measure is scoped to the business: one trader calling
// a Paint a "Paint Bucket" must not rename the measure for every other tenant.
func (s *Service) AddMeasureAlias(ctx context.Context, actor Actor, scope Scope, measureID, aliasText, regionCode string) (MeasureAlias, error) {
	if err := s.requireWrites(); err != nil {
		return MeasureAlias{}, err
	}
	if err := requireBusinessWide(scope, "Measure aliases"); err != nil {
		return MeasureAlias{}, err
	}
	alias, violation := validateName("alias", aliasText, 1, 80)
	if violation != nil {
		return MeasureAlias{}, validation(*violation)
	}
	region := strings.ToUpper(strings.TrimSpace(regionCode))
	if region != "" && !regionPattern.MatchString(region) {
		return MeasureAlias{}, fieldError("region_code", "Use an ISO country code, optionally with a region suffix.")
	}

	var created MeasureAlias
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		measure, err := loadMeasure(ctx, tx, actor.VendorID, measureID, false)
		if err != nil {
			return err
		}
		if measure.Status == StatusArchived {
			return errArchived("This measure definition")
		}
		var count int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM shop_measure_aliases
			WHERE measure_definition_id=$1 AND business_id=$2`,
			measure.ID, actor.VendorID).Scan(&count); err != nil {
			return err
		}
		if count >= MaxAliasesPerMeasure {
			return fieldError("alias", "A measure may have at most "+itoa(MaxAliasesPerMeasure)+" aliases.")
		}
		row := tx.QueryRow(ctx, `
			INSERT INTO shop_measure_aliases (id,measure_definition_id,owner_key,business_id,alias,region_code)
			VALUES ($1,$2,$3,$4,$5,NULLIF($6,''))
			RETURNING id,measure_definition_id,owner_key,COALESCE(business_id,''),alias,COALESCE(region_code,''),created_at`,
			newID("sma_"), measure.ID, measure.OwnerKey, actor.VendorID, alias, region)
		if err = row.Scan(&created.ID, &created.MeasureID, &created.OwnerKey, &created.BusinessID,
			&created.Alias, &created.RegionCode, &created.CreatedAt); err != nil {
			return mapDB(err, "Measure alias", CodeMeasureNotFound)
		}
		if _, err = bumpRevisionsForMeasure(ctx, tx, actor.VendorID, measure.ID); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "shop.measurement.alias_added", eventType: "shop.measurement.changed",
			aggregateType: "shop_measure_definition", aggregateID: measure.ID, version: measure.Version,
			after: AliasViewOf(created), payload: AliasViewOf(created),
		})
	})
	return created, err
}

// RemoveMeasureAlias deletes a vendor-owned alias. Aliases carry no
// historical authority — canonical codes do — so this is a real delete.
func (s *Service) RemoveMeasureAlias(ctx context.Context, actor Actor, scope Scope, measureID, aliasID string) error {
	if err := s.requireWrites(); err != nil {
		return err
	}
	if err := requireBusinessWide(scope, "Measure aliases"); err != nil {
		return err
	}
	return s.inTx(ctx, func(tx pgx.Tx) error {
		measure, err := loadMeasure(ctx, tx, actor.VendorID, measureID, false)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `
			DELETE FROM shop_measure_aliases
			WHERE id=$1 AND measure_definition_id=$2 AND business_id=$3`,
			strings.TrimSpace(aliasID), measure.ID, actor.VendorID)
		if err != nil {
			return mapDB(err, "Measure alias", CodeMeasureNotFound)
		}
		if tag.RowsAffected() == 0 {
			return errNotFound("Measure alias", CodeMeasureNotFound)
		}
		if _, err = bumpRevisionsForMeasure(ctx, tx, actor.VendorID, measure.ID); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "shop.measurement.alias_removed", eventType: "shop.measurement.changed",
			aggregateType: "shop_measure_definition", aggregateID: measure.ID, version: measure.Version,
			payload: map[string]any{"measure_ref": measure.ID, "alias_ref": strings.TrimSpace(aliasID)},
		})
	})
}

// ---------------------------------------------------------------------------
// Calibration
// ---------------------------------------------------------------------------

const calibrationCols = `id,business_id,COALESCE(branch_id,''),product_id,COALESCE(variant_id,''),
	measure_definition_id,measure_owner_key,approximate_weight_grams,approximate_volume_ml,tolerance_bps,
	calibration_source,effective_from,effective_to,status,created_at,updated_at,archived_at,version`

func scanCalibration(row rowScanner) (MeasureCalibration, error) {
	var c MeasureCalibration
	err := row.Scan(&c.ID, &c.BusinessID, &c.BranchID, &c.ProductID, &c.VariantID, &c.MeasureID,
		&c.MeasureOwnerKey, &c.WeightGrams, &c.VolumeML, &c.ToleranceBps, &c.Source,
		&c.EffectiveFrom, &c.EffectiveTo, &c.Status, &c.CreatedAt, &c.UpdatedAt, &c.ArchivedAt, &c.Version)
	return c, err
}

func loadCalibration(ctx context.Context, q querier, businessID, productID, calibrationID string, lock bool) (MeasureCalibration, error) {
	sql := `SELECT ` + calibrationCols + ` FROM shop_measure_calibrations
		WHERE business_id=$1 AND product_id=$2 AND id=$3`
	if lock {
		sql += ` FOR UPDATE`
	}
	calibration, err := scanCalibration(q.QueryRow(ctx, sql, businessID,
		strings.TrimSpace(productID), strings.TrimSpace(calibrationID)))
	if err != nil {
		return MeasureCalibration{}, mapDB(err, "Calibration", CodeCalibrationNotFound)
	}
	return calibration, nil
}

// CreateCalibrationInput records a vendor's measured container content.
type CreateCalibrationInput struct {
	BranchID     string
	VariantID    string
	MeasureID    string
	WeightGrams  *int64
	VolumeML     *int64
	ToleranceBps int
	Source       string
}

// CreateCalibration records how much of THIS product fits in THIS container,
// as approximate evidence with a tolerance and a source.
//
// It is deliberately per product — and optionally per branch — because that is
// the only honest scope: the same paint rubber holds a different mass of rice,
// beans, garri and egusi, and two branches may not even use the same rubber.
func (s *Service) CreateCalibration(ctx context.Context, actor Actor, scope Scope, productID string, in CreateCalibrationInput) (MeasureCalibration, error) {
	if err := s.requireWrites(); err != nil {
		return MeasureCalibration{}, err
	}
	if in.WeightGrams == nil && in.VolumeML == nil {
		return MeasureCalibration{}, fieldError("approximate_weight_grams",
			"Provide an approximate weight or an approximate volume.")
	}
	if in.WeightGrams != nil && (*in.WeightGrams <= 0 || *in.WeightGrams > 100000000) {
		return MeasureCalibration{}, fieldError("approximate_weight_grams", "Weight must be a positive number of grams.")
	}
	if in.VolumeML != nil && (*in.VolumeML <= 0 || *in.VolumeML > 100000000) {
		return MeasureCalibration{}, fieldError("approximate_volume_ml", "Volume must be a positive number of millilitres.")
	}
	if in.ToleranceBps < 0 || in.ToleranceBps > 10000 {
		return MeasureCalibration{}, fieldError("tolerance_bps", "Tolerance must be between 0 and 10000 basis points.")
	}
	source := strings.ToUpper(strings.TrimSpace(in.Source))
	if source == "" {
		source = CalibrationVendor
	}
	if !contains(CalibrationSources, source) {
		return MeasureCalibration{}, fieldError("calibration_source", "Choose a supported calibration source.")
	}
	branchID := strings.TrimSpace(in.BranchID)
	if branchID == "" {
		if err := requireBusinessWide(scope, "Business-wide calibrations"); err != nil {
			return MeasureCalibration{}, err
		}
	} else if !scope.Covers(branchID) {
		return MeasureCalibration{}, errForbidden()
	}

	var created MeasureCalibration
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		product, err := loadProduct(ctx, tx, actor.VendorID, productID, false)
		if err != nil {
			return err
		}
		if product.Status == StatusArchived {
			return errArchived("This product")
		}
		measure, err := loadMeasure(ctx, tx, actor.VendorID, in.MeasureID, false)
		if err != nil {
			return err
		}
		if measure.Status == StatusArchived {
			return errArchived("This measure definition")
		}
		if branchID != "" {
			if _, err = s.requireShopBranch(ctx, tx, actor.VendorID, branchID); err != nil {
				return err
			}
		}
		variantID := strings.TrimSpace(in.VariantID)
		if variantID != "" {
			if _, err = loadVariant(ctx, tx, actor.VendorID, product.ID, variantID, false); err != nil {
				return err
			}
		}
		row, err := scanCalibration(tx.QueryRow(ctx, `
			INSERT INTO shop_measure_calibrations
				(id,business_id,branch_id,product_id,variant_id,measure_definition_id,measure_owner_key,
				 approximate_weight_grams,approximate_volume_ml,tolerance_bps,calibration_source,status)
			VALUES ($1,$2,NULLIF($3,''),$4,NULLIF($5,''),$6,$7,$8,$9,$10,$11,'ACTIVE')
			RETURNING `+calibrationCols,
			newID("smc_"), actor.VendorID, branchID, product.ID, variantID, measure.ID, measure.OwnerKey,
			in.WeightGrams, in.VolumeML, in.ToleranceBps, source))
		if err != nil {
			return mapDB(err, "Calibration", CodeCalibrationNotFound)
		}
		created = row
		if _, err = bumpRevisionsForProduct(ctx, tx, actor.VendorID, product.ID); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "shop.measurement.calibrated", eventType: "shop.measurement.changed",
			aggregateType: "shop_measure_calibration", aggregateID: created.ID, branchID: created.BranchID,
			version: created.Version, after: CalibrationViewOf(created), payload: CalibrationViewOf(created),
		})
	})
	return created, err
}

// ListCalibrations reads a product's calibrations.
func (s *Service) ListCalibrations(ctx context.Context, actor Actor, scope Scope, productID string, includeArchived bool) ([]MeasureCalibration, error) {
	if scope.Empty() {
		return nil, errForbidden()
	}
	sql := `SELECT ` + calibrationCols + ` FROM shop_measure_calibrations
		WHERE business_id=$1 AND product_id=$2`
	args := []any{actor.VendorID, strings.TrimSpace(productID)}
	if !scope.Global {
		args = append(args, scope.BranchIDs)
		// Business-wide calibration evidence is shared catalogue truth. A
		// branch calibration is visible only to a grant covering that branch.
		sql += ` AND (branch_id IS NULL OR branch_id=ANY($3))`
	}
	if !includeArchived {
		sql += ` AND status <> 'ARCHIVED'`
	}
	sql += ` ORDER BY COALESCE(branch_id,''), measure_definition_id, created_at DESC LIMIT 200`
	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	calibrations := []MeasureCalibration{}
	for rows.Next() {
		calibration, scanErr := scanCalibration(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		calibrations = append(calibrations, calibration)
	}
	return calibrations, rows.Err()
}

// UpdateCalibrationInput revises a calibration.
type UpdateCalibrationInput struct {
	WeightGrams     *int64
	VolumeML        *int64
	ToleranceBps    *int
	Source          *string
	ExpectedVersion int64
}

// UpdateCalibration revises the measured evidence.
func (s *Service) UpdateCalibration(ctx context.Context, actor Actor, scope Scope, productID, calibrationID string, in UpdateCalibrationInput) (MeasureCalibration, error) {
	if err := s.requireWrites(); err != nil {
		return MeasureCalibration{}, err
	}
	var updated MeasureCalibration
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		current, err := loadCalibration(ctx, tx, actor.VendorID, productID, calibrationID, true)
		if err != nil {
			return err
		}
		if current.BranchID == "" {
			if err = requireBusinessWide(scope, "Business-wide calibrations"); err != nil {
				return err
			}
		} else if !scope.Covers(current.BranchID) {
			return errForbidden()
		}
		if err = expectedVersionMatches(current.Version, in.ExpectedVersion); err != nil {
			return err
		}
		if current.Status == StatusArchived {
			return errArchived("This calibration")
		}
		weight, volume := current.WeightGrams, current.VolumeML
		tolerance, source := current.ToleranceBps, current.Source
		if in.WeightGrams != nil {
			if *in.WeightGrams <= 0 || *in.WeightGrams > 100000000 {
				return fieldError("approximate_weight_grams", "Weight must be a positive number of grams.")
			}
			weight = in.WeightGrams
		}
		if in.VolumeML != nil {
			if *in.VolumeML <= 0 || *in.VolumeML > 100000000 {
				return fieldError("approximate_volume_ml", "Volume must be a positive number of millilitres.")
			}
			volume = in.VolumeML
		}
		if weight == nil && volume == nil {
			return fieldError("approximate_weight_grams", "Provide an approximate weight or an approximate volume.")
		}
		if in.ToleranceBps != nil {
			if *in.ToleranceBps < 0 || *in.ToleranceBps > 10000 {
				return fieldError("tolerance_bps", "Tolerance must be between 0 and 10000 basis points.")
			}
			tolerance = *in.ToleranceBps
		}
		if in.Source != nil {
			value := strings.ToUpper(strings.TrimSpace(*in.Source))
			if !contains(CalibrationSources, value) {
				return fieldError("calibration_source", "Choose a supported calibration source.")
			}
			source = value
		}
		row, err := scanCalibration(tx.QueryRow(ctx, `
			UPDATE shop_measure_calibrations
			SET approximate_weight_grams=$3, approximate_volume_ml=$4, tolerance_bps=$5,
			    calibration_source=$6, updated_at=now(), version=version+1
			WHERE business_id=$1 AND id=$2 RETURNING `+calibrationCols,
			actor.VendorID, current.ID, weight, volume, tolerance, source))
		if err != nil {
			return mapDB(err, "Calibration", CodeCalibrationNotFound)
		}
		updated = row
		if _, err = bumpRevisionsForProduct(ctx, tx, actor.VendorID, current.ProductID); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "shop.measurement.calibration_updated", eventType: "shop.measurement.changed",
			aggregateType: "shop_measure_calibration", aggregateID: updated.ID, branchID: updated.BranchID,
			version: updated.Version, before: CalibrationViewOf(current), after: CalibrationViewOf(updated),
			payload: CalibrationViewOf(updated),
		})
	})
	return updated, err
}

// ArchiveCalibration retires a calibration, refusing while a sell unit still
// relies on it for its approximate conversion.
func (s *Service) ArchiveCalibration(ctx context.Context, actor Actor, scope Scope, productID, calibrationID string, expectedVersion int64) (MeasureCalibration, error) {
	if err := s.requireWrites(); err != nil {
		return MeasureCalibration{}, err
	}
	var archived MeasureCalibration
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		current, err := loadCalibration(ctx, tx, actor.VendorID, productID, calibrationID, true)
		if err != nil {
			return err
		}
		if current.BranchID == "" {
			if err = requireBusinessWide(scope, "Business-wide calibrations"); err != nil {
				return err
			}
		} else if !scope.Covers(current.BranchID) {
			return errForbidden()
		}
		if err = expectedVersionMatches(current.Version, expectedVersion); err != nil {
			return err
		}
		if current.Status == StatusArchived {
			archived = current
			return nil
		}
		var dependents int
		if err = tx.QueryRow(ctx, `
			SELECT (SELECT count(*) FROM shop_sell_units
			        WHERE business_id=$1 AND calibration_id=$2 AND status <> 'ARCHIVED')
			     + (SELECT count(*) FROM shop_listing_sell_units
			        WHERE business_id=$1 AND calibration_id=$2)`,
			actor.VendorID, current.ID).Scan(&dependents); err != nil {
			return err
		}
		if dependents > 0 {
			return apperrors.Conflict("A sell unit still uses this calibration for its approximate conversion.", nil)
		}
		row, err := scanCalibration(tx.QueryRow(ctx, `
			UPDATE shop_measure_calibrations
			SET status='ARCHIVED', archived_at=now(), effective_to=now(), updated_at=now(), version=version+1
			WHERE business_id=$1 AND id=$2 RETURNING `+calibrationCols, actor.VendorID, current.ID))
		if err != nil {
			return mapDB(err, "Calibration", CodeCalibrationNotFound)
		}
		archived = row
		if _, err = bumpRevisionsForProduct(ctx, tx, actor.VendorID, current.ProductID); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "shop.measurement.calibration_archived", eventType: "shop.measurement.changed",
			aggregateType: "shop_measure_calibration", aggregateID: archived.ID, branchID: archived.BranchID,
			version: archived.Version, before: CalibrationViewOf(current), after: CalibrationViewOf(archived),
			payload: CalibrationViewOf(archived),
		})
	})
	return archived, err
}
