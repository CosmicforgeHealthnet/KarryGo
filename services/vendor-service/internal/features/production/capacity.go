package production

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

// ---------------------------------------------------------------------------
// The capacity engine
// ---------------------------------------------------------------------------
//
// Overbooking is prevented the only way it can be prevented: by serialising the
// readers of a pool's usage against its writers. Every reserving transaction
// locks the affected capacity pool ROWS with SELECT ... FOR UPDATE, ordered by
// id, and only then computes what is already taken.
//
// The ordering does two jobs. It serialises contenders on a shared pool, so of
// two simultaneous 60-unit requests against a capacity of 100, the second sees
// the first's committed allocation and is refused. And it makes multi-pool
// reservations deadlock-safe: two callers asking for the same pools in opposite
// orders still take the locks in the same order, so neither can hold what the
// other needs.
//
// What is explicitly NOT done here: reading remaining capacity, deciding in Go,
// and then inserting. That pattern has a window between the read and the write
// in which another transaction commits, and no amount of care elsewhere closes
// it.

// AllocationRequest is one pool's share of a reservation.
type AllocationRequest struct {
	CapacityPoolID   string `json:"capacity_pool_id"`
	Quantity         int    `json:"quantity"`
	ProductionLineID string `json:"production_line_id,omitempty"`
	ItemFamily       string `json:"item_family,omitempty"`
}

// ReserveInput is a request for capacity across one or more pools.
type ReserveInput struct {
	BranchID    string
	Capability  string
	WindowStart time.Time
	WindowEnd   time.Time
	Allocations []AllocationRequest
	SourceType  string
	SourceRef   string
	Note        string
	Priority    string
	// HoldFor, when positive, creates a HELD reservation that stops consuming
	// capacity when it expires. Zero creates a CONFIRMED reservation.
	HoldFor time.Duration
}

// lockedPool is a pool row held under FOR UPDATE for the rest of the
// transaction.
type lockedPool struct {
	id                   string
	code                 string
	capability           string
	branchID             string
	unitType             string
	defaultCapacity      int
	setupBufferMinutes   int
	cleanupBufferMinutes int
	status               string
}

// lockPools takes the pool rows in ascending id order.
//
// The ORDER BY inside the locking statement is the deadlock guarantee: the
// caller's allocation order is irrelevant, because PostgreSQL acquires the row
// locks in the order the query produces them.
func lockPools(ctx context.Context, tx pgx.Tx, businessID string, poolIDs []string) (map[string]lockedPool, error) {
	sorted := append([]string(nil), poolIDs...)
	sort.Strings(sorted)
	rows, err := tx.Query(ctx, `
		SELECT id, code, capability, branch_id, unit_type, default_capacity,
			setup_buffer_minutes, cleanup_buffer_minutes, status
		FROM production_capacity_pools
		WHERE business_id = $1 AND id = ANY($2)
		ORDER BY id
		FOR UPDATE`, businessID, sorted)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	pools := map[string]lockedPool{}
	for rows.Next() {
		var pool lockedPool
		if err = rows.Scan(&pool.id, &pool.code, &pool.capability, &pool.branchID, &pool.unitType,
			&pool.defaultCapacity, &pool.setupBufferMinutes, &pool.cleanupBufferMinutes,
			&pool.status); err != nil {
			return nil, err
		}
		pools[pool.id] = pool
	}
	return pools, rows.Err()
}

// effectiveWindow widens a requested window by the pool's setup and cleanup
// buffers, which is the interval the resource is genuinely occupied for.
func (p lockedPool) effectiveWindow(start, end time.Time) (time.Time, time.Time) {
	return start.Add(-time.Duration(p.setupBufferMinutes) * time.Minute),
		end.Add(time.Duration(p.cleanupBufferMinutes) * time.Minute)
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// effectiveBuffers resolves the bounded buffer hierarchy for a new
// allocation.  The largest applicable setup and cleanup values win: a product
// profile may require more time than its line, and a line may require more
// time than its pool, but a narrower profile must never accidentally shorten
// the period during which the shared resource is occupied.
func effectiveBuffers(ctx context.Context, tx pgx.Tx, businessID string, pool lockedPool, productionLineID, sourceType, sourceRef string) (int, int, error) {
	setup, cleanup := pool.setupBufferMinutes, pool.cleanupBufferMinutes
	var lineSetup, lineCleanup int
	if productionLineID != "" {
		var linePool *string
		var lineBranch, lineStatus string
		if err := tx.QueryRow(ctx, `SELECT capacity_pool_id,branch_id,status,setup_buffer_minutes,cleanup_buffer_minutes FROM production_lines WHERE business_id=$1 AND id=$2 FOR SHARE`, businessID, productionLineID).Scan(&linePool, &lineBranch, &lineStatus, &lineSetup, &lineCleanup); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return 0, 0, errNotFound("Production line", CodeLineNotFound)
			}
			return 0, 0, err
		}
		if lineStatus != StatusActive || lineBranch != pool.branchID || linePool == nil || *linePool != pool.id {
			return 0, 0, coded(apperrors.Conflict("The production line is not active for this capacity pool.", nil), CodeLineNotFound)
		}
	} else if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(setup_buffer_minutes),0),COALESCE(MAX(cleanup_buffer_minutes),0) FROM production_lines WHERE business_id=$1 AND capacity_pool_id=$2 AND status='ACTIVE'`, businessID, pool.id).Scan(&lineSetup, &lineCleanup); err != nil {
		return 0, 0, err
	}
	if lineSetup > setup {
		setup = lineSetup
	}
	if lineCleanup > cleanup {
		cleanup = lineCleanup
	}
	var profileSetup, profileCleanup int
	switch sourceType {
	case "BULK_PACKAGE":
		err := tx.QueryRow(ctx, `SELECT setup_buffer_minutes,cleanup_buffer_minutes FROM cloud_kitchen_packages WHERE business_id=$1 AND id=$2 AND status<>'ARCHIVED'`, businessID, sourceRef).Scan(&profileSetup, &profileCleanup)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return 0, 0, err
		}
	case "PASTRY_PRODUCT":
		err := tx.QueryRow(ctx, `SELECT setup_buffer_minutes,cleanup_buffer_minutes FROM pastry_products WHERE business_id=$1 AND id=$2 AND status<>'ARCHIVED'`, businessID, sourceRef).Scan(&profileSetup, &profileCleanup)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return 0, 0, err
		}
	case "CORPORATE_OCCURRENCE":
		err := tx.QueryRow(ctx, `SELECT p.setup_buffer_minutes,p.cleanup_buffer_minutes FROM cloud_kitchen_occurrences o JOIN cloud_kitchen_packages p ON p.id=o.package_id WHERE o.id=$1 AND p.business_id=$2`, sourceRef, businessID).Scan(&profileSetup, &profileCleanup)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return 0, 0, err
		}
	}
	if profileSetup > setup {
		setup = profileSetup
	}
	if profileCleanup > cleanup {
		cleanup = profileCleanup
	}
	return setup, cleanup, nil
}

// poolUsage sums what is already taken from a pool over an interval.
//
// The reservation-status predicate is the fail-safe part: only HELD and
// CONFIRMED count, and a HELD row stops counting once its hold_expires_at has
// passed — evaluated against the supplied instant, not against whether a
// cleanup worker has run.
func poolUsage(ctx context.Context, tx pgx.Tx, businessID, poolID string, start, end time.Time, at time.Time, excludeReservation string) (confirmed int, held int, err error) {
	err = tx.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN r.status = 'CONFIRMED'
				THEN a.quantity - a.released_quantity ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN r.status = 'HELD'
				AND (r.hold_expires_at IS NULL OR r.hold_expires_at > $5)
				THEN a.quantity - a.released_quantity ELSE 0 END), 0)
		FROM capacity_reservation_allocations a
		JOIN capacity_reservations r
		  ON r.id = a.reservation_id AND r.business_id = a.business_id
		WHERE a.business_id = $1
		  AND a.capacity_pool_id = $2
		  AND r.status IN ('HELD','CONFIRMED')
		  AND ($6 = '' OR r.id <> $6)
		  AND a.effective_start < $4
		  AND a.effective_end > $3`,
		businessID, poolID, start, end, at, excludeReservation).Scan(&confirmed, &held)
	return confirmed, held, err
}

