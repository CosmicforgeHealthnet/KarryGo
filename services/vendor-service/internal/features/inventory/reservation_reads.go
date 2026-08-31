package inventory

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

type reservationQuerier interface {
	QueryRow(
		ctx context.Context,
		sql string,
		args ...any,
	) pgx.Row

	Query(
		ctx context.Context,
		sql string,
		args ...any,
	) (pgx.Rows, error)
}

func loadReservationQ(
	ctx context.Context,
	q reservationQuerier,
	businessID string,
	reservationID string,
	forUpdate bool,
) (
	Reservation,
	error,
) {
	lockClause := ""

	if forUpdate {
		lockClause = " FOR UPDATE"
	}

	var out Reservation

	err := q.QueryRow(
		ctx,
		`
        SELECT
            id,
            business_id,
            branch_id,
            source_type,
            source_ref,
            request_fingerprint,
            allocation_policy,
            status,
            expires_at,
            held_at,
            confirmed_at,
            committed_at,
            released_at,
            expired_at,
            cancelled_at,
            created_at,
            updated_at,
            version
          FROM inventory_reservations
         WHERE id=$1
           AND business_id=$2
        `+lockClause,
		strings.TrimSpace(
			reservationID,
		),
		strings.TrimSpace(
			businessID,
		),
	).Scan(
		&out.ID,
		&out.BusinessID,
		&out.BranchID,
		&out.SourceType,
		&out.SourceRef,
		&out.RequestFingerprint,
		&out.AllocationPolicy,
		&out.Status,
		&out.ExpiresAt,
		&out.HeldAt,
		&out.ConfirmedAt,
		&out.CommittedAt,
		&out.ReleasedAt,
		&out.ExpiredAt,
		&out.CancelledAt,
		&out.CreatedAt,
		&out.UpdatedAt,
		&out.Version,
	)

	if errors.Is(
		err,
		pgx.ErrNoRows,
	) {
		return Reservation{},
			ErrReservationNotFound
	}

	if err != nil {
		return Reservation{}, err
	}

	rows, err := q.Query(
		ctx,
		`
        SELECT
            id,
            inventory_item_id,
            requested_quantity,
            physical_quantity,
            backorder_quantity,
            status
          FROM inventory_reservation_lines
         WHERE reservation_id=$1
           AND business_id=$2
         ORDER BY inventory_item_id,id
        `,
		out.ID,
		out.BusinessID,
	)

	if err != nil {
		return Reservation{}, err
	}

	defer rows.Close()

	for rows.Next() {
		var line ReservationLine

		if err := rows.Scan(
			&line.ID,
			&line.InventoryItemID,
			&line.RequestedQuantity,
			&line.PhysicalQuantity,
			&line.BackorderQuantity,
			&line.Status,
		); err != nil {
			return Reservation{}, err
		}

		out.Lines =
			append(
				out.Lines,
				line,
			)
	}

	if err := rows.Err(); err != nil {
		return Reservation{}, err
	}

	for index := range out.Lines {

		line :=
			&out.Lines[index]

		allocationRows, err :=
			q.Query(
				ctx,
				`
                SELECT
                    id,
                    inventory_item_id,
                    lot_id,
                    quantity,
                    status
                  FROM inventory_reservation_allocations
                 WHERE reservation_line_id=$1
                   AND business_id=$2
                 ORDER BY lot_id,id
                `,
				line.ID,
				out.BusinessID,
			)

		if err != nil {
			return Reservation{}, err
		}

		for allocationRows.Next() {
			var allocation ReservationAllocation

			if err := allocationRows.Scan(
				&allocation.ID,
				&allocation.InventoryItemID,
				&allocation.LotID,
				&allocation.Quantity,
				&allocation.Status,
			); err != nil {
				allocationRows.Close()

				return Reservation{},
					err
			}

			line.Allocations =
				append(
					line.Allocations,
					allocation,
				)
		}

		if err := allocationRows.Err(); err != nil {
			allocationRows.Close()

			return Reservation{},
				err
		}

		allocationRows.Close()
	}

	return out, nil
}

func findReservationBySourceQ(
	ctx context.Context,
	q reservationQuerier,
	businessID string,
	sourceType string,
	sourceRef string,
) (
	string,
	string,
	error,
) {
	var (
		reservationID string
		fingerprint   string
	)

	err := q.QueryRow(
		ctx,
		`
        SELECT
            id,
            request_fingerprint
          FROM inventory_reservations
         WHERE business_id=$1
           AND source_type=$2
           AND source_ref=$3
        `,
		businessID,
		sourceType,
		sourceRef,
	).Scan(
		&reservationID,
		&fingerprint,
	)

	if errors.Is(
		err,
		pgx.ErrNoRows,
	) {
		return "", "", nil
	}

	return reservationID,
		fingerprint,
		err
}

func reservationBranchQ(
	ctx context.Context,
	q reservationQuerier,
	businessID string,
	reservationID string,
) (
	string,
	error,
) {
	var branchID string

	err := q.QueryRow(
		ctx,
		`
        SELECT branch_id
          FROM inventory_reservations
         WHERE id=$1
           AND business_id=$2
        `,
		reservationID,
		businessID,
	).Scan(
		&branchID,
	)

	if errors.Is(
		err,
		pgx.ErrNoRows,
	) {
		return "",
			ErrReservationNotFound
	}

	return branchID, err
}

func reservationItemIDsQ(
	ctx context.Context,
	q reservationQuerier,
	businessID string,
	reservationID string,
) (
	[]string,
	error,
) {
	rows, err := q.Query(
		ctx,
		`
        SELECT inventory_item_id
          FROM inventory_reservation_lines
         WHERE reservation_id=$1
           AND business_id=$2
         ORDER BY inventory_item_id
        `,
		reservationID,
		businessID,
	)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var out []string

	for rows.Next() {
		var itemID string

		if err := rows.Scan(
			&itemID,
		); err != nil {
			return nil, err
		}

		out =
			append(
				out,
				itemID,
			)
	}

	return out, rows.Err()
}

func (s *StockService) ReservationBranch(
	ctx context.Context,
	businessID string,
	reservationID string,
) (
	string,
	error,
) {
	if s == nil || s.db == nil {
		return "",
			ErrDependencyMissing
	}

	return reservationBranchQ(
		ctx,
		s.db,
		businessID,
		reservationID,
	)
}

func (s *StockService) GetReservation(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	reservationID string,
) (
	Reservation,
	error,
) {
	if err := s.requireDependencies(); err != nil {
		return Reservation{},
			err
	}

	out, err :=
		loadReservationQ(
			ctx,
			s.db,
			actor.BusinessID,
			reservationID,
			false,
		)

	if err != nil {
		return Reservation{}, err
	}

	if err := s.foundation.RequireBranch(
		ctx,
		actor,
		scope,
		actor.BusinessID,
		out.BranchID,
		false,
	); err != nil {
		return Reservation{}, err
	}

	return out, nil
}
