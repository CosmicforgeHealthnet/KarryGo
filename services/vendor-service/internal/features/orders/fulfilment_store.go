package orders

// Section 17F fulfilment persistence.
//
// Every mission row is durable BEFORE the executing authority is called, and
// its mission reference is the key that authority keys on. A crash between the
// two therefore leaves a mission a retry can find, rather than a vehicle
// dispatched for goods nothing recorded.

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// FulfilledQuantities returns the latest picking quantity by authoritative
// Inventory item. The bool distinguishes “no Shop picking evidence exists”
// from a real outcome whose fulfilled quantity is zero.
func (s *Store) FulfilledQuantities(ctx context.Context, vendorOrderID string) (map[string]int64, bool, error) {
	if s == nil || s.db == nil {
		return nil, false, ErrDependencyMissing
	}
	rows, err := s.db.Query(ctx, `SELECT l.inventory_item_ref,
	 CASE WHEN l.pricing_basis='VARIABLE_MEASURE' THEN COALESCE(o.actual_measure_quantity,0) ELSE COALESCE(o.picked_quantity,0) END,
	 o.id IS NOT NULL
	 FROM commerce_vendor_order_lines l LEFT JOIN commerce_vendor_order_line_outcomes o ON o.order_line_id=l.id WHERE l.vendor_order_id=$1 AND l.inventory_item_ref<>''`, vendorOrderID)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	result := map[string]int64{}
	found := false
	for rows.Next() {
		var item string
		var quantity int64
		var hasOutcome bool
		if err = rows.Scan(&item, &quantity, &hasOutcome); err != nil {
			return nil, false, err
		}
		result[item] += quantity
		found = found || hasOutcome
	}
	return result, found, rows.Err()
}

// Reservation identities for replacements remain Inventory's authority. The
// order only supplies the approved variant and quantity from its own evidence.
func (s *Service) fulfilledReservationQuantities(ctx context.Context, order VendorOrder) (map[string]int64, bool, error) {
	quantities, found, err := s.store.FulfilledQuantities(ctx, order.ID)
	if err != nil {
		return nil, false, err
	}
	proposals, err := s.store.approvedSubstitutionsForOrder(ctx, order.ID)
	if err != nil {
		return nil, false, err
	}
	for _, proposal := range proposals {
		authority, ok := s.inventory.(ReplacementInventoryAuthority)
		if !ok {
			return nil, false, ErrDependencyMissing
		}
		item, resolveErr := authority.ResolveShopVariantItem(ctx, order.BusinessID, order.BranchID, proposal.ProposedVariantRef)
		if resolveErr != nil {
			return nil, false, resolveErr
		}
		quantity, valid := checkedMinorSum(quantities[item], proposal.ProposedQuantity)
		if !valid {
			return nil, false, ErrOrderConflict
		}
		quantities[item], found = quantity, true
	}
	return quantities, found, nil
}

