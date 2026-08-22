package recipes

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const versionColumns = `
    id,
    business_id,
    recipe_id,
    version_number,
    status,
    internal_notes,
    expected_output_numerator,
    expected_output_denominator,
    output_unit_code,
    serving_count,
    expected_waste_bps,
    activated_at,
    retired_at,
    version,
    created_at,
    updated_at
`

func scanVersion(
	row pgx.Row,
) (RecipeVersion, error) {
	var out RecipeVersion

	err := row.Scan(
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
		return RecipeVersion{}, ErrNotFound
	}

	return out, err
}

func (s *Service) CreateDraftVersion(
	ctx context.Context,
	actor Actor,
	recipeID string,
	input CreateVersionInput,
) (RecipeVersion, error) {
	input.OutputUnitCode =
		normalizeCode(
			input.OutputUnitCode,
		)

	input.InternalNotes =
		strings.TrimSpace(
			input.InternalNotes,
		)

	if input.ExpectedOutputDenominator == 0 {
		input.ExpectedOutputDenominator = 1
	}

	if err := validateVersionDefinition(input); err != nil {
		return RecipeVersion{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return RecipeVersion{}, err
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
		return RecipeVersion{}, err
	}

	if recipe.Status != RecipeStatusActive {
		return RecipeVersion{}, ErrInvalidState
	}

	var nextNumber int

	if err = tx.QueryRow(
		ctx,
		`
        SELECT
            COALESCE(
                max(version_number),
                0
            )+1
          FROM vendor_recipe_versions
         WHERE business_id=$1
           AND recipe_id=$2
        `,
		actor.BusinessID,
		recipeID,
	).Scan(&nextNumber); err != nil {
		return RecipeVersion{}, err
	}

	out := RecipeVersion{
		ID: "rcv_" + uuid.NewString(),

		BusinessID: actor.BusinessID,

		RecipeID: recipeID,

		VersionNumber: nextNumber,

		Status: VersionDraft,

		InternalNotes: input.InternalNotes,

		ExpectedOutputNumerator: input.ExpectedOutputNumerator,

		ExpectedOutputDenominator: input.ExpectedOutputDenominator,

		OutputUnitCode: input.OutputUnitCode,

		ServingCount: input.ServingCount,

		ExpectedWasteBPS: input.ExpectedWasteBPS,

		Version: 1,
	}

	err = tx.QueryRow(
		ctx,
		`
        INSERT INTO vendor_recipe_versions(
            id,
            business_id,
            recipe_id,
            version_number,
            status,
            internal_notes,
            expected_output_numerator,
            expected_output_denominator,
            output_unit_code,
            serving_count,
            expected_waste_bps
        )
        VALUES(
            $1,$2,$3,$4,'DRAFT',
            $5,$6,$7,$8,$9,$10
        )
        RETURNING
            created_at,
            updated_at
        `,
		out.ID,
		out.BusinessID,
		out.RecipeID,
		out.VersionNumber,
		out.InternalNotes,
		out.ExpectedOutputNumerator,
		out.ExpectedOutputDenominator,
		out.OutputUnitCode,
		out.ServingCount,
		out.ExpectedWasteBPS,
	).Scan(
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		return RecipeVersion{}, err
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
		return RecipeVersion{}, err
	}

	if err = s.auditTx(
		ctx,
		tx,
		actor,
		"vendor.recipe.version.created",
		"vendor_recipe_version",
		out.ID,
		recipe.BranchID,
		nil,
		out,
	); err != nil {
		return RecipeVersion{}, err
	}

	if err = s.publishTx(
		ctx,
		tx,
		actor,
		"vendor.recipe.version.created",
		"vendor_recipe_version",
		out.ID,
		out.Version,
		map[string]any{
			"recipe_id":      recipeID,
			"version_id":     out.ID,
			"version_number": out.VersionNumber,
			"branch_id":      recipe.BranchID,
		},
	); err != nil {
		return RecipeVersion{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return RecipeVersion{}, err
	}

	return out, nil
}

func (s *Service) GetVersion(
	ctx context.Context,
	actor Actor,
	recipeID string,
	versionID string,
) (RecipeVersion, error) {
	return scanVersion(
		s.db.QueryRow(
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
}

func (s *Service) ListVersions(
	ctx context.Context,
	actor Actor,
	recipeID string,
) ([]RecipeVersion, error) {
	if _, err := s.GetRecipe(
		ctx,
		actor,
		recipeID,
	); err != nil {
		return nil, err
	}

	rows, err := s.db.Query(
		ctx,
		`
        SELECT
        `+versionColumns+`
          FROM vendor_recipe_versions
         WHERE business_id=$1
           AND recipe_id=$2
         ORDER BY version_number DESC,id
        `,
		actor.BusinessID,
		recipeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RecipeVersion

	for rows.Next() {
		item, scanErr :=
			scanVersion(rows)

		if scanErr != nil {
			return nil, scanErr
		}

		out = append(out, item)
	}

	return out, rows.Err()
}

func (s *Service) UpdateDraftVersion(
	ctx context.Context,
	actor Actor,
	recipeID string,
	versionID string,
	expectedVersion int64,
	input UpdateVersionInput,
) (RecipeVersion, error) {
	if expectedVersion <= 0 {
		return RecipeVersion{}, ErrInvalidInput
	}

	if input.ExpectedOutputDenominator == 0 {
		input.ExpectedOutputDenominator = 1
	}

	input.OutputUnitCode =
		normalizeCode(
			input.OutputUnitCode,
		)

	input.InternalNotes =
		strings.TrimSpace(
			input.InternalNotes,
		)

	if err := validateVersionDefinition(input); err != nil {
		return RecipeVersion{}, err
	}

	var out RecipeVersion

	err := s.db.QueryRow(
		ctx,
		`
        UPDATE vendor_recipe_versions
           SET internal_notes=$5,
               expected_output_numerator=$6,
               expected_output_denominator=$7,
               output_unit_code=$8,
               serving_count=$9,
               expected_waste_bps=$10,
               version=version+1,
               updated_at=now()
         WHERE id=$1
           AND recipe_id=$2
           AND business_id=$3
           AND status='DRAFT'
           AND version=$4
        RETURNING
        `+versionColumns,
		versionID,
		recipeID,
		actor.BusinessID,
		expectedVersion,
		input.InternalNotes,
		input.ExpectedOutputNumerator,
		input.ExpectedOutputDenominator,
		input.OutputUnitCode,
		input.ServingCount,
		input.ExpectedWasteBPS,
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
		current, loadErr :=
			s.GetVersion(
				ctx,
				actor,
				recipeID,
				versionID,
			)

		if loadErr != nil {
			return RecipeVersion{}, loadErr
		}

		if current.Status != VersionDraft {
			return RecipeVersion{}, ErrInvalidState
		}

		return RecipeVersion{}, ErrVersionConflict
	}

	return out, err
}

func (s *Service) RetireVersion(
	ctx context.Context,
	actor Actor,
	recipeID string,
	versionID string,
	expectedVersion int64,
) (RecipeVersion, error) {
	if expectedVersion <= 0 {
		return RecipeVersion{}, ErrInvalidInput
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return RecipeVersion{}, err
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
		return RecipeVersion{}, err
	}

	current, err :=
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
		return RecipeVersion{}, err
	}

	if current.Version != expectedVersion {
		return RecipeVersion{}, ErrVersionConflict
	}

	if current.Status == VersionRetired {
		return current, nil
	}

	if current.Status != VersionActive {
		return RecipeVersion{}, ErrInvalidState
	}

	var out RecipeVersion

	err = tx.QueryRow(
		ctx,
		`
        UPDATE vendor_recipe_versions
           SET status='RETIRED',
               retired_at=now(),
               version=version+1,
               updated_at=now()
         WHERE id=$1
           AND recipe_id=$2
           AND business_id=$3
           AND status='ACTIVE'
           AND version=$4
        RETURNING
        `+versionColumns,
		versionID,
		recipeID,
		actor.BusinessID,
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
		return RecipeVersion{}, ErrVersionConflict
	}
	if err != nil {
		return RecipeVersion{}, err
	}

	if err = s.auditTx(
		ctx,
		tx,
		actor,
		"vendor.recipe.version.retired",
		"vendor_recipe_version",
		out.ID,
		recipe.BranchID,
		current,
		out,
	); err != nil {
		return RecipeVersion{}, err
	}

	if err = s.publishTx(
		ctx,
		tx,
		actor,
		"vendor.recipe.version.retired",
		"vendor_recipe_version",
		out.ID,
		out.Version,
		map[string]any{
			"recipe_id":  recipeID,
			"version_id": out.ID,
			"branch_id":  recipe.BranchID,
		},
	); err != nil {
		return RecipeVersion{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return RecipeVersion{}, err
	}

	return out, nil
}

func (s *Service) ResolveActiveVersion(
	ctx context.Context,
	businessID string,
	targetType string,
	targetRef string,
	variantRef string,
) (Recipe, RecipeVersion, error) {
	actor :=
		Actor{
			BusinessID: businessID,
		}

	var recipe Recipe

	err := s.db.QueryRow(
		ctx,
		`
        SELECT
        `+recipeColumns+`
          FROM vendor_recipes
         WHERE business_id=$1
           AND target_type=$2
           AND target_ref=$3
           AND target_variant_ref=$4
           AND status='ACTIVE'
        `,
		businessID,
		normalizeEnum(targetType),
		strings.TrimSpace(targetRef),
		strings.TrimSpace(variantRef),
	).Scan(
		&recipe.ID,
		&recipe.BusinessID,
		&recipe.BranchID,
		&recipe.TargetType,
		&recipe.TargetRef,
		&recipe.TargetVariantRef,
		&recipe.Name,
		&recipe.InternalDescription,
		&recipe.Status,
		&recipe.Version,
		&recipe.CreatedAt,
		&recipe.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Recipe{}, RecipeVersion{}, ErrNotFound
	}
	if err != nil {
		return Recipe{}, RecipeVersion{}, err
	}

	version, err :=
		scanVersion(
			s.db.QueryRow(
				ctx,
				`
                SELECT
                `+versionColumns+`
                  FROM vendor_recipe_versions
                 WHERE business_id=$1
                   AND recipe_id=$2
                   AND status='ACTIVE'
                `,
				businessID,
				recipe.ID,
			),
		)
	if err != nil {
		return Recipe{}, RecipeVersion{}, err
	}

	_ = actor

	return recipe, version, nil
}

func ensureDraftVersion(
	version RecipeVersion,
	expectedVersion int64,
) error {
	if expectedVersion <= 0 {
		return ErrInvalidInput
	}

	if version.Status != VersionDraft {
		return ErrInvalidState
	}

	if version.Version != expectedVersion {
		return ErrVersionConflict
	}

	return nil
}

func invalidVersionMessage(
	version RecipeVersion,
) error {
	return fmt.Errorf(
		"%w: version %d is %s",
		ErrInvalidState,
		version.VersionNumber,
		version.Status,
	)
}
