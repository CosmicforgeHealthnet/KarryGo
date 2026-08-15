package database

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestSection8HMigrationsRollbackAndReapplyAsAPair proves the intelligence and
// export schema installs, rolls back to the accepted Section 8G baseline, and
// reapplies — and that rolling 8H back damages nothing behind it.
//
// It also pins the two boundaries the section exists to hold: a score is an
// integer heuristic with a checked range and no probability anywhere, and an
// export cannot become readable without being complete.
func TestSection8HMigrationsRollbackAndReapplyAsAPair(t *testing.T) {
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping real-Postgres Section 8H replay")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatal(err)
	}

	section8HTables := []string{
		"crm_intelligence_evaluations", "crm_intelligence_scope_states",
		"crm_customer_intelligence_scores", "crm_exports",
	}

	rollbackUntilRemoved(t, ctx, pool, "000024_vendor_crm_intelligence_exports")
	for _, table := range section8HTables {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || exists {
			t.Fatalf("Section 8H table %s survived rollback exists=%t err=%v", table, exists, err)
		}
	}
	// Sections 5-8G must be untouched by an 8H rollback.
	for _, table := range []string{"corporate_customer_profiles", "crm_campaigns", "crm_campaign_runs",
		"crm_loyalty_accounts", "crm_tasks", "crm_interactions", "crm_tags", "crm_segments",
		"crm_segment_memberships", "vendor_customer_profiles", "vendor_staff", "vendor_businesses"} {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || !exists {
			t.Fatalf("accepted baseline table %s damaged by an 8H rollback exists=%t err=%v", table, exists, err)
		}
	}
	var permissions int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_permissions
		WHERE code LIKE 'vendor.crm.intelligence.%' OR code LIKE 'vendor.crm.exports.%'`).Scan(&permissions); err != nil ||
		permissions != 0 {
		t.Fatalf("Section 8H permissions survived rollback count=%d err=%v", permissions, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_permissions
		WHERE code LIKE 'vendor.crm.corporate.%'`).Scan(&permissions); err != nil || permissions != 5 {
		t.Fatalf("Section 8G permissions damaged by an 8H rollback count=%d err=%v", permissions, err)
	}

	if err = ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("reapply Section 8H: %v", err)
	}
	for _, table := range section8HTables {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || !exists {
			t.Fatalf("reapplied table %s exists=%t err=%v", table, exists, err)
		}
	}

	// Exact vocabularies, enforced in SQL rather than only in Go.
	vocabularies := map[string][]string{
		"crm_intelligence_evaluations_model_key_check":       {"RFM_V1", "CHURN_V1"},
		"crm_intelligence_evaluations_scope_check":           {"BUSINESS", "BRANCH"},
		"crm_intelligence_evaluations_status_check":          {"QUEUED", "RUNNING", "COMPLETED", "FAILED", "DEAD_LETTER", "SUPERSEDED"},
		"crm_customer_intelligence_scores_state_check":       {"NOT_AVAILABLE", "CURRENT", "STALE", "FAILED"},
		"crm_customer_intelligence_scores_risk_band_check":   {"LOW", "MEDIUM", "HIGH", "CRITICAL"},
		"crm_intelligence_scope_states_status_check":         {"NOT_AVAILABLE", "QUEUED", "RUNNING", "CURRENT", "STALE", "FAILED"},
		"crm_exports_export_type_check":                      {"CUSTOMER_SUMMARY", "SEGMENT_MEMBERS", "CORPORATE_CUSTOMERS", "LOYALTY_SUMMARY", "CAMPAIGN_PERFORMANCE_SUMMARY", "INTELLIGENCE_SCORES"},
		"crm_exports_format_check":                           {"CSV", "JSONL"},
		"crm_exports_status_check":                           {"QUEUED", "RUNNING", "READY", "FAILED", "CANCELLED", "EXPIRED"},
		"crm_customer_intelligence_scores_recency_score_che": {"1", "5"},
	}
	for constraint, values := range vocabularies {
		var definition string
		if err = pool.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint
			WHERE conname LIKE $1 || '%'`, constraint).Scan(&definition); err != nil {
			t.Fatalf("read %s: %v", constraint, err)
		}
		for _, value := range values {
			if !strings.Contains(definition, value) {
				t.Fatalf("%s does not contain %s: %s", constraint, value, definition)
			}
		}
	}

	// No status anywhere means "waiting" ambiguously.
	for _, constraint := range []string{"crm_exports_status_check",
		"crm_intelligence_evaluations_status_check", "crm_customer_intelligence_scores_state_check"} {
		var definition string
		if err = pool.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint
			WHERE conname=$1`, constraint).Scan(&definition); err != nil {
			t.Fatalf("read %s: %v", constraint, err)
		}
		if strings.Contains(definition, "'PENDING'") {
			t.Fatalf("%s contains the ambiguous state PENDING: %s", constraint, definition)
		}
	}

	// A CURRENT score carries exactly its model's columns; anything else
	// carries none. This is the structural half of "unavailable is not zero".
	var payload string
	if err = pool.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint
		WHERE conname='crm_intelligence_score_payload_ck'`).Scan(&payload); err != nil {
		t.Fatalf("the score payload constraint is missing: %v", err)
	}
	for _, required := range []string{"recency_score", "risk_score", "composite_score", "risk_band"} {
		if !strings.Contains(payload, required) {
			t.Fatalf("the payload constraint does not pin %s: %s", required, payload)
		}
	}
	// Storing an unavailable score with a number in it must be impossible.
	if _, err = pool.Exec(ctx, `INSERT INTO crm_customer_intelligence_scores
		(id,business_id,scope,evaluation_id,generation,customer_profile_id,model_key,model_version,
		 definition_hash,state,risk_score,risk_band,source_as_of,computed_at)
		VALUES ('x','x','BUSINESS','x',1,'x','CHURN_V1',1,'0000000000000000','NOT_AVAILABLE',
		        90,'CRITICAL',now(),now())`); err == nil {
		t.Fatal("an unavailable score with a risk number was storable")
	}

	// READY requires a complete artifact. A partial export cannot be released.
	var ready string
	if err = pool.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint
		WHERE conname='crm_export_ready_ck'`).Scan(&ready); err != nil {
		t.Fatalf("the export READY constraint is missing: %v", err)
	}
	for _, required := range []string{"artifact_ref", "checksum_sha256", "artifact_bytes",
		"completed_at", "expires_at"} {
		if !strings.Contains(ready, required) {
			t.Fatalf("the READY constraint does not require %s: %s", required, ready)
		}
	}

	// Tenancy: every Section 8H child reaches its parent through the composite
	// key, so PostgreSQL refuses a score or an export pointing at another
	// business's profile, branch or staff member.
	for _, constraint := range []string{
		"crm_intelligence_evaluation_branch_fk", "crm_intelligence_scope_state_branch_fk",
		"crm_intelligence_scope_state_evaluation_fk", "crm_intelligence_score_evaluation_fk",
		"crm_intelligence_score_profile_fk", "crm_intelligence_score_branch_fk",
		"crm_export_branch_fk", "crm_export_staff_fk",
	} {
		var definition string
		if err = pool.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint
			WHERE conname=$1`, constraint).Scan(&definition); err != nil {
			t.Fatalf("read %s: %v", constraint, err)
		}
		if !strings.Contains(definition, "business_id") && !strings.Contains(definition, "vendor_id") {
			t.Fatalf("%s is not tenant-composite: %s", constraint, definition)
		}
	}

	// Uniqueness the domain depends on: one current score per relationship per
	// model per scope, and one generation number per scope.
	for _, index := range []string{
		"crm_intelligence_scores_current_uq", "crm_intelligence_scores_generation_uq",
		"crm_intelligence_evaluation_generation_uq", "crm_intelligence_scope_states_uq",
	} {
		var unique bool
		if err = pool.QueryRow(ctx, `SELECT indisunique FROM pg_index i
			JOIN pg_class c ON c.oid = i.indexrelid WHERE c.relname=$1`, index).Scan(&unique); err != nil || !unique {
			t.Fatalf("index %s unique=%t err=%v", index, unique, err)
		}
	}
	for _, index := range []string{
		"crm_intelligence_evaluations_claim_idx", "crm_intelligence_evaluations_scope_idx",
		"crm_intelligence_scope_states_status_idx", "crm_intelligence_scores_lookup_idx",
		"crm_intelligence_scores_distribution_idx", "crm_intelligence_scores_evaluation_idx",
		"crm_exports_business_idx", "crm_exports_status_idx", "crm_exports_requester_idx",
		"crm_exports_claim_idx", "crm_exports_expiry_idx",
	} {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE indexname=$1)`, index).Scan(&exists); err != nil || !exists {
			t.Fatalf("index %s exists=%t err=%v", index, exists, err)
		}
	}

	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_permissions WHERE code IN
		('vendor.crm.intelligence.read','vendor.crm.intelligence.rebuild','vendor.crm.exports.read',
		 'vendor.crm.exports.create','vendor.crm.exports.download','vendor.crm.exports.cancel')`).
		Scan(&permissions); err != nil || permissions != 6 {
		t.Fatalf("Section 8H permissions=%d err=%v", permissions, err)
	}
	// There is no permission to write a score. Intelligence is derived, and a
	// grant that let a staff member declare a customer high-value would make
	// every other score unfalsifiable.
	var writeScore int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_permissions
		WHERE code LIKE 'vendor.crm.intelligence.%' AND code NOT IN
		 ('vendor.crm.intelligence.read','vendor.crm.intelligence.rebuild')`).Scan(&writeScore); err != nil ||
		writeScore != 0 {
		t.Fatalf("an intelligence write permission exists count=%d err=%v", writeScore, err)
	}
	// Operational roles hold nothing, and finance cannot take customer data out.
	var operational int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_role_permissions rp
		JOIN vendor_permissions p ON p.id=rp.permission_id
		WHERE rp.role_id IN ('role_kitchen_pastry','role_shop_inventory','role_procurement','role_vendor_rider')
		  AND (p.code LIKE 'vendor.crm.intelligence.%' OR p.code LIKE 'vendor.crm.exports.%')`).
		Scan(&operational); err != nil || operational != 0 {
		t.Fatalf("operational roles hold Section 8H permissions count=%d err=%v", operational, err)
	}
	var financeExports int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_role_permissions rp
		JOIN vendor_permissions p ON p.id=rp.permission_id
		WHERE rp.role_id='role_finance' AND p.code IN
		 ('vendor.crm.exports.create','vendor.crm.exports.download','vendor.crm.intelligence.read')`).
		Scan(&financeExports); err != nil || financeExports != 0 {
		t.Fatalf("finance can take CRM customer data out count=%d err=%v", financeExports, err)
	}

	// Section 8H stores heuristics and job metadata, not personal data, not
	// money and not a model artefact.
	var suspicious int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema='public' AND table_name IN
		 ('crm_intelligence_evaluations','crm_intelligence_scope_states',
		  'crm_customer_intelligence_scores','crm_exports')
		  AND (column_name ~ '(email|phone|first_name|last_name|address|date_of_birth|password|device_token|government_id)'
			OR column_name ~ '(amount|balance|currency|kobo|minor_units|price|wallet|invoice)'
			OR column_name ~ '(probability|likelihood|confidence|weights|features|model_artifact|training)')`).
		Scan(&suspicious); err != nil || suspicious != 0 {
		t.Fatalf("Section 8H schema holds personal, monetary or ML columns count=%d err=%v", suspicious, err)
	}
	// No stored URL: a persisted download link would be a credential with no
	// expiry that survives every check the export row exists to enforce.
	var urlColumns int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema='public' AND table_name='crm_exports'
		  AND column_name ~ '(url|link|signed|token)'`).Scan(&urlColumns); err != nil || urlColumns != 0 {
		t.Fatalf("crm_exports stores a URL or token count=%d err=%v", urlColumns, err)
	}
	// Every score column that holds a number is an integer type, so nothing can
	// be read as a probability.
	var floats int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema='public' AND table_name='crm_customer_intelligence_scores'
		  AND data_type IN ('double precision','real','numeric')`).Scan(&floats); err != nil || floats != 0 {
		t.Fatalf("a score column is floating point count=%d err=%v", floats, err)
	}
}
