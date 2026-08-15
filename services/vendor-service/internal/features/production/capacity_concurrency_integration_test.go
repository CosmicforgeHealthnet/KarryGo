package production

import (
	"context"
	"sync"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"
)

// The capacity gate.
//
// These tests are the reason the engine locks pool rows rather than reading a
// remaining count and deciding in Go. Each one is deterministic by
// construction: goroutines are released together and their OUTCOMES are counted,
// so nothing depends on which one happens to win or on any sleep.

func errorCode(t *testing.T, err error) string {
	t.Helper()
	if err == nil {
		return ""
	}
	appErr := apperrors.From(err)
	if appErr == nil || appErr.Details == nil {
		return ""
	}
	code, _ := appErr.Details["code"].(string)
	return code
}

// race runs fns simultaneously and returns how many returned nil.
//
// The barrier makes every goroutine reach its reservation call before any is
// allowed to proceed, which is what makes this a genuine contention test rather
// than a sequence that merely looks concurrent.
func race(fns ...func() error) (succeeded int, errs []error) {
	var (
		start sync.WaitGroup
		done  sync.WaitGroup
		mutex sync.Mutex
	)
	start.Add(1)
	for _, fn := range fns {
		done.Add(1)
		go func(fn func() error) {
			defer done.Done()
			start.Wait()
			err := fn()
			mutex.Lock()
			defer mutex.Unlock()
			if err == nil {
				succeeded++
			} else {
				errs = append(errs, err)
			}
		}(fn)
	}
	start.Done()
	done.Wait()
	return succeeded, errs
}

// TestCapacityGate_SixtyPlusSixtyAgainstOneHundred is the headline requirement:
// two simultaneous 60-unit requests against a capacity of 100 cannot both win.
func TestCapacityGate_SixtyPlusSixtyAgainstOneHundred(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil)
	poolID := env.seedPool(t, businessA, branchA1, "CLOUD_KITCHEN", "HOT_LINE_1", 100)

	succeeded, errs := race(
		func() error {
			_, err := env.reserve(businessA, staffA, branchA1, "CLOUD_KITCHEN", alloc(poolID, 60), 0)
			return err
		},
		func() error {
			_, err := env.reserve(businessA, staffA, branchA1, "CLOUD_KITCHEN", alloc(poolID, 60), 0)
			return err
		},
	)
	if succeeded != 1 {
		t.Fatalf("expected exactly one reservation to succeed, got %d", succeeded)
	}
	if len(errs) != 1 || errorCode(t, errs[0]) != CodeCapacityUnavailable {
		t.Fatalf("expected one capacity refusal, got %v", errs)
	}
	assertPoolConsumption(t, env, poolID, 60)
}

// TestCapacityGate_FortyPlusSixtyBothFit proves the lock does not over-refuse:
// two requests that genuinely fit must both succeed.
func TestCapacityGate_FortyPlusSixtyBothFit(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil)
	poolID := env.seedPool(t, businessA, branchA1, "CLOUD_KITCHEN", "HOT_LINE_1", 100)

	succeeded, errs := race(
		func() error {
			_, err := env.reserve(businessA, staffA, branchA1, "CLOUD_KITCHEN", alloc(poolID, 40), 0)
			return err
		},
		func() error {
			_, err := env.reserve(businessA, staffA, branchA1, "CLOUD_KITCHEN", alloc(poolID, 60), 0)
			return err
		},
	)
	if succeeded != 2 {
		t.Fatalf("expected both reservations to succeed, got %d (%v)", succeeded, errs)
	}
	assertPoolConsumption(t, env, poolID, 100)
}

