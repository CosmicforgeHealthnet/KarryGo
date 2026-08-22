package recipes

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

type masterNormalization struct {
	Status string

	Numerator   int64
	Denominator int64
	UnitCode    string
}

func effectiveControl(
	base string,
	override string,
) string {
	override =
		normalizeEnum(
			override,
		)

	if override != "" {
		return override
	}

	return normalizeEnum(base)
}

func (s *Service) normalizeIngredientTx(
	ctx context.Context,
	tx pgx.Tx,
	businessID string,
	branchID string,
	component IngredientComponent,
) (masterNormalization, []ValidationIssue, error) {
	var defaultUnit string
	var control string
	var status string

	err := tx.QueryRow(
		ctx,
		`
        SELECT
            default_unit_code,
            inventory_control_mode,
            status
          FROM vendor_ingredients
         WHERE id=$1
           AND business_id=$2
        `,
		component.IngredientID,
		businessID,
	).Scan(
		&defaultUnit,
		&control,
		&status,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return masterNormalization{},
			[]ValidationIssue{{
				Code:    "INGREDIENT_NOT_FOUND",
				Field:   "components.ingredient_id",
				Message: "Ingredient does not belong to this Vendor.",
			}},
			nil
	}
	if err != nil {
		return masterNormalization{}, nil, err
	}

	if status != "ACTIVE" {
		return masterNormalization{},
			[]ValidationIssue{{
				Code:    "INGREDIENT_NOT_ACTIVE",
				Field:   "components.ingredient_id",
				Message: "Ingredient must be ACTIVE before recipe activation.",
			}},
			nil
	}

	enabled := true
	productionEligible := true
	override := ""
	inventoryItemID := ""

	err = tx.QueryRow(
		ctx,
		`
        SELECT
            enabled,
            production_eligible,
            COALESCE(
                inventory_control_mode_override,
                ''
            ),
            COALESCE(
                inventory_item_id,
                ''
            )
          FROM vendor_ingredient_branch_configs
         WHERE business_id=$1
           AND branch_id=$2
           AND ingredient_id=$3
        `,
		businessID,
		branchID,
		component.IngredientID,
	).Scan(
		&enabled,
		&productionEligible,
		&override,
		&inventoryItemID,
	)

	if err != nil &&
		!errors.Is(err, pgx.ErrNoRows) {
		return masterNormalization{}, nil, err
	}

	mode :=
		effectiveControl(
			control,
			override,
		)

	if mode == ControlControlled {
		if !enabled ||
			!productionEligible ||
			strings.TrimSpace(inventoryItemID) == "" {
			return masterNormalization{},
				[]ValidationIssue{{
					Code:    "INGREDIENT_INVENTORY_MAPPING_REQUIRED",
					Field:   "components.ingredient_id",
					Message: "Inventory-controlled Ingredient requires an enabled production-eligible branch Inventory mapping.",
				}},
				nil
		}
	}

	inputQuantity, err :=
		normalizeRational(
			component.QuantityNumerator,
			component.QuantityDenominator,
		)
	if err != nil {
		return masterNormalization{}, nil, err
	}

	defaultUnit =
		normalizeCode(
			defaultUnit,
		)

	inputUnit :=
		normalizeCode(
			component.UnitCode,
		)

	if inputUnit == defaultUnit {
		return masterNormalization{
			Status:      NormalizationExact,
			Numerator:   inputQuantity.N,
			Denominator: inputQuantity.D,
			UnitCode:    defaultUnit,
		}, nil, nil
	}

	var conversionN int64
	var conversionD int64

	err = tx.QueryRow(
		ctx,
		`
        SELECT
            numerator,
            denominator
          FROM vendor_ingredient_unit_conversions
         WHERE business_id=$1
           AND ingredient_id=$2
           AND from_unit_code=$3
           AND to_unit_code=$4
           AND conversion_mode='EXACT_AUTHORITATIVE'
           AND status='ACTIVE'
         ORDER BY version DESC,id
         LIMIT 1
        `,
		businessID,
		component.IngredientID,
		inputUnit,
		defaultUnit,
	).Scan(
		&conversionN,
		&conversionD,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		if mode == ControlControlled {
			return masterNormalization{},
				[]ValidationIssue{{
					Code:    "EXACT_INGREDIENT_CONVERSION_REQUIRED",
					Field:   "components.unit_code",
					Message: "Inventory-controlled Ingredient requires an exact authoritative conversion into its configured usage basis.",
				}},
				nil
		}

		return masterNormalization{
			Status:      NormalizationUnresolved,
			Numerator:   0,
			Denominator: 1,
			UnitCode:    "",
		}, nil, nil
	}
	if err != nil {
		return masterNormalization{}, nil, err
	}

	normalized, err :=
		multiplyRational(
			inputQuantity,
			rational{
				N: conversionN,
				D: conversionD,
			},
		)
	if err != nil {
		return masterNormalization{}, nil, err
	}

	return masterNormalization{
		Status:      NormalizationExact,
		Numerator:   normalized.N,
		Denominator: normalized.D,
		UnitCode:    defaultUnit,
	}, nil, nil
}

