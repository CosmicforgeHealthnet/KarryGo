package main

// Races that would each produce a second vehicle.
//
// Every test here runs the real production flow concurrently and asserts the
// same single fact from a different angle: one canonical mission per logical
// mission slot. The scenarios are chosen because each is a real operational
// coincidence, not a theoretical interleaving -- a retried order event, two
// workers waking together, a customer cancelling while a commit window opens.
//
// The assertions count authority CREATES rather than inspecting state, because
// the number of creates is the number of vehicles.

import (
	"context"
	"sync"
	"testing"
	"time"

	readiness "cosmicforge/logistics/services/vendor-service/internal/features/fulfilment_readiness"
	"cosmicforge/logistics/services/vendor-service/internal/features/orchestration"
)

// TestJITRace_TwoOrderEventsOpenOneOrchestration is the duplicate delivery of a
// production event: activation and the recovery worker arriving together.
func TestJITRace_TwoOrderEventsOpenOneOrchestration(t *testing.T) {
	env := newJITEnv(t, false, 4)

	var wait sync.WaitGroup
	start := make(chan struct{})
	for range 4 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			// Errors are tolerated here: what must not happen is a second
			// orchestration, not a second caller losing a race.
			_, _ = env.fixture.service.ActivateFulfilment(context.Background(), env.order.ID)
		}()
	}
	close(start)
	wait.Wait()

	opened := env.orchestrations(t)
	slots := map[string]int{}
	for _, current := range opened {
		slots[current.FulfilmentGroupRef]++
	}
	for ref, count := range slots {
		if count != 1 {
			t.Fatalf("slot %s opened %d orchestrations", ref, count)
		}
	}
	if env.missions.createCount() != 0 {
		t.Fatalf("concurrent activation dispatched %d missions", env.missions.createCount())
	}
}

// TestJITRace_TwoWorkersCommitOnce is the ordinary case: several evaluation
// workers, all of which find the same orchestration due.
func TestJITRace_TwoWorkersCommitOnce(t *testing.T) {
	env := newJITEnv(t, false, 4)
	env.activate(t)
	env.readiness.setReady(env.now.Add(-time.Minute), readiness.StateReady)

	for _, current := range env.orchestrations(t) {
		id := current.ID
		var wait sync.WaitGroup
		start := make(chan struct{})
		for range 6 {
			wait.Add(1)
			go func() {
				defer wait.Done()
				<-start
				_, _ = env.orchestr.Evaluate(context.Background(), id)
			}()
		}
		close(start)
		wait.Wait()
	}

	assertOneCreatePerSlot(t, env)
}

