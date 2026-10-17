package productionmaster

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

func validateNameCode(
	code string,
	name string,
	description string,
) error {
	if len(strings.TrimSpace(code)) == 0 ||
		len(strings.TrimSpace(code)) > 80 ||
		len(strings.TrimSpace(name)) == 0 ||
		len(strings.TrimSpace(name)) > 200 ||
		len(strings.TrimSpace(description)) > 4000 {
		return ErrInvalidInput
	}

	return nil
}

func validateMasterCommon(
	code string,
	name string,
	description string,
	family string,
	unit string,
	control string,
	status string,
) error {
	if err := validateNameCode(
		code,
		name,
		description,
	); err != nil {
		return err
	}

	if !validMeasurementFamily(family) ||
		len(normalizeUnit(unit)) == 0 ||
		len(normalizeUnit(unit)) > 80 ||
		!validInventoryControlMode(control) ||
		!validMasterStatus(status) {
		return ErrInvalidInput
	}

	return nil
}

func validateConversion(
	defaultUnit string,
	input PutUnitConversionInput,
) error {
	input.MeasurementFamily =
		normalizeEnum(input.MeasurementFamily)
	input.FromUnitCode =
		normalizeUnit(input.FromUnitCode)
	input.ToUnitCode =
		normalizeUnit(input.ToUnitCode)
	input.ConversionMode =
		normalizeEnum(input.ConversionMode)
	input.Status =
		normalizeEnum(input.Status)

	if input.Status == "" {
		input.Status = StatusActive
	}

	if !validMeasurementFamily(input.MeasurementFamily) ||
		!validConversionMode(input.ConversionMode) ||
		!validConversionStatus(input.Status) ||
		input.FromUnitCode == "" ||
		input.ToUnitCode == "" ||
		input.FromUnitCode == input.ToUnitCode ||
		input.ToUnitCode != normalizeUnit(defaultUnit) ||
		input.Numerator <= 0 ||
		input.Denominator <= 0 ||
		input.Numerator > 1_000_000_000_000 ||
		input.Denominator > 1_000_000_000_000 ||
		len(strings.TrimSpace(input.SourceContext)) > 1000 {
		return ErrInvalidInput
	}

	if input.ConversionMode ==
		ConversionExactAuthoritative {
		if (!standardExactUnit(input.FromUnitCode) ||
			!standardExactUnit(input.ToUnitCode)) &&
			strings.TrimSpace(input.SourceContext) == "" {
			return fmt.Errorf(
				"%w: non-standard/local authoritative conversions require ingredient/material-specific source context",
				ErrUnsafeConversion,
			)
		}
	}

	return nil
}

func effectiveControlMode(
	base string,
	override string,
) string {
	override = normalizeEnum(override)

	if override != "" {
		return override
	}

	return normalizeEnum(base)
}

