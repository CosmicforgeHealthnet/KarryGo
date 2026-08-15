package shop

import (
	"context"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

type TransferLineInput struct {
	SourceStockBasisID      string `json:"source_stock_basis_id"`
	DestinationStockBasisID string `json:"destination_stock_basis_id"`
	Quantity                int64  `json:"quantity"`
}

type CreateTransferInput struct {
	SourceBranchID      string              `json:"source_branch_id"`
	DestinationBranchID string              `json:"destination_branch_id"`
	Notes               string              `json:"notes"`
	Lines               []TransferLineInput `json:"lines"`
}

type InventoryTransferLine struct {
	ID                       string `json:"id"`
	SourceStockBasisRef      string `json:"source_stock_basis_ref"`
	DestinationStockBasisRef string `json:"destination_stock_basis_ref"`
	VariantRef               string `json:"variant_ref"`
	SentQuantity             int64  `json:"sent_quantity"`
	ReceivedQuantity         int64  `json:"received_quantity"`
	DamagedQuantity          int64  `json:"damaged_quantity"`
	MissingQuantity          int64  `json:"missing_quantity"`
	Status                   string `json:"status"`
}

type InventoryTransfer struct {
	ID                   string                  `json:"id"`
	SourceBranchRef      string                  `json:"source_branch_ref"`
	DestinationBranchRef string                  `json:"destination_branch_ref"`
	Status               string                  `json:"status"`
	Notes                string                  `json:"notes,omitempty"`
	DispatchedAt         *time.Time              `json:"dispatched_at"`
	ReceivedAt           *time.Time              `json:"received_at"`
	CreatedAt            time.Time               `json:"created_at"`
	UpdatedAt            time.Time               `json:"updated_at"`
	Version              int64                   `json:"version"`
	Lines                []InventoryTransferLine `json:"lines"`
}

func (s *Service) CreateTransfer(ctx context.Context, actor Actor, scope Scope, input CreateTransferInput) (InventoryTransfer, error) {
	if err := s.requireWrites(); err != nil {
		return InventoryTransfer{}, err
	}
	if input.SourceBranchID == input.DestinationBranchID {
		return InventoryTransfer{}, fieldError("destination_branch_id", "Destination must differ from source.")
	}
	if err := s.requireBranchScope(scope, input.SourceBranchID); err != nil {
		return InventoryTransfer{}, err
	}
	if err := s.requireBranchScope(scope, input.DestinationBranchID); err != nil {
		return InventoryTransfer{}, err
	}
	if len(input.Lines) == 0 || len(input.Lines) > 200 {
		return InventoryTransfer{}, fieldError("lines", "Provide between 1 and 200 transfer lines.")
	}
	sort.Slice(input.Lines, func(i, j int) bool { return input.Lines[i].SourceStockBasisID < input.Lines[j].SourceStockBasisID })
	var out InventoryTransfer
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		if _, err := s.requireShopBranch(ctx, tx, actor.VendorID, input.SourceBranchID); err != nil {
			return err
		}
		if _, err := s.requireShopBranch(ctx, tx, actor.VendorID, input.DestinationBranchID); err != nil {
			return err
		}
		out = InventoryTransfer{ID: newID("sit_"), SourceBranchRef: input.SourceBranchID, DestinationBranchRef: input.DestinationBranchID, Status: "DRAFT", Notes: input.Notes, Version: 1}
		if err := tx.QueryRow(ctx, `INSERT INTO shop_inventory_transfers(id,business_id,source_branch_id,destination_branch_id,notes) VALUES($1,$2,$3,$4,$5) RETURNING created_at,updated_at`, out.ID, actor.VendorID, input.SourceBranchID, input.DestinationBranchID, input.Notes).Scan(&out.CreatedAt, &out.UpdatedAt); err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, line := range input.Lines {
			if line.Quantity <= 0 || line.Quantity > MaxInventoryQuantityPerLine {
				return fieldError("lines.quantity", "Quantity must be positive and within the controlled per-line limit.")
			}
			var sourceBranch, destinationBranch, sourceVariant, destinationVariant string
			if err := tx.QueryRow(ctx, `SELECT branch_id,variant_id FROM shop_inventory_stock_bases WHERE id=$1 AND business_id=$2 AND status='ACTIVE'`, line.SourceStockBasisID, actor.VendorID).Scan(&sourceBranch, &sourceVariant); err != nil {
				return mapDB(err, "Source stock basis", CodeStockBasisNotFound)
			}
			if err := tx.QueryRow(ctx, `SELECT branch_id,variant_id FROM shop_inventory_stock_bases WHERE id=$1 AND business_id=$2 AND status='ACTIVE'`, line.DestinationStockBasisID, actor.VendorID).Scan(&destinationBranch, &destinationVariant); err != nil {
				return mapDB(err, "Destination stock basis", CodeStockBasisNotFound)
			}
			if sourceBranch != input.SourceBranchID || destinationBranch != input.DestinationBranchID || sourceVariant != destinationVariant {
				return coded(apperrors.Validation("Transfer stock bases must represent the same variant in the declared branches.", nil), CodeTransferNotFound)
			}
			if seen[sourceVariant] {
				return fieldError("lines", "A variant may appear only once.")
			}
			seen[sourceVariant] = true
			item := InventoryTransferLine{ID: newID("sitl_"), SourceStockBasisRef: line.SourceStockBasisID, DestinationStockBasisRef: line.DestinationStockBasisID, VariantRef: sourceVariant, SentQuantity: line.Quantity, Status: "PENDING"}
			if _, err := tx.Exec(ctx, `INSERT INTO shop_inventory_transfer_lines(id,business_id,transfer_id,source_stock_basis_id,destination_stock_basis_id,variant_id,sent_quantity) VALUES($1,$2,$3,$4,$5,$6,$7)`, item.ID, actor.VendorID, out.ID, item.SourceStockBasisRef, item.DestinationStockBasisRef, item.VariantRef, item.SentQuantity); err != nil {
				return err
			}
			out.Lines = append(out.Lines, item)
		}
		return s.record(ctx, tx, actor, evidence{action: "shop.inventory.transfer.created", eventType: "shop.inventory.transfer.created", aggregateType: "shop_inventory_transfer", aggregateID: out.ID, branchID: out.SourceBranchRef, version: 1, after: out, payload: out})
	})
	return out, err
}

