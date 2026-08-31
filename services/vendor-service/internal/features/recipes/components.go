package recipes

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func validateComponentInput(
	input IngredientComponentInput,
) error {
	if strings.TrimSpace(input.IngredientID) == "" ||
		input.QuantityNumerator <= 0 ||
		input.QuantityDenominator <= 0 ||
		len(normalizeCode(input.UnitCode)) == 0 ||
		len(normalizeCode(input.UnitCode)) > 80 ||
		input.ExpectedWasteBPS < 0 ||
		input.ExpectedWasteBPS >= 10000 ||
		input.Sequence < 0 ||
		len(strings.TrimSpace(input.PreparationNote)) > 1000 {
		return ErrInvalidInput
	}

	_, err :=
		normalizeRational(
			input.QuantityNumerator,
			input.QuantityDenominator,
		)

	return err
}

func validatePackagingInput(
	input PackagingRequirementInput,
) error {
	if strings.TrimSpace(input.PackagingMaterialID) == "" ||
		input.QuantityNumerator <= 0 ||
		input.QuantityDenominator <= 0 ||
		len(normalizeCode(input.UnitCode)) == 0 ||
		len(normalizeCode(input.UnitCode)) > 80 ||
		input.Sequence < 0 {
		return ErrInvalidInput
	}

	_, err :=
		normalizeRational(
			input.QuantityNumerator,
			input.QuantityDenominator,
		)

	return err
}