// poolMaximum resolves the binding capacity limit for a pool over a window.
//
// Rules NARROW. The smallest applicable maximum wins, and the pool default is
// simply the widest one — a more specific rule can never raise a ceiling,
// because a rule that could would be a way to overbook the broader resource it
// sits inside.
func poolMaximum(ctx context.Context, tx pgx.Tx, businessID string, pool lockedPool, productionLineID, itemFamily string, start, end time.Time) (int, string, error) {
	maximum := pool.defaultCapacity
	blockingRule := ""
	var timezone string
	if err := tx.QueryRow(ctx, `SELECT timezone FROM vendor_branches WHERE business_id=$1 AND id=$2`, businessID, pool.branchID).Scan(&timezone); err != nil {
		return 0, "", err
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return 0, "", err
	}

	rows, err := tx.Query(ctx, `
		SELECT id, max_quantity, days_of_week, start_local_time, end_local_time,
			effective_start_date, effective_end_date, specificity, production_line_id, item_family
		FROM production_capacity_rules
		WHERE business_id = $1 AND capacity_pool_id = $2 AND status = 'ACTIVE'
		ORDER BY specificity DESC, id`, businessID, pool.id)
	if err != nil {
		return 0, "", err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var maxQuantity int
		var days []int16
		var startClock, endClock *time.Time
		var fromDate, toDate *time.Time
		var ruleLineID, ruleItemFamily *string
		var specificity int
		if err = rows.Scan(&id, &maxQuantity, &days, &startClock, &endClock,
			&fromDate, &toDate, &specificity, &ruleLineID, &ruleItemFamily); err != nil {
			return 0, "", err
		}
		if ruleLineID != nil && *ruleLineID != productionLineID {
			continue
		}
		if ruleItemFamily != nil && !strings.EqualFold(*ruleItemFamily, itemFamily) {
			continue
		}
		if !ruleApplies(days, startClock, endClock, fromDate, toDate, start, end, location) {
			continue
		}
		if maxQuantity < maximum {
			maximum = maxQuantity
			blockingRule = id
		}
	}
	if err = rows.Err(); err != nil {
		return 0, "", err
	}

	// A slot that covers the window may override capacity for that window only.
	var override *int
	if err = tx.QueryRow(ctx, `
		SELECT capacity_override FROM production_slots
		WHERE business_id = $1 AND capacity_pool_id = $2 AND status = 'OPEN'
		  AND capacity_override IS NOT NULL
		  AND start_at <= $3 AND end_at >= $4
		ORDER BY capacity_override ASC LIMIT 1`,
		businessID, pool.id, start, end).Scan(&override); err != nil && err != pgx.ErrNoRows {
		return 0, "", err
	}
	if override != nil && *override < maximum {
		maximum = *override
	}
	return maximum, blockingRule, nil
}

// ruleApplies decides whether a rule's recurrence covers an instant. Dates,
// weekdays, and local windows use the authoritative capacity-pool timezone.
func ruleApplies(days []int16, startClock, endClock, fromDate, toDate *time.Time, windowStart, windowEnd time.Time, location *time.Location) bool {
	localStart := windowStart.In(location)
	localEnd := windowEnd.In(location)
	day := time.Date(localStart.Year(), localStart.Month(), localStart.Day(), 0, 0, 0, 0, location)
	lastDay := time.Date(localEnd.Year(), localEnd.Month(), localEnd.Day(), 0, 0, 0, 0, location)
	for !day.After(lastDay) {
		dateUTC := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
		if fromDate != nil {
			from := time.Date(fromDate.Year(), fromDate.Month(), fromDate.Day(), 0, 0, 0, 0, time.UTC)
			if dateUTC.Before(from) {
				day = day.AddDate(0, 0, 1)
				continue
			}
		}
		if toDate != nil {
			to := time.Date(toDate.Year(), toDate.Month(), toDate.Day(), 0, 0, 0, 0, time.UTC)
			if dateUTC.After(to) {
				day = day.AddDate(0, 0, 1)
				continue
			}
		}
		weekdayMatches := len(days) == 0
		iso := int(day.Weekday())
		if iso == 0 {
			iso = 7
		}
		for _, wanted := range days {
			if int(wanted) == iso {
				weekdayMatches = true
				break
			}
		}
		if weekdayMatches {
			ruleStart, ruleEnd := day, day.AddDate(0, 0, 1)
			if startClock != nil && endClock != nil {
				ruleStart = time.Date(day.Year(), day.Month(), day.Day(), startClock.Hour(), startClock.Minute(), startClock.Second(), 0, location)
				ruleEnd = time.Date(day.Year(), day.Month(), day.Day(), endClock.Hour(), endClock.Minute(), endClock.Second(), 0, location)
				if !ruleEnd.After(ruleStart) {
					ruleEnd = ruleEnd.AddDate(0, 0, 1)
				}
			}
			if ruleStart.Before(windowEnd) && ruleEnd.After(windowStart) {
				return true
			}
		}
		day = day.AddDate(0, 0, 1)
	}
	return false
}

// blackoutFor finds an active blackout covering a pool's window, if any.
//
// A blackout matches when it targets the pool directly, the capability, or the
// whole branch. Existing reservations are untouched; only new ones are refused.
func blackoutFor(ctx context.Context, tx pgx.Tx, businessID, branchID, capability, poolID, productionLineID string, start, end time.Time) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `
		SELECT id FROM production_blackouts
		WHERE business_id = $1 AND branch_id = $2 AND status = 'ACTIVE'
		  AND start_at < $7 AND end_at > $6
		  AND ((production_line_id IS NOT NULL AND production_line_id = NULLIF($5,''))
		       OR (production_line_id IS NULL AND capacity_pool_id = $4)
		       OR (production_line_id IS NULL AND capacity_pool_id IS NULL
		           AND (capability IS NULL OR capability = $3)))
		ORDER BY start_at LIMIT 1`,
		businessID, branchID, capability, poolID, productionLineID, start, end).Scan(&id)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return id, err
}

func slotClosureFor(ctx context.Context, tx pgx.Tx, businessID, poolID, productionLineID string, start, end time.Time) (string, string, error) {
	var id, status string
	err := tx.QueryRow(ctx, `SELECT id,status FROM production_slots
		WHERE business_id=$1 AND capacity_pool_id=$2
		  AND status IN ('CLOSED','BLACKOUT')
		  AND start_at < $5 AND end_at > $4
		  AND (production_line_id IS NULL OR production_line_id=NULLIF($3,''))
		ORDER BY CASE status WHEN 'BLACKOUT' THEN 0 ELSE 1 END,start_at,id LIMIT 1`, businessID, poolID, productionLineID, start, end).Scan(&id, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", nil
	}
	return id, status, err
}

