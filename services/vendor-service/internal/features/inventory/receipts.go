package inventory

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	ReceiptDraft     = "DRAFT"
	ReceiptPosted    = "POSTED"
	ReceiptCancelled = "CANCELLED"
)

type ReceiptLineInput struct {
	InventoryItemID string

	Quantity int64

	LotNumber   string
	BatchNumber string

	ManufacturedAt *time.Time
	BestBeforeAt   *time.Time
	ExpiresAt      *time.Time

	UnitCostMinor *int64
}

type CreateReceiptInput struct {
	BranchID string

	SourceRef   string
	SupplierRef string

	Notes string

	ReceivedAt time.Time

	Lines []ReceiptLineInput
}

type ReceiptLine struct {
	ID string

	InventoryItemID string

	Quantity int64

	LotNumber   string
	BatchNumber string

	ManufacturedAt *time.Time
	BestBeforeAt   *time.Time
	ExpiresAt      *time.Time

	UnitCostMinor *int64

	LotID            string
	PostedMovementID string
}

type Receipt struct {
	ID string

	BusinessID string
	BranchID   string

	SourceRef   string
	SupplierRef string
	Notes       string

	ReceivedAt time.Time

	Status string

	PostedAt    *time.Time
	CancelledAt *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time

	Version int64

	Lines []ReceiptLine
}

func (s *StockService) CreateReceipt(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	input CreateReceiptInput,
) (
	Receipt,
	error,
) {
	if len(input.Lines) == 0 ||
		len(input.Lines) > 500 {
		return Receipt{},
			ErrInvalidQuantity
	}

	if len(input.Notes) > 2000 {
		return Receipt{},
			ErrInvalidQuantity
	}

	if input.ReceivedAt.IsZero() {
		input.ReceivedAt =
			s.foundation.Clock().Now()
	}

	var out Receipt

	err := s.withWriteTransaction(
		ctx,
		actor,
		scope,
		input.BranchID,
		func(
			inventoryTx InventoryTransaction,
			tx pgx.Tx,
		) error {
			for _, line := range input.Lines {

				if line.Quantity <= 0 ||
					line.Quantity >
						maxStockQuantity {

					return ErrInvalidQuantity
				}

				if line.UnitCostMinor != nil &&
					*line.UnitCostMinor < 0 {

					return ErrInvalidQuantity
				}

				_, err :=
					loadStockItemForUpdate(
						ctx,
						tx,
						actor.BusinessID,
						input.BranchID,
						line.InventoryItemID,
					)

				if err != nil {
					return err
				}
			}

			out = Receipt{
				ID: stockID("ir_"),

				BusinessID: actor.BusinessID,

				BranchID: strings.TrimSpace(
					input.BranchID,
				),

				SourceRef: strings.TrimSpace(
					input.SourceRef,
				),

				SupplierRef: strings.TrimSpace(
					input.SupplierRef,
				),

				Notes: input.Notes,

				ReceivedAt: input.ReceivedAt.UTC(),

				Status: ReceiptDraft,

				Version: 1,
			}

			err := tx.QueryRow(
				ctx,
				`
                INSERT INTO inventory_receipts (
                    id,
                    business_id,
                    branch_id,
                    source_ref,
                    supplier_ref,
                    notes,
                    received_at
                )
                VALUES (
                    $1,$2,$3,
                    NULLIF($4,''),
                    NULLIF($5,''),
                    $6,$7
                )
                RETURNING
                    created_at,
                    updated_at
                `,
				out.ID,
				out.BusinessID,
				out.BranchID,
				out.SourceRef,
				out.SupplierRef,
				out.Notes,
				out.ReceivedAt,
			).Scan(
				&out.CreatedAt,
				&out.UpdatedAt,
			)

			if err != nil {
				return err
			}

			for _, inputLine := range input.Lines {

				line := ReceiptLine{
					ID: stockID("irl_"),

					InventoryItemID: strings.TrimSpace(
						inputLine.InventoryItemID,
					),

					Quantity: inputLine.Quantity,

					LotNumber: strings.TrimSpace(
						inputLine.LotNumber,
					),

					BatchNumber: strings.TrimSpace(
						inputLine.BatchNumber,
					),

					ManufacturedAt: inputLine.ManufacturedAt,

					BestBeforeAt: inputLine.BestBeforeAt,

					ExpiresAt: inputLine.ExpiresAt,

					UnitCostMinor: inputLine.UnitCostMinor,
				}

				_, err := tx.Exec(
					ctx,
					`
                    INSERT INTO inventory_receipt_lines (
                        id,
                        business_id,
                        branch_id,
                        receipt_id,
                        inventory_item_id,
                        quantity,
                        lot_number,
                        batch_number,
                        manufactured_at,
                        best_before_at,
                        expires_at,
                        unit_cost_minor
                    )
                    VALUES (
                        $1,$2,$3,$4,$5,$6,
                        NULLIF($7,''),
                        NULLIF($8,''),
                        $9,$10,$11,$12
                    )
                    `,
					line.ID,
					out.BusinessID,
					out.BranchID,
					out.ID,
					line.InventoryItemID,
					line.Quantity,
					line.LotNumber,
					line.BatchNumber,
					line.ManufacturedAt,
					line.BestBeforeAt,
					line.ExpiresAt,
					line.UnitCostMinor,
				)

				if err != nil {
					return err
				}

				out.Lines = append(
					out.Lines,
					line,
				)
			}

			return s.recordMutation(
				ctx,
				inventoryTx,
				actor,
				scope,
				"inventory.receipt.created",
				"inventory.receipt.created",
				"inventory_receipt",
				out.ID,
				"",
				1,
				map[string]any{
					"receipt": out,
				},
			)
		},
	)

	return out, err
}

