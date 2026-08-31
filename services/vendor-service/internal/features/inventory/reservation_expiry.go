package inventory

import (
	"context"
)

func (s *StockService) expireReservationSystem(
	ctx context.Context,
	businessID string,
	reservationID string,
) (
	bool,
	error,
) {
	branchID, err :=
		reservationBranchQ(
			ctx,
			s.db,
			businessID,
			reservationID,
		)

	if err != nil {
		return false, err
	}

	scope, err :=
		ExactBranchScope(
			businessID,
			branchID,
		)

	if err != nil {
		return false, err
	}

	actor :=
		Actor{
			ActorRef: "inventory-reservation-expiry",

			BusinessID: businessID,
		}

	_,
		changed,
		err :=
		s.transitionReservation(
			ctx,
			actor,
			scope,
			reservationID,
			ReservationExpired,
			true,
		)

	return changed, err
}

// ExpireDueReservations is the durable worker pass.
//
// The reservation row itself is the durable work item: an active reservation
// with expires_at <= now() remains due until its release transaction commits.
func (s *StockService) ExpireDueReservations(
	ctx context.Context,
	limit int,
) (
	int,
	error,
) {
	if err := s.requireDependencies(); err != nil {
		return 0, err
	}

	if limit <= 0 {
		limit = 100
	}

	if limit > 1000 {
		limit = 1000
	}

	rows, err :=
		s.db.Query(
			ctx,
			`
            SELECT
                business_id,
                id
              FROM inventory_reservations
             WHERE status IN ('HELD','CONFIRMED')
               AND expires_at IS NOT NULL
               AND expires_at <= now()
             ORDER BY expires_at,id
             LIMIT $1
            `,
			limit,
		)

	if err != nil {
		return 0, err
	}

	type dueReservation struct {
		BusinessID    string
		ReservationID string
	}

	var due []dueReservation

	for rows.Next() {
		var item dueReservation

		if err := rows.Scan(
			&item.BusinessID,
			&item.ReservationID,
		); err != nil {
			rows.Close()

			return 0, err
		}

		due =
			append(
				due,
				item,
			)
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}

	rows.Close()

	processed := 0

	for _, item := range due {

		changed, err :=
			s.expireReservationSystem(
				ctx,
				item.BusinessID,
				item.ReservationID,
			)

		if err != nil {
			return processed, err
		}

		if changed {
			processed++
		}
	}

	return processed, nil
}

// ExpireDueReservationsForItems gives fresh authoritative reservation commands
// an inline stale-hold recovery path. Correctness therefore does not depend on
// the periodic sweep having already executed.
func (s *StockService) ExpireDueReservationsForItems(
	ctx context.Context,
	businessID string,
	itemIDs []string,
	limit int,
) (
	int,
	error,
) {
	if len(itemIDs) == 0 {
		return 0, nil
	}

	if limit <= 0 {
		limit = 100
	}

	if limit > 1000 {
		limit = 1000
	}

	rows, err :=
		s.db.Query(
			ctx,
			`
            SELECT DISTINCT
                r.id
              FROM inventory_reservations r
              JOIN inventory_reservation_lines l
                ON l.reservation_id=r.id
               AND l.business_id=r.business_id
             WHERE r.business_id=$1
               AND r.status IN ('HELD','CONFIRMED')
               AND r.expires_at IS NOT NULL
               AND r.expires_at <= now()
               AND l.inventory_item_id = ANY($2)
             ORDER BY r.id
             LIMIT $3
            `,
			businessID,
			itemIDs,
			limit,
		)

	if err != nil {
		return 0, err
	}

	var reservationIDs []string

	for rows.Next() {
		var reservationID string

		if err := rows.Scan(
			&reservationID,
		); err != nil {
			rows.Close()

			return 0, err
		}

		reservationIDs =
			append(
				reservationIDs,
				reservationID,
			)
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}

	rows.Close()

	processed := 0

	for _, reservationID := range reservationIDs {

		changed, err :=
			s.expireReservationSystem(
				ctx,
				businessID,
				reservationID,
			)

		if err != nil {
			return processed, err
		}

		if changed {
			processed++
		}
	}

	return processed, nil
}