func (s *Service) ReplaceComponents(
	ctx context.Context,
	actor Actor,
	recipeID string,
	versionID string,
	expectedVersion int64,
	input ReplaceComponentsInput,
) (VersionView, error) {
	if len(input.Components) > 500 {
		return VersionView{}, ErrInvalidInput
	}

	seen :=
		make(
			map[string]struct{},
			len(input.Components),
		)

	for index := range input.Components {
		item := &input.Components[index]

		item.IngredientID =
			strings.TrimSpace(
				item.IngredientID,
			)

		item.UnitCode =
			normalizeCode(
				item.UnitCode,
			)

		item.PreparationNote =
			strings.TrimSpace(
				item.PreparationNote,
			)

		if err := validateComponentInput(*item); err != nil {
			return VersionView{}, err
		}

		if _, exists :=
			seen[item.IngredientID]; exists {
			return VersionView{}, ErrInvalidInput
		}

		seen[item.IngredientID] =
			struct{}{}
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return VersionView{}, err
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
                FOR UPDATE
                `,
				recipeID,
				actor.BusinessID,
			),
		)
	if err != nil {
		return VersionView{}, err
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
		return VersionView{}, err
	}

	if err =
		ensureDraftVersion(
			version,
			expectedVersion,
		); err != nil {
		return VersionView{}, err
	}

	for _, item := range input.Components {
		var exists bool

		if err = tx.QueryRow(
			ctx,
			`
            SELECT EXISTS(
                SELECT 1
                  FROM vendor_ingredients
                 WHERE id=$1
                   AND business_id=$2
                   AND status <> 'ARCHIVED'
            )
            `,
			item.IngredientID,
			actor.BusinessID,
		).Scan(&exists); err != nil {
			return VersionView{}, err
		}

		if !exists {
			return VersionView{}, ErrNotFound
		}
	}

	if _, err = tx.Exec(
		ctx,
		`
        DELETE FROM vendor_recipe_components
         WHERE business_id=$1
           AND recipe_version_id=$2
        `,
		actor.BusinessID,
		versionID,
	); err != nil {
		return VersionView{}, err
	}

	for _, item := range input.Components {
		if _, err = tx.Exec(
			ctx,
			`
            INSERT INTO vendor_recipe_components(
                id,
                business_id,
                recipe_version_id,
                ingredient_id,
                quantity_numerator,
                quantity_denominator,
                unit_code,
                expected_waste_bps,
                is_optional,
                sequence,
                preparation_note
            )
            VALUES(
                $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11
            )
            `,
			"rci_"+uuid.NewString(),
			actor.BusinessID,
			versionID,
			item.IngredientID,
			item.QuantityNumerator,
			item.QuantityDenominator,
			item.UnitCode,
			item.ExpectedWasteBPS,
			item.Optional,
			item.Sequence,
			item.PreparationNote,
		); err != nil {
			return VersionView{}, err
		}
	}

	if _, err = tx.Exec(
		ctx,
		`
        UPDATE vendor_recipe_versions
           SET version=version+1,
               updated_at=now()
         WHERE id=$1
           AND business_id=$2
           AND recipe_id=$3
           AND status='DRAFT'
           AND version=$4
        `,
		versionID,
		actor.BusinessID,
		recipeID,
		expectedVersion,
	); err != nil {
		return VersionView{}, err
	}

	if err = s.auditTx(
		ctx,
		tx,
		actor,
		"vendor.recipe.version.components_updated",
		"vendor_recipe_version",
		versionID,
		recipe.BranchID,
		map[string]any{
			"component_count": -1,
		},
		map[string]any{
			"component_count": len(input.Components),
		},
	); err != nil {
		return VersionView{}, err
	}

	if err = s.publishTx(
		ctx,
		tx,
		actor,
		"vendor.recipe.version.components_updated",
		"vendor_recipe_version",
		versionID,
		expectedVersion+1,
		map[string]any{
			"recipe_id":       recipeID,
			"version_id":      versionID,
			"branch_id":       recipe.BranchID,
			"component_count": len(input.Components),
		},
	); err != nil {
		return VersionView{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return VersionView{}, err
	}

	return s.GetVersionView(
		ctx,
		actor,
		recipeID,
		versionID,
	)
}

func (s *Service) ReplacePackaging(
	ctx context.Context,
	actor Actor,
	recipeID string,
	versionID string,
	expectedVersion int64,
	input ReplacePackagingInput,
) (VersionView, error) {
	if len(input.Requirements) > 100 {
		return VersionView{}, ErrInvalidInput
	}

	seen :=
		make(
			map[string]struct{},
			len(input.Requirements),
		)

	for index := range input.Requirements {
		item := &input.Requirements[index]

		item.PackagingMaterialID =
			strings.TrimSpace(
				item.PackagingMaterialID,
			)

		item.UnitCode =
			normalizeCode(
				item.UnitCode,
			)

		if err := validatePackagingInput(*item); err != nil {
			return VersionView{}, err
		}

		if _, exists :=
			seen[item.PackagingMaterialID]; exists {
			return VersionView{}, ErrInvalidInput
		}

		seen[item.PackagingMaterialID] =
			struct{}{}
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return VersionView{}, err
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
                FOR UPDATE
                `,
				recipeID,
				actor.BusinessID,
			),
		)
	if err != nil {
		return VersionView{}, err
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
		return VersionView{}, err
	}

	if err =
		ensureDraftVersion(
			version,
			expectedVersion,
		); err != nil {
		return VersionView{}, err
	}

	for _, item := range input.Requirements {
		var exists bool

		if err = tx.QueryRow(
			ctx,
			`
            SELECT EXISTS(
                SELECT 1
                  FROM vendor_packaging_materials
                 WHERE id=$1
                   AND business_id=$2
                   AND status <> 'ARCHIVED'
            )
            `,
			item.PackagingMaterialID,
			actor.BusinessID,
		).Scan(&exists); err != nil {
			return VersionView{}, err
		}

		if !exists {
			return VersionView{}, ErrNotFound
		}
	}

	if _, err = tx.Exec(
		ctx,
		`
        DELETE FROM vendor_recipe_packaging_requirements
         WHERE business_id=$1
           AND recipe_version_id=$2
        `,
		actor.BusinessID,
		versionID,
	); err != nil {
		return VersionView{}, err
	}

	for _, item := range input.Requirements {
		if _, err = tx.Exec(
			ctx,
			`
            INSERT INTO vendor_recipe_packaging_requirements(
                id,
                business_id,
                recipe_version_id,
                packaging_material_id,
                quantity_numerator,
                quantity_denominator,
                unit_code,
                sequence
            )
            VALUES(
                $1,$2,$3,$4,$5,$6,$7,$8
            )
            `,
			"rpk_"+uuid.NewString(),
			actor.BusinessID,
			versionID,
			item.PackagingMaterialID,
			item.QuantityNumerator,
			item.QuantityDenominator,
			item.UnitCode,
			item.Sequence,
		); err != nil {
			return VersionView{}, err
		}
	}

	if _, err = tx.Exec(
		ctx,
		`
        UPDATE vendor_recipe_versions
           SET version=version+1,
               updated_at=now()
         WHERE id=$1
           AND business_id=$2
           AND recipe_id=$3
           AND status='DRAFT'
           AND version=$4
        `,
		versionID,
		actor.BusinessID,
		recipeID,
		expectedVersion,
	); err != nil {
		return VersionView{}, err
	}

	if err = s.auditTx(
		ctx,
		tx,
		actor,
		"vendor.recipe.version.packaging_updated",
		"vendor_recipe_version",
		versionID,
		recipe.BranchID,
		nil,
		map[string]any{
			"packaging_count": len(input.Requirements),
		},
	); err != nil {
		return VersionView{}, err
	}

	if err = s.publishTx(
		ctx,
		tx,
		actor,
		"vendor.recipe.version.packaging_updated",
		"vendor_recipe_version",
		versionID,
		expectedVersion+1,
		map[string]any{
			"recipe_id":       recipeID,
			"version_id":      versionID,
			"branch_id":       recipe.BranchID,
			"packaging_count": len(input.Requirements),
		},
	); err != nil {
		return VersionView{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return VersionView{}, err
	}

	return s.GetVersionView(
		ctx,
		actor,
		recipeID,
		versionID,
	)
}

