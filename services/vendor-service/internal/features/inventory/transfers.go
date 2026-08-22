package inventory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

type transferIdentity struct {
	ItemType string

	SourceRef string

	MeasureRef      string
	MeasureOwnerKey string

	BasisMode string
}

func loadTransferIdentityQ(
	ctx context.Context,
	q interface {
		QueryRow(
			context.Context,
			string,
			...any,
		) pgx.Row
	},
	businessID string,
	branchID string,
	itemID string,
) (
	transferIdentity,
	error,
) {
	var out transferIdentity

	err :=
		q.QueryRow(
			ctx,
			`
            SELECT
                item_type,
                source_ref,
                stock_measure_ref,
                stock_measure_owner_key,
                stock_basis_mode
              FROM inventory_items
             WHERE id=$1
               AND business_id=$2
               AND branch_id=$3
               AND status='ACTIVE'
            `,
			itemID,
			businessID,
			branchID,
		).Scan(
			&out.ItemType,
			&out.SourceRef,
			&out.MeasureRef,
			&out.MeasureOwnerKey,
			&out.BasisMode,
		)

	if errors.Is(
		err,
		pgx.ErrNoRows,
	) {
		return transferIdentity{},
			ErrItemNotFound
	}

	return out, err
}

func transferIdentitiesMatch(
	left transferIdentity,
	right transferIdentity,
) bool {
	return left.ItemType ==
		right.ItemType &&
		left.SourceRef ==
			right.SourceRef &&
		left.MeasureRef ==
			right.MeasureRef &&
		left.MeasureOwnerKey ==
			right.MeasureOwnerKey &&
		left.BasisMode ==
			right.BasisMode
}

func normalizeTransferLines(
	lines []TransferLineInput,
) (
	[]TransferLineInput,
	error,
) {
	if len(lines) == 0 ||
		len(lines) > 200 {
		return nil,
			ErrInvalidQuantity
	}

	out :=
		append(
			[]TransferLineInput(nil),
			lines...,
		)

	for index := range out {

		out[index].SourceInventoryItemID =
			strings.TrimSpace(
				out[index].
					SourceInventoryItemID,
			)

		out[index].
			DestinationInventoryItemID =
			strings.TrimSpace(
				out[index].
					DestinationInventoryItemID,
			)

		if out[index].
			SourceInventoryItemID == "" ||
			out[index].
				DestinationInventoryItemID == "" ||
			out[index].Quantity <= 0 ||
			out[index].Quantity >
				maxStockQuantity {

			return nil,
				ErrInvalidQuantity
		}
	}

	sort.Slice(
		out,
		func(
			i int,
			j int,
		) bool {
			return out[i].
				SourceInventoryItemID <
				out[j].
					SourceInventoryItemID
		},
	)

	for index := 1; index < len(out); index++ {

		if out[index-1].
			SourceInventoryItemID ==
			out[index].
				SourceInventoryItemID {

			return nil,
				ErrInvalidSource
		}
	}

	return out, nil
}

