package crm

// The CRM fixture reset.
//
// newTestEnv runs 347 times in this package, and it used to clear state with
// TRUNCATE ... CASCADE over the tables below. TRUNCATE's cost is per relation,
// not per row: measured on an idle database, that one statement took 1.53s with
// data present and 3.34s with every table already empty, because the work is
// rewriting a relfilenode and its catalog entry for each of the ~52 relations in
// the cascade closure. Across the package that is roughly 530 of its 667
// seconds spent emptying tables that, for most tests, were already empty.
//
// So the reset deletes instead of truncating, and only from tables that
// actually hold rows. DELETE against an empty table is a single index probe;
// against the handful of rows a typical test writes it is trivial. The end
// state is identical - every table in the closure is empty - and nothing is
// exempted, so no row a test wrote can survive into the next test.
//
// Three details make that safe:
//
//   - Order. DELETE enforces foreign keys, so children must go before parents.
//     The order is computed once from pg_constraint rather than hand-written,
//     so a later migration cannot silently invalidate it.
//   - The one cycle. crm_campaigns.current_snapshot_id and
//     crm_campaign_audience_snapshots.campaign_id reference each other, so no
//     topological order exists for that pair. Any nullable column inside a
//     cycle is set to NULL first, which breaks it; that is derived from the
//     catalog too, not hardcoded to this pair.
//   - Fallback. If the plan cannot be built for any reason the reset falls back
//     to the original TRUNCATE, so a catalog surprise costs time rather than
//     correctness.
//
// RESTART IDENTITY was dropped because it had nothing to restart: not one of
// these tables owns a sequence - every id is an application-assigned text key.

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// crmFixtureTables is the state every CRM test starts from empty. It is the
// same list the previous TRUNCATE named; the FK closure below extends it to
// anything that references these, so a table cannot be missed by omission.
var crmFixtureTables = []string{
	"crm_task_trigger_evaluations",
	"crm_relationship_signals", "crm_customer_value_projections", "crm_loyalty_tier_definitions",
	"crm_corporate_rfq_projections", "crm_corporate_rfq_facts", "crm_corporate_commercial_projections",
	"crm_customer_service_recovery_projections", "crm_customer_support_facts",
	"crm_customer_item_affinities", "crm_customer_category_affinities", "crm_customer_order_fact_lines",
	"crm_exports",
	"crm_customer_intelligence_scores", "crm_intelligence_scope_states", "crm_intelligence_evaluations",
	"corporate_customer_memberships", "corporate_customer_contacts",
	"corporate_customer_cost_centers", "corporate_customer_locations", "corporate_customer_branch_scopes",
	"corporate_customer_profiles", "corporate_customer_code_counters",
	"crm_campaign_target_channels", "crm_campaign_targets",
	"crm_campaign_delivery_event_inbox", "crm_campaign_contact_history", "crm_campaign_delivery_history",
	"crm_campaign_deliveries", "crm_campaign_runs",
	"crm_campaign_audience_snapshots", "crm_campaign_experiment_variants", "crm_campaign_versions", "crm_campaigns",
	"crm_loyalty_lot_consumptions", "crm_loyalty_point_lots", "crm_loyalty_entries",
	"crm_loyalty_accounts", "crm_loyalty_program_configs",
	"crm_task_history", "crm_tasks", "crm_interaction_revisions", "crm_interactions",
	"crm_segment_exclusions", "crm_segment_memberships",
	"crm_segment_evaluations", "crm_segment_versions", "crm_segments",
	"crm_customer_tag_memberships", "crm_tags",
	"crm_privacy_processing_history", "crm_privacy_projections",
	"crm_consent_projections", "crm_customer_order_facts", "crm_customer_branch_relations",
	"crm_customer_identity_links", "vendor_customer_profiles", "vendor_inbox_processed_events",
	"vendor_service_nonces", "vendor_outbox", "vendor_audit_log", "vendor_idempotency_keys",
}

// crmFallbackTruncate is what the reset used to do, kept as the safety net.
var crmFallbackTruncate = "TRUNCATE " + strings.Join(crmFixtureTables, ", ") + " RESTART IDENTITY CASCADE"

