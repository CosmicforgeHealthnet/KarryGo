package productionbatch

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/features/inventory"
	"cosmicforge/logistics/services/vendor-service/internal/features/production"
)

type Service struct {
	db         *pgxpool.Pool
	production *production.Service
	inventory  *inventory.StockService
}

func NewService(
	db *pgxpool.Pool,
	productionService *production.Service,
	inventoryService *inventory.StockService,
) *Service {
	return &Service{
		db:         db,
		production: productionService,
		inventory:  inventoryService,
	}
}

func (s *Service) Authorize(
	ctx context.Context,
	actor production.Actor,
	permission string,
) (production.Scope, error) {
	if s == nil || s.production == nil {
		return production.Scope{}, ErrStateConflict
	}
	return s.production.Authorize(ctx, actor, permission)
}

func inventoryActor(actor production.Actor) inventory.Actor {
	return inventory.Actor{
		ActorRef:      actor.StaffID,
		StaffID:       actor.StaffID,
		BusinessID:    actor.VendorID,
		CorrelationID: actor.CorrelationID,
		CausationID:   actor.CausationID,
		RequestID:     actor.CorrelationID,
	}
}

func inventoryScope(
	businessID string,
	branchID string,
) (inventory.BranchScope, error) {
	return inventory.ExactBranchScope(
		strings.TrimSpace(businessID),
		strings.TrimSpace(branchID),
	)
}

const batchColumns = `
    id,
    business_id,
    branch_id,
    recipe_id,
    recipe_version_id,
    recipe_version_number,
    target_type,
    target_ref,
    target_variant_ref,
    COALESCE(production_job_id,''),
    planned_output_numerator,
    planned_output_denominator,
    output_unit_code,
    expected_output_numerator,
    expected_output_denominator,
    recipe_expected_waste_bps,
    multiplier_numerator,
    multiplier_denominator,
    operating_model,
    COALESCE(prepared_inventory_item_id,''),
    output_inventory_numerator,
    output_inventory_denominator,
    COALESCE(inventory_reservation_id,''),
    status,
    actual_consumption_recorded,
    actual_output_numerator,
    actual_output_denominator,
    COALESCE(created_by_staff_id,''),
    COALESCE(planned_by_staff_id,''),
    COALESCE(started_by_staff_id,''),
    COALESCE(completed_by_staff_id,''),
    COALESCE(cancelled_by_staff_id,''),
    planned_at,
    started_at,
    completed_at,
    cancelled_at,
    cancel_reason,
    version,
    created_at,
    updated_at
`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanBatch(row rowScanner) (Batch, error) {
	var out Batch

	err := row.Scan(
		&out.ID,
		&out.BusinessID,
		&out.BranchID,
		&out.RecipeID,
		&out.RecipeVersionID,
		&out.RecipeVersionNumber,
		&out.TargetType,
		&out.TargetRef,
		&out.TargetVariantRef,
		&out.ProductionJobID,
		&out.PlannedOutputNumerator,
		&out.PlannedOutputDenominator,
		&out.OutputUnitCode,
		&out.ExpectedOutputNumerator,
		&out.ExpectedOutputDenominator,
		&out.RecipeExpectedWasteBPS,
		&out.MultiplierNumerator,
		&out.MultiplierDenominator,
		&out.OperatingModel,
		&out.PreparedInventoryItemID,
		&out.OutputInventoryNumerator,
		&out.OutputInventoryDenominator,
		&out.InventoryReservationID,
		&out.Status,
		&out.ActualConsumptionRecorded,
		&out.ActualOutputNumerator,
		&out.ActualOutputDenominator,
		&out.CreatedByStaffID,
		&out.PlannedByStaffID,
		&out.StartedByStaffID,
		&out.CompletedByStaffID,
		&out.CancelledByStaffID,
		&out.PlannedAt,
		&out.StartedAt,
		&out.CompletedAt,
		&out.CancelledAt,
		&out.CancelReason,
		&out.Version,
		&out.CreatedAt,
		&out.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return Batch{}, ErrNotFound
	}

	return out, err
}

func (s *Service) loadBatchQ(
	ctx context.Context,
	q interface {
		QueryRow(context.Context, string, ...any) pgx.Row
	},
	businessID string,
	batchID string,
	forUpdate bool,
) (Batch, error) {
	lock := ""
	if forUpdate {
		lock = " FOR UPDATE"
	}

	return scanBatch(
		q.QueryRow(
			ctx,
			`
            SELECT `+batchColumns+`
              FROM vendor_production_batches
             WHERE id=$1
               AND business_id=$2
            `+lock,
			strings.TrimSpace(batchID),
			strings.TrimSpace(businessID),
		),
	)
}

func (s *Service) GetBatch(
	ctx context.Context,
	actor production.Actor,
	batchID string,
) (Batch, error) {
	if s == nil || s.db == nil {
		return Batch{}, ErrStateConflict
	}

	return s.loadBatchQ(
		ctx,
		s.db,
		actor.VendorID,
		batchID,
		false,
	)
}

func (s *Service) validateProductionJobQ(
	ctx context.Context,
	q interface {
		QueryRow(context.Context, string, ...any) pgx.Row
	},
	businessID string,
	branchID string,
	jobID string,
) error {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return nil
	}

	var status string

	err := q.QueryRow(
		ctx,
		`
        SELECT status
          FROM production_jobs
         WHERE id=$1
           AND business_id=$2
           AND branch_id=$3
        `,
		jobID,
		businessID,
		branchID,
	).Scan(&status)

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrCapacityConflict
	}
	if err != nil {
		return err
	}

	if status == "CANCELLED" || status == "COMPLETED" {
		return ErrCapacityConflict
	}

	return nil
}

func (s *Service) loadPolicyQ(
	ctx context.Context,
	q interface {
		QueryRow(context.Context, string, ...any) pgx.Row
	},
	businessID string,
	branchID string,
	targetType string,
	targetRef string,
	targetVariantRef string,
) (StockPolicy, error) {
	var out StockPolicy

	err := q.QueryRow(
		ctx,
		`
        SELECT
            id,
            business_id,
            branch_id,
            target_type,
            target_ref,
            target_variant_ref,
            operating_model,
            COALESCE(prepared_inventory_item_id,''),
            output_inventory_numerator,
            output_inventory_denominator,
            status,
            version,
            created_at,
            updated_at
          FROM vendor_production_stock_policies
         WHERE business_id=$1
           AND branch_id=$2
           AND target_type=$3
           AND target_ref=$4
           AND target_variant_ref=$5
           AND status='ACTIVE'
        `,
		businessID,
		branchID,
		targetType,
		targetRef,
		targetVariantRef,
	).Scan(
		&out.ID,
		&out.BusinessID,
		&out.BranchID,
		&out.TargetType,
		&out.TargetRef,
		&out.TargetVariantRef,
		&out.OperatingModel,
		&out.PreparedInventoryItemID,
		&out.OutputInventoryNumerator,
		&out.OutputInventoryDenominator,
		&out.Status,
		&out.Version,
		&out.CreatedAt,
		&out.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return StockPolicy{
			BusinessID:                 businessID,
			BranchID:                   branchID,
			TargetType:                 targetType,
			TargetRef:                  targetRef,
			TargetVariantRef:           targetVariantRef,
			OperatingModel:             OperatingOnDemand,
			OutputInventoryNumerator:   1,
			OutputInventoryDenominator: 1,
			Status:                     "ACTIVE",
		}, nil
	}

	return out, err
}