func (s *Service) normalizePackagingTx(
	ctx context.Context,
	tx pgx.Tx,
	businessID string,
	branchID string,
	item PackagingRequirement,
) (masterNormalization, []ValidationIssue, error) {
	var defaultUnit string
	var control string
	var status string

	err := tx.QueryRow(
		ctx,
		`
        SELECT
            default_unit_code,
            inventory_control_mode,
            status
          FROM vendor_packaging_materials
         WHERE id=$1
           AND business_id=$2
        `,
		item.PackagingMaterialID,
		businessID,
	).Scan(
		&defaultUnit,
		&control,
		&status,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return masterNormalization{},
			[]ValidationIssue{{
				Code:    "PACKAGING_NOT_FOUND",
				Field:   "packaging_requirements.packaging_material_id",
				Message: "Packaging Material does not belong to this Vendor.",
			}},
			nil
	}
	if err != nil {
		return masterNormalization{}, nil, err
	}

	if status != "ACTIVE" {
		return masterNormalization{},
			[]ValidationIssue{{
				Code:    "PACKAGING_NOT_ACTIVE",
				Field:   "packaging_requirements.packaging_material_id",
				Message: "Packaging Material must be ACTIVE before recipe activation.",
			}},
			nil
	}

	enabled := true
	productionEligible := true
	override := ""
	inventoryItemID := ""

	err = tx.QueryRow(
		ctx,
		`
        SELECT
            enabled,
            production_eligible,
            COALESCE(
                inventory_control_mode_override,
                ''
            ),
            COALESCE(
                inventory_item_id,
                ''
            )
          FROM vendor_packaging_branch_configs
         WHERE business_id=$1
           AND branch_id=$2
           AND packaging_material_id=$3
        `,
		businessID,
		branchID,
		item.PackagingMaterialID,
	).Scan(
		&enabled,
		&productionEligible,
		&override,
		&inventoryItemID,
	)

	if err != nil &&
		!errors.Is(err, pgx.ErrNoRows) {
		return masterNormalization{}, nil, err
	}

	mode :=
		effectiveControl(
			control,
			override,
		)

	if mode == ControlControlled {
		if !enabled ||
			!productionEligible ||
			strings.TrimSpace(inventoryItemID) == "" {
			return masterNormalization{},
				[]ValidationIssue{{
					Code:    "PACKAGING_INVENTORY_MAPPING_REQUIRED",
					Field:   "packaging_requirements.packaging_material_id",
					Message: "Inventory-controlled Packaging requires an enabled production-eligible branch Inventory mapping.",
				}},
				nil
		}
	}

	inputQuantity, err :=
		normalizeRational(
			item.QuantityNumerator,
			item.QuantityDenominator,
		)
	if err != nil {
		return masterNormalization{}, nil, err
	}

	defaultUnit =
		normalizeCode(defaultUnit)

	inputUnit :=
		normalizeCode(
			item.UnitCode,
		)

	if inputUnit == defaultUnit {
		return masterNormalization{
			Status:      NormalizationExact,
			Numerator:   inputQuantity.N,
			Denominator: inputQuantity.D,
			UnitCode:    defaultUnit,
		}, nil, nil
	}

	var conversionN int64
	var conversionD int64

	err = tx.QueryRow(
		ctx,
		`
        SELECT
            numerator,
            denominator
          FROM vendor_packaging_unit_conversions
         WHERE business_id=$1
           AND packaging_material_id=$2
           AND from_unit_code=$3
           AND to_unit_code=$4
           AND conversion_mode='EXACT_AUTHORITATIVE'
           AND status='ACTIVE'
         ORDER BY version DESC,id
         LIMIT 1
        `,
		businessID,
		item.PackagingMaterialID,
		inputUnit,
		defaultUnit,
	).Scan(
		&conversionN,
		&conversionD,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		if mode == ControlControlled {
			return masterNormalization{},
				[]ValidationIssue{{
					Code:    "EXACT_PACKAGING_CONVERSION_REQUIRED",
					Field:   "packaging_requirements.unit_code",
					Message: "Inventory-controlled Packaging requires an exact authoritative conversion into its configured usage basis.",
				}},
				nil
		}

		return masterNormalization{
			Status:      NormalizationUnresolved,
			Numerator:   0,
			Denominator: 1,
			UnitCode:    "",
		}, nil, nil
	}
	if err != nil {
		return masterNormalization{}, nil, err
	}

	normalized, err :=
		multiplyRational(
			inputQuantity,
			rational{
				N: conversionN,
				D: conversionD,
			},
		)
	if err != nil {
		return masterNormalization{}, nil, err
	}

	return masterNormalization{
		Status:      NormalizationExact,
		Numerator:   normalized.N,
		Denominator: normalized.D,
		UnitCode:    defaultUnit,
	}, nil, nil
}