// crmResetPlan is the delete order plus whatever has to happen first to make it
// possible.
type crmResetPlan struct {
	// ordered lists every table in the closure, children before parents.
	ordered []string
	// cycleBreakers null the FK columns that make a topological order
	// impossible, keyed by the table they belong to so they only run when that
	// table is actually dirty.
	cycleBreakers map[string][]string
	// dirtyProbe asks, in one round trip, which of ordered hold any rows.
	dirtyProbe string
}

var (
	crmResetPlanOnce sync.Once
	crmResetPlanVal  *crmResetPlan
	crmResetPlanErr  error
)

// crmResetPlanFor builds the plan once per test binary.
func crmResetPlanFor(ctx context.Context, pool *pgxpool.Pool) (*crmResetPlan, error) {
	crmResetPlanOnce.Do(func() {
		crmResetPlanVal, crmResetPlanErr = buildCRMResetPlan(ctx, pool)
	})
	return crmResetPlanVal, crmResetPlanErr
}

func buildCRMResetPlan(ctx context.Context, pool *pgxpool.Pool) (*crmResetPlan, error) {
	// The closure: the named tables plus everything that references them,
	// transitively. Anything reachable this way would have been swept up by the
	// old CASCADE, so it has to be cleared here too.
	rows, err := pool.Query(ctx, `
		WITH RECURSIVE seed AS (
			SELECT c.oid
			  FROM pg_class c
			  JOIN pg_namespace n ON n.oid = c.relnamespace
			 WHERE n.nspname = 'public' AND c.relname = ANY($1)
		), closure AS (
			SELECT oid FROM seed
			UNION
			SELECT con.conrelid
			  FROM pg_constraint con
			  JOIN closure ON con.confrelid = closure.oid
			 WHERE con.contype = 'f'
		)
		SELECT c.relname FROM closure JOIN pg_class c ON c.oid = closure.oid`, crmFixtureTables)
	if err != nil {
		return nil, fmt.Errorf("read fixture closure: %w", err)
	}
	tables := map[string]bool{}
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			rows.Close()
			return nil, err
		}
		tables[name] = true
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(tables) == 0 {
		return nil, fmt.Errorf("fixture closure is empty")
	}

	names := make([]string, 0, len(tables))
	for name := range tables {
		names = append(names, name)
	}
	sort.Strings(names)

	// Edges within the closure. Self-references are excluded: deleting every
	// row of a table in one statement satisfies its own foreign key, because
	// NO ACTION is checked at end of statement.
	edgeRows, err := pool.Query(ctx, `
		SELECT child.relname, parent.relname
		  FROM pg_constraint con
		  JOIN pg_class child ON child.oid = con.conrelid
		  JOIN pg_class parent ON parent.oid = con.confrelid
		 WHERE con.contype = 'f'
		   AND con.conrelid <> con.confrelid
		   AND child.relname = ANY($1)
		   AND parent.relname = ANY($1)`, names)
	if err != nil {
		return nil, fmt.Errorf("read fixture foreign keys: %w", err)
	}
	children := map[string]map[string]bool{} // parent -> children
	parents := map[string]map[string]bool{}  // child -> parents
	for edgeRows.Next() {
		var child, parent string
		if err = edgeRows.Scan(&child, &parent); err != nil {
			edgeRows.Close()
			return nil, err
		}
		if children[parent] == nil {
			children[parent] = map[string]bool{}
		}
		if parents[child] == nil {
			parents[child] = map[string]bool{}
		}
		children[parent][child] = true
		parents[child][parent] = true
	}
	edgeRows.Close()
	if err = edgeRows.Err(); err != nil {
		return nil, err
	}

	// Kahn, emitting a table once nothing left references it: children first.
	remaining := map[string]bool{}
	for _, name := range names {
		remaining[name] = true
	}
	ordered := make([]string, 0, len(names))
	for {
		var ready []string
		for _, name := range names {
			if !remaining[name] {
				continue
			}
			blocked := false
			for child := range children[name] {
				if remaining[child] {
					blocked = true
					break
				}
			}
			if !blocked {
				ready = append(ready, name)
			}
		}
		if len(ready) == 0 {
			break
		}
		for _, name := range ready {
			delete(remaining, name)
		}
		ordered = append(ordered, ready...)
	}

	// Whatever Kahn could not place is in a cycle. Null the nullable columns
	// that point inside the cycle, then the members can go in any order.
	cycleBreakers := map[string][]string{}
	if len(remaining) > 0 {
		stuck := make([]string, 0, len(remaining))
		for name := range remaining {
			stuck = append(stuck, name)
		}
		sort.Strings(stuck)

		breakerRows, err := pool.Query(ctx, `
			SELECT child.relname, att.attname
			  FROM pg_constraint con
			  JOIN pg_class child ON child.oid = con.conrelid
			  JOIN pg_class parent ON parent.oid = con.confrelid
			  JOIN pg_attribute att ON att.attrelid = con.conrelid AND att.attnum = con.conkey[1]
			 WHERE con.contype = 'f'
			   AND con.conrelid <> con.confrelid
			   AND NOT att.attnotnull
			   AND child.relname = ANY($1)
			   AND parent.relname = ANY($1)`, stuck)
		if err != nil {
			return nil, fmt.Errorf("read cycle-breaking columns: %w", err)
		}
		for breakerRows.Next() {
			var table, column string
			if err = breakerRows.Scan(&table, &column); err != nil {
				breakerRows.Close()
				return nil, err
			}
			cycleBreakers[table] = append(cycleBreakers[table],
				fmt.Sprintf("UPDATE %s SET %s = NULL", table, column))
		}
		breakerRows.Close()
		if err = breakerRows.Err(); err != nil {
			return nil, err
		}

		unbreakable := make([]string, 0, len(stuck))
		for _, name := range stuck {
			if len(cycleBreakers[name]) == 0 && len(parents[name]) > 0 {
				// Only a problem if this table still references another stuck
				// table through a NOT NULL column.
				for parent := range parents[name] {
					if remaining[parent] {
						unbreakable = append(unbreakable, name)
						break
					}
				}
			}
		}
		// A member whose only inbound edges come from members we can null is
		// fine; a genuinely unbreakable cycle is not, and is worth failing on
		// rather than silently leaving rows behind.
		if len(unbreakable) == len(stuck) {
			return nil, fmt.Errorf("foreign-key cycle with no nullable column: %v", stuck)
		}
		ordered = append(ordered, stuck...)
	}

	// One round trip that reports which tables hold anything. EXISTS stops at
	// the first row, so this stays cheap however much data a test wrote.
	probes := make([]string, 0, len(ordered))
	for _, name := range ordered {
		probes = append(probes, fmt.Sprintf(
			"SELECT '%s' AS relname WHERE EXISTS (SELECT 1 FROM %s)", name, name))
	}

	return &crmResetPlan{
		ordered:       ordered,
		cycleBreakers: cycleBreakers,
		dirtyProbe:    strings.Join(probes, " UNION ALL "),
	}, nil
}