func (s *Service) CreateBatch(
	ctx context.Context,
	actor production.Actor,
	input CreateBatchInput,
) (Batch, error) {
	input.BranchID = strings.TrimSpace(input.BranchID)
	input.RecipeID = strings.TrimSpace(input.RecipeID)
	input.RecipeVersionID = strings.TrimSpace(input.RecipeVersionID)
	input.ProductionJobID = strings.TrimSpace(input.ProductionJobID)

	planned, err := normalizeFraction(
		input.PlannedOutputNumerator,
		input.PlannedOutputDenominator,
		false,
	)
	if err != nil ||
		input.BranchID == "" ||
		input.RecipeID == "" ||
		input.RecipeVersionID == "" {
		return Batch{}, ErrInvalidInput
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Batch{}, err
	}
	defer tx.Rollback(ctx)

	var targetType string
	var targetRef string
	var targetVariantRef string
	var outputUnit string
	var versionNumber int
	var versionStatus string
	var expectedN int64
	var expectedD int64
	var expectedWaste int

	err = tx.QueryRow(
		ctx,
		`
        SELECT
            r.target_type,
            r.target_ref,
            r.target_variant_ref,
            v.output_unit_code,
            v.version_number,
            v.status,
            v.expected_output_numerator,
            v.expected_output_denominator,
            v.expected_waste_bps
          FROM vendor_recipes r
          JOIN vendor_recipe_versions v
            ON v.recipe_id=r.id
           AND v.business_id=r.business_id
         WHERE r.id=$1
           AND r.business_id=$2
           AND r.branch_id=$3
           AND r.status='ACTIVE'
           AND v.id=$4
        `,
		input.RecipeID,
		actor.VendorID,
		input.BranchID,
		input.RecipeVersionID,
	).Scan(
		&targetType,
		&targetRef,
		&targetVariantRef,
		&outputUnit,
		&versionNumber,
		&versionStatus,
		&expectedN,
		&expectedD,
		&expectedWaste,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return Batch{}, ErrNotFound
	}
	if err != nil {
		return Batch{}, err
	}
	if versionStatus != "ACTIVE" {
		return Batch{}, ErrStateConflict
	}

	expected, err := normalizeFraction(expectedN, expectedD, false)
	if err != nil {
		return Batch{}, err
	}

	multiplier, err := divideFraction(planned, expected)
	if err != nil {
		return Batch{}, err
	}

	if err = s.validateProductionJobQ(
		ctx,
		tx,
		actor.VendorID,
		input.BranchID,
		input.ProductionJobID,
	); err != nil {
		return Batch{}, err
	}

	policy, err := s.loadPolicyQ(
		ctx,
		tx,
		actor.VendorID,
		input.BranchID,
		targetType,
		targetRef,
		targetVariantRef,
	)
	if err != nil {
		return Batch{}, err
	}

	out := Batch{
		ID:                         "vpb_" + uuid.NewString(),
		BusinessID:                 actor.VendorID,
		BranchID:                   input.BranchID,
		RecipeID:                   input.RecipeID,
		RecipeVersionID:            input.RecipeVersionID,
		RecipeVersionNumber:        versionNumber,
		TargetType:                 targetType,
		TargetRef:                  targetRef,
		TargetVariantRef:           targetVariantRef,
		ProductionJobID:            input.ProductionJobID,
		PlannedOutputNumerator:     planned.N,
		PlannedOutputDenominator:   planned.D,
		OutputUnitCode:             outputUnit,
		ExpectedOutputNumerator:    expected.N,
		ExpectedOutputDenominator:  expected.D,
		RecipeExpectedWasteBPS:     expectedWaste,
		MultiplierNumerator:        multiplier.N,
		MultiplierDenominator:      multiplier.D,
		OperatingModel:             policy.OperatingModel,
		PreparedInventoryItemID:    policy.PreparedInventoryItemID,
		OutputInventoryNumerator:   policy.OutputInventoryNumerator,
		OutputInventoryDenominator: policy.OutputInventoryDenominator,
		Status:                     StatusDraft,
		CreatedByStaffID:           actor.StaffID,
		Version:                    1,
	}

	err = tx.QueryRow(
		ctx,
		`
        INSERT INTO vendor_production_batches(
            id,
            business_id,
            branch_id,
            recipe_id,
            recipe_version_id,
            recipe_version_number,
            target_type,
            target_ref,
            target_variant_ref,
            production_job_id,
            planned_output_numerator,
            planned_output_denominator,
            output_unit_code,
            expected_output_numerator,
            expected_output_denominator,
            recipe_expected_waste_bps,
            multiplier_numerator,
            multiplier_denominator,
            operating_model,
            prepared_inventory_item_id,
            output_inventory_numerator,
            output_inventory_denominator,
            created_by_staff_id
        )
        VALUES(
            $1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,''),
            $11,$12,$13,$14,$15,$16,$17,$18,$19,NULLIF($20,''),
            $21,$22,NULLIF($23,'')
        )
        RETURNING created_at,updated_at
        `,
		out.ID,
		out.BusinessID,
		out.BranchID,
		out.RecipeID,
		out.RecipeVersionID,
		out.RecipeVersionNumber,
		out.TargetType,
		out.TargetRef,
		out.TargetVariantRef,
		out.ProductionJobID,
		out.PlannedOutputNumerator,
		out.PlannedOutputDenominator,
		out.OutputUnitCode,
		out.ExpectedOutputNumerator,
		out.ExpectedOutputDenominator,
		out.RecipeExpectedWasteBPS,
		out.MultiplierNumerator,
		out.MultiplierDenominator,
		out.OperatingModel,
		out.PreparedInventoryItemID,
		out.OutputInventoryNumerator,
		out.OutputInventoryDenominator,
		out.CreatedByStaffID,
	).Scan(
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		return Batch{}, err
	}

	if err = s.production.RecordMutation(
		ctx,
		tx,
		actor,
		"vendor.production_batch.created",
		"vendor.production_batch.created",
		"vendor_production_batch",
		out.ID,
		out.BranchID,
		out.Version,
		nil,
		out,
		out,
		"",
	); err != nil {
		return Batch{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return Batch{}, err
	}

	return out, nil
}

type materialPlan struct {
	kind        string
	referenceID string
	sourceID    string
	unit        string
	quantity    fraction
	wasteBPS    int
	optional    bool
	controlled  bool
	inventoryID string
	reserve     int64
	point       string
	sequence    int
}

func effectiveMode(base, override string) string {
	override = strings.TrimSpace(override)
	if override != "" {
		return override
	}
	return strings.TrimSpace(base)
}

func (s *Service) buildPlan(
	ctx context.Context,
	q interface {
		Query(context.Context, string, ...any) (pgx.Rows, error)
	},
	batch Batch,
) ([]materialPlan, error) {
	multiplier := fraction{
		N: batch.MultiplierNumerator,
		D: batch.MultiplierDenominator,
	}

	out := make([]materialPlan, 0)

	rows, err := q.Query(
		ctx,
		`
        SELECT
            c.id,
            c.ingredient_id,
            c.quantity_numerator,
            c.quantity_denominator,
            c.unit_code,
            c.normalization_status,
            c.normalized_quantity_numerator,
            c.normalized_quantity_denominator,
            c.normalized_unit_code,
            c.expected_waste_bps,
            c.is_optional,
            c.sequence,
            i.inventory_control_mode,
            COALESCE(bc.inventory_control_mode_override,''),
            COALESCE(bc.enabled,false),
            COALESCE(bc.production_eligible,false),
            COALESCE(bc.inventory_item_id,'')
          FROM vendor_recipe_components c
          JOIN vendor_ingredients i
            ON i.id=c.ingredient_id
           AND i.business_id=c.business_id
          LEFT JOIN vendor_ingredient_branch_configs bc
            ON bc.business_id=c.business_id
           AND bc.branch_id=$3
           AND bc.ingredient_id=c.ingredient_id
         WHERE c.business_id=$1
           AND c.recipe_version_id=$2
         ORDER BY c.sequence,c.id
        `,
		batch.BusinessID,
		batch.RecipeVersionID,
		batch.BranchID,
	)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var sourceID string
		var ref string
		var quantityN int64
		var quantityD int64
		var unit string
		var normalization string
		var normalizedN int64
		var normalizedD int64
		var normalizedUnit string
		var waste int
		var optional bool
		var sequence int
		var baseMode string
		var overrideMode string
		var enabled bool
		var productionEligible bool
		var inventoryID string

		if err = rows.Scan(
			&sourceID,
			&ref,
			&quantityN,
			&quantityD,
			&unit,
			&normalization,
			&normalizedN,
			&normalizedD,
			&normalizedUnit,
			&waste,
			&optional,
			&sequence,
			&baseMode,
			&overrideMode,
			&enabled,
			&productionEligible,
			&inventoryID,
		); err != nil {
			rows.Close()
			return nil, err
		}

		controlled := effectiveMode(baseMode, overrideMode) == "INVENTORY_CONTROLLED"

		base := fraction{}
		if normalization == "EXACT" {
			base, err = normalizeFraction(normalizedN, normalizedD, false)
			unit = normalizedUnit
		} else {
			base, err = normalizeFraction(quantityN, quantityD, false)
		}
		if err != nil {
			rows.Close()
			return nil, err
		}

		scaled, scaleErr := multiplyFraction(base, multiplier)
		if scaleErr != nil {
			rows.Close()
			return nil, scaleErr
		}

		effectiveWaste := waste
		if effectiveWaste == 0 {
			effectiveWaste = batch.RecipeExpectedWasteBPS
		}

		scaled, scaleErr = applyExpectedWaste(scaled, effectiveWaste)
		if scaleErr != nil {
			rows.Close()
			return nil, scaleErr
		}

		reserve := int64(0)

		if controlled {
			if !enabled ||
				!productionEligible ||
				inventoryID == "" ||
				normalization != "EXACT" {
				rows.Close()
				return nil, ErrInventoryRequired
			}

			if !optional {
				reserve, scaleErr = toExactInt64(scaled)
				if scaleErr != nil || reserve <= 0 {
					rows.Close()
					return nil, ErrInventoryRequired
				}
			}
		}

		out = append(out, materialPlan{
			kind:        MaterialIngredient,
			referenceID: ref,
			sourceID:    sourceID,
			unit:        unit,
			quantity:    scaled,
			wasteBPS:    effectiveWaste,
			optional:    optional,
			controlled:  controlled,
			inventoryID: inventoryID,
			reserve:     reserve,
			sequence:    sequence,
		})
	}

	if rows.Err() != nil {
		rows.Close()
		return nil, rows.Err()
	}
	rows.Close()

	rows, err = q.Query(
		ctx,
		`
        SELECT
            p.id,
            p.packaging_material_id,
            p.quantity_numerator,
            p.quantity_denominator,
            p.unit_code,
            p.normalization_status,
            p.normalized_quantity_numerator,
            p.normalized_quantity_denominator,
            p.normalized_unit_code,
            p.sequence,
            m.inventory_control_mode,
            COALESCE(bc.inventory_control_mode_override,''),
            COALESCE(bc.enabled,false),
            COALESCE(bc.production_eligible,false),
            COALESCE(bc.inventory_item_id,''),
            COALESCE(bc.production_consumption_point,'FULFILMENT')
          FROM vendor_recipe_packaging_requirements p
          JOIN vendor_packaging_materials m
            ON m.id=p.packaging_material_id
           AND m.business_id=p.business_id
          LEFT JOIN vendor_packaging_branch_configs bc
            ON bc.business_id=p.business_id
           AND bc.branch_id=$3
           AND bc.packaging_material_id=p.packaging_material_id
         WHERE p.business_id=$1
           AND p.recipe_version_id=$2
         ORDER BY p.sequence,p.id
        `,
		batch.BusinessID,
		batch.RecipeVersionID,
		batch.BranchID,
	)
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var sourceID string
		var ref string
		var quantityN int64
		var quantityD int64
		var unit string
		var normalization string
		var normalizedN int64
		var normalizedD int64
		var normalizedUnit string
		var sequence int
		var baseMode string
		var overrideMode string
		var enabled bool
		var productionEligible bool
		var inventoryID string
		var point string

		if err = rows.Scan(
			&sourceID,
			&ref,
			&quantityN,
			&quantityD,
			&unit,
			&normalization,
			&normalizedN,
			&normalizedD,
			&normalizedUnit,
			&sequence,
			&baseMode,
			&overrideMode,
			&enabled,
			&productionEligible,
			&inventoryID,
			&point,
		); err != nil {
			rows.Close()
			return nil, err
		}

		controlled := effectiveMode(baseMode, overrideMode) == "INVENTORY_CONTROLLED"

		base := fraction{}
		if normalization == "EXACT" {
			base, err = normalizeFraction(normalizedN, normalizedD, false)
			unit = normalizedUnit
		} else {
			base, err = normalizeFraction(quantityN, quantityD, false)
		}
		if err != nil {
			rows.Close()
			return nil, err
		}

		scaled, scaleErr := multiplyFraction(base, multiplier)
		if scaleErr != nil {
			rows.Close()
			return nil, scaleErr
		}

		reserve := int64(0)

		if controlled && point == "PRODUCTION" {
			if !enabled ||
				!productionEligible ||
				inventoryID == "" ||
				normalization != "EXACT" {
				rows.Close()
				return nil, ErrInventoryRequired
			}

			reserve, scaleErr = toExactInt64(scaled)
			if scaleErr != nil || reserve <= 0 {
				rows.Close()
				return nil, ErrInventoryRequired
			}
		}

		out = append(out, materialPlan{
			kind:        MaterialPackaging,
			referenceID: ref,
			sourceID:    sourceID,
			unit:        unit,
			quantity:    scaled,
			controlled:  controlled,
			inventoryID: inventoryID,
			reserve:     reserve,
			point:       point,
			sequence:    sequence,
		})
	}

	if rows.Err() != nil {
		rows.Close()
		return nil, rows.Err()
	}
	rows.Close()

	return out, nil
}

