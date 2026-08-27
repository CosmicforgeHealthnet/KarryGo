package database

// Shape tests for the two Section 27 migrations.
//
// They read the SQL rather than a live database, so they run everywhere and
// they fail loudly the moment a constraint that carries a product guarantee
// is quietly removed. That is the point: most of what makes these tables safe
// is a CHECK or a UNIQUE, and a Go test asserting the same rules in the
// service layer would keep passing after somebody dropped one.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	planCandidateMigration = "000102_commerce_fulfilment_plan_candidates"
	basketMigration        = "000103_commerce_basket_optimization"
)

func readMigrationSQL(t *testing.T, name, suffix string) string {
	t.Helper()

	path := filepath.Join("..", "..", "migrations", name+suffix)

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	return strings.ToLower(string(raw))
}

// ---------------------------------------------------------------------------
// Plan candidates
// ---------------------------------------------------------------------------

func TestPlanCandidateMigrationShape(t *testing.T) {
	sql := readMigrationSQL(t, planCandidateMigration, ".up.sql")

	if !strings.Contains(
		sql, "create table commerce_fulfilment_plan_candidates",
	) {
		t.Fatal("migration is missing the candidate table")
	}

	// The columns a Support agent reads to answer "why was my order split"
	// and "why did the faster option cost more". A row storing only the
	// winner could answer neither.
	for _, column := range []string{
		"kind",
		"mode",
		"status",
		"reason_code",
		"policy_version",
		"mission_count",
		"vendor_group_count",
		"delivery_gross_minor",
		"delivery_customer_minor",
		"completion_minutes",
		"detour_minutes",
		"multi_stop_required",
		"service_capabilities",
		"legs",
		"fingerprint",
	} {
		if !strings.Contains(sql, column) {
			t.Errorf("migration is missing column %q", column)
		}
	}

	// THE INVARIANT. A hard constraint refusal may only appear on an
	// INELIGIBLE row, so a safety limit can never be recorded as though the
	// optimizer had weighed it against a price.
	if !strings.Contains(sql, "commerce_fulfilment_plan_candidates_hard_reason_chk") {
		t.Fatal("the hard-reason constraint is missing")
	}

	if !strings.Contains(
		sql, "commerce_fulfilment_plan_candidates_ineligible_reason_chk",
	) {
		t.Fatal("the ineligible-reason constraint is missing")
	}

	// Every hard reason the Go vocabulary marks as a constraint refusal must
	// appear in the SQL list, or a future code change could write one onto a
	// REJECTED row and the database would accept it.
	for _, reason := range []string{
		"capacity_exceeded",
		"temperature_conflict",
		"fragile_unsafe",
		"separation_conflict",
		"ready_window_conflict",
		"eta_detour_exceeded",
		"multi_stop_unsupported",
		"service_or_class_mismatch",
		"urgency_conflict",
		"candidate_not_quotable",
		"pickup_proximity_exceeded",
	} {
		if !strings.Contains(sql, reason) {
			t.Errorf("hard reason %q is missing from the constraint", reason)
		}
	}

	// One selected alternative per plan. Two would be two plans each claiming
	// to be the one that ran.
	if !strings.Contains(
		sql, "commerce_fulfilment_plan_candidates_one_selected_idx",
	) {
		t.Fatal("the one-selected-per-plan index is missing")
	}

	// One row per distinct plan shape: the same plan under two labels is the
	// duplication the section forbids.
	if !strings.Contains(sql, "unique (plan_id, fingerprint)") {
		t.Fatal("the per-plan fingerprint uniqueness is missing")
	}

	// A multi-stop refusal must be about a multi-stop plan.
	if !strings.Contains(
		sql, "commerce_fulfilment_plan_candidates_multi_stop_chk",
	) {
		t.Fatal("the multi-stop coherence constraint is missing")
	}

	// Customer isolation by row shape, not only by query predicate.
	//
	// A candidate carries the composite customer_id and joins back to its
	// plan through BOTH columns. That is stronger than a self-unique would
	// be: the row cannot exist at all unless the plan it names belongs to the
	// customer it names, so a mismatched pair is unwritable rather than
	// merely unqueried.
	if !strings.Contains(sql, "customer_id text not null") {
		t.Fatal("candidates do not carry the customer explicitly")
	}

	if !strings.Contains(
		sql, "foreign key (plan_id, customer_id)",
	) || !strings.Contains(
		sql, "references commerce_fulfilment_plans (id, customer_id)",
	) {
		t.Fatal("the customer-scoped composite foreign key is missing")
	}

	down := readMigrationSQL(t, planCandidateMigration, ".down.sql")

	if !strings.Contains(
		down, "drop table if exists commerce_fulfilment_plan_candidates",
	) {
		t.Fatal("the down migration does not drop the table")
	}
}