func scanComponent(
	row pgx.Row,
) (IngredientComponent, error) {
	var out IngredientComponent

	err := row.Scan(
		&out.ID,
		&out.BusinessID,
		&out.RecipeVersionID,
		&out.IngredientID,
		&out.QuantityNumerator,
		&out.QuantityDenominator,
		&out.UnitCode,
		&out.NormalizationStatus,
		&out.NormalizedQuantityNumerator,
		&out.NormalizedQuantityDenominator,
		&out.NormalizedUnitCode,
		&out.ExpectedWasteBPS,
		&out.Optional,
		&out.Sequence,
		&out.PreparationNote,
		&out.Version,
		&out.CreatedAt,
		&out.UpdatedAt,
	)

	return out, err
}

func scanPackaging(
	row pgx.Row,
) (PackagingRequirement, error) {
	var out PackagingRequirement

	err := row.Scan(
		&out.ID,
		&out.BusinessID,
		&out.RecipeVersionID,
		&out.PackagingMaterialID,
		&out.QuantityNumerator,
		&out.QuantityDenominator,
		&out.UnitCode,
		&out.NormalizationStatus,
		&out.NormalizedQuantityNumerator,
		&out.NormalizedQuantityDenominator,
		&out.NormalizedUnitCode,
		&out.Sequence,
		&out.Version,
		&out.CreatedAt,
		&out.UpdatedAt,
	)

	return out, err
}

func (s *Service) GetVersionView(
	ctx context.Context,
	actor Actor,
	recipeID string,
	versionID string,
) (VersionView, error) {
	version, err :=
		s.GetVersion(
			ctx,
			actor,
			recipeID,
			versionID,
		)
	if err != nil {
		return VersionView{}, err
	}

	componentRows, err :=
		s.db.Query(
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
			actor.BusinessID,
			versionID,
		)
	if err != nil {
		return VersionView{}, err
	}

	var components []IngredientComponent

	for componentRows.Next() {
		item, scanErr :=
			scanComponent(
				componentRows,
			)

		if scanErr != nil {
			componentRows.Close()
			return VersionView{}, scanErr
		}

		components =
			append(
				components,
				item,
			)
	}

	err = componentRows.Err()
	componentRows.Close()

	if err != nil {
		return VersionView{}, err
	}

	packagingRows, err :=
		s.db.Query(
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
			actor.BusinessID,
			versionID,
		)
	if err != nil {
		return VersionView{}, err
	}

	var packaging []PackagingRequirement

	for packagingRows.Next() {
		item, scanErr :=
			scanPackaging(
				packagingRows,
			)

		if scanErr != nil {
			packagingRows.Close()
			return VersionView{}, scanErr
		}

		packaging =
			append(
				packaging,
				item,
			)
	}

	err = packagingRows.Err()
	packagingRows.Close()

	if err != nil {
		return VersionView{}, err
	}

	return VersionView{
		Version:    version,
		Components: components,
		Packaging:  packaging,
	}, nil
}

var _ = errors.Is