// TestCapacityGate_ExactFillThenOneMore proves the boundary is exact.
func TestCapacityGate_ExactFillThenOneMore(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil)
	poolID := env.seedPool(t, businessA, branchA1, "CLOUD_KITCHEN", "HOT_LINE_1", 100)

	if _, err := env.reserve(businessA, staffA, branchA1, "CLOUD_KITCHEN", alloc(poolID, 100), 0); err != nil {
		t.Fatalf("filling the pool exactly must succeed: %v", err)
	}
	_, err := env.reserve(businessA, staffA, branchA1, "CLOUD_KITCHEN", alloc(poolID, 1), 0)
	if errorCode(t, err) != CodeCapacityUnavailable {
		t.Fatalf("expected the 101st unit to be refused, got %v", err)
	}
	assertPoolConsumption(t, env, poolID, 100)
}

// TestCapacityGate_IndependentPoolsDoNotContend proves the lock is per pool.
// A shared lock would make two unrelated kitchens serialise on each other.
func TestCapacityGate_IndependentPoolsDoNotContend(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil)
	hot := env.seedPool(t, businessA, branchA1, "CLOUD_KITCHEN", "HOT_LINE_1", 60)
	cold := env.seedPool(t, businessA, branchA1, "CLOUD_KITCHEN", "COLD_PREP", 60)
	pastry := env.seedPool(t, businessA, branchA1, "PASTRY_BAKERY", "STANDARD_BAKERY", 60)

	succeeded, errs := race(
		func() error {
			_, err := env.reserve(businessA, staffA, branchA1, "CLOUD_KITCHEN", alloc(hot, 60), 0)
			return err
		},
		func() error {
			_, err := env.reserve(businessA, staffA, branchA1, "CLOUD_KITCHEN", alloc(cold, 60), 0)
			return err
		},
		func() error {
			_, err := env.reserve(businessA, staffA, branchA1, "PASTRY_BAKERY", alloc(pastry, 60), 0)
			return err
		},
	)
	if succeeded != 3 {
		t.Fatalf("independent pools must not contend: %d succeeded (%v)", succeeded, errs)
	}
}