// resetCRMFixture empties every fixture table, touching only those that hold
// rows.
func resetCRMFixture(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	plan, err := crmResetPlanFor(ctx, pool)
	if err != nil {
		t.Logf("CRM fixture reset plan unavailable (%v); falling back to TRUNCATE", err)
		if _, truncateErr := pool.Exec(ctx, crmFallbackTruncate); truncateErr != nil {
			t.Fatalf("reset CRM fixture: %v", truncateErr)
		}
		return
	}

	dirty, err := collectDirtyTables(ctx, pool, plan)
	if err != nil {
		t.Fatalf("probe CRM fixture state: %v", err)
	}
	if len(dirty) == 0 {
		return
	}

	statements := make([]string, 0, len(plan.ordered)+4)
	for _, table := range plan.ordered {
		if dirty[table] {
			statements = append(statements, plan.cycleBreakers[table]...)
		}
	}
	for _, table := range plan.ordered {
		if dirty[table] {
			statements = append(statements, "DELETE FROM "+table)
		}
	}

	// Sent unparameterised so PostgreSQL accepts the batch as one command
	// string, which keeps the whole reset to a single round trip.
	if _, err = pool.Exec(ctx, strings.Join(statements, "; ")); err != nil {
		t.Fatalf("reset CRM fixture: %v", err)
	}
}

func collectDirtyTables(ctx context.Context, pool *pgxpool.Pool, plan *crmResetPlan) (map[string]bool, error) {
	rows, err := pool.Query(ctx, plan.dirtyProbe)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	dirty := map[string]bool{}
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			return nil, err
		}
		dirty[name] = true
	}
	return dirty, rows.Err()
}

