package inventory

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const maxStockQuantity int64 = 1_000_000_000

type StockOptions struct {
	DB *pgxpool.Pool

	Foundation *Service

	Transactions TransactionBoundary
	Evidence     InventoryEvidenceRecorder
	Events       InventoryEventPublisher

	RecipeAvailability RecipeAvailabilityResolver
}

type StockService struct {
	db *pgxpool.Pool

	foundation *Service

	transactions       TransactionBoundary
	evidence           InventoryEvidenceRecorder
	events             InventoryEventPublisher
	recipeAvailability RecipeAvailabilityResolver
}

func NewStockService(
	opts StockOptions,
) *StockService {
	return &StockService{
		db:           opts.DB,
		foundation:   opts.Foundation,
		transactions: opts.Transactions,
		evidence:     opts.Evidence,
		events:       opts.Events,

		recipeAvailability: opts.RecipeAvailability,
	}
}

type stockItemState struct {
	ID string

	BusinessID string
	BranchID   string

	TrackingMode string
	LotPolicy    string

	Status string
}

type lotAllocation struct {
	ID       string
	Quantity int64
}

func stockID(
	prefix string,
) string {
	return prefix + uuid.NewString()
}

func (s *StockService) requireDependencies() error {
	if s == nil ||
		s.db == nil ||
		s.foundation == nil ||
		s.transactions == nil ||
		s.evidence == nil ||
		s.events == nil {

		return ErrDependencyMissing
	}

	return nil
}

func postgresTransaction(
	tx InventoryTransaction,
) (pgx.Tx, error) {
	postgresTx, ok :=
		tx.(PostgresTransactionAccessor)

	if !ok {
		return nil, ErrDependencyMissing
	}

	return postgresTx.PGX(), nil
}

func enableInventoryMutation(
	ctx context.Context,
	tx pgx.Tx,
) error {
	_, err := tx.Exec(
		ctx,
		`
        SELECT set_config(
            'karrygo.inventory_mutation',
            '1',
            true
        )
        `,
	)

	return err
}

func (s *StockService) withWriteTransaction(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	branchID string,
	work func(
		InventoryTransaction,
		pgx.Tx,
	) error,
) error {
	if err := s.requireDependencies(); err != nil {
		return err
	}

	branchID = strings.TrimSpace(branchID)

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

	return s.transactions.WithinInventoryTransaction(
		ctx,
		func(inventoryTx InventoryTransaction) error {
			tx, err :=
				postgresTransaction(
					inventoryTx,
				)

			if err != nil {
				return err
			}

			if err := enableInventoryMutation(
				ctx,
				tx,
			); err != nil {
				return err
			}

			return work(
				inventoryTx,
				tx,
			)
		},
	)
}

func loadStockItemForUpdate(
	ctx context.Context,
	tx pgx.Tx,
	businessID string,
	branchID string,
	itemID string,
) (
	stockItemState,
	error,
) {
	var item stockItemState

	err := tx.QueryRow(
		ctx,
		`
        SELECT
            id,
            business_id,
            branch_id,
            tracking_mode,
            lot_policy,
            status
          FROM inventory_items
         WHERE id=$1
           AND business_id=$2
           AND branch_id=$3
         FOR UPDATE
        `,
		strings.TrimSpace(itemID),
		strings.TrimSpace(businessID),
		strings.TrimSpace(branchID),
	).Scan(
		&item.ID,
		&item.BusinessID,
		&item.BranchID,
		&item.TrackingMode,
		&item.LotPolicy,
		&item.Status,
	)

	if errors.Is(
		err,
		pgx.ErrNoRows,
	) {
		return stockItemState{},
			ErrItemNotFound
	}

	if err != nil {
		return stockItemState{},
			err
	}

	if item.Status != ItemStatusActive {
		return stockItemState{},
			ErrSourceInactive
	}

	return item, nil
}

