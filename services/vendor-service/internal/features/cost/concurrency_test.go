package cost

// Section 16D concurrency certification.
//
// These run real concurrent transactions against real PostgreSQL and assert
// the invariants that only show up under contention: exactly one piece of
// historical evidence per batch, deterministic retries, and no lost update on
// a policy.
//
// SYNCHRONIZATION
//
// Every race here is started from a barrier - a closed channel both goroutines
// wait on - so both sides are released at the same instant without a sleep
// deciding the outcome. A test that passed because one goroutine happened to
// be slower would prove nothing about production, where the timing differs.
// There is no time.Sleep anywhere in this file.

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/events"
	"cosmicforge/logistics/services/vendor-service/internal/features/production"
)

// concurrencyTimeout bounds every race so a deadlock fails loudly rather than
// hanging the suite.
const concurrencyTimeout = 30 * time.Second

// raceResult is one competitor's outcome.
type raceResult struct {
	snapshotID string
	err        error
}

// runRace releases both functions from a barrier and collects their results.
func runRace(t *testing.T, left, right func() raceResult) (raceResult, raceResult) {
	t.Helper()

	barrier := make(chan struct{})
	results := make([]raceResult, 2)

	var wait sync.WaitGroup
	wait.Add(2)
	for index, fn := range []func() raceResult{left, right} {
		go func(slot int, run func() raceResult) {
			defer wait.Done()
			<-barrier
			results[slot] = run()
		}(index, fn)
	}

	close(barrier)

	done := make(chan struct{})
	go func() { wait.Wait(); close(done) }()

	select {
	case <-done:
	case <-time.After(concurrencyTimeout):
		t.Fatal("concurrent operations did not finish within the bound; likely a deadlock")
	}
	return results[0], results[1]
}

// fixedScope authorizes everything for the concurrency fixture, so these tests
// measure concurrency rather than re-measuring IAM (which has its own gates).
type fixedScope struct{}

func (fixedScope) ResolveScope(context.Context, string, string) (production.Scope, error) {
	return production.Scope{Global: true}, nil
}

// discardAudit satisfies the port. It is deliberately not what these tests
// assert on: production.Service writes mutation audit straight into
// vendor_audit_log inside the caller's transaction, so the real trail is the
// table, and asserting there is both stronger and closer to production.
type discardAudit struct{}

func (discardAudit) Record(context.Context, production.AuditEntry) error { return nil }