func (s *StockService) CreateTransfer(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	input CreateTransferInput,
) (
	Transfer,
	error,
) {
	input.SourceBranchID =
		strings.TrimSpace(
			input.SourceBranchID,
		)

	input.DestinationBranchID =
		strings.TrimSpace(
			input.DestinationBranchID,
		)

	if input.SourceBranchID == "" ||
		input.DestinationBranchID == "" ||
		input.SourceBranchID ==
			input.DestinationBranchID {

		return Transfer{},
			ErrInvalidSource
	}

	if len(input.Notes) > 2000 {
		return Transfer{},
			ErrInvalidSource
	}

	lines, err :=
		normalizeTransferLines(
			input.Lines,
		)

	if err != nil {
		return Transfer{}, err
	}

	if err := s.foundation.RequireBranch(
		ctx,
		actor,
		scope,
		actor.BusinessID,
		input.DestinationBranchID,
		true,
	); err != nil {
		return Transfer{}, err
	}

	var out Transfer

	err = s.withWriteTransaction(
		ctx,
		actor,
		scope,
		input.SourceBranchID,
		func(
			inventoryTx InventoryTransaction,
			tx pgx.Tx,
		) error {
			out = Transfer{
				ID: stockID("it_"),

				BusinessID: actor.BusinessID,

				SourceBranchID: input.SourceBranchID,

				DestinationBranchID: input.DestinationBranchID,

				Status: TransferDraft,

				Notes: input.Notes,

				Version: 1,
			}

			err :=
				tx.QueryRow(
					ctx,
					`
                    INSERT INTO inventory_transfers (
                        id,
                        business_id,
                        source_branch_id,
                        destination_branch_id,
                        notes
                    )
                    VALUES (
                        $1,$2,$3,$4,$5
                    )
                    RETURNING
                        created_at,
                        updated_at
                    `,
					out.ID,
					out.BusinessID,
					out.SourceBranchID,
					out.DestinationBranchID,
					out.Notes,
				).Scan(
					&out.CreatedAt,
					&out.UpdatedAt,
				)

			if err != nil {
				return err
			}

			for _, inputLine := range lines {

				sourceIdentity, err :=
					loadTransferIdentityQ(
						ctx,
						tx,
						actor.BusinessID,
						input.SourceBranchID,
						inputLine.
							SourceInventoryItemID,
					)

				if err != nil {
					return err
				}

				destinationIdentity, err :=
					loadTransferIdentityQ(
						ctx,
						tx,
						actor.BusinessID,
						input.DestinationBranchID,
						inputLine.
							DestinationInventoryItemID,
					)

				if err != nil {
					return err
				}

				if !transferIdentitiesMatch(
					sourceIdentity,
					destinationIdentity,
				) {
					return ErrTransferIdentityMismatch
				}

				line :=
					TransferLine{
						ID: stockID("itl_"),

						SourceInventoryItemID: inputLine.
							SourceInventoryItemID,

						DestinationInventoryItemID: inputLine.
							DestinationInventoryItemID,

						SentQuantity: inputLine.Quantity,

						Status: TransferLinePending,
					}

				_, err =
					tx.Exec(
						ctx,
						`
                        INSERT INTO inventory_transfer_lines (
                            id,
                            business_id,
                            transfer_id,
                            source_branch_id,
                            destination_branch_id,
                            source_inventory_item_id,
                            destination_inventory_item_id,
                            sent_quantity
                        )
                        VALUES (
                            $1,$2,$3,$4,$5,$6,$7,$8
                        )
                        `,
						line.ID,
						actor.BusinessID,
						out.ID,
						input.SourceBranchID,
						input.DestinationBranchID,
						line.SourceInventoryItemID,
						line.DestinationInventoryItemID,
						line.SentQuantity,
					)

				if err != nil {
					return err
				}

				out.Lines =
					append(
						out.Lines,
						line,
					)
			}

			exactScope, _ :=
				ExactBranchScope(
					actor.BusinessID,
					input.SourceBranchID,
				)

			revision, _ :=
				currentInventoryRevisionQ(
					ctx,
					tx,
					actor.BusinessID,
					input.SourceBranchID,
				)

			if revision == 0 {
				revision = 1
			}

			return s.recordMutation(
				ctx,
				inventoryTx,
				actor,
				exactScope,
				"inventory.transfer.created",
				"inventory.transfer.created",
				"inventory_transfer",
				out.ID,
				"",
				revision,
				map[string]any{
					"transfer": out,
				},
			)
		},
	)

	return out, err
}