// TestCapacityGate_MultiPoolIsAtomic proves a custom cake that needs an oven, a
// decorator and a chiller is not half-reservable: one exhausted pool rolls the
// whole reservation back and leaves no allocation behind.
func TestCapacityGate_MultiPoolIsAtomic(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil)
	cakes := env.seedPool(t, businessA, branchA1, "PASTRY_BAKERY", "CUSTOM_CAKES", 10)
	decoration := env.seedPool(t, businessA, branchA1, "PASTRY_BAKERY", "DECORATION", 10)
	chilling := env.seedPool(t, businessA, branchA1, "PASTRY_BAKERY", "CHILLING", 1)

	start, end := testWindow()
	// Fill the chiller so exactly one leg of the three cannot be satisfied.
	if _, err := env.reserveAt(businessA, staffA, branchA1, "PASTRY_BAKERY", start, end,
		alloc(chilling, 1), 0); err != nil {
		t.Fatal(err)
	}

	_, _, err := env.service.Reserve(context.Background(), actorFor(businessA, staffA), globalScope(),
		ReserveInput{BranchID: branchA1, Capability: "PASTRY_BAKERY",
			WindowStart: start, WindowEnd: end, SourceType: "CUSTOM_CAKE_QUOTE",
			Allocations: []AllocationRequest{
				{CapacityPoolID: cakes, Quantity: 2},
				{CapacityPoolID: decoration, Quantity: 1},
				{CapacityPoolID: chilling, Quantity: 1},
			}})
	if errorCode(t, err) != CodeCapacityUnavailable {
		t.Fatalf("expected the exhausted pool to refuse the whole reservation, got %v", err)
	}
	// The two pools that could have supplied their share must be untouched.
	assertPoolConsumption(t, env, cakes, 0)
	assertPoolConsumption(t, env, decoration, 0)
	assertPoolConsumption(t, env, chilling, 1)

	var orphans int
	if err := env.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM capacity_reservations WHERE business_id=$1 AND source_type='CUSTOM_CAKE_QUOTE'`,
		businessA).Scan(&orphans); err != nil {
		t.Fatal(err)
	}
	if orphans != 0 {
		t.Fatalf("a failed multi-pool reservation left %d reservation rows behind", orphans)
	}
}

// TestCapacityGate_ReversedAllocationOrderDoesNotDeadlock runs two multi-pool
// reservations that request the same pools in opposite orders.
//
// Without the ORDER BY inside the locking statement this is the classic
// lock-order deadlock; with it, both callers take the same locks in the same
// order and the pair simply serialises.
func TestCapacityGate_ReversedAllocationOrderDoesNotDeadlock(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil)
	first := env.seedPool(t, businessA, branchA1, "PASTRY_BAKERY", "AAA_OVEN", 100)
	second := env.seedPool(t, businessA, branchA1, "PASTRY_BAKERY", "ZZZ_PACKING", 100)
	start, end := testWindow()

	forward := func() error {
		_, _, err := env.service.Reserve(context.Background(), actorFor(businessA, staffA), globalScope(),
			ReserveInput{BranchID: branchA1, Capability: "PASTRY_BAKERY", WindowStart: start, WindowEnd: end,
				Allocations: []AllocationRequest{{CapacityPoolID: first, Quantity: 10}, {CapacityPoolID: second, Quantity: 10}}})
		return err
	}
	reverse := func() error {
		_, _, err := env.service.Reserve(context.Background(), actorFor(businessA, staffA), globalScope(),
			ReserveInput{BranchID: branchA1, Capability: "PASTRY_BAKERY", WindowStart: start, WindowEnd: end,
				Allocations: []AllocationRequest{{CapacityPoolID: second, Quantity: 10}, {CapacityPoolID: first, Quantity: 10}}})
		return err
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		succeeded, errs := race(forward, reverse, forward, reverse)
		if succeeded != 4 {
			t.Errorf("expected all four reservations to succeed, got %d (%v)", succeeded, errs)
		}
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("reversed allocation ordering deadlocked")
	}
	// Four reservations of ten units each landed on both pools, in whichever
	// order the lock granted them.
	assertPoolConsumption(t, env, first, 40)
	assertPoolConsumption(t, env, second, 40)
}

// TestCapacityGate_ExpiredHoldStopsConsuming proves correctness does not depend
// on a cleanup worker: an expired hold frees its capacity the moment it lapses.
func TestCapacityGate_ExpiredHoldStopsConsuming(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil)
	poolID := env.seedPool(t, businessA, branchA1, "PASTRY_BAKERY", "CUSTOM_CAKES", 10)

	held, err := env.reserve(businessA, staffA, branchA1, "PASTRY_BAKERY", alloc(poolID, 10), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if held.Status != ReservationHeld {
		t.Fatalf("expected a HELD reservation, got %s", held.Status)
	}
	// A live hold consumes.
	if _, err = env.reserve(businessA, staffA, branchA1, "PASTRY_BAKERY", alloc(poolID, 1), 0); errorCode(t, err) != CodeCapacityUnavailable {
		t.Fatalf("a live hold must consume capacity, got %v", err)
	}

	// Expire it in the past WITHOUT running any worker and without changing its
	// status: the row still says HELD.
	if _, err = env.pool.Exec(context.Background(),
		`UPDATE capacity_reservations SET hold_expires_at = now() - interval '1 minute' WHERE id=$1`,
		held.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = env.reserve(businessA, staffA, branchA1, "PASTRY_BAKERY", alloc(poolID, 10), 0); err != nil {
		t.Fatalf("an expired hold must stop consuming capacity even before cleanup: %v", err)
	}

	var status string
	if err = env.pool.QueryRow(context.Background(),
		`SELECT status FROM capacity_reservations WHERE id=$1`, held.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != ReservationHeld {
		t.Fatalf("the test must not depend on a worker having rewritten the status, got %s", status)
	}
}

// TestCapacityGate_ReleaseVersusCompetingReservation proves a release and a
// competing request cannot both see the same units.
func TestCapacityGate_ReleaseVersusCompetingReservation(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil)
	poolID := env.seedPool(t, businessA, branchA1, "CLOUD_KITCHEN", "HOT_LINE_1", 100)

	existing, err := env.reserve(businessA, staffA, branchA1, "CLOUD_KITCHEN", alloc(poolID, 100), 0)
	if err != nil {
		t.Fatal(err)
	}

	var releaseErr error
	succeeded, _ := race(
		func() error {
			_, _, releaseErr = env.service.Release(context.Background(), actorFor(businessA, staffA), globalScope(),
				existing.ID, ReleaseInput{ExpectedVersion: existing.Version})
			return releaseErr
		},
		func() error {
			_, err := env.reserve(businessA, staffA, branchA1, "CLOUD_KITCHEN", alloc(poolID, 100), 0)
			return err
		},
	)
	// The release always succeeds; the competing reservation succeeds only if it
	// ran after it. Either interleaving is correct — what must never happen is
	// the pool ending up over its limit.
	if succeeded < 1 {
		t.Fatal("the release must succeed")
	}
	assertPoolWithinCapacity(t, env, poolID, 100)
}

// TestCapacityGate_BlackoutVersusReservation proves a committed blackout closes
// the window for new reservations without disturbing existing ones.
func TestCapacityGate_BlackoutVersusReservation(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil)
	poolID := env.seedPool(t, businessA, branchA1, "CLOUD_KITCHEN", "HOT_LINE_1", 100)
	start, end := testWindow()

	before, err := env.reserve(businessA, staffA, branchA1, "CLOUD_KITCHEN", alloc(poolID, 10), 0)
	if err != nil {
		t.Fatal(err)
	}

	if _, err = env.pool.Exec(context.Background(), `
		INSERT INTO production_blackouts
			(id,business_id,branch_id,capability,capacity_pool_id,start_at,end_at,reason_code,status)
		VALUES ('blk_1',$1,$2,'CLOUD_KITCHEN',$3,$4,$5,'MAINTENANCE','ACTIVE')`,
		businessA, branchA1, poolID, start, end); err != nil {
		t.Fatal(err)
	}

	if _, err = env.reserve(businessA, staffA, branchA1, "CLOUD_KITCHEN", alloc(poolID, 1), 0); errorCode(t, err) != CodeBlackoutConflict {
		t.Fatalf("expected a blackout conflict after the blackout commits, got %v", err)
	}
	// The pre-existing reservation is still there and still accounted for.
	reservation, _, err := env.service.GetReservation(context.Background(), actorFor(businessA, staffA),
		globalScope(), before.ID)
	if err != nil || reservation.Status != ReservationConfirmed {
		t.Fatalf("a blackout must not destroy existing reservations: %v %+v", err, reservation)
	}
	assertPoolConsumption(t, env, poolID, 10)
}

// TestCapacityGate_RescheduleVersusCompetingReservation proves a move never
// gives up its source window before it has taken the target.
func TestCapacityGate_RescheduleVersusCompetingReservation(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil)
	poolID := env.seedPool(t, businessA, branchA1, "CLOUD_KITCHEN", "HOT_LINE_1", 10)
	start, end := testWindow()
	targetStart, targetEnd := start.Add(24*time.Hour), end.Add(24*time.Hour)

	moving, err := env.reserveAt(businessA, staffA, branchA1, "CLOUD_KITCHEN", start, end, alloc(poolID, 10), 0)
	if err != nil {
		t.Fatal(err)
	}
	// Fill the target window so the move cannot land.
	if _, err = env.reserveAt(businessA, staffA, branchA1, "CLOUD_KITCHEN",
		targetStart, targetEnd, alloc(poolID, 10), 0); err != nil {
		t.Fatal(err)
	}

	_, _, err = env.service.Reschedule(context.Background(), actorFor(businessA, staffA), globalScope(),
		moving.ID, RescheduleInput{WindowStart: targetStart, WindowEnd: targetEnd,
			ExpectedVersion: moving.Version})
	if errorCode(t, err) != CodeRescheduleConflict {
		t.Fatalf("expected a reschedule conflict, got %v", err)
	}
	// The original reservation still holds its original window: a failed move
	// must not have released anything.
	reservation, allocations, err := env.service.GetReservation(context.Background(),
		actorFor(businessA, staffA), globalScope(), moving.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reservation.WindowStart.Equal(start) {
		t.Fatalf("a failed reschedule moved the window anyway: %s", reservation.WindowStart)
	}
	if len(allocations) != 1 || allocations[0].Quantity != 10 {
		t.Fatalf("a failed reschedule disturbed the allocation: %+v", allocations)
	}
}

// TestCapacityGate_SuccessfulRescheduleMovesTheWindow is the positive case.
func TestCapacityGate_SuccessfulRescheduleMovesTheWindow(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil)
	poolID := env.seedPool(t, businessA, branchA1, "CLOUD_KITCHEN", "HOT_LINE_1", 10)
	start, end := testWindow()
	targetStart, targetEnd := start.Add(48*time.Hour), end.Add(48*time.Hour)

	moving, err := env.reserveAt(businessA, staffA, branchA1, "CLOUD_KITCHEN", start, end, alloc(poolID, 10), 0)
	if err != nil {
		t.Fatal(err)
	}
	moved, allocations, err := env.service.Reschedule(context.Background(), actorFor(businessA, staffA),
		globalScope(), moving.ID, RescheduleInput{WindowStart: targetStart, WindowEnd: targetEnd,
			ExpectedVersion: moving.Version})
	if err != nil {
		t.Fatalf("reschedule: %v", err)
	}
	if !moved.WindowStart.Equal(targetStart) || len(allocations) != 1 {
		t.Fatalf("unexpected reschedule result: %+v %+v", moved, allocations)
	}
	// The source window is free again, so a new reservation can take it.
	if _, err = env.reserveAt(businessA, staffA, branchA1, "CLOUD_KITCHEN", start, end,
		alloc(poolID, 10), 0); err != nil {
		t.Fatalf("the vacated window must be reusable: %v", err)
	}
}

// TestCapacityGate_PartialReleaseFreesExactlyWhatItSays.
func TestCapacityGate_PartialReleaseFreesExactlyWhatItSays(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil)
	poolID := env.seedPool(t, businessA, branchA1, "CLOUD_KITCHEN", "HOT_LINE_1", 100)

	reservation, err := env.reserve(businessA, staffA, branchA1, "CLOUD_KITCHEN", alloc(poolID, 100), 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = env.service.Release(context.Background(), actorFor(businessA, staffA), globalScope(),
		reservation.ID, ReleaseInput{ExpectedVersion: reservation.Version,
			Partial: []AllocationRequest{{CapacityPoolID: poolID, Quantity: 20}}}); err != nil {
		t.Fatalf("partial release: %v", err)
	}
	assertPoolConsumption(t, env, poolID, 80)
	if _, err = env.reserve(businessA, staffA, branchA1, "CLOUD_KITCHEN", alloc(poolID, 20), 0); err != nil {
		t.Fatalf("the released units must be reusable: %v", err)
	}
	if _, err = env.reserve(businessA, staffA, branchA1, "CLOUD_KITCHEN", alloc(poolID, 1), 0); errorCode(t, err) != CodeCapacityUnavailable {
		t.Fatalf("a partial release must not free more than it said, got %v", err)
	}

	// The original grant stays visible as evidence rather than being edited away.
	var adjustments int
	if err = env.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM capacity_allocation_adjustments WHERE reservation_id=$1`,
		reservation.ID).Scan(&adjustments); err != nil {
		t.Fatal(err)
	}
	if adjustments < 2 {
		t.Fatalf("expected the allocation and its partial release to be recorded, got %d rows", adjustments)
	}
}

