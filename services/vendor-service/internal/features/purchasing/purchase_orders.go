package purchasing

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

type purchaseQuerier interface {
	QueryRow(
		context.Context,
		string,
		...any,
	) pgx.Row

	Query(
		context.Context,
		string,
		...any,
	) (
		pgx.Rows,
		error,
	)
}

const purchaseOrderColumns = `
    id,
    business_id,
    branch_id,
    supplier_id,
    purchase_order_number,
    supplier_code_snapshot,
    supplier_name_snapshot,
    supplier_legal_name_snapshot,
    payment_terms_days_snapshot,
    delivery_terms_snapshot,
    currency_code,
    subtotal_minor,
    total_amount_minor,
    status,
    notes,
    expected_delivery_date,
    created_by_staff_id,
    submitted_by_staff_id,
    ordered_by_staff_id,
    cancelled_by_staff_id,
    closed_by_staff_id,
    approval_policy_id,
    approval_policy_version,
    approval_rule_id,
    approval_rule_version,
    required_approval_count,
    approval_permission_code,
    approval_scope,
    self_approval_allowed,
    approval_step_up_required,
    approval_generation,
    submitted_at,
    approved_at,
    ordered_at,
    received_at,
    closed_at,
    rejected_at,
    cancelled_at,
    version,
    created_at,
    updated_at
`

func scanPurchaseOrder(
	row rowScanner,
) (
	PurchaseOrder,
	error,
) {
	var out PurchaseOrder

	err :=
		row.Scan(
			&out.ID,
			&out.BusinessID,
			&out.BranchID,
			&out.SupplierID,
			&out.PurchaseOrderNumber,
			&out.SupplierCodeSnapshot,
			&out.SupplierNameSnapshot,
			&out.SupplierLegalNameSnapshot,
			&out.PaymentTermsDaysSnapshot,
			&out.DeliveryTermsSnapshot,
			&out.CurrencyCode,
			&out.SubtotalMinor,
			&out.TotalAmountMinor,
			&out.Status,
			&out.Notes,
			&out.ExpectedDeliveryDate,
			&out.CreatedByStaffID,
			&out.SubmittedByStaffID,
			&out.OrderedByStaffID,
			&out.CancelledByStaffID,
			&out.ClosedByStaffID,
			&out.ApprovalPolicyID,
			&out.ApprovalPolicyVersion,
			&out.ApprovalRuleID,
			&out.ApprovalRuleVersion,
			&out.RequiredApprovalCount,
			&out.ApprovalPermissionCode,
			&out.ApprovalScope,
			&out.SelfApprovalAllowed,
			&out.ApprovalStepUpRequired,
			&out.ApprovalGeneration,
			&out.SubmittedAt,
			&out.ApprovedAt,
			&out.OrderedAt,
			&out.ReceivedAt,
			&out.ClosedAt,
			&out.RejectedAt,
			&out.CancelledAt,
			&out.Version,
			&out.CreatedAt,
			&out.UpdatedAt,
		)

	return out, err
}

const purchaseOrderLineColumns = `
    id,
    business_id,
    purchase_order_id,
    supplier_product_id,
    supplier_product_version,
    inventory_item_id,
    supplier_sku_snapshot,
    purchase_unit_code_snapshot,
    purchase_unit_name_snapshot,
    conversion_mode_snapshot,
    inventory_units_per_purchase_unit_snapshot,
    inventory_measure_ref_snapshot,
    inventory_measure_owner_key_snapshot,
    inventory_unit_kind_snapshot,
    purchase_quantity,
    converted_inventory_quantity,
    unit_cost_minor,
    line_total_minor,
    currency_code_snapshot,
    received_purchase_quantity,
    version,
    created_at,
    updated_at
`

func scanPurchaseOrderLine(
	row rowScanner,
) (
	PurchaseOrderLine,
	error,
) {
	var out PurchaseOrderLine

	err :=
		row.Scan(
			&out.ID,
			&out.BusinessID,
			&out.PurchaseOrderID,
			&out.SupplierProductID,
			&out.SupplierProductVersion,
			&out.InventoryItemID,
			&out.SupplierSKUSnapshot,
			&out.PurchaseUnitCodeSnapshot,
			&out.PurchaseUnitNameSnapshot,
			&out.ConversionModeSnapshot,
			&out.InventoryUnitsPerPurchaseUnitSnapshot,
			&out.InventoryMeasureRefSnapshot,
			&out.InventoryMeasureOwnerKeySnapshot,
			&out.InventoryUnitKindSnapshot,
			&out.PurchaseQuantity,
			&out.ConvertedInventoryQuantity,
			&out.UnitCostMinor,
			&out.LineTotalMinor,
			&out.CurrencyCodeSnapshot,
			&out.ReceivedPurchaseQuantity,
			&out.Version,
			&out.CreatedAt,
			&out.UpdatedAt,
		)

	return out, err
}