func loadTransferQ(
	ctx context.Context,
	q reservationQuerier,
	businessID string,
	id string,
	forUpdate bool,
) (
	Transfer,
	error,
) {
	lock :=
		""

	if forUpdate {
		lock =
			" FOR UPDATE"
	}

	var out Transfer

	err :=
		q.QueryRow(
			ctx,
			`
            SELECT
                id,
                business_id,
                source_branch_id,
                destination_branch_id,
                status,
                notes,
                approved_at,
                dispatched_at,
                received_at,
                cancelled_at,
                created_at,
                updated_at,
                version
              FROM inventory_transfers
             WHERE id=$1
               AND business_id=$2
            `+lock,
			id,
			businessID,
		).Scan(
			&out.ID,
			&out.BusinessID,
			&out.SourceBranchID,
			&out.DestinationBranchID,
			&out.Status,
			&out.Notes,
			&out.ApprovedAt,
			&out.DispatchedAt,
			&out.ReceivedAt,
			&out.CancelledAt,
			&out.CreatedAt,
			&out.UpdatedAt,
			&out.Version,
		)

	if errors.Is(
		err,
		pgx.ErrNoRows,
	) {
		return Transfer{},
			ErrTransferNotFound
	}

	if err != nil {
		return Transfer{}, err
	}

	rows, err :=
		q.Query(
			ctx,
			`
            SELECT
                id,
                source_inventory_item_id,
                destination_inventory_item_id,
                sent_quantity,
                received_quantity,
                damaged_quantity,
                missing_quantity,
                status
              FROM inventory_transfer_lines
             WHERE transfer_id=$1
               AND business_id=$2
             ORDER BY source_inventory_item_id,id
            `,
			out.ID,
			out.BusinessID,
		)

	if err != nil {
		return Transfer{}, err
	}

	defer rows.Close()

	for rows.Next() {
		var line TransferLine

		if err := rows.Scan(
			&line.ID,
			&line.SourceInventoryItemID,
			&line.DestinationInventoryItemID,
			&line.SentQuantity,
			&line.ReceivedQuantity,
			&line.DamagedQuantity,
			&line.MissingQuantity,
			&line.Status,
		); err != nil {
			return Transfer{}, err
		}

		out.Lines =
			append(
				out.Lines,
				line,
			)
	}

	return out, rows.Err()
}

func (s *StockService) TransferBranches(
	ctx context.Context,
	businessID string,
	id string,
) (
	string,
	string,
	error,
) {
	var (
		source      string
		destination string
	)

	err :=
		s.db.QueryRow(
			ctx,
			`
            SELECT
                source_branch_id,
                destination_branch_id
              FROM inventory_transfers
             WHERE id=$1
               AND business_id=$2
            `,
			id,
			businessID,
		).Scan(
			&source,
			&destination,
		)

	if errors.Is(
		err,
		pgx.ErrNoRows,
	) {
		return "",
			"",
			ErrTransferNotFound
	}

	return source,
		destination,
		err
}

func (s *StockService) GetTransfer(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	id string,
) (
	Transfer,
	error,
) {
	out, err :=
		loadTransferQ(
			ctx,
			s.db,
			actor.BusinessID,
			id,
			false,
		)

	if err != nil {
		return Transfer{}, err
	}

	if err :=
		s.foundation.RequireBranch(
			ctx,
			actor,
			scope,
			actor.BusinessID,
			out.SourceBranchID,
			false,
		); err != nil {

		return Transfer{}, err
	}

	if err :=
		s.foundation.RequireBranch(
			ctx,
			actor,
			scope,
			actor.BusinessID,
			out.DestinationBranchID,
			false,
		); err != nil {

		return Transfer{}, err
	}

	return out, nil
}

func (s *StockService) ApproveTransfer(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	id string,
) (
	Transfer,
	error,
) {
	return s.transitionTransferHeader(
		ctx,
		actor,
		scope,
		id,
		TransferApproved,
	)
}

func (s *StockService) CancelTransfer(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	id string,
) (
	Transfer,
	error,
) {
	return s.transitionTransferHeader(
		ctx,
		actor,
		scope,
		id,
		TransferCancelled,
	)
}