func loadReceiptForUpdate(
	ctx context.Context,
	tx pgx.Tx,
	businessID string,
	receiptID string,
) (
	Receipt,
	error,
) {
	var out Receipt

	err := tx.QueryRow(
		ctx,
		`
        SELECT
            id,
            business_id,
            branch_id,
            COALESCE(source_ref,''),
            COALESCE(supplier_ref,''),
            notes,
            received_at,
            status,
            posted_at,
            cancelled_at,
            created_at,
            updated_at,
            version
          FROM inventory_receipts
         WHERE id=$1
           AND business_id=$2
         FOR UPDATE
        `,
		receiptID,
		businessID,
	).Scan(
		&out.ID,
		&out.BusinessID,
		&out.BranchID,
		&out.SourceRef,
		&out.SupplierRef,
		&out.Notes,
		&out.ReceivedAt,
		&out.Status,
		&out.PostedAt,
		&out.CancelledAt,
		&out.CreatedAt,
		&out.UpdatedAt,
		&out.Version,
	)

	if errors.Is(
		err,
		pgx.ErrNoRows,
	) {
		return Receipt{},
			ErrReceiptNotFound
	}

	if err != nil {
		return Receipt{}, err
	}

	rows, err := tx.Query(
		ctx,
		`
        SELECT
            id,
            inventory_item_id,
            quantity,
            COALESCE(lot_number,''),
            COALESCE(batch_number,''),
            manufactured_at,
            best_before_at,
            expires_at,
            unit_cost_minor,
            COALESCE(lot_id,''),
            COALESCE(posted_movement_id,'')
          FROM inventory_receipt_lines
         WHERE receipt_id=$1
           AND business_id=$2
         ORDER BY id
        `,
		out.ID,
		out.BusinessID,
	)

	if err != nil {
		return Receipt{}, err
	}

	defer rows.Close()

	for rows.Next() {
		var line ReceiptLine

		if err := rows.Scan(
			&line.ID,
			&line.InventoryItemID,
			&line.Quantity,
			&line.LotNumber,
			&line.BatchNumber,
			&line.ManufacturedAt,
			&line.BestBeforeAt,
			&line.ExpiresAt,
			&line.UnitCostMinor,
			&line.LotID,
			&line.PostedMovementID,
		); err != nil {
			return Receipt{}, err
		}

		out.Lines = append(
			out.Lines,
			line,
		)
	}

	return out, rows.Err()
}