func (s *Service) GetTransfer(ctx context.Context, actor Actor, scope Scope, id string) (InventoryTransfer, error) {
	var out InventoryTransfer
	err := s.db.QueryRow(ctx, `SELECT id,source_branch_id,destination_branch_id,status,notes,dispatched_at,received_at,created_at,updated_at,version
		FROM shop_inventory_transfers WHERE id=$1 AND business_id=$2`, id, actor.VendorID).
		Scan(&out.ID, &out.SourceBranchRef, &out.DestinationBranchRef, &out.Status, &out.Notes, &out.DispatchedAt, &out.ReceivedAt, &out.CreatedAt, &out.UpdatedAt, &out.Version)
	if err != nil {
		return out, mapDB(err, "Transfer", CodeTransferNotFound)
	}
	if err = s.requireBranchScope(scope, out.SourceBranchRef); err != nil {
		return InventoryTransfer{}, err
	}
	if err = s.requireBranchScope(scope, out.DestinationBranchRef); err != nil {
		return InventoryTransfer{}, err
	}
	if err = loadTransferLines(ctx, s.db, actor.VendorID, &out); err != nil {
		return InventoryTransfer{}, err
	}
	return out, nil
}

// CancelTransfer cancels only stock-neutral draft/approved transfers. Once
// dispatched, discrepancy-aware receipt is required so in-transit evidence is
// never silently erased.
func (s *Service) CancelTransfer(ctx context.Context, actor Actor, scope Scope, id string) (InventoryTransfer, error) {
	if err := s.requireWrites(); err != nil {
		return InventoryTransfer{}, err
	}
	var out InventoryTransfer
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var status string
		err := tx.QueryRow(ctx, `SELECT id,source_branch_id,destination_branch_id,status,notes,dispatched_at,received_at,created_at,updated_at,version
			FROM shop_inventory_transfers WHERE id=$1 AND business_id=$2 FOR UPDATE`, id, actor.VendorID).
			Scan(&out.ID, &out.SourceBranchRef, &out.DestinationBranchRef, &status, &out.Notes, &out.DispatchedAt, &out.ReceivedAt, &out.CreatedAt, &out.UpdatedAt, &out.Version)
		if err != nil {
			return mapDB(err, "Transfer", CodeTransferNotFound)
		}
		if err = s.requireBranchScope(scope, out.SourceBranchRef); err != nil {
			return err
		}
		if err = s.requireBranchScope(scope, out.DestinationBranchRef); err != nil {
			return err
		}
		if status == "CANCELLED" {
			out.Status = status
			return loadTransferLines(ctx, tx, actor.VendorID, &out)
		}
		if status != "DRAFT" && status != "APPROVED" {
			return coded(apperrors.Conflict("A dispatched transfer must be resolved through receipt and discrepancy evidence.", nil), CodeReservationStateConflict)
		}
		if _, err = tx.Exec(ctx, `UPDATE shop_inventory_transfer_lines SET status='CANCELLED',updated_at=now() WHERE transfer_id=$1 AND business_id=$2`, id, actor.VendorID); err != nil {
			return err
		}
		if err = tx.QueryRow(ctx, `UPDATE shop_inventory_transfers SET status='CANCELLED',updated_at=now(),version=version+1 WHERE id=$1 AND business_id=$2 RETURNING status,updated_at,version`, id, actor.VendorID).Scan(&out.Status, &out.UpdatedAt, &out.Version); err != nil {
			return err
		}
		if err = loadTransferLines(ctx, tx, actor.VendorID, &out); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{action: "shop.inventory.transfer.cancelled", eventType: "shop.inventory.transfer.cancelled", aggregateType: "shop_inventory_transfer", aggregateID: id, branchID: out.SourceBranchRef, version: out.Version, after: out, payload: out})
	})
	return out, err
}

