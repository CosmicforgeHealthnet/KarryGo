package inventory

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
)

func lockItemsDeterministicallyTx(
	ctx context.Context,
	tx pgx.Tx,
	businessID string,
	branchID string,
	itemIDs []string,
) (
	map[string]stockItemState,
	error,
) {
	ordered :=
		append(
			[]string(nil),
			itemIDs...,
		)

	sort.Strings(ordered)

	states :=
		make(
			map[string]stockItemState,
			len(ordered),
		)

	for _, itemID := range ordered {

		item, err :=
			loadStockItemForUpdate(
				ctx,
				tx,
				businessID,
				branchID,
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

func reservationInputItemIDs(
	input CreateReservationInput,
) []string {
	out :=
		make(
			[]string,
			0,
			len(input.Lines),
		)

	for _, line := range input.Lines {

		out =
			append(
				out,
				line.InventoryItemID,
			)
	}

	return out
}

func (s *StockService) CreateReservation(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	input CreateReservationInput,
) (
	Reservation,
	error,
) {
	if err := s.requireDependencies(); err != nil {
		return Reservation{},
			err
	}

	normalized,
		fingerprint,
		err :=
		normalizeReservationInput(
			input,
		)

	if err != nil {
		return Reservation{}, err
	}

	// Domain-level idempotency check before any stock mutation.
	existingID,
		existingFingerprint,
		err :=
		findReservationBySourceQ(
			ctx,
			s.db,
			actor.BusinessID,
			normalized.SourceType,
			normalized.SourceRef,
		)

	if err != nil {
		return Reservation{}, err
	}

	if existingID != "" {
		if existingFingerprint !=
			fingerprint {

			return Reservation{},
				ErrReservationIdempotencyConflict
		}

		return s.GetReservation(
			ctx,
			actor,
			scope,
			existingID,
		)
	}

	if normalized.ExpiresAt != nil &&
		!normalized.ExpiresAt.
			After(
				s.foundation.Clock().Now(),
			) {

		return Reservation{},
			ErrReservationExpired
	}

	// Correctness is not dependent solely on a periodic sweeper. Before
	// competing for fresh stock, synchronously settle already-expired holds
	// that touch the requested items.
	if _, err := s.ExpireDueReservationsForItems(
		ctx,
		actor.BusinessID,
		reservationInputItemIDs(
			normalized,
		),
		500,
	); err != nil {
		return Reservation{}, err
	}

	var out Reservation

	err = s.withWriteTransaction(
		ctx,
		actor,
		scope,
		normalized.BranchID,
		func(
			inventoryTx InventoryTransaction,
			tx pgx.Tx,
		) error {
			itemIDs :=
				reservationInputItemIDs(
					normalized,
				)

			states, err :=
				lockItemsDeterministicallyTx(
					ctx,
					tx,
					actor.BusinessID,
					normalized.BranchID,
					itemIDs,
				)

			if err != nil {
				return err
			}

			reservationID :=
				stockID("ires_")

			var createdAt time.Time

			err = tx.QueryRow(
				ctx,
				`
                INSERT INTO inventory_reservations (
                    id,
                    business_id,
                    branch_id,
                    source_type,
                    source_ref,
                    request_fingerprint,
                    allocation_policy,
                    status,
                    expires_at
                )
                VALUES (
                    $1,$2,$3,$4,$5,$6,
                    'ALL_OR_NOTHING',
                    'HELD',
                    $7
                )
                ON CONFLICT (
                    business_id,
                    source_type,
                    source_ref
                )
                DO NOTHING
                RETURNING created_at
                `,
				reservationID,
				actor.BusinessID,
				normalized.BranchID,
				normalized.SourceType,
				normalized.SourceRef,
				fingerprint,
				normalized.ExpiresAt,
			).Scan(
				&createdAt,
			)

			if errors.Is(
				err,
				pgx.ErrNoRows,
			) {
				existingID,
					existingFingerprint,
					findErr :=
					findReservationBySourceQ(
						ctx,
						tx,
						actor.BusinessID,
						normalized.SourceType,
						normalized.SourceRef,
					)

				if findErr != nil {
					return findErr
				}

				if existingID == "" ||
					existingFingerprint !=
						fingerprint {

					return ErrReservationIdempotencyConflict
				}

				out, findErr =
					loadReservationQ(
						ctx,
						tx,
						actor.BusinessID,
						existingID,
						false,
					)

				return findErr
			}

			if err != nil {
				return err
			}

			out = Reservation{
				ID: reservationID,

				BusinessID: actor.BusinessID,

				BranchID: normalized.BranchID,

				SourceType: normalized.SourceType,

				SourceRef: normalized.SourceRef,

				RequestFingerprint: fingerprint,

				AllocationPolicy: ReservationAllocationAllOrNothing,

				Status: ReservationHeld,

				ExpiresAt: normalized.ExpiresAt,

				HeldAt: createdAt,

				CreatedAt: createdAt,

				UpdatedAt: createdAt,

				Version: 1,
			}

			var lastInventoryRevision int64

			// Input lines have already been normalised and sorted by item ID.
			// Item locks were also acquired in this same stable order.
			for _, requested := range normalized.Lines {

				item :=
					states[requested.InventoryItemID]

				if err := ensureBalanceTx(
					ctx,
					tx,
					item,
				); err != nil {
					return err
				}

				allocations, err :=
					allocateAvailableLotsTx(
						ctx,
						tx,
						item,
						requested.Quantity,
					)

				if err != nil {
					// Transaction rollback ensures no earlier line survives.
					return err
				}

				line :=
					ReservationLine{
						ID: stockID("iresl_"),

						InventoryItemID: item.ID,

						RequestedQuantity: requested.Quantity,

						PhysicalQuantity: requested.Quantity,

						BackorderQuantity: 0,

						Status: ReservationHeld,
					}

				_, err = tx.Exec(
					ctx,
					`
                    INSERT INTO inventory_reservation_lines (
                        id,
                        business_id,
                        branch_id,
                        reservation_id,
                        inventory_item_id,
                        requested_quantity,
                        physical_quantity,
                        backorder_quantity,
                        status
                    )
                    VALUES (
                        $1,$2,$3,$4,$5,$6,$6,0,'HELD'
                    )
                    `,
					line.ID,
					actor.BusinessID,
					normalized.BranchID,
					out.ID,
					item.ID,
					requested.Quantity,
				)

				if err != nil {
					return err
				}

				for _, allocation := range allocations {

					_, err := tx.Exec(
						ctx,
						`
                        UPDATE inventory_lots
                           SET reserved_quantity=
                               reserved_quantity+$3,
                               updated_at=now(),
                               version=version+1
                         WHERE id=$1
                           AND business_id=$2
                           AND status='ACTIVE'
                           AND (
                               expires_at IS NULL
                               OR expires_at > now()
                           )
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
				}

				balanceRevision, err :=
					updateBalanceTx(
						ctx,
						tx,
						item,
						0,
						requested.Quantity,
						0,
						0,
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

				lastInventoryRevision =
					inventoryRevision

				for _, allocation := range allocations {

					movement, err :=
						appendMovementTx(
							ctx,
							tx,
							actor,
							item,
							allocation.ID,
							MovementReserve,
							allocation.Quantity,
							0,
							allocation.Quantity,
							0,
							0,
							0,
							0,
							"RESERVATION",
							out.ID,
							"",
							inventoryRevision,
							balanceRevision,
						)

					if err != nil {
						return err
					}

					_ = movement

					reservationAllocation :=
						ReservationAllocation{
							ID: stockID("iresa_"),

							InventoryItemID: item.ID,

							LotID: allocation.ID,

							Quantity: allocation.Quantity,

							Status: ReservationHeld,
						}

					_, err = tx.Exec(
						ctx,
						`
                        INSERT INTO inventory_reservation_allocations (
                            id,
                            business_id,
                            branch_id,
                            reservation_id,
                            reservation_line_id,
                            inventory_item_id,
                            lot_id,
                            quantity,
                            status
                        )
                        VALUES (
                            $1,$2,$3,$4,$5,$6,$7,$8,'HELD'
                        )
                        `,
						reservationAllocation.ID,
						actor.BusinessID,
						normalized.BranchID,
						out.ID,
						line.ID,
						item.ID,
						reservationAllocation.LotID,
						reservationAllocation.Quantity,
					)

					if err != nil {
						return err
					}

					line.Allocations =
						append(
							line.Allocations,
							reservationAllocation,
						)
				}

				out.Lines =
					append(
						out.Lines,
						line,
					)
			}

			return s.recordMutation(
				ctx,
				inventoryTx,
				actor,
				scope,
				"inventory.reservation.held",
				"inventory.reservation.held",
				"inventory_reservation",
				out.ID,
				"",
				lastInventoryRevision,
				map[string]any{
					"reservation": out,

					"source_type": out.SourceType,

					"source_ref": out.SourceRef,
				},
			)
		},
	)

	return out, err
}