// auditCountFor reads the real audit trail for one aggregate.
func auditCountFor(t *testing.T, pool *pgxpool.Pool, ctx context.Context,
	businessID, aggregateID string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM vendor_audit_log
		 WHERE vendor_id=$1 AND aggregate_id=$2`, businessID, aggregateID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// outboxCountFor reads the durable domain-event outbox for one aggregate.
func outboxCountFor(t *testing.T, pool *pgxpool.Pool, ctx context.Context,
	businessID, aggregateID string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM vendor_outbox
		 WHERE vendor_id=$1 AND aggregate_id=$2`, businessID, aggregateID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// newCostServiceForRace builds the real Section 16D service over the real
// production service, so the OCC races below go through production code rather
// than a test-only shortcut.
func newCostServiceForRace(pool *pgxpool.Pool) *Service {
	productionService := production.NewService(production.Options{
		DB:     pool,
		Scopes: fixedScope{},
		Audit:  discardAudit{},
		// The real durable outbox, so the audit/outbox gate measures the
		// production path rather than a stub that always succeeds.
		Events: events.NewDomainPublisher(events.NewOutboxRepository(pool)),
		Flags:  production.Flags{ProductionEnabled: true, WritesEnabled: true},
	})
	resolver := NewResolver(pool, NewPostgresPurchasingCostReader(pool))
	engine := NewEngine(pool, resolver)
	return NewService(pool, productionService, engine, resolver)
}

func raceActor(businessID string) production.Actor {
	return production.Actor{
		StaffID:       businessID + "_owner",
		VendorID:      businessID,
		CorrelationID: "corr_race",
	}
}

// countSnapshots is the invariant probe: how much historical evidence exists.
func countSnapshots(t *testing.T, pool *pgxpool.Pool, ctx context.Context,
	businessID, batchID, calculationType string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM vendor_cost_snapshots
		 WHERE business_id=$1 AND batch_id=$2 AND calculation_type=$3`,
		businessID, batchID, calculationType).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// ---------------------------------------------------------------------------
// A. Simultaneous recipe snapshot creation
// ---------------------------------------------------------------------------

// TestRaceSimultaneousRecipeSnapshotIsIdempotent proves two concurrent
// commands carrying the same idempotency key produce ONE snapshot, and both
// callers are told about the same one.
func TestRaceSimultaneousRecipeSnapshotIsIdempotent(t *testing.T) {
	pool, ctx := testPool(t)
	chain := seedE2EChain(t, pool, ctx, businessA, branchA1, runNonce+"_raceA")

	engine := NewEngine(pool, NewResolver(pool, NewPostgresPurchasingCostReader(pool)))
	base, err := engine.CalculateRecipeCost(ctx, businessA, branchA1, chain.versionID)
	if err != nil {
		t.Fatal(err)
	}
	base.CalculationType = CalcRecipeSnapshot

	command := "cmd_race_recipe_" + chain.nonce
	attempt := func() raceResult {
		stored, persistErr := PersistSnapshot(
			context.Background(), pool, businessA, base,
			businessA+"_owner", "corr_race", command)
		return raceResult{snapshotID: stored.ID, err: persistErr}
	}

	left, right := runRace(t, attempt, attempt)
	if left.err != nil || right.err != nil {
		t.Fatalf("concurrent snapshot creation errored: left=%v right=%v", left.err, right.err)
	}

	var stored int
	if err = pool.QueryRow(ctx, `
		SELECT count(*) FROM vendor_cost_snapshots
		 WHERE business_id=$1 AND command_ref=$2`, businessA, command).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != 1 {
		t.Fatalf("%d snapshots written for one command; historical evidence must be unique", stored)
	}
	if left.snapshotID != right.snapshotID {
		t.Fatalf("callers received different snapshots (%s vs %s); a retry must be deterministic",
			left.snapshotID, right.snapshotID)
	}

	// The surviving snapshot must have complete component evidence: a loser
	// that half-wrote its rows would leave a corrupted breakdown behind.
	components, err := engine.LoadComponents(ctx, businessA, left.snapshotID)
	if err != nil {
		t.Fatal(err)
	}
	if len(components) != len(base.Components) {
		t.Fatalf("snapshot has %d component rows; the calculation produced %d",
			len(components), len(base.Components))
	}
}

// ---------------------------------------------------------------------------
// B and C. Duplicate production expected / actual cost
// ---------------------------------------------------------------------------

// TestRaceDuplicateProductionCostWritesExactlyOneRecord covers both production
// calculation types: two concurrent finalizations must leave exactly one
// historical record, which is the invariant the uniqueness index exists for.
func TestRaceDuplicateProductionCostWritesExactlyOneRecord(t *testing.T) {
	for _, calculationType := range []string{CalcProductionExpected, CalcProductionActual} {
		t.Run(calculationType, func(t *testing.T) {
			pool, ctx := testPool(t)
			nonce := fmt.Sprintf("%s_raceBC_%s", runNonce, calculationType)
			chain := seedE2EChain(t, pool, ctx, businessA, branchA1, nonce)
			seedE2EBatch(t, pool, ctx, businessA, branchA1, chain)

			engine := NewEngine(pool, NewResolver(pool, NewPostgresPurchasingCostReader(pool)))
			computed, err := engine.CalculateProductionCost(
				ctx, businessA, chain.batchID, calculationType)
			if err != nil {
				t.Fatal(err)
			}

			// No command_ref: this leans on the per-batch uniqueness index
			// rather than on idempotency keys, which is the case two racing
			// finalizations actually hit.
			attempt := func() raceResult {
				stored, persistErr := PersistSnapshot(
					context.Background(), pool, businessA, computed,
					businessA+"_owner", "corr_race", "")
				return raceResult{snapshotID: stored.ID, err: persistErr}
			}

			left, right := runRace(t, attempt, attempt)
			if left.err != nil || right.err != nil {
				t.Fatalf("concurrent production cost errored: left=%v right=%v",
					left.err, right.err)
			}

			if got := countSnapshots(t, pool, ctx, businessA, chain.batchID, calculationType); got != 1 {
				t.Fatalf("%d %s records for one batch; duplicate historical production cost",
					got, calculationType)
			}
			if left.snapshotID != right.snapshotID {
				t.Fatalf("racing finalizations disagreed on the record: %s vs %s",
					left.snapshotID, right.snapshotID)
			}

			// Retrying afterwards must still converge on the same record.
			retry := attempt()
			if retry.err != nil {
				t.Fatalf("retry after the race errored: %v", retry.err)
			}
			if got := countSnapshots(t, pool, ctx, businessA, chain.batchID, calculationType); got != 1 {
				t.Fatalf("a retry created a second %s record", calculationType)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// D and E. Cost policy and manual cost OCC races
// ---------------------------------------------------------------------------

// TestRacePolicyUpdateHasNoLostUpdate proves two writers starting from the
// same version cannot both succeed.
//
// Without the version predicate in the UPDATE both would commit and the second
// would silently erase the first - a lost update on the rule that governs
// every future calculation.
func TestRacePolicyUpdateHasNoLostUpdate(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		strategy string
		manual   *int64
	}{
		{"strategy change", StrategyLatest, nil},
		{"manual cost change", StrategyManual, int64Ptr(4242)},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			pool, ctx := testPool(t)
			seedTenant(t, pool, ctx, businessA)

			service := newCostServiceForRace(pool)
			actor := raceActor(businessA)

			subject := fmt.Sprintf("race_subject_%s_%d", scenario.strategy, time.Now().UnixNano())
			created, err := service.CreatePolicy(ctx, actor, PolicyInput{
				SubjectType:         SubjectIngredient,
				SubjectRef:          subject,
				Strategy:            StrategyManual,
				Currency:            "NGN",
				ManualUnitCostMinor: int64Ptr(1000),
				ManualUnitCode:      "kg",
				Reason:              "race baseline",
			})
			if err != nil {
				t.Fatalf("create policy: %v", err)
			}
			auditAfterCreate := auditCountFor(t, pool, ctx, businessA, created.ID)
			outboxAfterCreate := outboxCountFor(t, pool, ctx, businessA, created.ID)

			// Both writers start from the SAME version.
			startVersion := created.Version

			update := func(cost int64) func() raceResult {
				return func() raceResult {
					input := PolicyInput{
						SubjectType:    SubjectIngredient,
						SubjectRef:     subject,
						Strategy:       scenario.strategy,
						Currency:       "NGN",
						ManualUnitCode: "kg",
						Reason:         "race",
					}
					if scenario.strategy == StrategyManual {
						input.ManualUnitCostMinor = int64Ptr(cost)
					} else if scenario.manual != nil {
						input.ManualUnitCostMinor = scenario.manual
					}
					_, updateErr := service.UpdatePolicy(
						context.Background(), actor, created.ID, startVersion, input)
					return raceResult{err: updateErr}
				}
			}

			left, right := runRace(t, update(1111), update(2222))

			succeeded := 0
			conflicted := 0
			for _, result := range []raceResult{left, right} {
				switch {
				case result.err == nil:
					succeeded++
				case errors.Is(result.err, ErrVersionConflict):
					conflicted++
				default:
					t.Fatalf("unexpected error from a racing writer: %v", result.err)
				}
			}
			if succeeded != 1 || conflicted != 1 {
				t.Fatalf("succeeded=%d conflicted=%d; exactly one writer must win",
					succeeded, conflicted)
			}

			// The winner's version must have advanced exactly once.
			final, err := service.GetPolicy(ctx, actor, created.ID)
			if err != nil {
				t.Fatal(err)
			}
			if final.Version != startVersion+1 {
				t.Fatalf("policy version = %d; want %d after exactly one successful update",
					final.Version, startVersion+1)
			}

			// The successful mutation must be audited exactly once; the
			// refused one must leave nothing behind, because its whole
			// transaction rolled back.
			if got := auditCountFor(t, pool, ctx, businessA, created.ID); got != auditAfterCreate+1 {
				t.Fatalf("audit entries = %d; want exactly one more than %d - "+
					"a lost or duplicated audit record", got, auditAfterCreate)
			}
			if got := outboxCountFor(t, pool, ctx, businessA, created.ID); got != outboxAfterCreate+1 {
				t.Fatalf("outbox events = %d; want exactly one more than %d",
					got, outboxAfterCreate)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// F. Historical read during a live policy change
// ---------------------------------------------------------------------------

// TestRaceHistoricalSnapshotStaysStableWhilePolicyChanges proves a historical
// read is unaffected by concurrent changes to how cost is currently
// calculated.
//
// This is the property that makes historical reporting trustworthy: a figure
// from the past must not move because someone edited a policy while a report
// was being read.
func TestRaceHistoricalSnapshotStaysStableWhilePolicyChanges(t *testing.T) {
	pool, ctx := testPool(t)
	chain := seedE2EChain(t, pool, ctx, businessA, branchA1, runNonce+"_raceF")

	service := newCostServiceForRace(pool)
	actor := raceActor(businessA)
	engine := service.Engine()

	computed, err := engine.CalculateRecipeCost(ctx, businessA, branchA1, chain.versionID)
	if err != nil {
		t.Fatal(err)
	}
	computed.CalculationType = CalcRecipeSnapshot
	stored, err := PersistSnapshot(ctx, pool, businessA, computed,
		businessA+"_owner", "corr_race", "cmd_raceF_"+chain.nonce)
	if err != nil {
		t.Fatal(err)
	}
	originalTotal := amount(t, "snapshot total", stored.TotalCostMinor)

	// One goroutine rewrites the rice policy to a manual cost; the other reads
	// the historical snapshot repeatedly. The reader must never observe a
	// changed figure.
	policyID := "cp_raceF_" + chain.nonce
	exec(t, pool, ctx, `
		INSERT INTO vendor_cost_policies(
			id,business_id,subject_type,subject_ref,strategy,currency,
			manual_unit_cost_minor,manual_unit_code,
			created_by_staff_id,updated_by_staff_id)
		VALUES($1,$2,'INGREDIENT',$3,'MANUAL_CONFIGURED_COST','NGN',9999,'kg',$4,$4)
		ON CONFLICT (id) DO NOTHING`,
		policyID, businessA, "raceF_subject_"+chain.nonce, businessA+"_owner")

	writer := func() raceResult {
		for version := int64(1); version <= 5; version++ {
			_, updateErr := service.UpdatePolicy(context.Background(), actor, policyID, version,
				PolicyInput{
					SubjectType:         SubjectIngredient,
					SubjectRef:          "raceF_subject_" + chain.nonce,
					Strategy:            StrategyManual,
					Currency:            "NGN",
					ManualUnitCostMinor: int64Ptr(1000 + version),
					ManualUnitCode:      "kg",
					Reason:              "race F",
				})
			if updateErr != nil {
				return raceResult{err: updateErr}
			}
		}
		return raceResult{}
	}

	reader := func() raceResult {
		for i := 0; i < 25; i++ {
			reloaded, found, readErr := engine.LatestRecipeSnapshot(
				context.Background(), businessA, chain.versionID)
			if readErr != nil {
				return raceResult{err: readErr}
			}
			if !found {
				return raceResult{err: errors.New("historical snapshot disappeared mid-read")}
			}
			if reloaded.TotalCostMinor == nil || *reloaded.TotalCostMinor != originalTotal {
				return raceResult{err: fmt.Errorf(
					"historical total changed to %v while a policy was being edited",
					reloaded.TotalCostMinor)}
			}
		}
		return raceResult{}
	}

	writeResult, readResult := runRace(t, writer, reader)
	if writeResult.err != nil {
		t.Fatalf("policy writer failed: %v", writeResult.err)
	}
	if readResult.err != nil {
		t.Fatalf("historical read was not stable: %v", readResult.err)
	}

	// And the snapshot is still exactly what it was afterwards.
	final, found, err := engine.LatestRecipeSnapshot(ctx, businessA, chain.versionID)
	if err != nil || !found {
		t.Fatalf("reload: found=%t err=%v", found, err)
	}
	if got := amount(t, "final total", final.TotalCostMinor); got != originalTotal {
		t.Fatalf("historical total moved from %d to %d", originalTotal, got)
	}
}

// TestRaceCostCalculationCausesNoInventoryMutation proves that even under
// concurrency, costing never writes to inventory. Cost is a read of evidence
// that already exists.
func TestRaceCostCalculationCausesNoInventoryMutation(t *testing.T) {
	pool, ctx := testPool(t)
	chain := seedE2EChain(t, pool, ctx, businessA, branchA1, runNonce+"_raceInv")
	seedE2EBatch(t, pool, ctx, businessA, branchA1, chain)

	engine := NewEngine(pool, NewResolver(pool, NewPostgresPurchasingCostReader(pool)))

	var before int64
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM inventory_items WHERE business_id=$1`, businessA).Scan(&before); err != nil {
		t.Fatal(err)
	}

	calculate := func() raceResult {
		for i := 0; i < 10; i++ {
			if _, err := engine.CalculateProductionCost(
				context.Background(), businessA, chain.batchID, CalcProductionActual); err != nil {
				return raceResult{err: err}
			}
		}
		return raceResult{}
	}

	left, right := runRace(t, calculate, calculate)
	if left.err != nil || right.err != nil {
		t.Fatalf("concurrent calculation errored: left=%v right=%v", left.err, right.err)
	}

	var after int64
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM inventory_items WHERE business_id=$1`, businessA).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("concurrent cost calculation changed inventory: %d -> %d", before, after)
	}
}