// Reserve takes capacity atomically across every requested pool.
//
// All-or-nothing is the load-bearing property: if any single pool cannot supply
// its share, the whole transaction rolls back and no partial allocation
// survives. A cake that needs an oven, a decorator and a chiller is not half
// reservable.
func (s *Service) Reserve(ctx context.Context, actor Actor, scope Scope, in ReserveInput) (Reservation, []AllocationView, error) {
	if err := s.requireWrites(); err != nil {
		return Reservation{}, nil, err
	}
	branchID := strings.TrimSpace(in.BranchID)
	capability := strings.ToUpper(strings.TrimSpace(in.Capability))
	if !inVocabulary(Capabilities, capability) {
		return Reservation{}, nil, validation(apperrors.FieldViolation{
			Field: "capability", Message: "Unsupported production capability."})
	}
	if !scope.Covers(branchID) {
		return Reservation{}, nil, errForbidden()
	}
	if err := validateWindow(in.WindowStart, in.WindowEnd); err != nil {
		return Reservation{}, nil, err
	}
	requested, err := normalizeAllocations(in.Allocations)
	if err != nil {
		return Reservation{}, nil, err
	}
	sourceType := strings.ToUpper(strings.TrimSpace(in.SourceType))
	if sourceType == "" {
		sourceType = "MANUAL"
	}
	if !inVocabulary(SourceTypes, sourceType) {
		return Reservation{}, nil, validation(apperrors.FieldViolation{
			Field: "source_type", Message: "Unsupported reservation source."})
	}
	priority := strings.ToUpper(strings.TrimSpace(in.Priority))
	if priority == "" {
		priority = "NORMAL"
	}
	if !inVocabulary(Priorities, priority) {
		return Reservation{}, nil, validation(apperrors.FieldViolation{
			Field: "priority", Message: "Priority must be NORMAL, HIGH or URGENT."})
	}
	note := strings.TrimSpace(in.Note)
	if len(note) > 280 {
		return Reservation{}, nil, validation(apperrors.FieldViolation{
			Field: "note", Message: "Note is too long."})
	}

	var (
		reservation Reservation
		allocations []AllocationView
	)
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		now := s.now()
		var holdExpires *time.Time
		status := ReservationConfirmed
		if in.HoldFor > 0 {
			status = ReservationHeld
			expiry := now.Add(in.HoldFor)
			holdExpires = &expiry
		}

		granted, err := s.allocateUnderLock(ctx, tx, actor, allocationContext{
			branchID: branchID, capability: capability,
			windowStart: in.WindowStart, windowEnd: in.WindowEnd,
			requested: requested, at: now, sourceType: sourceType, sourceRef: strings.TrimSpace(in.SourceRef),
		})
		if err != nil {
			return err
		}

		reservationID := newID("cres_")
		row := tx.QueryRow(ctx, `
			INSERT INTO capacity_reservations
				(id,business_id,branch_id,capability,window_start,window_end,
				 source_type,source_ref,status,hold_expires_at,priority,note,command_ref)
			VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),$9,$10,$11,$12,NULLIF($13,''))
			RETURNING `+reservationCols,
			reservationID, actor.VendorID, branchID, capability, in.WindowStart, in.WindowEnd,
			sourceType, strings.TrimSpace(in.SourceRef), status, holdExpires, priority, note,
			actor.CommandRef)
		reservation, err = scanReservation(row)
		if err != nil {
			return mapDB(err, "Reservation", CodeReservationNotFound)
		}

		for _, grant := range granted {
			if _, err = tx.Exec(ctx, `
				INSERT INTO capacity_reservation_allocations
					(id,reservation_id,business_id,capacity_pool_id,production_line_id,quantity,effective_start,effective_end)
				VALUES ($1,$2,$3,$4,NULLIF($5,''),$6,$7,$8)`,
				newID("cal_"), reservation.ID, actor.VendorID, grant.CapacityPoolID,
				grant.ProductionLineID, grant.Quantity, grant.EffectiveStart, grant.EffectiveEnd); err != nil {
				return mapDB(err, "Reservation allocation", CodeInvalidAllocation)
			}
			if err = recordAdjustment(ctx, tx, actor, reservation.ID, grant.CapacityPoolID,
				"ALLOCATED", 0, grant.Quantity); err != nil {
				return err
			}
			allocations = append(allocations, grant)
		}

		revision, err := bumpCalendarRevision(ctx, tx, actor.VendorID, branchID, capability)
		if err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "production.reservation.created", eventType: "production.reservation.created",
			aggregateType: "capacity_reservation", aggregateID: reservation.ID, branchID: branchID,
			version: reservation.Version, after: ReservationViewOf(reservation, allocations, now),
			payload: map[string]any{"reservation": ReservationViewOf(reservation, allocations, now),
				"calendar_revision": revision},
		})
	})
	if err != nil {
		return Reservation{}, nil, err
	}
	return reservation, allocations, nil
}

// ReserveInTx is the transaction-composable form used by Vendor-owned
// workflows such as custom-cake quote acceptance. The caller owns the
// transaction; capacity locks, the reservation rows, audit, outbox and the
// caller's snapshot therefore commit or roll back as one unit.
func (s *Service) ReserveInTx(ctx context.Context, tx pgx.Tx, actor Actor, scope Scope, in ReserveInput) (Reservation, []AllocationView, error) {
	if err := s.requireWrites(); err != nil {
		return Reservation{}, nil, err
	}
	branchID := strings.TrimSpace(in.BranchID)
	capability := strings.ToUpper(strings.TrimSpace(in.Capability))
	if !inVocabulary(Capabilities, capability) {
		return Reservation{}, nil, validation(apperrors.FieldViolation{Field: "capability", Message: "Unsupported production capability."})
	}
	if !scope.Covers(branchID) {
		return Reservation{}, nil, errForbidden()
	}
	if err := validateWindow(in.WindowStart, in.WindowEnd); err != nil {
		return Reservation{}, nil, err
	}
	requested, err := normalizeAllocations(in.Allocations)
	if err != nil {
		return Reservation{}, nil, err
	}
	sourceType := strings.ToUpper(strings.TrimSpace(in.SourceType))
	if sourceType == "" {
		sourceType = "MANUAL"
	}
	if !inVocabulary(SourceTypes, sourceType) {
		return Reservation{}, nil, validation(apperrors.FieldViolation{Field: "source_type", Message: "Unsupported reservation source."})
	}
	priority := strings.ToUpper(strings.TrimSpace(in.Priority))
	if priority == "" {
		priority = "NORMAL"
	}
	if !inVocabulary(Priorities, priority) {
		return Reservation{}, nil, validation(apperrors.FieldViolation{Field: "priority", Message: "Priority must be NORMAL, HIGH or URGENT."})
	}
	if len(strings.TrimSpace(in.Note)) > 280 {
		return Reservation{}, nil, validation(apperrors.FieldViolation{Field: "note", Message: "Note is too long."})
	}
	now := s.now()
	status := ReservationConfirmed
	var holdExpires *time.Time
	if in.HoldFor > 0 {
		status = ReservationHeld
		expiry := now.Add(in.HoldFor)
		holdExpires = &expiry
	}
	granted, err := s.allocateUnderLock(ctx, tx, actor, allocationContext{branchID: branchID, capability: capability, windowStart: in.WindowStart, windowEnd: in.WindowEnd, requested: requested, at: now, sourceType: sourceType, sourceRef: strings.TrimSpace(in.SourceRef)})
	if err != nil {
		return Reservation{}, nil, err
	}
	reservationID := newID("cres_")
	reservation, err := scanReservation(tx.QueryRow(ctx, `INSERT INTO capacity_reservations(id,business_id,branch_id,capability,window_start,window_end,source_type,source_ref,status,hold_expires_at,priority,note,command_ref) VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),$9,$10,$11,$12,NULLIF($13,'')) RETURNING `+reservationCols, reservationID, actor.VendorID, branchID, capability, in.WindowStart, in.WindowEnd, sourceType, strings.TrimSpace(in.SourceRef), status, holdExpires, priority, strings.TrimSpace(in.Note), actor.CommandRef))
	if err != nil {
		return Reservation{}, nil, mapDB(err, "Reservation", CodeReservationNotFound)
	}
	allocations := make([]AllocationView, 0, len(granted))
	for _, grant := range granted {
		if _, err = tx.Exec(ctx, `INSERT INTO capacity_reservation_allocations(id,reservation_id,business_id,capacity_pool_id,production_line_id,quantity,effective_start,effective_end) VALUES($1,$2,$3,$4,NULLIF($5,''),$6,$7,$8)`, newID("cal_"), reservation.ID, actor.VendorID, grant.CapacityPoolID, grant.ProductionLineID, grant.Quantity, grant.EffectiveStart, grant.EffectiveEnd); err != nil {
			return Reservation{}, nil, mapDB(err, "Reservation allocation", CodeInvalidAllocation)
		}
		if err = recordAdjustment(ctx, tx, actor, reservation.ID, grant.CapacityPoolID, "ALLOCATED", 0, grant.Quantity); err != nil {
			return Reservation{}, nil, err
		}
		allocations = append(allocations, grant)
	}
	revision, err := bumpCalendarRevision(ctx, tx, actor.VendorID, branchID, capability)
	if err != nil {
		return Reservation{}, nil, err
	}
	if err = s.record(ctx, tx, actor, evidence{action: "production.reservation.created", eventType: "production.reservation.created", aggregateType: "capacity_reservation", aggregateID: reservation.ID, branchID: branchID, version: reservation.Version, after: ReservationViewOf(reservation, allocations, now), payload: map[string]any{"reservation": ReservationViewOf(reservation, allocations, now), "calendar_revision": revision}}); err != nil {
		return Reservation{}, nil, err
	}
	return reservation, allocations, nil
}

