package inventory

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

func terminalTimestampColumn(
	status string,
) string {
	switch status {
	case ReservationCommitted:
		return "committed_at"

	case ReservationReleased:
		return "released_at"

	case ReservationExpired:
		return "expired_at"

	case ReservationCancelled:
		return "cancelled_at"

	default:
		return ""
	}
}

func (s *StockService) ConfirmReservation(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	reservationID string,
) (
	Reservation,
	error,
) {
	out,
		_,
		err :=
		s.transitionReservation(
			ctx,
			actor,
			scope,
			reservationID,
			ReservationConfirmed,
			false,
		)

	return out, err
}

func (s *StockService) CommitReservation(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	reservationID string,
) (
	Reservation,
	error,
) {
	out,
		_,
		err :=
		s.transitionReservation(
			ctx,
			actor,
			scope,
			reservationID,
			ReservationCommitted,
			false,
		)

	return out, err
}

func (s *StockService) ReleaseReservation(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	reservationID string,
) (
	Reservation,
	error,
) {
	out,
		_,
		err :=
		s.transitionReservation(
			ctx,
			actor,
			scope,
			reservationID,
			ReservationReleased,
			false,
		)

	return out, err
}

func (s *StockService) CancelReservation(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	reservationID string,
) (
	Reservation,
	error,
) {
	out,
		_,
		err :=
		s.transitionReservation(
			ctx,
			actor,
			scope,
			reservationID,
			ReservationCancelled,
			false,
		)

	return out, err
}

func lockReservationItemsTx(
	ctx context.Context,
	tx pgx.Tx,
	businessID string,
	branchID string,
	reservationID string,
) (
	map[string]stockItemState,
	error,
) {
	itemIDs, err :=
		reservationItemIDsQ(
			ctx,
			tx,
			businessID,
			reservationID,
		)

	if err != nil {
		return nil, err
	}

	if len(itemIDs) == 0 {
		return nil,
			ErrReservationNotFound
	}

	return lockItemsDeterministicallyTx(
		ctx,
		tx,
		businessID,
		branchID,
		itemIDs,
	)
}

func validateCommitLotsTx(
	ctx context.Context,
	tx pgx.Tx,
	reservation Reservation,
) error {
	for _, line := range reservation.Lines {

		for _, allocation := range line.Allocations {

			var (
				onHand   int64
				reserved int64
				expired  bool
			)

			err := tx.QueryRow(
				ctx,
				`
                SELECT
                    on_hand_quantity,
                    reserved_quantity,
                    (
                        expires_at IS NOT NULL
                        AND expires_at <= now()
                    )
                  FROM inventory_lots
                 WHERE id=$1
                   AND business_id=$2
                   AND inventory_item_id=$3
                 FOR UPDATE
                `,
				allocation.LotID,
				reservation.BusinessID,
				line.InventoryItemID,
			).Scan(
				&onHand,
				&reserved,
				&expired,
			)

			if errors.Is(
				err,
				pgx.ErrNoRows,
			) {
				return ErrInsufficientStock
			}

			if err != nil {
				return err
			}

			if expired {
				return ErrReservationStockExpired
			}

			if reserved <
				allocation.Quantity ||
				onHand <
					allocation.Quantity {

				return ErrInsufficientStock
			}
		}
	}

	return nil
}

