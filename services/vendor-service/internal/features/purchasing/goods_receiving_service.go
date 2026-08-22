package purchasing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/features/inventory"
)

type GoodsReceivingEventPublisher interface {
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

type GoodsReceivingService struct {
	db     *pgxpool.Pool
	po     *PurchaseOrderService
	stock  *inventory.StockService
	events GoodsReceivingEventPublisher
}

func NewGoodsReceivingService(
	db *pgxpool.Pool,
	po *PurchaseOrderService,
	stock *inventory.StockService,
	events GoodsReceivingEventPublisher,
) *GoodsReceivingService {
	return &GoodsReceivingService{
		db:     db,
		po:     po,
		stock:  stock,
		events: events,
	}
}

func receivingHash(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func mustJSON(value any) []byte {
	raw, _ := json.Marshal(value)
	if len(raw) == 0 {
		return []byte(`{}`)
	}
	return raw
}

func absInt64(value int64) int64 {
	if value == math.MinInt64 {
		return math.MaxInt64
	}
	if value < 0 {
		return -value
	}
	return value
}

func multiplyExactInt64(a, b int64) (int64, error) {
	if a < 0 || b < 0 {
		return 0, ErrPurchasingExactConversion
	}
	if a == 0 || b == 0 {
		return 0, nil
	}
	if a > math.MaxInt64/b {
		return 0, ErrPurchasingExactConversion
	}
	return a * b, nil
}

func costVarianceBps(expected, actual int64) int64 {
	if expected <= 0 {
		if actual == expected {
			return 0
		}
		return 10000
	}

	diff := absInt64(actual - expected)

	if diff > math.MaxInt64/10000 {
		return math.MaxInt64
	}

	return diff * 10000 / expected
}

func allowedOverDelivery(
	ordered int64,
	toleranceBps int64,
) (int64, error) {
	if ordered <= 0 ||
		toleranceBps < 0 ||
		toleranceBps > 10000 {
		return 0, ErrGoodsReceiptInvalid
	}

	extraNumerator, err :=
		multiplyExactInt64(
			ordered,
			toleranceBps,
		)
	if err != nil {
		return 0, err
	}

	extra := extraNumerator / 10000

	if ordered > math.MaxInt64-extra {
		return 0, ErrGoodsReceiptInvalid
	}

	return ordered + extra, nil
}

func validateInspectionQuantities(
	line InspectGoodsReceiptLineInput,
) error {
	if line.DeliveredQuantity < 0 ||
		line.AcceptedQuantity < 0 ||
		line.DamagedQuantity < 0 ||
		line.RejectedQuantity < 0 ||
		line.ShortQuantity < 0 {
		return ErrGoodsReceiptInvalid
	}

	sum, err := multiplyExactInt64(1, line.AcceptedQuantity)
	if err != nil {
		return err
	}

	if sum > math.MaxInt64-line.DamagedQuantity {
		return ErrGoodsReceiptInvalid
	}
	sum += line.DamagedQuantity

	if sum > math.MaxInt64-line.RejectedQuantity {
		return ErrGoodsReceiptInvalid
	}
	sum += line.RejectedQuantity

	if line.DeliveredQuantity != sum {
		return fmt.Errorf(
			"%w: delivered must equal accepted + damaged + rejected",
			ErrGoodsReceiptInvalid,
		)
	}

	if line.WrongItem {
		if strings.TrimSpace(line.PurchaseOrderLineID) != "" ||
			line.AcceptedQuantity != 0 {
			return fmt.Errorf(
				"%w: wrong items cannot be linked to or accepted against a PO line",
				ErrGoodsReceiptInvalid,
			)
		}

		if strings.TrimSpace(line.WrongItemReference) == "" ||
			line.DeliveredQuantity <= 0 ||
			strings.TrimSpace(line.PurchaseUnitCode) == "" {
			return ErrGoodsReceiptInvalid
		}

		return nil
	}

	if strings.TrimSpace(line.PurchaseOrderLineID) == "" {
		return ErrGoodsReceiptInvalid
	}

	return nil
}

func (s *GoodsReceivingService) auditTx(
	ctx context.Context,
	tx pgx.Tx,
	actor GoodsReceivingActor,
	action string,
	aggregateType string,
	aggregateID string,
	branchID string,
	before any,
	after any,
	reason string,
) error {
	if s == nil || tx == nil {
		return ErrGoodsReceiptInvalid
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
            $8::jsonb,$9::jsonb,NULLIF($10,'')
        )
        `,
		"va_"+uuid.NewString(),
		actor.BusinessID,
		actor.StaffID,
		action,
		aggregateType,
		aggregateID,
		branchID,
		mustJSON(before),
		mustJSON(after),
		reason,
	)

	return err
}

func (s *GoodsReceivingService) publishTx(
	ctx context.Context,
	tx pgx.Tx,
	actor GoodsReceivingActor,
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

func scanGoodsReceipt(
	row pgx.Row,
) (GoodsReceipt, error) {
	var out GoodsReceipt

	err := row.Scan(
		&out.ID,
		&out.BusinessID,
		&out.PurchaseOrderID,
		&out.SupplierID,
		&out.BranchID,
		&out.SupplierDeliveryReference,
		&out.InvoiceReference,
		&out.InvoiceTotalMinor,
		&out.Currency,
		&out.Status,
		&out.ReceivedAt,
		&out.InspectedAt,
		&out.PostedAt,
		&out.ReversedAt,
		&out.ReceivedByStaffID,
		&out.InspectedByStaffID,
		&out.ReversedByStaffID,
		&out.InventoryReceiptID,
		&out.Notes,
		&out.ReversalReason,
		&out.Version,
		&out.CreatedAt,
		&out.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return GoodsReceipt{}, ErrGoodsReceiptNotFound
	}

	return out, err
}

const goodsReceiptSelect = `
SELECT
    id,
    business_id,
    purchase_order_id,
    supplier_id,
    branch_id,
    COALESCE(supplier_delivery_reference,''),
    COALESCE(invoice_reference,''),
    invoice_total_minor,
    currency,
    status,
    received_at,
    inspected_at,
    posted_at,
    reversed_at,
    received_by_staff_id,
    COALESCE(inspected_by_staff_id,''),
    COALESCE(reversed_by_staff_id,''),
    COALESCE(inventory_receipt_id,''),
    notes,
    COALESCE(reversal_reason,''),
    version,
    created_at,
    updated_at
FROM vendor_goods_receipts
`

func (s *GoodsReceivingService) PurchaseOrderBranch(
	ctx context.Context,
	businessID string,
	purchaseOrderID string,
) (string, error) {
	var branchID string

	err := s.db.QueryRow(
		ctx,
		`
        SELECT branch_id
          FROM vendor_purchase_orders
         WHERE id=$1
           AND business_id=$2
        `,
		strings.TrimSpace(purchaseOrderID),
		strings.TrimSpace(businessID),
	).Scan(&branchID)

	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrGoodsReceiptNotFound
	}

	return branchID, err
}

func (s *GoodsReceivingService) GoodsReceiptBranch(
	ctx context.Context,
	businessID string,
	receiptID string,
) (string, error) {
	var branchID string

	err := s.db.QueryRow(
		ctx,
		`
        SELECT branch_id
          FROM vendor_goods_receipts
         WHERE id=$1
           AND business_id=$2
        `,
		strings.TrimSpace(receiptID),
		strings.TrimSpace(businessID),
	).Scan(&branchID)

	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrGoodsReceiptNotFound
	}

	return branchID, err
}

func (s *GoodsReceivingService) DiscrepancyBranch(
	ctx context.Context,
	businessID string,
	discrepancyID string,
) (string, error) {
	var branchID string

	err := s.db.QueryRow(
		ctx,
		`
        SELECT branch_id
          FROM vendor_supplier_discrepancies
         WHERE id=$1
           AND business_id=$2
        `,
		strings.TrimSpace(discrepancyID),
		strings.TrimSpace(businessID),
	).Scan(&branchID)

	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrSupplierDiscrepancyNotFound
	}

	return branchID, err
}

func defaultReceivingPolicy(
	businessID string,
	branchID string,
) GoodsReceivingPolicy {
	branch := strings.TrimSpace(branchID)

	return GoodsReceivingPolicy{
		BusinessID: businessID,
		BranchID:   &branch,

		OverDeliveryMode:         OverDeliveryDeny,
		OverDeliveryToleranceBps: 0,

		CostVarianceApprovalBps:   0,
		CostVarianceApprovalMinor: 0,

		RequireCostVarianceApproval: true,
		Version:                     1,
	}
}

func scanReceivingPolicy(
	row pgx.Row,
) (GoodsReceivingPolicy, error) {
	var out GoodsReceivingPolicy

	err := row.Scan(
		&out.ID,
		&out.BusinessID,
		&out.BranchID,
		&out.OverDeliveryMode,
		&out.OverDeliveryToleranceBps,
		&out.CostVarianceApprovalBps,
		&out.CostVarianceApprovalMinor,
		&out.RequireCostVarianceApproval,
		&out.Version,
		&out.CreatedAt,
		&out.UpdatedAt,
	)

	return out, err
}

func (s *GoodsReceivingService) GetReceivingPolicy(
	ctx context.Context,
	actor GoodsReceivingActor,
	branchID string,
) (GoodsReceivingPolicy, error) {
	if s == nil || s.db == nil {
		return GoodsReceivingPolicy{}, ErrGoodsReceiptInvalid
	}

	return s.effectivePolicyQ(
		ctx,
		s.db,
		actor.BusinessID,
		strings.TrimSpace(branchID),
	)
}

// policyQuerier lets both pool and tx satisfy effectivePolicyTx.
type policyQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (s *GoodsReceivingService) effectivePolicyQ(
	ctx context.Context,
	q policyQuerier,
	businessID string,
	branchID string,
) (GoodsReceivingPolicy, error) {
	out, err := scanReceivingPolicy(
		q.QueryRow(
			ctx,
			`
            SELECT
                id,
                business_id,
                branch_id,
                over_delivery_mode,
                over_delivery_tolerance_bps,
                cost_variance_approval_bps,
                cost_variance_approval_minor,
                require_cost_variance_approval,
                version,
                created_at,
                updated_at
              FROM vendor_goods_receiving_policies
             WHERE business_id=$1
               AND (
                   branch_id=$2
                   OR branch_id IS NULL
               )
             ORDER BY
                   CASE WHEN branch_id=$2 THEN 0 ELSE 1 END
             LIMIT 1
            `,
			businessID,
			branchID,
		),
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return defaultReceivingPolicy(
			businessID,
			branchID,
		), nil
	}

	return out, err
}

func (s *GoodsReceivingService) PutReceivingPolicy(
	ctx context.Context,
	actor GoodsReceivingActor,
	branchID string,
	input PutGoodsReceivingPolicyInput,
) (GoodsReceivingPolicy, error) {
	branchID = strings.TrimSpace(branchID)

	if branchID == "" ||
		(input.OverDeliveryMode != OverDeliveryDeny &&
			input.OverDeliveryMode != OverDeliveryAllowWithTolerance) ||
		input.OverDeliveryToleranceBps < 0 ||
		input.OverDeliveryToleranceBps > 10000 ||
		input.CostVarianceApprovalBps < 0 ||
		input.CostVarianceApprovalBps > 10000 ||
		input.CostVarianceApprovalMinor < 0 {
		return GoodsReceivingPolicy{}, ErrGoodsReceiptInvalid
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return GoodsReceivingPolicy{}, err
	}
	defer tx.Rollback(ctx)

	var branchBusiness string

	if err = tx.QueryRow(
		ctx,
		`
        SELECT business_id
          FROM vendor_branches
         WHERE id=$1
           AND business_id=$2
        `,
		branchID,
		actor.BusinessID,
	).Scan(&branchBusiness); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return GoodsReceivingPolicy{}, ErrGoodsReceiptNotFound
		}
		return GoodsReceivingPolicy{}, err
	}

	before, _ :=
		s.effectivePolicyQ(
			ctx,
			tx,
			actor.BusinessID,
			branchID,
		)

	var out GoodsReceivingPolicy

	err = tx.QueryRow(
		ctx,
		`
        INSERT INTO vendor_goods_receiving_policies(
            id,
            business_id,
            branch_id,
            over_delivery_mode,
            over_delivery_tolerance_bps,
            cost_variance_approval_bps,
            cost_variance_approval_minor,
            require_cost_variance_approval
        )
        VALUES($1,$2,$3,$4,$5,$6,$7,$8)
        ON CONFLICT(business_id,branch_id)
        WHERE branch_id IS NOT NULL
        DO UPDATE SET
            over_delivery_mode=EXCLUDED.over_delivery_mode,
            over_delivery_tolerance_bps=
                EXCLUDED.over_delivery_tolerance_bps,
            cost_variance_approval_bps=
                EXCLUDED.cost_variance_approval_bps,
            cost_variance_approval_minor=
                EXCLUDED.cost_variance_approval_minor,
            require_cost_variance_approval=
                EXCLUDED.require_cost_variance_approval,
            version=vendor_goods_receiving_policies.version+1,
            updated_at=now()
        RETURNING
            id,business_id,branch_id,
            over_delivery_mode,
            over_delivery_tolerance_bps,
            cost_variance_approval_bps,
            cost_variance_approval_minor,
            require_cost_variance_approval,
            version,
            created_at,
            updated_at
        `,
		"grp_"+uuid.NewString(),
		actor.BusinessID,
		branchID,
		input.OverDeliveryMode,
		input.OverDeliveryToleranceBps,
		input.CostVarianceApprovalBps,
		input.CostVarianceApprovalMinor,
		input.RequireCostVarianceApproval,
	).Scan(
		&out.ID,
		&out.BusinessID,
		&out.BranchID,
		&out.OverDeliveryMode,
		&out.OverDeliveryToleranceBps,
		&out.CostVarianceApprovalBps,
		&out.CostVarianceApprovalMinor,
		&out.RequireCostVarianceApproval,
		&out.Version,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		return GoodsReceivingPolicy{}, err
	}

	if err = s.auditTx(
		ctx,
		tx,
		actor,
		"vendor.purchasing.receiving_policy.updated",
		"vendor_goods_receiving_policy",
		out.ID,
		branchID,
		before,
		out,
		"",
	); err != nil {
		return GoodsReceivingPolicy{}, err
	}

	if err = s.publishTx(
		ctx,
		tx,
		actor,
		"vendor.purchasing.receiving_policy.updated",
		"vendor_goods_receiving_policy",
		out.ID,
		out.Version,
		map[string]any{
			"branch_id":                      branchID,
			"over_delivery_mode":             out.OverDeliveryMode,
			"over_delivery_tolerance_bps":    out.OverDeliveryToleranceBps,
			"cost_variance_approval_bps":     out.CostVarianceApprovalBps,
			"cost_variance_approval_minor":   out.CostVarianceApprovalMinor,
			"require_cost_variance_approval": out.RequireCostVarianceApproval,
		},
	); err != nil {
		return GoodsReceivingPolicy{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return GoodsReceivingPolicy{}, err
	}

	return out, nil
}

func (s *GoodsReceivingService) CreateGoodsReceipt(
	ctx context.Context,
	actor GoodsReceivingActor,
	input CreateGoodsReceiptInput,
) (GoodsReceipt, error) {
	if s == nil ||
		s.db == nil ||
		strings.TrimSpace(actor.BusinessID) == "" ||
		strings.TrimSpace(actor.StaffID) == "" ||
		strings.TrimSpace(input.PurchaseOrderID) == "" {
		return GoodsReceipt{}, ErrGoodsReceiptInvalid
	}

	if input.InvoiceTotalMinor != nil &&
		*input.InvoiceTotalMinor < 0 {
		return GoodsReceipt{}, ErrGoodsReceiptInvalid
	}

	if input.ReceivedAt.IsZero() {
		input.ReceivedAt = time.Now().UTC()
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return GoodsReceipt{}, err
	}
	defer tx.Rollback(ctx)

	var supplierID string
	var branchID string
	var poStatus string
	var currency string

	err = tx.QueryRow(
		ctx,
		`
        SELECT
            supplier_id,
            branch_id,
            status,
            currency_code
          FROM vendor_purchase_orders
         WHERE id=$1
           AND business_id=$2
         FOR UPDATE
        `,
		strings.TrimSpace(input.PurchaseOrderID),
		actor.BusinessID,
	).Scan(
		&supplierID,
		&branchID,
		&poStatus,
		&currency,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return GoodsReceipt{}, ErrGoodsReceiptNotFound
	}
	if err != nil {
		return GoodsReceipt{}, err
	}

	if poStatus != "ORDERED" &&
		poStatus != "PARTIALLY_RECEIVED" {
		return GoodsReceipt{}, ErrGoodsReceiptState
	}

	out := GoodsReceipt{
		ID: "gr_" + uuid.NewString(),

		BusinessID:      actor.BusinessID,
		PurchaseOrderID: strings.TrimSpace(input.PurchaseOrderID),
		SupplierID:      supplierID,
		BranchID:        branchID,

		SupplierDeliveryReference: strings.TrimSpace(input.SupplierDeliveryReference),
		InvoiceReference:          strings.TrimSpace(input.InvoiceReference),
		InvoiceTotalMinor:         input.InvoiceTotalMinor,

		Currency: currency,
		Status:   GoodsReceiptDraft,

		ReceivedAt: input.ReceivedAt,

		ReceivedByStaffID: actor.StaffID,

		Notes: strings.TrimSpace(input.Notes),

		Version: 1,
	}

	err = tx.QueryRow(
		ctx,
		`
        INSERT INTO vendor_goods_receipts(
            id,
            business_id,
            purchase_order_id,
            supplier_id,
            branch_id,
            supplier_delivery_reference,
            invoice_reference,
            invoice_total_minor,
            currency,
            status,
            received_at,
            received_by_staff_id,
            notes
        )
        VALUES(
            $1,$2,$3,$4,$5,
            NULLIF($6,''),
            NULLIF($7,''),
            $8,$9,'DRAFT',$10,$11,$12
        )
        RETURNING created_at,updated_at
        `,
		out.ID,
		out.BusinessID,
		out.PurchaseOrderID,
		out.SupplierID,
		out.BranchID,
		out.SupplierDeliveryReference,
		out.InvoiceReference,
		out.InvoiceTotalMinor,
		out.Currency,
		out.ReceivedAt,
		out.ReceivedByStaffID,
		out.Notes,
	).Scan(
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		return GoodsReceipt{}, err
	}

	if err = s.auditTx(
		ctx,
		tx,
		actor,
		"vendor.purchasing.goods_receipt.created",
		"vendor_goods_receipt",
		out.ID,
		out.BranchID,
		nil,
		out,
		"",
	); err != nil {
		return GoodsReceipt{}, err
	}

	if err = s.publishTx(
		ctx,
		tx,
		actor,
		"vendor.purchasing.goods_receipt.created",
		"vendor_goods_receipt",
		out.ID,
		out.Version,
		map[string]any{
			"goods_receipt_id":  out.ID,
			"purchase_order_id": out.PurchaseOrderID,
			"supplier_id":       out.SupplierID,
			"branch_id":         out.BranchID,
			"status":            out.Status,
		},
	); err != nil {
		return GoodsReceipt{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return GoodsReceipt{}, err
	}

	return out, nil
}

type poLineSnapshot struct {
	ID string

	SupplierProductID string
	InventoryItemID   string

	OrderedQuantity  int64
	ReceivedQuantity int64

	PurchaseUnitCode string
	Conversion       int64

	ExpectedUnitCostMinor int64
}

func (s *GoodsReceivingService) loadPOLineTx(
	ctx context.Context,
	tx pgx.Tx,
	businessID string,
	purchaseOrderID string,
	lineID string,
) (poLineSnapshot, error) {
	var out poLineSnapshot

	err := tx.QueryRow(
		ctx,
		`
        SELECT
            id,
            COALESCE(supplier_product_id,''),
            COALESCE(inventory_item_id,''),
            purchase_quantity,
            received_purchase_quantity,
            purchase_unit_code_snapshot,
            inventory_units_per_purchase_unit_snapshot,
            unit_cost_minor
          FROM vendor_purchase_order_lines
         WHERE id=$1
           AND business_id=$2
           AND purchase_order_id=$3
         FOR UPDATE
        `,
		strings.TrimSpace(lineID),
		businessID,
		purchaseOrderID,
	).Scan(
		&out.ID,
		&out.SupplierProductID,
		&out.InventoryItemID,
		&out.OrderedQuantity,
		&out.ReceivedQuantity,
		&out.PurchaseUnitCode,
		&out.Conversion,
		&out.ExpectedUnitCostMinor,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return poLineSnapshot{}, ErrGoodsReceiptNotFound
	}

	if err != nil {
		return poLineSnapshot{}, err
	}

	if out.OrderedQuantity <= 0 ||
		out.ReceivedQuantity < 0 ||
		out.Conversion <= 0 ||
		out.SupplierProductID == "" ||
		out.InventoryItemID == "" {
		return poLineSnapshot{},
			ErrPurchasingExactConversion
	}

	return out, nil
}

func costDiscrepancyStatus(
	policy GoodsReceivingPolicy,
	diffMinor int64,
	varianceBps int64,
) string {
	if !policy.RequireCostVarianceApproval {
		return DiscrepancyOpen
	}

	if diffMinor <= 0 {
		return DiscrepancyOpen
	}

	if policy.CostVarianceApprovalMinor == 0 &&
		policy.CostVarianceApprovalBps == 0 {
		return DiscrepancyApprovalRequired
	}

	if policy.CostVarianceApprovalMinor > 0 &&
		diffMinor >= policy.CostVarianceApprovalMinor {
		return DiscrepancyApprovalRequired
	}

	if policy.CostVarianceApprovalBps > 0 &&
		varianceBps >= policy.CostVarianceApprovalBps {
		return DiscrepancyApprovalRequired
	}

	return DiscrepancyOpen
}

func (s *GoodsReceivingService) insertDiscrepancyTx(
	ctx context.Context,
	tx pgx.Tx,
	actor GoodsReceivingActor,
	receipt GoodsReceipt,
	line *GoodsReceiptLine,
	discrepancyType string,
	status string,
	quantity *int64,
	expectedCost *int64,
	actualCost *int64,
	varianceMinor *int64,
	varianceBps *int64,
	reason string,
	notes string,
) error {
	id := "sd_" + uuid.NewString()

	var poLineID any
	var receiptLineID any

	if line != nil {
		if line.PurchaseOrderLineID != "" {
			poLineID = line.PurchaseOrderLineID
		}
		if line.ID != "" {
			receiptLineID = line.ID
		}
	}

	_, err := tx.Exec(
		ctx,
		`
        INSERT INTO vendor_supplier_discrepancies(
            id,
            business_id,
            supplier_id,
            purchase_order_id,
            purchase_order_line_id,
            goods_receipt_id,
            goods_receipt_line_id,
            branch_id,
            discrepancy_type,
            status,
            quantity,
            expected_cost_minor,
            actual_cost_minor,
            variance_minor,
            variance_bps,
            currency,
            reason_code,
            notes,
            opened_by_staff_id
        )
        VALUES(
            $1,$2,$3,$4,$5,$6,$7,$8,
            $9,$10,$11,$12,$13,$14,$15,
            $16,NULLIF($17,''),$18,$19
        )
        `,
		id,
		actor.BusinessID,
		receipt.SupplierID,
		receipt.PurchaseOrderID,
		poLineID,
		receipt.ID,
		receiptLineID,
		receipt.BranchID,
		discrepancyType,
		status,
		quantity,
		expectedCost,
		actualCost,
		varianceMinor,
		varianceBps,
		receipt.Currency,
		reason,
		strings.TrimSpace(notes),
		actor.StaffID,
	)
	if err != nil {
		return err
	}

	return s.publishTx(
		ctx,
		tx,
		actor,
		"vendor.purchasing.supplier_discrepancy.opened",
		"vendor_supplier_discrepancy",
		id,
		1,
		map[string]any{
			"discrepancy_id":    id,
			"goods_receipt_id":  receipt.ID,
			"purchase_order_id": receipt.PurchaseOrderID,
			"discrepancy_type":  discrepancyType,
			"status":            status,
			"branch_id":         receipt.BranchID,
		},
	)
}

func (s *GoodsReceivingService) InspectGoodsReceipt(
	ctx context.Context,
	actor GoodsReceivingActor,
	receiptID string,
	expectedVersion int64,
	input InspectGoodsReceiptInput,
) (GoodsReceipt, error) {
	if s == nil ||
		s.db == nil ||
		expectedVersion <= 0 ||
		len(input.Lines) == 0 {
		return GoodsReceipt{}, ErrGoodsReceiptInvalid
	}

	for _, line := range input.Lines {
		if err := validateInspectionQuantities(line); err != nil {
			return GoodsReceipt{}, err
		}
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return GoodsReceipt{}, err
	}
	defer tx.Rollback(ctx)

	receipt, err := scanGoodsReceipt(
		tx.QueryRow(
			ctx,
			goodsReceiptSelect+
				` WHERE id=$1 AND business_id=$2 FOR UPDATE`,
			strings.TrimSpace(receiptID),
			actor.BusinessID,
		),
	)
	if err != nil {
		return GoodsReceipt{}, err
	}

	if receipt.Version != expectedVersion {
		return GoodsReceipt{}, ErrGoodsReceiptVersion
	}

	if receipt.Status != GoodsReceiptDraft &&
		receipt.Status != GoodsReceiptInspected {
		return GoodsReceipt{}, ErrGoodsReceiptState
	}

	var poStatus string

	if err = tx.QueryRow(
		ctx,
		`
        SELECT status
          FROM vendor_purchase_orders
         WHERE id=$1
           AND business_id=$2
         FOR UPDATE
        `,
		receipt.PurchaseOrderID,
		actor.BusinessID,
	).Scan(&poStatus); err != nil {
		return GoodsReceipt{}, err
	}

	if poStatus != "ORDERED" &&
		poStatus != "PARTIALLY_RECEIVED" {
		return GoodsReceipt{}, ErrGoodsReceiptState
	}

	policy, err := s.effectivePolicyQ(
		ctx,
		tx,
		actor.BusinessID,
		receipt.BranchID,
	)
	if err != nil {
		return GoodsReceipt{}, err
	}

	// Re-inspection before confirmation replaces only the mutable inspection
	// draft and voids unresolved discrepancy projections. Posted evidence is
	// never touched.
	if receipt.Status == GoodsReceiptInspected {
		if _, err = tx.Exec(
			ctx,
			`
            UPDATE vendor_supplier_discrepancies
               SET status='VOIDED',
                   updated_at=now(),
                   version=version+1
             WHERE business_id=$1
               AND goods_receipt_id=$2
               AND status IN ('OPEN','APPROVAL_REQUIRED')
            `,
			actor.BusinessID,
			receipt.ID,
		); err != nil {
			return GoodsReceipt{}, err
		}

		if _, err = tx.Exec(
			ctx,
			`
            DELETE FROM vendor_goods_receipt_lines
             WHERE business_id=$1
               AND goods_receipt_id=$2
            `,
			actor.BusinessID,
			receipt.ID,
		); err != nil {
			return GoodsReceipt{}, err
		}
	}

	seenPOLines := map[string]bool{}
	lines := make([]GoodsReceiptLine, 0, len(input.Lines))

	var expectedInvoiceMinor int64

	for _, requested := range input.Lines {
		if requested.WrongItem {
			line := GoodsReceiptLine{
				ID: "grl_" + uuid.NewString(),

				GoodsReceiptID:  receipt.ID,
				PurchaseOrderID: receipt.PurchaseOrderID,

				PurchaseUnitCode:              strings.TrimSpace(requested.PurchaseUnitCode),
				InventoryUnitsPerPurchaseUnit: 1,

				DeliveredQuantity: requested.DeliveredQuantity,
				AcceptedQuantity:  0,
				DamagedQuantity:   requested.DamagedQuantity,
				RejectedQuantity:  requested.RejectedQuantity,
				ShortQuantity:     requested.ShortQuantity,

				InventoryQuantity: 0,

				WrongItem:          true,
				WrongItemReference: strings.TrimSpace(requested.WrongItemReference),

				QualityIssue: requested.QualityIssue,
				QualityNotes: strings.TrimSpace(requested.QualityNotes),

				ActualUnitCostMinor: requested.ActualUnitCostMinor,

				Currency: receipt.Currency,

				LotNumber:   strings.TrimSpace(requested.LotNumber),
				BatchNumber: strings.TrimSpace(requested.BatchNumber),

				ManufacturedAt: requested.ManufacturedAt,
				BestBeforeAt:   requested.BestBeforeAt,
				ExpiresAt:      requested.ExpiresAt,
			}

			if err = tx.QueryRow(
				ctx,
				`
                INSERT INTO vendor_goods_receipt_lines(
                    id,business_id,goods_receipt_id,purchase_order_id,
                    purchase_order_line_id,
                    supplier_product_id,
                    inventory_item_id,
                    purchase_unit_code,
                    inventory_units_per_purchase_unit,
                    ordered_quantity_snapshot,
                    received_before_snapshot,
                    delivered_quantity,
                    accepted_quantity,
                    damaged_quantity,
                    rejected_quantity,
                    short_quantity,
                    inventory_quantity,
                    wrong_item,
                    wrong_item_reference,
                    quality_issue,
                    quality_notes,
                    expected_unit_cost_minor,
                    actual_unit_cost_minor,
                    currency,
                    lot_number,
                    batch_number,
                    manufactured_at,
                    best_before_at,
                    expires_at
                )
                VALUES(
                    $1,$2,$3,$4,
                    NULL,NULL,NULL,
                    $5,1,NULL,NULL,
                    $6,0,$7,$8,$9,0,
                    true,NULLIF($10,''),
                    $11,$12,NULL,$13,$14,
                    NULLIF($15,''),
                    NULLIF($16,''),
                    $17,$18,$19
                )
                RETURNING created_at
                `,
				line.ID,
				actor.BusinessID,
				receipt.ID,
				receipt.PurchaseOrderID,
				line.PurchaseUnitCode,
				line.DeliveredQuantity,
				line.DamagedQuantity,
				line.RejectedQuantity,
				line.ShortQuantity,
				line.WrongItemReference,
				line.QualityIssue,
				line.QualityNotes,
				line.ActualUnitCostMinor,
				line.Currency,
				line.LotNumber,
				line.BatchNumber,
				line.ManufacturedAt,
				line.BestBeforeAt,
				line.ExpiresAt,
			).Scan(&line.CreatedAt); err != nil {
				return GoodsReceipt{}, err
			}

			qty := line.DeliveredQuantity

			if err = s.insertDiscrepancyTx(
				ctx,
				tx,
				actor,
				receipt,
				&line,
				DiscrepancyWrongItem,
				DiscrepancyOpen,
				&qty,
				nil,
				line.ActualUnitCostMinor,
				nil,
				nil,
				"WRONG_ITEM",
				line.WrongItemReference,
			); err != nil {
				return GoodsReceipt{}, err
			}

			if line.QualityIssue {
				if err = s.insertDiscrepancyTx(
					ctx,
					tx,
					actor,
					receipt,
					&line,
					DiscrepancyQualityIssue,
					DiscrepancyOpen,
					&qty,
					nil,
					nil,
					nil,
					nil,
					"QUALITY_ISSUE",
					line.QualityNotes,
				); err != nil {
					return GoodsReceipt{}, err
				}
			}

			lines = append(lines, line)
			continue
		}

		if seenPOLines[requested.PurchaseOrderLineID] {
			return GoodsReceipt{},
				fmt.Errorf(
					"%w: the same PurchaseOrderLine appears twice in one receipt",
					ErrGoodsReceiptInvalid,
				)
		}
		seenPOLines[requested.PurchaseOrderLineID] = true

		snapshot, loadErr := s.loadPOLineTx(
			ctx,
			tx,
			actor.BusinessID,
			receipt.PurchaseOrderID,
			requested.PurchaseOrderLineID,
		)
		if loadErr != nil {
			return GoodsReceipt{}, loadErr
		}

		inventoryQty, convertErr :=
			multiplyExactInt64(
				requested.AcceptedQuantity,
				snapshot.Conversion,
			)
		if convertErr != nil {
			return GoodsReceipt{}, convertErr
		}

		proposedReceived := snapshot.ReceivedQuantity

		if proposedReceived >
			math.MaxInt64-requested.AcceptedQuantity {
			return GoodsReceipt{}, ErrGoodsReceiptInvalid
		}

		proposedReceived += requested.AcceptedQuantity

		overQuantity := int64(0)

		if proposedReceived > snapshot.OrderedQuantity {
			overQuantity =
				proposedReceived -
					snapshot.OrderedQuantity

			if policy.OverDeliveryMode ==
				OverDeliveryDeny {
				return GoodsReceipt{},
					ErrPurchasingOverDelivery
			}

			maxAllowed, allowErr :=
				allowedOverDelivery(
					snapshot.OrderedQuantity,
					policy.OverDeliveryToleranceBps,
				)
			if allowErr != nil {
				return GoodsReceipt{}, allowErr
			}

			if proposedReceived > maxAllowed {
				return GoodsReceipt{},
					ErrPurchasingOverDelivery
			}
		}

		actualCost := snapshot.ExpectedUnitCostMinor

		if requested.ActualUnitCostMinor != nil {
			if *requested.ActualUnitCostMinor < 0 {
				return GoodsReceipt{}, ErrGoodsReceiptInvalid
			}

			actualCost = *requested.ActualUnitCostMinor
		}

		orderedSnapshot := snapshot.OrderedQuantity
		receivedSnapshot := snapshot.ReceivedQuantity
		expectedCost := snapshot.ExpectedUnitCostMinor
		actualCostCopy := actualCost

		line := GoodsReceiptLine{
			ID: "grl_" + uuid.NewString(),

			GoodsReceiptID:      receipt.ID,
			PurchaseOrderID:     receipt.PurchaseOrderID,
			PurchaseOrderLineID: snapshot.ID,

			SupplierProductID: snapshot.SupplierProductID,
			InventoryItemID:   snapshot.InventoryItemID,

			PurchaseUnitCode:              snapshot.PurchaseUnitCode,
			InventoryUnitsPerPurchaseUnit: snapshot.Conversion,

			OrderedQuantitySnapshot: &orderedSnapshot,
			ReceivedBeforeSnapshot:  &receivedSnapshot,

			DeliveredQuantity: requested.DeliveredQuantity,
			AcceptedQuantity:  requested.AcceptedQuantity,
			DamagedQuantity:   requested.DamagedQuantity,
			RejectedQuantity:  requested.RejectedQuantity,
			ShortQuantity:     requested.ShortQuantity,

			InventoryQuantity: inventoryQty,

			QualityIssue: requested.QualityIssue,
			QualityNotes: strings.TrimSpace(requested.QualityNotes),

			ExpectedUnitCostMinor: &expectedCost,
			ActualUnitCostMinor:   &actualCostCopy,

			Currency: receipt.Currency,

			LotNumber:   strings.TrimSpace(requested.LotNumber),
			BatchNumber: strings.TrimSpace(requested.BatchNumber),

			ManufacturedAt: requested.ManufacturedAt,
			BestBeforeAt:   requested.BestBeforeAt,
			ExpiresAt:      requested.ExpiresAt,
		}

		if err = tx.QueryRow(
			ctx,
			`
            INSERT INTO vendor_goods_receipt_lines(
                id,business_id,goods_receipt_id,purchase_order_id,
                purchase_order_line_id,
                supplier_product_id,
                inventory_item_id,
                purchase_unit_code,
                inventory_units_per_purchase_unit,
                ordered_quantity_snapshot,
                received_before_snapshot,
                delivered_quantity,
                accepted_quantity,
                damaged_quantity,
                rejected_quantity,
                short_quantity,
                inventory_quantity,
                wrong_item,
                wrong_item_reference,
                quality_issue,
                quality_notes,
                expected_unit_cost_minor,
                actual_unit_cost_minor,
                currency,
                lot_number,
                batch_number,
                manufactured_at,
                best_before_at,
                expires_at
            )
            VALUES(
                $1,$2,$3,$4,$5,$6,$7,$8,$9,
                $10,$11,$12,$13,$14,$15,$16,$17,
                false,NULL,$18,$19,$20,$21,$22,
                NULLIF($23,''),
                NULLIF($24,''),
                $25,$26,$27
            )
            RETURNING created_at
            `,
			line.ID,
			actor.BusinessID,
			receipt.ID,
			receipt.PurchaseOrderID,
			line.PurchaseOrderLineID,
			line.SupplierProductID,
			line.InventoryItemID,
			line.PurchaseUnitCode,
			line.InventoryUnitsPerPurchaseUnit,
			line.OrderedQuantitySnapshot,
			line.ReceivedBeforeSnapshot,
			line.DeliveredQuantity,
			line.AcceptedQuantity,
			line.DamagedQuantity,
			line.RejectedQuantity,
			line.ShortQuantity,
			line.InventoryQuantity,
			line.QualityIssue,
			line.QualityNotes,
			line.ExpectedUnitCostMinor,
			line.ActualUnitCostMinor,
			line.Currency,
			line.LotNumber,
			line.BatchNumber,
			line.ManufacturedAt,
			line.BestBeforeAt,
			line.ExpiresAt,
		).Scan(&line.CreatedAt); err != nil {
			return GoodsReceipt{}, err
		}

		if line.DeliveredQuantity > 0 {
			expectedLineInvoice, calcErr :=
				multiplyExactInt64(
					line.DeliveredQuantity,
					snapshot.ExpectedUnitCostMinor,
				)
			if calcErr != nil {
				return GoodsReceipt{}, calcErr
			}

			if expectedInvoiceMinor >
				math.MaxInt64-expectedLineInvoice {
				return GoodsReceipt{}, ErrGoodsReceiptInvalid
			}

			expectedInvoiceMinor += expectedLineInvoice
		}

		if line.DamagedQuantity > 0 {
			qty := line.DamagedQuantity

			if err = s.insertDiscrepancyTx(
				ctx,
				tx,
				actor,
				receipt,
				&line,
				DiscrepancyDamaged,
				DiscrepancyOpen,
				&qty, nil, nil, nil, nil,
				"DAMAGED",
				line.QualityNotes,
			); err != nil {
				return GoodsReceipt{}, err
			}
		}

		if line.RejectedQuantity > 0 {
			qty := line.RejectedQuantity

			if err = s.insertDiscrepancyTx(
				ctx,
				tx,
				actor,
				receipt,
				&line,
				DiscrepancyRejected,
				DiscrepancyOpen,
				&qty, nil, nil, nil, nil,
				"REJECTED",
				line.QualityNotes,
			); err != nil {
				return GoodsReceipt{}, err
			}
		}

		if line.ShortQuantity > 0 {
			qty := line.ShortQuantity

			if err = s.insertDiscrepancyTx(
				ctx,
				tx,
				actor,
				receipt,
				&line,
				DiscrepancyShortDelivery,
				DiscrepancyOpen,
				&qty, nil, nil, nil, nil,
				"SHORT_DELIVERY",
				"",
			); err != nil {
				return GoodsReceipt{}, err
			}
		}

		if overQuantity > 0 {
			qty := overQuantity

			if err = s.insertDiscrepancyTx(
				ctx,
				tx,
				actor,
				receipt,
				&line,
				DiscrepancyOverDelivery,
				DiscrepancyOpen,
				&qty, nil, nil, nil, nil,
				"OVER_DELIVERY_WITHIN_POLICY",
				"",
			); err != nil {
				return GoodsReceipt{}, err
			}
		}

		if line.QualityIssue {
			qty := line.DeliveredQuantity

			if err = s.insertDiscrepancyTx(
				ctx,
				tx,
				actor,
				receipt,
				&line,
				DiscrepancyQualityIssue,
				DiscrepancyOpen,
				&qty, nil, nil, nil, nil,
				"QUALITY_ISSUE",
				line.QualityNotes,
			); err != nil {
				return GoodsReceipt{}, err
			}
		}

		if actualCost != snapshot.ExpectedUnitCostMinor {
			expectedTotal, calcErr :=
				multiplyExactInt64(
					line.DeliveredQuantity,
					snapshot.ExpectedUnitCostMinor,
				)
			if calcErr != nil {
				return GoodsReceipt{}, calcErr
			}

			actualTotal, calcErr :=
				multiplyExactInt64(
					line.DeliveredQuantity,
					actualCost,
				)
			if calcErr != nil {
				return GoodsReceipt{}, calcErr
			}

			diff := absInt64(actualTotal - expectedTotal)
			bps := costVarianceBps(
				expectedTotal,
				actualTotal,
			)

			status := costDiscrepancyStatus(
				policy,
				diff,
				bps,
			)

			if err = s.insertDiscrepancyTx(
				ctx,
				tx,
				actor,
				receipt,
				&line,
				DiscrepancyCostVariance,
				status,
				nil,
				&expectedTotal,
				&actualTotal,
				&diff,
				&bps,
				"COST_VARIANCE",
				"Supplier unit cost differs from the approved PurchaseOrder snapshot.",
			); err != nil {
				return GoodsReceipt{}, err
			}
		}

		lines = append(lines, line)
	}

	// Invoice-level variance is recorded separately. The PurchaseOrder remains
	// immutable; an invoice never rewrites the approved PO cost snapshot.
	if receipt.InvoiceTotalMinor != nil &&
		*receipt.InvoiceTotalMinor != expectedInvoiceMinor {
		actual := *receipt.InvoiceTotalMinor
		expected := expectedInvoiceMinor
		diff := absInt64(actual - expected)
		bps := costVarianceBps(expected, actual)

		status := costDiscrepancyStatus(
			policy,
			diff,
			bps,
		)

		if err = s.insertDiscrepancyTx(
			ctx,
			tx,
			actor,
			receipt,
			nil,
			DiscrepancyCostVariance,
			status,
			nil,
			&expected,
			&actual,
			&diff,
			&bps,
			"INVOICE_TOTAL_COST_VARIANCE",
			"Supplier invoice total differs from the expected cost of this delivery.",
		); err != nil {
			return GoodsReceipt{}, err
		}
	}

	inspectedAt := time.Now().UTC()

	err = tx.QueryRow(
		ctx,
		`
        UPDATE vendor_goods_receipts
           SET status='INSPECTED',
               inspected_at=$3,
               inspected_by_staff_id=$4,
               updated_at=now(),
               version=version+1
         WHERE id=$1
           AND business_id=$2
           AND version=$5
           AND status IN ('DRAFT','INSPECTED')
        RETURNING
            status,
            inspected_at,
            inspected_by_staff_id,
            updated_at,
            version
        `,
		receipt.ID,
		actor.BusinessID,
		inspectedAt,
		actor.StaffID,
		expectedVersion,
	).Scan(
		&receipt.Status,
		&receipt.InspectedAt,
		&receipt.InspectedByStaffID,
		&receipt.UpdatedAt,
		&receipt.Version,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return GoodsReceipt{}, ErrGoodsReceiptVersion
	}
	if err != nil {
		return GoodsReceipt{}, err
	}

	receipt.Lines = lines

	if err = s.auditTx(
		ctx,
		tx,
		actor,
		"vendor.purchasing.goods_receipt.inspected",
		"vendor_goods_receipt",
		receipt.ID,
		receipt.BranchID,
		nil,
		map[string]any{
			"status":     receipt.Status,
			"line_count": len(lines),
		},
		"",
	); err != nil {
		return GoodsReceipt{}, err
	}

	if err = s.publishTx(
		ctx,
		tx,
		actor,
		"vendor.purchasing.goods_receipt.inspected",
		"vendor_goods_receipt",
		receipt.ID,
		receipt.Version,
		map[string]any{
			"goods_receipt_id":  receipt.ID,
			"purchase_order_id": receipt.PurchaseOrderID,
			"branch_id":         receipt.BranchID,
			"status":            receipt.Status,
			"line_count":        len(lines),
		},
	); err != nil {
		return GoodsReceipt{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return GoodsReceipt{}, err
	}

	return receipt, nil
}

func scanReceiptLine(
	row pgx.Row,
) (GoodsReceiptLine, error) {
	var out GoodsReceiptLine

	err := row.Scan(
		&out.ID,
		&out.GoodsReceiptID,
		&out.PurchaseOrderID,
		&out.PurchaseOrderLineID,
		&out.SupplierProductID,
		&out.InventoryItemID,
		&out.PurchaseUnitCode,
		&out.InventoryUnitsPerPurchaseUnit,
		&out.OrderedQuantitySnapshot,
		&out.ReceivedBeforeSnapshot,
		&out.DeliveredQuantity,
		&out.AcceptedQuantity,
		&out.DamagedQuantity,
		&out.RejectedQuantity,
		&out.ShortQuantity,
		&out.InventoryQuantity,
		&out.WrongItem,
		&out.WrongItemReference,
		&out.QualityIssue,
		&out.QualityNotes,
		&out.ExpectedUnitCostMinor,
		&out.ActualUnitCostMinor,
		&out.Currency,
		&out.LotNumber,
		&out.BatchNumber,
		&out.ManufacturedAt,
		&out.BestBeforeAt,
		&out.ExpiresAt,
		&out.InventoryReceiptLineID,
		&out.InventoryMovementID,
		&out.InventoryLotID,
		&out.CreatedAt,
	)

	return out, err
}

const goodsReceiptLineSelect = `
SELECT
    id,
    goods_receipt_id,
    purchase_order_id,
    COALESCE(purchase_order_line_id,''),
    COALESCE(supplier_product_id,''),
    COALESCE(inventory_item_id,''),
    purchase_unit_code,
    inventory_units_per_purchase_unit,
    ordered_quantity_snapshot,
    received_before_snapshot,
    delivered_quantity,
    accepted_quantity,
    damaged_quantity,
    rejected_quantity,
    short_quantity,
    inventory_quantity,
    wrong_item,
    COALESCE(wrong_item_reference,''),
    quality_issue,
    quality_notes,
    expected_unit_cost_minor,
    actual_unit_cost_minor,
    currency,
    COALESCE(lot_number,''),
    COALESCE(batch_number,''),
    manufactured_at,
    best_before_at,
    expires_at,
    COALESCE(inventory_receipt_line_id,''),
    COALESCE(inventory_movement_id,''),
    COALESCE(inventory_lot_id,''),
    created_at
FROM vendor_goods_receipt_lines
`

func (s *GoodsReceivingService) loadReceiptLinesTx(
	ctx context.Context,
	tx pgx.Tx,
	businessID string,
	receiptID string,
	forUpdate bool,
) ([]GoodsReceiptLine, error) {
	query :=
		goodsReceiptLineSelect +
			`
            WHERE business_id=$1
              AND goods_receipt_id=$2
            ORDER BY id
            `

	if forUpdate {
		query += ` FOR UPDATE`
	}

	rows, err := tx.Query(
		ctx,
		query,
		businessID,
		receiptID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GoodsReceiptLine

	for rows.Next() {
		line, scanErr := scanReceiptLine(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, line)
	}

	return out, rows.Err()
}

func (s *GoodsReceivingService) commandReplayTx(
	ctx context.Context,
	tx pgx.Tx,
	actor GoodsReceivingActor,
	operation string,
	requestHash string,
) (bool, string, error) {
	if strings.TrimSpace(actor.CommandRef) == "" {
		return false, "", ErrPurchasingIdempotency
	}

	var existingOperation string
	var existingHash string
	var resultRef string

	err := tx.QueryRow(
		ctx,
		`
        SELECT
            operation,
            request_hash,
            goods_receipt_id
          FROM vendor_goods_receipt_commands
         WHERE business_id=$1
           AND command_ref=$2
        `,
		actor.BusinessID,
		actor.CommandRef,
	).Scan(
		&existingOperation,
		&existingHash,
		&resultRef,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}

	if existingOperation != operation ||
		existingHash != requestHash {
		return false, "", ErrPurchasingIdempotency
	}

	return true, resultRef, nil
}

func (s *GoodsReceivingService) insertCommandTx(
	ctx context.Context,
	tx pgx.Tx,
	actor GoodsReceivingActor,
	operation string,
	requestHash string,
	receipt GoodsReceipt,
) error {
	_, err := tx.Exec(
		ctx,
		`
        INSERT INTO vendor_goods_receipt_commands(
            id,
            business_id,
            command_ref,
            operation,
            request_hash,
            goods_receipt_id,
            result_version
        )
        VALUES($1,$2,$3,$4,$5,$6,$7)
        `,
		"grcmd_"+uuid.NewString(),
		actor.BusinessID,
		actor.CommandRef,
		operation,
		requestHash,
		receipt.ID,
		receipt.Version,
	)

	return err
}

func (s *GoodsReceivingService) discrepancyCountTx(
	ctx context.Context,
	tx pgx.Tx,
	businessID string,
	receiptID string,
) (int, error) {
	var count int

	err := tx.QueryRow(
		ctx,
		`
        SELECT count(*)
          FROM vendor_supplier_discrepancies
         WHERE business_id=$1
           AND goods_receipt_id=$2
           AND status <> 'VOIDED'
        `,
		businessID,
		receiptID,
	).Scan(&count)

	return count, err
}

func exactInventoryUnitCost(
	purchaseUnitCost int64,
	conversion int64,
) *int64 {
	if conversion <= 0 ||
		purchaseUnitCost < 0 ||
		purchaseUnitCost%conversion != 0 {
		return nil
	}

	value := purchaseUnitCost / conversion
	return &value
}

func (s *GoodsReceivingService) ConfirmGoodsReceipt(
	ctx context.Context,
	actor GoodsReceivingActor,
	receiptID string,
	expectedVersion int64,
) (ConfirmGoodsReceiptResult, error) {
	if s == nil ||
		s.db == nil ||
		s.stock == nil ||
		s.po == nil ||
		expectedVersion <= 0 ||
		strings.TrimSpace(actor.CommandRef) == "" {
		return ConfirmGoodsReceiptResult{},
			ErrGoodsReceiptInvalid
	}

	requestHash, err := receivingHash(
		struct {
			Operation string `json:"operation"`
			ReceiptID string `json:"receipt_id"`
			Version   int64  `json:"version"`
		}{
			Operation: "CONFIRM",
			ReceiptID: strings.TrimSpace(receiptID),
			Version:   expectedVersion,
		},
	)
	if err != nil {
		return ConfirmGoodsReceiptResult{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return ConfirmGoodsReceiptResult{}, err
	}
	defer tx.Rollback(ctx)

	replayed, replayRef, err :=
		s.commandReplayTx(
			ctx,
			tx,
			actor,
			"CONFIRM",
			requestHash,
		)
	if err != nil {
		return ConfirmGoodsReceiptResult{}, err
	}

	if replayed {
		receipt, loadErr := scanGoodsReceipt(
			tx.QueryRow(
				ctx,
				goodsReceiptSelect+
					` WHERE id=$1 AND business_id=$2`,
				replayRef,
				actor.BusinessID,
			),
		)
		if loadErr != nil {
			return ConfirmGoodsReceiptResult{}, loadErr
		}

		discrepancyCount, countErr :=
			s.discrepancyCountTx(
				ctx,
				tx,
				actor.BusinessID,
				receipt.ID,
			)
		if countErr != nil {
			return ConfirmGoodsReceiptResult{}, countErr
		}

		var costCount int
		if countErr = tx.QueryRow(
			ctx,
			`
            SELECT count(*)
              FROM vendor_purchase_cost_history
             WHERE business_id=$1
               AND goods_receipt_id=$2
               AND evidence_type='OBSERVED'
            `,
			actor.BusinessID,
			receipt.ID,
		).Scan(&costCount); countErr != nil {
			return ConfirmGoodsReceiptResult{}, countErr
		}

		if err = tx.Commit(ctx); err != nil {
			return ConfirmGoodsReceiptResult{}, err
		}

		return ConfirmGoodsReceiptResult{
			Receipt:            receipt,
			InventoryReceiptID: receipt.InventoryReceiptID,
			DiscrepancyCount:   discrepancyCount,
			CostHistoryCount:   costCount,
			Replayed:           true,
		}, nil
	}

	receipt, err := scanGoodsReceipt(
		tx.QueryRow(
			ctx,
			goodsReceiptSelect+
				` WHERE id=$1 AND business_id=$2 FOR UPDATE`,
			strings.TrimSpace(receiptID),
			actor.BusinessID,
		),
	)
	if err != nil {
		return ConfirmGoodsReceiptResult{}, err
	}

	if receipt.Version != expectedVersion {
		return ConfirmGoodsReceiptResult{},
			ErrGoodsReceiptVersion
	}

	if receipt.Status != GoodsReceiptInspected {
		return ConfirmGoodsReceiptResult{},
			ErrGoodsReceiptState
	}

	lines, err := s.loadReceiptLinesTx(
		ctx,
		tx,
		actor.BusinessID,
		receipt.ID,
		true,
	)
	if err != nil {
		return ConfirmGoodsReceiptResult{}, err
	}

	scope, err := inventory.ExactBranchScope(
		actor.BusinessID,
		receipt.BranchID,
	)
	if err != nil {
		return ConfirmGoodsReceiptResult{}, err
	}

	inventoryInput := inventory.PurchasingReceiptPostingInput{
		BusinessID: actor.BusinessID,
		BranchID:   receipt.BranchID,

		GoodsReceiptID: receipt.ID,
		SupplierRef:    receipt.SupplierID,

		ReceivedAt: receipt.ReceivedAt,
	}

	progress := map[string]int64{}

	var acceptedPurchaseTotal int64
	var acceptedInventoryTotal int64

	for _, line := range lines {
		if line.WrongItem ||
			line.AcceptedQuantity == 0 {
			continue
		}

		if line.PurchaseOrderLineID == "" ||
			line.InventoryItemID == "" ||
			line.SupplierProductID == "" ||
			line.InventoryUnitsPerPurchaseUnit <= 0 {
			return ConfirmGoodsReceiptResult{},
				ErrPurchasingExactConversion
		}

		if line.OrderedQuantitySnapshot == nil ||
			line.ReceivedBeforeSnapshot == nil {
			return ConfirmGoodsReceiptResult{},
				ErrGoodsReceiptInvalid
		}

		absolute := *line.ReceivedBeforeSnapshot

		if absolute >
			math.MaxInt64-line.AcceptedQuantity {
			return ConfirmGoodsReceiptResult{},
				ErrGoodsReceiptInvalid
		}
		absolute += line.AcceptedQuantity

		// PO progress represents fulfilment of the approved PO and therefore
		// caps at ordered quantity. Allowed excess stock still posts to
		// Inventory and is separately evidenced as OVER_DELIVERY.
		if absolute > *line.OrderedQuantitySnapshot {
			absolute = *line.OrderedQuantitySnapshot
		}

		progress[line.PurchaseOrderLineID] = absolute

		if acceptedPurchaseTotal >
			math.MaxInt64-line.AcceptedQuantity {
			return ConfirmGoodsReceiptResult{},
				ErrGoodsReceiptInvalid
		}
		acceptedPurchaseTotal += line.AcceptedQuantity

		if acceptedInventoryTotal >
			math.MaxInt64-line.InventoryQuantity {
			return ConfirmGoodsReceiptResult{},
				ErrGoodsReceiptInvalid
		}
		acceptedInventoryTotal += line.InventoryQuantity

		cost := int64(0)

		if line.ActualUnitCostMinor != nil {
			cost = *line.ActualUnitCostMinor
		} else if line.ExpectedUnitCostMinor != nil {
			cost = *line.ExpectedUnitCostMinor
		}

		inventoryInput.Lines = append(
			inventoryInput.Lines,
			inventory.PurchasingReceiptLineInput{
				ExternalLineRef: line.ID,
				InventoryItemID: line.InventoryItemID,
				Quantity:        line.InventoryQuantity,

				LotNumber:   line.LotNumber,
				BatchNumber: line.BatchNumber,

				ManufacturedAt: line.ManufacturedAt,
				BestBeforeAt:   line.BestBeforeAt,
				ExpiresAt:      line.ExpiresAt,

				InventoryUnitCostMinor: exactInventoryUnitCost(
					cost,
					line.InventoryUnitsPerPurchaseUnit,
				),
			},
		)
	}

	inventoryResult, err :=
		s.stock.PostPurchasingReceiptTx(
			ctx,
			tx,
			actor.InventoryActor(),
			scope,
			inventoryInput,
		)
	if err != nil {
		return ConfirmGoodsReceiptResult{}, err
	}

	inventoryByExternal :=
		map[string]inventory.PurchasingReceiptPostingLineResult{}

	for _, posted := range inventoryResult.Lines {
		inventoryByExternal[posted.ExternalLineRef] =
			posted
	}

	costHistoryCount := 0

	for _, line := range lines {
		if line.WrongItem ||
			line.AcceptedQuantity == 0 {
			continue
		}

		posted, ok :=
			inventoryByExternal[line.ID]
		if !ok {
			return ConfirmGoodsReceiptResult{},
				fmt.Errorf(
					"%w: accepted receipt line has no Inventory posting result",
					ErrGoodsReceiptState,
				)
		}

		if _, err = tx.Exec(
			ctx,
			`
            UPDATE vendor_goods_receipt_lines
               SET inventory_receipt_line_id=$3,
                   inventory_movement_id=$4,
                   inventory_lot_id=NULLIF($5,'')
             WHERE id=$1
               AND business_id=$2
            `,
			line.ID,
			actor.BusinessID,
			posted.InventoryReceiptLineID,
			posted.MovementID,
			posted.LotID,
		); err != nil {
			return ConfirmGoodsReceiptResult{}, err
		}

		unitCost := int64(0)

		if line.ActualUnitCostMinor != nil {
			unitCost = *line.ActualUnitCostMinor
		} else if line.ExpectedUnitCostMinor != nil {
			unitCost = *line.ExpectedUnitCostMinor
		}

		totalCost, calcErr :=
			multiplyExactInt64(
				line.AcceptedQuantity,
				unitCost,
			)
		if calcErr != nil {
			return ConfirmGoodsReceiptResult{},
				calcErr
		}

		inventoryUnitCost :=
			exactInventoryUnitCost(
				unitCost,
				line.InventoryUnitsPerPurchaseUnit,
			)

		_, err = tx.Exec(
			ctx,
			`
            INSERT INTO vendor_purchase_cost_history(
                id,
                business_id,
                branch_id,
                supplier_id,
                supplier_product_id,
                inventory_item_id,
                purchase_order_id,
                purchase_order_line_id,
                goods_receipt_id,
                goods_receipt_line_id,
                evidence_type,
                purchase_unit_code,
                inventory_units_per_purchase_unit,
                purchase_quantity,
                inventory_quantity,
                purchase_unit_cost_minor,
                total_cost_minor,
                inventory_unit_cost_minor,
                currency,
                observed_at,
                recorded_by_staff_id
            )
            VALUES(
                $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,
                'OBSERVED',$11,$12,$13,$14,$15,$16,
                $17,$18,now(),$19
            )
            `,
			"pch_"+uuid.NewString(),
			actor.BusinessID,
			receipt.BranchID,
			receipt.SupplierID,
			line.SupplierProductID,
			line.InventoryItemID,
			receipt.PurchaseOrderID,
			line.PurchaseOrderLineID,
			receipt.ID,
			line.ID,
			line.PurchaseUnitCode,
			line.InventoryUnitsPerPurchaseUnit,
			line.AcceptedQuantity,
			line.InventoryQuantity,
			unitCost,
			totalCost,
			inventoryUnitCost,
			receipt.Currency,
			actor.StaffID,
		)
		if err != nil {
			return ConfirmGoodsReceiptResult{}, err
		}

		costHistoryCount++

		// SupplierProduct last_cost is current purchasing metadata only.
		// It is not a Wallet balance, accounting posting, or rewrite of the
		// immutable cost-history evidence above.
		if _, err = tx.Exec(
			ctx,
			`
            UPDATE vendor_supplier_products
               SET last_cost_minor=$3,
                   updated_at=now(),
                   version=version+1
             WHERE id=$1
               AND business_id=$2
            `,
			line.SupplierProductID,
			actor.BusinessID,
			unitCost,
		); err != nil {
			return ConfirmGoodsReceiptResult{}, err
		}

		if err = s.publishTx(
			ctx,
			tx,
			actor,
			"vendor.purchasing.purchase_cost.observed",
			"vendor_purchase_cost_history",
			line.ID,
			1,
			map[string]any{
				"goods_receipt_id":      receipt.ID,
				"goods_receipt_line_id": line.ID,
				"supplier_product_id":   line.SupplierProductID,
				"inventory_item_id":     line.InventoryItemID,
				"purchase_quantity":     line.AcceptedQuantity,
				"inventory_quantity":    line.InventoryQuantity,
				"currency":              receipt.Currency,
			},
		); err != nil {
			return ConfirmGoodsReceiptResult{}, err
		}
	}

	if len(progress) > 0 {
		if err = s.applyPOReceivingProgressTx(
			ctx,
			tx,
			actor,
			receipt.PurchaseOrderID,
			progress,
		); err != nil {
			return ConfirmGoodsReceiptResult{}, err
		}
	}

	now := time.Now().UTC()

	err = tx.QueryRow(
		ctx,
		`
        UPDATE vendor_goods_receipts
           SET status='POSTED',
               inventory_receipt_id=NULLIF($3,''),
               posted_at=$4,
               updated_at=now(),
               version=version+1
         WHERE id=$1
           AND business_id=$2
           AND status='INSPECTED'
           AND version=$5
        RETURNING
            status,
            COALESCE(inventory_receipt_id,''),
            posted_at,
            updated_at,
            version
        `,
		receipt.ID,
		actor.BusinessID,
		inventoryResult.InventoryReceiptID,
		now,
		expectedVersion,
	).Scan(
		&receipt.Status,
		&receipt.InventoryReceiptID,
		&receipt.PostedAt,
		&receipt.UpdatedAt,
		&receipt.Version,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ConfirmGoodsReceiptResult{},
			ErrGoodsReceiptVersion
	}
	if err != nil {
		return ConfirmGoodsReceiptResult{}, err
	}

	discrepancyCount, err :=
		s.discrepancyCountTx(
			ctx,
			tx,
			actor.BusinessID,
			receipt.ID,
		)
	if err != nil {
		return ConfirmGoodsReceiptResult{}, err
	}

	if err = s.auditTx(
		ctx,
		tx,
		actor,
		"vendor.purchasing.goods_receipt.posted",
		"vendor_goods_receipt",
		receipt.ID,
		receipt.BranchID,
		map[string]any{"status": GoodsReceiptInspected},
		map[string]any{
			"status":                      GoodsReceiptPosted,
			"accepted_purchase_quantity":  acceptedPurchaseTotal,
			"accepted_inventory_quantity": acceptedInventoryTotal,
			"inventory_receipt_id":        receipt.InventoryReceiptID,
		},
		"",
	); err != nil {
		return ConfirmGoodsReceiptResult{}, err
	}

	if err = s.publishTx(
		ctx,
		tx,
		actor,
		"vendor.purchasing.goods_receipt.posted",
		"vendor_goods_receipt",
		receipt.ID,
		receipt.Version,
		map[string]any{
			"goods_receipt_id":            receipt.ID,
			"purchase_order_id":           receipt.PurchaseOrderID,
			"branch_id":                   receipt.BranchID,
			"status":                      receipt.Status,
			"accepted_purchase_quantity":  acceptedPurchaseTotal,
			"accepted_inventory_quantity": acceptedInventoryTotal,
			"inventory_receipt_id":        receipt.InventoryReceiptID,
			"discrepancy_count":           discrepancyCount,
		},
	); err != nil {
		return ConfirmGoodsReceiptResult{}, err
	}

	if err = s.insertCommandTx(
		ctx,
		tx,
		actor,
		"CONFIRM",
		requestHash,
		receipt,
	); err != nil {
		return ConfirmGoodsReceiptResult{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return ConfirmGoodsReceiptResult{}, err
	}

	return ConfirmGoodsReceiptResult{
		Receipt: receipt,

		InventoryReceiptID: receipt.InventoryReceiptID,

		AcceptedPurchaseQuantity:  acceptedPurchaseTotal,
		AcceptedInventoryQuantity: acceptedInventoryTotal,

		DiscrepancyCount: discrepancyCount,
		CostHistoryCount: costHistoryCount,
	}, nil
}

// reconcilePOReceivingAfterReversalTx is intentionally separate from 15B's
// monotonic ApplyReceivingProgressTx. Reversal is the one controlled path that
// may reduce receipt-derived progress. It recomputes from authoritative,
//
//	POSTED GoodsReceipts
//
// rather than trusting a client-supplied decrement.
func (s *GoodsReceivingService) reconcilePOReceivingAfterReversalTx(
	ctx context.Context,
	tx pgx.Tx,
	actor GoodsReceivingActor,
	purchaseOrderID string,
) error {
	type lineState struct {
		id       string
		ordered  int64
		received int64
	}

	rows, err := tx.Query(
		ctx,
		`
        SELECT
            id,
            purchase_quantity
          FROM vendor_purchase_order_lines
         WHERE business_id=$1
           AND purchase_order_id=$2
         ORDER BY id
         FOR UPDATE
        `,
		actor.BusinessID,
		purchaseOrderID,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	var states []lineState

	// Read the line set to completion before querying anything else. A
	// connection carrying an open result set cannot also run a statement, so
	// asking for each line's received quantity inside this loop fails with
	// "conn busy" and no reversal can ever reconcile the purchase order.
	for rows.Next() {
		var state lineState

		if err = rows.Scan(
			&state.id,
			&state.ordered,
		); err != nil {
			return err
		}

		states = append(states, state)
	}

	if err = rows.Err(); err != nil {
		return err
	}

	rows.Close()

	for index := range states {
		if err = tx.QueryRow(
			ctx,
			`
            SELECT COALESCE(SUM(l.accepted_quantity),0)
              FROM vendor_goods_receipt_lines l
              JOIN vendor_goods_receipts r
                ON r.id=l.goods_receipt_id
               AND r.business_id=l.business_id
             WHERE l.business_id=$1
               AND l.purchase_order_id=$2
               AND l.purchase_order_line_id=$3
               AND r.status='POSTED'
            `,
			actor.BusinessID,
			purchaseOrderID,
			states[index].id,
		).Scan(&states[index].received); err != nil {
			return err
		}

		if states[index].received > states[index].ordered {
			// Allowed over-delivery never turns PO progress beyond the approved
			// ordered quantity.
			states[index].received = states[index].ordered
		}
	}

	allReceived := len(states) > 0
	anyReceived := false

	for _, state := range states {
		if state.received > 0 {
			anyReceived = true
		}

		if state.received < state.ordered {
			allReceived = false
		}

		tag, updateErr := tx.Exec(
			ctx,
			`
            UPDATE vendor_purchase_order_lines
               SET received_purchase_quantity=$4,
                   updated_at=now(),
                   version=version+1
             WHERE id=$1
               AND business_id=$2
               AND purchase_order_id=$3
            `,
			state.id,
			actor.BusinessID,
			purchaseOrderID,
			state.received,
		)
		if updateErr != nil {
			return updateErr
		}
		if tag.RowsAffected() != 1 {
			return ErrGoodsReceiptState
		}
	}

	status := "ORDERED"

	if allReceived {
		status = "RECEIVED"
	} else if anyReceived {
		status = "PARTIALLY_RECEIVED"
	}

	tag, err := tx.Exec(
		ctx,
		`
        UPDATE vendor_purchase_orders
           SET status=$3,
               updated_at=now(),
               version=version+1
         WHERE id=$1
           AND business_id=$2
           AND status IN (
               'ORDERED',
               'PARTIALLY_RECEIVED',
               'RECEIVED'
           )
        `,
		purchaseOrderID,
		actor.BusinessID,
		status,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrGoodsReceiptState
	}

	return nil
}

func (s *GoodsReceivingService) ReverseGoodsReceipt(
	ctx context.Context,
	actor GoodsReceivingActor,
	receiptID string,
	expectedVersion int64,
	reason string,
) (GoodsReceipt, error) {
	reason = strings.TrimSpace(reason)

	if s == nil ||
		s.db == nil ||
		s.stock == nil ||
		expectedVersion <= 0 ||
		reason == "" ||
		len(reason) > 2000 ||
		strings.TrimSpace(actor.CommandRef) == "" {
		return GoodsReceipt{}, ErrGoodsReceiptInvalid
	}

	requestHash, err := receivingHash(
		struct {
			Operation string `json:"operation"`
			ReceiptID string `json:"receipt_id"`
			Version   int64  `json:"version"`
			Reason    string `json:"reason"`
		}{
			Operation: "REVERSE",
			ReceiptID: strings.TrimSpace(receiptID),
			Version:   expectedVersion,
			Reason:    reason,
		},
	)
	if err != nil {
		return GoodsReceipt{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return GoodsReceipt{}, err
	}
	defer tx.Rollback(ctx)

	replayed, replayRef, err :=
		s.commandReplayTx(
			ctx,
			tx,
			actor,
			"REVERSE",
			requestHash,
		)
	if err != nil {
		return GoodsReceipt{}, err
	}

	if replayed {
		receipt, loadErr := scanGoodsReceipt(
			tx.QueryRow(
				ctx,
				goodsReceiptSelect+
					` WHERE id=$1 AND business_id=$2`,
				replayRef,
				actor.BusinessID,
			),
		)
		if loadErr != nil {
			return GoodsReceipt{}, loadErr
		}

		if err = tx.Commit(ctx); err != nil {
			return GoodsReceipt{}, err
		}

		return receipt, nil
	}

	receipt, err := scanGoodsReceipt(
		tx.QueryRow(
			ctx,
			goodsReceiptSelect+
				` WHERE id=$1 AND business_id=$2 FOR UPDATE`,
			strings.TrimSpace(receiptID),
			actor.BusinessID,
		),
	)
	if err != nil {
		return GoodsReceipt{}, err
	}

	if receipt.Version != expectedVersion {
		return GoodsReceipt{}, ErrGoodsReceiptVersion
	}

	if receipt.Status != GoodsReceiptPosted {
		return GoodsReceipt{}, ErrGoodsReceiptState
	}

	var poStatus string

	if err = tx.QueryRow(
		ctx,
		`
        SELECT status
          FROM vendor_purchase_orders
         WHERE id=$1
           AND business_id=$2
         FOR UPDATE
        `,
		receipt.PurchaseOrderID,
		actor.BusinessID,
	).Scan(&poStatus); err != nil {
		return GoodsReceipt{}, err
	}

	if poStatus == "CLOSED" ||
		poStatus == "CANCELLED" {
		return GoodsReceipt{}, ErrGoodsReceiptState
	}

	scope, err := inventory.ExactBranchScope(
		actor.BusinessID,
		receipt.BranchID,
	)
	if err != nil {
		return GoodsReceipt{}, err
	}

	err = s.stock.ReversePurchasingReceiptTx(
		ctx,
		tx,
		actor.InventoryActor(),
		scope,
		actor.BusinessID,
		receipt.BranchID,
		receipt.ID,
	)
	if err != nil {
		return GoodsReceipt{},
			fmt.Errorf(
				"%w: %v",
				ErrPurchasingReversalUnsafe,
				err,
			)
	}

	lines, err := s.loadReceiptLinesTx(
		ctx,
		tx,
		actor.BusinessID,
		receipt.ID,
		true,
	)
	if err != nil {
		return GoodsReceipt{}, err
	}

	for _, line := range lines {
		if line.WrongItem ||
			line.AcceptedQuantity <= 0 ||
			line.SupplierProductID == "" {
			continue
		}

		unitCost := int64(0)

		if line.ActualUnitCostMinor != nil {
			unitCost = *line.ActualUnitCostMinor
		} else if line.ExpectedUnitCostMinor != nil {
			unitCost = *line.ExpectedUnitCostMinor
		}

		totalCost, calcErr :=
			multiplyExactInt64(
				line.AcceptedQuantity,
				unitCost,
			)
		if calcErr != nil {
			return GoodsReceipt{}, calcErr
		}

		_, err = tx.Exec(
			ctx,
			`
            INSERT INTO vendor_purchase_cost_history(
                id,
                business_id,
                branch_id,
                supplier_id,
                supplier_product_id,
                inventory_item_id,
                purchase_order_id,
                purchase_order_line_id,
                goods_receipt_id,
                goods_receipt_line_id,
                evidence_type,
                purchase_unit_code,
                inventory_units_per_purchase_unit,
                purchase_quantity,
                inventory_quantity,
                purchase_unit_cost_minor,
                total_cost_minor,
                inventory_unit_cost_minor,
                currency,
                observed_at,
                recorded_by_staff_id
            )
            VALUES(
                $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,
                'REVERSAL',$11,$12,$13,$14,$15,$16,
                $17,$18,now(),$19
            )
            `,
			"pch_"+uuid.NewString(),
			actor.BusinessID,
			receipt.BranchID,
			receipt.SupplierID,
			line.SupplierProductID,
			line.InventoryItemID,
			receipt.PurchaseOrderID,
			line.PurchaseOrderLineID,
			receipt.ID,
			line.ID,
			line.PurchaseUnitCode,
			line.InventoryUnitsPerPurchaseUnit,
			line.AcceptedQuantity,
			line.InventoryQuantity,
			unitCost,
			totalCost,
			exactInventoryUnitCost(
				unitCost,
				line.InventoryUnitsPerPurchaseUnit,
			),
			receipt.Currency,
			actor.StaffID,
		)
		if err != nil {
			return GoodsReceipt{}, err
		}
	}

	if err = s.reconcilePOReceivingAfterReversalTx(
		ctx,
		tx,
		actor,
		receipt.PurchaseOrderID,
	); err != nil {
		return GoodsReceipt{}, err
	}

	// Unresolved projections from a reversed receipt are no longer actionable,
	// but their audit/cost evidence remains.
	if _, err = tx.Exec(
		ctx,
		`
        UPDATE vendor_supplier_discrepancies
           SET status='VOIDED',
               updated_at=now(),
               version=version+1
         WHERE business_id=$1
           AND goods_receipt_id=$2
           AND status IN ('OPEN','APPROVAL_REQUIRED')
        `,
		actor.BusinessID,
		receipt.ID,
	); err != nil {
		return GoodsReceipt{}, err
	}

	now := time.Now().UTC()

	err = tx.QueryRow(
		ctx,
		`
        UPDATE vendor_goods_receipts
           SET status='REVERSED',
               reversed_at=$3,
               reversed_by_staff_id=$4,
               reversal_reason=$5,
               updated_at=now(),
               version=version+1
         WHERE id=$1
           AND business_id=$2
           AND status='POSTED'
           AND version=$6
        RETURNING
            status,
            reversed_at,
            reversed_by_staff_id,
            reversal_reason,
            updated_at,
            version
        `,
		receipt.ID,
		actor.BusinessID,
		now,
		actor.StaffID,
		reason,
		expectedVersion,
	).Scan(
		&receipt.Status,
		&receipt.ReversedAt,
		&receipt.ReversedByStaffID,
		&receipt.ReversalReason,
		&receipt.UpdatedAt,
		&receipt.Version,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return GoodsReceipt{}, ErrGoodsReceiptVersion
	}
	if err != nil {
		return GoodsReceipt{}, err
	}

	if err = s.auditTx(
		ctx,
		tx,
		actor,
		"vendor.purchasing.goods_receipt.reversed",
		"vendor_goods_receipt",
		receipt.ID,
		receipt.BranchID,
		map[string]any{"status": GoodsReceiptPosted},
		map[string]any{"status": GoodsReceiptReversed},
		reason,
	); err != nil {
		return GoodsReceipt{}, err
	}

	if err = s.publishTx(
		ctx,
		tx,
		actor,
		"vendor.purchasing.goods_receipt.reversed",
		"vendor_goods_receipt",
		receipt.ID,
		receipt.Version,
		map[string]any{
			"goods_receipt_id":  receipt.ID,
			"purchase_order_id": receipt.PurchaseOrderID,
			"branch_id":         receipt.BranchID,
			"status":            receipt.Status,
		},
	); err != nil {
		return GoodsReceipt{}, err
	}

	if err = s.insertCommandTx(
		ctx,
		tx,
		actor,
		"REVERSE",
		requestHash,
		receipt,
	); err != nil {
		return GoodsReceipt{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return GoodsReceipt{}, err
	}

	return receipt, nil
}

func (s *GoodsReceivingService) GetGoodsReceipt(
	ctx context.Context,
	actor GoodsReceivingActor,
	receiptID string,
) (GoodsReceipt, error) {
	receipt, err := scanGoodsReceipt(
		s.db.QueryRow(
			ctx,
			goodsReceiptSelect+
				` WHERE id=$1 AND business_id=$2`,
			strings.TrimSpace(receiptID),
			actor.BusinessID,
		),
	)
	if err != nil {
		return GoodsReceipt{}, err
	}

	rows, err := s.db.Query(
		ctx,
		goodsReceiptLineSelect+
			`
            WHERE business_id=$1
              AND goods_receipt_id=$2
            ORDER BY id
            `,
		actor.BusinessID,
		receipt.ID,
	)
	if err != nil {
		return GoodsReceipt{}, err
	}
	defer rows.Close()

	for rows.Next() {
		line, scanErr := scanReceiptLine(rows)
		if scanErr != nil {
			return GoodsReceipt{}, scanErr
		}
		receipt.Lines = append(receipt.Lines, line)
	}

	return receipt, rows.Err()
}

func (s *GoodsReceivingService) ListGoodsReceipts(
	ctx context.Context,
	actor GoodsReceivingActor,
	branchID string,
	status string,
	limit int,
) ([]GoodsReceipt, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	status = strings.TrimSpace(status)

	rows, err := s.db.Query(
		ctx,
		goodsReceiptSelect+
			`
            WHERE business_id=$1
              AND branch_id=$2
              AND ($3='' OR status=$3)
            ORDER BY received_at DESC,id DESC
            LIMIT $4
            `,
		actor.BusinessID,
		strings.TrimSpace(branchID),
		status,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GoodsReceipt

	for rows.Next() {
		receipt, scanErr := scanGoodsReceipt(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, receipt)
	}

	return out, rows.Err()
}

func (s *GoodsReceivingService) AttachGoodsReceiptDocument(
	ctx context.Context,
	actor GoodsReceivingActor,
	receiptID string,
	input AttachGoodsReceiptDocumentInput,
) (GoodsReceiptDocument, error) {
	input.DocumentType =
		strings.ToUpper(
			strings.TrimSpace(input.DocumentType),
		)
	input.MediaRef = strings.TrimSpace(input.MediaRef)
	input.ExternalReference =
		strings.TrimSpace(input.ExternalReference)

	switch input.DocumentType {
	case "DELIVERY_NOTE",
		"INVOICE",
		"PACKING_LIST",
		"QUALITY_CERTIFICATE",
		"OTHER":
	default:
		return GoodsReceiptDocument{},
			ErrGoodsReceiptInvalid
	}

	if input.MediaRef == "" ||
		len(input.MediaRef) > 300 {
		return GoodsReceiptDocument{},
			ErrGoodsReceiptInvalid
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return GoodsReceiptDocument{}, err
	}
	defer tx.Rollback(ctx)

	receipt, err := scanGoodsReceipt(
		tx.QueryRow(
			ctx,
			goodsReceiptSelect+
				` WHERE id=$1 AND business_id=$2 FOR UPDATE`,
			receiptID,
			actor.BusinessID,
		),
	)
	if err != nil {
		return GoodsReceiptDocument{}, err
	}

	if receipt.Status == GoodsReceiptCancelled {
		return GoodsReceiptDocument{},
			ErrGoodsReceiptState
	}

	out := GoodsReceiptDocument{
		ID:                "grdoc_" + uuid.NewString(),
		GoodsReceiptID:    receipt.ID,
		DocumentType:      input.DocumentType,
		MediaRef:          input.MediaRef,
		ExternalReference: input.ExternalReference,
		AttachedByStaffID: actor.StaffID,
	}

	err = tx.QueryRow(
		ctx,
		`
        INSERT INTO vendor_goods_receipt_documents(
            id,
            business_id,
            goods_receipt_id,
            document_type,
            media_ref,
            external_reference,
            attached_by_staff_id
        )
        VALUES(
            $1,$2,$3,$4,$5,NULLIF($6,''),$7
        )
        RETURNING created_at
        `,
		out.ID,
		actor.BusinessID,
		receipt.ID,
		out.DocumentType,
		out.MediaRef,
		out.ExternalReference,
		actor.StaffID,
	).Scan(&out.CreatedAt)
	if err != nil {
		return GoodsReceiptDocument{}, err
	}

	if err = s.auditTx(
		ctx,
		tx,
		actor,
		"vendor.purchasing.goods_receipt.document_attached",
		"vendor_goods_receipt_document",
		out.ID,
		receipt.BranchID,
		nil,
		map[string]any{
			"goods_receipt_id": receipt.ID,
			"document_type":    out.DocumentType,
			"media_ref":        out.MediaRef,
		},
		"",
	); err != nil {
		return GoodsReceiptDocument{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return GoodsReceiptDocument{}, err
	}

	return out, nil
}

func (s *GoodsReceivingService) ListGoodsReceiptDocuments(
	ctx context.Context,
	actor GoodsReceivingActor,
	receiptID string,
) ([]GoodsReceiptDocument, error) {
	rows, err := s.db.Query(
		ctx,
		`
        SELECT
            id,
            goods_receipt_id,
            document_type,
            media_ref,
            COALESCE(external_reference,''),
            attached_by_staff_id,
            created_at
          FROM vendor_goods_receipt_documents
         WHERE business_id=$1
           AND goods_receipt_id=$2
         ORDER BY created_at DESC,id DESC
        `,
		actor.BusinessID,
		receiptID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GoodsReceiptDocument

	for rows.Next() {
		var item GoodsReceiptDocument

		if err = rows.Scan(
			&item.ID,
			&item.GoodsReceiptID,
			&item.DocumentType,
			&item.MediaRef,
			&item.ExternalReference,
			&item.AttachedByStaffID,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}

		out = append(out, item)
	}

	return out, rows.Err()
}

func scanDiscrepancy(
	row pgx.Row,
) (SupplierDiscrepancy, error) {
	var out SupplierDiscrepancy

	err := row.Scan(
		&out.ID,
		&out.SupplierID,
		&out.PurchaseOrderID,
		&out.PurchaseOrderLineID,
		&out.GoodsReceiptID,
		&out.GoodsReceiptLineID,
		&out.BranchID,
		&out.Type,
		&out.Status,
		&out.Quantity,
		&out.ExpectedCostMinor,
		&out.ActualCostMinor,
		&out.VarianceMinor,
		&out.VarianceBps,
		&out.Currency,
		&out.ReasonCode,
		&out.Notes,
		&out.OpenedByStaffID,
		&out.DecidedByStaffID,
		&out.ResolvedByStaffID,
		&out.DecidedAt,
		&out.ResolvedAt,
		&out.Version,
		&out.CreatedAt,
		&out.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return SupplierDiscrepancy{},
			ErrSupplierDiscrepancyNotFound
	}

	return out, err
}

const discrepancySelect = `
SELECT
    id,
    supplier_id,
    purchase_order_id,
    COALESCE(purchase_order_line_id,''),
    goods_receipt_id,
    COALESCE(goods_receipt_line_id,''),
    branch_id,
    discrepancy_type,
    status,
    quantity,
    expected_cost_minor,
    actual_cost_minor,
    variance_minor,
    variance_bps,
    COALESCE(currency,''),
    COALESCE(reason_code,''),
    notes,
    opened_by_staff_id,
    COALESCE(decided_by_staff_id,''),
    COALESCE(resolved_by_staff_id,''),
    decided_at,
    resolved_at,
    version,
    created_at,
    updated_at
FROM vendor_supplier_discrepancies
`

func (s *GoodsReceivingService) GetDiscrepancy(
	ctx context.Context,
	actor GoodsReceivingActor,
	id string,
) (SupplierDiscrepancy, error) {
	return scanDiscrepancy(
		s.db.QueryRow(
			ctx,
			discrepancySelect+
				` WHERE id=$1 AND business_id=$2`,
			id,
			actor.BusinessID,
		),
	)
}

func (s *GoodsReceivingService) ListDiscrepancies(
	ctx context.Context,
	actor GoodsReceivingActor,
	branchID string,
	status string,
	discrepancyType string,
	limit int,
) ([]SupplierDiscrepancy, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	rows, err := s.db.Query(
		ctx,
		discrepancySelect+
			`
            WHERE business_id=$1
              AND branch_id=$2
              AND ($3='' OR status=$3)
              AND ($4='' OR discrepancy_type=$4)
            ORDER BY created_at DESC,id DESC
            LIMIT $5
            `,
		actor.BusinessID,
		branchID,
		strings.TrimSpace(status),
		strings.TrimSpace(discrepancyType),
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SupplierDiscrepancy

	for rows.Next() {
		item, scanErr := scanDiscrepancy(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, item)
	}

	return out, rows.Err()
}

func (s *GoodsReceivingService) TransitionDiscrepancy(
	ctx context.Context,
	actor GoodsReceivingActor,
	id string,
	expectedVersion int64,
	target string,
	notes string,
) (SupplierDiscrepancy, error) {
	if expectedVersion <= 0 {
		return SupplierDiscrepancy{},
			ErrGoodsReceiptInvalid
	}

	target = strings.ToUpper(strings.TrimSpace(target))
	notes = strings.TrimSpace(notes)

	switch target {
	case DiscrepancyApproved,
		DiscrepancyRejectedStatus,
		DiscrepancyResolved:
	default:
		return SupplierDiscrepancy{},
			ErrGoodsReceiptInvalid
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return SupplierDiscrepancy{}, err
	}
	defer tx.Rollback(ctx)

	current, err := scanDiscrepancy(
		tx.QueryRow(
			ctx,
			discrepancySelect+
				` WHERE id=$1 AND business_id=$2 FOR UPDATE`,
			id,
			actor.BusinessID,
		),
	)
	if err != nil {
		return SupplierDiscrepancy{}, err
	}

	if current.Version != expectedVersion {
		return SupplierDiscrepancy{},
			ErrGoodsReceiptVersion
	}

	switch target {
	case DiscrepancyApproved,
		DiscrepancyRejectedStatus:
		if current.Status != DiscrepancyApprovalRequired &&
			current.Status != DiscrepancyOpen {
			return SupplierDiscrepancy{},
				ErrSupplierDiscrepancyState
		}

	case DiscrepancyResolved:
		if current.Status != DiscrepancyOpen &&
			current.Status != DiscrepancyApproved &&
			current.Status != DiscrepancyRejectedStatus {
			return SupplierDiscrepancy{},
				ErrSupplierDiscrepancyState
		}
	}

	var out SupplierDiscrepancy

	// Explicit UPDATE + reload keeps each state transition easy to audit.
	if target == DiscrepancyResolved {
		tag, updateErr := tx.Exec(
			ctx,
			`
            UPDATE vendor_supplier_discrepancies
               SET status='RESOLVED',
                   resolved_by_staff_id=$3,
                   resolved_at=now(),
                   notes=CASE
                       WHEN $4='' THEN notes
                       ELSE $4
                   END,
                   version=version+1,
                   updated_at=now()
             WHERE id=$1
               AND business_id=$2
               AND version=$5
            `,
			id,
			actor.BusinessID,
			actor.StaffID,
			notes,
			expectedVersion,
		)
		if updateErr != nil {
			return SupplierDiscrepancy{}, updateErr
		}
		if tag.RowsAffected() != 1 {
			return SupplierDiscrepancy{},
				ErrGoodsReceiptVersion
		}
	} else {
		tag, updateErr := tx.Exec(
			ctx,
			`
            UPDATE vendor_supplier_discrepancies
               SET status=$3,
                   decided_by_staff_id=$4,
                   decided_at=now(),
                   notes=CASE
                       WHEN $5='' THEN notes
                       ELSE $5
                   END,
                   version=version+1,
                   updated_at=now()
             WHERE id=$1
               AND business_id=$2
               AND version=$6
            `,
			id,
			actor.BusinessID,
			target,
			actor.StaffID,
			notes,
			expectedVersion,
		)
		if updateErr != nil {
			return SupplierDiscrepancy{}, updateErr
		}
		if tag.RowsAffected() != 1 {
			return SupplierDiscrepancy{},
				ErrGoodsReceiptVersion
		}
	}

	out, err = scanDiscrepancy(
		tx.QueryRow(
			ctx,
			discrepancySelect+
				` WHERE id=$1 AND business_id=$2`,
			id,
			actor.BusinessID,
		),
	)
	if err != nil {
		return SupplierDiscrepancy{}, err
	}

	if err = s.auditTx(
		ctx,
		tx,
		actor,
		"vendor.purchasing.supplier_discrepancy."+
			strings.ToLower(target),
		"vendor_supplier_discrepancy",
		out.ID,
		out.BranchID,
		current,
		out,
		notes,
	); err != nil {
		return SupplierDiscrepancy{}, err
	}

	if err = s.publishTx(
		ctx,
		tx,
		actor,
		"vendor.purchasing.supplier_discrepancy."+
			strings.ToLower(target),
		"vendor_supplier_discrepancy",
		out.ID,
		out.Version,
		map[string]any{
			"discrepancy_id":   out.ID,
			"goods_receipt_id": out.GoodsReceiptID,
			"type":             out.Type,
			"status":           out.Status,
			"branch_id":        out.BranchID,
		},
	); err != nil {
		return SupplierDiscrepancy{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return SupplierDiscrepancy{}, err
	}

	return out, nil
}

func (s *GoodsReceivingService) ListPurchaseCostHistory(
	ctx context.Context,
	actor GoodsReceivingActor,
	branchID string,
	inventoryItemID string,
	limit int,
) ([]PurchaseCostHistory, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	rows, err := s.db.Query(
		ctx,
		`
        SELECT
            id,
            branch_id,
            supplier_id,
            supplier_product_id,
            inventory_item_id,
            purchase_order_id,
            purchase_order_line_id,
            goods_receipt_id,
            goods_receipt_line_id,
            evidence_type,
            purchase_unit_code,
            inventory_units_per_purchase_unit,
            purchase_quantity,
            inventory_quantity,
            purchase_unit_cost_minor,
            total_cost_minor,
            inventory_unit_cost_minor,
            currency,
            observed_at,
            created_at
          FROM vendor_purchase_cost_history
         WHERE business_id=$1
           AND branch_id=$2
           AND ($3='' OR inventory_item_id=$3)
         ORDER BY observed_at DESC,id DESC
         LIMIT $4
        `,
		actor.BusinessID,
		strings.TrimSpace(branchID),
		strings.TrimSpace(inventoryItemID),
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PurchaseCostHistory

	for rows.Next() {
		var item PurchaseCostHistory

		if err = rows.Scan(
			&item.ID,
			&item.BranchID,
			&item.SupplierID,
			&item.SupplierProductID,
			&item.InventoryItemID,
			&item.PurchaseOrderID,
			&item.PurchaseOrderLineID,
			&item.GoodsReceiptID,
			&item.GoodsReceiptLineID,
			&item.EvidenceType,
			&item.PurchaseUnitCode,
			&item.InventoryUnitsPerPurchaseUnit,
			&item.PurchaseQuantity,
			&item.InventoryQuantity,
			&item.PurchaseUnitCostMinor,
			&item.TotalCostMinor,
			&item.InventoryUnitCostMinor,
			&item.Currency,
			&item.ObservedAt,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}

		out = append(out, item)
	}

	return out, rows.Err()
}

// compile-time usage guards for imports used by deterministic ordering in
// certification extensions. Keeping sorting here also documents that receipt
// progress is always emitted deterministically.
var _ = sort.Strings