// allocationContext carries what allocateUnderLock needs to decide.
type allocationContext struct {
	branchID    string
	capability  string
	windowStart time.Time
	windowEnd   time.Time
	requested   []AllocationRequest
	at          time.Time
	sourceType  string
	sourceRef   string
	// excludeReservation lets a reschedule ignore its own current allocations
	// when measuring what is available, so a move within the same pool does not
	// compete with itself.
	excludeReservation string
}

// allocateUnderLock is the whole safety property in one place: lock, measure,
// decide, and let the caller write inside the same transaction.
func (s *Service) allocateUnderLock(ctx context.Context, tx pgx.Tx, actor Actor, in allocationContext) ([]AllocationView, error) {
	poolIDs := make([]string, 0, len(in.requested))
	for _, request := range in.requested {
		poolIDs = append(poolIDs, request.CapacityPoolID)
	}
	pools, err := lockPools(ctx, tx, actor.VendorID, poolIDs)
	if err != nil {
		return nil, err
	}

	granted := make([]AllocationView, 0, len(in.requested))
	// Iterate in sorted pool order too, so the error a caller sees is
	// deterministic rather than dependent on request ordering.
	sorted := append([]AllocationRequest(nil), in.requested...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].CapacityPoolID < sorted[j].CapacityPoolID })

	for _, request := range sorted {
		pool, found := pools[request.CapacityPoolID]
		if !found {
			return nil, errNotFound("Capacity pool", CodePoolNotFound)
		}
		if pool.status != StatusActive {
			return nil, coded(apperrors.Conflict("Capacity pool "+pool.code+" is not active.", nil),
				CodePoolNotFound)
		}
		if pool.branchID != in.branchID {
			// A pool from another branch is reported as absent rather than
			// refused with detail, so pool ids cannot be probed across branches.
			return nil, errNotFound("Capacity pool", CodePoolNotFound)
		}

		setupBuffer, cleanupBuffer, err := effectiveBuffers(ctx, tx, actor.VendorID, pool, request.ProductionLineID, in.sourceType, in.sourceRef)
		if err != nil {
			return nil, err
		}
		effectiveStart := in.windowStart.Add(-time.Duration(setupBuffer) * time.Minute)
		effectiveEnd := in.windowEnd.Add(time.Duration(cleanupBuffer) * time.Minute)
		closedSlot, closedStatus, err := slotClosureFor(ctx, tx, actor.VendorID, pool.id, request.ProductionLineID, effectiveStart, effectiveEnd)
		if err != nil {
			return nil, err
		}
		if closedSlot != "" {
			failure := apperrors.Conflict("Production slot "+closedSlot+" is "+strings.ToLower(closedStatus)+" for this window.", nil)
			failure.Details = map[string]any{"slot_ref": closedSlot, "slot_status": closedStatus}
			return nil, coded(failure, CodeSlotClosed)
		}

		blackout, err := blackoutFor(ctx, tx, actor.VendorID, in.branchID, pool.capability,
			pool.id, request.ProductionLineID, effectiveStart, effectiveEnd)
		if err != nil {
			return nil, err
		}
		if blackout != "" {
			return nil, coded(apperrors.Conflict(
				"Production is blacked out for "+pool.code+" during this window.", nil), CodeBlackoutConflict)
		}

		itemFamily := request.ItemFamily
		if itemFamily == "" {
			itemFamily = in.sourceType
		}
		maximum, blockingRule, err := poolMaximum(ctx, tx, actor.VendorID, pool, request.ProductionLineID, itemFamily, effectiveStart, effectiveEnd)
		if err != nil {
			return nil, err
		}
		confirmed, held, err := poolUsage(ctx, tx, actor.VendorID, pool.id,
			effectiveStart, effectiveEnd, in.at, in.excludeReservation)
		if err != nil {
			return nil, err
		}
		remaining := maximum - confirmed - held
		if request.Quantity > remaining {
			detail := map[string]any{
				"capacity_pool_ref": pool.id, "code": pool.code, "unit_type": pool.unitType,
				"maximum": maximum, "confirmed": confirmed, "held": held,
				"remaining": remaining, "requested": request.Quantity,
			}
			if blockingRule != "" {
				detail["blocking_rule_ref"] = blockingRule
			}
			failure := apperrors.Conflict(
				"Not enough production capacity in "+pool.code+" for this window.", nil)
			failure.Details = detail
			return nil, coded(failure, CodeCapacityUnavailable)
		}
		granted = append(granted, AllocationView{
			CapacityPoolID: pool.id, ProductionLineID: request.ProductionLineID, Quantity: request.Quantity,
			EffectiveStart: effectiveStart, EffectiveEnd: effectiveEnd,
		})
	}
	return granted, nil
}

// normalizeAllocations validates and de-duplicates a requested allocation set.
func normalizeAllocations(requests []AllocationRequest) ([]AllocationRequest, error) {
	if len(requests) == 0 {
		return nil, coded(apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "allocations", Message: "At least one capacity allocation is required."}}),
			CodeInvalidAllocation)
	}
	if len(requests) > MaxPoolsPerRequest {
		return nil, coded(apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "allocations", Message: "Too many capacity pools were requested."}}),
			CodeInvalidAllocation)
	}
	seen := map[string]bool{}
	out := make([]AllocationRequest, 0, len(requests))
	for _, request := range requests {
		poolID := strings.TrimSpace(request.CapacityPoolID)
		if poolID == "" {
			return nil, coded(apperrors.Validation("Check your details.", []apperrors.FieldViolation{
				{Field: "allocations", Message: "A capacity pool reference is required."}}),
				CodeInvalidAllocation)
		}
		if seen[poolID] {
			// Two entries for one pool would make "how much does this
			// reservation hold" ambiguous, and the unique index would refuse it
			// anyway — better to say why.
			return nil, coded(apperrors.Validation("Check your details.", []apperrors.FieldViolation{
				{Field: "allocations", Message: "The same capacity pool was requested more than once."}}),
				CodeInvalidAllocation)
		}
		seen[poolID] = true
		if request.Quantity <= 0 || request.Quantity > MaxCapacity {
			return nil, coded(apperrors.Validation("Check your details.", []apperrors.FieldViolation{
				{Field: "allocations", Message: "Quantity must be between 1 and " + itoa(MaxCapacity) + "."}}),
				CodeInvalidAllocation)
		}
		out = append(out, AllocationRequest{CapacityPoolID: poolID, ProductionLineID: strings.TrimSpace(request.ProductionLineID), ItemFamily: strings.TrimSpace(request.ItemFamily), Quantity: request.Quantity})
	}
	return out, nil
}