const purchaseApprovalColumns = `
    id,
    business_id,
    purchase_order_id,
    approval_generation,
    approver_staff_id,
    decision,
    permission_code_snapshot,
    approval_scope_snapshot,
    amount_minor_snapshot,
    purchase_order_version_before,
    step_up_consumed,
    note,
    created_at
`

func scanPurchaseApproval(
	row rowScanner,
) (
	PurchaseOrderApproval,
	error,
) {
	var out PurchaseOrderApproval

	err :=
		row.Scan(
			&out.ID,
			&out.BusinessID,
			&out.PurchaseOrderID,
			&out.ApprovalGeneration,
			&out.ApproverStaffID,
			&out.Decision,
			&out.PermissionCodeSnapshot,
			&out.ApprovalScopeSnapshot,
			&out.AmountMinorSnapshot,
			&out.PurchaseOrderVersionBefore,
			&out.StepUpConsumed,
			&out.Note,
			&out.CreatedAt,
		)

	return out, err
}

func (s *PurchaseOrderService) loadPurchaseOrder(
	ctx context.Context,
	q purchaseQuerier,
	businessID string,
	purchaseOrderID string,
	lock bool,
) (
	PurchaseOrder,
	error,
) {
	query :=
		`SELECT ` +
			purchaseOrderColumns +
			`
           FROM vendor_purchase_orders
          WHERE business_id=$1
            AND id=$2`

	if lock {
		query += ` FOR UPDATE`
	}

	out, err :=
		scanPurchaseOrder(
			q.QueryRow(
				ctx,
				query,
				strings.TrimSpace(
					businessID,
				),
				strings.TrimSpace(
					purchaseOrderID,
				),
			),
		)

	if errors.Is(
		err,
		pgx.ErrNoRows,
	) {
		return PurchaseOrder{},
			notFound(
				"Purchase order",
			)
	}

	return out, err
}