func (s *StockService) PostReceipt(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	receiptID string,
) (
	Receipt,
	error,
) {
	if err := s.requireDependencies(); err != nil {
		return Receipt{}, err
	}

	var out Receipt

	var branchID string

	err := s.transactions.WithinInventoryTransaction(
		ctx,
		func(inventoryTx InventoryTransaction) error {
			tx, err :=
				postgresTransaction(
					inventoryTx,
				)

			if err != nil {
				return err
			}

			receipt, err :=
				loadReceiptForUpdate(
					ctx,
					tx,
					actor.BusinessID,
					receiptID,
				)

			if err != nil {
				return err
			}

			branchID =
				receipt.BranchID

			if err := s.foundation.RequireBranch(
				ctx,
				actor,
				scope,
				actor.BusinessID,
				branchID,
				true,
			); err != nil {
				return err
			}

			if receipt.Status !=
				ReceiptDraft {

				return ErrReceiptStateConflict
			}

			if err := enableInventoryMutation(
				ctx,
				tx,
			); err != nil {
				return err
			}

			var lastRevision int64

			for index := range receipt.Lines {

				line :=
					&receipt.Lines[index]

				item, err :=
					loadStockItemForUpdate(
						ctx,
						tx,
						actor.BusinessID,
						receipt.BranchID,
						line.InventoryItemID,
					)

				if err != nil {
					return err
				}

				_,
					movement,
					lotID,
					err :=
					s.increasePhysicalTx(
						ctx,
						tx,
						actor,
						item,
						line.Quantity,
						MovementReceive,
						"RECEIPT",
						line.ID,
						"",
						LotMetadata{
							LotNumber: line.LotNumber,

							BatchNumber: line.BatchNumber,

							ManufacturedAt: line.ManufacturedAt,

							BestBeforeAt: line.BestBeforeAt,

							ExpiresAt: line.ExpiresAt,
						},
					)

				if err != nil {
					return err
				}

				line.LotID =
					lotID

				line.PostedMovementID =
					movement.ID

				lastRevision =
					movement.InventoryRevision

				_, err = tx.Exec(
					ctx,
					`
                    UPDATE inventory_receipt_lines
                       SET lot_id=$3,
                           posted_movement_id=$4
                     WHERE id=$1
                       AND business_id=$2
                    `,
					line.ID,
					receipt.BusinessID,
					line.LotID,
					line.PostedMovementID,
				)

				if err != nil {
					return err
				}
			}

			now :=
				s.foundation.Clock().Now()

			receipt.Status =
				ReceiptPosted

			receipt.PostedAt =
				&now

			receipt.Version++

			err = tx.QueryRow(
				ctx,
				`
                UPDATE inventory_receipts
                   SET status='POSTED',
                       posted_at=$3,
                       updated_at=now(),
                       version=version+1
                 WHERE id=$1
                   AND business_id=$2
                RETURNING updated_at
                `,
				receipt.ID,
				receipt.BusinessID,
				now,
			).Scan(
				&receipt.UpdatedAt,
			)

			if err != nil {
				return err
			}

			out = receipt

			return s.recordMutation(
				ctx,
				inventoryTx,
				actor,
				scope,
				"inventory.receipt.posted",
				"inventory.receipt.posted",
				"inventory_receipt",
				receipt.ID,
				"",
				lastRevision,
				map[string]any{
					"receipt": receipt,
				},
			)
		},
	)

	return out, err
}