func recordAdjustment(ctx context.Context, tx pgx.Tx, actor Actor, reservationID, poolID, adjustmentType string, previous, next int) error {
	actorType := "vendor_staff"
	if actor.StaffID == "" {
		actorType = "system"
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO capacity_allocation_adjustments
			(id,business_id,reservation_id,capacity_pool_id,adjustment_type,
			 previous_quantity,new_quantity,actor_type,actor_id,correlation_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,''),NULLIF($10,''))`,
		newID("cadj_"), actor.VendorID, reservationID, poolID, adjustmentType,
		previous, next, actorType, actor.StaffID, actor.CorrelationID)
	return err
}

// ---------------------------------------------------------------------------
// Reservation reads
// ---------------------------------------------------------------------------

const reservationCols = `id,business_id,branch_id,capability,window_start,window_end,
	source_type,source_ref,status,hold_expires_at,priority,note,command_ref,
	created_at,updated_at,version`

func scanReservation(row rowScanner) (Reservation, error) {
	var r Reservation
	err := row.Scan(&r.ID, &r.BusinessID, &r.BranchID, &r.Capability, &r.WindowStart, &r.WindowEnd,
		&r.SourceType, &r.SourceRef, &r.Status, &r.HoldExpiresAt, &r.Priority, &r.Note,
		&r.CommandRef, &r.CreatedAt, &r.UpdatedAt, &r.Version)
	return r, err
}

func loadReservation(ctx context.Context, q querier, businessID, reservationID string, lock bool) (Reservation, error) {
	sql := `SELECT ` + reservationCols + ` FROM capacity_reservations WHERE business_id=$1 AND id=$2`
	if lock {
		sql += ` FOR UPDATE`
	}
	reservation, err := scanReservation(q.QueryRow(ctx, sql, businessID, strings.TrimSpace(reservationID)))
	if err != nil {
		return Reservation{}, mapDB(err, "Reservation", CodeReservationNotFound)
	}
	return reservation, nil
}

func loadAllocations(ctx context.Context, q querier, businessID, reservationID string) ([]AllocationView, error) {
	rows, err := q.Query(ctx, `
		SELECT capacity_pool_id,COALESCE(production_line_id,''),quantity,released_quantity,effective_start,effective_end
		FROM capacity_reservation_allocations
		WHERE business_id=$1 AND reservation_id=$2 ORDER BY capacity_pool_id`,
		businessID, reservationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AllocationView{}
	for rows.Next() {
		var view AllocationView
		if err = rows.Scan(&view.CapacityPoolID, &view.ProductionLineID, &view.Quantity, &view.ReleasedQuantity,
			&view.EffectiveStart, &view.EffectiveEnd); err != nil {
			return nil, err
		}
		out = append(out, view)
	}
	return out, rows.Err()
}

// GetReservation reads one reservation inside the caller's reach.
func (s *Service) GetReservation(ctx context.Context, actor Actor, scope Scope, reservationID string) (Reservation, []AllocationView, error) {
	reservation, err := loadReservation(ctx, s.db, actor.VendorID, reservationID, false)
	if err != nil {
		return Reservation{}, nil, err
	}
	if !scope.Covers(reservation.BranchID) {
		return Reservation{}, nil, errNotFound("Reservation", CodeReservationNotFound)
	}
	allocations, err := loadAllocations(ctx, s.db, actor.VendorID, reservation.ID)
	if err != nil {
		return Reservation{}, nil, err
	}
	return reservation, allocations, nil
}

// ---------------------------------------------------------------------------
// Confirm, release, reschedule
// ---------------------------------------------------------------------------

// Confirm promotes a hold to a confirmed reservation.
//
// It rechecks capacity under lock rather than trusting the original hold,
// because a hold that expired before confirmation stopped consuming capacity
// and someone else may legitimately have taken it.
func (s *Service) Confirm(ctx context.Context, actor Actor, scope Scope, reservationID string, expectedVersion int64) (Reservation, []AllocationView, error) {
	if err := s.requireWrites(); err != nil {
		return Reservation{}, nil, err
	}
	var (
		confirmed   Reservation
		allocations []AllocationView
	)
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var err error
		confirmed, allocations, err = s.confirmInTx(ctx, tx, actor, scope, reservationID, expectedVersion)
		return err
	})
	if err != nil {
		return Reservation{}, nil, err
	}
	return confirmed, allocations, nil
}

// ConfirmInTx is the composable promotion used by quote acceptance so the
// held capacity, immutable accepted snapshot and conversion intent share one
// commit boundary.
func (s *Service) ConfirmInTx(ctx context.Context, tx pgx.Tx, actor Actor, scope Scope, reservationID string, expectedVersion int64) (Reservation, []AllocationView, error) {
	if err := s.requireWrites(); err != nil {
		return Reservation{}, nil, err
	}
	return s.confirmInTx(ctx, tx, actor, scope, reservationID, expectedVersion)
}

func (s *Service) confirmInTx(ctx context.Context, tx pgx.Tx, actor Actor, scope Scope, reservationID string, expectedVersion int64) (Reservation, []AllocationView, error) {
	now := s.now()
	current, err := loadReservation(ctx, tx, actor.VendorID, reservationID, true)
	if err != nil {
		return Reservation{}, nil, err
	}
	if !scope.Covers(current.BranchID) {
		return Reservation{}, nil, errNotFound("Reservation", CodeReservationNotFound)
	}
	if current.Version != expectedVersion {
		return Reservation{}, nil, errStaleVersion()
	}
	switch current.Status {
	case ReservationConfirmed:
		return Reservation{}, nil, coded(apperrors.Conflict("The reservation is already confirmed.", nil), CodeInvalidTransition)
	case ReservationHeld:
	default:
		return Reservation{}, nil, coded(apperrors.Conflict("Only a held reservation can be confirmed.", nil), CodeInvalidTransition)
	}

	existing, err := loadAllocations(ctx, tx, actor.VendorID, current.ID)
	if err != nil {
		return Reservation{}, nil, err
	}
	// If the hold lapsed, its capacity was released to everyone else the
	// moment it expired; re-taking it has to win the same race a new
	// request would.
	if current.HoldExpiresAt != nil && !current.HoldExpiresAt.After(now) {
		requested := make([]AllocationRequest, 0, len(existing))
		for _, allocation := range existing {
			requested = append(requested, AllocationRequest{
				CapacityPoolID: allocation.CapacityPoolID, ProductionLineID: allocation.ProductionLineID, Quantity: allocation.Quantity - allocation.ReleasedQuantity})
		}
		if _, err = s.allocateUnderLock(ctx, tx, actor, allocationContext{
			branchID: current.BranchID, capability: current.Capability,
			windowStart: current.WindowStart, windowEnd: current.WindowEnd,
			requested: requested, at: now, excludeReservation: current.ID,
			sourceType: current.SourceType, sourceRef: valueOrEmpty(current.SourceRef),
		}); err != nil {
			return Reservation{}, nil, err
		}
	}

	confirmed, err := scanReservation(tx.QueryRow(ctx, `
			UPDATE capacity_reservations SET status='CONFIRMED', hold_expires_at=NULL,
				updated_at=now(), version=version+1
			WHERE business_id=$1 AND id=$2 RETURNING `+reservationCols,
		actor.VendorID, current.ID))
	if err != nil {
		return Reservation{}, nil, mapDB(err, "Reservation", CodeReservationNotFound)
	}
	revision, err := bumpCalendarRevision(ctx, tx, actor.VendorID, current.BranchID, current.Capability)
	if err != nil {
		return Reservation{}, nil, err
	}
	if err = s.record(ctx, tx, actor, evidence{
		action: "production.reservation.confirmed", eventType: "production.reservation.confirmed",
		aggregateType: "capacity_reservation", aggregateID: confirmed.ID, branchID: confirmed.BranchID,
		version: confirmed.Version,
		before:  ReservationViewOf(current, existing, now), after: ReservationViewOf(confirmed, existing, now),
		payload: map[string]any{"reservation": ReservationViewOf(confirmed, existing, now),
			"calendar_revision": revision},
	}); err != nil {
		return Reservation{}, nil, err
	}
	return confirmed, existing, nil
}

// CloseReservationInTx performs a full terminal release inside a caller-owned
// transaction. It is used by quote decline/expiry workers so commercial state
// and capacity state cannot diverge.
func (s *Service) CloseReservationInTx(ctx context.Context, tx pgx.Tx, actor Actor, scope Scope, reservationID, targetStatus, reason string) (Reservation, []AllocationView, error) {
	if err := s.requireWrites(); err != nil {
		return Reservation{}, nil, err
	}
	if targetStatus != ReservationReleased && targetStatus != ReservationCancelled && targetStatus != ReservationExpired {
		return Reservation{}, nil, coded(apperrors.Conflict("Invalid reservation close state.", nil), CodeInvalidTransition)
	}
	now := s.now()
	current, err := loadReservation(ctx, tx, actor.VendorID, reservationID, true)
	if err != nil {
		return Reservation{}, nil, err
	}
	if !scope.Covers(current.BranchID) {
		return Reservation{}, nil, errNotFound("Reservation", CodeReservationNotFound)
	}
	existing, err := loadAllocations(ctx, tx, actor.VendorID, current.ID)
	if err != nil {
		return Reservation{}, nil, err
	}
	if current.Status != ReservationHeld && current.Status != ReservationConfirmed {
		return current, existing, nil
	}
	for _, allocation := range existing {
		remaining := allocation.Quantity - allocation.ReleasedQuantity
		if remaining > 0 {
			if err = recordAdjustment(ctx, tx, actor, current.ID, allocation.CapacityPoolID, "FULL_RELEASE", remaining, 0); err != nil {
				return Reservation{}, nil, err
			}
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE capacity_reservation_allocations SET released_quantity=quantity,updated_at=now() WHERE business_id=$1 AND reservation_id=$2`, actor.VendorID, current.ID); err != nil {
		return Reservation{}, nil, err
	}
	closed, err := scanReservation(tx.QueryRow(ctx, `UPDATE capacity_reservations SET status=$3,hold_expires_at=NULL,note=CASE WHEN $4='' THEN note ELSE $4 END,updated_at=now(),version=version+1 WHERE business_id=$1 AND id=$2 RETURNING `+reservationCols, actor.VendorID, current.ID, targetStatus, strings.TrimSpace(reason)))
	if err != nil {
		return Reservation{}, nil, err
	}
	closedAllocations, err := loadAllocations(ctx, tx, actor.VendorID, current.ID)
	if err != nil {
		return Reservation{}, nil, err
	}
	revision, err := bumpCalendarRevision(ctx, tx, actor.VendorID, current.BranchID, current.Capability)
	if err != nil {
		return Reservation{}, nil, err
	}
	actionSuffix := strings.ToLower(targetStatus)
	if err = s.record(ctx, tx, actor, evidence{action: "production.reservation." + actionSuffix, eventType: "production.reservation." + actionSuffix, aggregateType: "capacity_reservation", aggregateID: current.ID, branchID: current.BranchID, version: closed.Version, before: ReservationViewOf(current, existing, now), after: ReservationViewOf(closed, closedAllocations, now), payload: map[string]any{"reservation": ReservationViewOf(closed, closedAllocations, now), "calendar_revision": revision}, reason: reason}); err != nil {
		return Reservation{}, nil, err
	}
	return closed, closedAllocations, nil
}