// TestCapacityGate_BuffersExtendTheOccupiedWindow proves cleanup time really
// blocks the next incompatible job rather than being decorative.
func TestCapacityGate_BuffersExtendTheOccupiedWindow(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil)
	poolID := env.seedPoolWithBuffers(t, businessA, branchA1, "PASTRY_BAKERY", "DECORATION", 1, 0, 15)
	start, end := testWindow()

	if _, err := env.reserveAt(businessA, staffA, branchA1, "PASTRY_BAKERY", start, end,
		alloc(poolID, 1), 0); err != nil {
		t.Fatal(err)
	}
	// Starting 10 minutes after the previous job ends is still inside its
	// 15-minute cleanup.
	if _, err := env.reserveAt(businessA, staffA, branchA1, "PASTRY_BAKERY",
		end.Add(10*time.Minute), end.Add(70*time.Minute), alloc(poolID, 1), 0); errorCode(t, err) != CodeCapacityUnavailable {
		t.Fatalf("the cleanup buffer must still occupy the resource, got %v", err)
	}
	// Starting after the buffer is fine.
	if _, err := env.reserveAt(businessA, staffA, branchA1, "PASTRY_BAKERY",
		end.Add(20*time.Minute), end.Add(80*time.Minute), alloc(poolID, 1), 0); err != nil {
		t.Fatalf("a window beyond the cleanup buffer must be reservable: %v", err)
	}
}