func loadComponentsTx(
	ctx context.Context,
	tx pgx.Tx,
	businessID string,
	versionID string,
) ([]IngredientComponent, error) {
	rows, err :=
		tx.Query(
			ctx,
			`
            SELECT
                id,
                business_id,
                recipe_version_id,
                ingredient_id,
                quantity_numerator,
                quantity_denominator,
                unit_code,
                normalization_status,
                normalized_quantity_numerator,
                normalized_quantity_denominator,
                normalized_unit_code,
                expected_waste_bps,
                is_optional,
                sequence,
                preparation_note,
                version,
                created_at,
                updated_at
              FROM vendor_recipe_components
             WHERE business_id=$1
               AND recipe_version_id=$2
             ORDER BY sequence,id
            `,
			businessID,
			versionID,
		)
	if err != nil {
		return nil, err
	}

	var out []IngredientComponent

	for rows.Next() {
		item, scanErr :=
			scanComponent(rows)

		if scanErr != nil {
			rows.Close()
			return nil, scanErr
		}

		out = append(out, item)
	}

	err = rows.Err()
	rows.Close()

	return out, err
}

func loadPackagingTx(
	ctx context.Context,
	tx pgx.Tx,
	businessID string,
	versionID string,
) ([]PackagingRequirement, error) {
	rows, err :=
		tx.Query(
			ctx,
			`
            SELECT
                id,
                business_id,
                recipe_version_id,
                packaging_material_id,
                quantity_numerator,
                quantity_denominator,
                unit_code,
                normalization_status,
                normalized_quantity_numerator,
                normalized_quantity_denominator,
                normalized_unit_code,
                sequence,
                version,
                created_at,
                updated_at
              FROM vendor_recipe_packaging_requirements
             WHERE business_id=$1
               AND recipe_version_id=$2
             ORDER BY sequence,id
            `,
			businessID,
			versionID,
		)
	if err != nil {
		return nil, err
	}

	var out []PackagingRequirement

	for rows.Next() {
		item, scanErr :=
			scanPackaging(rows)

		if scanErr != nil {
			rows.Close()
			return nil, scanErr
		}

		out = append(out, item)
	}

	err = rows.Err()
	rows.Close()

	return out, err
}