// ---------------------------------------------------------------------------
// Basket optimization
// ---------------------------------------------------------------------------

func TestBasketOptimizationMigrationShape(t *testing.T) {
	sql := readMigrationSQL(t, basketMigration, ".up.sql")

	for _, table := range []string{
		"create table commerce_basket_proposals",
		"create table commerce_recurring_basket_templates",
		"create table commerce_recurring_basket_generations",
	} {
		if !strings.Contains(sql, table) {
			t.Errorf("migration is missing %q", table)
		}
	}

	// THE EXACTLY-ONCE CONSTRAINT.
	//
	// This single line is what makes one scheduled occurrence produce one
	// generation across a retry, a replay, a restart, a duplicate scheduler
	// delivery and two concurrent workers. An application check cannot
	// survive any of them.
	if !strings.Contains(sql, "unique (template_id, occurrence_key)") {
		t.Fatal("the exactly-once occurrence constraint is missing")
	}

	// A proposal's parts must add up to its whole, enforced by the database
	// rather than by whoever wrote the total.
	if !strings.Contains(sql, "commerce_basket_proposals_total_chk") {
		t.Fatal("the proposal total constraint is missing")
	}

	// Acceptance and the cart version it produced are constrained together,
	// so a replayed acceptance always has a truthful version to return.
	if !strings.Contains(sql, "commerce_basket_proposals_accepted_chk") {
		t.Fatal("the acceptance coherence constraint is missing")
	}

	// One live offer per distinct basket per cart version: re-running the
	// optimizer must not accumulate identical acceptable offers.
	if !strings.Contains(sql, "commerce_basket_proposals_live_idx") {
		t.Fatal("the live-proposal uniqueness is missing")
	}

	// An unbounded standing purchase authority is refused by the schema, not
	// only by the service.
	if !strings.Contains(
		sql, "commerce_recurring_basket_templates_ceiling_chk",
	) {
		t.Fatal("the auto-approval ceiling constraint is missing")
	}

	// A monthly basket must have a day that exists in every month.
	if !strings.Contains(sql, "day_of_month between 1 and 28") {
		t.Fatal("the day-of-month cap is missing")
	}

	// The timezone is required. A schedule resolved against server-local time
	// drifts with deployments and is wrong for everybody outside that zone.
	if !strings.Contains(sql, "timezone text not null check (timezone <> '')") {
		t.Fatal("the required-timezone constraint is missing")
	}

	// An unfulfillable occurrence must not carry an acceptable proposal.
	if !strings.Contains(
		sql, "commerce_recurring_basket_generations_unfulfillable_chk",
	) {
		t.Fatal("the unfulfillable coherence constraint is missing")
	}

	// Customer isolation by row shape on both customer-owned tables.
	if strings.Count(sql, "unique (id, customer_id)") < 2 {
		t.Fatal("customer-scoped composite keys are missing")
	}

	down := readMigrationSQL(t, basketMigration, ".down.sql")

	// Dependency order matters: generations reference both other tables.
	generations := strings.Index(
		down, "drop table if exists commerce_recurring_basket_generations",
	)
	templates := strings.Index(
		down, "drop table if exists commerce_recurring_basket_templates",
	)
	proposals := strings.Index(
		down, "drop table if exists commerce_basket_proposals",
	)

	if generations < 0 || templates < 0 || proposals < 0 {
		t.Fatal("the down migration does not drop all three tables")
	}

	if generations > templates || generations > proposals {
		t.Fatal("generations must be dropped before the tables it references")
	}
}

// TestSection27MigrationsArePaired guards the runner's own contract.
//
// The migration runner drives rollback from the .down.sql files, so an up
// without a down is a migration that cannot be reversed - discovered, if at
// all, during a rollback nobody wants to be debugging.
func TestSection27MigrationsArePaired(t *testing.T) {
	for _, name := range []string{planCandidateMigration, basketMigration} {
		for _, suffix := range []string{".up.sql", ".down.sql"} {
			path := filepath.Join("..", "..", "migrations", name+suffix)

			if _, err := os.Stat(path); err != nil {
				t.Errorf("missing migration file %s: %v", path, err)
			}
		}
	}
}
