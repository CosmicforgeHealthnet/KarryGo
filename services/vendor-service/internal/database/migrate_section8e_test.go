package database

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestSection8EMigrationsRollbackAndReapplyAsAPair proves the campaign
// intelligence schema and its permissions install, roll back cleanly to the
// accepted Section 8D baseline, and reapply — and that rolling 8E back damages
// nothing behind it.
//
// It also pins the boundary the whole section exists to hold: the schema can
// describe who a campaign should reach and cannot describe a delivery.
func TestSection8EMigrationsRollbackAndReapplyAsAPair(t *testing.T) {
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping real-Postgres Section 8E replay")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatal(err)
	}

	section8ETables := []string{"crm_campaigns", "crm_campaign_versions",
		"crm_campaign_experiment_variants", "crm_campaign_audience_snapshots",
		"crm_campaign_targets", "crm_campaign_target_channels"}

	rollbackUntilRemoved(t, ctx, pool, "000018_vendor_crm_campaign_intelligence")
	for _, table := range section8ETables {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || exists {
			t.Fatalf("Section 8E table %s survived rollback exists=%t err=%v", table, exists, err)
		}
	}
	// Everything Sections 5-8D own must be untouched by an 8E rollback.
	for _, table := range []string{"crm_loyalty_accounts", "crm_tasks", "crm_interactions", "crm_tags",
		"crm_segments", "crm_segment_memberships", "vendor_customer_profiles", "vendor_staff", "vendor_businesses"} {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || !exists {
			t.Fatalf("accepted baseline table %s damaged by an 8E rollback exists=%t err=%v", table, exists, err)
		}
	}
	var permissions int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_permissions WHERE code LIKE 'vendor.crm.campaigns.%'`).Scan(&permissions); err != nil || permissions != 0 {
		t.Fatalf("Section 8E permissions survived rollback count=%d err=%v", permissions, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_permissions WHERE code LIKE 'vendor.crm.loyalty.%'`).Scan(&permissions); err != nil || permissions != 4 {
		t.Fatalf("Section 8D permissions damaged by an 8E rollback count=%d err=%v", permissions, err)
	}

	if err = ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("reapply Section 8E: %v", err)
	}
	for _, table := range section8ETables {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || !exists {
			t.Fatalf("reapplied table %s exists=%t err=%v", table, exists, err)
		}
	}

	// Exact vocabularies, enforced in SQL rather than only in Go.
	vocabularies := map[string][]string{
		"crm_campaigns_status_check":                                    {"DRAFT", "PREPARING", "PREPARED", "PREPARATION_FAILED", "ARCHIVED"},
		"crm_campaigns_objective_check":                                 {"GENERAL_MARKETING", "RETENTION", "REACTIVATION", "LOYALTY", "ANNOUNCEMENT"},
		"crm_campaigns_scope_type_check":                                {"BUSINESS", "BRANCH"},
		"crm_campaign_audience_snapshots_status_check":                  {"BUILDING", "CURRENT", "FAILED", "SUPERSEDED"},
		"crm_campaign_target_channels_channel_check":                    {"EMAIL", "PUSH", "IN_APP", "SMS"},
		"crm_campaign_target_channels_snapshot_eligibility_state_check": {"ELIGIBLE_AT_SNAPSHOT", "INELIGIBLE_CONSENT"},
		"crm_campaign_target_channels_snapshot_consent_decision_check":  {"ALLOW", "DENY", "UNKNOWN"},
	}
	for constraint, values := range vocabularies {
		var definition string
		if err = pool.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname=$1`, constraint).Scan(&definition); err != nil {
			t.Fatalf("read %s: %v", constraint, err)
		}
		for _, value := range values {
			if !strings.Contains(definition, "'"+value+"'") {
				t.Fatalf("%s does not contain %s: %s", constraint, value, definition)
			}
		}
	}

	// The campaign lifecycle stops before delivery. A status that described a
	// send would mean Section 8F had leaked backwards into 8E.
	var lifecycle string
	if err = pool.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname='crm_campaigns_status_check'`).Scan(&lifecycle); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"SCHEDULED", "SENDING", "SENT", "DELIVERED"} {
		if strings.Contains(lifecycle, "'"+forbidden+"'") {
			t.Fatalf("campaign lifecycle contains delivery state %s: %s", forbidden, lifecycle)
		}
	}
	// And no channel row may ever claim authority to send.
	var eligibility string
	if err = pool.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint
		WHERE conname='crm_campaign_target_channels_snapshot_eligibility_state_check'`).Scan(&eligibility); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(eligibility, "AUTHORIZED_TO_SEND") {
		t.Fatalf("target channel eligibility claims send authority: %s", eligibility)
	}

	// Tenancy: every Section 8E child reaches its parent through the composite
	// (id, business_id) key, so PostgreSQL — not only the HTTP layer — refuses a
	// campaign pointing at another business's segment, branch or customer.
	for _, constraint := range []string{
		"crm_campaign_branch_fk", "crm_campaign_version_campaign_fk", "crm_campaign_version_segment_fk",
		"crm_campaign_version_branch_fk", "crm_campaign_variant_version_fk",
		"crm_campaign_snapshot_campaign_fk", "crm_campaign_snapshot_version_fk",
		"crm_campaign_snapshot_segment_fk", "crm_campaign_snapshot_generation_fk",
		"crm_campaign_target_snapshot_fk", "crm_campaign_target_profile_fk",
		"crm_campaign_target_channel_target_fk", "crm_campaign_target_channel_snapshot_fk",
		"crm_campaign_current_snapshot_fk",
	} {
		var definition string
		if err = pool.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname=$1`, constraint).Scan(&definition); err != nil {
			t.Fatalf("read %s: %v", constraint, err)
		}
		if !strings.Contains(definition, "business_id") {
			t.Fatalf("%s is not tenant-composite: %s", constraint, definition)
		}
	}

	// Indexes the hot paths depend on. A missing one turns audience preparation
	// or the worker claim into a sequential scan.
	for _, index := range []string{
		"crm_campaigns_status_idx", "crm_campaigns_branch_idx", "crm_campaigns_objective_idx",
		"crm_campaign_versions_campaign_idx", "crm_campaign_variant_single_control_idx",
		"crm_campaign_snapshots_single_building_idx", "crm_campaign_snapshots_campaign_idx",
		"crm_campaign_snapshots_status_idx", "crm_campaign_snapshots_due_idx",
		"crm_campaign_snapshots_retention_idx", "crm_campaign_targets_snapshot_idx",
		"crm_campaign_targets_cohort_idx", "crm_campaign_targets_profile_idx",
		"crm_campaign_target_channels_eligibility_idx",
	} {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE indexname=$1)`, index).Scan(&exists); err != nil || !exists {
			t.Fatalf("index %s exists=%t err=%v", index, exists, err)
		}
	}

	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_permissions WHERE code IN
		('vendor.crm.campaigns.read','vendor.crm.campaigns.manage','vendor.crm.campaigns.preview',
		 'vendor.crm.campaigns.prepare','vendor.crm.campaigns.targets.read')`).Scan(&permissions); err != nil || permissions != 5 {
		t.Fatalf("Section 8E permissions=%d err=%v", permissions, err)
	}
	// Roles with no marketing reach must hold none of it.
	var operational int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_role_permissions rp
		JOIN vendor_permissions p ON p.id=rp.permission_id
		WHERE rp.role_id IN ('role_finance','role_kitchen_pastry','role_shop_inventory','role_procurement','role_vendor_rider')
		  AND p.code LIKE 'vendor.crm.campaigns.%'`).Scan(&operational); err != nil || operational != 0 {
		t.Fatalf("operational roles hold campaign permissions count=%d err=%v", operational, err)
	}
	// Analyst is read/preview only, and never gets the customer-by-customer list.
	var analystWrites int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_role_permissions rp
		JOIN vendor_permissions p ON p.id=rp.permission_id
		WHERE rp.role_id='role_analyst_auditor' AND p.code IN
		 ('vendor.crm.campaigns.read','vendor.crm.campaigns.manage','vendor.crm.campaigns.preview',
		  'vendor.crm.campaigns.prepare','vendor.crm.campaigns.targets.read')
		  AND p.code NOT IN ('vendor.crm.campaigns.read','vendor.crm.campaigns.preview')`).Scan(&analystWrites); err != nil || analystWrites != 0 {
		t.Fatalf("analyst holds a campaign write or target-read permission count=%d err=%v", analystWrites, err)
	}

	// Section 8E's campaign-intelligence tables store no personal data and no
	// delivery machinery. Section 8F has its own execution tables.
	var suspicious int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema='public' AND table_name IN
		 ('crm_campaigns','crm_campaign_versions','crm_campaign_experiment_variants',
		  'crm_campaign_audience_snapshots','crm_campaign_targets','crm_campaign_target_channels')
		  AND (column_name ~ '(email|phone|address|first_name|last_name|password|device_token|push_token)'
			OR column_name ~ '(scheduled_at|sent_at|delivered_at|provider|template|message_body|delivery)')`).Scan(&suspicious); err != nil || suspicious != 0 {
		t.Fatalf("campaign schema holds personal or delivery columns count=%d err=%v", suspicious, err)
	}

	// Preparing an audience is never permission to contact it, and the database
	// will not accept a row that says otherwise.
	var recheck string
	if err = pool.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint
		WHERE conname='crm_campaign_target_channels_send_time_recheck_required_check'`).Scan(&recheck); err != nil {
		t.Fatalf("send-time recheck constraint missing: %v", err)
	}
	if !strings.Contains(recheck, "send_time_recheck_required") {
		t.Fatalf("send-time recheck constraint does not pin the column: %s", recheck)
	}
}