func ensureBalanceTx(
	ctx context.Context,
	tx pgx.Tx,
	item stockItemState,
) error {
	_, err := tx.Exec(
		ctx,
		`
        INSERT INTO inventory_balances (
            id,
            business_id,
            branch_id,
            inventory_item_id
        )
        VALUES (
            $1,$2,$3,$4
        )
        ON CONFLICT (
            business_id,
            branch_id,
            inventory_item_id
        )
        DO NOTHING
        `,
		stockID("ib_"),
		item.BusinessID,
		item.BranchID,
		item.ID,
	)

	return err
}

func updateBalanceTx(
	ctx context.Context,
	tx pgx.Tx,
	item stockItemState,
	onHandDelta int64,
	reservedDelta int64,
	quarantinedDelta int64,
	inTransitDelta int64,
	incomingDelta int64,
	backorderDelta int64,
) (
	int64,
	error,
) {
	var revision int64

	err := tx.QueryRow(
		ctx,
		`
        UPDATE inventory_balances
           SET on_hand_quantity=
                   on_hand_quantity+$4,
               reserved_quantity=
                   reserved_quantity+$5,
               quarantined_quantity=
                   quarantined_quantity+$6,
               in_transit_quantity=
                   in_transit_quantity+$7,
               incoming_quantity=
                   incoming_quantity+$8,
               backorder_quantity=
                   backorder_quantity+$9,
               revision=revision+1,
               updated_at=now()
         WHERE business_id=$1
           AND branch_id=$2
           AND inventory_item_id=$3
           AND on_hand_quantity+$4 >= 0
           AND reserved_quantity+$5 >= 0
           AND quarantined_quantity+$6 >= 0
           AND in_transit_quantity+$7 >= 0
           AND incoming_quantity+$8 >= 0
           AND backorder_quantity+$9 >= 0
           AND (
               reserved_quantity+$5
               + quarantined_quantity+$6
           ) <= on_hand_quantity+$4
        RETURNING revision
        `,
		item.BusinessID,
		item.BranchID,
		item.ID,
		onHandDelta,
		reservedDelta,
		quarantinedDelta,
		inTransitDelta,
		incomingDelta,
		backorderDelta,
	).Scan(
		&revision,
	)

	if errors.Is(
		err,
		pgx.ErrNoRows,
	) {
		return 0,
			ErrInsufficientStock
	}

	return revision, err
}

func bumpInventoryRevisionTx(
	ctx context.Context,
	tx pgx.Tx,
	businessID string,
	branchID string,
) (
	int64,
	error,
) {
	var revision int64

	err := tx.QueryRow(
		ctx,
		`
        INSERT INTO inventory_revisions (
            business_id,
            branch_id,
            revision,
            updated_at
        )
        VALUES (
            $1,$2,1,now()
        )
        ON CONFLICT (
            business_id,
            branch_id
        )
        DO UPDATE SET
            revision=
                inventory_revisions.revision+1,
            updated_at=now()
        RETURNING revision
        `,
		businessID,
		branchID,
	).Scan(
		&revision,
	)

	return revision, err
}