func (s *StockService) applyReservationReleaseTx(
	ctx context.Context,
	tx pgx.Tx,
	actor Actor,
	reservation Reservation,
	target string,
	states map[string]stockItemState,
) (
	int64,
	error,
) {
	var lastRevision int64

	for _, line := range reservation.Lines {

		item :=
			states[line.InventoryItemID]

		for _, allocation := range line.Allocations {

			tag, err :=
				tx.Exec(
					ctx,
					`
                    UPDATE inventory_lots
                       SET reserved_quantity=
                           reserved_quantity-$3,
                           updated_at=now(),
                           version=version+1
                     WHERE id=$1
                       AND business_id=$2
                       AND reserved_quantity >= $3
                    `,
					allocation.LotID,
					reservation.BusinessID,
					allocation.Quantity,
				)

			if err != nil {
				return 0, err
			}

			if tag.RowsAffected() != 1 {
				return 0,
					ErrInsufficientStock
			}
		}

		balanceRevision, err :=
			updateBalanceTx(
				ctx,
				tx,
				item,
				0,
				-line.PhysicalQuantity,
				0,
				0,
				0,
				0,
			)

		if err != nil {
			return 0, err
		}

		inventoryRevision, err :=
			bumpInventoryRevisionTx(
				ctx,
				tx,
				item.BusinessID,
				item.BranchID,
			)

		if err != nil {
			return 0, err
		}

		lastRevision =
			inventoryRevision

		for _, allocation := range line.Allocations {

			_, err :=
				appendMovementTx(
					ctx,
					tx,
					actor,
					item,
					allocation.LotID,
					MovementRelease,
					allocation.Quantity,
					0,
					-allocation.Quantity,
					0,
					0,
					0,
					0,
					"RESERVATION",
					reservation.ID,
					target,
					inventoryRevision,
					balanceRevision,
				)

			if err != nil {
				return 0, err
			}

			_, err = tx.Exec(
				ctx,
				`
                UPDATE inventory_reservation_allocations
                   SET status=$3,
                       updated_at=now()
                 WHERE id=$1
                   AND business_id=$2
                   AND status='HELD'
                `,
				allocation.ID,
				reservation.BusinessID,
				target,
			)

			if err != nil {
				return 0, err
			}
		}

		_, err = tx.Exec(
			ctx,
			`
            UPDATE inventory_reservation_lines
               SET status=$3,
                   updated_at=now()
             WHERE id=$1
               AND business_id=$2
            `,
			line.ID,
			reservation.BusinessID,
			target,
		)

		if err != nil {
			return 0, err
		}
	}

	return lastRevision, nil
}

func (s *StockService) applyReservationCommitTx(
	ctx context.Context,
	tx pgx.Tx,
	actor Actor,
	reservation Reservation,
	states map[string]stockItemState,
) (
	int64,
	error,
) {
	if err := validateCommitLotsTx(
		ctx,
		tx,
		reservation,
	); err != nil {
		return 0, err
	}

	var lastRevision int64

	for _, line := range reservation.Lines {

		item :=
			states[line.InventoryItemID]

		for _, allocation := range line.Allocations {

			tag, err :=
				tx.Exec(
					ctx,
					`
                    UPDATE inventory_lots
                       SET on_hand_quantity=
                           on_hand_quantity-$3,
                           reserved_quantity=
                           reserved_quantity-$3,
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
                       AND reserved_quantity >= $3
                       AND on_hand_quantity >= $3
                       AND (
                           expires_at IS NULL
                           OR expires_at > now()
                       )
                    `,
					allocation.LotID,
					reservation.BusinessID,
					allocation.Quantity,
				)

			if err != nil {
				return 0, err
			}

			if tag.RowsAffected() != 1 {
				return 0,
					ErrReservationStockExpired
			}
		}

		balanceRevision, err :=
			updateBalanceTx(
				ctx,
				tx,
				item,
				-line.PhysicalQuantity,
				-line.PhysicalQuantity,
				0,
				0,
				0,
				0,
			)

		if err != nil {
			return 0, err
		}

		inventoryRevision, err :=
			bumpInventoryRevisionTx(
				ctx,
				tx,
				item.BusinessID,
				item.BranchID,
			)

		if err != nil {
			return 0, err
		}

		lastRevision =
			inventoryRevision

		for _, allocation := range line.Allocations {

			_, err :=
				appendMovementTx(
					ctx,
					tx,
					actor,
					item,
					allocation.LotID,
					MovementCommit,
					allocation.Quantity,
					-allocation.Quantity,
					-allocation.Quantity,
					0,
					0,
					0,
					0,
					"RESERVATION",
					reservation.ID,
					"COMMITTED",
					inventoryRevision,
					balanceRevision,
				)

			if err != nil {
				return 0, err
			}

			_, err = tx.Exec(
				ctx,
				`
                UPDATE inventory_reservation_allocations
                   SET status='COMMITTED',
                       updated_at=now()
                 WHERE id=$1
                   AND business_id=$2
                   AND status='HELD'
                `,
				allocation.ID,
				reservation.BusinessID,
			)

			if err != nil {
				return 0, err
			}
		}

		_, err = tx.Exec(
			ctx,
			`
            UPDATE inventory_reservation_lines
               SET status='COMMITTED',
                   updated_at=now()
             WHERE id=$1
               AND business_id=$2
            `,
			line.ID,
			reservation.BusinessID,
		)

		if err != nil {
			return 0, err
		}
	}

	return lastRevision, nil
}