func (s *Service) DispatchTransfer(ctx context.Context, actor Actor, scope Scope, id string) (InventoryTransfer, error) {
	if err := s.requireWrites(); err != nil {
		return InventoryTransfer{}, err
	}
	var out InventoryTransfer
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var status string
		err := tx.QueryRow(ctx, `SELECT id,source_branch_id,destination_branch_id,status,notes,dispatched_at,received_at,created_at,updated_at,version FROM shop_inventory_transfers WHERE id=$1 AND business_id=$2 FOR UPDATE`, id, actor.VendorID).Scan(&out.ID, &out.SourceBranchRef, &out.DestinationBranchRef, &status, &out.Notes, &out.DispatchedAt, &out.ReceivedAt, &out.CreatedAt, &out.UpdatedAt, &out.Version)
		if err != nil {
			return mapDB(err, "Transfer", CodeTransferNotFound)
		}
		if err = s.requireBranchScope(scope, out.SourceBranchRef); err != nil {
			return err
		}
		if err = s.requireBranchScope(scope, out.DestinationBranchRef); err != nil {
			return err
		}
		if status == "IN_TRANSIT" {
			out.Status = status
			return loadTransferLines(ctx, tx, actor.VendorID, &out)
		}
		if status != "DRAFT" && status != "APPROVED" {
			return coded(apperrors.Conflict("Transfer cannot be dispatched from this state.", nil), CodeReservationStateConflict)
		}
		if err = loadTransferLines(ctx, tx, actor.VendorID, &out); err != nil {
			return err
		}
		sort.Slice(out.Lines, func(i, j int) bool { return out.Lines[i].SourceStockBasisRef < out.Lines[j].SourceStockBasisRef })
		for _, line := range out.Lines {
			_, _, _, err = lockInventoryBalance(ctx, tx, actor.VendorID, line.SourceStockBasisRef)
			if err != nil {
				return err
			}
			var onHand, reserved, quarantined int64
			if err = tx.QueryRow(ctx, `SELECT on_hand_quantity,reserved_quantity,quarantined_quantity FROM shop_inventory_balances WHERE business_id=$1 AND stock_basis_id=$2`, actor.VendorID, line.SourceStockBasisRef).Scan(&onHand, &reserved, &quarantined); err != nil {
				return err
			}
			if line.SentQuantity > onHand-reserved-quarantined {
				return coded(apperrors.Conflict("Transfer exceeds sellable source stock.", nil), CodeInventoryInsufficient)
			}
			remaining := line.SentQuantity
			rows, queryErr := tx.Query(ctx, `SELECT id,on_hand_quantity-reserved_quantity-quarantined_quantity FROM shop_inventory_lots WHERE business_id=$1 AND stock_basis_id=$2 AND status='ACTIVE' AND (expires_at IS NULL OR expires_at>now()) AND on_hand_quantity>reserved_quantity+quarantined_quantity ORDER BY expires_at ASC NULLS LAST,received_at,id FOR UPDATE`, actor.VendorID, line.SourceStockBasisRef)
			if queryErr != nil {
				return queryErr
			}
			type lot struct {
				id        string
				available int64
			}
			var lots []lot
			for rows.Next() {
				var item lot
				if err = rows.Scan(&item.id, &item.available); err != nil {
					rows.Close()
					return err
				}
				lots = append(lots, item)
			}
			rows.Close()
			for _, item := range lots {
				if remaining == 0 {
					break
				}
				take := item.available
				if take > remaining {
					take = remaining
				}
				if _, err = tx.Exec(ctx, `UPDATE shop_inventory_lots SET on_hand_quantity=on_hand_quantity-$3,status=CASE WHEN on_hand_quantity-$3=0 THEN 'DEPLETED' ELSE status END,updated_at=now(),version=version+1 WHERE id=$1 AND business_id=$2`, item.id, actor.VendorID, take); err != nil {
					return err
				}
				remaining -= take
			}
			if remaining != 0 {
				return coded(apperrors.Conflict("Transfer could not allocate valid lots.", nil), CodeInventoryInsufficient)
			}
			if _, err = tx.Exec(ctx, `UPDATE shop_inventory_balances SET on_hand_quantity=on_hand_quantity-$3,in_transit_quantity=in_transit_quantity+$3,revision=revision+1,updated_at=now() WHERE business_id=$1 AND stock_basis_id=$2`, actor.VendorID, line.SourceStockBasisRef, line.SentQuantity); err != nil {
				return err
			}
			revision, revErr := bumpInventoryRevision(ctx, tx, actor.VendorID, out.SourceBranchRef)
			if revErr != nil {
				return revErr
			}
			if _, err = tx.Exec(ctx, `INSERT INTO shop_inventory_movements(id,business_id,branch_id,stock_basis_id,variant_id,movement_type,quantity,on_hand_delta,in_transit_delta,source_type,source_ref,actor_ref,inventory_revision) VALUES($1,$2,$3,$4,$5,'TRANSFER_OUT',$6,$7,$6,'TRANSFER',$8,NULLIF($9,''),$10)`, newID("sim_"), actor.VendorID, out.SourceBranchRef, line.SourceStockBasisRef, line.VariantRef, line.SentQuantity, -line.SentQuantity, id, actor.StaffID, revision); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `UPDATE shop_inventory_transfer_lines SET status='IN_TRANSIT',updated_at=now() WHERE id=$1`, line.ID); err != nil {
				return err
			}
		}
		err = tx.QueryRow(ctx, `UPDATE shop_inventory_transfers SET status='IN_TRANSIT',dispatched_at=now(),updated_at=now(),version=version+1 WHERE id=$1 RETURNING status,dispatched_at,updated_at,version`, id).Scan(&out.Status, &out.DispatchedAt, &out.UpdatedAt, &out.Version)
		if err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{action: "shop.inventory.transfer.dispatched", eventType: "shop.inventory.transfer.dispatched", aggregateType: "shop_inventory_transfer", aggregateID: id, branchID: out.SourceBranchRef, version: out.Version, after: out, payload: out})
	})
	return out, err
}