func (s *Service) validateVersionTx(
	ctx context.Context,
	tx pgx.Tx,
	recipe Recipe,
	version RecipeVersion,
	persistNormalization bool,
) ([]ValidationIssue, error) {
	var issues []ValidationIssue

	target, err :=
		resolveTargetQ(
			ctx,
			tx,
			recipe.BusinessID,
			recipe.TargetType,
			recipe.TargetRef,
			recipe.TargetVariantRef,
		)
	if errors.Is(err, ErrTargetInvalid) {
		issues =
			append(
				issues,
				ValidationIssue{
					Code:    "TARGET_INVALID",
					Field:   "recipe.target",
					Message: "The recipe target no longer resolves to a valid Vendor catalogue entity.",
				},
			)
	} else if err != nil {
		return nil, err
	} else if target.BranchID != recipe.BranchID {
		issues =
			append(
				issues,
				ValidationIssue{
					Code:    "TARGET_BRANCH_MISMATCH",
					Field:   "recipe.target",
					Message: "The catalogue target branch no longer matches the recipe branch.",
				},
			)
	}

	if version.ExpectedOutputNumerator <= 0 ||
		version.ExpectedOutputDenominator <= 0 ||
		strings.TrimSpace(version.OutputUnitCode) == "" {
		issues =
			append(
				issues,
				ValidationIssue{
					Code:    "YIELD_INVALID",
					Field:   "expected_output",
					Message: "Expected recipe yield must be positive and have an output unit.",
				},
			)
	}

	if version.ExpectedWasteBPS < 0 ||
		version.ExpectedWasteBPS >= 10000 {
		issues =
			append(
				issues,
				ValidationIssue{
					Code:    "WASTE_INVALID",
					Field:   "expected_waste_bps",
					Message: "Expected waste must be between 0 and 9999 basis points.",
				},
			)
	}

	components, err :=
		loadComponentsTx(
			ctx,
			tx,
			recipe.BusinessID,
			version.ID,
		)
	if err != nil {
		return nil, err
	}

	if len(components) == 0 {
		issues =
			append(
				issues,
				ValidationIssue{
					Code:    "COMPONENTS_REQUIRED",
					Field:   "components",
					Message: "A recipe version must contain at least one Ingredient component before activation.",
				},
			)
	}

	for _, component := range components {
		normalization, componentIssues, normalizeErr :=
			s.normalizeIngredientTx(
				ctx,
				tx,
				recipe.BusinessID,
				recipe.BranchID,
				component,
			)
		if normalizeErr != nil {
			return nil, normalizeErr
		}

		issues =
			append(
				issues,
				componentIssues...,
			)

		if persistNormalization &&
			len(componentIssues) == 0 {
			if _, err = tx.Exec(
				ctx,
				`
                UPDATE vendor_recipe_components
                   SET normalization_status=$4,
                       normalized_quantity_numerator=$5,
                       normalized_quantity_denominator=$6,
                       normalized_unit_code=$7,
                       version=version+1,
                       updated_at=now()
                 WHERE id=$1
                   AND business_id=$2
                   AND recipe_version_id=$3
                `,
				component.ID,
				recipe.BusinessID,
				version.ID,
				normalization.Status,
				normalization.Numerator,
				normalization.Denominator,
				normalization.UnitCode,
			); err != nil {
				return nil, err
			}
		}
	}

	packaging, err :=
		loadPackagingTx(
			ctx,
			tx,
			recipe.BusinessID,
			version.ID,
		)
	if err != nil {
		return nil, err
	}

	for _, item := range packaging {
		normalization, itemIssues, normalizeErr :=
			s.normalizePackagingTx(
				ctx,
				tx,
				recipe.BusinessID,
				recipe.BranchID,
				item,
			)
		if normalizeErr != nil {
			return nil, normalizeErr
		}

		issues =
			append(
				issues,
				itemIssues...,
			)

		if persistNormalization &&
			len(itemIssues) == 0 {
			if _, err = tx.Exec(
				ctx,
				`
                UPDATE vendor_recipe_packaging_requirements
                   SET normalization_status=$4,
                       normalized_quantity_numerator=$5,
                       normalized_quantity_denominator=$6,
                       normalized_unit_code=$7,
                       version=version+1,
                       updated_at=now()
                 WHERE id=$1
                   AND business_id=$2
                   AND recipe_version_id=$3
                `,
				item.ID,
				recipe.BusinessID,
				version.ID,
				normalization.Status,
				normalization.Numerator,
				normalization.Denominator,
				normalization.UnitCode,
			); err != nil {
				return nil, err
			}
		}
	}

	return issues, nil
}