func (s *Service) PreviewBatch(
	ctx context.Context,
	actor production.Actor,
	batchID string,
) (PreviewResult, error) {
	batch, err := s.GetBatch(ctx, actor, batchID)
	if err != nil {
		return PreviewResult{}, err
	}

	if batch.Status != StatusDraft {
		return PreviewResult{}, ErrStateConflict
	}

	plan, err := s.buildPlan(ctx, s.db, batch)
	if err != nil {
		return PreviewResult{}, err
	}

	out := PreviewResult{
		BatchID:      batch.ID,
		Requirements: make([]RequirementPreview, 0, len(plan)),
	}

	for _, item := range plan {
		out.Requirements = append(
			out.Requirements,
			RequirementPreview{
				Kind:                       item.kind,
				ReferenceID:                item.referenceID,
				UnitCode:                   item.unit,
				QuantityNumerator:          item.quantity.N,
				QuantityDenominator:        item.quantity.D,
				ExpectedWasteBPS:           item.wasteBPS,
				InventoryControlled:        item.controlled,
				InventoryItemID:            item.inventoryID,
				ReservationQuantity:        item.reserve,
				ProductionConsumptionPoint: item.point,
				Optional:                   item.optional,
			},
		)
	}

	return out, nil
}

func aggregateReservationLines(
	plan []materialPlan,
) ([]inventory.ReservationLineInput, error) {
	totals := map[string]int64{}

	for _, item := range plan {
		if item.reserve <= 0 {
			continue
		}

		current := totals[item.inventoryID]
		if item.reserve > 0 && current > int64(^uint64(0)>>1)-item.reserve {
			return nil, ErrInvalidInput
		}
		totals[item.inventoryID] = current + item.reserve
	}

	ids := make([]string, 0, len(totals))
	for id := range totals {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	out := make([]inventory.ReservationLineInput, 0, len(ids))
	for _, id := range ids {
		out = append(out, inventory.ReservationLineInput{
			InventoryItemID: id,
			Quantity:        totals[id],
		})
	}

	return out, nil
}

func (s *Service) PlanBatch(
	ctx context.Context,
	actor production.Actor,
	batchID string,
	expectedVersion int64,
) (BatchDetail, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return BatchDetail{}, err
	}
	defer tx.Rollback(ctx)

	batch, err := s.loadBatchQ(
		ctx,
		tx,
		actor.VendorID,
		batchID,
		true,
	)
	if err != nil {
		return BatchDetail{}, err
	}

	if batch.Status != StatusDraft {
		return BatchDetail{}, ErrStateConflict
	}
	if batch.Version != expectedVersion {
		return BatchDetail{}, ErrVersionConflict
	}

	plan, err := s.buildPlan(ctx, tx, batch)
	if err != nil {
		return BatchDetail{}, err
	}

	reservationLines, err := aggregateReservationLines(plan)
	if err != nil {
		return BatchDetail{}, err
	}

	var reservation inventory.Reservation
	reservationCreated := false

	if len(reservationLines) > 0 {
		scope, scopeErr := inventoryScope(
			actor.VendorID,
			batch.BranchID,
		)
		if scopeErr != nil {
			return BatchDetail{}, scopeErr
		}

		sourceRef :=
			"PRODUCTION_BATCH:" +
				batch.ID +
				":" +
				uuid.NewString()

		reservation, err = s.inventory.CreateReservation(
			ctx,
			inventoryActor(actor),
			scope,
			inventory.CreateReservationInput{
				BranchID:   batch.BranchID,
				SourceType: inventory.ReservationSourceInternal,
				SourceRef:  sourceRef,
				Lines:      reservationLines,
			},
		)
		if err != nil {
			return BatchDetail{}, ErrInventoryShortage
		}

		reservationCreated = true

		reservation, err = s.inventory.ConfirmReservation(
			ctx,
			inventoryActor(actor),
			scope,
			reservation.ID,
		)
		if err != nil {
			_, _ = s.inventory.CancelReservation(
				ctx,
				inventoryActor(actor),
				scope,
				reservation.ID,
			)
			return BatchDetail{}, err
		}
	}

	cleanupReservation := func() {
		if !reservationCreated {
			return
		}
		scope, scopeErr := inventoryScope(
			actor.VendorID,
			batch.BranchID,
		)
		if scopeErr != nil {
			return
		}
		_, _ = s.inventory.ReleaseReservation(
			context.Background(),
			inventoryActor(actor),
			scope,
			reservation.ID,
		)
	}

	for _, item := range plan {
		if item.kind == MaterialIngredient {
			_, err = tx.Exec(
				ctx,
				`
                INSERT INTO vendor_production_batch_components(
                    id,
                    business_id,
                    batch_id,
                    ingredient_id,
                    recipe_component_id,
                    unit_code,
                    planned_quantity_numerator,
                    planned_quantity_denominator,
                    expected_waste_bps,
                    optional,
                    inventory_controlled,
                    inventory_item_id,
                    reserved_quantity,
                    sequence
                )
                VALUES(
                    $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NULLIF($12,''),$13,$14
                )
                `,
				"vpbc_"+uuid.NewString(),
				batch.BusinessID,
				batch.ID,
				item.referenceID,
				item.sourceID,
				item.unit,
				item.quantity.N,
				item.quantity.D,
				item.wasteBPS,
				item.optional,
				item.controlled,
				item.inventoryID,
				item.reserve,
				item.sequence,
			)
		} else {
			_, err = tx.Exec(
				ctx,
				`
                INSERT INTO vendor_production_batch_packaging(
                    id,
                    business_id,
                    batch_id,
                    packaging_material_id,
                    recipe_packaging_requirement_id,
                    unit_code,
                    planned_quantity_numerator,
                    planned_quantity_denominator,
                    production_consumption_point,
                    inventory_controlled,
                    inventory_item_id,
                    reserved_quantity,
                    sequence
                )
                VALUES(
                    $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NULLIF($11,''),$12,$13
                )
                `,
				"vpbp_"+uuid.NewString(),
				batch.BusinessID,
				batch.ID,
				item.referenceID,
				item.sourceID,
				item.unit,
				item.quantity.N,
				item.quantity.D,
				item.point,
				item.controlled,
				item.inventoryID,
				item.reserve,
				item.sequence,
			)
		}

		if err != nil {
			cleanupReservation()
			return BatchDetail{}, err
		}
	}

	if reservationCreated {
		_, err = tx.Exec(
			ctx,
			`
            INSERT INTO vendor_production_batch_reservations(
                id,
                business_id,
                branch_id,
                batch_id,
                reservation_kind,
                sequence,
                inventory_reservation_id,
                source_ref,
                status
            )
            VALUES(
                $1,$2,$3,$4,'PRIMARY',1,$5,$6,'CONFIRMED'
            )
            `,
			"vpbr_"+uuid.NewString(),
			batch.BusinessID,
			batch.BranchID,
			batch.ID,
			reservation.ID,
			reservation.SourceRef,
		)
		if err != nil {
			cleanupReservation()
			return BatchDetail{}, err
		}
	}

	err = tx.QueryRow(
		ctx,
		`
        UPDATE vendor_production_batches
           SET status='PLANNED',
               inventory_reservation_id=NULLIF($4,''),
               planned_by_staff_id=NULLIF($5,''),
               planned_at=now(),
               version=version+1,
               updated_at=now()
         WHERE id=$1
           AND business_id=$2
           AND version=$3
           AND status='DRAFT'
         RETURNING `+batchColumns,
		batch.ID,
		batch.BusinessID,
		expectedVersion,
		reservation.ID,
		actor.StaffID,
	).Scan(
		&batch.ID,
		&batch.BusinessID,
		&batch.BranchID,
		&batch.RecipeID,
		&batch.RecipeVersionID,
		&batch.RecipeVersionNumber,
		&batch.TargetType,
		&batch.TargetRef,
		&batch.TargetVariantRef,
		&batch.ProductionJobID,
		&batch.PlannedOutputNumerator,
		&batch.PlannedOutputDenominator,
		&batch.OutputUnitCode,
		&batch.ExpectedOutputNumerator,
		&batch.ExpectedOutputDenominator,
		&batch.RecipeExpectedWasteBPS,
		&batch.MultiplierNumerator,
		&batch.MultiplierDenominator,
		&batch.OperatingModel,
		&batch.PreparedInventoryItemID,
		&batch.OutputInventoryNumerator,
		&batch.OutputInventoryDenominator,
		&batch.InventoryReservationID,
		&batch.Status,
		&batch.ActualConsumptionRecorded,
		&batch.ActualOutputNumerator,
		&batch.ActualOutputDenominator,
		&batch.CreatedByStaffID,
		&batch.PlannedByStaffID,
		&batch.StartedByStaffID,
		&batch.CompletedByStaffID,
		&batch.CancelledByStaffID,
		&batch.PlannedAt,
		&batch.StartedAt,
		&batch.CompletedAt,
		&batch.CancelledAt,
		&batch.CancelReason,
		&batch.Version,
		&batch.CreatedAt,
		&batch.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		cleanupReservation()
		return BatchDetail{}, ErrVersionConflict
	}
	if err != nil {
		cleanupReservation()
		return BatchDetail{}, err
	}

	if err = s.production.RecordMutation(
		ctx,
		tx,
		actor,
		"vendor.production_batch.planned",
		"vendor.production_batch.planned",
		"vendor_production_batch",
		batch.ID,
		batch.BranchID,
		batch.Version,
		nil,
		batch,
		map[string]any{
			"batch_id":                 batch.ID,
			"inventory_reservation_id": reservation.ID,
			"requirement_count":        len(plan),
		},
		"",
	); err != nil {
		cleanupReservation()
		return BatchDetail{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		cleanupReservation()
		return BatchDetail{}, err
	}

	return s.GetBatchDetail(ctx, actor, batch.ID)
}

func (s *Service) loadComponents(
	ctx context.Context,
	businessID string,
	batchID string,
) ([]ComponentSnapshot, error) {
	rows, err := s.db.Query(
		ctx,
		`
        SELECT
            id,
            ingredient_id,
            recipe_component_id,
            unit_code,
            planned_quantity_numerator,
            planned_quantity_denominator,
            expected_waste_bps,
            optional,
            inventory_controlled,
            COALESCE(inventory_item_id,''),
            reserved_quantity,
            actual_used_numerator,
            actual_used_denominator,
            actual_waste_numerator,
            actual_waste_denominator,
            actual_recorded,
            variance_numerator,
            variance_denominator,
            sequence
          FROM vendor_production_batch_components
         WHERE business_id=$1
           AND batch_id=$2
         ORDER BY sequence,id
        `,
		businessID,
		batchID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ComponentSnapshot, 0)

	for rows.Next() {
		var item ComponentSnapshot
		if err = rows.Scan(
			&item.ID,
			&item.IngredientID,
			&item.RecipeComponentID,
			&item.UnitCode,
			&item.PlannedQuantityNumerator,
			&item.PlannedQuantityDenominator,
			&item.ExpectedWasteBPS,
			&item.Optional,
			&item.InventoryControlled,
			&item.InventoryItemID,
			&item.ReservedQuantity,
			&item.ActualUsedNumerator,
			&item.ActualUsedDenominator,
			&item.ActualWasteNumerator,
			&item.ActualWasteDenominator,
			&item.ActualRecorded,
			&item.VarianceNumerator,
			&item.VarianceDenominator,
			&item.Sequence,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}

	return out, rows.Err()
}

func (s *Service) loadPackaging(
	ctx context.Context,
	businessID string,
	batchID string,
) ([]PackagingSnapshot, error) {
	rows, err := s.db.Query(
		ctx,
		`
        SELECT
            id,
            packaging_material_id,
            recipe_packaging_requirement_id,
            unit_code,
            planned_quantity_numerator,
            planned_quantity_denominator,
            production_consumption_point,
            inventory_controlled,
            COALESCE(inventory_item_id,''),
            reserved_quantity,
            actual_used_numerator,
            actual_used_denominator,
            actual_waste_numerator,
            actual_waste_denominator,
            actual_recorded,
            variance_numerator,
            variance_denominator,
            sequence
          FROM vendor_production_batch_packaging
         WHERE business_id=$1
           AND batch_id=$2
         ORDER BY sequence,id
        `,
		businessID,
		batchID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]PackagingSnapshot, 0)

	for rows.Next() {
		var item PackagingSnapshot
		if err = rows.Scan(
			&item.ID,
			&item.PackagingMaterialID,
			&item.RecipePackagingRequirementID,
			&item.UnitCode,
			&item.PlannedQuantityNumerator,
			&item.PlannedQuantityDenominator,
			&item.ProductionConsumptionPoint,
			&item.InventoryControlled,
			&item.InventoryItemID,
			&item.ReservedQuantity,
			&item.ActualUsedNumerator,
			&item.ActualUsedDenominator,
			&item.ActualWasteNumerator,
			&item.ActualWasteDenominator,
			&item.ActualRecorded,
			&item.VarianceNumerator,
			&item.VarianceDenominator,
			&item.Sequence,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}

	return out, rows.Err()
}

func (s *Service) GetBatchDetail(
	ctx context.Context,
	actor production.Actor,
	batchID string,
) (BatchDetail, error) {
	batch, err := s.GetBatch(ctx, actor, batchID)
	if err != nil {
		return BatchDetail{}, err
	}

	components, err := s.loadComponents(
		ctx,
		actor.VendorID,
		batch.ID,
	)
	if err != nil {
		return BatchDetail{}, err
	}

	packaging, err := s.loadPackaging(
		ctx,
		actor.VendorID,
		batch.ID,
	)
	if err != nil {
		return BatchDetail{}, err
	}

	return BatchDetail{
		Batch:      batch,
		Components: components,
		Packaging:  packaging,
	}, nil
}

func (s *Service) ListBatches(
	ctx context.Context,
	actor production.Actor,
	scope production.Scope,
	branchID string,
	status string,
	limit int,
	offset int,
) ([]Batch, bool, error) {
	branchID = strings.TrimSpace(branchID)
	status = strings.ToUpper(strings.TrimSpace(status))

	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		return nil, false, ErrInvalidInput
	}

	if branchID != "" && !scope.Covers(branchID) {
		return nil, false, ErrNotFound
	}

	if !scope.Global && branchID == "" {
		return nil, false, ErrInvalidInput
	}

	rows, err := s.db.Query(
		ctx,
		`
        SELECT `+batchColumns+`
          FROM vendor_production_batches
         WHERE business_id=$1
           AND ($2='' OR branch_id=$2)
           AND ($3='' OR status=$3)
         ORDER BY created_at DESC,id DESC
         LIMIT $4 OFFSET $5
        `,
		actor.VendorID,
		branchID,
		status,
		limit+1,
		offset,
	)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	out := make([]Batch, 0, limit+1)

	for rows.Next() {
		item, scanErr := scanBatch(rows)
		if scanErr != nil {
			return nil, false, scanErr
		}

		if !scope.Covers(item.BranchID) {
			continue
		}

		out = append(out, item)
	}

	if rows.Err() != nil {
		return nil, false, rows.Err()
	}

	hasMore := len(out) > limit
	if hasMore {
		out = out[:limit]
	}

	return out, hasMore, nil
}

func (s *Service) StartBatch(
	ctx context.Context,
	actor production.Actor,
	batchID string,
	expectedVersion int64,
) (Batch, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Batch{}, err
	}
	defer tx.Rollback(ctx)

	batch, err := s.loadBatchQ(
		ctx,
		tx,
		actor.VendorID,
		batchID,
		true,
	)
	if err != nil {
		return Batch{}, err
	}

	if batch.Version != expectedVersion {
		return Batch{}, ErrVersionConflict
	}
	if batch.Status != StatusPlanned {
		return Batch{}, ErrStateConflict
	}

	if err = s.validateProductionJobQ(
		ctx,
		tx,
		batch.BusinessID,
		batch.BranchID,
		batch.ProductionJobID,
	); err != nil {
		return Batch{}, err
	}

	rows, err := tx.Query(
		ctx,
		`
        SELECT
            r.id,
            r.status,
            r.expires_at
          FROM vendor_production_batch_reservations br
          JOIN inventory_reservations r
            ON r.id=br.inventory_reservation_id
           AND r.business_id=br.business_id
           AND r.branch_id=br.branch_id
         WHERE br.business_id=$1
           AND br.batch_id=$2
         ORDER BY r.id
         FOR SHARE OF r
        `,
		batch.BusinessID,
		batch.ID,
	)
	if err != nil {
		return Batch{}, err
	}

	now := s.production.Now()

	for rows.Next() {
		var reservationID string
		var status string
		var expiresAt *time.Time

		if err = rows.Scan(
			&reservationID,
			&status,
			&expiresAt,
		); err != nil {
			rows.Close()
			return Batch{}, err
		}

		if status != inventory.ReservationHeld &&
			status != inventory.ReservationConfirmed {
			rows.Close()
			return Batch{}, ErrInventoryShortage
		}

		if expiresAt != nil && !expiresAt.After(now) {
			rows.Close()
			return Batch{}, ErrInventoryShortage
		}

		_ = reservationID
	}
	if rows.Err() != nil {
		rows.Close()
		return Batch{}, rows.Err()
	}
	rows.Close()

	batch, err = scanBatch(
		tx.QueryRow(
			ctx,
			`
            UPDATE vendor_production_batches
               SET status='IN_PROGRESS',
                   started_by_staff_id=NULLIF($4,''),
                   started_at=now(),
                   version=version+1,
                   updated_at=now()
             WHERE id=$1
               AND business_id=$2
               AND version=$3
               AND status='PLANNED'
             RETURNING `+batchColumns,
			batch.ID,
			batch.BusinessID,
			expectedVersion,
			actor.StaffID,
		),
	)
	if err != nil {
		return Batch{}, err
	}

	if err = s.production.RecordMutation(
		ctx,
		tx,
		actor,
		"vendor.production_batch.started",
		"vendor.production_batch.started",
		"vendor_production_batch",
		batch.ID,
		batch.BranchID,
		batch.Version,
		nil,
		batch,
		batch,
		"",
	); err != nil {
		return Batch{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return Batch{}, err
	}

	return batch, nil
}

// mutationTarget loads one frozen batch material under the same transaction.
type mutationTarget struct {
	kind string
	id   string

	unit string

	planned fraction

	controlled bool
	optional   bool
	point      string

	inventoryID string
	reserved    int64

	used     fraction
	waste    fraction
	recorded bool
}

func loadMutationTargetQ(
	ctx context.Context,
	tx pgx.Tx,
	businessID string,
	batchID string,
	kind string,
	ref string,
) (mutationTarget, error) {
	kind = strings.ToUpper(strings.TrimSpace(kind))
	ref = strings.TrimSpace(ref)

	var out mutationTarget
	out.kind = kind

	if kind == MaterialIngredient {
		var plannedN, plannedD int64
		var usedN, usedD int64
		var wasteN, wasteD int64

		err := tx.QueryRow(
			ctx,
			`
            SELECT
                id,
                unit_code,
                planned_quantity_numerator,
                planned_quantity_denominator,
                inventory_controlled,
                optional,
                COALESCE(inventory_item_id,''),
                reserved_quantity,
                actual_used_numerator,
                actual_used_denominator,
                actual_waste_numerator,
                actual_waste_denominator,
                actual_recorded
              FROM vendor_production_batch_components
             WHERE business_id=$1
               AND batch_id=$2
               AND ingredient_id=$3
             FOR UPDATE
            `,
			businessID,
			batchID,
			ref,
		).Scan(
			&out.id,
			&out.unit,
			&plannedN,
			&plannedD,
			&out.controlled,
			&out.optional,
			&out.inventoryID,
			&out.reserved,
			&usedN,
			&usedD,
			&wasteN,
			&wasteD,
			&out.recorded,
		)

		if errors.Is(err, pgx.ErrNoRows) {
			return mutationTarget{}, ErrNotFound
		}
		if err != nil {
			return mutationTarget{}, err
		}

		out.planned, err = normalizeFraction(plannedN, plannedD, false)
		if err != nil {
			return mutationTarget{}, err
		}
		out.used, err = normalizeFraction(usedN, usedD, true)
		if err != nil {
			return mutationTarget{}, err
		}
		out.waste, err = normalizeFraction(wasteN, wasteD, true)
		return out, err
	}

	if kind == MaterialPackaging {
		var plannedN, plannedD int64
		var usedN, usedD int64
		var wasteN, wasteD int64

		err := tx.QueryRow(
			ctx,
			`
            SELECT
                id,
                unit_code,
                planned_quantity_numerator,
                planned_quantity_denominator,
                inventory_controlled,
                production_consumption_point,
                COALESCE(inventory_item_id,''),
                reserved_quantity,
                actual_used_numerator,
                actual_used_denominator,
                actual_waste_numerator,
                actual_waste_denominator,
                actual_recorded
              FROM vendor_production_batch_packaging
             WHERE business_id=$1
               AND batch_id=$2
               AND packaging_material_id=$3
             FOR UPDATE
            `,
			businessID,
			batchID,
			ref,
		).Scan(
			&out.id,
			&out.unit,
			&plannedN,
			&plannedD,
			&out.controlled,
			&out.point,
			&out.inventoryID,
			&out.reserved,
			&usedN,
			&usedD,
			&wasteN,
			&wasteD,
			&out.recorded,
		)

		if errors.Is(err, pgx.ErrNoRows) {
			return mutationTarget{}, ErrNotFound
		}
		if err != nil {
			return mutationTarget{}, err
		}

		out.planned, err = normalizeFraction(plannedN, plannedD, false)
		if err != nil {
			return mutationTarget{}, err
		}
		out.used, err = normalizeFraction(usedN, usedD, true)
		if err != nil {
			return mutationTarget{}, err
		}
		out.waste, err = normalizeFraction(wasteN, wasteD, true)
		return out, err
	}

	return mutationTarget{}, ErrInvalidInput
}

func ensureControlledWithinReservation(
	target mutationTarget,
	used fraction,
	waste fraction,
) error {
	if !target.controlled ||
		(target.kind == MaterialPackaging &&
			target.point != "PRODUCTION") {
		return nil
	}

	if used.D != 1 || waste.D != 1 {
		return ErrInventoryRequired
	}

	total := used.N + waste.N

	if total > target.reserved {
		return ErrTopUpRequired
	}

	return nil
}

func (s *Service) RecordActualConsumption(
	ctx context.Context,
	actor production.Actor,
	batchID string,
	input ActualConsumptionInput,
	expectedVersion int64,
) (BatchDetail, error) {
	input.Kind = strings.ToUpper(strings.TrimSpace(input.Kind))
	input.ReferenceID = strings.TrimSpace(input.ReferenceID)

	actual, err := normalizeFraction(
		input.QuantityNumerator,
		input.QuantityDenominator,
		true,
	)
	if err != nil || input.ReferenceID == "" {
		return BatchDetail{}, ErrInvalidInput
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return BatchDetail{}, err
	}
	defer tx.Rollback(ctx)

	batch, err := s.loadBatchQ(
		ctx,
		tx,
		actor.VendorID,
		batchID,
		true,
	)
	if err != nil {
		return BatchDetail{}, err
	}

	if batch.Status != StatusInProgress {
		return BatchDetail{}, ErrStateConflict
	}
	if batch.Version != expectedVersion {
		return BatchDetail{}, ErrVersionConflict
	}

	target, err := loadMutationTargetQ(
		ctx,
		tx,
		batch.BusinessID,
		batch.ID,
		input.Kind,
		input.ReferenceID,
	)
	if err != nil {
		return BatchDetail{}, err
	}

	if err = ensureControlledWithinReservation(
		target,
		actual,
		target.waste,
	); err != nil {
		return BatchDetail{}, err
	}

	variance, err := subtractFraction(actual, target.planned)
	if err != nil {
		return BatchDetail{}, err
	}

	if input.Kind == MaterialIngredient {
		_, err = tx.Exec(
			ctx,
			`
            UPDATE vendor_production_batch_components
               SET actual_used_numerator=$4,
                   actual_used_denominator=$5,
                   actual_recorded=true,
                   variance_numerator=$6,
                   variance_denominator=$7,
                   updated_at=now()
             WHERE id=$1
               AND business_id=$2
               AND batch_id=$3
            `,
			target.id,
			batch.BusinessID,
			batch.ID,
			actual.N,
			actual.D,
			variance.N,
			variance.D,
		)
	} else {
		_, err = tx.Exec(
			ctx,
			`
            UPDATE vendor_production_batch_packaging
               SET actual_used_numerator=$4,
                   actual_used_denominator=$5,
                   actual_recorded=true,
                   variance_numerator=$6,
                   variance_denominator=$7,
                   updated_at=now()
             WHERE id=$1
               AND business_id=$2
               AND batch_id=$3
            `,
			target.id,
			batch.BusinessID,
			batch.ID,
			actual.N,
			actual.D,
			variance.N,
			variance.D,
		)
	}
	if err != nil {
		return BatchDetail{}, err
	}

	batch, err = scanBatch(
		tx.QueryRow(
			ctx,
			`
            UPDATE vendor_production_batches
               SET actual_consumption_recorded=true,
                   version=version+1,
                   updated_at=now()
             WHERE id=$1
               AND business_id=$2
               AND version=$3
             RETURNING `+batchColumns,
			batch.ID,
			batch.BusinessID,
			expectedVersion,
		),
	)
	if err != nil {
		return BatchDetail{}, err
	}

	if err = s.production.RecordMutation(
		ctx,
		tx,
		actor,
		"vendor.production.actual_consumption.recorded",
		"vendor.production.actual_consumption.recorded",
		"vendor_production_batch",
		batch.ID,
		batch.BranchID,
		batch.Version,
		nil,
		map[string]any{
			"kind":         input.Kind,
			"reference_id": input.ReferenceID,
			"quantity_n":   actual.N,
			"quantity_d":   actual.D,
		},
		map[string]any{
			"batch_id": batch.ID,
		},
		"",
	); err != nil {
		return BatchDetail{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return BatchDetail{}, err
	}

	return s.GetBatchDetail(ctx, actor, batch.ID)
}

func (s *Service) RecordWaste(
	ctx context.Context,
	actor production.Actor,
	batchID string,
	input WasteInput,
	expectedVersion int64,
) (BatchDetail, error) {
	input.Kind = strings.ToUpper(strings.TrimSpace(input.Kind))
	input.ReferenceID = strings.TrimSpace(input.ReferenceID)
	input.ReasonCode = strings.TrimSpace(input.ReasonCode)

	delta, err := normalizeFraction(
		input.QuantityNumerator,
		input.QuantityDenominator,
		false,
	)
	if err != nil ||
		input.ReferenceID == "" ||
		input.ReasonCode == "" {
		return BatchDetail{}, ErrInvalidInput
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return BatchDetail{}, err
	}
	defer tx.Rollback(ctx)

	batch, err := s.loadBatchQ(
		ctx,
		tx,
		actor.VendorID,
		batchID,
		true,
	)
	if err != nil {
		return BatchDetail{}, err
	}

	if batch.Status != StatusInProgress {
		return BatchDetail{}, ErrStateConflict
	}
	if batch.Version != expectedVersion {
		return BatchDetail{}, ErrVersionConflict
	}

	target, err := loadMutationTargetQ(
		ctx,
		tx,
		batch.BusinessID,
		batch.ID,
		input.Kind,
		input.ReferenceID,
	)
	if err != nil {
		return BatchDetail{}, err
	}

	newWaste, err := addFraction(target.waste, delta)
	if err != nil {
		return BatchDetail{}, err
	}

	if err = ensureControlledWithinReservation(
		target,
		target.used,
		newWaste,
	); err != nil {
		return BatchDetail{}, err
	}

	actualTotal, err := addFraction(target.used, newWaste)
	if err != nil {
		return BatchDetail{}, err
	}

	variance, err := subtractFraction(actualTotal, target.planned)
	if err != nil {
		return BatchDetail{}, err
	}

	if input.Kind == MaterialIngredient {
		_, err = tx.Exec(
			ctx,
			`
            UPDATE vendor_production_batch_components
               SET actual_waste_numerator=$4,
                   actual_waste_denominator=$5,
                   variance_numerator=$6,
                   variance_denominator=$7,
                   updated_at=now()
             WHERE id=$1
               AND business_id=$2
               AND batch_id=$3
            `,
			target.id,
			batch.BusinessID,
			batch.ID,
			newWaste.N,
			newWaste.D,
			variance.N,
			variance.D,
		)
	} else {
		_, err = tx.Exec(
			ctx,
			`
            UPDATE vendor_production_batch_packaging
               SET actual_waste_numerator=$4,
                   actual_waste_denominator=$5,
                   variance_numerator=$6,
                   variance_denominator=$7,
                   updated_at=now()
             WHERE id=$1
               AND business_id=$2
               AND batch_id=$3
            `,
			target.id,
			batch.BusinessID,
			batch.ID,
			newWaste.N,
			newWaste.D,
			variance.N,
			variance.D,
		)
	}
	if err != nil {
		return BatchDetail{}, err
	}

	_, err = tx.Exec(
		ctx,
		`
        INSERT INTO vendor_production_batch_waste_events(
            id,
            business_id,
            branch_id,
            batch_id,
            material_kind,
            material_ref,
            quantity_numerator,
            quantity_denominator,
            unit_code,
            reason_code,
            actor_ref
        )
        VALUES(
            $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NULLIF($11,'')
        )
        `,
		"vpbw_"+uuid.NewString(),
		batch.BusinessID,
		batch.BranchID,
		batch.ID,
		input.Kind,
		input.ReferenceID,
		delta.N,
		delta.D,
		target.unit,
		input.ReasonCode,
		actor.StaffID,
	)
	if err != nil {
		return BatchDetail{}, err
	}

	batch, err = scanBatch(
		tx.QueryRow(
			ctx,
			`
            UPDATE vendor_production_batches
               SET version=version+1,
                   updated_at=now()
             WHERE id=$1
               AND business_id=$2
               AND version=$3
             RETURNING `+batchColumns,
			batch.ID,
			batch.BusinessID,
			expectedVersion,
		),
	)
	if err != nil {
		return BatchDetail{}, err
	}

	if err = s.production.RecordMutation(
		ctx,
		tx,
		actor,
		"vendor.production.waste.recorded",
		"vendor.production.waste.recorded",
		"vendor_production_batch",
		batch.ID,
		batch.BranchID,
		batch.Version,
		nil,
		map[string]any{
			"kind":         input.Kind,
			"reference_id": input.ReferenceID,
			"quantity_n":   delta.N,
			"quantity_d":   delta.D,
			"reason_code":  input.ReasonCode,
		},
		map[string]any{
			"batch_id": batch.ID,
		},
		input.ReasonCode,
	); err != nil {
		return BatchDetail{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return BatchDetail{}, err
	}

	return s.GetBatchDetail(ctx, actor, batch.ID)
}

func (s *Service) TopUpInventory(
	ctx context.Context,
	actor production.Actor,
	batchID string,
	input TopUpInput,
	expectedVersion int64,
) (BatchDetail, error) {
	if len(input.Lines) == 0 || len(input.Lines) > 100 {
		return BatchDetail{}, ErrInvalidInput
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return BatchDetail{}, err
	}
	defer tx.Rollback(ctx)

	batch, err := s.loadBatchQ(
		ctx,
		tx,
		actor.VendorID,
		batchID,
		true,
	)
	if err != nil {
		return BatchDetail{}, err
	}

	if batch.Status != StatusPlanned &&
		batch.Status != StatusInProgress {
		return BatchDetail{}, ErrStateConflict
	}
	if batch.Version != expectedVersion {
		return BatchDetail{}, ErrVersionConflict
	}

	type resolvedLine struct {
		kind        string
		ref         string
		inventoryID string
		quantity    int64
		targetID    string
	}

	resolved := make([]resolvedLine, 0, len(input.Lines))
	reservationTotals := map[string]int64{}

	for _, line := range input.Lines {
		line.Kind = strings.ToUpper(strings.TrimSpace(line.Kind))
		line.ReferenceID = strings.TrimSpace(line.ReferenceID)

		if line.ReferenceID == "" || line.Quantity <= 0 {
			return BatchDetail{}, ErrInvalidInput
		}

		target, loadErr := loadMutationTargetQ(
			ctx,
			tx,
			batch.BusinessID,
			batch.ID,
			line.Kind,
			line.ReferenceID,
		)
		if loadErr != nil {
			return BatchDetail{}, loadErr
		}

		if !target.controlled ||
			target.inventoryID == "" ||
			(target.kind == MaterialPackaging &&
				target.point != "PRODUCTION") {
			return BatchDetail{}, ErrInvalidInput
		}

		reservationTotals[target.inventoryID] += line.Quantity

		resolved = append(resolved, resolvedLine{
			kind:        line.Kind,
			ref:         line.ReferenceID,
			inventoryID: target.inventoryID,
			quantity:    line.Quantity,
			targetID:    target.id,
		})
	}

	ids := make([]string, 0, len(reservationTotals))
	for id := range reservationTotals {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	reservationLines := make([]inventory.ReservationLineInput, 0, len(ids))
	for _, id := range ids {
		reservationLines = append(
			reservationLines,
			inventory.ReservationLineInput{
				InventoryItemID: id,
				Quantity:        reservationTotals[id],
			},
		)
	}

	scope, err := inventoryScope(batch.BusinessID, batch.BranchID)
	if err != nil {
		return BatchDetail{}, err
	}

	sourceRef :=
		"PRODUCTION_BATCH_TOPUP:" +
			batch.ID +
			":" +
			uuid.NewString()

	reservation, err := s.inventory.CreateReservation(
		ctx,
		inventoryActor(actor),
		scope,
		inventory.CreateReservationInput{
			BranchID:   batch.BranchID,
			SourceType: inventory.ReservationSourceInternal,
			SourceRef:  sourceRef,
			Lines:      reservationLines,
		},
	)
	if err != nil {
		return BatchDetail{}, ErrInventoryShortage
	}

	reservation, err = s.inventory.ConfirmReservation(
		ctx,
		inventoryActor(actor),
		scope,
		reservation.ID,
	)
	if err != nil {
		_, _ = s.inventory.CancelReservation(
			ctx,
			inventoryActor(actor),
			scope,
			reservation.ID,
		)
		return BatchDetail{}, err
	}

	cleanup := func() {
		_, _ = s.inventory.ReleaseReservation(
			context.Background(),
			inventoryActor(actor),
			scope,
			reservation.ID,
		)
	}

	var sequence int
	err = tx.QueryRow(
		ctx,
		`
        SELECT COALESCE(MAX(sequence),0)+1
          FROM vendor_production_batch_reservations
         WHERE business_id=$1
           AND batch_id=$2
        `,
		batch.BusinessID,
		batch.ID,
	).Scan(&sequence)
	if err != nil {
		cleanup()
		return BatchDetail{}, err
	}

	_, err = tx.Exec(
		ctx,
		`
        INSERT INTO vendor_production_batch_reservations(
            id,
            business_id,
            branch_id,
            batch_id,
            reservation_kind,
            sequence,
            inventory_reservation_id,
            source_ref,
            status
        )
        VALUES(
            $1,$2,$3,$4,'TOP_UP',$5,$6,$7,'CONFIRMED'
        )
        `,
		"vpbr_"+uuid.NewString(),
		batch.BusinessID,
		batch.BranchID,
		batch.ID,
		sequence,
		reservation.ID,
		reservation.SourceRef,
	)
	if err != nil {
		cleanup()
		return BatchDetail{}, err
	}

	for _, line := range resolved {
		table := "vendor_production_batch_components"
		if line.kind == MaterialPackaging {
			table = "vendor_production_batch_packaging"
		}

		_, err = tx.Exec(
			ctx,
			`
            UPDATE `+table+`
               SET reserved_quantity=reserved_quantity+$4,
                   updated_at=now()
             WHERE id=$1
               AND business_id=$2
               AND batch_id=$3
            `,
			line.targetID,
			batch.BusinessID,
			batch.ID,
			line.quantity,
		)
		if err != nil {
			cleanup()
			return BatchDetail{}, err
		}
	}

	batch, err = scanBatch(
		tx.QueryRow(
			ctx,
			`
            UPDATE vendor_production_batches
               SET version=version+1,
                   updated_at=now()
             WHERE id=$1
               AND business_id=$2
               AND version=$3
             RETURNING `+batchColumns,
			batch.ID,
			batch.BusinessID,
			expectedVersion,
		),
	)
	if err != nil {
		cleanup()
		return BatchDetail{}, err
	}

	if err = s.production.RecordMutation(
		ctx,
		tx,
		actor,
		"vendor.production_batch.inventory_topped_up",
		"vendor.production_batch.inventory_topped_up",
		"vendor_production_batch",
		batch.ID,
		batch.BranchID,
		batch.Version,
		nil,
		map[string]any{
			"reservation_id": reservation.ID,
			"line_count":     len(resolved),
		},
		map[string]any{"batch_id": batch.ID},
		"",
	); err != nil {
		cleanup()
		return BatchDetail{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		cleanup()
		return BatchDetail{}, err
	}

	return s.GetBatchDetail(ctx, actor, batch.ID)
}

func (s *Service) reservationIDsQ(
	ctx context.Context,
	q interface {
		Query(context.Context, string, ...any) (pgx.Rows, error)
	},
	businessID string,
	batchID string,
) ([]string, error) {
	rows, err := q.Query(
		ctx,
		`
        SELECT inventory_reservation_id
          FROM vendor_production_batch_reservations
         WHERE business_id=$1
           AND batch_id=$2
           AND status IN ('HELD','CONFIRMED')
         ORDER BY sequence,id
        `,
		businessID,
		batchID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]string, 0)
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}

	return out, rows.Err()
}

func (s *Service) CompleteBatch(
	ctx context.Context,
	actor production.Actor,
	batchID string,
	input CompleteInput,
	expectedVersion int64,
) (BatchDetail, error) {
	actualOutput, err := normalizeFraction(
		input.ActualOutputNumerator,
		input.ActualOutputDenominator,
		false,
	)
	if err != nil {
		return BatchDetail{}, ErrInvalidInput
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return BatchDetail{}, err
	}
	defer tx.Rollback(ctx)

	batch, err := s.loadBatchQ(
		ctx,
		tx,
		actor.VendorID,
		batchID,
		true,
	)
	if err != nil {
		return BatchDetail{}, err
	}

	if batch.Status != StatusInProgress {
		return BatchDetail{}, ErrStateConflict
	}
	if batch.Version != expectedVersion {
		return BatchDetail{}, ErrVersionConflict
	}

	actualByItem := map[string]inventory.ProductionActualLine{}

	rows, err := tx.Query(
		ctx,
		`
        SELECT
            inventory_controlled,
            optional,
            COALESCE(inventory_item_id,''),
            reserved_quantity,
            actual_used_numerator,
            actual_used_denominator,
            actual_waste_numerator,
            actual_waste_denominator,
            actual_recorded
          FROM vendor_production_batch_components
         WHERE business_id=$1
           AND batch_id=$2
         ORDER BY inventory_item_id,id
        `,
		batch.BusinessID,
		batch.ID,
	)
	if err != nil {
		return BatchDetail{}, err
	}

	for rows.Next() {
		var controlled bool
		var optional bool
		var inventoryID string
		var reserved int64
		var usedN, usedD int64
		var wasteN, wasteD int64
		var recorded bool

		if err = rows.Scan(
			&controlled,
			&optional,
			&inventoryID,
			&reserved,
			&usedN,
			&usedD,
			&wasteN,
			&wasteD,
			&recorded,
		); err != nil {
			rows.Close()
			return BatchDetail{}, err
		}

		if !controlled {
			continue
		}

		if !optional && !recorded {
			rows.Close()
			return BatchDetail{}, ErrStateConflict
		}

		if !recorded {
			continue
		}

		if usedD != 1 || wasteD != 1 {
			rows.Close()
			return BatchDetail{}, ErrInventoryRequired
		}

		if usedN+wasteN > reserved {
			rows.Close()
			return BatchDetail{}, ErrTopUpRequired
		}

		current := actualByItem[inventoryID]
		current.InventoryItemID = inventoryID
		current.UsedQuantity += usedN
		current.WasteQuantity += wasteN
		actualByItem[inventoryID] = current
	}

	if rows.Err() != nil {
		rows.Close()
		return BatchDetail{}, rows.Err()
	}
	rows.Close()

	rows, err = tx.Query(
		ctx,
		`
        SELECT
            inventory_controlled,
            production_consumption_point,
            COALESCE(inventory_item_id,''),
            reserved_quantity,
            actual_used_numerator,
            actual_used_denominator,
            actual_waste_numerator,
            actual_waste_denominator,
            actual_recorded
          FROM vendor_production_batch_packaging
         WHERE business_id=$1
           AND batch_id=$2
         ORDER BY inventory_item_id,id
        `,
		batch.BusinessID,
		batch.ID,
	)
	if err != nil {
		return BatchDetail{}, err
	}

	for rows.Next() {
		var controlled bool
		var point string
		var inventoryID string
		var reserved int64
		var usedN, usedD int64
		var wasteN, wasteD int64
		var recorded bool

		if err = rows.Scan(
			&controlled,
			&point,
			&inventoryID,
			&reserved,
			&usedN,
			&usedD,
			&wasteN,
			&wasteD,
			&recorded,
		); err != nil {
			rows.Close()
			return BatchDetail{}, err
		}

		if !controlled || point != "PRODUCTION" {
			continue
		}

		if !recorded {
			rows.Close()
			return BatchDetail{}, ErrStateConflict
		}

		if usedD != 1 || wasteD != 1 {
			rows.Close()
			return BatchDetail{}, ErrInventoryRequired
		}

		if usedN+wasteN > reserved {
			rows.Close()
			return BatchDetail{}, ErrTopUpRequired
		}

		current := actualByItem[inventoryID]
		current.InventoryItemID = inventoryID
		current.UsedQuantity += usedN
		current.WasteQuantity += wasteN
		actualByItem[inventoryID] = current
	}

	if rows.Err() != nil {
		rows.Close()
		return BatchDetail{}, rows.Err()
	}
	rows.Close()

	actualLines := make([]inventory.ProductionActualLine, 0, len(actualByItem))
	for _, line := range actualByItem {
		actualLines = append(actualLines, line)
	}
	sort.Slice(actualLines, func(i, j int) bool {
		return actualLines[i].InventoryItemID <
			actualLines[j].InventoryItemID
	})

	reservationIDs, err := s.reservationIDsQ(
		ctx,
		tx,
		batch.BusinessID,
		batch.ID,
	)
	if err != nil {
		return BatchDetail{}, err
	}

	var output *inventory.ProductionOutputInput

	if batch.OperatingModel == OperatingBatchStock {
		outputFactor, factorErr := normalizeFraction(
			batch.OutputInventoryNumerator,
			batch.OutputInventoryDenominator,
			false,
		)
		if factorErr != nil {
			return BatchDetail{}, factorErr
		}

		output, factorErr = preparedOutputInput(batch.OperatingModel, actualOutput, outputFactor, batch.PreparedInventoryItemID)
		if factorErr != nil {
			return BatchDetail{}, factorErr
		}
	}

	var posting inventory.ProductionPostingResult

	if len(reservationIDs) > 0 || output != nil {
		invScope, scopeErr := inventoryScope(
			batch.BusinessID,
			batch.BranchID,
		)
		if scopeErr != nil {
			return BatchDetail{}, scopeErr
		}

		posting, err = s.inventory.FinalizeProductionBatch(
			ctx,
			inventoryActor(actor),
			invScope,
			batch.BranchID,
			batch.ID,
			reservationIDs,
			actualLines,
			output,
		)
		if err != nil {
			return BatchDetail{}, err
		}

		for _, split := range posting.Splits {
			_, err = tx.Exec(
				ctx,
				`
                INSERT INTO vendor_production_batch_inventory_splits(
                    id,
                    business_id,
                    branch_id,
                    batch_id,
                    inventory_reservation_id,
                    inventory_item_id,
                    lot_id,
                    reserved_quantity,
                    used_quantity,
                    waste_quantity,
                    released_quantity
                )
                VALUES(
                    $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11
                )
                ON CONFLICT(
                    business_id,
                    batch_id,
                    inventory_reservation_id,
                    inventory_item_id,
                    lot_id
                )
                DO NOTHING
                `,
				"vpbis_"+uuid.NewString(),
				batch.BusinessID,
				batch.BranchID,
				batch.ID,
				split.ReservationID,
				split.InventoryItemID,
				split.LotID,
				split.ReservedQuantity,
				split.UsedQuantity,
				split.WasteQuantity,
				split.ReleasedQuantity,
			)
			if err != nil {
				return BatchDetail{}, err
			}
		}

		if posting.OutputInventoryItemID != "" {
			_, err = tx.Exec(
				ctx,
				`
                INSERT INTO vendor_production_batch_outputs(
                    id,
                    business_id,
                    branch_id,
                    batch_id,
                    inventory_item_id,
                    quantity,
                    inventory_source_ref,
                    inventory_lot_id
                )
                VALUES(
                    $1,$2,$3,$4,$5,$6,$7,NULLIF($8,'')
                )
                ON CONFLICT(business_id,batch_id)
                DO NOTHING
                `,
				"vpbo_"+uuid.NewString(),
				batch.BusinessID,
				batch.BranchID,
				batch.ID,
				posting.OutputInventoryItemID,
				posting.OutputQuantity,
				batch.ID,
				posting.OutputLotID,
			)
			if err != nil {
				return BatchDetail{}, err
			}
		}

		_, err = tx.Exec(
			ctx,
			`
            UPDATE vendor_production_batch_reservations
               SET status=CASE
                       WHEN inventory_reservation_id = ANY($3)
                       THEN 'COMMITTED'
                       ELSE status
                   END,
                   updated_at=now()
             WHERE business_id=$1
               AND batch_id=$2
            `,
			batch.BusinessID,
			batch.ID,
			reservationIDs,
		)
		if err != nil {
			return BatchDetail{}, err
		}
	}

	batch, err = scanBatch(
		tx.QueryRow(
			ctx,
			`
            UPDATE vendor_production_batches
               SET status='COMPLETED',
                   actual_output_numerator=$4,
                   actual_output_denominator=$5,
                   completed_by_staff_id=NULLIF($6,''),
                   completed_at=now(),
                   version=version+1,
                   updated_at=now()
             WHERE id=$1
               AND business_id=$2
               AND version=$3
               AND status='IN_PROGRESS'
             RETURNING `+batchColumns,
			batch.ID,
			batch.BusinessID,
			expectedVersion,
			actualOutput.N,
			actualOutput.D,
			actor.StaffID,
		),
	)
	if err != nil {
		return BatchDetail{}, err
	}

	if err = s.production.RecordMutation(
		ctx,
		tx,
		actor,
		"vendor.production_batch.completed",
		"vendor.production_batch.completed",
		"vendor_production_batch",
		batch.ID,
		batch.BranchID,
		batch.Version,
		nil,
		batch,
		map[string]any{
			"batch_id":         batch.ID,
			"inventory_posted": len(reservationIDs) > 0 || output != nil,
			"prepared_output":  posting.OutputQuantity,
		},
		"",
	); err != nil {
		return BatchDetail{}, err
	}

	if posting.OutputInventoryItemID != "" {
		if err = s.production.RecordMutation(
			ctx,
			tx,
			actor,
			"vendor.production.prepared_stock.created",
			"vendor.production.prepared_stock.created",
			"vendor_production_batch",
			batch.ID,
			batch.BranchID,
			batch.Version,
			nil,
			map[string]any{
				"inventory_item_id": posting.OutputInventoryItemID,
				"quantity":          posting.OutputQuantity,
				"inventory_lot_id":  posting.OutputLotID,
			},
			map[string]any{"batch_id": batch.ID},
			"",
		); err != nil {
			return BatchDetail{}, err
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return BatchDetail{}, err
	}

	return s.GetBatchDetail(ctx, actor, batch.ID)
}

func (s *Service) CancelBatch(
	ctx context.Context,
	actor production.Actor,
	batchID string,
	input CancelInput,
	expectedVersion int64,
) (Batch, error) {
	input.Reason = strings.TrimSpace(input.Reason)
	if input.Reason == "" || len(input.Reason) > 1000 {
		return Batch{}, ErrInvalidInput
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Batch{}, err
	}
	defer tx.Rollback(ctx)

	batch, err := s.loadBatchQ(
		ctx,
		tx,
		actor.VendorID,
		batchID,
		true,
	)
	if err != nil {
		return Batch{}, err
	}

	if batch.Version != expectedVersion {
		return Batch{}, ErrVersionConflict
	}

	switch batch.Status {
	case StatusDraft, StatusPlanned, StatusInProgress:
	default:
		return Batch{}, ErrStateConflict
	}

	var alreadyPosted bool

	err = tx.QueryRow(
		ctx,
		`
        SELECT EXISTS(
            SELECT 1
              FROM inventory_production_postings
             WHERE business_id=$1
               AND batch_ref=$2
        )
        `,
		batch.BusinessID,
		batch.ID,
	).Scan(&alreadyPosted)
	if err != nil {
		return Batch{}, err
	}

	if alreadyPosted {
		return Batch{}, ErrCorrectionRequired
	}

	reservationIDs, err := s.reservationIDsQ(
		ctx,
		tx,
		batch.BusinessID,
		batch.ID,
	)
	if err != nil {
		return Batch{}, err
	}

	if len(reservationIDs) > 0 {
		scope, scopeErr := inventoryScope(
			batch.BusinessID,
			batch.BranchID,
		)
		if scopeErr != nil {
			return Batch{}, scopeErr
		}

		for _, reservationID := range reservationIDs {
			current, getErr := s.inventory.GetReservation(
				ctx,
				inventoryActor(actor),
				scope,
				reservationID,
			)
			if getErr != nil {
				return Batch{}, getErr
			}

			switch current.Status {
			case inventory.ReservationHeld,
				inventory.ReservationConfirmed:

				if _, getErr = s.inventory.ReleaseReservation(
					ctx,
					inventoryActor(actor),
					scope,
					reservationID,
				); getErr != nil {
					return Batch{}, getErr
				}

			case inventory.ReservationReleased,
				inventory.ReservationCancelled:
				// Idempotent safe terminal state.

			case inventory.ReservationCommitted:
				return Batch{}, ErrCorrectionRequired

			default:
				return Batch{}, ErrStateConflict
			}
		}

		_, err = tx.Exec(
			ctx,
			`
            UPDATE vendor_production_batch_reservations
               SET status='RELEASED',
                   updated_at=now()
             WHERE business_id=$1
               AND batch_id=$2
               AND status IN ('HELD','CONFIRMED')
            `,
			batch.BusinessID,
			batch.ID,
		)
		if err != nil {
			return Batch{}, err
		}
	}

	before := batch

	batch, err = scanBatch(
		tx.QueryRow(
			ctx,
			`
            UPDATE vendor_production_batches
               SET status='CANCELLED',
                   cancelled_by_staff_id=NULLIF($4,''),
                   cancelled_at=now(),
                   cancel_reason=$5,
                   version=version+1,
                   updated_at=now()
             WHERE id=$1
               AND business_id=$2
               AND version=$3
               AND status IN ('DRAFT','PLANNED','IN_PROGRESS')
             RETURNING `+batchColumns,
			batch.ID,
			batch.BusinessID,
			expectedVersion,
			actor.StaffID,
			input.Reason,
		),
	)
	if err != nil {
		return Batch{}, err
	}

	if err = s.production.RecordMutation(
		ctx,
		tx,
		actor,
		"vendor.production_batch.cancelled",
		"vendor.production_batch.cancelled",
		"vendor_production_batch",
		batch.ID,
		batch.BranchID,
		batch.Version,
		before,
		batch,
		map[string]any{
			"batch_id": batch.ID,
			"reason":   input.Reason,
		},
		input.Reason,
	); err != nil {
		return Batch{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return Batch{}, err
	}

	return batch, nil
}

func (s *Service) ListReservations(
	ctx context.Context,
	actor production.Actor,
	batchID string,
) ([]ReservationView, error) {
	if _, err := s.GetBatch(ctx, actor, batchID); err != nil {
		return nil, err
	}

	rows, err := s.db.Query(
		ctx,
		`
        SELECT
            id,
            reservation_kind,
            sequence,
            inventory_reservation_id,
            source_ref,
            status,
            created_at
          FROM vendor_production_batch_reservations
         WHERE business_id=$1
           AND batch_id=$2
         ORDER BY sequence,id
        `,
		actor.VendorID,
		batchID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ReservationView, 0)

	for rows.Next() {
		var item ReservationView
		if err = rows.Scan(
			&item.ID,
			&item.Kind,
			&item.Sequence,
			&item.InventoryReservationID,
			&item.SourceRef,
			&item.Status,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}

	return out, rows.Err()
}

func (s *Service) GetOutput(
	ctx context.Context,
	actor production.Actor,
	batchID string,
) (OutputView, error) {
	if _, err := s.GetBatch(ctx, actor, batchID); err != nil {
		return OutputView{}, err
	}

	var out OutputView

	err := s.db.QueryRow(
		ctx,
		`
        SELECT
            id,
            inventory_item_id,
            quantity,
            inventory_source_ref,
            COALESCE(inventory_lot_id,''),
            created_at
          FROM vendor_production_batch_outputs
         WHERE business_id=$1
           AND batch_id=$2
        `,
		actor.VendorID,
		batchID,
	).Scan(
		&out.ID,
		&out.InventoryItemID,
		&out.Quantity,
		&out.InventorySourceRef,
		&out.InventoryLotID,
		&out.CreatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return OutputView{}, ErrNotFound
	}

	return out, err
}

func (s *Service) GetStockPolicy(
	ctx context.Context,
	actor production.Actor,
	branchID string,
	targetType string,
	targetRef string,
	targetVariantRef string,
) (StockPolicy, error) {
	return s.loadPolicyQ(
		ctx,
		s.db,
		actor.VendorID,
		strings.TrimSpace(branchID),
		strings.ToUpper(strings.TrimSpace(targetType)),
		strings.TrimSpace(targetRef),
		strings.TrimSpace(targetVariantRef),
	)
}

func (s *Service) PutStockPolicy(
	ctx context.Context,
	actor production.Actor,
	branchID string,
	input PutStockPolicyInput,
	expectedVersion int64,
) (StockPolicy, error) {
	branchID = strings.TrimSpace(branchID)
	input.TargetType = strings.ToUpper(strings.TrimSpace(input.TargetType))
	input.TargetRef = strings.TrimSpace(input.TargetRef)
	input.TargetVariantRef = strings.TrimSpace(input.TargetVariantRef)
	input.OperatingModel = strings.ToUpper(strings.TrimSpace(input.OperatingModel))
	input.PreparedInventoryItemID = strings.TrimSpace(input.PreparedInventoryItemID)

	if branchID == "" ||
		input.TargetRef == "" ||
		(input.TargetType != "FOOD_ITEM" &&
			input.TargetType != "PASTRY_PRODUCT" &&
			input.TargetType != "CLOUD_KITCHEN_PACKAGE") ||
		(input.OperatingModel != OperatingOnDemand &&
			input.OperatingModel != OperatingBatchStock) {
		return StockPolicy{}, ErrInvalidInput
	}

	conversion, err := normalizeFraction(
		input.OutputInventoryNumerator,
		input.OutputInventoryDenominator,
		false,
	)
	if err != nil {
		return StockPolicy{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return StockPolicy{}, err
	}
	defer tx.Rollback(ctx)

	if input.OperatingModel == OperatingBatchStock {
		if input.PreparedInventoryItemID == "" {
			return StockPolicy{}, ErrInventoryRequired
		}

		var itemType string
		var status string

		err = tx.QueryRow(
			ctx,
			`
            SELECT item_type,status
              FROM inventory_items
             WHERE id=$1
               AND business_id=$2
               AND branch_id=$3
            `,
			input.PreparedInventoryItemID,
			actor.VendorID,
			branchID,
		).Scan(
			&itemType,
			&status,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return StockPolicy{}, ErrInventoryRequired
		}
		if err != nil {
			return StockPolicy{}, err
		}

		if itemType != inventory.ItemTypeProductionComponent ||
			status != inventory.ItemStatusActive {
			return StockPolicy{}, ErrInventoryRequired
		}
	} else {
		input.PreparedInventoryItemID = ""
	}

	var existing StockPolicy
	existing, err = s.loadPolicyQ(
		ctx,
		tx,
		actor.VendorID,
		branchID,
		input.TargetType,
		input.TargetRef,
		input.TargetVariantRef,
	)
	if err != nil {
		return StockPolicy{}, err
	}

	var out StockPolicy

	if existing.ID == "" {
		if expectedVersion != 0 {
			return StockPolicy{}, ErrVersionConflict
		}

		err = tx.QueryRow(
			ctx,
			`
            INSERT INTO vendor_production_stock_policies(
                id,
                business_id,
                branch_id,
                target_type,
                target_ref,
                target_variant_ref,
                operating_model,
                prepared_inventory_item_id,
                output_inventory_numerator,
                output_inventory_denominator
            )
            VALUES(
                $1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),$9,$10
            )
            RETURNING
                id,
                business_id,
                branch_id,
                target_type,
                target_ref,
                target_variant_ref,
                operating_model,
                COALESCE(prepared_inventory_item_id,''),
                output_inventory_numerator,
                output_inventory_denominator,
                status,
                version,
                created_at,
                updated_at
            `,
			"vpsp_"+uuid.NewString(),
			actor.VendorID,
			branchID,
			input.TargetType,
			input.TargetRef,
			input.TargetVariantRef,
			input.OperatingModel,
			input.PreparedInventoryItemID,
			conversion.N,
			conversion.D,
		).Scan(
			&out.ID,
			&out.BusinessID,
			&out.BranchID,
			&out.TargetType,
			&out.TargetRef,
			&out.TargetVariantRef,
			&out.OperatingModel,
			&out.PreparedInventoryItemID,
			&out.OutputInventoryNumerator,
			&out.OutputInventoryDenominator,
			&out.Status,
			&out.Version,
			&out.CreatedAt,
			&out.UpdatedAt,
		)
	} else {
		if existing.Version != expectedVersion {
			return StockPolicy{}, ErrVersionConflict
		}

		err = tx.QueryRow(
			ctx,
			`
            UPDATE vendor_production_stock_policies
               SET operating_model=$4,
                   prepared_inventory_item_id=NULLIF($5,''),
                   output_inventory_numerator=$6,
                   output_inventory_denominator=$7,
                   version=version+1,
                   updated_at=now()
             WHERE id=$1
               AND business_id=$2
               AND version=$3
             RETURNING
                id,
                business_id,
                branch_id,
                target_type,
                target_ref,
                target_variant_ref,
                operating_model,
                COALESCE(prepared_inventory_item_id,''),
                output_inventory_numerator,
                output_inventory_denominator,
                status,
                version,
                created_at,
                updated_at
            `,
			existing.ID,
			actor.VendorID,
			expectedVersion,
			input.OperatingModel,
			input.PreparedInventoryItemID,
			conversion.N,
			conversion.D,
		).Scan(
			&out.ID,
			&out.BusinessID,
			&out.BranchID,
			&out.TargetType,
			&out.TargetRef,
			&out.TargetVariantRef,
			&out.OperatingModel,
			&out.PreparedInventoryItemID,
			&out.OutputInventoryNumerator,
			&out.OutputInventoryDenominator,
			&out.Status,
			&out.Version,
			&out.CreatedAt,
			&out.UpdatedAt,
		)
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return StockPolicy{}, ErrVersionConflict
	}
	if err != nil {
		return StockPolicy{}, err
	}

	if err = s.production.RecordMutation(
		ctx,
		tx,
		actor,
		"vendor.production.stock_policy.changed",
		"vendor.production.stock_policy.changed",
		"vendor_production_stock_policy",
		out.ID,
		out.BranchID,
		out.Version,
		existing,
		out,
		out,
		"",
	); err != nil {
		return StockPolicy{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return StockPolicy{}, err
	}

	return out, nil
}

func (s *Service) DebugBoundary() string {
	return fmt.Sprintf(
		"productionbatch:capacity=%t inventory=%t",
		s.production != nil,
		s.inventory != nil,
	)
}