// TestCapacityGate_RulesNarrowAndNeverWiden proves a more specific rule cannot
// raise a ceiling above the pool default.
func TestCapacityGate_RulesNarrowAndNeverWiden(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil)
	poolID := env.seedPool(t, businessA, branchA1, "CLOUD_KITCHEN", "HOT_LINE_1", 100)

	// A rule that tries to allow 500 must not widen the pool's 100.
	if _, err := env.pool.Exec(context.Background(), `
		INSERT INTO production_capacity_rules
			(id,business_id,branch_id,capability,capacity_pool_id,max_quantity,specificity,status)
		VALUES ('crule_wide',$1,$2,'CLOUD_KITCHEN',$3,500,90,'ACTIVE')`,
		businessA, branchA1, poolID); err != nil {
		t.Fatal(err)
	}
	if _, err := env.reserve(businessA, staffA, branchA1, "CLOUD_KITCHEN", alloc(poolID, 101), 0); errorCode(t, err) != CodeCapacityUnavailable {
		t.Fatalf("a rule must not widen the pool default, got %v", err)
	}

	// A rule that narrows to 40 binds immediately.
	if _, err := env.pool.Exec(context.Background(), `
		INSERT INTO production_capacity_rules
			(id,business_id,branch_id,capability,capacity_pool_id,max_quantity,specificity,status)
		VALUES ('crule_narrow',$1,$2,'CLOUD_KITCHEN',$3,40,95,'ACTIVE')`,
		businessA, branchA1, poolID); err != nil {
		t.Fatal(err)
	}
	if _, err := env.reserve(businessA, staffA, branchA1, "CLOUD_KITCHEN", alloc(poolID, 41), 0); errorCode(t, err) != CodeCapacityUnavailable {
		t.Fatalf("the narrower rule must bind, got %v", err)
	}
	if _, err := env.reserve(businessA, staffA, branchA1, "CLOUD_KITCHEN", alloc(poolID, 40), 0); err != nil {
		t.Fatalf("exactly the narrowed maximum must fit: %v", err)
	}
}

