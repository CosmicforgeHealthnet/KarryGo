package shop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

const (
	PermInventoryRead    = "vendor.shop.inventory.read"
	PermInventoryReceive = "vendor.shop.inventory.receive"
	PermInventoryReserve = "vendor.shop.inventory.reserve"

	CodeStockBasisNotFound         = "SHOP_STOCK_BASIS_NOT_FOUND"
	CodeInventoryInsufficient      = "SHOP_INSUFFICIENT_STOCK"
	CodeInventoryConversionMissing = "INVENTORY_CONVERSION_UNRESOLVED"
	CodeReservationNotFound        = "SHOP_RESERVATION_NOT_FOUND"
	CodeReservationStateConflict   = "SHOP_RESERVATION_STATE_CONFLICT"
	CodeReservationFingerprint     = "SHOP_RESERVATION_FINGERPRINT_CONFLICT"
)

const MaxInventoryQuantityPerLine int64 = 1_000_000_000

const (
	ConsumptionSameUnit  = "SAME_UNIT"
	ConsumptionExact     = "EXACT_CONVERSION"
	ConsumptionVariable  = "VARIABLE_FULFILMENT"
	AllocationFIFO       = "FIFO"
	AllocationFEFO       = "FEFO"
	ReservationHeld      = "HELD"
	ReservationConfirmed = "CONFIRMED"
	ReservationCommitted = "COMMITTED"
	ReservationReleased  = "RELEASED"
	ReservationCancelled = "CANCELLED"
	ReservationExpired   = "EXPIRED"
)