func (s *PurchaseOrderService) hydratePurchaseOrder(
	ctx context.Context,
	q purchaseQuerier,
	order PurchaseOrder,
) (
	PurchaseOrder,
	error,
) {
	rows, err :=
		q.Query(
			ctx,
			`SELECT `+
				purchaseOrderLineColumns+
				`
               FROM vendor_purchase_order_lines
              WHERE business_id=$1
                AND purchase_order_id=$2
              ORDER BY created_at,id`,
			order.BusinessID,
			order.ID,
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	defer rows.Close()

	order.Lines =
		make(
			[]PurchaseOrderLine,
			0,
		)

	for rows.Next() {
		line, scanErr :=
			scanPurchaseOrderLine(
				rows,
			)

		if scanErr != nil {
			return PurchaseOrder{},
				scanErr
		}

		order.Lines =
			append(
				order.Lines,
				line,
			)
	}

	if err = rows.Err(); err != nil {
		return PurchaseOrder{}, err
	}

	if err =
		q.QueryRow(
			ctx,
			`SELECT count(*)
               FROM vendor_purchase_order_approvals
              WHERE business_id=$1
                AND purchase_order_id=$2
                AND approval_generation=$3
                AND decision='APPROVED'`,
			order.BusinessID,
			order.ID,
			order.ApprovalGeneration,
		).Scan(
			&order.ApprovalCount,
		); err != nil {

		return PurchaseOrder{}, err
	}

	return order, nil
}

func (s *PurchaseOrderService) appendStatusHistory(
	ctx context.Context,
	tx pgx.Tx,
	actor Actor,
	order PurchaseOrder,
	fromStatus string,
	toStatus string,
	reason string,
) error {
	if fromStatus == toStatus {
		return nil
	}

	_, err :=
		tx.Exec(
			ctx,
			`INSERT INTO vendor_purchase_order_status_history(
                id,
                business_id,
                purchase_order_id,
                from_status,
                to_status,
                actor_ref,
                purchase_order_version,
                reason
            )
            VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
			"vpoh_"+
				uuid.NewString(),
			order.BusinessID,
			order.ID,
			fromStatus,
			toStatus,
			actor.StaffID,
			order.Version,
			strings.TrimSpace(
				reason,
			),
		)

	return err
}

func purchaseOrderCommandHash(
	value any,
) (
	string,
	error,
) {
	raw, err :=
		json.Marshal(
			value,
		)

	if err != nil {
		return "", err
	}

	sum :=
		sha256.Sum256(
			raw,
		)

	return hex.EncodeToString(
		sum[:],
	), nil
}

func (s *PurchaseOrderService) replayPurchaseOrderCommand(
	ctx context.Context,
	tx pgx.Tx,
	actor Actor,
	purchaseOrderID string,
	commandType string,
	requestHash string,
) (
	PurchaseOrder,
	bool,
	error,
) {
	commandRef :=
		strings.TrimSpace(
			actor.CommandRef,
		)

	if commandRef == "" {
		return PurchaseOrder{},
			false,
			apperrors.BadRequest(
				"Idempotency-Key is required for this command.",
				nil,
			)
	}

	var (
		existingOrderID string
		existingType    string
		existingHash    string
		snapshot        []byte
	)

	err :=
		tx.QueryRow(
			ctx,
			`SELECT
                purchase_order_id,
                command_type,
                request_hash,
                result_snapshot
               FROM vendor_purchase_order_commands
              WHERE business_id=$1
                AND command_ref=$2`,
			actor.VendorID,
			commandRef,
		).Scan(
			&existingOrderID,
			&existingType,
			&existingHash,
			&snapshot,
		)

	if errors.Is(
		err,
		pgx.ErrNoRows,
	) {
		return PurchaseOrder{},
			false,
			nil
	}

	if err != nil {
		return PurchaseOrder{},
			false,
			err
	}

	if existingOrderID !=
		strings.TrimSpace(
			purchaseOrderID,
		) ||
		existingType != commandType ||
		existingHash != requestHash {

		return PurchaseOrder{},
			false,
			apperrors.Conflict(
				"This idempotency key was already used for another purchase-order command.",
				nil,
			)
	}

	var out PurchaseOrder

	if err =
		json.Unmarshal(
			snapshot,
			&out,
		); err != nil {

		return PurchaseOrder{},
			false,
			err
	}

	return out,
		true,
		nil
}

func (s *PurchaseOrderService) insertPurchaseOrderCommand(
	ctx context.Context,
	tx pgx.Tx,
	actor Actor,
	order PurchaseOrder,
	commandType string,
	requestHash string,
) error {
	snapshot, err :=
		json.Marshal(
			order,
		)

	if err != nil {
		return err
	}

	_, err =
		tx.Exec(
			ctx,
			`INSERT INTO vendor_purchase_order_commands(
                id,
                business_id,
                command_ref,
                purchase_order_id,
                command_type,
                request_hash,
                result_snapshot
            )
            VALUES($1,$2,$3,$4,$5,$6,$7)`,
			"vpocmd_"+
				uuid.NewString(),
			actor.VendorID,
			strings.TrimSpace(
				actor.CommandRef,
			),
			order.ID,
			commandType,
			requestHash,
			snapshot,
		)

	return mapWriteError(err)
}

type supplierSnapshot struct {
	Code      string
	Name      string
	LegalName string

	PaymentTermsDays int
	DeliveryTerms    string
	TermsCurrency    string
}

func (s *PurchaseOrderService) loadSupplierSnapshot(
	ctx context.Context,
	tx pgx.Tx,
	businessID string,
	supplierID string,
) (
	supplierSnapshot,
	error,
) {
	var out supplierSnapshot

	err :=
		tx.QueryRow(
			ctx,
			`SELECT
                s.code,
                s.name,
                s.legal_name,
                COALESCE(t.payment_terms_days,0),
                COALESCE(t.delivery_terms,''),
                COALESCE(t.currency_code,'')
               FROM vendor_suppliers s
               LEFT JOIN vendor_supplier_terms t
                 ON t.supplier_id=s.id
                AND t.business_id=s.business_id
              WHERE s.business_id=$1
                AND s.id=$2
                AND s.status='ACTIVE'
              FOR UPDATE OF s`,
			businessID,
			supplierID,
		).Scan(
			&out.Code,
			&out.Name,
			&out.LegalName,
			&out.PaymentTermsDays,
			&out.DeliveryTerms,
			&out.TermsCurrency,
		)

	if errors.Is(
		err,
		pgx.ErrNoRows,
	) {
		return supplierSnapshot{},
			notFound(
				"Supplier",
			)
	}

	return out, err
}

func multiplyMoney(
	quantity int64,
	unitCost int64,
) (
	int64,
	error,
) {
	if quantity <= 0 ||
		unitCost < 0 {

		return 0,
			validation(
				"lines",
				"Purchase quantity and cost are invalid.",
			)
	}

	if unitCost > 0 &&
		quantity >
			math.MaxInt64/unitCost {

		return 0,
			validation(
				"lines",
				"Purchase-order amount exceeds the supported range.",
			)
	}

	return quantity * unitCost,
		nil
}

func addMoney(
	total int64,
	line int64,
) (
	int64,
	error,
) {
	if line < 0 ||
		total >
			math.MaxInt64-line {

		return 0,
			validation(
				"lines",
				"Purchase-order total exceeds the supported range.",
			)
	}

	return total + line,
		nil
}

func (s *PurchaseOrderService) buildPurchaseOrderLines(
	ctx context.Context,
	tx pgx.Tx,
	businessID string,
	branchID string,
	supplierID string,
	currencyCode string,
	inputs []PurchaseOrderLineInput,
) (
	[]PurchaseOrderLine,
	int64,
	error,
) {
	if len(inputs) == 0 {
		return nil,
			0,
			validation(
				"lines",
				"Purchase order requires at least one line.",
			)
	}

	if len(inputs) > 500 {
		return nil,
			0,
			validation(
				"lines",
				"Purchase order has too many lines.",
			)
	}

	seen :=
		map[string]bool{}

	out :=
		make(
			[]PurchaseOrderLine,
			0,
			len(inputs),
		)

	var total int64

	for _, input := range inputs {
		productID :=
			strings.TrimSpace(
				input.SupplierProductID,
			)

		if productID == "" {
			return nil,
				0,
				validation(
					"lines.supplier_product_id",
					"Choose a SupplierProduct.",
				)
		}

		if seen[productID] {
			return nil,
				0,
				validation(
					"lines",
					"SupplierProduct may appear only once per purchase order.",
				)
		}

		seen[productID] = true

		if input.PurchaseQuantity <= 0 {
			return nil,
				0,
				validation(
					"lines.purchase_quantity",
					"Purchase quantity must be positive.",
				)
		}

		var (
			supplierSKU      string
			purchaseUnitCode string
			purchaseUnitName string
			conversionMode   string

			factor int64

			moq      int64
			multiple int64

			typical sql.NullInt64

			productCurrency string

			inventoryItemID string

			measureRef      string
			measureOwnerKey string
			unitKind        string

			productVersion int64
		)

		err :=
			tx.QueryRow(
				ctx,
				`SELECT
                    sp.supplier_sku,
                    sp.purchase_unit_code,
                    sp.purchase_unit_name,
                    sp.conversion_mode,
                    sp.inventory_units_per_purchase_unit,
                    sp.minimum_order_quantity,
                    sp.purchase_multiple,
                    sp.typical_cost_minor,
                    sp.currency_code,
                    sp.inventory_item_id,
                    sp.inventory_measure_ref,
                    sp.inventory_measure_owner_key,
                    sp.inventory_unit_kind,
                    sp.version
                   FROM vendor_supplier_products sp
                   JOIN inventory_items ii
                     ON ii.id=sp.inventory_item_id
                    AND ii.business_id=sp.business_id
                    AND ii.branch_id=sp.branch_id
                    AND ii.status='ACTIVE'
                  WHERE sp.id=$1
                    AND sp.business_id=$2
                    AND sp.supplier_id=$3
                    AND sp.branch_id=$4
                    AND sp.status='ACTIVE'
                  FOR UPDATE OF sp`,
				productID,
				businessID,
				supplierID,
				branchID,
			).Scan(
				&supplierSKU,
				&purchaseUnitCode,
				&purchaseUnitName,
				&conversionMode,
				&factor,
				&moq,
				&multiple,
				&typical,
				&productCurrency,
				&inventoryItemID,
				&measureRef,
				&measureOwnerKey,
				&unitKind,
				&productVersion,
			)

		if errors.Is(
			err,
			pgx.ErrNoRows,
		) {
			return nil,
				0,
				validation(
					"lines.supplier_product_id",
					"SupplierProduct must belong to this Supplier, Vendor and branch and map to an active Inventory item.",
				)
		}

		if err != nil {
			return nil, 0, err
		}

		if productCurrency != currencyCode {
			return nil,
				0,
				validation(
					"currency_code",
					"All SupplierProducts must use the purchase-order currency.",
				)
		}

		if input.PurchaseQuantity < moq {
			return nil,
				0,
				validation(
					"lines.purchase_quantity",
					"Purchase quantity is below the SupplierProduct MOQ.",
				)
		}

		if multiple <= 0 ||
			input.PurchaseQuantity%multiple != 0 {

			return nil,
				0,
				validation(
					"lines.purchase_quantity",
					"Purchase quantity must respect the SupplierProduct purchase multiple.",
				)
		}

		converted, err :=
			ConvertPurchaseQuantity(
				input.PurchaseQuantity,
				factor,
			)

		if err != nil {
			return nil, 0, err
		}

		var unitCost int64

		switch {
		case input.UnitCostMinor != nil:
			unitCost =
				*input.UnitCostMinor

		case typical.Valid:
			unitCost =
				typical.Int64

		default:
			return nil,
				0,
				validation(
					"lines.unit_cost_minor",
					"Provide a unit cost because this SupplierProduct has no typical cost.",
				)
		}

		if unitCost < 0 {
			return nil,
				0,
				validation(
					"lines.unit_cost_minor",
					"Unit cost cannot be negative.",
				)
		}

		lineTotal, err :=
			multiplyMoney(
				input.PurchaseQuantity,
				unitCost,
			)

		if err != nil {
			return nil, 0, err
		}

		total, err =
			addMoney(
				total,
				lineTotal,
			)

		if err != nil {
			return nil, 0, err
		}

		out =
			append(
				out,
				PurchaseOrderLine{
					ID: "vpol_" +
						uuid.NewString(),

					BusinessID: businessID,

					SupplierProductID: productID,

					SupplierProductVersion: productVersion,

					InventoryItemID: inventoryItemID,

					SupplierSKUSnapshot: supplierSKU,

					PurchaseUnitCodeSnapshot: purchaseUnitCode,

					PurchaseUnitNameSnapshot: purchaseUnitName,

					ConversionModeSnapshot: conversionMode,

					InventoryUnitsPerPurchaseUnitSnapshot: factor,

					InventoryMeasureRefSnapshot: measureRef,

					InventoryMeasureOwnerKeySnapshot: measureOwnerKey,

					InventoryUnitKindSnapshot: unitKind,

					PurchaseQuantity: input.PurchaseQuantity,

					ConvertedInventoryQuantity: converted,

					UnitCostMinor: unitCost,

					LineTotalMinor: lineTotal,

					CurrencyCodeSnapshot: productCurrency,
				},
			)
	}

	return out,
		total,
		nil
}

func insertPurchaseOrderLines(
	ctx context.Context,
	tx pgx.Tx,
	purchaseOrderID string,
	lines []PurchaseOrderLine,
) error {
	for index := range lines {
		line := &lines[index]

		line.PurchaseOrderID =
			purchaseOrderID

		_, err :=
			tx.Exec(
				ctx,
				`INSERT INTO vendor_purchase_order_lines(
                    id,
                    business_id,
                    purchase_order_id,
                    supplier_product_id,
                    supplier_product_version,
                    inventory_item_id,
                    supplier_sku_snapshot,
                    purchase_unit_code_snapshot,
                    purchase_unit_name_snapshot,
                    conversion_mode_snapshot,
                    inventory_units_per_purchase_unit_snapshot,
                    inventory_measure_ref_snapshot,
                    inventory_measure_owner_key_snapshot,
                    inventory_unit_kind_snapshot,
                    purchase_quantity,
                    converted_inventory_quantity,
                    unit_cost_minor,
                    line_total_minor,
                    currency_code_snapshot
                )
                VALUES(
                    $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,
                    $11,$12,$13,$14,$15,$16,$17,$18,$19
                )`,
				line.ID,
				line.BusinessID,
				line.PurchaseOrderID,
				line.SupplierProductID,
				line.SupplierProductVersion,
				line.InventoryItemID,
				line.SupplierSKUSnapshot,
				line.PurchaseUnitCodeSnapshot,
				line.PurchaseUnitNameSnapshot,
				line.ConversionModeSnapshot,
				line.InventoryUnitsPerPurchaseUnitSnapshot,
				line.InventoryMeasureRefSnapshot,
				line.InventoryMeasureOwnerKeySnapshot,
				line.InventoryUnitKindSnapshot,
				line.PurchaseQuantity,
				line.ConvertedInventoryQuantity,
				line.UnitCostMinor,
				line.LineTotalMinor,
				line.CurrencyCodeSnapshot,
			)

		if err != nil {
			return mapWriteError(err)
		}
	}

	return nil
}

func newPurchaseOrderNumber() string {
	raw :=
		strings.ToUpper(
			strings.ReplaceAll(
				uuid.NewString(),
				"-",
				"",
			),
		)

	return "PO-" +
		raw[:12]
}

func (s *PurchaseOrderService) CreatePurchaseOrder(
	ctx context.Context,
	actor Actor,
	input CreatePurchaseOrderInput,
) (
	PurchaseOrder,
	error,
) {
	scope, err :=
		s.writeScope(
			ctx,
			actor,
			PermissionManage,
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	input.BranchID =
		strings.TrimSpace(
			input.BranchID,
		)

	input.SupplierID =
		strings.TrimSpace(
			input.SupplierID,
		)

	input.Notes =
		strings.TrimSpace(
			input.Notes,
		)

	if input.BranchID == "" {
		return PurchaseOrder{},
			validation(
				"branch_id",
				"Choose a branch.",
			)
	}

	if input.SupplierID == "" {
		return PurchaseOrder{},
			validation(
				"supplier_id",
				"Choose a Supplier.",
			)
	}

	if len(input.Notes) > 8000 {
		return PurchaseOrder{},
			validation(
				"notes",
				"Purchase-order notes are too long.",
			)
	}

	if err =
		requireBranch(
			scope,
			input.BranchID,
		); err != nil {

		return PurchaseOrder{}, err
	}

	currency, err :=
		normalizedCurrency(
			input.CurrencyCode,
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	tx, err :=
		begin(
			ctx,
			s.base.db,
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	supplier, err :=
		s.loadSupplierSnapshot(
			ctx,
			tx,
			actor.VendorID,
			input.SupplierID,
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	if supplier.TermsCurrency != "" &&
		supplier.TermsCurrency != currency {

		return PurchaseOrder{},
			validation(
				"currency_code",
				"Purchase-order currency conflicts with the Supplier terms currency.",
			)
	}

	lines, total, err :=
		s.buildPurchaseOrderLines(
			ctx,
			tx,
			actor.VendorID,
			input.BranchID,
			input.SupplierID,
			currency,
			input.Lines,
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	order, err :=
		scanPurchaseOrder(
			tx.QueryRow(
				ctx,
				`INSERT INTO vendor_purchase_orders(
                    id,
                    business_id,
                    branch_id,
                    supplier_id,
                    purchase_order_number,
                    supplier_code_snapshot,
                    supplier_name_snapshot,
                    supplier_legal_name_snapshot,
                    payment_terms_days_snapshot,
                    delivery_terms_snapshot,
                    currency_code,
                    subtotal_minor,
                    total_amount_minor,
                    notes,
                    expected_delivery_date,
                    created_by_staff_id
                )
                VALUES(
                    $1,$2,$3,$4,$5,$6,$7,$8,
                    $9,$10,$11,$12,$12,$13,$14,$15
                )
                RETURNING `+
					purchaseOrderColumns,
				"vpo_"+
					uuid.NewString(),
				actor.VendorID,
				input.BranchID,
				input.SupplierID,
				newPurchaseOrderNumber(),
				supplier.Code,
				supplier.Name,
				supplier.LegalName,
				supplier.PaymentTermsDays,
				supplier.DeliveryTerms,
				currency,
				total,
				input.Notes,
				input.ExpectedDeliveryDate,
				actor.StaffID,
			),
		)

	if err != nil {
		return PurchaseOrder{},
			mapWriteError(err)
	}

	if err =
		insertPurchaseOrderLines(
			ctx,
			tx,
			order.ID,
			lines,
		); err != nil {

		return PurchaseOrder{}, err
	}

	order, err =
		s.hydratePurchaseOrder(
			ctx,
			tx,
			order,
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	if err =
		s.base.recordMutation(
			ctx,
			tx,
			actor,
			"purchasing.purchase_order.created",
			"vendor.purchasing.purchase_order.created",
			"vendor_purchase_order",
			order.ID,
			order.BranchID,
			order.Version,
			nil,
			order,
			map[string]any{
				"purchase_order_id":  order.ID,
				"branch_id":          order.BranchID,
				"supplier_id":        order.SupplierID,
				"status":             order.Status,
				"total_amount_minor": order.TotalAmountMinor,
				"currency_code":      order.CurrencyCode,
			},
		); err != nil {

		return PurchaseOrder{}, err
	}

	if err =
		tx.Commit(
			ctx,
		); err != nil {

		return PurchaseOrder{}, err
	}

	return order, nil
}

func (s *PurchaseOrderService) GetPurchaseOrder(
	ctx context.Context,
	actor Actor,
	purchaseOrderID string,
) (
	PurchaseOrder,
	error,
) {
	scope, err :=
		s.readScope(
			ctx,
			actor,
			PermissionRead,
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	order, err :=
		s.loadPurchaseOrder(
			ctx,
			s.base.db,
			actor.VendorID,
			purchaseOrderID,
			false,
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	if err =
		requireBranch(
			scope,
			order.BranchID,
		); err != nil {

		return PurchaseOrder{}, err
	}

	return s.hydratePurchaseOrder(
		ctx,
		s.base.db,
		order,
	)
}

func (s *PurchaseOrderService) ListPurchaseOrders(
	ctx context.Context,
	actor Actor,
	branchFilter string,
	status string,
	limit int,
	offset int,
) (
	[]PurchaseOrder,
	error,
) {
	scope, err :=
		s.readScope(
			ctx,
			actor,
			PermissionRead,
		)

	if err != nil {
		return nil, err
	}

	branchFilter =
		strings.TrimSpace(
			branchFilter,
		)

	status =
		strings.ToUpper(
			strings.TrimSpace(
				status,
			),
		)

	if branchFilter != "" &&
		!scope.Covers(
			branchFilter,
		) {

		return nil,
			apperrors.Forbidden(
				"Purchasing access is not permitted for this branch.",
				nil,
			)
	}

	limit, offset =
		normalizePage(
			limit,
			offset,
		)

	branches :=
		scope.BranchIDs

	if branches == nil {
		branches = []string{}
	}

	rows, err :=
		s.base.db.Query(
			ctx,
			`SELECT `+
				purchaseOrderColumns+
				`
               FROM vendor_purchase_orders
              WHERE business_id=$1
                AND (
                    $2::boolean
                    OR branch_id=ANY($3::text[])
                )
                AND (
                    $4::text=''
                    OR branch_id=$4
                )
                AND (
                    $5::text=''
                    OR status=$5
                )
              ORDER BY created_at DESC,id DESC
              LIMIT $6 OFFSET $7`,
			actor.VendorID,
			scope.Global,
			branches,
			branchFilter,
			status,
			limit,
			offset,
		)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	out :=
		make(
			[]PurchaseOrder,
			0,
		)

	for rows.Next() {
		order, scanErr :=
			scanPurchaseOrder(
				rows,
			)

		if scanErr != nil {
			return nil, scanErr
		}

		out =
			append(
				out,
				order,
			)
	}

	return out,
		rows.Err()
}

func (s *PurchaseOrderService) UpdatePurchaseOrder(
	ctx context.Context,
	actor Actor,
	purchaseOrderID string,
	input UpdatePurchaseOrderInput,
) (
	PurchaseOrder,
	error,
) {
	scope, err :=
		s.writeScope(
			ctx,
			actor,
			PermissionManage,
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	if input.ExpectedVersion <= 0 {
		return PurchaseOrder{},
			versionConflict()
	}

	input.Notes =
		strings.TrimSpace(
			input.Notes,
		)

	if len(input.Notes) > 8000 {
		return PurchaseOrder{},
			validation(
				"notes",
				"Purchase-order notes are too long.",
			)
	}

	tx, err :=
		begin(
			ctx,
			s.base.db,
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	current, err :=
		s.loadPurchaseOrder(
			ctx,
			tx,
			actor.VendorID,
			purchaseOrderID,
			true,
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	if err =
		requireBranch(
			scope,
			current.BranchID,
		); err != nil {

		return PurchaseOrder{}, err
	}

	if current.Version !=
		input.ExpectedVersion {

		return PurchaseOrder{},
			versionConflict()
	}

	if current.Status !=
		PurchaseOrderDraft {

		return PurchaseOrder{},
			apperrors.Conflict(
				"Only a DRAFT purchase order can change its header.",
				nil,
			)
	}

	order, err :=
		scanPurchaseOrder(
			tx.QueryRow(
				ctx,
				`UPDATE vendor_purchase_orders
                    SET notes=$3,
                        expected_delivery_date=$4,
                        version=version+1,
                        updated_at=now()
                  WHERE business_id=$1
                    AND id=$2
                  RETURNING `+
					purchaseOrderColumns,
				actor.VendorID,
				current.ID,
				input.Notes,
				input.ExpectedDeliveryDate,
			),
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	order, err =
		s.hydratePurchaseOrder(
			ctx,
			tx,
			order,
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	before, err :=
		s.hydratePurchaseOrder(
			ctx,
			tx,
			current,
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	if err =
		s.base.recordMutation(
			ctx,
			tx,
			actor,
			"purchasing.purchase_order.updated",
			"vendor.purchasing.purchase_order.updated",
			"vendor_purchase_order",
			order.ID,
			order.BranchID,
			order.Version,
			before,
			order,
			map[string]any{
				"purchase_order_id": order.ID,
				"status":            order.Status,
			},
		); err != nil {

		return PurchaseOrder{}, err
	}

	if err =
		tx.Commit(
			ctx,
		); err != nil {

		return PurchaseOrder{}, err
	}

	return order, nil
}

func (s *PurchaseOrderService) ReplacePurchaseOrderLines(
	ctx context.Context,
	actor Actor,
	purchaseOrderID string,
	input ReplacePurchaseOrderLinesInput,
) (
	PurchaseOrder,
	error,
) {
	scope, err :=
		s.writeScope(
			ctx,
			actor,
			PermissionManage,
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	if input.ExpectedVersion <= 0 {
		return PurchaseOrder{},
			versionConflict()
	}

	tx, err :=
		begin(
			ctx,
			s.base.db,
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	current, err :=
		s.loadPurchaseOrder(
			ctx,
			tx,
			actor.VendorID,
			purchaseOrderID,
			true,
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	if err =
		requireBranch(
			scope,
			current.BranchID,
		); err != nil {

		return PurchaseOrder{}, err
	}

	if current.Version !=
		input.ExpectedVersion {

		return PurchaseOrder{},
			versionConflict()
	}

	switch current.Status {
	case PurchaseOrderDraft,
		PurchaseOrderSubmitted,
		PurchaseOrderApproved:

	default:
		return PurchaseOrder{},
			apperrors.Conflict(
				"Purchase-order lines cannot change after the order has been placed.",
				nil,
			)
	}

	before, err :=
		s.hydratePurchaseOrder(
			ctx,
			tx,
			current,
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	lines, total, err :=
		s.buildPurchaseOrderLines(
			ctx,
			tx,
			current.BusinessID,
			current.BranchID,
			current.SupplierID,
			current.CurrencyCode,
			input.Lines,
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	if _, err =
		tx.Exec(
			ctx,
			`DELETE FROM vendor_purchase_order_lines
              WHERE business_id=$1
                AND purchase_order_id=$2`,
			current.BusinessID,
			current.ID,
		); err != nil {

		return PurchaseOrder{}, err
	}

	if err =
		insertPurchaseOrderLines(
			ctx,
			tx,
			current.ID,
			lines,
		); err != nil {

		return PurchaseOrder{}, err
	}

	nextStatus :=
		current.Status

	nextGeneration :=
		current.ApprovalGeneration

	resetApproval :=
		current.Status !=
			PurchaseOrderDraft

	if resetApproval {
		nextStatus =
			PurchaseOrderDraft

		nextGeneration++
	}

	order, err :=
		scanPurchaseOrder(
			tx.QueryRow(
				ctx,
				`UPDATE vendor_purchase_orders
                    SET subtotal_minor=$3,
                        total_amount_minor=$3,
                        status=$4,
                        approval_generation=$5,

                        approval_policy_id=
                            CASE WHEN $6
                                 THEN ''
                                 ELSE approval_policy_id
                            END,

                        approval_policy_version=
                            CASE WHEN $6
                                 THEN 0
                                 ELSE approval_policy_version
                            END,

                        approval_rule_id=
                            CASE WHEN $6
                                 THEN ''
                                 ELSE approval_rule_id
                            END,

                        approval_rule_version=
                            CASE WHEN $6
                                 THEN 0
                                 ELSE approval_rule_version
                            END,

                        required_approval_count=
                            CASE WHEN $6
                                 THEN 0
                                 ELSE required_approval_count
                            END,

                        approval_permission_code=
                            CASE WHEN $6
                                 THEN ''
                                 ELSE approval_permission_code
                            END,

                        approval_scope=
                            CASE WHEN $6
                                 THEN ''
                                 ELSE approval_scope
                            END,

                        self_approval_allowed=
                            CASE WHEN $6
                                 THEN false
                                 ELSE self_approval_allowed
                            END,

                        approval_step_up_required=
                            CASE WHEN $6
                                 THEN false
                                 ELSE approval_step_up_required
                            END,

                        submitted_by_staff_id=
                            CASE WHEN $6
                                 THEN ''
                                 ELSE submitted_by_staff_id
                            END,

                        submitted_at=
                            CASE WHEN $6
                                 THEN NULL
                                 ELSE submitted_at
                            END,

                        approved_at=
                            CASE WHEN $6
                                 THEN NULL
                                 ELSE approved_at
                            END,

                        rejected_at=NULL,

                        version=version+1,
                        updated_at=now()

                  WHERE business_id=$1
                    AND id=$2
                  RETURNING `+
					purchaseOrderColumns,
				current.BusinessID,
				current.ID,
				total,
				nextStatus,
				nextGeneration,
				resetApproval,
			),
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	order, err =
		s.hydratePurchaseOrder(
			ctx,
			tx,
			order,
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	if resetApproval {
		if err =
			s.appendStatusHistory(
				ctx,
				tx,
				actor,
				order,
				current.Status,
				PurchaseOrderDraft,
				"Purchase-order commercial content changed; prior approvals invalidated.",
			); err != nil {

			return PurchaseOrder{}, err
		}
	}

	if err =
		s.base.recordMutation(
			ctx,
			tx,
			actor,
			"purchasing.purchase_order.lines_replaced",
			"vendor.purchasing.purchase_order.changed",
			"vendor_purchase_order",
			order.ID,
			order.BranchID,
			order.Version,
			before,
			order,
			map[string]any{
				"purchase_order_id":    order.ID,
				"status":               order.Status,
				"total_amount_minor":   order.TotalAmountMinor,
				"approval_invalidated": resetApproval,
				"approval_generation":  order.ApprovalGeneration,
			},
		); err != nil {

		return PurchaseOrder{}, err
	}

	if err =
		tx.Commit(
			ctx,
		); err != nil {

		return PurchaseOrder{}, err
	}

	return order, nil
}