func TestCapacityRulesOnlyBindMatchingLineAndItemFamily(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil)
	poolID := env.seedPool(t, businessA, branchA1, "PASTRY_BAKERY", "SCOPED_RULES", 100)
	lineID := "pline_scoped_rules"
	if _, err := env.pool.Exec(context.Background(), `INSERT INTO production_lines(id,business_id,branch_id,capability,name,code,production_class,capacity_pool_id) VALUES($1,$2,$3,'PASTRY_BAKERY','Scoped line','SCOPED_LINE','BAKERY',$4)`, lineID, businessA, branchA1, poolID); err != nil {
		t.Fatal(err)
	}
	if _, err := env.pool.Exec(context.Background(), `INSERT INTO production_capacity_rules(id,business_id,branch_id,capability,capacity_pool_id,production_line_id,max_quantity,specificity) VALUES('rule_line_scoped',$1,$2,'PASTRY_BAKERY',$3,$4,10,90),('rule_family_scoped',$1,$2,'PASTRY_BAKERY',$3,NULL,20,80)`, businessA, branchA1, poolID, lineID); err != nil {
		t.Fatal(err)
	}
	if _, err := env.pool.Exec(context.Background(), `UPDATE production_capacity_rules SET item_family='PASTRY_PRODUCT' WHERE id='rule_family_scoped'`); err != nil {
		t.Fatal(err)
	}
	start, end := testWindow()
	preview := func(allocation AllocationRequest, sourceType string) CapacityPreview {
		out, err := env.service.PreviewCapacity(context.Background(), actorFor(businessA, staffA), globalScope(), ReserveInput{BranchID: branchA1, Capability: "PASTRY_BAKERY", WindowStart: start, WindowEnd: end, Allocations: []AllocationRequest{allocation}, SourceType: sourceType})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	if got := preview(AllocationRequest{CapacityPoolID: poolID, Quantity: 50}, "MANUAL").Pools[0].Maximum; got != 100 {
		t.Fatalf("line/family rules narrowed unrelated work to %d; want 100", got)
	}
	if got := preview(AllocationRequest{CapacityPoolID: poolID, ProductionLineID: lineID, Quantity: 10}, "MANUAL").Pools[0].Maximum; got != 10 {
		t.Fatalf("matching line rule maximum = %d; want 10", got)
	}
	if got := preview(AllocationRequest{CapacityPoolID: poolID, Quantity: 20}, "PASTRY_PRODUCT").Pools[0].Maximum; got != 20 {
		t.Fatalf("matching item-family rule maximum = %d; want 20", got)
	}
}