func (s *StockService) transitionTransferHeader(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	id string,
	target string,
) (
	Transfer,
	error,
) {
	source,
		destination,
		err :=
		s.TransferBranches(
			ctx,
			actor.BusinessID,
			id,
		)

	if err != nil {
		return Transfer{}, err
	}

	if err :=
		s.foundation.RequireBranch(
			ctx,
			actor,
			scope,
			actor.BusinessID,
			destination,
			true,
		); err != nil {

		return Transfer{}, err
	}

	var out Transfer

	err = s.withWriteTransaction(
		ctx,
		actor,
		scope,
		source,
		func(
			inventoryTx InventoryTransaction,
			tx pgx.Tx,
		) error {
			current, err :=
				loadTransferQ(
					ctx,
					tx,
					actor.BusinessID,
					id,
					true,
				)

			if err != nil {
				return err
			}

			if current.Status ==
				target {

				out = current
				return nil
			}

			switch target {
			case TransferApproved:
				if current.Status !=
					TransferDraft {

					return ErrTransferStateConflict
				}

				now :=
					s.foundation.Clock().Now()

				current.ApprovedAt =
					&now

				err =
					tx.QueryRow(
						ctx,
						`
                        UPDATE inventory_transfers
                           SET status='APPROVED',
                               approved_at=$3,
                               updated_at=now(),
                               version=version+1
                         WHERE id=$1
                           AND business_id=$2
                        RETURNING
                            status,
                            updated_at,
                            version
                        `,
						current.ID,
						current.BusinessID,
						now,
					).Scan(
						&current.Status,
						&current.UpdatedAt,
						&current.Version,
					)

			case TransferCancelled:
				if current.Status !=
					TransferDraft &&
					current.Status !=
						TransferApproved {

					return ErrTransferStateConflict
				}

				now :=
					s.foundation.Clock().Now()

				current.CancelledAt =
					&now

				_, err =
					tx.Exec(
						ctx,
						`
                        UPDATE inventory_transfer_lines
                           SET status='CANCELLED',
                               updated_at=now()
                         WHERE transfer_id=$1
                           AND business_id=$2
                        `,
						current.ID,
						current.BusinessID,
					)

				if err == nil {
					err =
						tx.QueryRow(
							ctx,
							`
                            UPDATE inventory_transfers
                               SET status='CANCELLED',
                                   cancelled_at=$3,
                                   updated_at=now(),
                                   version=version+1
                             WHERE id=$1
                               AND business_id=$2
                            RETURNING
                                status,
                                updated_at,
                                version
                            `,
							current.ID,
							current.BusinessID,
							now,
						).Scan(
							&current.Status,
							&current.UpdatedAt,
							&current.Version,
						)
				}

			default:
				return ErrTransferStateConflict
			}

			if err != nil {
				return err
			}

			out, err =
				loadTransferQ(
					ctx,
					tx,
					actor.BusinessID,
					current.ID,
					false,
				)

			if err != nil {
				return err
			}

			exactScope, _ :=
				ExactBranchScope(
					actor.BusinessID,
					source,
				)

			revision, _ :=
				currentInventoryRevisionQ(
					ctx,
					tx,
					actor.BusinessID,
					source,
				)

			if revision == 0 {
				revision = 1
			}

			return s.recordMutation(
				ctx,
				inventoryTx,
				actor,
				exactScope,
				"inventory.transfer."+
					strings.ToLower(target),
				"inventory.transfer."+
					strings.ToLower(target),
				"inventory_transfer",
				out.ID,
				target,
				revision,
				map[string]any{
					"transfer": out,
				},
			)
		},
	)

	return out, err
}