func appendMovementTx(
	ctx context.Context,
	tx pgx.Tx,
	actor Actor,
	item stockItemState,
	lotID string,
	movementType string,
	quantity int64,
	onHandDelta int64,
	reservedDelta int64,
	quarantinedDelta int64,
	inTransitDelta int64,
	incomingDelta int64,
	backorderDelta int64,
	sourceType string,
	sourceRef string,
	reasonCode string,
	inventoryRevision int64,
	balanceRevision int64,
) (
	Movement,
	error,
) {
	movement := Movement{
		ID: stockID("im_"),

		BusinessID:      item.BusinessID,
		BranchID:        item.BranchID,
		InventoryItemID: item.ID,

		LotID: lotID,

		Type:     movementType,
		Quantity: quantity,

		OnHandDelta:      onHandDelta,
		ReservedDelta:    reservedDelta,
		QuarantinedDelta: quarantinedDelta,
		InTransitDelta:   inTransitDelta,
		IncomingDelta:    incomingDelta,
		BackorderDelta:   backorderDelta,

		SourceType: strings.TrimSpace(sourceType),
		SourceRef:  strings.TrimSpace(sourceRef),

		ReasonCode: strings.TrimSpace(reasonCode),
		ActorRef:   actor.Identity(),

		InventoryRevision: inventoryRevision,

		BalanceRevision: balanceRevision,
	}

	err := tx.QueryRow(
		ctx,
		`
        INSERT INTO inventory_movements (
            id,
            business_id,
            branch_id,
            inventory_item_id,
            lot_id,
            movement_type,
            quantity,
            on_hand_delta,
            reserved_delta,
            quarantined_delta,
            in_transit_delta,
            incoming_delta,
            backorder_delta,
            source_type,
            source_ref,
            reason_code,
            actor_ref,
            inventory_revision,
            balance_revision
        )
        VALUES (
            $1,$2,$3,$4,NULLIF($5,''),
            $6,$7,$8,$9,$10,$11,$12,$13,
            $14,$15,NULLIF($16,''),
            NULLIF($17,''),$18,$19
        )
        RETURNING occurred_at
        `,
		movement.ID,
		movement.BusinessID,
		movement.BranchID,
		movement.InventoryItemID,
		movement.LotID,
		movement.Type,
		movement.Quantity,
		movement.OnHandDelta,
		movement.ReservedDelta,
		movement.QuarantinedDelta,
		movement.InTransitDelta,
		movement.IncomingDelta,
		movement.BackorderDelta,
		movement.SourceType,
		movement.SourceRef,
		movement.ReasonCode,
		movement.ActorRef,
		movement.InventoryRevision,
		movement.BalanceRevision,
	).Scan(
		&movement.OccurredAt,
	)

	return movement, err
}

func (s *StockService) recordMutationPrimary(
	ctx context.Context,
	inventoryTx InventoryTransaction,
	actor Actor,
	scope BranchScope,
	action string,
	eventType string,
	aggregateType string,
	aggregateRef string,
	reason string,
	revision int64,
	metadata map[string]any,
) error {
	if err := s.evidence.RecordInventoryEvidence(
		ctx,
		inventoryTx,
		Evidence{
			Action: action,

			AggregateType: aggregateType,

			AggregateRef: aggregateRef,

			Actor: actor,

			Scope: scope,

			ReasonCode: reason,

			Metadata: metadata,
		},
	); err != nil {
		return err
	}

	branchID := ""

	if scope.BranchID != nil {
		branchID = *scope.BranchID
	}

	return s.events.PublishInventoryEvent(
		ctx,
		inventoryTx,
		Event{
			Type: eventType,

			BusinessID: actor.BusinessID,

			BranchID: &branchID,

			AggregateType: aggregateType,

			AggregateRef: aggregateRef,

			Revision: revision,

			CorrelationID: actor.CorrelationID,

			CausationID: actor.CausationID,

			OccurredAt: s.foundation.Clock().Now(),

			Payload: metadata,
		},
	)
}

func createLotTx(
	ctx context.Context,
	tx pgx.Tx,
	item stockItemState,
	quantity int64,
	sourceType string,
	sourceRef string,
	meta LotMetadata,
) (
	string,
	error,
) {
	if quantity <= 0 {
		return "", ErrInvalidQuantity
	}

	receivedAt := time.Now().UTC()

	if meta.ExpiresAt != nil &&
		!meta.ExpiresAt.After(receivedAt) {
		return "", ErrInvalidQuantity
	}

	lotID := stockID("il_")

	_, err := tx.Exec(
		ctx,
		`
        INSERT INTO inventory_lots (
            id,
            business_id,
            branch_id,
            inventory_item_id,
            lot_number,
            batch_number,
            source_type,
            source_ref,
            received_at,
            manufactured_at,
            best_before_at,
            expires_at,
            on_hand_quantity
        )
        VALUES (
            $1,$2,$3,$4,
            NULLIF($5,''),
            NULLIF($6,''),
            $7,$8,$9,$10,$11,$12,$13
        )
        `,
		lotID,
		item.BusinessID,
		item.BranchID,
		item.ID,
		strings.TrimSpace(meta.LotNumber),
		strings.TrimSpace(meta.BatchNumber),
		strings.TrimSpace(sourceType),
		strings.TrimSpace(sourceRef),
		receivedAt,
		meta.ManufacturedAt,
		meta.BestBeforeAt,
		meta.ExpiresAt,
		quantity,
	)

	return lotID, err
}