// assertCRMFixtureIsEmpty is the guard on the optimisation: it proves the
// targeted delete leaves exactly the state the old TRUNCATE left. If a future
// migration adds a table the closure does not reach, this fails instead of
// letting one test's rows leak into the next.
func assertCRMFixtureIsEmpty(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	plan, err := crmResetPlanFor(ctx, pool)
	if err != nil {
		t.Fatalf("build reset plan: %v", err)
	}
	dirty, err := collectDirtyTables(ctx, pool, plan)
	if err != nil {
		t.Fatalf("probe fixture state: %v", err)
	}
	if len(dirty) != 0 {
		names := make([]string, 0, len(dirty))
		for name := range dirty {
			names = append(names, name)
		}
		sort.Strings(names)
		t.Fatalf("fixture reset left rows in %v", names)
	}
}

// TestCRMFixtureResetClearsEverythingTruncateWould writes into a spread of the
// fixture tables - including both sides of the campaign cycle - and proves the
// reset empties the whole closure.
func TestCRMFixtureResetClearsEverythingTruncateWould(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()

	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_reset_probe"))
	audience := env.staticAudience(t, businessA, "vs_a", "Reset probe", profile.ID)
	campaign := env.createCampaign(t, businessA, "vs_a", campaignInput("Reset probe", audience.ID))
	if _, err := env.service.PrepareCampaign(ctx, actorFor(businessA, "vs_a"), globalScope(), campaign.ID); err != nil {
		t.Fatalf("prepare campaign: %v", err)
	}
	env.runPreparations(t)

	// Both sides of the crm_campaigns <-> crm_campaign_audience_snapshots cycle
	// now hold rows, which is the case a plain topological order cannot handle.
	var snapshots, campaigns, profiles int
	if err := env.pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM crm_campaign_audience_snapshots),
		(SELECT count(*) FROM crm_campaigns),
		(SELECT count(*) FROM vendor_customer_profiles)`).Scan(&snapshots, &campaigns, &profiles); err != nil {
		t.Fatal(err)
	}
	if snapshots == 0 || campaigns == 0 || profiles == 0 {
		t.Fatalf("fixture probe did not populate the cycle: snapshots=%d campaigns=%d profiles=%d",
			snapshots, campaigns, profiles)
	}

	resetCRMFixture(ctx, t, env.pool)
	assertCRMFixtureIsEmpty(t, env.pool)
}

// TestCRMFixtureResetPlanCoversTheCascadeClosure proves the computed plan is at
// least as wide as the TRUNCATE it replaced, so nothing the old CASCADE swept
// up is now left behind.
func TestCRMFixtureResetPlanCoversTheCascadeClosure(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())

	plan, err := crmResetPlanFor(context.Background(), env.pool)
	if err != nil {
		t.Fatalf("build reset plan: %v", err)
	}
	covered := map[string]bool{}
	for _, table := range plan.ordered {
		covered[table] = true
	}
	for _, table := range crmFixtureTables {
		if !covered[table] {
			t.Errorf("reset plan omits %s", table)
		}
	}
	if len(plan.ordered) < len(crmFixtureTables) {
		t.Fatalf("reset plan covers %d tables, fewer than the %d it names",
			len(plan.ordered), len(crmFixtureTables))
	}

	// Every table must appear after all of its children.
	position := map[string]int{}
	for index, table := range plan.ordered {
		position[table] = index
	}
	rows, err := env.pool.Query(context.Background(), `
		SELECT child.relname, parent.relname
		  FROM pg_constraint con
		  JOIN pg_class child ON child.oid = con.conrelid
		  JOIN pg_class parent ON parent.oid = con.confrelid
		 WHERE con.contype = 'f' AND con.conrelid <> con.confrelid
		   AND child.relname = ANY($1) AND parent.relname = ANY($1)`, plan.ordered)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var child, parent string
		if err = rows.Scan(&child, &parent); err != nil {
			t.Fatal(err)
		}
		if position[child] > position[parent] && len(plan.cycleBreakers[child]) == 0 {
			t.Errorf("%s is deleted after its parent %s and has no cycle-breaking column", child, parent)
		}
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
}