func (s *StockService) CancelReceipt(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	receiptID string,
) (
	Receipt,
	error,
) {
	if err := s.requireDependencies(); err != nil {
		return Receipt{}, err
	}

	var out Receipt

	err := s.transactions.WithinInventoryTransaction(
		ctx,
		func(inventoryTx InventoryTransaction) error {
			tx, err :=
				postgresTransaction(
					inventoryTx,
				)

			if err != nil {
				return err
			}

			receipt, err :=
				loadReceiptForUpdate(
					ctx,
					tx,
					actor.BusinessID,
					receiptID,
				)

			if err != nil {
				return err
			}

			if err := s.foundation.RequireBranch(
				ctx,
				actor,
				scope,
				actor.BusinessID,
				receipt.BranchID,
				true,
			); err != nil {
				return err
			}

			if receipt.Status ==
				ReceiptCancelled {

				return ErrReceiptStateConflict
			}

			now :=
				s.foundation.Clock().Now()

			var lastRevision int64

			if receipt.Status ==
				ReceiptPosted {

				if err := enableInventoryMutation(
					ctx,
					tx,
				); err != nil {
					return err
				}

				// Prove every receipt-created lot remains completely intact
				// before reversing any one of them.
				for _, line := range receipt.Lines {

					if line.LotID == "" {
						return ErrReceiptNotReversible
					}

					var (
						onHand      int64
						reserved    int64
						quarantined int64
						sourceType  string
						sourceRef   string
						status      string
					)

					err := tx.QueryRow(
						ctx,
						`
                        SELECT
                            on_hand_quantity,
                            reserved_quantity,
                            quarantined_quantity,
                            source_type,
                            source_ref,
                            status
                          FROM inventory_lots
                         WHERE id=$1
                           AND business_id=$2
                           AND inventory_item_id=$3
                         FOR UPDATE
                        `,
						line.LotID,
						receipt.BusinessID,
						line.InventoryItemID,
					).Scan(
						&onHand,
						&reserved,
						&quarantined,
						&sourceType,
						&sourceRef,
						&status,
					)

					if err != nil ||
						onHand != line.Quantity ||
						reserved != 0 ||
						quarantined != 0 ||
						sourceType != "RECEIPT" ||
						sourceRef != line.ID ||
						status != "ACTIVE" {

						return ErrReceiptNotReversible
					}
				}

				for _, line := range receipt.Lines {

					item, err :=
						loadStockItemForUpdate(
							ctx,
							tx,
							receipt.BusinessID,
							receipt.BranchID,
							line.InventoryItemID,
						)

					if err != nil {
						return err
					}

					_, err = tx.Exec(
						ctx,
						`
                        UPDATE inventory_lots
                           SET on_hand_quantity=0,
                               status='ARCHIVED',
                               updated_at=now(),
                               version=version+1
                         WHERE id=$1
                           AND business_id=$2
                        `,
						line.LotID,
						receipt.BusinessID,
					)

					if err != nil {
						return err
					}

					balanceRevision, err :=
						updateBalanceTx(
							ctx,
							tx,
							item,
							-line.Quantity,
							0,
							0,
							0,
							0,
							0,
						)

					if err != nil {
						return ErrReceiptNotReversible
					}

					inventoryRevision, err :=
						bumpInventoryRevisionTx(
							ctx,
							tx,
							item.BusinessID,
							item.BranchID,
						)

					if err != nil {
						return err
					}

					_, err =
						appendMovementTx(
							ctx,
							tx,
							actor,
							item,
							line.LotID,
							MovementAdjustmentOut,
							line.Quantity,
							-line.Quantity,
							0,
							0,
							0,
							0,
							0,
							"RECEIPT_CANCEL",
							receipt.ID,
							"RECEIPT_CANCELLED",
							inventoryRevision,
							balanceRevision,
						)

					if err != nil {
						return err
					}

					lastRevision =
						inventoryRevision
				}
			}

			receipt.Status =
				ReceiptCancelled

			receipt.CancelledAt =
				&now

			receipt.Version++

			err = tx.QueryRow(
				ctx,
				`
                UPDATE inventory_receipts
                   SET status='CANCELLED',
                       cancelled_at=$3,
                       updated_at=now(),
                       version=version+1
                 WHERE id=$1
                   AND business_id=$2
                RETURNING updated_at
                `,
				receipt.ID,
				receipt.BusinessID,
				now,
			).Scan(
				&receipt.UpdatedAt,
			)

			if err != nil {
				return err
			}

			out = receipt

			if lastRevision == 0 {
				lastRevision = 1
			}

			return s.recordMutation(
				ctx,
				inventoryTx,
				actor,
				scope,
				"inventory.receipt.cancelled",
				"inventory.receipt.cancelled",
				"inventory_receipt",
				receipt.ID,
				"",
				lastRevision,
				map[string]any{
					"receipt": receipt,
				},
			)
		},
	)

	return out, err
}