// InventoryStockBasis defines the integer unit in which one branch/variant is
// authoritative. It deliberately does not contain a calibrated approximation.
type InventoryStockBasis struct {
	ID                 string    `json:"id"`
	BranchRef          string    `json:"branch_ref"`
	ListingRef         string    `json:"listing_ref"`
	ProductRef         string    `json:"product_ref"`
	VariantRef         string    `json:"variant_ref"`
	MeasureRef         string    `json:"measure_ref"`
	ConsumptionMode    string    `json:"consumption_mode"`
	AllocationPolicy   string    `json:"allocation_policy"`
	LotTrackingEnabled bool      `json:"lot_tracking_enabled"`
	Status             string    `json:"status"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	Version            int64     `json:"version"`
}

type ConfigureStockBasisInput struct {
	ListingID        string `json:"listing_id"`
	MeasureID        string `json:"measure_id"`
	ConsumptionMode  string `json:"consumption_mode"`
	AllocationPolicy string `json:"allocation_policy"`
	LotTracking      bool   `json:"lot_tracking"`
	ExpectedVersion  int64  `json:"expected_version"`
}

// InventoryBalance is the materialized operational authority. Available is
// derived and never persisted independently.
type InventoryBalance struct {
	StockBasisRef       string    `json:"stock_basis_ref"`
	BranchRef           string    `json:"branch_ref"`
	VariantRef          string    `json:"variant_ref"`
	OnHandQuantity      int64     `json:"on_hand_quantity"`
	ReservedQuantity    int64     `json:"reserved_quantity"`
	QuarantinedQuantity int64     `json:"quarantined_quantity"`
	InTransitQuantity   int64     `json:"in_transit_quantity"`
	BackorderQuantity   int64     `json:"backorder_quantity"`
	AvailableQuantity   int64     `json:"available_quantity"`
	Revision            int64     `json:"revision"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type ReceiptLineInput struct {
	StockBasisID   string     `json:"stock_basis_id"`
	Quantity       int64      `json:"quantity"`
	LotNumber      string     `json:"lot_number"`
	BatchNumber    string     `json:"batch_number"`
	ManufacturedAt *time.Time `json:"manufactured_at"`
	BestBeforeAt   *time.Time `json:"best_before_at"`
	ExpiresAt      *time.Time `json:"expires_at"`
	UnitCostMinor  *int64     `json:"unit_cost_minor"`
}

type CreateReceiptInput struct {
	BranchID    string             `json:"branch_id"`
	SourceRef   string             `json:"source_ref"`
	SupplierRef string             `json:"supplier_ref"`
	Notes       string             `json:"notes"`
	ReceivedAt  time.Time          `json:"received_at"`
	Lines       []ReceiptLineInput `json:"lines"`
}

type InventoryReceipt struct {
	ID         string                 `json:"id"`
	BranchRef  string                 `json:"branch_ref"`
	SourceRef  string                 `json:"source_ref,omitempty"`
	ReceivedAt time.Time              `json:"received_at"`
	Status     string                 `json:"status"`
	PostedAt   *time.Time             `json:"posted_at,omitempty"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
	Version    int64                  `json:"version"`
	Lines      []InventoryReceiptLine `json:"lines,omitempty"`
}

type InventoryReceiptLine struct {
	ID             string     `json:"id"`
	StockBasisRef  string     `json:"stock_basis_ref"`
	VariantRef     string     `json:"variant_ref"`
	Quantity       int64      `json:"quantity"`
	LotNumber      string     `json:"lot_number,omitempty"`
	BatchNumber    string     `json:"batch_number,omitempty"`
	ManufacturedAt *time.Time `json:"manufactured_at,omitempty"`
	BestBeforeAt   *time.Time `json:"best_before_at,omitempty"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	LotRef         string     `json:"lot_ref,omitempty"`
}

type ReserveInventoryLineInput struct {
	StockBasisID string `json:"stock_basis_id"`
	Quantity     int64  `json:"quantity"`
}

type ReserveInventoryInput struct {
	SourceType string                      `json:"source_type"`
	SourceRef  string                      `json:"source_ref"`
	ExpiresAt  *time.Time                  `json:"expires_at"`
	Lines      []ReserveInventoryLineInput `json:"lines"`
}

type InventoryReservationLine struct {
	ID                string `json:"id"`
	BranchRef         string `json:"branch_ref"`
	StockBasisRef     string `json:"stock_basis_ref"`
	ListingRef        string `json:"listing_ref"`
	VariantRef        string `json:"variant_ref"`
	RequestedQuantity int64  `json:"requested_quantity"`
	PhysicalQuantity  int64  `json:"physical_quantity"`
	BackorderQuantity int64  `json:"backorder_quantity"`
	Status            string `json:"status"`
}

type InventoryReservation struct {
	ID         string                     `json:"id"`
	SourceType string                     `json:"source_type"`
	SourceRef  string                     `json:"source_ref"`
	Status     string                     `json:"status"`
	ExpiresAt  *time.Time                 `json:"expires_at"`
	CreatedAt  time.Time                  `json:"created_at"`
	UpdatedAt  time.Time                  `json:"updated_at"`
	Version    int64                      `json:"version"`
	Lines      []InventoryReservationLine `json:"lines"`
}

func validConsumptionMode(value string) bool {
	return value == ConsumptionSameUnit || value == ConsumptionExact || value == ConsumptionVariable
}

func (s *Service) ConfigureStockBasis(ctx context.Context, actor Actor, scope Scope, branchID string, input ConfigureStockBasisInput) (InventoryStockBasis, error) {
	if err := s.requireWrites(); err != nil {
		return InventoryStockBasis{}, err
	}
	if err := s.requireBranchScope(scope, branchID); err != nil {
		return InventoryStockBasis{}, err
	}
	if !validConsumptionMode(input.ConsumptionMode) {
		return InventoryStockBasis{}, fieldError("consumption_mode", "Use SAME_UNIT, EXACT_CONVERSION or VARIABLE_FULFILMENT.")
	}
	if input.AllocationPolicy == "" {
		input.AllocationPolicy = AllocationFIFO
	}
	if input.AllocationPolicy != AllocationFIFO && input.AllocationPolicy != AllocationFEFO {
		return InventoryStockBasis{}, fieldError("allocation_policy", "Use FIFO or FEFO.")
	}
	var out InventoryStockBasis
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		if _, err := s.requireShopBranch(ctx, tx, actor.VendorID, branchID); err != nil {
			return err
		}
		var productID, variantID, measureOwner, measureFamily string
		err := tx.QueryRow(ctx, `SELECT l.product_id,l.variant_id,m.owner_key,m.measurement_family
			FROM shop_branch_listings l JOIN shop_measure_definitions m ON m.id=$4
			WHERE l.business_id=$1 AND l.branch_id=$2 AND l.id=$3 AND l.status='ACTIVE'
			AND m.status='ACTIVE' AND m.owner_key IN ('PLATFORM',$1)`, actor.VendorID, branchID, strings.TrimSpace(input.ListingID), strings.TrimSpace(input.MeasureID)).Scan(&productID, &variantID, &measureOwner, &measureFamily)
		if err != nil {
			return mapDB(err, "Stock basis", CodeStockBasisNotFound)
		}
		if input.ConsumptionMode == ConsumptionExact && !IsConvertibleFamily(measureFamily) {
			return coded(apperrors.Conflict("This measure has no exact canonical inventory conversion.", nil), CodeInventoryConversionMissing)
		}
		if input.ConsumptionMode == ConsumptionSameUnit {
			var eligible bool
			err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM shop_listing_sell_units lu
				JOIN shop_sell_units su ON su.id=lu.sell_unit_id AND su.business_id=lu.business_id
				WHERE lu.business_id=$1 AND lu.listing_id=$2 AND lu.enabled AND su.status='ACTIVE'
				AND su.measure_definition_id=$3 AND su.conversion_type <> 'CALIBRATED_APPROXIMATE')`, actor.VendorID, input.ListingID, input.MeasureID).Scan(&eligible)
			if err != nil {
				return err
			}
			if !eligible {
				return coded(apperrors.Conflict("No enabled sell unit consumes this stock measure exactly.", nil), CodeInventoryConversionMissing)
			}
		}
		id := newID("sib_")
		err = tx.QueryRow(ctx, `INSERT INTO shop_inventory_stock_bases
			(id,business_id,branch_id,listing_id,product_id,variant_id,measure_definition_id,measure_owner_key,consumption_mode,lot_policy,lot_tracking_enabled)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
			ON CONFLICT (business_id,branch_id,variant_id) DO UPDATE SET
			listing_id=EXCLUDED.listing_id,measure_definition_id=EXCLUDED.measure_definition_id,
			measure_owner_key=EXCLUDED.measure_owner_key,consumption_mode=EXCLUDED.consumption_mode,
			lot_policy=EXCLUDED.lot_policy,lot_tracking_enabled=EXCLUDED.lot_tracking_enabled,
			status='ACTIVE',updated_at=now(),version=shop_inventory_stock_bases.version+1
			RETURNING id,branch_id,listing_id,product_id,variant_id,measure_definition_id,consumption_mode,lot_policy,lot_tracking_enabled,status,created_at,updated_at,version`,
			id, actor.VendorID, branchID, input.ListingID, productID, variantID, input.MeasureID, measureOwner, input.ConsumptionMode, input.AllocationPolicy, input.LotTracking).
			Scan(&out.ID, &out.BranchRef, &out.ListingRef, &out.ProductRef, &out.VariantRef, &out.MeasureRef, &out.ConsumptionMode, &out.AllocationPolicy, &out.LotTrackingEnabled, &out.Status, &out.CreatedAt, &out.UpdatedAt, &out.Version)
		if err != nil {
			return mapDB(err, "Stock basis", CodeStockBasisNotFound)
		}
		_, err = tx.Exec(ctx, `INSERT INTO shop_inventory_balances(id,business_id,branch_id,stock_basis_id,variant_id)
			VALUES($1,$2,$3,$4,$5) ON CONFLICT (business_id,branch_id,variant_id) DO NOTHING`, newID("sbl_"), actor.VendorID, branchID, out.ID, variantID)
		if err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{action: "shop.inventory.stock_basis.configured", eventType: "shop.inventory.stock_basis.configured", aggregateType: "shop_inventory_stock_basis", aggregateID: out.ID, branchID: branchID, version: out.Version, after: out, payload: out})
	})
	return out, err
}

func (s *Service) CreateReceipt(ctx context.Context, actor Actor, scope Scope, input CreateReceiptInput) (InventoryReceipt, error) {
	if err := s.requireWrites(); err != nil {
		return InventoryReceipt{}, err
	}
	if err := s.requireBranchScope(scope, input.BranchID); err != nil {
		return InventoryReceipt{}, err
	}
	if len(input.Lines) == 0 || len(input.Lines) > 500 {
		return InventoryReceipt{}, fieldError("lines", "Provide between 1 and 500 receipt lines.")
	}
	if len(input.Notes) > 2000 {
		return InventoryReceipt{}, fieldError("notes", "Notes cannot exceed 2000 characters.")
	}
	if input.ReceivedAt.IsZero() {
		input.ReceivedAt = s.now()
	}
	var out InventoryReceipt
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		if _, err := s.requireShopBranch(ctx, tx, actor.VendorID, input.BranchID); err != nil {
			return err
		}
		out = InventoryReceipt{ID: newID("sir_"), BranchRef: input.BranchID, SourceRef: strings.TrimSpace(input.SourceRef), ReceivedAt: input.ReceivedAt, Status: "DRAFT", Version: 1}
		err := tx.QueryRow(ctx, `INSERT INTO shop_inventory_receipts(id,business_id,branch_id,source_ref,supplier_ref,notes,received_at)
			VALUES($1,$2,$3,NULLIF($4,''),NULLIF($5,''),$6,$7) RETURNING created_at,updated_at`, out.ID, actor.VendorID, input.BranchID, out.SourceRef, strings.TrimSpace(input.SupplierRef), input.Notes, input.ReceivedAt).Scan(&out.CreatedAt, &out.UpdatedAt)
		if err != nil {
			return err
		}
		for _, line := range input.Lines {
			if line.Quantity <= 0 || line.Quantity > MaxInventoryQuantityPerLine {
				return fieldError("lines.quantity", "Quantity must be positive and no greater than 1,000,000,000 canonical units.")
			}
			var branchID string
			if err = tx.QueryRow(ctx, `SELECT branch_id FROM shop_inventory_stock_bases WHERE id=$1 AND business_id=$2 AND status='ACTIVE'`, line.StockBasisID, actor.VendorID).Scan(&branchID); err != nil {
				return mapDB(err, "Stock basis", CodeStockBasisNotFound)
			}
			if branchID != input.BranchID {
				return errNotFound("Stock basis", CodeStockBasisNotFound)
			}
			_, err = tx.Exec(ctx, `INSERT INTO shop_inventory_receipt_lines(id,business_id,receipt_id,stock_basis_id,variant_id,quantity,lot_number,batch_number,manufactured_at,best_before_at,expires_at,unit_cost_minor)
				SELECT $1,$2,$3,b.id,b.variant_id,$5,NULLIF($6,''),NULLIF($7,''),$8,$9,$10,$11 FROM shop_inventory_stock_bases b WHERE b.id=$4 AND b.business_id=$2`, newID("sirl_"), actor.VendorID, out.ID, line.StockBasisID, line.Quantity, line.LotNumber, line.BatchNumber, line.ManufacturedAt, line.BestBeforeAt, line.ExpiresAt, line.UnitCostMinor)
			if err != nil {
				return err
			}
		}
		return s.record(ctx, tx, actor, evidence{action: "shop.inventory.receipt.created", eventType: "shop.inventory.receipt.created", aggregateType: "shop_inventory_receipt", aggregateID: out.ID, branchID: input.BranchID, version: 1, after: out, payload: out})
	})
	return out, err
}

func bumpInventoryRevision(ctx context.Context, tx pgx.Tx, businessID, branchID string) (int64, error) {
	var revision int64
	err := tx.QueryRow(ctx, `INSERT INTO shop_inventory_revisions(business_id,branch_id,revision,updated_at) VALUES($1,$2,1,now())
		ON CONFLICT (business_id,branch_id) DO UPDATE SET revision=shop_inventory_revisions.revision+1,updated_at=now() RETURNING revision`, businessID, branchID).Scan(&revision)
	return revision, err
}

func (s *Service) PostReceipt(ctx context.Context, actor Actor, scope Scope, receiptID string) (InventoryReceipt, error) {
	if err := s.requireWrites(); err != nil {
		return InventoryReceipt{}, err
	}
	var out InventoryReceipt
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var status string
		err := tx.QueryRow(ctx, `SELECT id,branch_id,COALESCE(source_ref,''),received_at,status,posted_at,version FROM shop_inventory_receipts WHERE id=$1 AND business_id=$2 FOR UPDATE`, receiptID, actor.VendorID).Scan(&out.ID, &out.BranchRef, &out.SourceRef, &out.ReceivedAt, &status, &out.PostedAt, &out.Version)
		if err != nil {
			return mapDB(err, "Receipt", CodeStockBasisNotFound)
		}
		if err = s.requireBranchScope(scope, out.BranchRef); err != nil {
			return err
		}
		if status == "POSTED" {
			out.Status = status
			return nil
		}
		if status != "DRAFT" {
			return coded(apperrors.Conflict("Only a draft receipt can be posted.", nil), CodeReservationStateConflict)
		}
		rows, err := tx.Query(ctx, `SELECT rl.id,rl.stock_basis_id,rl.variant_id,rl.quantity,COALESCE(rl.lot_number,''),COALESCE(rl.batch_number,''),rl.manufactured_at,rl.best_before_at,rl.expires_at
			FROM shop_inventory_receipt_lines rl WHERE rl.business_id=$1 AND rl.receipt_id=$2 ORDER BY rl.stock_basis_id,rl.id FOR UPDATE`, actor.VendorID, receiptID)
		if err != nil {
			return err
		}
		type line struct {
			id, basis, variant                string
			quantity                          int64
			lot, batch                        string
			manufactured, bestBefore, expires *time.Time
		}
		var lines []line
		for rows.Next() {
			var l line
			if err = rows.Scan(&l.id, &l.basis, &l.variant, &l.quantity, &l.lot, &l.batch, &l.manufactured, &l.bestBefore, &l.expires); err != nil {
				rows.Close()
				return err
			}
			lines = append(lines, l)
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return err
		}
		for _, l := range lines {
			var branchID string
			if err = tx.QueryRow(ctx, `SELECT branch_id FROM shop_inventory_balances WHERE business_id=$1 AND stock_basis_id=$2 FOR UPDATE`, actor.VendorID, l.basis).Scan(&branchID); err != nil {
				return err
			}
			lotID := newID("sil_")
			_, err = tx.Exec(ctx, `INSERT INTO shop_inventory_lots(id,business_id,branch_id,stock_basis_id,variant_id,lot_number,batch_number,source_ref,received_at,manufactured_at,best_before_at,expires_at,on_hand_quantity)
				VALUES($1,$2,$3,$4,$5,NULLIF($6,''),NULLIF($7,''),$8,$9,$10,$11,$12,$13)`, lotID, actor.VendorID, branchID, l.basis, l.variant, l.lot, l.batch, receiptID, out.ReceivedAt, l.manufactured, l.bestBefore, l.expires, l.quantity)
			if err != nil {
				return err
			}
			var balanceRevision int64
			err = tx.QueryRow(ctx, `UPDATE shop_inventory_balances SET on_hand_quantity=on_hand_quantity+$3,revision=revision+1,updated_at=now() WHERE business_id=$1 AND stock_basis_id=$2 RETURNING revision`, actor.VendorID, l.basis, l.quantity).Scan(&balanceRevision)
			if err != nil {
				return err
			}
			inventoryRevision, err := bumpInventoryRevision(ctx, tx, actor.VendorID, branchID)
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `INSERT INTO shop_inventory_movements(id,business_id,branch_id,stock_basis_id,variant_id,lot_id,movement_type,quantity,on_hand_delta,source_type,source_ref,actor_ref,inventory_revision)
				VALUES($1,$2,$3,$4,$5,$6,'RECEIPT',$7,$7,'RECEIPT',$8,NULLIF($9,''),$10)`, newID("sim_"), actor.VendorID, branchID, l.basis, l.variant, lotID, l.quantity, receiptID, actor.StaffID, inventoryRevision)
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `UPDATE shop_inventory_receipt_lines SET lot_id=$1 WHERE id=$2 AND business_id=$3`, lotID, l.id, actor.VendorID)
			if err != nil {
				return err
			}
			if err = s.allocateBackordersFromLot(ctx, tx, actor, branchID, l.basis, l.variant, lotID, l.quantity); err != nil {
				return err
			}
			_ = balanceRevision
		}
		err = tx.QueryRow(ctx, `UPDATE shop_inventory_receipts SET status='POSTED',posted_at=now(),updated_at=now(),version=version+1 WHERE id=$1 AND business_id=$2 RETURNING status,posted_at,updated_at,version`, receiptID, actor.VendorID).Scan(&out.Status, &out.PostedAt, &out.UpdatedAt, &out.Version)
		if err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{action: "shop.inventory.receipt.posted", eventType: "shop.inventory.receipt.posted", aggregateType: "shop_inventory_receipt", aggregateID: out.ID, branchID: out.BranchRef, version: out.Version, after: out, payload: out})
	})
	return out, err
}

// CancelReceipt cancels a draft without stock effects, or reverses an intact
// posted receipt using the exact receipt lots. A posted receipt whose stock has
// already been reserved, quarantined, transferred, or consumed must be
// corrected through explicit stock operations instead of fabricating a
// reversal that would make physical inventory negative.
func (s *Service) CancelReceipt(ctx context.Context, actor Actor, scope Scope, receiptID string) (InventoryReceipt, error) {
	if err := s.requireWrites(); err != nil {
		return InventoryReceipt{}, err
	}
	var out InventoryReceipt
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var status string
		err := tx.QueryRow(ctx, `SELECT id,branch_id,COALESCE(source_ref,''),received_at,status,posted_at,created_at,updated_at,version
			FROM shop_inventory_receipts WHERE id=$1 AND business_id=$2 FOR UPDATE`, receiptID, actor.VendorID).
			Scan(&out.ID, &out.BranchRef, &out.SourceRef, &out.ReceivedAt, &status, &out.PostedAt, &out.CreatedAt, &out.UpdatedAt, &out.Version)
		if err != nil {
			return mapDB(err, "Receipt", CodeStockBasisNotFound)
		}
		if err = s.requireBranchScope(scope, out.BranchRef); err != nil {
			return err
		}
		if status == "CANCELLED" {
			out.Status = status
			return loadReceiptLines(ctx, tx, actor.VendorID, &out)
		}
		if status == "POSTED" {
			rows, queryErr := tx.Query(ctx, `SELECT rl.id,rl.stock_basis_id,rl.variant_id,rl.quantity,rl.lot_id,
				l.on_hand_quantity,l.reserved_quantity,l.quarantined_quantity
				FROM shop_inventory_receipt_lines rl
				JOIN shop_inventory_lots l ON l.id=rl.lot_id AND l.business_id=rl.business_id
				WHERE rl.business_id=$1 AND rl.receipt_id=$2 ORDER BY rl.stock_basis_id,rl.id FOR UPDATE OF rl,l`, actor.VendorID, receiptID)
			if queryErr != nil {
				return queryErr
			}
			type reversalLine struct {
				id, basis, variant, lot                 string
				quantity, onHand, reserved, quarantined int64
			}
			var lines []reversalLine
			for rows.Next() {
				var line reversalLine
				if err = rows.Scan(&line.id, &line.basis, &line.variant, &line.quantity, &line.lot, &line.onHand, &line.reserved, &line.quarantined); err != nil {
					rows.Close()
					return err
				}
				lines = append(lines, line)
			}
			rows.Close()
			if err = rows.Err(); err != nil {
				return err
			}
			for _, line := range lines {
				if line.onHand != line.quantity || line.reserved != 0 || line.quarantined != 0 {
					return coded(apperrors.Conflict("Posted receipt stock is no longer intact; use an explicit adjustment or return workflow.", nil), CodeReservationStateConflict)
				}
				if _, _, _, err = lockInventoryBalance(ctx, tx, actor.VendorID, line.basis); err != nil {
					return err
				}
				if _, err = tx.Exec(ctx, `UPDATE shop_inventory_lots SET on_hand_quantity=0,status='ARCHIVED',updated_at=now(),version=version+1 WHERE id=$1 AND business_id=$2`, line.lot, actor.VendorID); err != nil {
					return err
				}
				if _, err = tx.Exec(ctx, `UPDATE shop_inventory_balances SET on_hand_quantity=on_hand_quantity-$3,revision=revision+1,updated_at=now() WHERE business_id=$1 AND stock_basis_id=$2`, actor.VendorID, line.basis, line.quantity); err != nil {
					return err
				}
				revision, revisionErr := bumpInventoryRevision(ctx, tx, actor.VendorID, out.BranchRef)
				if revisionErr != nil {
					return revisionErr
				}
				if _, err = tx.Exec(ctx, `INSERT INTO shop_inventory_movements(id,business_id,branch_id,stock_basis_id,variant_id,lot_id,movement_type,quantity,on_hand_delta,source_type,source_ref,reason_code,actor_ref,inventory_revision)
					VALUES($1,$2,$3,$4,$5,$6,'ADJUSTMENT_OUT',$7,$8,'RECEIPT',$9,'RECEIPT_REVERSAL',NULLIF($10,''),$11)`, newID("sim_"), actor.VendorID, out.BranchRef, line.basis, line.variant, line.lot, line.quantity, -line.quantity, receiptID, actor.StaffID, revision); err != nil {
					return err
				}
			}
		} else if status != "DRAFT" {
			return coded(apperrors.Conflict("Receipt cannot be cancelled from this state.", nil), CodeReservationStateConflict)
		}
		err = tx.QueryRow(ctx, `UPDATE shop_inventory_receipts SET status='CANCELLED',updated_at=now(),version=version+1 WHERE id=$1 AND business_id=$2 RETURNING status,updated_at,version`, receiptID, actor.VendorID).Scan(&out.Status, &out.UpdatedAt, &out.Version)
		if err != nil {
			return err
		}
		if err = loadReceiptLines(ctx, tx, actor.VendorID, &out); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{action: "shop.inventory.receipt.cancelled", eventType: "shop.inventory.receipt.cancelled", aggregateType: "shop_inventory_receipt", aggregateID: out.ID, branchID: out.BranchRef, version: out.Version, after: out, payload: out})
	})
	return out, err
}

func (s *Service) allocateBackordersFromLot(ctx context.Context, tx pgx.Tx, actor Actor, branchID, basisID, variantID, lotID string, received int64) error {
	businessID := actor.VendorID
	rows, err := tx.Query(ctx, `SELECT l.id,l.reservation_id,l.backorder_quantity
		FROM shop_inventory_reservation_lines l
		JOIN shop_inventory_reservations r ON r.id=l.reservation_id AND r.business_id=l.business_id
		WHERE l.business_id=$1 AND l.stock_basis_id=$2 AND l.backorder_quantity>0
		AND l.status='HELD' AND r.status IN ('HELD','CONFIRMED')
		ORDER BY r.created_at,r.id,l.id FOR UPDATE OF l,r`, businessID, basisID)
	if err != nil {
		return err
	}
	type obligation struct {
		lineID, reservationID string
		quantity              int64
	}
	var obligations []obligation
	for rows.Next() {
		var item obligation
		if err = rows.Scan(&item.lineID, &item.reservationID, &item.quantity); err != nil {
			rows.Close()
			return err
		}
		obligations = append(obligations, item)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	remaining := received
	for _, item := range obligations {
		if remaining == 0 {
			break
		}
		take := item.quantity
		if take > remaining {
			take = remaining
		}
		if _, err = tx.Exec(ctx, `UPDATE shop_inventory_lots SET reserved_quantity=reserved_quantity+$3,updated_at=now(),version=version+1
			WHERE id=$1 AND business_id=$2`, lotID, businessID, take); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO shop_inventory_reservation_allocations(id,business_id,reservation_line_id,lot_id,quantity)
			VALUES($1,$2,$3,$4,$5) ON CONFLICT (reservation_line_id,lot_id) DO UPDATE SET quantity=shop_inventory_reservation_allocations.quantity+EXCLUDED.quantity,updated_at=now()`, newID("sirva_"), businessID, item.lineID, lotID, take); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE shop_inventory_reservation_lines SET physical_quantity=physical_quantity+$3,
			backorder_quantity=backorder_quantity-$3,updated_at=now() WHERE id=$1 AND business_id=$2`, item.lineID, businessID, take); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE shop_inventory_balances SET reserved_quantity=reserved_quantity+$3,
			backorder_quantity=backorder_quantity-$3,revision=revision+1,updated_at=now() WHERE business_id=$1 AND stock_basis_id=$2`, businessID, basisID, take); err != nil {
			return err
		}
		revision, revisionErr := bumpInventoryRevision(ctx, tx, businessID, branchID)
		if revisionErr != nil {
			return revisionErr
		}
		if _, err = tx.Exec(ctx, `INSERT INTO shop_inventory_movements(id,business_id,branch_id,stock_basis_id,variant_id,lot_id,movement_type,quantity,reserved_delta,source_type,source_ref,inventory_revision)
			VALUES($1,$2,$3,$4,$5,$6,'PREORDER_ALLOCATION',$7,$7,'RESERVATION',$8,$9)`, newID("sim_"), businessID, branchID, basisID, variantID, lotID, take, item.reservationID, revision); err != nil {
			return err
		}
		payload := map[string]any{"reservation_id": item.reservationID, "reservation_line_id": item.lineID, "stock_basis_id": basisID, "lot_id": lotID, "allocated_quantity": take, "inventory_revision": revision}
		if err = s.record(ctx, tx, actor, evidence{action: "shop.inventory.backorder.allocated", eventType: "shop.inventory.backorder.allocated", aggregateType: "shop_inventory_reservation_line", aggregateID: item.lineID, branchID: branchID, version: revision, after: payload, payload: payload}); err != nil {
			return err
		}
		remaining -= take
	}
	return nil
}