func (s *Store) approvedSubstitutionsForOrder(ctx context.Context, vendorOrderID string) ([]SubstitutionProposal, error) {
	rows, err := s.db.Query(ctx, `SELECT id,parent_order_id,vendor_order_id,order_line_id,customer_id,business_id,branch_id,proposed_item_ref,proposed_variant_ref,proposed_catalogue_ref,replacement_snapshot,proposed_quantity,proposed_price_minor,price_difference_minor,reason,expires_at,status,response_ref,version,created_at,responded_at,updated_at,COALESCE(case_id,'') FROM commerce_substitution_proposals WHERE vendor_order_id=$1 AND status='APPROVED' ORDER BY id`, vendorOrderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var proposals []SubstitutionProposal
	for rows.Next() {
		proposal, readErr := scanSubstitution(rows)
		if readErr != nil {
			return nil, readErr
		}
		proposals = append(proposals, proposal)
	}
	return proposals, rows.Err()
}

const missionColumns = `
	m.id,m.mission_ref,m.parent_order_id,m.plan_mission_ref,m.service,m.vehicle_class,
	m.agreed_fee_minor,m.currency,m.status,m.external_ref,m.refusal_code,
	m.failure_reason,m.version,m.created_at,m.updated_at`

// GetParentByID reads one order without a customer scope.
//
// It exists for activation and recovery, which act on behalf of the system
// rather than a signed-in customer. Every CUSTOMER-facing read still goes
// through GetParent, which is scoped.
func (s *Store) GetParentByID(ctx context.Context, parentOrderID string) (ParentOrder, error) {
	if s == nil || s.db == nil {
		return ParentOrder{}, ErrDependencyMissing
	}
	var customerID string
	if err := s.db.QueryRow(ctx,
		`SELECT customer_id FROM commerce_parent_orders WHERE id=$1`,
		parentOrderID).Scan(&customerID); errors.Is(err, pgx.ErrNoRows) {
		return ParentOrder{}, ErrOrderNotFound
	} else if err != nil {
		return ParentOrder{}, err
	}
	return loadParent(ctx, s.db, customerID, parentOrderID)
}

// SetInventoryState records where one allocation's stock stands.
//
// The state is explicit rather than inferred, which is what stops stock being
// committed twice by two readers that each guessed it had not been.
func (s *Store) SetInventoryState(
	ctx context.Context, vendorOrderID string, state InventoryState, now time.Time,
) error {
	if s == nil || s.db == nil {
		return ErrDependencyMissing
	}
	_, err := s.db.Exec(ctx, `
		UPDATE commerce_vendor_orders
		   SET inventory_state=$2,version=version+1,updated_at=$3
		 WHERE id=$1 AND inventory_state <> 'NOT_REQUIRED'`,
		vendorOrderID, state, now)
	return err
}

// EnsureMission creates the durable mission row, or returns the existing one.
//
// UNIQUE(mission_ref) and UNIQUE(parent_order_id, plan_mission_ref) together
// mean a retried activation can never produce a second mission for the same
// delivery.
func (s *Store) EnsureMission(
	ctx context.Context, order ParentOrder, plan MissionPlan, now time.Time,
) (OrderMission, bool, error) {
	if s == nil || s.db == nil {
		return OrderMission{}, false, ErrDependencyMissing
	}

	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return OrderMission{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err = tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1,17))`,
		order.ID+":"+plan.PlanMissionRef); err != nil {
		return OrderMission{}, false, err
	}

	existing, err := loadMissionBy(ctx, tx,
		`m.parent_order_id=$1 AND m.plan_mission_ref=$2`, order.ID, plan.PlanMissionRef)
	if err == nil {
		if err = tx.Commit(ctx); err != nil {
			return OrderMission{}, false, err
		}
		return existing, false, nil
	}
	if !errors.Is(err, ErrOrderNotFound) {
		return OrderMission{}, false, err
	}

	missionID := "cvm_" + uuid.NewString()
	// The mission reference is derived from the order and its plan mission, so
	// it is stable across retries and across a restart. The executing
	// authority keys on it, which is what makes creation idempotent END TO END
	// rather than only on this side.
	missionRef := "vfm_" + order.ID + "_" + plan.PlanMissionRef
	if strings.TrimSpace(plan.MissionRef) != "" {
		missionRef = strings.TrimSpace(plan.MissionRef)
	}

	if _, err = tx.Exec(ctx, `
		INSERT INTO commerce_order_missions(
			id,mission_ref,parent_order_id,customer_id,plan_mission_ref,service,
			vehicle_class,agreed_fee_minor,currency,status,version,created_at,updated_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'PENDING',1,$10,$10)`,
		missionID, missionRef, order.ID, order.CustomerID, plan.PlanMissionRef,
		plan.Service, plan.VehicleClass, plan.AgreedFeeMinor, order.Currency,
		now); err != nil {
		return OrderMission{}, false, err
	}

	for _, vendorOrderID := range plan.VendorOrderIDs {
		if _, err = tx.Exec(ctx, `
			INSERT INTO commerce_order_mission_allocations(
				id,mission_id,parent_order_id,vendor_order_id,created_at)
			VALUES($1,$2,$3,$4,$5)`,
			"cvma_"+uuid.NewString(), missionID, order.ID, vendorOrderID, now); err != nil {
			return OrderMission{}, false, err
		}
	}

	created, err := loadMissionBy(ctx, tx, `m.id=$1`, missionID)
	if err != nil {
		return OrderMission{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return OrderMission{}, false, err
	}
	return created, true, nil
}

// GetMissionByRef reads one mission slot by its stable reference.
//
// The reference, not the internal id, is what the just-in-time orchestration
// carries: it is derived from the order and its plan mission, so it survives a
// restart and means the same thing to the executing authority.
func (s *Store) GetMissionByRef(ctx context.Context, missionRef string) (OrderMission, error) {
	if s == nil || s.db == nil {
		return OrderMission{}, ErrDependencyMissing
	}
	return loadMissionBy(ctx, s.db, `m.mission_ref=$1`, missionRef)
}

// RecordMissionResult stores the executing authority's own answer.
func (s *Store) RecordMissionResult(
	ctx context.Context, missionID string, result MissionResult, now time.Time,
) error {
	if s == nil || s.db == nil {
		return ErrDependencyMissing
	}
	_, err := s.db.Exec(ctx, `
		UPDATE commerce_order_missions
		   SET status=$2,external_ref=$3,refusal_code=$4,failure_reason='',
		       version=version+1,updated_at=$5
		 WHERE id=$1 AND status='PENDING'`,
		missionID, result.Status, result.ExternalRef, result.RefusalCode, now)
	return err
}

// RecordMissionFailure records that the authority could not be reached.
//
// The mission stays PENDING on purpose: an unreachable authority is an UNKNOWN
// outcome, and recovery retries the same mission reference rather than this
// code deciding the delivery will never happen.
func (s *Store) RecordMissionFailure(
	ctx context.Context, missionID, reason string, now time.Time,
) error {
	if s == nil || s.db == nil {
		return ErrDependencyMissing
	}
	_, err := s.db.Exec(ctx, `
		UPDATE commerce_order_missions
		   SET failure_reason=$2,version=version+1,updated_at=$3
		 WHERE id=$1 AND status='PENDING'`, missionID, reason, now)
	return err
}

// ListMissions reads one order's missions.
func (s *Store) ListMissions(
	ctx context.Context, parentOrderID string,
) ([]OrderMission, error) {
	if s == nil || s.db == nil {
		return nil, ErrDependencyMissing
	}
	rows, err := s.db.Query(ctx,
		`SELECT `+missionColumns+` FROM commerce_order_missions m
		  WHERE m.parent_order_id=$1 ORDER BY m.plan_mission_ref, m.id`, parentOrderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var missions []OrderMission
	for rows.Next() {
		mission, scanErr := scanMission(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		missions = append(missions, mission)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	for i := range missions {
		if missions[i].VendorOrderIDs, err = loadMissionAllocations(
			ctx, s.db, missions[i].ID,
		); err != nil {
			return nil, err
		}
	}
	return missions, nil
}

// ListOrdersAwaitingActivation finds paid orders whose fulfilment has not been
// fully activated.
//
// This is the crash-recovery read for the fulfilment-pending boundary: a
// process that died between holding a payment and creating a mission left an
// order here, and it is found by state rather than by a goroutine that no
// longer exists.
func (s *Store) ListOrdersAwaitingActivation(
	ctx context.Context, limit int,
) ([]string, error) {
	if s == nil || s.db == nil {
		return nil, ErrDependencyMissing
	}
	rows, err := s.db.Query(ctx, `
		SELECT DISTINCT p.id
		  FROM commerce_parent_orders p
		  JOIN commerce_vendor_orders v ON v.parent_order_id=p.id
		 WHERE p.payment_state='HELD'
		   AND v.status NOT IN ('CANCELLED','REJECTED','REFUND_PENDING','REFUNDED','PARTIALLY_REFUNDED','COMPLETED')
		   AND (
		         v.inventory_state = 'HELD'
		         OR EXISTS(
		              SELECT 1 FROM commerce_order_missions m
		               WHERE m.parent_order_id=p.id AND m.status='PENDING')
		         OR (
		              v.delivery_responsibility='PLATFORM_LOGISTICS'
		              AND NOT EXISTS(
		                   SELECT 1 FROM commerce_order_mission_allocations a
		                    WHERE a.vendor_order_id=v.id)
		         )
		       )
		 ORDER BY p.id
		 LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func loadMissionBy(
	ctx context.Context, q queryer, where string, args ...any,
) (OrderMission, error) {
	mission, err := scanMission(q.QueryRow(ctx,
		`SELECT `+missionColumns+` FROM commerce_order_missions m WHERE `+where, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return OrderMission{}, ErrOrderNotFound
	}
	if err != nil {
		return OrderMission{}, err
	}
	if mission.VendorOrderIDs, err = loadMissionAllocations(ctx, q, mission.ID); err != nil {
		return OrderMission{}, err
	}
	return mission, nil
}

func scanMission(row scanner) (OrderMission, error) {
	var mission OrderMission
	err := row.Scan(&mission.ID, &mission.MissionRef, &mission.ParentOrderID,
		&mission.PlanMissionRef, &mission.Service, &mission.VehicleClass,
		&mission.AgreedFeeMinor, &mission.Currency, &mission.Status,
		&mission.ExternalRef, &mission.RefusalCode, &mission.FailureReason,
		&mission.Version, &mission.CreatedAt, &mission.UpdatedAt)
	return mission, err
}

func loadMissionAllocations(
	ctx context.Context, q queryer, missionID string,
) ([]string, error) {
	rows, err := q.Query(ctx,
		`SELECT vendor_order_id FROM commerce_order_mission_allocations
		  WHERE mission_id=$1 ORDER BY vendor_order_id`, missionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