// ExpireHolds is evidence cleanup, not the capacity correctness mechanism:
// poolUsage already ignores elapsed holds by clock. The worker makes lifecycle
// state explicit for operators and downstream history.
func (s *Service) ExpireHolds(ctx context.Context, limit int) (int, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `SELECT id,business_id FROM capacity_reservations WHERE status='HELD' AND hold_expires_at<=now() ORDER BY hold_expires_at FOR UPDATE SKIP LOCKED LIMIT $1`, limit)
	if err != nil {
		return 0, err
	}
	type candidate struct{ id, business string }
	candidates := []candidate{}
	for rows.Next() {
		var c candidate
		if err = rows.Scan(&c.id, &c.business); err != nil {
			rows.Close()
			return 0, err
		}
		candidates = append(candidates, c)
	}
	rows.Close()
	count := 0
	for _, candidate := range candidates {
		tx, beginErr := s.db.Begin(ctx)
		if beginErr != nil {
			return count, beginErr
		}
		_, _, closeErr := s.CloseReservationInTx(ctx, tx, Actor{VendorID: candidate.business}, Scope{Global: true}, candidate.id, ReservationExpired, "hold expired")
		if closeErr == nil {
			closeErr = tx.Commit(ctx)
		}
		_ = tx.Rollback(ctx)
		if closeErr != nil {
			return count, closeErr
		}
		count++
	}
	return count, nil
}

// ReleaseInput describes a full or partial release.
type ReleaseInput struct {
	ExpectedVersion int64 `json:"expected_version"`
	// Partial, when set, releases only this many units from the named pools and
	// leaves the reservation active. Empty means release everything.
	Partial []AllocationRequest `json:"partial"`
	Reason  string              `json:"reason"`
}