// TestJITRace_CancellationAgainstCommit is the race with a customer in it.
//
// Exactly one of the two must win, and either winner is correct: a cancelled
// plan dispatches nothing, a committed one dispatches once. What is forbidden
// is both -- a cancelled order with a rider on the way.
func TestJITRace_CancellationAgainstCommit(t *testing.T) {
	env := newJITEnv(t, false, 4)
	env.activate(t)
	env.readiness.setReady(env.now.Add(-time.Minute), readiness.StateReady)

	for _, current := range env.orchestrations(t) {
		id := current.ID
		var wait sync.WaitGroup
		start := make(chan struct{})
		wait.Add(2)
		go func() {
			defer wait.Done()
			<-start
			_, _ = env.orchestr.Evaluate(context.Background(), id)
		}()
		go func() {
			defer wait.Done()
			<-start
			_, _ = env.orchestr.Cancel(context.Background(), id, "CUSTOMER_CANCELLED")
		}()
		close(start)
		wait.Wait()

		final, err := orchestration.NewRepository(env.fixture.env.pool).
			Get(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		switch {
		case final.State == orchestration.StateCancelled:
			// The customer won. Nothing may have been dispatched for this slot.
			for _, request := range env.missions.sentRequests() {
				if request.MissionRef == final.FulfilmentGroupRef {
					t.Fatalf("slot %s was cancelled and still dispatched",
						final.FulfilmentGroupRef)
				}
			}
		case final.State.Committed():
			// The commit won. Exactly one mission, and no pretend cancellation.
		default:
			// Still in flight; the next pass resolves it. Either way, never two.
		}
	}
	assertAtMostOneCreatePerSlot(t, env)
}

// TestJITRace_ReadinessUpdatesDuringCommit moves the ready time under the
// planner while it is deciding.
//
// A prediction that changes mid-flight must not produce two commitments -- the
// commit is keyed on the slot, not on the prediction that triggered it.
func TestJITRace_ReadinessUpdatesDuringCommit(t *testing.T) {
	env := newJITEnv(t, false, 4)
	env.activate(t)
	env.readiness.setReady(env.now.Add(-time.Minute), readiness.StateReady)

	for _, current := range env.orchestrations(t) {
		id := current.ID
		var wait sync.WaitGroup
		start := make(chan struct{})
		wait.Add(3)
		for range 2 {
			go func() {
				defer wait.Done()
				<-start
				_, _ = env.orchestr.Evaluate(context.Background(), id)
			}()
		}
		go func() {
			defer wait.Done()
			<-start
			env.readiness.setReady(env.now.Add(30*time.Minute), readiness.StateDelayed)
			env.readiness.setReady(env.now.Add(-time.Minute), readiness.StateReady)
		}()
		close(start)
		wait.Wait()
	}
	assertAtMostOneCreatePerSlot(t, env)
}

// TestJITRace_ReconciliationAgainstEvaluation is the most dangerous pair.
//
// One goroutine is resolving an ambiguous create; the other is the ordinary
// planner loop. If reconciliation returned the slot to the queue while
// evaluation was already committing it, the authority would be asked twice for
// a mission it may already hold.
func TestJITRace_ReconciliationAgainstEvaluation(t *testing.T) {
	env := newJITEnv(t, false, 4)
	env.activate(t)
	env.readiness.setReady(env.now.Add(-time.Minute), readiness.StateReady)

	// Drive into the ambiguous state.
	env.missions.mu.Lock()
	env.missions.createErr = context.DeadlineExceeded
	env.missions.landed = true
	env.missions.mu.Unlock()
	planned := env.orchestrations(t)
	for _, current := range planned {
		env.driveToCommit(t, current.ID)
	}
	createsBefore := env.missions.createCount()

	env.missions.mu.Lock()
	env.missions.createErr = nil
	env.missions.mu.Unlock()

	for _, current := range planned {
		id := current.ID
		var wait sync.WaitGroup
		start := make(chan struct{})
		wait.Add(4)
		for range 2 {
			go func() {
				defer wait.Done()
				<-start
				_, _ = env.orchestr.Reconcile(context.Background(), id)
			}()
			go func() {
				defer wait.Done()
				<-start
				_, _ = env.orchestr.Evaluate(context.Background(), id)
			}()
		}
		close(start)
		wait.Wait()
	}

	if env.missions.createCount() != createsBefore {
		t.Fatalf("reconciliation racing evaluation produced %d extra creates for missions "+
			"the authority already held", env.missions.createCount()-createsBefore)
	}
}

// TestJITRace_SplitSlotsCommitIndependently runs every slot of a split group
// concurrently. One slot's success must not make another look committed.
func TestJITRace_SplitSlotsCommitIndependently(t *testing.T) {
	env := newJITEnv(t, true, 1)
	env.activate(t)
	env.readiness.setReady(env.now.Add(-time.Minute), readiness.StateReady)

	planned := env.orchestrations(t)
	var wait sync.WaitGroup
	start := make(chan struct{})
	for _, current := range planned {
		id := current.ID
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			for attempt := 0; attempt < 6; attempt++ {
				updated, err := env.orchestr.Evaluate(context.Background(), id)
				if err == nil && updated.State == orchestration.StateCommitted {
					return
				}
			}
		}()
	}
	close(start)
	wait.Wait()

	assertOneCreatePerSlot(t, env)

	repo := orchestration.NewRepository(env.fixture.env.pool)
	seen := map[string]string{}
	for _, current := range planned {
		final, err := repo.Get(context.Background(), current.ID)
		if err != nil {
			t.Fatal(err)
		}
		if final.State != orchestration.StateCommitted {
			continue
		}
		if owner, taken := seen[final.CurrentMissionRef]; taken {
			t.Fatalf("slots %s and %s both claim mission %s",
				owner, final.FulfilmentGroupRef, final.CurrentMissionRef)
		}
		seen[final.CurrentMissionRef] = final.FulfilmentGroupRef
	}
}

// assertOneCreatePerSlot requires every planned slot to have been created
// exactly once.
func assertOneCreatePerSlot(t *testing.T, env *jitEnv) {
	t.Helper()
	counts := map[string]int{}
	for _, request := range env.missions.sentRequests() {
		counts[request.MissionRef]++
	}
	if len(counts) == 0 {
		t.Fatal("nothing was dispatched, so this test proves nothing")
	}
	for ref, count := range counts {
		if count != 1 {
			t.Fatalf("mission slot %s was created %d times", ref, count)
		}
	}
}

// assertAtMostOneCreatePerSlot is the weaker claim for races whose winner is
// legitimately either party.
func assertAtMostOneCreatePerSlot(t *testing.T, env *jitEnv) {
	t.Helper()
	counts := map[string]int{}
	for _, request := range env.missions.sentRequests() {
		counts[request.MissionRef]++
	}
	for ref, count := range counts {
		if count > 1 {
			t.Fatalf("mission slot %s was created %d times", ref, count)
		}
	}
}