func reservationFingerprint(input ReserveInventoryInput) string {
	copyInput := input
	copyInput.SourceType = strings.ToUpper(strings.TrimSpace(copyInput.SourceType))
	copyInput.SourceRef = strings.TrimSpace(copyInput.SourceRef)
	sort.Slice(copyInput.Lines, func(i, j int) bool { return copyInput.Lines[i].StockBasisID < copyInput.Lines[j].StockBasisID })
	raw, _ := json.Marshal(copyInput)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (s *Service) ReserveInventory(ctx context.Context, actor Actor, scope Scope, input ReserveInventoryInput) (InventoryReservation, error) {
	if err := s.requireWrites(); err != nil {
		return InventoryReservation{}, err
	}
	input.SourceType = strings.ToUpper(strings.TrimSpace(input.SourceType))
	input.SourceRef = strings.TrimSpace(input.SourceRef)
	if !contains([]string{"ORDER", "RFQ", "MANUAL", "INTERNAL_TEST", "OTHER_CONTROLLED"}, input.SourceType) {
		return InventoryReservation{}, fieldError("source_type", "Unknown reservation source type.")
	}
	if input.SourceRef == "" {
		return InventoryReservation{}, fieldError("source_ref", "Source reference is required.")
	}
	if input.ExpiresAt != nil && !input.ExpiresAt.After(s.now()) {
		return InventoryReservation{}, fieldError("expires_at", "Reservation expiry must be in the future.")
	}
	if len(input.Lines) == 0 || len(input.Lines) > 100 {
		return InventoryReservation{}, fieldError("lines", "Provide between 1 and 100 reservation lines.")
	}
	for _, line := range input.Lines {
		if line.Quantity <= 0 || line.Quantity > MaxInventoryQuantityPerLine {
			return InventoryReservation{}, fieldError("lines.quantity", "Quantity must be positive and no greater than 1,000,000,000 canonical units.")
		}
	}
	sort.Slice(input.Lines, func(i, j int) bool { return input.Lines[i].StockBasisID < input.Lines[j].StockBasisID })
	for i := 1; i < len(input.Lines); i++ {
		if input.Lines[i-1].StockBasisID == input.Lines[i].StockBasisID {
			return InventoryReservation{}, fieldError("lines", "A stock basis may appear only once.")
		}
	}
	fingerprint := reservationFingerprint(input)
	var out InventoryReservation
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var expiredReleases []expiredReleaseEvidence
		var existingFingerprint string
		err := tx.QueryRow(ctx, `SELECT id,request_fingerprint FROM shop_inventory_reservations WHERE business_id=$1 AND source_type=$2 AND source_ref=$3 FOR UPDATE`, actor.VendorID, input.SourceType, input.SourceRef).Scan(&out.ID, &existingFingerprint)
		if err == nil {
			if existingFingerprint != fingerprint {
				return coded(apperrors.Conflict("This reservation source was already used with a different request.", nil), CodeReservationFingerprint)
			}
			loaded, loadErr := loadReservation(ctx, tx, actor.VendorID, out.ID)
			out = loaded
			return loadErr
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		out = InventoryReservation{ID: newID("sirv_"), SourceType: input.SourceType, SourceRef: input.SourceRef, Status: ReservationHeld, ExpiresAt: input.ExpiresAt, Version: 1}
		err = tx.QueryRow(ctx, `INSERT INTO shop_inventory_reservations(id,business_id,source_type,source_ref,request_fingerprint,status,expires_at)
			VALUES($1,$2,$3,$4,$5,'HELD',$6) RETURNING created_at,updated_at`, out.ID, actor.VendorID, input.SourceType, input.SourceRef, fingerprint, input.ExpiresAt).Scan(&out.CreatedAt, &out.UpdatedAt)
		if err != nil {
			return mapDB(err, "Reservation", CodeReservationFingerprint)
		}
		branches := map[string]bool{}
		for _, requestLine := range input.Lines {
			var branchID, listingID, variantID, mode, policy string
			err = tx.QueryRow(ctx, `SELECT b.branch_id,b.listing_id,b.variant_id,b.consumption_mode,b.lot_policy
				FROM shop_inventory_stock_bases b WHERE b.id=$1 AND b.business_id=$2 AND b.status='ACTIVE'`, requestLine.StockBasisID, actor.VendorID).Scan(&branchID, &listingID, &variantID, &mode, &policy)
			if err != nil {
				return mapDB(err, "Stock basis", CodeStockBasisNotFound)
			}
			if err = s.requireBranchScope(scope, branchID); err != nil {
				return err
			}
			branches[branchID] = true
			var expired expiredReleaseEvidence
			if err = releaseExpiredForBasis(ctx, tx, actor.VendorID, requestLine.StockBasisID, s.now(), &expired); err != nil {
				return err
			}
			if expired.PhysicalQuantity > 0 || expired.BackorderQuantity > 0 {
				expiredReleases = append(expiredReleases, expired)
			}
			var onHand, reserved, quarantined, outstandingBackorder int64
			err = tx.QueryRow(ctx, `SELECT on_hand_quantity,reserved_quantity,quarantined_quantity,backorder_quantity FROM shop_inventory_balances WHERE business_id=$1 AND stock_basis_id=$2 FOR UPDATE`, actor.VendorID, requestLine.StockBasisID).Scan(&onHand, &reserved, &quarantined, &outstandingBackorder)
			if err != nil {
				return mapDB(err, "Inventory balance", CodeStockBasisNotFound)
			}
			available := onHand - reserved - quarantined
			physicalQuantity := requestLine.Quantity
			backorderQuantity := int64(0)
			if requestLine.Quantity > available {
				physicalQuantity = available
				backorderQuantity = requestLine.Quantity - available
				var policyMode string
				var maximumOutstanding, maximumPerRequest *int64
				var availableFrom, cutoffAt *time.Time
				err = tx.QueryRow(ctx, `SELECT mode,maximum_outstanding_quantity,maximum_per_request,available_from,cutoff_at
					FROM shop_inventory_policies WHERE business_id=$1 AND branch_id=$2 AND variant_id=$3 AND listing_id=$4 AND status='ACTIVE'`, actor.VendorID, branchID, variantID, listingID).
					Scan(&policyMode, &maximumOutstanding, &maximumPerRequest, &availableFrom, &cutoffAt)
				if errors.Is(err, pgx.ErrNoRows) || policyMode == "NONE" {
					return coded(apperrors.Conflict("There is not enough sellable physical stock for this reservation.", nil), CodeInventoryInsufficient)
				}
				if err != nil {
					return err
				}
				now := s.now()
				if policyMode == "PREORDER" && (availableFrom == nil || !now.Before(*availableFrom)) {
					return coded(apperrors.Conflict("Preorder is not available for this stock request.", nil), CodeInventoryPolicyLimit)
				}
				if cutoffAt != nil && now.After(*cutoffAt) {
					return coded(apperrors.Conflict("The preorder/backorder cutoff has passed.", nil), CodeInventoryPolicyLimit)
				}
				if maximumPerRequest != nil && backorderQuantity > *maximumPerRequest {
					return coded(apperrors.Conflict("This request exceeds the preorder/backorder per-request limit.", nil), CodeInventoryPolicyLimit)
				}
				if maximumOutstanding != nil && outstandingBackorder+backorderQuantity > *maximumOutstanding {
					return coded(apperrors.Conflict("This request exceeds the outstanding preorder/backorder limit.", nil), CodeInventoryPolicyLimit)
				}
			}
			line := InventoryReservationLine{ID: newID("sirvl_"), BranchRef: branchID, StockBasisRef: requestLine.StockBasisID, ListingRef: listingID, VariantRef: variantID, RequestedQuantity: requestLine.Quantity, PhysicalQuantity: physicalQuantity, BackorderQuantity: backorderQuantity, Status: ReservationHeld}
			_, err = tx.Exec(ctx, `INSERT INTO shop_inventory_reservation_lines(id,business_id,reservation_id,branch_id,stock_basis_id,listing_id,variant_id,requested_quantity,physical_quantity,backorder_quantity,consumption_mode)
				VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, line.ID, actor.VendorID, out.ID, branchID, line.StockBasisRef, listingID, variantID, requestLine.Quantity, physicalQuantity, backorderQuantity, mode)
			if err != nil {
				return err
			}
			lotRows, err := tx.Query(ctx, `SELECT id,on_hand_quantity-reserved_quantity-quarantined_quantity AS available
				FROM shop_inventory_lots WHERE business_id=$1 AND stock_basis_id=$2 AND status='ACTIVE'
				AND (expires_at IS NULL OR expires_at>$3) AND on_hand_quantity>reserved_quantity+quarantined_quantity
				ORDER BY CASE WHEN $4='FEFO' THEN expires_at END ASC NULLS LAST,received_at,id FOR UPDATE`, actor.VendorID, requestLine.StockBasisID, s.now(), policy)
			if err != nil {
				return err
			}
			type lotAvailable struct {
				id        string
				available int64
			}
			var lots []lotAvailable
			for lotRows.Next() {
				var lot lotAvailable
				if err = lotRows.Scan(&lot.id, &lot.available); err != nil {
					lotRows.Close()
					return err
				}
				lots = append(lots, lot)
			}
			lotRows.Close()
			if err = lotRows.Err(); err != nil {
				return err
			}
			remaining := physicalQuantity
			for _, lot := range lots {
				if remaining == 0 {
					break
				}
				take := lot.available
				if take > remaining {
					take = remaining
				}
				_, err = tx.Exec(ctx, `UPDATE shop_inventory_lots SET reserved_quantity=reserved_quantity+$3,updated_at=now(),version=version+1 WHERE id=$1 AND business_id=$2`, lot.id, actor.VendorID, take)
				if err != nil {
					return err
				}
				_, err = tx.Exec(ctx, `INSERT INTO shop_inventory_reservation_allocations(id,business_id,reservation_line_id,lot_id,quantity) VALUES($1,$2,$3,$4,$5)`, newID("sirva_"), actor.VendorID, line.ID, lot.id, take)
				if err != nil {
					return err
				}
				remaining -= take
			}
			if remaining != 0 {
				return coded(apperrors.Conflict("No valid non-expired, non-quarantined lot can satisfy this reservation.", nil), CodeInventoryInsufficient)
			}
			_, err = tx.Exec(ctx, `UPDATE shop_inventory_balances SET reserved_quantity=reserved_quantity+$3,backorder_quantity=backorder_quantity+$4,revision=revision+1,updated_at=now() WHERE business_id=$1 AND stock_basis_id=$2`, actor.VendorID, requestLine.StockBasisID, physicalQuantity, backorderQuantity)
			if err != nil {
				return err
			}
			inventoryRevision, err := bumpInventoryRevision(ctx, tx, actor.VendorID, branchID)
			if err != nil {
				return err
			}
			if physicalQuantity > 0 {
				_, err = tx.Exec(ctx, `INSERT INTO shop_inventory_movements(id,business_id,branch_id,stock_basis_id,variant_id,movement_type,quantity,reserved_delta,source_type,source_ref,actor_ref,inventory_revision)
					VALUES($1,$2,$3,$4,$5,'RESERVATION',$6,$6,$7,$8,NULLIF($9,''),$10)`, newID("sim_"), actor.VendorID, branchID, requestLine.StockBasisID, variantID, physicalQuantity, input.SourceType, input.SourceRef, actor.StaffID, inventoryRevision)
				if err != nil {
					return err
				}
			}
			out.Lines = append(out.Lines, line)
		}
		for _, expired := range expiredReleases {
			systemActor := Actor{VendorID: actor.VendorID, CorrelationID: actor.CorrelationID, CausationID: actor.CausationID}
			if err = s.record(ctx, tx, systemActor, evidence{action: "shop.inventory.reservation.expired", eventType: "shop.inventory.reservation.expired", aggregateType: "shop_inventory_stock_basis", aggregateID: expired.StockBasisRef, branchID: expired.BranchRef, version: expired.InventoryRevision, after: expired, payload: expired, reason: "HOLD_EXPIRED"}); err != nil {
				return err
			}
		}
		return s.record(ctx, tx, actor, evidence{action: "shop.inventory.reservation.held", eventType: "shop.inventory.reservation.held", aggregateType: "shop_inventory_reservation", aggregateID: out.ID, version: out.Version, after: out, payload: out})
	})
	return out, err
}

type expiredReleaseEvidence struct {
	BranchRef         string   `json:"branch_ref"`
	StockBasisRef     string   `json:"stock_basis_ref"`
	ReservationRefs   []string `json:"reservation_refs"`
	PhysicalQuantity  int64    `json:"physical_quantity"`
	BackorderQuantity int64    `json:"backorder_quantity"`
	InventoryRevision int64    `json:"inventory_revision"`
}

func releaseExpiredForBasis(ctx context.Context, tx pgx.Tx, businessID, basisID string, now time.Time, evidenceOut *expiredReleaseEvidence) error {
	rows, err := tx.Query(ctx, `SELECT l.id,l.reservation_id,l.branch_id,l.variant_id,l.physical_quantity,l.backorder_quantity
		FROM shop_inventory_reservation_lines l
		JOIN shop_inventory_reservations r ON r.id=l.reservation_id AND r.business_id=l.business_id
		WHERE l.business_id=$1 AND l.stock_basis_id=$2 AND l.status='HELD'
		AND r.status IN ('HELD','CONFIRMED') AND r.expires_at<=$3
		ORDER BY r.id,l.id FOR UPDATE OF l,r`, businessID, basisID, now)
	if err != nil {
		return err
	}
	type expiredLine struct {
		line, reservation, branch, variant string
		physical, backorder                int64
	}
	var lines []expiredLine
	for rows.Next() {
		var line expiredLine
		if err = rows.Scan(&line.line, &line.reservation, &line.branch, &line.variant, &line.physical, &line.backorder); err != nil {
			rows.Close()
			return err
		}
		lines = append(lines, line)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	if len(lines) == 0 {
		return nil
	}
	var totalPhysical, totalBackorder int64
	reservations := map[string]bool{}
	for _, line := range lines {
		allocationRows, queryErr := tx.Query(ctx, `SELECT id,lot_id,quantity FROM shop_inventory_reservation_allocations
			WHERE business_id=$1 AND reservation_line_id=$2 AND status='HELD' ORDER BY lot_id FOR UPDATE`, businessID, line.line)
		if queryErr != nil {
			return queryErr
		}
		type expiredAllocation struct {
			id, lot  string
			quantity int64
		}
		var allocations []expiredAllocation
		for allocationRows.Next() {
			var allocation expiredAllocation
			if err = allocationRows.Scan(&allocation.id, &allocation.lot, &allocation.quantity); err != nil {
				allocationRows.Close()
				return err
			}
			allocations = append(allocations, allocation)
		}
		allocationRows.Close()
		if err = allocationRows.Err(); err != nil {
			return err
		}
		for _, allocation := range allocations {
			if _, err = tx.Exec(ctx, `UPDATE shop_inventory_lots SET reserved_quantity=reserved_quantity-$3,updated_at=now(),version=version+1 WHERE id=$1 AND business_id=$2`, allocation.lot, businessID, allocation.quantity); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `UPDATE shop_inventory_reservation_allocations SET status='EXPIRED',updated_at=now() WHERE id=$1 AND business_id=$2`, allocation.id, businessID); err != nil {
				return err
			}
		}
		totalPhysical += line.physical
		totalBackorder += line.backorder
		reservations[line.reservation] = true
		if _, err = tx.Exec(ctx, `UPDATE shop_inventory_reservation_lines SET status='EXPIRED',updated_at=now() WHERE id=$1 AND business_id=$2`, line.line, businessID); err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `UPDATE shop_inventory_balances SET reserved_quantity=reserved_quantity-$3,
		backorder_quantity=backorder_quantity-$4,revision=revision+1,updated_at=now()
		WHERE business_id=$1 AND stock_basis_id=$2`, businessID, basisID, totalPhysical, totalBackorder)
	if err != nil {
		return err
	}
	var inventoryRevision int64
	if totalPhysical > 0 {
		revision, revisionErr := bumpInventoryRevision(ctx, tx, businessID, lines[0].branch)
		if revisionErr != nil {
			return revisionErr
		}
		inventoryRevision = revision
		if _, err = tx.Exec(ctx, `INSERT INTO shop_inventory_movements(id,business_id,branch_id,stock_basis_id,variant_id,movement_type,quantity,reserved_delta,source_type,source_ref,inventory_revision)
			VALUES($1,$2,$3,$4,$5,'RESERVATION_RELEASE',$6,$7,'RESERVATION_EXPIRY',$8,$9)`, newID("sim_"), businessID, lines[0].branch, basisID, lines[0].variant, totalPhysical, -totalPhysical, lines[0].reservation, revision); err != nil {
			return err
		}
	} else if totalBackorder > 0 {
		if inventoryRevision, err = bumpInventoryRevision(ctx, tx, businessID, lines[0].branch); err != nil {
			return err
		}
	}
	for id := range reservations {
		if _, err = tx.Exec(ctx, `UPDATE shop_inventory_reservations SET status='EXPIRED',updated_at=now(),version=version+1 WHERE id=$1 AND business_id=$2 AND NOT EXISTS(SELECT 1 FROM shop_inventory_reservation_lines WHERE reservation_id=$1 AND business_id=$2 AND status='HELD')`, id, businessID); err != nil {
			return err
		}
	}
	if evidenceOut != nil {
		reservationRefs := make([]string, 0, len(reservations))
		for id := range reservations {
			reservationRefs = append(reservationRefs, id)
		}
		sort.Strings(reservationRefs)
		*evidenceOut = expiredReleaseEvidence{BranchRef: lines[0].branch, StockBasisRef: basisID, ReservationRefs: reservationRefs, PhysicalQuantity: totalPhysical, BackorderQuantity: totalBackorder, InventoryRevision: inventoryRevision}
	}
	return nil
}

func loadReservation(ctx context.Context, q querier, businessID, id string) (InventoryReservation, error) {
	var out InventoryReservation
	err := q.QueryRow(ctx, `SELECT id,source_type,source_ref,status,expires_at,created_at,updated_at,version FROM shop_inventory_reservations WHERE id=$1 AND business_id=$2`, id, businessID).Scan(&out.ID, &out.SourceType, &out.SourceRef, &out.Status, &out.ExpiresAt, &out.CreatedAt, &out.UpdatedAt, &out.Version)
	if err != nil {
		return out, mapDB(err, "Reservation", CodeReservationNotFound)
	}
	rows, err := q.Query(ctx, `SELECT id,branch_id,stock_basis_id,listing_id,variant_id,requested_quantity,physical_quantity,backorder_quantity,status FROM shop_inventory_reservation_lines WHERE reservation_id=$1 AND business_id=$2 ORDER BY id`, id, businessID)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var line InventoryReservationLine
		if err = rows.Scan(&line.ID, &line.BranchRef, &line.StockBasisRef, &line.ListingRef, &line.VariantRef, &line.RequestedQuantity, &line.PhysicalQuantity, &line.BackorderQuantity, &line.Status); err != nil {
			return out, err
		}
		out.Lines = append(out.Lines, line)
	}
	return out, rows.Err()
}

func (s *Service) InventoryBalance(ctx context.Context, actor Actor, scope Scope, basisID string) (InventoryBalance, error) {
	var out InventoryBalance
	err := s.db.QueryRow(ctx, `SELECT b.stock_basis_id,b.branch_id,b.variant_id,b.on_hand_quantity,b.reserved_quantity,b.quarantined_quantity,b.in_transit_quantity,b.backorder_quantity,b.revision,b.updated_at
		FROM shop_inventory_balances b WHERE b.business_id=$1 AND b.stock_basis_id=$2`, actor.VendorID, basisID).Scan(&out.StockBasisRef, &out.BranchRef, &out.VariantRef, &out.OnHandQuantity, &out.ReservedQuantity, &out.QuarantinedQuantity, &out.InTransitQuantity, &out.BackorderQuantity, &out.Revision, &out.UpdatedAt)
	if err != nil {
		return out, mapDB(err, "Inventory balance", CodeStockBasisNotFound)
	}
	if err = s.requireBranchScope(scope, out.BranchRef); err != nil {
		return InventoryBalance{}, err
	}
	out.AvailableQuantity = out.OnHandQuantity - out.ReservedQuantity - out.QuarantinedQuantity
	return out, nil
}