// Release frees capacity, wholly or partly.
//
// A partial release is recorded as an adjustment rather than an edit: the
// original grant stays visible, so "we booked 100 and dropped to 80" remains
// legible afterwards instead of looking like it was always 80.
func (s *Service) Release(ctx context.Context, actor Actor, scope Scope, reservationID string, in ReleaseInput) (Reservation, []AllocationView, error) {
	if err := s.requireWrites(); err != nil {
		return Reservation{}, nil, err
	}
	var (
		released    Reservation
		allocations []AllocationView
	)
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		now := s.now()
		current, err := loadReservation(ctx, tx, actor.VendorID, reservationID, true)
		if err != nil {
			return err
		}
		if !scope.Covers(current.BranchID) {
			return errNotFound("Reservation", CodeReservationNotFound)
		}
		if current.Version != in.ExpectedVersion {
			return errStaleVersion()
		}
		if current.Status != ReservationHeld && current.Status != ReservationConfirmed {
			return coded(apperrors.Conflict("Only an active reservation can be released.", nil),
				CodeInvalidTransition)
		}
		existing, err := loadAllocations(ctx, tx, actor.VendorID, current.ID)
		if err != nil {
			return err
		}
		// Lock the pools even to release, so a concurrent reservation measuring
		// this pool either sees the release or does not, never half of it.
		poolIDs := make([]string, 0, len(existing))
		for _, allocation := range existing {
			poolIDs = append(poolIDs, allocation.CapacityPoolID)
		}
		if _, err = lockPools(ctx, tx, actor.VendorID, poolIDs); err != nil {
			return err
		}

		if len(in.Partial) == 0 {
			released, err = scanReservation(tx.QueryRow(ctx, `
				UPDATE capacity_reservations SET status='RELEASED', updated_at=now(), version=version+1
				WHERE business_id=$1 AND id=$2 RETURNING `+reservationCols,
				actor.VendorID, current.ID))
			if err != nil {
				return mapDB(err, "Reservation", CodeReservationNotFound)
			}
			for _, allocation := range existing {
				if err = recordAdjustment(ctx, tx, actor, current.ID, allocation.CapacityPoolID,
					"FULL_RELEASE", allocation.Quantity-allocation.ReleasedQuantity, 0); err != nil {
					return err
				}
			}
		} else {
			byPool := map[string]AllocationView{}
			for _, allocation := range existing {
				byPool[allocation.CapacityPoolID] = allocation
			}
			for _, request := range in.Partial {
				allocation, found := byPool[request.CapacityPoolID]
				if !found {
					return coded(apperrors.Validation("Check your details.", []apperrors.FieldViolation{
						{Field: "partial", Message: "That capacity pool is not part of this reservation."}}),
						CodeInvalidAllocation)
				}
				remaining := allocation.Quantity - allocation.ReleasedQuantity
				if request.Quantity <= 0 || request.Quantity > remaining {
					return coded(apperrors.Validation("Check your details.", []apperrors.FieldViolation{
						{Field: "partial", Message: "Released quantity must be between 1 and the amount still held."}}),
						CodeInvalidAllocation)
				}
				if _, err = tx.Exec(ctx, `
					UPDATE capacity_reservation_allocations
					SET released_quantity = released_quantity + $3, updated_at = now()
					WHERE business_id=$1 AND reservation_id=$2 AND capacity_pool_id=$4`,
					actor.VendorID, current.ID, request.Quantity, request.CapacityPoolID); err != nil {
					return err
				}
				if err = recordAdjustment(ctx, tx, actor, current.ID, request.CapacityPoolID,
					"PARTIAL_RELEASE", remaining, remaining-request.Quantity); err != nil {
					return err
				}
			}
			released, err = scanReservation(tx.QueryRow(ctx, `
				UPDATE capacity_reservations SET updated_at=now(), version=version+1
				WHERE business_id=$1 AND id=$2 RETURNING `+reservationCols,
				actor.VendorID, current.ID))
			if err != nil {
				return mapDB(err, "Reservation", CodeReservationNotFound)
			}
		}

		allocations, err = loadAllocations(ctx, tx, actor.VendorID, current.ID)
		if err != nil {
			return err
		}
		revision, err := bumpCalendarRevision(ctx, tx, actor.VendorID, current.BranchID, current.Capability)
		if err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "production.reservation.released", eventType: "production.reservation.released",
			aggregateType: "capacity_reservation", aggregateID: released.ID, branchID: released.BranchID,
			version: released.Version,
			before:  ReservationViewOf(current, existing, now), after: ReservationViewOf(released, allocations, now),
			payload: map[string]any{"reservation": ReservationViewOf(released, allocations, now),
				"partial": len(in.Partial) > 0, "calendar_revision": revision},
			reason: strings.TrimSpace(in.Reason),
		})
	})
	if err != nil {
		return Reservation{}, nil, err
	}
	return released, allocations, nil
}

// RescheduleInput moves a reservation to a new window.
type RescheduleInput struct {
	WindowStart     time.Time
	WindowEnd       time.Time
	ExpectedVersion int64
}

// Reschedule moves a reservation, taking the target before giving up the source.
//
// The order matters and is the whole point: the target window is checked and
// claimed inside the same transaction that still holds the original allocation.
// Releasing first and then discovering the target is full would leave a
// reservation with nothing, which is exactly the failure this ordering prevents.
func (s *Service) Reschedule(ctx context.Context, actor Actor, scope Scope, reservationID string, in RescheduleInput) (Reservation, []AllocationView, error) {
	var moved Reservation
	var allocations []AllocationView
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		var e error
		moved, allocations, e = s.RescheduleInTx(ctx, tx, actor, scope, reservationID, in)
		return e
	})
	if err != nil {
		return Reservation{}, nil, err
	}
	return moved, allocations, nil
}

// RescheduleInTx reuses the native availability locks, blackout/expiry checks,
// allocations and calendar revision in the caller's owner transaction.
func (s *Service) RescheduleInTx(ctx context.Context, tx pgx.Tx, actor Actor, scope Scope, reservationID string, in RescheduleInput) (Reservation, []AllocationView, error) {
	if err := s.requireWrites(); err != nil {
		return Reservation{}, nil, err
	}
	if err := validateWindow(in.WindowStart, in.WindowEnd); err != nil {
		return Reservation{}, nil, err
	}
	var (
		moved       Reservation
		allocations []AllocationView
	)
	err := func() error {
		now := s.now()
		current, err := loadReservation(ctx, tx, actor.VendorID, reservationID, true)
		if err != nil {
			return err
		}
		if !scope.Covers(current.BranchID) {
			return errNotFound("Reservation", CodeReservationNotFound)
		}
		if current.Version != in.ExpectedVersion {
			return errStaleVersion()
		}
		if current.Status != ReservationHeld && current.Status != ReservationConfirmed {
			return coded(apperrors.Conflict("Only an active reservation can be rescheduled.", nil),
				CodeInvalidTransition)
		}
		existing, err := loadAllocations(ctx, tx, actor.VendorID, current.ID)
		if err != nil {
			return err
		}
		requested := make([]AllocationRequest, 0, len(existing))
		for _, allocation := range existing {
			requested = append(requested, AllocationRequest{
				CapacityPoolID:   allocation.CapacityPoolID,
				ProductionLineID: allocation.ProductionLineID,
				Quantity:         allocation.Quantity - allocation.ReleasedQuantity})
		}
		// Measure the target window excluding this reservation's own current
		// allocations, so moving within a pool does not contend with itself.
		granted, err := s.allocateUnderLock(ctx, tx, actor, allocationContext{
			branchID: current.BranchID, capability: current.Capability,
			windowStart: in.WindowStart, windowEnd: in.WindowEnd,
			requested: requested, at: now, excludeReservation: current.ID,
			sourceType: current.SourceType, sourceRef: valueOrEmpty(current.SourceRef),
		})
		if err != nil {
			// Surface the move-specific code so a client can tell "the target is
			// full" apart from "you asked for something impossible".
			var appErr *apperrors.Error
			if errors.As(err, &appErr) && appErr.Details != nil &&
				appErr.Details["code"] == CodeCapacityUnavailable {
				appErr.Details["code"] = CodeRescheduleConflict
			}
			return err
		}

		for _, grant := range granted {
			if _, err = tx.Exec(ctx, `
				UPDATE capacity_reservation_allocations
				SET effective_start=$3, effective_end=$4, updated_at=now()
				WHERE business_id=$1 AND reservation_id=$2 AND capacity_pool_id=$5`,
				actor.VendorID, current.ID, grant.EffectiveStart, grant.EffectiveEnd,
				grant.CapacityPoolID); err != nil {
				return err
			}
			if err = recordAdjustment(ctx, tx, actor, current.ID, grant.CapacityPoolID,
				"RESCHEDULED", grant.Quantity, grant.Quantity); err != nil {
				return err
			}
		}
		moved, err = scanReservation(tx.QueryRow(ctx, `
			UPDATE capacity_reservations SET window_start=$3, window_end=$4,
				updated_at=now(), version=version+1
			WHERE business_id=$1 AND id=$2 RETURNING `+reservationCols,
			actor.VendorID, current.ID, in.WindowStart, in.WindowEnd))
		if err != nil {
			return mapDB(err, "Reservation", CodeReservationNotFound)
		}
		allocations, err = loadAllocations(ctx, tx, actor.VendorID, current.ID)
		if err != nil {
			return err
		}
		revision, err := bumpCalendarRevision(ctx, tx, actor.VendorID, current.BranchID, current.Capability)
		if err != nil {
			return err
		}
		recordErr := s.record(ctx, tx, actor, evidence{
			action: "production.reservation.rescheduled", eventType: "production.reservation.rescheduled",
			aggregateType: "capacity_reservation", aggregateID: moved.ID, branchID: moved.BranchID,
			version: moved.Version,
			before:  ReservationViewOf(current, existing, now), after: ReservationViewOf(moved, allocations, now),
			payload: map[string]any{"reservation": ReservationViewOf(moved, allocations, now),
				"calendar_revision": revision},
		})
		if recordErr != nil {
			return recordErr
		}
		return nil

	}()
	if err != nil {
		return Reservation{}, nil, err
	}
	return moved, allocations, nil
}