type ReceiveTransferLineInput struct {
	LineID           string `json:"line_id"`
	ReceivedQuantity int64  `json:"received_quantity"`
	DamagedQuantity  int64  `json:"damaged_quantity"`
	MissingQuantity  int64  `json:"missing_quantity"`
}

func (s *Service) ReceiveTransfer(ctx context.Context, actor Actor, scope Scope, id string, inputs []ReceiveTransferLineInput) (InventoryTransfer, error) {
	if len(inputs) == 0 {
		return InventoryTransfer{}, fieldError("lines", "Receipt lines are required.")
	}
	var out InventoryTransfer
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var status string
		if err := tx.QueryRow(ctx, `SELECT id,source_branch_id,destination_branch_id,status,notes,dispatched_at,received_at,created_at,updated_at,version FROM shop_inventory_transfers WHERE id=$1 AND business_id=$2 FOR UPDATE`, id, actor.VendorID).Scan(&out.ID, &out.SourceBranchRef, &out.DestinationBranchRef, &status, &out.Notes, &out.DispatchedAt, &out.ReceivedAt, &out.CreatedAt, &out.UpdatedAt, &out.Version); err != nil {
			return mapDB(err, "Transfer", CodeTransferNotFound)
		}
		if err := s.requireBranchScope(scope, out.DestinationBranchRef); err != nil {
			return err
		}
		if status != "IN_TRANSIT" && status != "PARTIALLY_RECEIVED" {
			return coded(apperrors.Conflict("Transfer is not in transit.", nil), CodeReservationStateConflict)
		}
		for _, input := range inputs {
			if input.ReceivedQuantity < 0 || input.DamagedQuantity < 0 || input.MissingQuantity < 0 || input.ReceivedQuantity+input.DamagedQuantity+input.MissingQuantity == 0 {
				return fieldError("lines", "Resolved transfer quantities must be non-negative and non-zero.")
			}
			var line InventoryTransferLine
			if err := tx.QueryRow(ctx, `SELECT id,source_stock_basis_id,destination_stock_basis_id,variant_id,sent_quantity,received_quantity,damaged_quantity,missing_quantity,status FROM shop_inventory_transfer_lines WHERE id=$1 AND transfer_id=$2 AND business_id=$3 FOR UPDATE`, input.LineID, id, actor.VendorID).Scan(&line.ID, &line.SourceStockBasisRef, &line.DestinationStockBasisRef, &line.VariantRef, &line.SentQuantity, &line.ReceivedQuantity, &line.DamagedQuantity, &line.MissingQuantity, &line.Status); err != nil {
				return mapDB(err, "Transfer line", CodeTransferNotFound)
			}
			resolved := input.ReceivedQuantity + input.DamagedQuantity + input.MissingQuantity
			if line.ReceivedQuantity+line.DamagedQuantity+line.MissingQuantity+resolved > line.SentQuantity {
				return fieldError("lines", "Resolved quantity exceeds quantity sent.")
			}
			basisOrder := sortedStrings([]string{line.SourceStockBasisRef, line.DestinationStockBasisRef})
			for _, basis := range basisOrder {
				if _, _, _, err := lockInventoryBalance(ctx, tx, actor.VendorID, basis); err != nil {
					return err
				}
			}
			if input.ReceivedQuantity > 0 {
				lotID := newID("sil_")
				if _, err := tx.Exec(ctx, `INSERT INTO shop_inventory_lots(id,business_id,branch_id,stock_basis_id,variant_id,source_ref,received_at,on_hand_quantity) VALUES($1,$2,$3,$4,$5,$6,now(),$7)`, lotID, actor.VendorID, out.DestinationBranchRef, line.DestinationStockBasisRef, line.VariantRef, id, input.ReceivedQuantity); err != nil {
					return err
				}
				if _, err := tx.Exec(ctx, `UPDATE shop_inventory_balances SET on_hand_quantity=on_hand_quantity+$3,revision=revision+1,updated_at=now() WHERE business_id=$1 AND stock_basis_id=$2`, actor.VendorID, line.DestinationStockBasisRef, input.ReceivedQuantity); err != nil {
					return err
				}
				revision, err := bumpInventoryRevision(ctx, tx, actor.VendorID, out.DestinationBranchRef)
				if err != nil {
					return err
				}
				if _, err = tx.Exec(ctx, `INSERT INTO shop_inventory_movements(id,business_id,branch_id,stock_basis_id,variant_id,lot_id,movement_type,quantity,on_hand_delta,source_type,source_ref,actor_ref,inventory_revision) VALUES($1,$2,$3,$4,$5,$6,'TRANSFER_IN',$7,$7,'TRANSFER',$8,NULLIF($9,''),$10)`, newID("sim_"), actor.VendorID, out.DestinationBranchRef, line.DestinationStockBasisRef, line.VariantRef, lotID, input.ReceivedQuantity, id, actor.StaffID, revision); err != nil {
					return err
				}
			}
			if _, err := tx.Exec(ctx, `UPDATE shop_inventory_balances SET in_transit_quantity=in_transit_quantity-$3,revision=revision+1,updated_at=now() WHERE business_id=$1 AND stock_basis_id=$2`, actor.VendorID, line.SourceStockBasisRef, resolved); err != nil {
				return err
			}
			sourceRevision, err := bumpInventoryRevision(ctx, tx, actor.VendorID, out.SourceBranchRef)
			if err != nil {
				return err
			}
			if input.DamagedQuantity+input.MissingQuantity > 0 {
				loss := input.DamagedQuantity + input.MissingQuantity
				if _, err = tx.Exec(ctx, `INSERT INTO shop_inventory_movements(id,business_id,branch_id,stock_basis_id,variant_id,movement_type,quantity,in_transit_delta,source_type,source_ref,reason_code,actor_ref,inventory_revision) VALUES($1,$2,$3,$4,$5,'DAMAGE',$6,$7,'TRANSFER',$8,'TRANSFER_DISCREPANCY',NULLIF($9,''),$10)`, newID("sim_"), actor.VendorID, out.SourceBranchRef, line.SourceStockBasisRef, line.VariantRef, loss, -loss, id, actor.StaffID, sourceRevision); err != nil {
					return err
				}
			}
			newResolved := line.ReceivedQuantity + line.DamagedQuantity + line.MissingQuantity + resolved
			lineStatus := "PARTIALLY_RECEIVED"
			if newResolved == line.SentQuantity {
				lineStatus = "RECEIVED"
			}
			if _, err = tx.Exec(ctx, `UPDATE shop_inventory_transfer_lines SET received_quantity=received_quantity+$2,damaged_quantity=damaged_quantity+$3,missing_quantity=missing_quantity+$4,status=$5,updated_at=now() WHERE id=$1`, line.ID, input.ReceivedQuantity, input.DamagedQuantity, input.MissingQuantity, lineStatus); err != nil {
				return err
			}
		}
		var incomplete int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM shop_inventory_transfer_lines WHERE transfer_id=$1 AND status<>'RECEIVED'`, id).Scan(&incomplete); err != nil {
			return err
		}
		target := "PARTIALLY_RECEIVED"
		if incomplete == 0 {
			target = "RECEIVED"
		}
		err := tx.QueryRow(ctx, `UPDATE shop_inventory_transfers SET status=$2,received_at=CASE WHEN $2='RECEIVED' THEN now() ELSE received_at END,updated_at=now(),version=version+1 WHERE id=$1 RETURNING status,received_at,updated_at,version`, id, target).Scan(&out.Status, &out.ReceivedAt, &out.UpdatedAt, &out.Version)
		if err != nil {
			return err
		}
		if err = loadTransferLines(ctx, tx, actor.VendorID, &out); err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{action: "shop.inventory.transfer.received", eventType: "shop.inventory.transfer.received", aggregateType: "shop_inventory_transfer", aggregateID: id, branchID: out.DestinationBranchRef, version: out.Version, after: out, payload: out})
	})
	return out, err
}

func loadTransferLines(ctx context.Context, q querier, businessID string, out *InventoryTransfer) error {
	rows, err := q.Query(ctx, `SELECT id,source_stock_basis_id,destination_stock_basis_id,variant_id,sent_quantity,received_quantity,damaged_quantity,missing_quantity,status FROM shop_inventory_transfer_lines WHERE transfer_id=$1 AND business_id=$2 ORDER BY id`, out.ID, businessID)
	if err != nil {
		return err
	}
	defer rows.Close()
	out.Lines = nil
	for rows.Next() {
		var line InventoryTransferLine
		if err = rows.Scan(&line.ID, &line.SourceStockBasisRef, &line.DestinationStockBasisRef, &line.VariantRef, &line.SentQuantity, &line.ReceivedQuantity, &line.DamagedQuantity, &line.MissingQuantity, &line.Status); err != nil {
			return err
		}
		out.Lines = append(out.Lines, line)
	}
	return rows.Err()
}