func (s *StockService) increasePhysicalTx(
	ctx context.Context,
	tx pgx.Tx,
	actor Actor,
	item stockItemState,
	quantity int64,
	movementType string,
	sourceType string,
	sourceRef string,
	reasonCode string,
	meta LotMetadata,
) (
	Balance,
	Movement,
	string,
	error,
) {
	if quantity <= 0 ||
		quantity > maxStockQuantity {
		return Balance{},
			Movement{},
			"",
			ErrInvalidQuantity
	}

	if err := ensureBalanceTx(
		ctx,
		tx,
		item,
	); err != nil {
		return Balance{},
			Movement{},
			"",
			err
	}

	lotID, err :=
		createLotTx(
			ctx,
			tx,
			item,
			quantity,
			sourceType,
			sourceRef,
			meta,
		)

	if err != nil {
		return Balance{},
			Movement{},
			"",
			err
	}

	balanceRevision, err :=
		updateBalanceTx(
			ctx,
			tx,
			item,
			quantity,
			0,
			0,
			0,
			0,
			0,
		)

	if err != nil {
		return Balance{},
			Movement{},
			"",
			err
	}

	inventoryRevision, err :=
		bumpInventoryRevisionTx(
			ctx,
			tx,
			item.BusinessID,
			item.BranchID,
		)

	if err != nil {
		return Balance{},
			Movement{},
			"",
			err
	}

	movement, err :=
		appendMovementTx(
			ctx,
			tx,
			actor,
			item,
			lotID,
			movementType,
			quantity,
			quantity,
			0,
			0,
			0,
			0,
			0,
			sourceType,
			sourceRef,
			reasonCode,
			inventoryRevision,
			balanceRevision,
		)

	if err != nil {
		return Balance{},
			Movement{},
			"",
			err
	}

	balance, err :=
		loadBalanceTx(
			ctx,
			tx,
			item.BusinessID,
			item.ID,
		)

	return balance,
		movement,
		lotID,
		err
}

func allocationOrder(
	policy string,
) string {
	if policy == LotPolicyFEFO {
		return `
            expires_at ASC NULLS LAST,
            received_at ASC,
            id ASC
        `
	}

	return `
        received_at ASC,
        id ASC
    `
}

