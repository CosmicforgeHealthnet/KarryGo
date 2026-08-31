package recipes

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type EventPublisher interface {
	Publish(
		ctx context.Context,
		tx pgx.Tx,
		eventType string,
		aggregateType string,
		aggregateID string,
		aggregateVersion int64,
		vendorID string,
		correlationID string,
		causationID string,
		payload any,
	) error
}

type Service struct {
	db     *pgxpool.Pool
	events EventPublisher
}

func NewService(
	db *pgxpool.Pool,
	events EventPublisher,
) *Service {
	return &Service{
		db:     db,
		events: events,
	}
}

func validateRecipeInput(
	input CreateRecipeInput,
) error {
	if !validTargetType(input.TargetType) ||
		len(strings.TrimSpace(input.TargetRef)) == 0 ||
		len(strings.TrimSpace(input.TargetRef)) > 200 ||
		len(strings.TrimSpace(input.TargetVariantRef)) > 200 ||
		len(strings.TrimSpace(input.Name)) == 0 ||
		len(strings.TrimSpace(input.Name)) > 200 ||
		len(strings.TrimSpace(input.InternalDescription)) > 4000 {
		return ErrInvalidInput
	}

	return nil
}

func validateVersionDefinition(
	input CreateVersionInput,
) error {
	if input.ExpectedOutputNumerator <= 0 ||
		input.ExpectedOutputDenominator <= 0 ||
		len(normalizeCode(input.OutputUnitCode)) == 0 ||
		len(normalizeCode(input.OutputUnitCode)) > 80 ||
		input.ServingCount < 0 ||
		input.ExpectedWasteBPS < 0 ||
		input.ExpectedWasteBPS >= 10000 ||
		len(strings.TrimSpace(input.InternalNotes)) > 4000 {
		return ErrInvalidInput
	}

	_, err := normalizeRational(
		input.ExpectedOutputNumerator,
		input.ExpectedOutputDenominator,
	)

	return err
}

func (s *Service) auditTx(
	ctx context.Context,
	tx pgx.Tx,
	actor Actor,
	action string,
	aggregateType string,
	aggregateID string,
	branchID string,
	before any,
	after any,
) error {
	_, err := tx.Exec(
		ctx,
		`
        INSERT INTO vendor_audit_log(
            id,
            vendor_id,
            actor_type,
            actor_id,
            action,
            aggregate_type,
            aggregate_id,
            branch_id,
            before_data,
            after_data
        )
        VALUES(
            $1,$2,'vendor_staff',
            NULLIF($3,''),
            $4,$5,$6,
            NULLIF($7,''),
            COALESCE(to_jsonb($8::text),'{}'::jsonb),
            COALESCE(to_jsonb($9::text),'{}'::jsonb)
        )
        `,
		"va_"+uuid.NewString(),
		actor.BusinessID,
		actor.StaffID,
		action,
		aggregateType,
		aggregateID,
		branchID,
		fmt.Sprintf("%v", before),
		fmt.Sprintf("%v", after),
	)

	return err
}

func (s *Service) publishTx(
	ctx context.Context,
	tx pgx.Tx,
	actor Actor,
	eventType string,
	aggregateType string,
	aggregateID string,
	version int64,
	payload any,
) error {
	if s.events == nil {
		return nil
	}

	return s.events.Publish(
		ctx,
		tx,
		eventType,
		aggregateType,
		aggregateID,
		version,
		actor.BusinessID,
		actor.CorrelationID,
		actor.CausationID,
		payload,
	)
}

const recipeColumns = `
    id,
    business_id,
    branch_id,
    target_type,
    target_ref,
    target_variant_ref,
    name,
    internal_description,
    status,
    version,
    created_at,
    updated_at
`

func scanRecipe(
	row pgx.Row,
) (Recipe, error) {
	var out Recipe

	err := row.Scan(
		&out.ID,
		&out.BusinessID,
		&out.BranchID,
		&out.TargetType,
		&out.TargetRef,
		&out.TargetVariantRef,
		&out.Name,
		&out.InternalDescription,
		&out.Status,
		&out.Version,
		&out.CreatedAt,
		&out.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return Recipe{}, ErrNotFound
	}

	return out, err
}