func (s *Service) ValidateVersion(
	ctx context.Context,
	actor Actor,
	recipeID string,
	versionID string,
) (ValidationResult, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return ValidationResult{}, err
	}
	defer tx.Rollback(ctx)

	recipe, err :=
		scanRecipe(
			tx.QueryRow(
				ctx,
				`
                SELECT
                `+recipeColumns+`
                  FROM vendor_recipes
                 WHERE id=$1
                   AND business_id=$2
                `,
				recipeID,
				actor.BusinessID,
			),
		)
	if err != nil {
		return ValidationResult{}, err
	}

	version, err :=
		scanVersion(
			tx.QueryRow(
				ctx,
				`
                SELECT
                `+versionColumns+`
                  FROM vendor_recipe_versions
                 WHERE id=$1
                   AND recipe_id=$2
                   AND business_id=$3
                `,
				versionID,
				recipeID,
				actor.BusinessID,
			),
		)
	if err != nil {
		return ValidationResult{}, err
	}

	issues, err :=
		s.validateVersionTx(
			ctx,
			tx,
			recipe,
			version,
			false,
		)
	if err != nil {
		return ValidationResult{}, err
	}

	return ValidationResult{
		Valid:  len(issues) == 0,
		Issues: issues,
	}, nil
}

func (s *Service) ActivateVersion(
	ctx context.Context,
	actor Actor,
	recipeID string,
	versionID string,
	expectedVersion int64,
) (RecipeVersion, []ValidationIssue, error) {
	if expectedVersion <= 0 {
		return RecipeVersion{}, nil, ErrInvalidInput
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return RecipeVersion{}, nil, err
	}
	defer tx.Rollback(ctx)

	// Lock the stable Recipe identity first. Concurrent activation attempts for
	// the same Recipe therefore serialize before either can retire/activate.
	recipe, err :=
		scanRecipe(
			tx.QueryRow(
				ctx,
				`
                SELECT
                `+recipeColumns+`
                  FROM vendor_recipes
                 WHERE id=$1
                   AND business_id=$2
                FOR UPDATE
                `,
				recipeID,
				actor.BusinessID,
			),
		)
	if err != nil {
		return RecipeVersion{}, nil, err
	}

	if recipe.Status != RecipeStatusActive {
		return RecipeVersion{}, nil, ErrInvalidState
	}

	version, err :=
		scanVersion(
			tx.QueryRow(
				ctx,
				`
                SELECT
                `+versionColumns+`
                  FROM vendor_recipe_versions
                 WHERE id=$1
                   AND recipe_id=$2
                   AND business_id=$3
                FOR UPDATE
                `,
				versionID,
				recipeID,
				actor.BusinessID,
			),
		)
	if err != nil {
		return RecipeVersion{}, nil, err
	}

	if err =
		ensureDraftVersion(
			version,
			expectedVersion,
		); err != nil {
		return RecipeVersion{}, nil, err
	}

	issues, err :=
		s.validateVersionTx(
			ctx,
			tx,
			recipe,
			version,
			true,
		)
	if err != nil {
		return RecipeVersion{}, nil, err
	}

	if len(issues) > 0 {
		return RecipeVersion{},
			issues,
			ErrValidationFailed
	}

	var oldActiveID string
	var oldActiveVersion int64

	oldErr :=
		tx.QueryRow(
			ctx,
			`
            SELECT
                id,
                version
              FROM vendor_recipe_versions
             WHERE business_id=$1
               AND recipe_id=$2
               AND status='ACTIVE'
            FOR UPDATE
            `,
			actor.BusinessID,
			recipeID,
		).Scan(
			&oldActiveID,
			&oldActiveVersion,
		)

	if oldErr != nil &&
		!errors.Is(oldErr, pgx.ErrNoRows) {
		return RecipeVersion{}, nil, oldErr
	}

	if oldActiveID != "" {
		if _, err = tx.Exec(
			ctx,
			`
            UPDATE vendor_recipe_versions
               SET status='RETIRED',
                   retired_at=now(),
                   version=version+1,
                   updated_at=now()
             WHERE id=$1
               AND business_id=$2
               AND recipe_id=$3
               AND status='ACTIVE'
            `,
			oldActiveID,
			actor.BusinessID,
			recipeID,
		); err != nil {
			return RecipeVersion{}, nil, err
		}

		if err = s.publishTx(
			ctx,
			tx,
			actor,
			"vendor.recipe.version.retired",
			"vendor_recipe_version",
			oldActiveID,
			oldActiveVersion+1,
			map[string]any{
				"recipe_id":  recipeID,
				"version_id": oldActiveID,
				"branch_id":  recipe.BranchID,
				"reason":     "SUPERSEDED",
			},
		); err != nil {
			return RecipeVersion{}, nil, err
		}
	}

	var out RecipeVersion

	err = tx.QueryRow(
		ctx,
		`
        UPDATE vendor_recipe_versions
           SET status='ACTIVE',
               activated_at=now(),
               retired_at=NULL,
               version=version+1,
               updated_at=now()
         WHERE id=$1
           AND business_id=$2
           AND recipe_id=$3
           AND status='DRAFT'
           AND version=$4
        RETURNING
        `+versionColumns,
		versionID,
		actor.BusinessID,
		recipeID,
		expectedVersion,
	).Scan(
		&out.ID,
		&out.BusinessID,
		&out.RecipeID,
		&out.VersionNumber,
		&out.Status,
		&out.InternalNotes,
		&out.ExpectedOutputNumerator,
		&out.ExpectedOutputDenominator,
		&out.OutputUnitCode,
		&out.ServingCount,
		&out.ExpectedWasteBPS,
		&out.ActivatedAt,
		&out.RetiredAt,
		&out.Version,
		&out.CreatedAt,
		&out.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return RecipeVersion{}, nil, ErrVersionConflict
	}
	if err != nil {
		return RecipeVersion{}, nil, err
	}

	if _, err = tx.Exec(
		ctx,
		`
        UPDATE vendor_recipes
           SET version=version+1,
               updated_at=now()
         WHERE id=$1
           AND business_id=$2
        `,
		recipeID,
		actor.BusinessID,
	); err != nil {
		return RecipeVersion{}, nil, err
	}

	if err = s.auditTx(
		ctx,
		tx,
		actor,
		"vendor.recipe.version.activated",
		"vendor_recipe_version",
		out.ID,
		recipe.BranchID,
		version,
		out,
	); err != nil {
		return RecipeVersion{}, nil, err
	}

	if err = s.publishTx(
		ctx,
		tx,
		actor,
		"vendor.recipe.version.activated",
		"vendor_recipe_version",
		out.ID,
		out.Version,
		map[string]any{
			"recipe_id":          recipeID,
			"version_id":         out.ID,
			"version_number":     out.VersionNumber,
			"branch_id":          recipe.BranchID,
			"target_type":        recipe.TargetType,
			"target_ref":         recipe.TargetRef,
			"target_variant_ref": recipe.TargetVariantRef,
		},
	); err != nil {
		return RecipeVersion{}, nil, err
	}

	if err = tx.Commit(ctx); err != nil {
		return RecipeVersion{}, nil, err
	}

	return out, nil, nil
}