func (s *StockService) transitionReservation(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	reservationID string,
	target string,
	requireDue bool,
) (
	Reservation,
	bool,
	error,
) {
	if err := s.requireDependencies(); err != nil {
		return Reservation{},
			false,
			err
	}

	branchID, err :=
		reservationBranchQ(
			ctx,
			s.db,
			actor.BusinessID,
			reservationID,
		)

	if err != nil {
		return Reservation{},
			false,
			err
	}

	var (
		out     Reservation
		changed bool
	)

	err = s.withWriteTransaction(
		ctx,
		actor,
		scope,
		branchID,
		func(
			inventoryTx InventoryTransaction,
			tx pgx.Tx,
		) error {
			states, err :=
				lockReservationItemsTx(
					ctx,
					tx,
					actor.BusinessID,
					branchID,
					reservationID,
				)

			if err != nil {
				return err
			}

			reservation, err :=
				loadReservationQ(
					ctx,
					tx,
					actor.BusinessID,
					reservationID,
					true,
				)

			if err != nil {
				return err
			}

			if reservation.Status ==
				target {

				out = reservation
				return nil
			}

			if reservationTerminal(
				reservation.Status,
			) {
				return ErrReservationStateConflict
			}

			now :=
				s.foundation.Clock().Now()

			due :=
				reservation.ExpiresAt != nil &&
					!reservation.ExpiresAt.
						After(now)

			if target ==
				ReservationCommitted &&
				due {

				return ErrReservationExpired
			}

			if requireDue &&
				!due {

				out = reservation
				return nil
			}

			var lastRevision int64

			switch target {
			case ReservationConfirmed:
				if reservation.Status !=
					ReservationHeld {

					return ErrReservationStateConflict
				}

				if due {
					return ErrReservationExpired
				}

				_, err =
					tx.Exec(
						ctx,
						`
                        UPDATE inventory_reservation_lines
                           SET status='CONFIRMED',
                               updated_at=now()
                         WHERE reservation_id=$1
                           AND business_id=$2
                           AND status='HELD'
                        `,
						reservation.ID,
						reservation.BusinessID,
					)

				if err != nil {
					return err
				}

			case ReservationCommitted:
				if reservation.Status !=
					ReservationHeld &&
					reservation.Status !=
						ReservationConfirmed {

					return ErrReservationStateConflict
				}

				lastRevision, err =
					s.applyReservationCommitTx(
						ctx,
						tx,
						actor,
						reservation,
						states,
					)

				if err != nil {
					return err
				}

			case ReservationReleased,
				ReservationCancelled,
				ReservationExpired:

				if reservation.Status !=
					ReservationHeld &&
					reservation.Status !=
						ReservationConfirmed {

					return ErrReservationStateConflict
				}

				lastRevision, err =
					s.applyReservationReleaseTx(
						ctx,
						tx,
						actor,
						reservation,
						target,
						states,
					)

				if err != nil {
					return err
				}

			default:
				return ErrReservationStateConflict
			}

			var updatedAt interface{}

			switch target {
			case ReservationConfirmed:
				updatedAt =
					reservation.ConfirmedAt

				err = tx.QueryRow(
					ctx,
					`
                    UPDATE inventory_reservations
                       SET status='CONFIRMED',
                           confirmed_at=$3,
                           updated_at=now(),
                           version=version+1
                     WHERE id=$1
                       AND business_id=$2
                    RETURNING
                        updated_at,
                        version
                    `,
					reservation.ID,
					reservation.BusinessID,
					now,
				).Scan(
					&reservation.UpdatedAt,
					&reservation.Version,
				)

				if err == nil {
					reservation.ConfirmedAt =
						&now
				}

			case ReservationCommitted:
				err = tx.QueryRow(
					ctx,
					`
                    UPDATE inventory_reservations
                       SET status='COMMITTED',
                           committed_at=$3,
                           updated_at=now(),
                           version=version+1
                     WHERE id=$1
                       AND business_id=$2
                    RETURNING
                        updated_at,
                        version
                    `,
					reservation.ID,
					reservation.BusinessID,
					now,
				).Scan(
					&reservation.UpdatedAt,
					&reservation.Version,
				)

				if err == nil {
					reservation.CommittedAt =
						&now
				}

			case ReservationReleased:
				err = tx.QueryRow(
					ctx,
					`
                    UPDATE inventory_reservations
                       SET status='RELEASED',
                           released_at=$3,
                           updated_at=now(),
                           version=version+1
                     WHERE id=$1
                       AND business_id=$2
                    RETURNING
                        updated_at,
                        version
                    `,
					reservation.ID,
					reservation.BusinessID,
					now,
				).Scan(
					&reservation.UpdatedAt,
					&reservation.Version,
				)

				if err == nil {
					reservation.ReleasedAt =
						&now
				}

			case ReservationExpired:
				err = tx.QueryRow(
					ctx,
					`
                    UPDATE inventory_reservations
                       SET status='EXPIRED',
                           expired_at=$3,
                           updated_at=now(),
                           version=version+1
                     WHERE id=$1
                       AND business_id=$2
                    RETURNING
                        updated_at,
                        version
                    `,
					reservation.ID,
					reservation.BusinessID,
					now,
				).Scan(
					&reservation.UpdatedAt,
					&reservation.Version,
				)

				if err == nil {
					reservation.ExpiredAt =
						&now
				}

			case ReservationCancelled:
				err = tx.QueryRow(
					ctx,
					`
                    UPDATE inventory_reservations
                       SET status='CANCELLED',
                           cancelled_at=$3,
                           updated_at=now(),
                           version=version+1
                     WHERE id=$1
                       AND business_id=$2
                    RETURNING
                        updated_at,
                        version
                    `,
					reservation.ID,
					reservation.BusinessID,
					now,
				).Scan(
					&reservation.UpdatedAt,
					&reservation.Version,
				)

				if err == nil {
					reservation.CancelledAt =
						&now
				}
			}

			_ = updatedAt

			if err != nil {
				return err
			}

			reservation.Status =
				target

			out, err =
				loadReservationQ(
					ctx,
					tx,
					actor.BusinessID,
					reservation.ID,
					false,
				)

			if err != nil {
				return err
			}

			changed = true

			eventRevision :=
				lastRevision

			if eventRevision == 0 {
				eventRevision =
					out.Version
			}

			return s.recordMutation(
				ctx,
				inventoryTx,
				actor,
				scope,
				"inventory.reservation."+
					target,
				"inventory.reservation."+
					target,
				"inventory_reservation",
				out.ID,
				target,
				eventRevision,
				map[string]any{
					"reservation": out,
				},
			)
		},
	)

	return out,
		changed,
		err
}