func (s *Service) CreateRecipe(
	ctx context.Context,
	actor Actor,
	input CreateRecipeInput,
) (Recipe, error) {
	input.TargetType =
		normalizeEnum(
			input.TargetType,
		)

	input.TargetRef =
		strings.TrimSpace(
			input.TargetRef,
		)

	input.TargetVariantRef =
		strings.TrimSpace(
			input.TargetVariantRef,
		)

	input.Name =
		strings.TrimSpace(
			input.Name,
		)

	input.InternalDescription =
		strings.TrimSpace(
			input.InternalDescription,
		)

	if err := validateRecipeInput(input); err != nil {
		return Recipe{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Recipe{}, err
	}
	defer tx.Rollback(ctx)

	target, err :=
		resolveTargetQ(
			ctx,
			tx,
			actor.BusinessID,
			input.TargetType,
			input.TargetRef,
			input.TargetVariantRef,
		)
	if err != nil {
		return Recipe{}, err
	}

	out := Recipe{
		ID: "rcp_" + uuid.NewString(),

		BusinessID: actor.BusinessID,

		BranchID: target.BranchID,

		TargetType: input.TargetType,

		TargetRef: input.TargetRef,

		TargetVariantRef: input.TargetVariantRef,

		Name: input.Name,

		InternalDescription: input.InternalDescription,

		Status: RecipeStatusActive,

		Version: 1,
	}

	err = tx.QueryRow(
		ctx,
		`
        INSERT INTO vendor_recipes(
            id,
            business_id,
            branch_id,
            target_type,
            target_ref,
            target_variant_ref,
            name,
            internal_description,
            status
        )
        VALUES(
            $1,$2,$3,$4,$5,$6,$7,$8,'ACTIVE'
        )
        RETURNING
            created_at,
            updated_at
        `,
		out.ID,
		out.BusinessID,
		out.BranchID,
		out.TargetType,
		out.TargetRef,
		out.TargetVariantRef,
		out.Name,
		out.InternalDescription,
	).Scan(
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		return Recipe{}, err
	}

	if err = s.auditTx(
		ctx,
		tx,
		actor,
		"vendor.recipe.created",
		"vendor_recipe",
		out.ID,
		out.BranchID,
		nil,
		out,
	); err != nil {
		return Recipe{}, err
	}

	if err = s.publishTx(
		ctx,
		tx,
		actor,
		"vendor.recipe.created",
		"vendor_recipe",
		out.ID,
		out.Version,
		map[string]any{
			"recipe_id":   out.ID,
			"branch_id":   out.BranchID,
			"target_type": out.TargetType,
			"target_ref":  out.TargetRef,
			"variant_ref": out.TargetVariantRef,
		},
	); err != nil {
		return Recipe{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return Recipe{}, err
	}

	return out, nil
}

func (s *Service) GetRecipe(
	ctx context.Context,
	actor Actor,
	recipeID string,
) (Recipe, error) {
	return scanRecipe(
		s.db.QueryRow(
			ctx,
			`
            SELECT
            `+recipeColumns+`
              FROM vendor_recipes
             WHERE id=$1
               AND business_id=$2
            `,
			strings.TrimSpace(recipeID),
			actor.BusinessID,
		),
	)
}

func (s *Service) ListRecipes(
	ctx context.Context,
	actor Actor,
	query string,
	targetType string,
	status string,
	limit int,
	offset int,
) ([]Recipe, error) {
	if limit <= 0 ||
		limit > 200 {
		limit = 50
	}

	if offset < 0 ||
		offset > 1_000_000 {
		offset = 0
	}

	query =
		strings.TrimSpace(query)

	targetType =
		normalizeEnum(targetType)

	status =
		normalizeEnum(status)

	if targetType != "" &&
		!validTargetType(targetType) {
		return nil, ErrInvalidInput
	}

	rows, err := s.db.Query(
		ctx,
		`
        SELECT
        `+recipeColumns+`
          FROM vendor_recipes
         WHERE business_id=$1
           AND ($2='' OR target_type=$2)
           AND ($3='' OR status=$3)
           AND (
               $4=''
               OR name ILIKE '%' || $4 || '%'
               OR target_ref ILIKE '%' || $4 || '%'
           )
         ORDER BY updated_at DESC,id
         LIMIT $5
         OFFSET $6
        `,
		actor.BusinessID,
		targetType,
		status,
		query,
		limit,
		offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Recipe

	for rows.Next() {
		item, scanErr :=
			scanRecipe(rows)

		if scanErr != nil {
			return nil, scanErr
		}

		out = append(out, item)
	}

	return out, rows.Err()
}

func (s *Service) ArchiveRecipe(
	ctx context.Context,
	actor Actor,
	recipeID string,
	expectedVersion int64,
) (Recipe, error) {
	if expectedVersion <= 0 {
		return Recipe{}, ErrInvalidInput
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Recipe{}, err
	}
	defer tx.Rollback(ctx)

	before, err :=
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
		return Recipe{}, err
	}

	if before.Version != expectedVersion {
		return Recipe{}, ErrVersionConflict
	}

	if before.Status == RecipeStatusArchived {
		return before, nil
	}

	var activeCount int

	if err = tx.QueryRow(
		ctx,
		`
        SELECT count(*)
          FROM vendor_recipe_versions
         WHERE business_id=$1
           AND recipe_id=$2
           AND status='ACTIVE'
        `,
		actor.BusinessID,
		recipeID,
	).Scan(&activeCount); err != nil {
		return Recipe{}, err
	}

	if activeCount > 0 {
		return Recipe{},
			fmt.Errorf(
				"%w: retire the active recipe version before archiving the recipe",
				ErrInvalidState,
			)
	}

	var out Recipe

	err = tx.QueryRow(
		ctx,
		`
        UPDATE vendor_recipes
           SET status='ARCHIVED',
               version=version+1,
               updated_at=now()
         WHERE id=$1
           AND business_id=$2
           AND version=$3
        RETURNING
        `+recipeColumns,
		recipeID,
		actor.BusinessID,
		expectedVersion,
	).Scan(
		&out.ID,
		&out.BusinessID,
		&out.BranchID,
		&out.TargetType,
		&out.TargetRef,
		&out.TargetVariantRef,
		&out.Name,
		&out.InternalDescription,
		&out.Status,
		&out.Version,
		&out.CreatedAt,
		&out.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return Recipe{}, ErrVersionConflict
	}
	if err != nil {
		return Recipe{}, err
	}

	if err = s.auditTx(
		ctx,
		tx,
		actor,
		"vendor.recipe.archived",
		"vendor_recipe",
		out.ID,
		out.BranchID,
		before,
		out,
	); err != nil {
		return Recipe{}, err
	}

	if err = s.publishTx(
		ctx,
		tx,
		actor,
		"vendor.recipe.archived",
		"vendor_recipe",
		out.ID,
		out.Version,
		map[string]any{
			"recipe_id": out.ID,
			"branch_id": out.BranchID,
		},
	); err != nil {
		return Recipe{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return Recipe{}, err
	}

	return out, nil
}