func mappingStatus(
	effectiveMode string,
	inventoryItemID string,
) string {
	if effectiveMode == InventoryUntracked {
		return "NOT_REQUIRED"
	}

	if strings.TrimSpace(inventoryItemID) == "" {
		return "NOT_CONFIGURED"
	}

	return "CONFIGURED"
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
	reason string,
) error {
	if tx == nil {
		return ErrInvalidInput
	}

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
            after_data,
            reason
        )
        VALUES(
            $1,$2,'vendor_staff',NULLIF($3,''),
            $4,$5,$6,NULLIF($7,''),
            COALESCE(to_jsonb($8::text),'{}'::jsonb),
            COALESCE(to_jsonb($9::text),'{}'::jsonb),
            NULLIF($10,'')
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
		reason,
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

func scanIngredient(
	row pgx.Row,
) (Ingredient, error) {
	var out Ingredient

	err := row.Scan(
		&out.ID,
		&out.BusinessID,
		&out.Code,
		&out.Name,
		&out.Description,
		&out.MeasurementFamily,
		&out.DefaultUnitCode,
		&out.InventoryControlMode,
		&out.Perishable,
		&out.RequiresExpiry,
		&out.Status,
		&out.Version,
		&out.CreatedAt,
		&out.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return Ingredient{}, ErrNotFound
	}

	return out, err
}

const ingredientSelect = `
SELECT
    id,
    business_id,
    code,
    name,
    description,
    measurement_family,
    default_unit_code,
    inventory_control_mode,
    perishable,
    requires_expiry,
    status,
    version,
    created_at,
    updated_at
FROM vendor_ingredients
`

func (s *Service) CreateIngredient(
	ctx context.Context,
	actor Actor,
	input CreateIngredientInput,
) (Ingredient, error) {
	input.Code = normalizeCode(input.Code)
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.MeasurementFamily =
		normalizeEnum(input.MeasurementFamily)
	input.DefaultUnitCode =
		normalizeUnit(input.DefaultUnitCode)
	input.InventoryControlMode =
		normalizeEnum(input.InventoryControlMode)
	input.Status =
		normalizeEnum(input.Status)

	if input.Status == "" {
		input.Status = StatusActive
	}

	if err := validateMasterCommon(
		input.Code,
		input.Name,
		input.Description,
		input.MeasurementFamily,
		input.DefaultUnitCode,
		input.InventoryControlMode,
		input.Status,
	); err != nil {
		return Ingredient{}, err
	}

	if input.RequiresExpiry && !input.Perishable {
		return Ingredient{}, ErrInvalidInput
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Ingredient{}, err
	}
	defer tx.Rollback(ctx)

	out := Ingredient{
		ID: "ing_" + uuid.NewString(),

		BusinessID: actor.BusinessID,

		Code:        input.Code,
		Name:        input.Name,
		Description: input.Description,

		MeasurementFamily: input.MeasurementFamily,
		DefaultUnitCode:   input.DefaultUnitCode,

		InventoryControlMode: input.InventoryControlMode,

		Perishable:     input.Perishable,
		RequiresExpiry: input.RequiresExpiry,

		Status:  input.Status,
		Version: 1,
	}

	err = tx.QueryRow(
		ctx,
		`
        INSERT INTO vendor_ingredients(
            id,
            business_id,
            code,
            name,
            description,
            measurement_family,
            default_unit_code,
            inventory_control_mode,
            perishable,
            requires_expiry,
            status
        )
        VALUES(
            $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11
        )
        RETURNING
            created_at,
            updated_at
        `,
		out.ID,
		out.BusinessID,
		out.Code,
		out.Name,
		out.Description,
		out.MeasurementFamily,
		out.DefaultUnitCode,
		out.InventoryControlMode,
		out.Perishable,
		out.RequiresExpiry,
		out.Status,
	).Scan(
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		return Ingredient{}, err
	}

	if err = s.auditTx(
		ctx,
		tx,
		actor,
		"vendor.ingredient.created",
		"vendor_ingredient",
		out.ID,
		"",
		nil,
		out,
		"",
	); err != nil {
		return Ingredient{}, err
	}

	if err = s.publishTx(
		ctx,
		tx,
		actor,
		"vendor.ingredient.created",
		"vendor_ingredient",
		out.ID,
		out.Version,
		map[string]any{
			"ingredient_id": out.ID,
			"status":        out.Status,
		},
	); err != nil {
		return Ingredient{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return Ingredient{}, err
	}

	return out, nil
}

func (s *Service) GetIngredient(
	ctx context.Context,
	actor Actor,
	id string,
) (Ingredient, error) {
	return scanIngredient(
		s.db.QueryRow(
			ctx,
			ingredientSelect+
				` WHERE id=$1 AND business_id=$2`,
			strings.TrimSpace(id),
			actor.BusinessID,
		),
	)
}

func (s *Service) ListIngredients(
	ctx context.Context,
	actor Actor,
	query string,
	status string,
	limit int,
) ([]Ingredient, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	query = strings.TrimSpace(query)
	status = normalizeEnum(status)

	rows, err := s.db.Query(
		ctx,
		ingredientSelect+
			`
            WHERE business_id=$1
              AND (
                  $2=''
                  OR status=$2
              )
              AND (
                  $3=''
                  OR name ILIKE '%' || $3 || '%'
                  OR code ILIKE '%' || $3 || '%'
              )
            ORDER BY name,id
            LIMIT $4
            `,
		actor.BusinessID,
		status,
		query,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Ingredient

	for rows.Next() {
		item, scanErr := scanIngredient(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, item)
	}

	return out, rows.Err()
}

func (s *Service) UpdateIngredient(
	ctx context.Context,
	actor Actor,
	id string,
	expectedVersion int64,
	input UpdateIngredientInput,
) (Ingredient, error) {
	if expectedVersion <= 0 {
		return Ingredient{}, ErrInvalidInput
	}

	input.Code = normalizeCode(input.Code)
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.MeasurementFamily =
		normalizeEnum(input.MeasurementFamily)
	input.DefaultUnitCode =
		normalizeUnit(input.DefaultUnitCode)
	input.InventoryControlMode =
		normalizeEnum(input.InventoryControlMode)
	input.Status =
		normalizeEnum(input.Status)

	if err := validateMasterCommon(
		input.Code,
		input.Name,
		input.Description,
		input.MeasurementFamily,
		input.DefaultUnitCode,
		input.InventoryControlMode,
		input.Status,
	); err != nil {
		return Ingredient{}, err
	}

	if input.RequiresExpiry && !input.Perishable {
		return Ingredient{}, ErrInvalidInput
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Ingredient{}, err
	}
	defer tx.Rollback(ctx)

	before, err := scanIngredient(
		tx.QueryRow(
			ctx,
			ingredientSelect+
				`
                WHERE id=$1
                  AND business_id=$2
                FOR UPDATE
                `,
			id,
			actor.BusinessID,
		),
	)
	if err != nil {
		return Ingredient{}, err
	}

	if before.Version != expectedVersion {
		return Ingredient{}, ErrVersionConflict
	}

	if before.Status == StatusArchived {
		return Ingredient{}, ErrInvalidState
	}

	var out Ingredient

	err = tx.QueryRow(
		ctx,
		`
        UPDATE vendor_ingredients
           SET code=$3,
               name=$4,
               description=$5,
               measurement_family=$6,
               default_unit_code=$7,
               inventory_control_mode=$8,
               perishable=$9,
               requires_expiry=$10,
               status=$11,
               version=version+1,
               updated_at=now()
         WHERE id=$1
           AND business_id=$2
           AND version=$12
        RETURNING
            id,
            business_id,
            code,
            name,
            description,
            measurement_family,
            default_unit_code,
            inventory_control_mode,
            perishable,
            requires_expiry,
            status,
            version,
            created_at,
            updated_at
        `,
		id,
		actor.BusinessID,
		input.Code,
		input.Name,
		input.Description,
		input.MeasurementFamily,
		input.DefaultUnitCode,
		input.InventoryControlMode,
		input.Perishable,
		input.RequiresExpiry,
		input.Status,
		expectedVersion,
	).Scan(
		&out.ID,
		&out.BusinessID,
		&out.Code,
		&out.Name,
		&out.Description,
		&out.MeasurementFamily,
		&out.DefaultUnitCode,
		&out.InventoryControlMode,
		&out.Perishable,
		&out.RequiresExpiry,
		&out.Status,
		&out.Version,
		&out.CreatedAt,
		&out.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return Ingredient{}, ErrVersionConflict
	}
	if err != nil {
		return Ingredient{}, err
	}

	if err = s.auditTx(
		ctx,
		tx,
		actor,
		"vendor.ingredient.updated",
		"vendor_ingredient",
		out.ID,
		"",
		before,
		out,
		"",
	); err != nil {
		return Ingredient{}, err
	}

	if err = s.publishTx(
		ctx,
		tx,
		actor,
		"vendor.ingredient.updated",
		"vendor_ingredient",
		out.ID,
		out.Version,
		map[string]any{
			"ingredient_id": out.ID,
			"status":        out.Status,
		},
	); err != nil {
		return Ingredient{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return Ingredient{}, err
	}

	return out, nil
}

func (s *Service) ArchiveIngredient(
	ctx context.Context,
	actor Actor,
	id string,
	expectedVersion int64,
) (Ingredient, error) {
	if expectedVersion <= 0 {
		return Ingredient{}, ErrInvalidInput
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Ingredient{}, err
	}
	defer tx.Rollback(ctx)

	before, err := scanIngredient(
		tx.QueryRow(
			ctx,
			ingredientSelect+
				`
                WHERE id=$1
                  AND business_id=$2
                FOR UPDATE
                `,
			id,
			actor.BusinessID,
		),
	)
	if err != nil {
		return Ingredient{}, err
	}

	if before.Version != expectedVersion {
		return Ingredient{}, ErrVersionConflict
	}

	if before.Status == StatusArchived {
		return before, nil
	}

	var out Ingredient

	err = tx.QueryRow(
		ctx,
		`
        UPDATE vendor_ingredients
           SET status='ARCHIVED',
               version=version+1,
               updated_at=now()
         WHERE id=$1
           AND business_id=$2
           AND version=$3
        RETURNING
            id,
            business_id,
            code,
            name,
            description,
            measurement_family,
            default_unit_code,
            inventory_control_mode,
            perishable,
            requires_expiry,
            status,
            version,
            created_at,
            updated_at
        `,
		id,
		actor.BusinessID,
		expectedVersion,
	).Scan(
		&out.ID,
		&out.BusinessID,
		&out.Code,
		&out.Name,
		&out.Description,
		&out.MeasurementFamily,
		&out.DefaultUnitCode,
		&out.InventoryControlMode,
		&out.Perishable,
		&out.RequiresExpiry,
		&out.Status,
		&out.Version,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Ingredient{}, ErrVersionConflict
	}
	if err != nil {
		return Ingredient{}, err
	}

	if err = s.auditTx(
		ctx,
		tx,
		actor,
		"vendor.ingredient.archived",
		"vendor_ingredient",
		out.ID,
		"",
		before,
		out,
		"",
	); err != nil {
		return Ingredient{}, err
	}

	if err = s.publishTx(
		ctx,
		tx,
		actor,
		"vendor.ingredient.archived",
		"vendor_ingredient",
		out.ID,
		out.Version,
		map[string]any{
			"ingredient_id": out.ID,
		},
	); err != nil {
		return Ingredient{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return Ingredient{}, err
	}

	return out, nil
}

func scanPackaging(
	row pgx.Row,
) (PackagingMaterial, error) {
	var out PackagingMaterial

	err := row.Scan(
		&out.ID,
		&out.BusinessID,
		&out.Code,
		&out.Name,
		&out.Description,
		&out.MeasurementFamily,
		&out.DefaultUnitCode,
		&out.InventoryControlMode,
		&out.Status,
		&out.Version,
		&out.CreatedAt,
		&out.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return PackagingMaterial{}, ErrNotFound
	}

	return out, err
}

const packagingSelect = `
SELECT
    id,
    business_id,
    code,
    name,
    description,
    measurement_family,
    default_unit_code,
    inventory_control_mode,
    status,
    version,
    created_at,
    updated_at
FROM vendor_packaging_materials
`

func (s *Service) CreatePackagingMaterial(
	ctx context.Context,
	actor Actor,
	input CreatePackagingMaterialInput,
) (PackagingMaterial, error) {
	input.Code = normalizeCode(input.Code)
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.MeasurementFamily =
		normalizeEnum(input.MeasurementFamily)
	input.DefaultUnitCode =
		normalizeUnit(input.DefaultUnitCode)
	input.InventoryControlMode =
		normalizeEnum(input.InventoryControlMode)
	input.Status =
		normalizeEnum(input.Status)

	if input.Status == "" {
		input.Status = StatusActive
	}

	if err := validateMasterCommon(
		input.Code,
		input.Name,
		input.Description,
		input.MeasurementFamily,
		input.DefaultUnitCode,
		input.InventoryControlMode,
		input.Status,
	); err != nil {
		return PackagingMaterial{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return PackagingMaterial{}, err
	}
	defer tx.Rollback(ctx)

	out := PackagingMaterial{
		ID: "pkg_" + uuid.NewString(),

		BusinessID: actor.BusinessID,

		Code:        input.Code,
		Name:        input.Name,
		Description: input.Description,

		MeasurementFamily: input.MeasurementFamily,
		DefaultUnitCode:   input.DefaultUnitCode,

		InventoryControlMode: input.InventoryControlMode,

		Status:  input.Status,
		Version: 1,
	}

	err = tx.QueryRow(
		ctx,
		`
        INSERT INTO vendor_packaging_materials(
            id,
            business_id,
            code,
            name,
            description,
            measurement_family,
            default_unit_code,
            inventory_control_mode,
            status
        )
        VALUES(
            $1,$2,$3,$4,$5,$6,$7,$8,$9
        )
        RETURNING
            created_at,
            updated_at
        `,
		out.ID,
		out.BusinessID,
		out.Code,
		out.Name,
		out.Description,
		out.MeasurementFamily,
		out.DefaultUnitCode,
		out.InventoryControlMode,
		out.Status,
	).Scan(
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		return PackagingMaterial{}, err
	}

	if err = s.auditTx(
		ctx,
		tx,
		actor,
		"vendor.packaging.created",
		"vendor_packaging_material",
		out.ID,
		"",
		nil,
		out,
		"",
	); err != nil {
		return PackagingMaterial{}, err
	}

	if err = s.publishTx(
		ctx,
		tx,
		actor,
		"vendor.packaging.created",
		"vendor_packaging_material",
		out.ID,
		out.Version,
		map[string]any{
			"packaging_material_id": out.ID,
			"status":                out.Status,
		},
	); err != nil {
		return PackagingMaterial{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return PackagingMaterial{}, err
	}

	return out, nil
}

func (s *Service) GetPackagingMaterial(
	ctx context.Context,
	actor Actor,
	id string,
) (PackagingMaterial, error) {
	return scanPackaging(
		s.db.QueryRow(
			ctx,
			packagingSelect+
				` WHERE id=$1 AND business_id=$2`,
			strings.TrimSpace(id),
			actor.BusinessID,
		),
	)
}

func (s *Service) ListPackagingMaterials(
	ctx context.Context,
	actor Actor,
	query string,
	status string,
	limit int,
) ([]PackagingMaterial, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	query = strings.TrimSpace(query)
	status = normalizeEnum(status)

	rows, err := s.db.Query(
		ctx,
		packagingSelect+
			`
            WHERE business_id=$1
              AND (
                  $2=''
                  OR status=$2
              )
              AND (
                  $3=''
                  OR name ILIKE '%' || $3 || '%'
                  OR code ILIKE '%' || $3 || '%'
              )
            ORDER BY name,id
            LIMIT $4
            `,
		actor.BusinessID,
		status,
		query,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PackagingMaterial

	for rows.Next() {
		item, scanErr := scanPackaging(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, item)
	}

	return out, rows.Err()
}

func (s *Service) UpdatePackagingMaterial(
	ctx context.Context,
	actor Actor,
	id string,
	expectedVersion int64,
	input UpdatePackagingMaterialInput,
) (PackagingMaterial, error) {
	if expectedVersion <= 0 {
		return PackagingMaterial{}, ErrInvalidInput
	}

	input.Code = normalizeCode(input.Code)
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.MeasurementFamily =
		normalizeEnum(input.MeasurementFamily)
	input.DefaultUnitCode =
		normalizeUnit(input.DefaultUnitCode)
	input.InventoryControlMode =
		normalizeEnum(input.InventoryControlMode)
	input.Status =
		normalizeEnum(input.Status)

	if err := validateMasterCommon(
		input.Code,
		input.Name,
		input.Description,
		input.MeasurementFamily,
		input.DefaultUnitCode,
		input.InventoryControlMode,
		input.Status,
	); err != nil {
		return PackagingMaterial{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return PackagingMaterial{}, err
	}
	defer tx.Rollback(ctx)

	before, err := scanPackaging(
		tx.QueryRow(
			ctx,
			packagingSelect+
				`
                WHERE id=$1
                  AND business_id=$2
                FOR UPDATE
                `,
			id,
			actor.BusinessID,
		),
	)
	if err != nil {
		return PackagingMaterial{}, err
	}

	if before.Version != expectedVersion {
		return PackagingMaterial{}, ErrVersionConflict
	}

	if before.Status == StatusArchived {
		return PackagingMaterial{}, ErrInvalidState
	}

	var out PackagingMaterial

	err = tx.QueryRow(
		ctx,
		`
        UPDATE vendor_packaging_materials
           SET code=$3,
               name=$4,
               description=$5,
               measurement_family=$6,
               default_unit_code=$7,
               inventory_control_mode=$8,
               status=$9,
               version=version+1,
               updated_at=now()
         WHERE id=$1
           AND business_id=$2
           AND version=$10
        RETURNING
            id,
            business_id,
            code,
            name,
            description,
            measurement_family,
            default_unit_code,
            inventory_control_mode,
            status,
            version,
            created_at,
            updated_at
        `,
		id,
		actor.BusinessID,
		input.Code,
		input.Name,
		input.Description,
		input.MeasurementFamily,
		input.DefaultUnitCode,
		input.InventoryControlMode,
		input.Status,
		expectedVersion,
	).Scan(
		&out.ID,
		&out.BusinessID,
		&out.Code,
		&out.Name,
		&out.Description,
		&out.MeasurementFamily,
		&out.DefaultUnitCode,
		&out.InventoryControlMode,
		&out.Status,
		&out.Version,
		&out.CreatedAt,
		&out.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return PackagingMaterial{}, ErrVersionConflict
	}
	if err != nil {
		return PackagingMaterial{}, err
	}

	if err = s.auditTx(
		ctx,
		tx,
		actor,
		"vendor.packaging.updated",
		"vendor_packaging_material",
		out.ID,
		"",
		before,
		out,
		"",
	); err != nil {
		return PackagingMaterial{}, err
	}

	if err = s.publishTx(
		ctx,
		tx,
		actor,
		"vendor.packaging.updated",
		"vendor_packaging_material",
		out.ID,
		out.Version,
		map[string]any{
			"packaging_material_id": out.ID,
			"status":                out.Status,
		},
	); err != nil {
		return PackagingMaterial{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return PackagingMaterial{}, err
	}

	return out, nil
}

func (s *Service) ArchivePackagingMaterial(
	ctx context.Context,
	actor Actor,
	id string,
	expectedVersion int64,
) (PackagingMaterial, error) {
	if expectedVersion <= 0 {
		return PackagingMaterial{}, ErrInvalidInput
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return PackagingMaterial{}, err
	}
	defer tx.Rollback(ctx)

	before, err := scanPackaging(
		tx.QueryRow(
			ctx,
			packagingSelect+
				`
                WHERE id=$1
                  AND business_id=$2
                FOR UPDATE
                `,
			id,
			actor.BusinessID,
		),
	)
	if err != nil {
		return PackagingMaterial{}, err
	}

	if before.Version != expectedVersion {
		return PackagingMaterial{}, ErrVersionConflict
	}

	if before.Status == StatusArchived {
		return before, nil
	}

	var out PackagingMaterial

	err = tx.QueryRow(
		ctx,
		`
        UPDATE vendor_packaging_materials
           SET status='ARCHIVED',
               version=version+1,
               updated_at=now()
         WHERE id=$1
           AND business_id=$2
           AND version=$3
        RETURNING
            id,
            business_id,
            code,
            name,
            description,
            measurement_family,
            default_unit_code,
            inventory_control_mode,
            status,
            version,
            created_at,
            updated_at
        `,
		id,
		actor.BusinessID,
		expectedVersion,
	).Scan(
		&out.ID,
		&out.BusinessID,
		&out.Code,
		&out.Name,
		&out.Description,
		&out.MeasurementFamily,
		&out.DefaultUnitCode,
		&out.InventoryControlMode,
		&out.Status,
		&out.Version,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return PackagingMaterial{}, ErrVersionConflict
	}
	if err != nil {
		return PackagingMaterial{}, err
	}

	if err = s.auditTx(
		ctx,
		tx,
		actor,
		"vendor.packaging.archived",
		"vendor_packaging_material",
		out.ID,
		"",
		before,
		out,
		"",
	); err != nil {
		return PackagingMaterial{}, err
	}

	if err = s.publishTx(
		ctx,
		tx,
		actor,
		"vendor.packaging.archived",
		"vendor_packaging_material",
		out.ID,
		out.Version,
		map[string]any{
			"packaging_material_id": out.ID,
		},
	); err != nil {
		return PackagingMaterial{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return PackagingMaterial{}, err
	}

	return out, nil
}

func scanConversion(
	row pgx.Row,
) (UnitConversion, error) {
	var out UnitConversion

	err := row.Scan(
		&out.ID,
		&out.BusinessID,
		&out.OwnerID,
		&out.MeasurementFamily,
		&out.FromUnitCode,
		&out.ToUnitCode,
		&out.ConversionMode,
		&out.Numerator,
		&out.Denominator,
		&out.SourceContext,
		&out.Status,
		&out.Version,
		&out.CreatedAt,
		&out.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return UnitConversion{}, ErrNotFound
	}

	return out, err
}

func (s *Service) CreateIngredientConversion(
	ctx context.Context,
	actor Actor,
	ingredientID string,
	input PutUnitConversionInput,
) (UnitConversion, error) {
	ingredient, err :=
		s.GetIngredient(
			ctx,
			actor,
			ingredientID,
		)
	if err != nil {
		return UnitConversion{}, err
	}

	if ingredient.Status == StatusArchived {
		return UnitConversion{}, ErrInvalidState
	}

	input.MeasurementFamily =
		normalizeEnum(input.MeasurementFamily)
	input.FromUnitCode =
		normalizeUnit(input.FromUnitCode)
	input.ToUnitCode =
		normalizeUnit(input.ToUnitCode)
	input.ConversionMode =
		normalizeEnum(input.ConversionMode)
	input.Status =
		normalizeEnum(input.Status)
	input.SourceContext =
		strings.TrimSpace(input.SourceContext)

	if input.Status == "" {
		input.Status = StatusActive
	}

	if input.MeasurementFamily !=
		ingredient.MeasurementFamily {
		return UnitConversion{}, ErrInvalidInput
	}

	if err = validateConversion(
		ingredient.DefaultUnitCode,
		input,
	); err != nil {
		return UnitConversion{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return UnitConversion{}, err
	}
	defer tx.Rollback(ctx)

	out := UnitConversion{
		ID: "iuc_" + uuid.NewString(),

		BusinessID: actor.BusinessID,
		OwnerID:    ingredient.ID,

		MeasurementFamily: input.MeasurementFamily,

		FromUnitCode: input.FromUnitCode,
		ToUnitCode:   input.ToUnitCode,

		ConversionMode: input.ConversionMode,

		Numerator:   input.Numerator,
		Denominator: input.Denominator,

		SourceContext: input.SourceContext,

		Status:  input.Status,
		Version: 1,
	}

	err = tx.QueryRow(
		ctx,
		`
        INSERT INTO vendor_ingredient_unit_conversions(
            id,
            business_id,
            ingredient_id,
            measurement_family,
            from_unit_code,
            to_unit_code,
            conversion_mode,
            numerator,
            denominator,
            source_context,
            status
        )
        VALUES(
            $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11
        )
        RETURNING
            created_at,
            updated_at
        `,
		out.ID,
		out.BusinessID,
		out.OwnerID,
		out.MeasurementFamily,
		out.FromUnitCode,
		out.ToUnitCode,
		out.ConversionMode,
		out.Numerator,
		out.Denominator,
		out.SourceContext,
		out.Status,
	).Scan(
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		return UnitConversion{}, err
	}

	if err = s.auditTx(
		ctx,
		tx,
		actor,
		"vendor.ingredient.unit_conversion.created",
		"vendor_ingredient_unit_conversion",
		out.ID,
		"",
		nil,
		out,
		"",
	); err != nil {
		return UnitConversion{}, err
	}

	if err = s.publishTx(
		ctx,
		tx,
		actor,
		"vendor.ingredient.unit_conversion.created",
		"vendor_ingredient_unit_conversion",
		out.ID,
		out.Version,
		map[string]any{
			"ingredient_id":   ingredient.ID,
			"conversion_mode": out.ConversionMode,
		},
	); err != nil {
		return UnitConversion{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return UnitConversion{}, err
	}

	return out, nil
}

func (s *Service) ListIngredientConversions(
	ctx context.Context,
	actor Actor,
	ingredientID string,
) ([]UnitConversion, error) {
	if _, err := s.GetIngredient(
		ctx,
		actor,
		ingredientID,
	); err != nil {
		return nil, err
	}

	rows, err := s.db.Query(
		ctx,
		`
        SELECT
            id,
            business_id,
            ingredient_id,
            measurement_family,
            from_unit_code,
            to_unit_code,
            conversion_mode,
            numerator,
            denominator,
            source_context,
            status,
            version,
            created_at,
            updated_at
          FROM vendor_ingredient_unit_conversions
         WHERE business_id=$1
           AND ingredient_id=$2
         ORDER BY from_unit_code,to_unit_code,id
        `,
		actor.BusinessID,
		ingredientID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []UnitConversion

	for rows.Next() {
		item, scanErr := scanConversion(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, item)
	}

	return out, rows.Err()
}

func (s *Service) UpdateIngredientConversion(
	ctx context.Context,
	actor Actor,
	ingredientID string,
	conversionID string,
	expectedVersion int64,
	input PutUnitConversionInput,
) (UnitConversion, error) {
	if expectedVersion <= 0 {
		return UnitConversion{}, ErrInvalidInput
	}

	ingredient, err :=
		s.GetIngredient(
			ctx,
			actor,
			ingredientID,
		)
	if err != nil {
		return UnitConversion{}, err
	}

	if ingredient.Status == StatusArchived {
		return UnitConversion{}, ErrInvalidState
	}

	input.MeasurementFamily =
		normalizeEnum(input.MeasurementFamily)
	input.FromUnitCode =
		normalizeUnit(input.FromUnitCode)
	input.ToUnitCode =
		normalizeUnit(input.ToUnitCode)
	input.ConversionMode =
		normalizeEnum(input.ConversionMode)
	input.Status =
		normalizeEnum(input.Status)
	input.SourceContext =
		strings.TrimSpace(input.SourceContext)

	if err = validateConversion(
		ingredient.DefaultUnitCode,
		input,
	); err != nil {
		return UnitConversion{}, err
	}

	if input.MeasurementFamily !=
		ingredient.MeasurementFamily {
		return UnitConversion{}, ErrInvalidInput
	}

	var out UnitConversion

	err = s.db.QueryRow(
		ctx,
		`
        UPDATE vendor_ingredient_unit_conversions
           SET measurement_family=$4,
               from_unit_code=$5,
               to_unit_code=$6,
               conversion_mode=$7,
               numerator=$8,
               denominator=$9,
               source_context=$10,
               status=$11,
               version=version+1,
               updated_at=now()
         WHERE id=$1
           AND business_id=$2
           AND ingredient_id=$3
           AND version=$12
        RETURNING
            id,
            business_id,
            ingredient_id,
            measurement_family,
            from_unit_code,
            to_unit_code,
            conversion_mode,
            numerator,
            denominator,
            source_context,
            status,
            version,
            created_at,
            updated_at
        `,
		conversionID,
		actor.BusinessID,
		ingredientID,
		input.MeasurementFamily,
		input.FromUnitCode,
		input.ToUnitCode,
		input.ConversionMode,
		input.Numerator,
		input.Denominator,
		input.SourceContext,
		input.Status,
		expectedVersion,
	).Scan(
		&out.ID,
		&out.BusinessID,
		&out.OwnerID,
		&out.MeasurementFamily,
		&out.FromUnitCode,
		&out.ToUnitCode,
		&out.ConversionMode,
		&out.Numerator,
		&out.Denominator,
		&out.SourceContext,
		&out.Status,
		&out.Version,
		&out.CreatedAt,
		&out.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return UnitConversion{}, ErrVersionConflict
	}

	return out, err
}

func (s *Service) CreatePackagingConversion(
	ctx context.Context,
	actor Actor,
	materialID string,
	input PutUnitConversionInput,
) (UnitConversion, error) {
	material, err :=
		s.GetPackagingMaterial(
			ctx,
			actor,
			materialID,
		)
	if err != nil {
		return UnitConversion{}, err
	}

	if material.Status == StatusArchived {
		return UnitConversion{}, ErrInvalidState
	}

	input.MeasurementFamily =
		normalizeEnum(input.MeasurementFamily)
	input.FromUnitCode =
		normalizeUnit(input.FromUnitCode)
	input.ToUnitCode =
		normalizeUnit(input.ToUnitCode)
	input.ConversionMode =
		normalizeEnum(input.ConversionMode)
	input.Status =
		normalizeEnum(input.Status)
	input.SourceContext =
		strings.TrimSpace(input.SourceContext)

	if input.Status == "" {
		input.Status = StatusActive
	}

	if input.MeasurementFamily !=
		material.MeasurementFamily {
		return UnitConversion{}, ErrInvalidInput
	}

	if err = validateConversion(
		material.DefaultUnitCode,
		input,
	); err != nil {
		return UnitConversion{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return UnitConversion{}, err
	}
	defer tx.Rollback(ctx)

	out := UnitConversion{
		ID: "puc_" + uuid.NewString(),

		BusinessID: actor.BusinessID,
		OwnerID:    material.ID,

		MeasurementFamily: input.MeasurementFamily,

		FromUnitCode: input.FromUnitCode,
		ToUnitCode:   input.ToUnitCode,

		ConversionMode: input.ConversionMode,

		Numerator:   input.Numerator,
		Denominator: input.Denominator,

		SourceContext: input.SourceContext,

		Status:  input.Status,
		Version: 1,
	}

	err = tx.QueryRow(
		ctx,
		`
        INSERT INTO vendor_packaging_unit_conversions(
            id,
            business_id,
            packaging_material_id,
            measurement_family,
            from_unit_code,
            to_unit_code,
            conversion_mode,
            numerator,
            denominator,
            source_context,
            status
        )
        VALUES(
            $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11
        )
        RETURNING
            created_at,
            updated_at
        `,
		out.ID,
		out.BusinessID,
		out.OwnerID,
		out.MeasurementFamily,
		out.FromUnitCode,
		out.ToUnitCode,
		out.ConversionMode,
		out.Numerator,
		out.Denominator,
		out.SourceContext,
		out.Status,
	).Scan(
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		return UnitConversion{}, err
	}

	if err = s.auditTx(
		ctx,
		tx,
		actor,
		"vendor.packaging.unit_conversion.created",
		"vendor_packaging_unit_conversion",
		out.ID,
		"",
		nil,
		out,
		"",
	); err != nil {
		return UnitConversion{}, err
	}

	if err = s.publishTx(
		ctx,
		tx,
		actor,
		"vendor.packaging.unit_conversion.created",
		"vendor_packaging_unit_conversion",
		out.ID,
		out.Version,
		map[string]any{
			"packaging_material_id": material.ID,
			"conversion_mode":       out.ConversionMode,
		},
	); err != nil {
		return UnitConversion{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return UnitConversion{}, err
	}

	return out, nil
}

func (s *Service) ListPackagingConversions(
	ctx context.Context,
	actor Actor,
	materialID string,
) ([]UnitConversion, error) {
	if _, err := s.GetPackagingMaterial(
		ctx,
		actor,
		materialID,
	); err != nil {
		return nil, err
	}

	rows, err := s.db.Query(
		ctx,
		`
        SELECT
            id,
            business_id,
            packaging_material_id,
            measurement_family,
            from_unit_code,
            to_unit_code,
            conversion_mode,
            numerator,
            denominator,
            source_context,
            status,
            version,
            created_at,
            updated_at
          FROM vendor_packaging_unit_conversions
         WHERE business_id=$1
           AND packaging_material_id=$2
         ORDER BY from_unit_code,to_unit_code,id
        `,
		actor.BusinessID,
		materialID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []UnitConversion

	for rows.Next() {
		item, scanErr := scanConversion(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, item)
	}

	return out, rows.Err()
}

func (s *Service) UpdatePackagingConversion(
	ctx context.Context,
	actor Actor,
	materialID string,
	conversionID string,
	expectedVersion int64,
	input PutUnitConversionInput,
) (UnitConversion, error) {
	if expectedVersion <= 0 {
		return UnitConversion{}, ErrInvalidInput
	}

	material, err :=
		s.GetPackagingMaterial(
			ctx,
			actor,
			materialID,
		)
	if err != nil {
		return UnitConversion{}, err
	}

	if material.Status == StatusArchived {
		return UnitConversion{}, ErrInvalidState
	}

	input.MeasurementFamily =
		normalizeEnum(input.MeasurementFamily)
	input.FromUnitCode =
		normalizeUnit(input.FromUnitCode)
	input.ToUnitCode =
		normalizeUnit(input.ToUnitCode)
	input.ConversionMode =
		normalizeEnum(input.ConversionMode)
	input.Status =
		normalizeEnum(input.Status)
	input.SourceContext =
		strings.TrimSpace(input.SourceContext)

	if err = validateConversion(
		material.DefaultUnitCode,
		input,
	); err != nil {
		return UnitConversion{}, err
	}

	if input.MeasurementFamily !=
		material.MeasurementFamily {
		return UnitConversion{}, ErrInvalidInput
	}

	var out UnitConversion

	err = s.db.QueryRow(
		ctx,
		`
        UPDATE vendor_packaging_unit_conversions
           SET measurement_family=$4,
               from_unit_code=$5,
               to_unit_code=$6,
               conversion_mode=$7,
               numerator=$8,
               denominator=$9,
               source_context=$10,
               status=$11,
               version=version+1,
               updated_at=now()
         WHERE id=$1
           AND business_id=$2
           AND packaging_material_id=$3
           AND version=$12
        RETURNING
            id,
            business_id,
            packaging_material_id,
            measurement_family,
            from_unit_code,
            to_unit_code,
            conversion_mode,
            numerator,
            denominator,
            source_context,
            status,
            version,
            created_at,
            updated_at
        `,
		conversionID,
		actor.BusinessID,
		materialID,
		input.MeasurementFamily,
		input.FromUnitCode,
		input.ToUnitCode,
		input.ConversionMode,
		input.Numerator,
		input.Denominator,
		input.SourceContext,
		input.Status,
		expectedVersion,
	).Scan(
		&out.ID,
		&out.BusinessID,
		&out.OwnerID,
		&out.MeasurementFamily,
		&out.FromUnitCode,
		&out.ToUnitCode,
		&out.ConversionMode,
		&out.Numerator,
		&out.Denominator,
		&out.SourceContext,
		&out.Status,
		&out.Version,
		&out.CreatedAt,
		&out.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return UnitConversion{}, ErrVersionConflict
	}

	return out, err
}

func (s *Service) validateInventoryMappingTx(
	ctx context.Context,
	tx pgx.Tx,
	businessID string,
	branchID string,
	inventoryItemID string,
	expectedItemType string,
) error {
	if strings.TrimSpace(inventoryItemID) == "" {
		return nil
	}

	var itemType string
	var status string

	err := tx.QueryRow(
		ctx,
		`
        SELECT
            item_type,
            status
          FROM inventory_items
         WHERE id=$1
           AND business_id=$2
           AND branch_id=$3
        `,
		inventoryItemID,
		businessID,
		branchID,
	).Scan(
		&itemType,
		&status,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInventoryMapping
	}
	if err != nil {
		return err
	}

	if itemType != expectedItemType ||
		status != StatusActive {
		return ErrInventoryMapping
	}

	return nil
}

func scanBranchConfig(
	row pgx.Row,
	baseMode string,
) (BranchConfig, error) {
	var out BranchConfig

	err := row.Scan(
		&out.ID,
		&out.BusinessID,
		&out.BranchID,
		&out.OwnerID,
		&out.Enabled,
		&out.ProductionEligible,
		&out.InventoryControlModeOverride,
		&out.InventoryItemID,
		&out.Version,
		&out.CreatedAt,
		&out.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return BranchConfig{}, ErrNotFound
	}
	if err != nil {
		return BranchConfig{}, err
	}

	out.EffectiveInventoryControlMode =
		effectiveControlMode(
			baseMode,
			out.InventoryControlModeOverride,
		)

	out.InventoryMappingStatus =
		mappingStatus(
			out.EffectiveInventoryControlMode,
			out.InventoryItemID,
		)

	return out, nil
}

func validateBranchConfigInput(
	baseMode string,
	input PutBranchConfigInput,
) (string, error) {
	override :=
		normalizeEnum(
			input.InventoryControlModeOverride,
		)

	if override != "" &&
		!validInventoryControlMode(override) {
		return "", ErrInvalidInput
	}

	effective :=
		effectiveControlMode(
			baseMode,
			override,
		)

	if effective == InventoryUntracked &&
		strings.TrimSpace(input.InventoryItemID) != "" {
		return "", ErrInventoryMapping
	}

	// An inventory-controlled config with no mapping gets a branch stock
	// item provisioned automatically when it is saved.

	return effective, nil
}

func (s *Service) PutIngredientBranchConfig(
	ctx context.Context,
	actor Actor,
	ingredientID string,
	branchID string,
	input PutBranchConfigInput,
) (BranchConfig, error) {
	ingredient, err :=
		s.GetIngredient(
			ctx,
			actor,
			ingredientID,
		)
	if err != nil {
		return BranchConfig{}, err
	}

	if ingredient.Status == StatusArchived {
		return BranchConfig{}, ErrInvalidState
	}

	effectiveMode, err :=
		validateBranchConfigInput(
			ingredient.InventoryControlMode,
			input,
		)
	if err != nil {
		return BranchConfig{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return BranchConfig{}, err
	}
	defer tx.Rollback(ctx)

	var exists bool

	if err = tx.QueryRow(
		ctx,
		`
        SELECT EXISTS(
            SELECT 1
              FROM vendor_branches
             WHERE id=$1
               AND business_id=$2
        )
        `,
		branchID,
		actor.BusinessID,
	).Scan(&exists); err != nil {
		return BranchConfig{}, err
	}

	if !exists {
		return BranchConfig{}, ErrNotFound
	}

	if effectiveMode == InventoryControlled &&
		strings.TrimSpace(input.InventoryItemID) == "" {
		input.InventoryItemID, err = ensureMasterInventoryItemTx(
			ctx,
			tx,
			actor.BusinessID,
			branchID,
			"INGREDIENT",
			ingredient.ID,
			ingredient.DefaultUnitCode,
			ingredient.MeasurementFamily,
		)
		if err != nil {
			return BranchConfig{}, err
		}
	}

	if effectiveMode ==
		InventoryControlled {
		if err = s.validateInventoryMappingTx(
			ctx,
			tx,
			actor.BusinessID,
			branchID,
			input.InventoryItemID,
			"INGREDIENT",
		); err != nil {
			return BranchConfig{}, err
		}
	}

	var out BranchConfig

	err = tx.QueryRow(
		ctx,
		`
        INSERT INTO vendor_ingredient_branch_configs(
            id,
            business_id,
            branch_id,
            ingredient_id,
            enabled,
            production_eligible,
            inventory_control_mode_override,
            inventory_item_id
        )
        VALUES(
            $1,$2,$3,$4,$5,$6,
            NULLIF($7,''),
            NULLIF($8,'')
        )
        ON CONFLICT(
            business_id,
            branch_id,
            ingredient_id
        )
        DO UPDATE SET
            enabled=EXCLUDED.enabled,
            production_eligible=
                EXCLUDED.production_eligible,
            inventory_control_mode_override=
                EXCLUDED.inventory_control_mode_override,
            inventory_item_id=
                EXCLUDED.inventory_item_id,
            version=
                vendor_ingredient_branch_configs.version+1,
            updated_at=now()
        RETURNING
            id,
            business_id,
            branch_id,
            ingredient_id,
            enabled,
            production_eligible,
            COALESCE(
                inventory_control_mode_override,
                ''
            ),
            COALESCE(
                inventory_item_id,
                ''
            ),
            version,
            created_at,
            updated_at
        `,
		"ibc_"+uuid.NewString(),
		actor.BusinessID,
		branchID,
		ingredientID,
		input.Enabled,
		input.ProductionEligible,
		normalizeEnum(
			input.InventoryControlModeOverride,
		),
		strings.TrimSpace(
			input.InventoryItemID,
		),
	).Scan(
		&out.ID,
		&out.BusinessID,
		&out.BranchID,
		&out.OwnerID,
		&out.Enabled,
		&out.ProductionEligible,
		&out.InventoryControlModeOverride,
		&out.InventoryItemID,
		&out.Version,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		return BranchConfig{}, err
	}

	out.EffectiveInventoryControlMode =
		effectiveMode

	out.InventoryMappingStatus =
		mappingStatus(
			effectiveMode,
			out.InventoryItemID,
		)

	if err = s.auditTx(
		ctx,
		tx,
		actor,
		"vendor.ingredient.inventory_mapping.changed",
		"vendor_ingredient_branch_config",
		out.ID,
		branchID,
		nil,
		out,
		"",
	); err != nil {
		return BranchConfig{}, err
	}

	if err = s.publishTx(
		ctx,
		tx,
		actor,
		"vendor.ingredient.inventory_mapping.changed",
		"vendor_ingredient_branch_config",
		out.ID,
		out.Version,
		map[string]any{
			"ingredient_id":  ingredientID,
			"branch_id":      branchID,
			"mapping_status": out.InventoryMappingStatus,
		},
	); err != nil {
		return BranchConfig{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return BranchConfig{}, err
	}

	return out, nil
}

func (s *Service) GetIngredientBranchConfig(
	ctx context.Context,
	actor Actor,
	ingredientID string,
	branchID string,
) (BranchConfig, error) {
	ingredient, err :=
		s.GetIngredient(
			ctx,
			actor,
			ingredientID,
		)
	if err != nil {
		return BranchConfig{}, err
	}

	return scanBranchConfig(
		s.db.QueryRow(
			ctx,
			`
            SELECT
                id,
                business_id,
                branch_id,
                ingredient_id,
                enabled,
                production_eligible,
                COALESCE(
                    inventory_control_mode_override,
                    ''
                ),
                COALESCE(
                    inventory_item_id,
                    ''
                ),
                version,
                created_at,
                updated_at
              FROM vendor_ingredient_branch_configs
             WHERE business_id=$1
               AND branch_id=$2
               AND ingredient_id=$3
            `,
			actor.BusinessID,
			branchID,
			ingredientID,
		),
		ingredient.InventoryControlMode,
	)
}

func (s *Service) PutPackagingBranchConfig(
	ctx context.Context,
	actor Actor,
	materialID string,
	branchID string,
	input PutBranchConfigInput,
) (BranchConfig, error) {
	material, err :=
		s.GetPackagingMaterial(
			ctx,
			actor,
			materialID,
		)
	if err != nil {
		return BranchConfig{}, err
	}

	if material.Status == StatusArchived {
		return BranchConfig{}, ErrInvalidState
	}

	effectiveMode, err :=
		validateBranchConfigInput(
			material.InventoryControlMode,
			input,
		)
	if err != nil {
		return BranchConfig{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return BranchConfig{}, err
	}
	defer tx.Rollback(ctx)

	var exists bool

	if err = tx.QueryRow(
		ctx,
		`
        SELECT EXISTS(
            SELECT 1
              FROM vendor_branches
             WHERE id=$1
               AND business_id=$2
        )
        `,
		branchID,
		actor.BusinessID,
	).Scan(&exists); err != nil {
		return BranchConfig{}, err
	}

	if !exists {
		return BranchConfig{}, ErrNotFound
	}

	if effectiveMode == InventoryControlled &&
		strings.TrimSpace(input.InventoryItemID) == "" {
		input.InventoryItemID, err = ensureMasterInventoryItemTx(
			ctx,
			tx,
			actor.BusinessID,
			branchID,
			"PACKAGING",
			material.ID,
			material.DefaultUnitCode,
			material.MeasurementFamily,
		)
		if err != nil {
			return BranchConfig{}, err
		}
	}

	if effectiveMode ==
		InventoryControlled {
		if err = s.validateInventoryMappingTx(
			ctx,
			tx,
			actor.BusinessID,
			branchID,
			input.InventoryItemID,
			"PACKAGING",
		); err != nil {
			return BranchConfig{}, err
		}
	}

	var out BranchConfig

	err = tx.QueryRow(
		ctx,
		`
        INSERT INTO vendor_packaging_branch_configs(
            id,
            business_id,
            branch_id,
            packaging_material_id,
            enabled,
            production_eligible,
            inventory_control_mode_override,
            inventory_item_id
        )
        VALUES(
            $1,$2,$3,$4,$5,$6,
            NULLIF($7,''),
            NULLIF($8,'')
        )
        ON CONFLICT(
            business_id,
            branch_id,
            packaging_material_id
        )
        DO UPDATE SET
            enabled=EXCLUDED.enabled,
            production_eligible=
                EXCLUDED.production_eligible,
            inventory_control_mode_override=
                EXCLUDED.inventory_control_mode_override,
            inventory_item_id=
                EXCLUDED.inventory_item_id,
            version=
                vendor_packaging_branch_configs.version+1,
            updated_at=now()
        RETURNING
            id,
            business_id,
            branch_id,
            packaging_material_id,
            enabled,
            production_eligible,
            COALESCE(
                inventory_control_mode_override,
                ''
            ),
            COALESCE(
                inventory_item_id,
                ''
            ),
            version,
            created_at,
            updated_at
        `,
		"pbc_"+uuid.NewString(),
		actor.BusinessID,
		branchID,
		materialID,
		input.Enabled,
		input.ProductionEligible,
		normalizeEnum(
			input.InventoryControlModeOverride,
		),
		strings.TrimSpace(
			input.InventoryItemID,
		),
	).Scan(
		&out.ID,
		&out.BusinessID,
		&out.BranchID,
		&out.OwnerID,
		&out.Enabled,
		&out.ProductionEligible,
		&out.InventoryControlModeOverride,
		&out.InventoryItemID,
		&out.Version,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		return BranchConfig{}, err
	}

	out.EffectiveInventoryControlMode =
		effectiveMode

	out.InventoryMappingStatus =
		mappingStatus(
			effectiveMode,
			out.InventoryItemID,
		)

	if err = s.auditTx(
		ctx,
		tx,
		actor,
		"vendor.packaging.inventory_mapping.changed",
		"vendor_packaging_branch_config",
		out.ID,
		branchID,
		nil,
		out,
		"",
	); err != nil {
		return BranchConfig{}, err
	}

	if err = s.publishTx(
		ctx,
		tx,
		actor,
		"vendor.packaging.inventory_mapping.changed",
		"vendor_packaging_branch_config",
		out.ID,
		out.Version,
		map[string]any{
			"packaging_material_id": materialID,
			"branch_id":             branchID,
			"mapping_status":        out.InventoryMappingStatus,
		},
	); err != nil {
		return BranchConfig{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return BranchConfig{}, err
	}

	return out, nil
}

func (s *Service) GetPackagingBranchConfig(
	ctx context.Context,
	actor Actor,
	materialID string,
	branchID string,
) (BranchConfig, error) {
	material, err :=
		s.GetPackagingMaterial(
			ctx,
			actor,
			materialID,
		)
	if err != nil {
		return BranchConfig{}, err
	}

	return scanBranchConfig(
		s.db.QueryRow(
			ctx,
			`
            SELECT
                id,
                business_id,
                branch_id,
                packaging_material_id,
                enabled,
                production_eligible,
                COALESCE(
                    inventory_control_mode_override,
                    ''
                ),
                COALESCE(
                    inventory_item_id,
                    ''
                ),
                version,
                created_at,
                updated_at
              FROM vendor_packaging_branch_configs
             WHERE business_id=$1
               AND branch_id=$2
               AND packaging_material_id=$3
            `,
			actor.BusinessID,
			branchID,
			materialID,
		),
		material.InventoryControlMode,
	)
}