func (s *StockService) DispatchTransfer(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	id string,
) (
	Transfer,
	error,
) {
	source,
		destination,
		err :=
		s.TransferBranches(
			ctx,
			actor.BusinessID,
			id,
		)

	if err != nil {
		return Transfer{}, err
	}

	if err :=
		s.foundation.RequireBranch(
			ctx,
			actor,
			scope,
			actor.BusinessID,
			destination,
			true,
		); err != nil {

		return Transfer{}, err
	}

	var out Transfer

	err = s.withWriteTransaction(
		ctx,
		actor,
		scope,
		source,
		func(
			inventoryTx InventoryTransaction,
			tx pgx.Tx,
		) error {
			current, err :=
				loadTransferQ(
					ctx,
					tx,
					actor.BusinessID,
					id,
					true,
				)

			if err != nil {
				return err
			}

			if current.Status ==
				TransferInTransit {

				out = current
				return nil
			}

			if current.Status !=
				TransferDraft &&
				current.Status !=
					TransferApproved {

				return ErrTransferStateConflict
			}

			itemIDs :=
				make(
					[]string,
					0,
					len(current.Lines),
				)

			for _, line := range current.Lines {

				itemIDs =
					append(
						itemIDs,
						line.SourceInventoryItemID,
					)
			}

			states, err :=
				lockItemsDeterministicallyTx(
					ctx,
					tx,
					actor.BusinessID,
					source,
					itemIDs,
				)

			if err != nil {
				return err
			}

			var lastRevision int64

			for _, line := range current.Lines {

				item :=
					states[line.SourceInventoryItemID]

				allocations, err :=
					allocateAvailableLotsTx(
						ctx,
						tx,
						item,
						line.SentQuantity,
					)

				if err != nil {
					return err
				}

				for _, allocation := range allocations {

					tag, err :=
						tx.Exec(
							ctx,
							`
                            UPDATE inventory_lots
                               SET on_hand_quantity=
                                   on_hand_quantity-$3,
                                   status=
                                   CASE
                                       WHEN on_hand_quantity-$3=0
                                       THEN 'DEPLETED'
                                       ELSE status
                                   END,
                                   updated_at=now(),
                                   version=version+1
                             WHERE id=$1
                               AND business_id=$2
                               AND (
                                   on_hand_quantity
                                   - reserved_quantity
                                   - quarantined_quantity
                               ) >= $3
                            `,
							allocation.ID,
							actor.BusinessID,
							allocation.Quantity,
						)

					if err != nil {
						return err
					}

					if tag.RowsAffected() != 1 {
						return ErrInsufficientStock
					}
				}

				balanceRevision, err :=
					updateBalanceTx(
						ctx,
						tx,
						item,
						-line.SentQuantity,
						0,
						0,
						line.SentQuantity,
						0,
						0,
					)

				if err != nil {
					return err
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

				lastRevision =
					inventoryRevision

				for _, allocation := range allocations {

					_, err :=
						appendMovementTx(
							ctx,
							tx,
							actor,
							item,
							allocation.ID,
							MovementTransferOut,
							allocation.Quantity,
							-allocation.Quantity,
							0,
							0,
							allocation.Quantity,
							0,
							0,
							"TRANSFER",
							current.ID,
							"TRANSFER_DISPATCH",
							inventoryRevision,
							balanceRevision,
						)

					if err != nil {
						return err
					}
				}

				_, err =
					tx.Exec(
						ctx,
						`
                        UPDATE inventory_transfer_lines
                           SET status='IN_TRANSIT',
                               updated_at=now()
                         WHERE id=$1
                           AND business_id=$2
                        `,
						line.ID,
						actor.BusinessID,
					)

				if err != nil {
					return err
				}
			}

			now :=
				s.foundation.Clock().Now()

			err =
				tx.QueryRow(
					ctx,
					`
                    UPDATE inventory_transfers
                       SET status='IN_TRANSIT',
                           dispatched_at=$3,
                           updated_at=now(),
                           version=version+1
                     WHERE id=$1
                       AND business_id=$2
                    RETURNING
                        status,
                        dispatched_at,
                        updated_at,
                        version
                    `,
					current.ID,
					current.BusinessID,
					now,
				).Scan(
					&current.Status,
					&current.DispatchedAt,
					&current.UpdatedAt,
					&current.Version,
				)

			if err != nil {
				return err
			}

			out, err =
				loadTransferQ(
					ctx,
					tx,
					actor.BusinessID,
					current.ID,
					false,
				)

			if err != nil {
				return err
			}

			exactScope, _ :=
				ExactBranchScope(
					actor.BusinessID,
					source,
				)

			if lastRevision == 0 {
				lastRevision = 1
			}

			return s.recordMutation(
				ctx,
				inventoryTx,
				actor,
				exactScope,
				"inventory.transfer.dispatched",
				"inventory.transfer.dispatched",
				"inventory_transfer",
				out.ID,
				"",
				lastRevision,
				map[string]any{
					"transfer": out,
				},
			)
		},
	)

	return out, err
}

func transferReceiptFingerprint(
	input ReceiveTransferInput,
) (
	ReceiveTransferInput,
	string,
	[]byte,
	error,
) {
	input.SourceRef =
		strings.TrimSpace(
			input.SourceRef,
		)

	if input.SourceRef == "" ||
		len(input.Lines) == 0 ||
		len(input.Lines) > 200 {

		return ReceiveTransferInput{},
			"",
			nil,
			ErrInvalidSource
	}

	out :=
		append(
			[]ReceiveTransferLineInput(nil),
			input.Lines...,
		)

	for index := range out {

		out[index].LineID =
			strings.TrimSpace(
				out[index].LineID,
			)

		if out[index].LineID == "" ||
			out[index].ReceivedQuantity < 0 ||
			out[index].DamagedQuantity < 0 ||
			out[index].MissingQuantity < 0 ||
			out[index].ReceivedQuantity+
				out[index].DamagedQuantity+
				out[index].MissingQuantity <= 0 {

			return ReceiveTransferInput{},
				"",
				nil,
				ErrInvalidQuantity
		}
	}

	sort.Slice(
		out,
		func(
			i int,
			j int,
		) bool {
			return out[i].LineID <
				out[j].LineID
		},
	)

	for index := 1; index < len(out); index++ {

		if out[index-1].LineID ==
			out[index].LineID {

			return ReceiveTransferInput{},
				"",
				nil,
				ErrInvalidSource
		}
	}

	input.Lines = out

	payload, err :=
		json.Marshal(
			input,
		)

	if err != nil {
		return ReceiveTransferInput{},
			"",
			nil,
			err
	}

	sum :=
		sha256.Sum256(
			payload,
		)

	return input,
		hex.EncodeToString(
			sum[:],
		),
		payload,
		nil
}

func lockTransferItemsTx(
	ctx context.Context,
	tx pgx.Tx,
	businessID string,
	transfer Transfer,
) (
	map[string]stockItemState,
	error,
) {
	branchByItem :=
		map[string]string{}

	var ids []string

	for _, line := range transfer.Lines {

		if _, exists :=
			branchByItem[line.SourceInventoryItemID]; !exists {

			branchByItem[line.SourceInventoryItemID] =
				transfer.SourceBranchID

			ids =
				append(
					ids,
					line.SourceInventoryItemID,
				)
		}

		if _, exists :=
			branchByItem[line.DestinationInventoryItemID]; !exists {

			branchByItem[line.DestinationInventoryItemID] =
				transfer.DestinationBranchID

			ids =
				append(
					ids,
					line.DestinationInventoryItemID,
				)
		}
	}

	sort.Strings(ids)

	states :=
		make(
			map[string]stockItemState,
			len(ids),
		)

	for _, itemID := range ids {

		item, err :=
			loadStockItemForUpdate(
				ctx,
				tx,
				businessID,
				branchByItem[itemID],
				itemID,
			)

		if err != nil {
			return nil, err
		}

		states[itemID] =
			item
	}

	return states, nil
}

func (s *StockService) ReceiveTransfer(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	id string,
	input ReceiveTransferInput,
) (
	Transfer,
	error,
) {
	source,
		destination,
		err :=
		s.TransferBranches(
			ctx,
			actor.BusinessID,
			id,
		)

	if err != nil {
		return Transfer{}, err
	}

	if err :=
		s.foundation.RequireBranch(
			ctx,
			actor,
			scope,
			actor.BusinessID,
			destination,
			true,
		); err != nil {

		return Transfer{}, err
	}

	normalized,
		fingerprint,
		payload,
		err :=
		transferReceiptFingerprint(
			input,
		)

	if err != nil {
		return Transfer{}, err
	}

	var out Transfer

	err = s.withWriteTransaction(
		ctx,
		actor,
		scope,
		source,
		func(
			inventoryTx InventoryTransaction,
			tx pgx.Tx,
		) error {
			current, err :=
				loadTransferQ(
					ctx,
					tx,
					actor.BusinessID,
					id,
					true,
				)

			if err != nil {
				return err
			}

			var existingFingerprint string

			err =
				tx.QueryRow(
					ctx,
					`
                    SELECT request_fingerprint
                      FROM inventory_transfer_receipts
                     WHERE business_id=$1
                       AND transfer_id=$2
                       AND source_ref=$3
                    `,
					actor.BusinessID,
					current.ID,
					normalized.SourceRef,
				).Scan(
					&existingFingerprint,
				)

			if err == nil {
				if existingFingerprint !=
					fingerprint {

					return ErrTransferIdempotencyConflict
				}

				out = current
				return nil
			}

			if !errors.Is(
				err,
				pgx.ErrNoRows,
			) {
				return err
			}

			if current.Status !=
				TransferInTransit &&
				current.Status !=
					TransferPartiallyReceived {

				return ErrTransferStateConflict
			}

			states, err :=
				lockTransferItemsTx(
					ctx,
					tx,
					actor.BusinessID,
					current,
				)

			if err != nil {
				return err
			}

			lineByID :=
				make(
					map[string]TransferLine,
					len(current.Lines),
				)

			for _, line := range current.Lines {

				lineByID[line.ID] =
					line
			}

			for _, resolution := range normalized.Lines {

				line, ok :=
					lineByID[resolution.LineID]

				if !ok {
					return ErrTransferNotFound
				}

				alreadyResolved :=
					line.ReceivedQuantity +
						line.DamagedQuantity +
						line.MissingQuantity

				newResolved :=
					resolution.ReceivedQuantity +
						resolution.DamagedQuantity +
						resolution.MissingQuantity

				if alreadyResolved+
					newResolved >
					line.SentQuantity {

					return ErrInvalidQuantity
				}
			}

			_, err =
				tx.Exec(
					ctx,
					`
                    INSERT INTO inventory_transfer_receipts (
                        id,
                        business_id,
                        transfer_id,
                        source_ref,
                        request_fingerprint,
                        request_payload
                    )
                    VALUES (
                        $1,$2,$3,$4,$5,$6
                    )
                    `,
					stockID("itr_"),
					actor.BusinessID,
					current.ID,
					normalized.SourceRef,
					fingerprint,
					json.RawMessage(payload),
				)

			if err != nil {
				return err
			}

			var lastRevision int64

			for _, resolution := range normalized.Lines {

				line :=
					lineByID[resolution.LineID]

				sourceItem :=
					states[line.SourceInventoryItemID]

				destinationItem :=
					states[line.DestinationInventoryItemID]

				resolved :=
					resolution.ReceivedQuantity +
						resolution.DamagedQuantity +
						resolution.MissingQuantity

				if err :=
					ensureBalanceTx(
						ctx,
						tx,
						destinationItem,
					); err != nil {

					return err
				}

				sourceBalanceRevision, err :=
					updateBalanceTx(
						ctx,
						tx,
						sourceItem,
						0,
						0,
						0,
						-resolved,
						0,
						0,
					)

				if err != nil {
					return err
				}

				sourceRevision, err :=
					bumpInventoryRevisionTx(
						ctx,
						tx,
						sourceItem.BusinessID,
						sourceItem.BranchID,
					)

				if err != nil {
					return err
				}

				lastRevision =
					sourceRevision

				if resolution.
					ReceivedQuantity > 0 {

					_, err :=
						appendMovementTx(
							ctx,
							tx,
							actor,
							sourceItem,
							"",
							MovementTransferOut,
							resolution.
								ReceivedQuantity,
							0,
							0,
							0,
							-resolution.
								ReceivedQuantity,
							0,
							0,
							"TRANSFER",
							current.ID,
							"TRANSFER_RECEIVED",
							sourceRevision,
							sourceBalanceRevision,
						)

					if err != nil {
						return err
					}

					lotID, err :=
						createLotTx(
							ctx,
							tx,
							destinationItem,
							resolution.
								ReceivedQuantity,
							"TRANSFER",
							current.ID,
							LotMetadata{},
						)

					if err != nil {
						return err
					}

					destinationBalanceRevision,
						err :=
						updateBalanceTx(
							ctx,
							tx,
							destinationItem,
							resolution.
								ReceivedQuantity,
							0,
							0,
							0,
							0,
							0,
						)

					if err != nil {
						return err
					}

					destinationRevision,
						err :=
						bumpInventoryRevisionTx(
							ctx,
							tx,
							destinationItem.
								BusinessID,
							destinationItem.
								BranchID,
						)

					if err != nil {
						return err
					}

					lastRevision =
						destinationRevision

					_, err =
						appendMovementTx(
							ctx,
							tx,
							actor,
							destinationItem,
							lotID,
							MovementTransferIn,
							resolution.
								ReceivedQuantity,
							resolution.
								ReceivedQuantity,
							0,
							0,
							0,
							0,
							0,
							"TRANSFER",
							current.ID,
							"TRANSFER_RECEIVED",
							destinationRevision,
							destinationBalanceRevision,
						)

					if err != nil {
						return err
					}
				}

				if resolution.
					DamagedQuantity > 0 {

					_, err :=
						appendMovementTx(
							ctx,
							tx,
							actor,
							sourceItem,
							"",
							MovementDamage,
							resolution.
								DamagedQuantity,
							0,
							0,
							0,
							-resolution.
								DamagedQuantity,
							0,
							0,
							"TRANSFER",
							current.ID,
							"TRANSFER_DAMAGED",
							sourceRevision,
							sourceBalanceRevision,
						)

					if err != nil {
						return err
					}
				}

				if resolution.
					MissingQuantity > 0 {

					_, err :=
						appendMovementTx(
							ctx,
							tx,
							actor,
							sourceItem,
							"",
							MovementAdjustmentOut,
							resolution.
								MissingQuantity,
							0,
							0,
							0,
							-resolution.
								MissingQuantity,
							0,
							0,
							"TRANSFER",
							current.ID,
							"TRANSFER_MISSING",
							sourceRevision,
							sourceBalanceRevision,
						)

					if err != nil {
						return err
					}
				}

				totalReceived :=
					line.ReceivedQuantity +
						resolution.
							ReceivedQuantity

				totalDamaged :=
					line.DamagedQuantity +
						resolution.
							DamagedQuantity

				totalMissing :=
					line.MissingQuantity +
						resolution.
							MissingQuantity

				totalResolved :=
					totalReceived +
						totalDamaged +
						totalMissing

				lineStatus :=
					TransferLinePartiallyReceived

				if totalResolved ==
					line.SentQuantity {

					lineStatus =
						TransferLineReceived
				}

				_, err =
					tx.Exec(
						ctx,
						`
                        UPDATE inventory_transfer_lines
                           SET received_quantity=$3,
                               damaged_quantity=$4,
                               missing_quantity=$5,
                               status=$6,
                               updated_at=now()
                         WHERE id=$1
                           AND business_id=$2
                        `,
						line.ID,
						actor.BusinessID,
						totalReceived,
						totalDamaged,
						totalMissing,
						lineStatus,
					)

				if err != nil {
					return err
				}

				if resolution.
					DamagedQuantity+
					resolution.
						MissingQuantity > 0 {

					_, err =
						upsertAlertTx(
							ctx,
							tx,
							actor.BusinessID,
							current.SourceBranchID,
							line.SourceInventoryItemID,
							AlertTransferException,
							"CRITICAL",
							"TRANSFER_EXCEPTION:"+
								current.ID+
								":"+
								line.ID,
							map[string]any{
								"transfer_id": current.ID,

								"line_id": line.ID,

								"damaged_quantity": totalDamaged,

								"missing_quantity": totalMissing,
							},
						)

					if err != nil {
						return err
					}
				}
			}

			var incomplete int

			err =
				tx.QueryRow(
					ctx,
					`
                    SELECT count(*)
                      FROM inventory_transfer_lines
                     WHERE transfer_id=$1
                       AND business_id=$2
                       AND status <> 'RECEIVED'
                    `,
					current.ID,
					actor.BusinessID,
				).Scan(
					&incomplete,
				)

			if err != nil {
				return err
			}

			target :=
				TransferPartiallyReceived

			if incomplete == 0 {
				target =
					TransferReceived
			}

			now :=
				s.foundation.Clock().Now()

			err =
				tx.QueryRow(
					ctx,
					`
                    UPDATE inventory_transfers
                       SET status=$3,
                           received_at=
                           CASE
                               WHEN $3='RECEIVED'
                               THEN $4
                               ELSE received_at
                           END,
                           updated_at=now(),
                           version=version+1
                     WHERE id=$1
                       AND business_id=$2
                    RETURNING
                        status,
                        received_at,
                        updated_at,
                        version
                    `,
					current.ID,
					current.BusinessID,
					target,
					now,
				).Scan(
					&current.Status,
					&current.ReceivedAt,
					&current.UpdatedAt,
					&current.Version,
				)

			if err != nil {
				return err
			}

			out, err =
				loadTransferQ(
					ctx,
					tx,
					actor.BusinessID,
					current.ID,
					false,
				)

			if err != nil {
				return err
			}

			exactScope, _ :=
				ExactBranchScope(
					actor.BusinessID,
					destination,
				)

			if lastRevision == 0 {
				lastRevision = 1
			}

			return s.recordMutation(
				ctx,
				inventoryTx,
				actor,
				exactScope,
				"inventory.transfer.received",
				"inventory.transfer.received",
				"inventory_transfer",
				out.ID,
				"",
				lastRevision,
				map[string]any{
					"transfer": out,

					"receipt_source_ref": normalized.SourceRef,
				},
			)
		},
	)

	return out, err
}