func (s *Service) ScaleVersion(
	ctx context.Context,
	actor Actor,
	recipeID string,
	versionID string,
	input ScaleInput,
) (ScaleResult, error) {
	if input.OutputDenominator == 0 {
		input.OutputDenominator = 1
	}

	input.OutputUnitCode =
		normalizeCode(
			input.OutputUnitCode,
		)

	if input.OutputNumerator <= 0 ||
		input.OutputDenominator <= 0 ||
		input.OutputUnitCode == "" {
		return ScaleResult{}, ErrInvalidInput
	}

	view, err :=
		s.GetVersionView(
			ctx,
			actor,
			recipeID,
			versionID,
		)
	if err != nil {
		return ScaleResult{}, err
	}

	if input.OutputUnitCode !=
		normalizeCode(
			view.Version.OutputUnitCode,
		) {
		return ScaleResult{}, ErrInvalidInput
	}

	requested, err :=
		normalizeRational(
			input.OutputNumerator,
			input.OutputDenominator,
		)
	if err != nil {
		return ScaleResult{}, err
	}

	expected, err :=
		normalizeRational(
			view.Version.ExpectedOutputNumerator,
			view.Version.ExpectedOutputDenominator,
		)
	if err != nil {
		return ScaleResult{}, err
	}

	multiplier, err :=
		divideRational(
			requested,
			expected,
		)
	if err != nil {
		return ScaleResult{}, err
	}

	result :=
		ScaleResult{
			RecipeVersionID: versionID,

			MultiplierNumerator: multiplier.N,

			MultiplierDenominator: multiplier.D,
		}

	for _, component := range view.Components {
		base :=
			rational{
				N: component.QuantityNumerator,
				D: component.QuantityDenominator,
			}

		unit :=
			component.UnitCode

		status :=
			component.NormalizationStatus

		if component.NormalizationStatus ==
			NormalizationExact {
			base =
				rational{
					N: component.NormalizedQuantityNumerator,
					D: component.NormalizedQuantityDenominator,
				}

			unit =
				component.NormalizedUnitCode
		}

		scaled, scaleErr :=
			multiplyRational(
				base,
				multiplier,
			)
		if scaleErr != nil {
			return ScaleResult{}, scaleErr
		}

		waste :=
			component.ExpectedWasteBPS

		if waste == 0 {
			waste =
				view.Version.ExpectedWasteBPS
		}

		scaled, scaleErr =
			applyExpectedWaste(
				scaled,
				waste,
			)
		if scaleErr != nil {
			return ScaleResult{}, scaleErr
		}

		result.Requirements =
			append(
				result.Requirements,
				ScaledRequirement{
					Kind: "INGREDIENT",

					ReferenceID: component.IngredientID,

					QuantityNumerator: scaled.N,

					QuantityDenominator: scaled.D,

					UnitCode: unit,

					ExpectedWasteBPS: waste,

					NormalizationStatus: status,
				},
			)
	}

	for _, item := range view.Packaging {
		base :=
			rational{
				N: item.QuantityNumerator,
				D: item.QuantityDenominator,
			}

		unit :=
			item.UnitCode

		status :=
			item.NormalizationStatus

		if item.NormalizationStatus ==
			NormalizationExact {
			base =
				rational{
					N: item.NormalizedQuantityNumerator,
					D: item.NormalizedQuantityDenominator,
				}

			unit =
				item.NormalizedUnitCode
		}

		scaled, scaleErr :=
			multiplyRational(
				base,
				multiplier,
			)
		if scaleErr != nil {
			return ScaleResult{}, scaleErr
		}

		result.Requirements =
			append(
				result.Requirements,
				ScaledRequirement{
					Kind: "PACKAGING",

					ReferenceID: item.PackagingMaterialID,

					QuantityNumerator: scaled.N,

					QuantityDenominator: scaled.D,

					UnitCode: unit,

					NormalizationStatus: status,
				},
			)
	}

	return result, nil
}

var _ = fmt.Sprintf
