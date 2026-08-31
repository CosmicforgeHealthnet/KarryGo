package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Section 8 completion migration guard.
//
// The pair `000026`/`000027` must roll back and reapply without damaging
// Sections 5-8I, because the completion is additive to accepted schema: it adds
// columns to `crm_customer_order_facts`, `vendor_customer_profiles`,
// `crm_interactions` and `crm_tasks`, and a rollback that left one of those
// behind would leave an 8H-era table carrying a completion-era constraint.

func TestSection8CompletionMigrationsRollbackAndReapplyAsAPair(t *testing.T) {
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping real-Postgres Section 8 completion replay")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatal(err)
	}

	// Every table the completion introduces exists at the tip.
	completionTables := []string{
		"crm_customer_order_fact_lines",
		"crm_customer_category_affinities",
		"crm_customer_item_affinities",
		"crm_customer_support_facts",
		"crm_customer_service_recovery_projections",
		"crm_corporate_commercial_projections",
		"crm_corporate_rfq_facts",
		"crm_corporate_rfq_projections",
		"crm_loyalty_tier_definitions",
		"crm_customer_value_projections",
		"crm_relationship_signals",
		"crm_task_trigger_evaluations",
	}
	for _, table := range completionTables {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || !exists {
			t.Fatalf("%s is missing at the migration tip exists=%t err=%v", table, exists, err)
		}
	}

	// Availability is structural, not conventional: a projection that reports
	// NOT_AVAILABLE must be incapable of also carrying a count, or the
	// difference between "never observed" and "observed zero" would rest on
	// application code remembering to null the column.
	unavailableWithACount := []struct {
		table  string
		insert string
	}{
		{"crm_customer_service_recovery_projections",
			`INSERT INTO crm_customer_service_recovery_projections
			 (business_id, profile_id, state, complaint_count, refund_count, computed_at)
			 VALUES ('biz_x','prof_x','NOT_AVAILABLE',0,0, now())`},
		{"crm_corporate_commercial_projections",
			`INSERT INTO crm_corporate_commercial_projections
			 (business_id, corporate_customer_id, state, order_count, computed_at)
			 VALUES ('biz_x','corp_x','NOT_AVAILABLE',0, now())`},
		{"crm_corporate_rfq_projections",
			`INSERT INTO crm_corporate_rfq_projections
			 (business_id, corporate_customer_id, state, rfq_count, computed_at)
			 VALUES ('biz_x','corp_x','NOT_AVAILABLE',0, now())`},
	}
	for _, c := range unavailableWithACount {
		if _, err := pool.Exec(ctx, c.insert); err == nil {
			t.Fatalf("%s accepted a NOT_AVAILABLE row carrying a zero count. "+
				"Unavailable and zero are different facts and only one of them is true; "+
				"the database must refuse to store the confusion", c.table)
		}
	}

	// A value band exists exactly when the projection is CURRENT, so an
	// unavailable value can never be read as the lowest band.
	if _, err := pool.Exec(ctx, `INSERT INTO crm_customer_value_projections
		(business_id, profile_id, model_version, definition_hash, state, band, reason_codes, source_as_of)
		VALUES ('biz_x','prof_x',1,'hash','NOT_AVAILABLE','STANDARD','{NO_DATA}', now())`); err == nil {
		t.Fatal("an unavailable customer value was allowed to carry a band")
	}

	// Loyalty tiers confer recognition, not benefits.
	var monetary int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_name = 'crm_loyalty_tier_definitions'
		  AND (column_name LIKE '%amount%' OR column_name LIKE '%minor%'
		    OR column_name LIKE '%currency%' OR column_name LIKE '%discount%'
		    OR column_name LIKE '%benefit%' OR column_name LIKE '%price%')`).Scan(&monetary); err != nil {
		t.Fatal(err)
	}
	if monetary != 0 {
		t.Fatalf("crm_loyalty_tier_definitions carries %d monetary or benefit columns; "+
			"payment-wallet-service is the only monetary authority on this platform", monetary)
	}

	// The three completion permissions land, and exactly one is high-risk:
	// defining the thresholds that decide a customer's public status.
	var permissions, highRisk int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_permissions WHERE code IN
		('vendor.crm.customer_value.read','vendor.crm.loyalty.tiers.manage',
		 'vendor.crm.followup_triggers.read')`).Scan(&permissions); err != nil || permissions != 3 {
		t.Fatalf("completion permissions=%d err=%v", permissions, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_permissions WHERE code IN
		('vendor.crm.customer_value.read','vendor.crm.loyalty.tiers.manage',
		 'vendor.crm.followup_triggers.read') AND is_high_risk`).Scan(&highRisk); err != nil || highRisk != 1 {
		t.Fatalf("high-risk completion permissions=%d; want tiers.manage alone err=%v", highRisk, err)
	}

	// There is deliberately no permission to WRITE a derived value: a grant
	// that let a staff member declare a customer high-value would make every
	// other customer's band unfalsifiable.
	var writable int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_permissions
		WHERE code LIKE 'vendor.crm.customer_value.%'
		  AND code <> 'vendor.crm.customer_value.read'`).Scan(&writable); err != nil || writable != 0 {
		t.Fatalf("customer value has %d write permissions; it is derived and must have none", writable)
	}

	// Roll the pair back.
	rollbackUntilRemoved(t, ctx, pool, "000026_vendor_crm_section8_completion")

	for _, table := range completionTables {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || exists {
			t.Fatalf("%s survived the rollback; the down migration untracked it without dropping it exists=%t err=%v",
				table, exists, err)
		}
	}
	// The additive columns on accepted tables must go too.
	for _, col := range []struct{ table, column string }{
		{"crm_customer_order_facts", "order_type"},
		{"crm_customer_order_facts", "corporate_customer_id"},
		{"vendor_customer_profiles", "observed_order_count"},
		{"crm_interactions", "related_order_ref"},
		{"crm_tasks", "trigger_key"},
		{"crm_tasks", "completion_outcome"},
	} {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns
			WHERE table_name=$1 AND column_name=$2)`, col.table, col.column).Scan(&exists); err != nil || exists {
			t.Fatalf("%s.%s survived the rollback exists=%t err=%v", col.table, col.column, exists, err)
		}
	}

	// Sections 5-8I are undamaged: spot-check one table from each of the
	// accepted CRM sections rather than trusting that "the tip still applies".
	for _, table := range []string{
		"vendor_customer_profiles", "crm_segments", "crm_tasks",
		"crm_loyalty_accounts", "crm_campaigns", "crm_campaign_deliveries",
		"corporate_customer_profiles", "crm_customer_intelligence_scores", "crm_exports",
	} {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || !exists {
			t.Fatalf("%s was damaged by the Section 8 completion rollback exists=%t err=%v", table, exists, err)
		}
	}

	// Reapply.
	if err = ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	for _, table := range completionTables {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || !exists {
			t.Fatalf("%s is missing after reapply exists=%t err=%v", table, exists, err)
		}
	}
}