// ---------------------------------------------------------------------------
// Preview and utilization
// ---------------------------------------------------------------------------

// PreviewCapacity answers "could this be reserved" without taking anything.
//
// It is explicitly non-binding. The reserving transaction rechecks every pool
// under lock, because between a preview and a request somebody else may have
// taken the capacity — and a preview that promised otherwise would be worse
// than no preview at all.
func (s *Service) PreviewCapacity(ctx context.Context, actor Actor, scope Scope, in ReserveInput) (CapacityPreview, error) {
	branchID := strings.TrimSpace(in.BranchID)
	if !scope.Covers(branchID) {
		return CapacityPreview{}, errForbidden()
	}
	if err := validateWindow(in.WindowStart, in.WindowEnd); err != nil {
		return CapacityPreview{}, err
	}
	requested, err := normalizeAllocations(in.Allocations)
	if err != nil {
		return CapacityPreview{}, err
	}
	capability := strings.ToUpper(strings.TrimSpace(in.Capability))
	now := s.now()

	preview := CapacityPreview{WindowStart: in.WindowStart, WindowEnd: in.WindowEnd,
		Available: true, Pools: []PoolAvailability{}, EvaluatedAt: now}

	err = s.inTx(ctx, func(tx pgx.Tx) error {
		pools, err := lockPools(ctx, tx, actor.VendorID, poolIDsOf(requested))
		if err != nil {
			return err
		}
		sorted := append([]AllocationRequest(nil), requested...)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].CapacityPoolID < sorted[j].CapacityPoolID })
		for _, request := range sorted {
			pool, found := pools[request.CapacityPoolID]
			if !found || pool.branchID != branchID {
				return errNotFound("Capacity pool", CodePoolNotFound)
			}
			setupBuffer, cleanupBuffer, err := effectiveBuffers(ctx, tx, actor.VendorID, pool, request.ProductionLineID, strings.ToUpper(strings.TrimSpace(in.SourceType)), strings.TrimSpace(in.SourceRef))
			if err != nil {
				return err
			}
			effectiveStart := in.WindowStart.Add(-time.Duration(setupBuffer) * time.Minute)
			effectiveEnd := in.WindowEnd.Add(time.Duration(cleanupBuffer) * time.Minute)
			availability := PoolAvailability{CapacityPoolID: pool.id, Code: pool.code,
				UnitType: pool.unitType, Requested: request.Quantity}
			closedSlot, closedStatus, err := slotClosureFor(ctx, tx, actor.VendorID, pool.id, request.ProductionLineID, effectiveStart, effectiveEnd)
			if err != nil {
				return err
			}

			blackout, err := blackoutFor(ctx, tx, actor.VendorID, branchID, pool.capability,
				pool.id, request.ProductionLineID, effectiveStart, effectiveEnd)
			if err != nil {
				return err
			}
			itemFamily := request.ItemFamily
			if itemFamily == "" {
				itemFamily = strings.ToUpper(strings.TrimSpace(in.SourceType))
			}
			maximum, blockingRule, err := poolMaximum(ctx, tx, actor.VendorID, pool, request.ProductionLineID, itemFamily, effectiveStart, effectiveEnd)
			if err != nil {
				return err
			}
			confirmed, held, err := poolUsage(ctx, tx, actor.VendorID, pool.id,
				effectiveStart, effectiveEnd, now, "")
			if err != nil {
				return err
			}
			availability.Maximum = maximum
			availability.Confirmed = confirmed
			availability.Held = held
			availability.Remaining = maximum - confirmed - held
			availability.BlockingRuleRef = blockingRule
			availability.BlackoutRef = blackout
			if closedSlot != "" {
				availability.BlackoutRef = "slot:" + closedSlot
			}
			availability.Sufficient = blackout == "" && closedSlot == "" && request.Quantity <= availability.Remaining
			if !availability.Sufficient && preview.Available {
				preview.Available = false
				preview.BlockingPool = pool.id
				if closedSlot != "" {
					preview.BlockingReason = CodeSlotClosed + ":" + closedStatus
				} else if blackout != "" {
					preview.BlockingReason = CodeBlackoutConflict
				} else {
					preview.BlockingReason = CodeCapacityUnavailable
				}
			}
			preview.Pools = append(preview.Pools, availability)
		}
		if capability != "" {
			preview.CalendarRevision, err = s.CalendarRevision(ctx, tx, actor.VendorID, branchID, capability)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return CapacityPreview{}, err
	}
	return preview, nil
}

func poolIDsOf(requests []AllocationRequest) []string {
	out := make([]string, 0, len(requests))
	for _, request := range requests {
		out = append(out, request.CapacityPoolID)
	}
	return out
}

// Utilization reports load per pool over a window.
//
// The percentage is basis points computed from integers, so there is no hidden
// rounding and no float in an operational number someone may act on.
func (s *Service) Utilization(ctx context.Context, actor Actor, scope Scope, branchID, capability string, start, end time.Time) ([]PoolUtilization, error) {
	if !scope.Covers(strings.TrimSpace(branchID)) {
		return nil, errForbidden()
	}
	if err := validateWindow(start, end); err != nil {
		return nil, err
	}
	now := s.now()
	out := []PoolUtilization{}
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id FROM production_capacity_pools WHERE business_id=$1 AND branch_id=$2 AND status='ACTIVE' AND ($3='' OR capability=$3) ORDER BY code`, actor.VendorID, strings.TrimSpace(branchID), strings.ToUpper(strings.TrimSpace(capability)))
		if err != nil {
			return err
		}
		ids := []string{}
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		rows.Close()
		if err = rows.Err(); err != nil || len(ids) == 0 {
			return err
		}
		pools, err := lockPools(ctx, tx, actor.VendorID, ids)
		if err != nil {
			return err
		}
		for _, id := range ids {
			pool := pools[id]
			maximum, _, err := poolMaximum(ctx, tx, actor.VendorID, pool, "", "", start, end)
			if err != nil {
				return err
			}
			confirmed, held, err := poolUsage(ctx, tx, actor.VendorID, pool.id, start, end, now, "")
			if err != nil {
				return err
			}
			used := confirmed + held
			basisPoints := 0
			if maximum > 0 {
				basisPoints = used * 10000 / maximum
			}
			out = append(out, PoolUtilization{CapacityPoolID: pool.id, Code: pool.code, UnitType: pool.unitType, Maximum: maximum, Confirmed: confirmed, Held: held, Remaining: maximum - used, UtilizationBasisPoints: basisPoints})
		}
		return nil
	})
	return out, err
}