func allocateAvailableLotsTx(
	ctx context.Context,
	tx pgx.Tx,
	item stockItemState,
	quantity int64,
) (
	[]lotAllocation,
	error,
) {
	orderBy :=
		allocationOrder(
			item.LotPolicy,
		)

	rows, err := tx.Query(
		ctx,
		`
        SELECT
            id,
            on_hand_quantity
            - reserved_quantity
            - quarantined_quantity
          FROM inventory_lots
         WHERE business_id=$1
           AND branch_id=$2
           AND inventory_item_id=$3
           AND status='ACTIVE'
           AND (
               expires_at IS NULL
               OR expires_at > now()
           )
           AND on_hand_quantity >
               reserved_quantity
               + quarantined_quantity
         ORDER BY `+orderBy+`
         FOR UPDATE
        `,
		item.BusinessID,
		item.BranchID,
		item.ID,
	)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	remaining := quantity

	allocations :=
		make(
			[]lotAllocation,
			0,
			4,
		)

	for rows.Next() {
		var (
			lotID     string
			available int64
		)

		if err := rows.Scan(
			&lotID,
			&available,
		); err != nil {
			return nil, err
		}

		if available <= 0 {
			continue
		}

		take := available

		if take > remaining {
			take = remaining
		}

		allocations = append(
			allocations,
			lotAllocation{
				ID:       lotID,
				Quantity: take,
			},
		)

		remaining -= take

		if remaining == 0 {
			break
		}
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if remaining != 0 {
		return nil,
			ErrInsufficientStock
	}

	return allocations, nil
}

func allocateReservedLotsTx(
	ctx context.Context,
	tx pgx.Tx,
	item stockItemState,
	quantity int64,
) (
	[]lotAllocation,
	error,
) {
	rows, err := tx.Query(
		ctx,
		`
        SELECT
            id,
            reserved_quantity
          FROM inventory_lots
         WHERE business_id=$1
           AND branch_id=$2
           AND inventory_item_id=$3
           AND reserved_quantity > 0
         ORDER BY received_at,id
         FOR UPDATE
        `,
		item.BusinessID,
		item.BranchID,
		item.ID,
	)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	remaining := quantity

	var allocations []lotAllocation

	for rows.Next() {
		var (
			lotID    string
			reserved int64
		)

		if err := rows.Scan(
			&lotID,
			&reserved,
		); err != nil {
			return nil, err
		}

		take := reserved

		if take > remaining {
			take = remaining
		}

		allocations = append(
			allocations,
			lotAllocation{
				ID:       lotID,
				Quantity: take,
			},
		)

		remaining -= take

		if remaining == 0 {
			break
		}
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if remaining != 0 {
		return nil,
			ErrInsufficientStock
	}

	return allocations, nil
}

func allocateQuarantinedLotsTx(
	ctx context.Context,
	tx pgx.Tx,
	item stockItemState,
	quantity int64,
) (
	[]lotAllocation,
	error,
) {
	rows, err := tx.Query(
		ctx,
		`
        SELECT
            id,
            quarantined_quantity
          FROM inventory_lots
         WHERE business_id=$1
           AND branch_id=$2
           AND inventory_item_id=$3
           AND quarantined_quantity > 0
         ORDER BY received_at,id
         FOR UPDATE
        `,
		item.BusinessID,
		item.BranchID,
		item.ID,
	)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	remaining := quantity

	var allocations []lotAllocation

	for rows.Next() {
		var (
			lotID       string
			quarantined int64
		)

		if err := rows.Scan(
			&lotID,
			&quarantined,
		); err != nil {
			return nil, err
		}

		take := quarantined

		if take > remaining {
			take = remaining
		}

		allocations = append(
			allocations,
			lotAllocation{
				ID:       lotID,
				Quantity: take,
			},
		)

		remaining -= take

		if remaining == 0 {
			break
		}
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if remaining != 0 {
		return nil,
			ErrInsufficientStock
	}

	return allocations, nil
}

func (s *StockService) Receive(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	input ReceiveInput,
) (
	Balance,
	error,
) {
	input.BranchID =
		strings.TrimSpace(
			input.BranchID,
		)

	input.InventoryItemID =
		strings.TrimSpace(
			input.InventoryItemID,
		)

	if input.Quantity <= 0 ||
		input.Quantity > maxStockQuantity {
		return Balance{},
			ErrInvalidQuantity
	}

	if input.SourceType == "" {
		input.SourceType = "MANUAL_RECEIVE"
	}

	if input.SourceRef == "" {
		input.SourceRef =
			stockID("recv_")
	}

	var out Balance

	err := s.withWriteTransaction(
		ctx,
		actor,
		scope,
		input.BranchID,
		func(
			inventoryTx InventoryTransaction,
			tx pgx.Tx,
		) error {
			item, err :=
				loadStockItemForUpdate(
					ctx,
					tx,
					actor.BusinessID,
					input.BranchID,
					input.InventoryItemID,
				)

			if err != nil {
				return err
			}

			balance,
				movement,
				_,
				err :=
				s.increasePhysicalTx(
					ctx,
					tx,
					actor,
					item,
					input.Quantity,
					MovementReceive,
					input.SourceType,
					input.SourceRef,
					input.ReasonCode,
					input.Lot,
				)

			if err != nil {
				return err
			}

			out = balance

			return s.recordMutation(
				ctx,
				inventoryTx,
				actor,
				scope,
				"inventory.stock.received",
				"inventory.stock.received",
				"inventory_item",
				item.ID,
				input.ReasonCode,
				movement.InventoryRevision,
				map[string]any{
					"movement": movement,
					"balance":  balance,
				},
			)
		},
	)

	return out, err
}

func (s *StockService) Reserve(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	input QuantityMutationInput,
) (
	Balance,
	error,
) {
	return s.changeReserved(
		ctx,
		actor,
		scope,
		input,
		true,
	)
}

func (s *StockService) Release(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	input QuantityMutationInput,
) (
	Balance,
	error,
) {
	return s.changeReserved(
		ctx,
		actor,
		scope,
		input,
		false,
	)
}

func (s *StockService) changeReserved(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	input QuantityMutationInput,
	reserve bool,
) (
	Balance,
	error,
) {
	if input.Quantity <= 0 ||
		input.Quantity > maxStockQuantity {
		return Balance{},
			ErrInvalidQuantity
	}

	var out Balance

	err := s.withWriteTransaction(
		ctx,
		actor,
		scope,
		input.BranchID,
		func(
			inventoryTx InventoryTransaction,
			tx pgx.Tx,
		) error {
			item, err :=
				loadStockItemForUpdate(
					ctx,
					tx,
					actor.BusinessID,
					input.BranchID,
					input.InventoryItemID,
				)

			if err != nil {
				return err
			}

			if err := ensureBalanceTx(
				ctx,
				tx,
				item,
			); err != nil {
				return err
			}

			var allocations []lotAllocation

			if reserve {
				allocations, err =
					allocateAvailableLotsTx(
						ctx,
						tx,
						item,
						input.Quantity,
					)
			} else {
				allocations, err =
					allocateReservedLotsTx(
						ctx,
						tx,
						item,
						input.Quantity,
					)
			}

			if err != nil {
				return err
			}

			delta := input.Quantity

			movementType :=
				MovementReserve

			if !reserve {
				delta = -input.Quantity
				movementType =
					MovementRelease
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
                    `,
					allocation.ID,
					item.BusinessID,
					func() int64 {
						if reserve {
							return allocation.Quantity
						}

						return -allocation.Quantity
					}(),
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
					delta,
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

			var movements []Movement

			for _, allocation := range allocations {

				lotDelta :=
					allocation.Quantity

				if !reserve {
					lotDelta =
						-allocation.Quantity
				}

				movement, err :=
					appendMovementTx(
						ctx,
						tx,
						actor,
						item,
						allocation.ID,
						movementType,
						allocation.Quantity,
						0,
						lotDelta,
						0,
						0,
						0,
						0,
						input.SourceType,
						input.SourceRef,
						input.ReasonCode,
						inventoryRevision,
						balanceRevision,
					)

				if err != nil {
					return err
				}

				movements = append(
					movements,
					movement,
				)
			}

			out, err =
				loadBalanceTx(
					ctx,
					tx,
					item.BusinessID,
					item.ID,
				)

			if err != nil {
				return err
			}

			action :=
				"inventory.stock.reserved"

			if !reserve {
				action =
					"inventory.stock.released"
			}

			return s.recordMutation(
				ctx,
				inventoryTx,
				actor,
				scope,
				action,
				action,
				"inventory_item",
				item.ID,
				input.ReasonCode,
				inventoryRevision,
				map[string]any{
					"movements": movements,
					"balance":   out,
				},
			)
		},
	)

	return out, err
}

func (s *StockService) Quarantine(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	input QuantityMutationInput,
) (
	Balance,
	error,
) {
	return s.changeQuarantine(
		ctx,
		actor,
		scope,
		input,
		true,
	)
}

func (s *StockService) ReleaseQuarantine(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	input QuantityMutationInput,
) (
	Balance,
	error,
) {
	return s.changeQuarantine(
		ctx,
		actor,
		scope,
		input,
		false,
	)
}

func (s *StockService) changeQuarantine(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	input QuantityMutationInput,
	quarantine bool,
) (
	Balance,
	error,
) {
	if input.Quantity <= 0 ||
		input.Quantity > maxStockQuantity {
		return Balance{},
			ErrInvalidQuantity
	}

	var out Balance

	err := s.withWriteTransaction(
		ctx,
		actor,
		scope,
		input.BranchID,
		func(
			inventoryTx InventoryTransaction,
			tx pgx.Tx,
		) error {
			item, err :=
				loadStockItemForUpdate(
					ctx,
					tx,
					actor.BusinessID,
					input.BranchID,
					input.InventoryItemID,
				)

			if err != nil {
				return err
			}

			var allocations []lotAllocation

			if quarantine {
				allocations, err =
					allocateAvailableLotsTx(
						ctx,
						tx,
						item,
						input.Quantity,
					)
			} else {
				allocations, err =
					allocateQuarantinedLotsTx(
						ctx,
						tx,
						item,
						input.Quantity,
					)
			}

			if err != nil {
				return err
			}

			delta := input.Quantity
			movementType :=
				MovementQuarantineIn

			if !quarantine {
				delta = -input.Quantity
				movementType =
					MovementQuarantineRelease
			}

			for _, allocation := range allocations {

				lotDelta :=
					allocation.Quantity

				if !quarantine {
					lotDelta =
						-allocation.Quantity
				}

				_, err := tx.Exec(
					ctx,
					`
                    UPDATE inventory_lots
                       SET quarantined_quantity=
                           quarantined_quantity+$3,
                           status=
                           CASE
                               WHEN quarantined_quantity+$3
                                    = on_hand_quantity
                               THEN 'QUARANTINED'
                               WHEN quarantined_quantity+$3 = 0
                                    AND on_hand_quantity > 0
                               THEN 'ACTIVE'
                               ELSE status
                           END,
                           updated_at=now(),
                           version=version+1
                     WHERE id=$1
                       AND business_id=$2
                    `,
					allocation.ID,
					item.BusinessID,
					lotDelta,
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
					0,
					delta,
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

			var movements []Movement

			for _, allocation := range allocations {

				lotDelta :=
					allocation.Quantity

				if !quarantine {
					lotDelta =
						-allocation.Quantity
				}

				movement, err :=
					appendMovementTx(
						ctx,
						tx,
						actor,
						item,
						allocation.ID,
						movementType,
						allocation.Quantity,
						0,
						0,
						lotDelta,
						0,
						0,
						0,
						input.SourceType,
						input.SourceRef,
						input.ReasonCode,
						inventoryRevision,
						balanceRevision,
					)

				if err != nil {
					return err
				}

				movements = append(
					movements,
					movement,
				)
			}

			out, err =
				loadBalanceTx(
					ctx,
					tx,
					item.BusinessID,
					item.ID,
				)

			if err != nil {
				return err
			}

			action :=
				"inventory.stock.quarantined"

			if !quarantine {
				action =
					"inventory.stock.quarantine_released"
			}

			return s.recordMutation(
				ctx,
				inventoryTx,
				actor,
				scope,
				action,
				action,
				"inventory_item",
				item.ID,
				input.ReasonCode,
				inventoryRevision,
				map[string]any{
					"movements": movements,
					"balance":   out,
				},
			)
		},
	)

	return out, err
}

func (s *StockService) Waste(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	input QuantityMutationInput,
) (
	Balance,
	error,
) {
	return s.decreasePhysical(
		ctx,
		actor,
		scope,
		input,
		MovementWaste,
	)
}

func (s *StockService) Adjust(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	input QuantityMutationInput,
	delta int64,
) (
	Balance,
	error,
) {
	if delta == 0 ||
		delta > maxStockQuantity ||
		delta < -maxStockQuantity {
		return Balance{},
			ErrInvalidQuantity
	}

	input.Quantity = delta

	if delta > 0 {
		receive := ReceiveInput{
			BranchID: input.BranchID,

			InventoryItemID: input.InventoryItemID,

			Quantity: delta,

			SourceType: input.SourceType,

			SourceRef: input.SourceRef,

			ReasonCode: input.ReasonCode,
		}

		var out Balance

		err := s.withWriteTransaction(
			ctx,
			actor,
			scope,
			input.BranchID,
			func(
				inventoryTx InventoryTransaction,
				tx pgx.Tx,
			) error {
				item, err :=
					loadStockItemForUpdate(
						ctx,
						tx,
						actor.BusinessID,
						input.BranchID,
						input.InventoryItemID,
					)

				if err != nil {
					return err
				}

				balance,
					movement,
					_,
					err :=
					s.increasePhysicalTx(
						ctx,
						tx,
						actor,
						item,
						receive.Quantity,
						MovementAdjustmentIn,
						receive.SourceType,
						receive.SourceRef,
						receive.ReasonCode,
						receive.Lot,
					)

				if err != nil {
					return err
				}

				out = balance

				return s.recordMutation(
					ctx,
					inventoryTx,
					actor,
					scope,
					"inventory.stock.adjusted",
					"inventory.stock.adjusted",
					"inventory_item",
					item.ID,
					input.ReasonCode,
					movement.InventoryRevision,
					map[string]any{
						"movement": movement,
						"balance":  balance,
					},
				)
			},
		)

		return out, err
	}

	input.Quantity = -delta

	return s.decreasePhysical(
		ctx,
		actor,
		scope,
		input,
		MovementAdjustmentOut,
	)
}

func (s *StockService) decreasePhysical(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	input QuantityMutationInput,
	movementType string,
) (
	Balance,
	error,
) {
	if (movementType == MovementWaste ||
		movementType == MovementDamage ||
		movementType == MovementExpiry) &&
		strings.TrimSpace(input.ReasonCode) == "" {

		return Balance{}, ErrReasonRequired
	}
	if input.Quantity <= 0 ||
		input.Quantity > maxStockQuantity {
		return Balance{},
			ErrInvalidQuantity
	}

	var out Balance

	err := s.withWriteTransaction(
		ctx,
		actor,
		scope,
		input.BranchID,
		func(
			inventoryTx InventoryTransaction,
			tx pgx.Tx,
		) error {
			item, err :=
				loadStockItemForUpdate(
					ctx,
					tx,
					actor.BusinessID,
					input.BranchID,
					input.InventoryItemID,
				)

			if err != nil {
				return err
			}

			allocations, err :=
				allocateAvailableLotsTx(
					ctx,
					tx,
					item,
					input.Quantity,
				)

			if err != nil {
				return err
			}

			for _, allocation := range allocations {

				_, err := tx.Exec(
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
                    `,
					allocation.ID,
					item.BusinessID,
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
					-input.Quantity,
					0,
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

			var movements []Movement

			for _, allocation := range allocations {

				movement, err :=
					appendMovementTx(
						ctx,
						tx,
						actor,
						item,
						allocation.ID,
						movementType,
						allocation.Quantity,
						-allocation.Quantity,
						0,
						0,
						0,
						0,
						0,
						input.SourceType,
						input.SourceRef,
						input.ReasonCode,
						inventoryRevision,
						balanceRevision,
					)

				if err != nil {
					return err
				}

				movements = append(
					movements,
					movement,
				)
			}

			out, err =
				loadBalanceTx(
					ctx,
					tx,
					item.BusinessID,
					item.ID,
				)

			if err != nil {
				return err
			}

			action :=
				"inventory.stock.wasted"

			if movementType ==
				MovementAdjustmentOut {
				action =
					"inventory.stock.adjusted"
			}

			return s.recordMutation(
				ctx,
				inventoryTx,
				actor,
				scope,
				action,
				action,
				"inventory_item",
				item.ID,
				input.ReasonCode,
				inventoryRevision,
				map[string]any{
					"movements": movements,
					"balance":   out,
				},
			)
		},
	)

	return out, err
}