// ---------------------------------------------------------------------------
// Assertions
// ---------------------------------------------------------------------------

// assertPoolConsumption reads consumption straight from the database rather
// than from the service, so the assertion is about committed state.
func assertPoolConsumption(t *testing.T, env *testEnv, poolID string, expected int) {
	t.Helper()
	var used int
	if err := env.pool.QueryRow(context.Background(), `
		SELECT COALESCE(SUM(a.quantity - a.released_quantity),0)
		FROM capacity_reservation_allocations a
		JOIN capacity_reservations r ON r.id = a.reservation_id
		WHERE a.capacity_pool_id=$1 AND r.status IN ('HELD','CONFIRMED')
		  AND (r.status <> 'HELD' OR r.hold_expires_at IS NULL OR r.hold_expires_at > now())`,
		poolID).Scan(&used); err != nil {
		t.Fatal(err)
	}
	if used != expected {
		t.Fatalf("pool %s consumption = %d; want %d", poolID, used, expected)
	}
}

func assertPoolWithinCapacity(t *testing.T, env *testEnv, poolID string, capacity int) {
	t.Helper()
	var used int
	if err := env.pool.QueryRow(context.Background(), `
		SELECT COALESCE(SUM(a.quantity - a.released_quantity),0)
		FROM capacity_reservation_allocations a
		JOIN capacity_reservations r ON r.id = a.reservation_id
		WHERE a.capacity_pool_id=$1 AND r.status IN ('HELD','CONFIRMED')`,
		poolID).Scan(&used); err != nil {
		t.Fatal(err)
	}
	if used > capacity {
		t.Fatalf("pool %s is overbooked: %d used against a capacity of %d", poolID, used, capacity)
	}
}
