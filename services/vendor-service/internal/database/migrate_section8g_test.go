package database

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestSection8GMigrationsRollbackAndReapplyAsAPair proves the corporate CRM
// schema and its permissions install, roll back to the accepted Section 8F
// baseline, and reapply — and that rolling 8G back leaves Sections 5-8F intact,
// including the Section 8C interaction and task tables 8G extends in place.
//
// It also pins the boundary the section exists to hold: the schema can describe
// a commercial relationship with an organization and cannot describe an order,
// an invoice, credit or a budget.
func TestSection8GMigrationsRollbackAndReapplyAsAPair(t *testing.T) {
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping real-Postgres Section 8G replay")
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

	section8GTables := []string{
		"corporate_customer_code_counters", "corporate_customer_profiles",
		"corporate_customer_branch_scopes", "corporate_customer_locations",
		"corporate_customer_cost_centers", "corporate_customer_contacts",
		"corporate_customer_memberships",
	}

	rollbackUntilRemoved(t, ctx, pool, "000022_vendor_crm_corporate_customers")
	for _, table := range section8GTables {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || exists {
			t.Fatalf("Section 8G table %s survived rollback exists=%t err=%v", table, exists, err)
		}
	}
	// Sections 5-8F must be untouched by an 8G rollback.
	for _, table := range []string{"crm_campaigns", "crm_campaign_targets", "crm_loyalty_accounts",
		"crm_tasks", "crm_interactions", "crm_tags", "crm_segments", "crm_segment_memberships",
		"vendor_customer_profiles", "vendor_staff", "vendor_businesses"} {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || !exists {
			t.Fatalf("accepted baseline table %s damaged by an 8G rollback exists=%t err=%v", table, exists, err)
		}
	}
	// The Section 8C tables 8G extends in place must be back to their accepted
	// shape: no corporate columns, and both subject columns mandatory again.
	for _, column := range []string{"corporate_customer_id", "corporate_contact_id",
		"corporate_location_id", "corporate_cost_center_id"} {
		for _, table := range []string{"crm_interactions", "crm_tasks"} {
			var present bool
			if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns
				WHERE table_schema='public' AND table_name=$1 AND column_name=$2)`, table, column).Scan(&present); err != nil || present {
				t.Fatalf("%s.%s survived an 8G rollback present=%t err=%v", table, column, present, err)
			}
		}
	}
	for _, table := range []string{"crm_interactions", "crm_tasks"} {
		for _, column := range []string{"customer_profile_id", "original_profile_id"} {
			var nullable string
			if err = pool.QueryRow(ctx, `SELECT is_nullable FROM information_schema.columns
				WHERE table_schema='public' AND table_name=$1 AND column_name=$2`, table, column).Scan(&nullable); err != nil {
				t.Fatalf("read %s.%s nullability: %v", table, column, err)
			}
			if nullable != "NO" {
				t.Fatalf("%s.%s stayed nullable after an 8G rollback", table, column)
			}
		}
	}
	var permissions int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_permissions WHERE code LIKE 'vendor.crm.corporate.%'`).Scan(&permissions); err != nil || permissions != 0 {
		t.Fatalf("Section 8G permissions survived rollback count=%d err=%v", permissions, err)
	}
	// The five Section 8E campaign permissions plus the four Section 8F
	// execution permissions.
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_permissions WHERE code LIKE 'vendor.crm.campaigns.%'`).Scan(&permissions); err != nil || permissions != 9 {
		t.Fatalf("Section 8E/8F permissions damaged by an 8G rollback count=%d err=%v", permissions, err)
	}

	if err = ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("reapply Section 8G: %v", err)
	}
	for _, table := range section8GTables {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&exists); err != nil || !exists {
			t.Fatalf("reapplied table %s exists=%t err=%v", table, exists, err)
		}
	}

	// Exact vocabularies, enforced in SQL rather than only in Go.
	vocabularies := map[string][]string{
		"corporate_customer_profiles_organization_type_check": {"COMPANY", "INSTITUTION", "SCHOOL",
			"HOSPITAL", "HOTEL", "RESTAURANT", "RETAILER", "NGO", "ASSOCIATION", "GOVERNMENT", "OTHER"},
		"corporate_customer_profiles_status_check":                  {"PROSPECT", "ACTIVE", "ON_HOLD", "INACTIVE", "ARCHIVED"},
		"corporate_customer_profiles_scope_mode_check":              {"BUSINESS", "BRANCH_SET"},
		"corporate_customer_contacts_status_check":                  {"ACTIVE", "INACTIVE", "ARCHIVED"},
		"corporate_customer_memberships_status_check":               {"ACTIVE", "INACTIVE", "ARCHIVED"},
		"corporate_customer_locations_status_check":                 {"ACTIVE", "INACTIVE", "ARCHIVED"},
		"corporate_customer_cost_centers_status_check":              {"ACTIVE", "INACTIVE", "ARCHIVED"},
		"corporate_customer_locations_location_type_check":          {"HEADQUARTERS", "OFFICE", "BRANCH", "STORE", "WAREHOUSE", "SITE", "CAMPUS", "OTHER"},
		"corporate_customer_profiles_corporate_customer_code_check": {"CORP-"},
	}
	for constraint, values := range vocabularies {
		var definition string
		if err = pool.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname=$1`, constraint).Scan(&definition); err != nil {
			t.Fatalf("read %s: %v", constraint, err)
		}
		for _, value := range values {
			if !strings.Contains(definition, value) {
				t.Fatalf("%s does not contain %s: %s", constraint, value, definition)
			}
		}
	}

	// The corporate lifecycle stops at the relationship. A status describing a
	// commercial document would mean corporate ordering had leaked into 8G.
	var lifecycle string
	if err = pool.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint
		WHERE conname='corporate_customer_profiles_status_check'`).Scan(&lifecycle); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"ORDERED", "INVOICED", "SUSPENDED_FOR_CREDIT", "OVERDUE", "IN_ARREARS"} {
		if strings.Contains(lifecycle, "'"+forbidden+"'") {
			t.Fatalf("the corporate lifecycle contains commercial state %s: %s", forbidden, lifecycle)
		}
	}

	// A Section 8C row now names exactly one subject: an individual customer or
	// a corporate account, never both and never neither.
	for _, constraint := range []string{"crm_interaction_subject_ck", "crm_task_subject_ck"} {
		var definition string
		if err = pool.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname=$1`, constraint).Scan(&definition); err != nil {
			t.Fatalf("read %s: %v", constraint, err)
		}
		if !strings.Contains(definition, "customer_profile_id") || !strings.Contains(definition, "corporate_customer_id") {
			t.Fatalf("%s does not pin both subjects: %s", constraint, definition)
		}
	}

	// Tenancy: every Section 8G child reaches its parent through the composite
	// key, so PostgreSQL — not only the HTTP layer — refuses a contact, site,
	// cost centre, membership or activity row pointing at another business's or
	// another account's parent.
	tenantComposite := []string{
		"corporate_profile_manager_fk", "corporate_profile_creator_fk",
		"corporate_branch_scope_profile_fk", "corporate_branch_scope_branch_fk",
		"corporate_location_profile_fk", "corporate_location_creator_fk",
		"corporate_cost_center_profile_fk", "corporate_cost_center_location_fk",
		"corporate_cost_center_creator_fk", "corporate_contact_profile_fk",
		"corporate_contact_customer_fk", "corporate_contact_original_customer_fk",
		"corporate_contact_creator_fk", "corporate_membership_profile_fk",
		"corporate_membership_contact_fk", "corporate_membership_location_fk",
		"corporate_membership_cost_center_fk", "corporate_membership_creator_fk",
		"crm_interaction_corporate_fk", "crm_interaction_corporate_contact_fk",
		"crm_interaction_corporate_location_fk", "crm_interaction_corporate_cost_center_fk",
		"crm_task_corporate_fk", "crm_task_corporate_contact_fk",
		"crm_task_corporate_location_fk", "crm_task_corporate_cost_center_fk",
	}
	for _, constraint := range tenantComposite {
		var definition string
		if err = pool.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname=$1`, constraint).Scan(&definition); err != nil {
			t.Fatalf("read %s: %v", constraint, err)
		}
		if !strings.Contains(definition, "business_id") {
			t.Fatalf("%s is not tenant-composite: %s", constraint, definition)
		}
	}
	// The nested-resource keys additionally carry the account, so a contact,
	// site or cost centre from another account of the same vendor is refused.
	for _, constraint := range []string{
		"corporate_membership_contact_fk", "corporate_membership_location_fk",
		"corporate_membership_cost_center_fk", "corporate_cost_center_location_fk",
		"crm_interaction_corporate_contact_fk", "crm_task_corporate_contact_fk",
	} {
		var definition string
		if err = pool.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname=$1`, constraint).Scan(&definition); err != nil {
			t.Fatalf("read %s: %v", constraint, err)
		}
		if !strings.Contains(definition, "corporate_customer_id") {
			t.Fatalf("%s does not pin the owning account: %s", constraint, definition)
		}
	}

	// Uniqueness the domain depends on.
	for _, index := range []string{
		"corporate_contacts_live_person_uq", "corporate_contacts_primary_uq",
		"corporate_memberships_live_uq", "corporate_profiles_external_ref_uq",
	} {
		var unique bool
		if err = pool.QueryRow(ctx, `SELECT indisunique FROM pg_index i
			JOIN pg_class c ON c.oid = i.indexrelid WHERE c.relname=$1`, index).Scan(&unique); err != nil || !unique {
			t.Fatalf("index %s unique=%t err=%v", index, unique, err)
		}
	}

	// Indexes the hot paths depend on. A missing one turns an account list or a
	// corporate timeline into a sequential scan.
	for _, index := range []string{
		"corporate_profiles_status_idx", "corporate_profiles_name_idx",
		"corporate_profiles_manager_idx", "corporate_profiles_type_idx",
		"corporate_branch_scopes_branch_idx", "corporate_locations_status_idx",
		"corporate_cost_centers_status_idx", "corporate_cost_centers_location_idx",
		"corporate_contacts_status_idx", "corporate_contacts_profile_idx",
		"corporate_memberships_contact_idx", "corporate_memberships_location_idx",
		"corporate_memberships_cost_center_idx",
		"crm_interactions_corporate_timeline_idx", "crm_tasks_corporate_status_idx",
	} {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE indexname=$1)`, index).Scan(&exists); err != nil || !exists {
			t.Fatalf("index %s exists=%t err=%v", index, exists, err)
		}
	}

	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_permissions WHERE code IN
		('vendor.crm.corporate.read','vendor.crm.corporate.manage','vendor.crm.corporate.contacts.manage',
		 'vendor.crm.corporate.locations.manage','vendor.crm.corporate.cost_centers.manage')`).Scan(&permissions); err != nil || permissions != 5 {
		t.Fatalf("Section 8G permissions=%d err=%v", permissions, err)
	}
	// Operational roles with no account-management remit hold none of it.
	var operational int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_role_permissions rp
		JOIN vendor_permissions p ON p.id=rp.permission_id
		WHERE rp.role_id IN ('role_kitchen_pastry','role_vendor_rider')
		  AND p.code LIKE 'vendor.crm.corporate.%'`).Scan(&operational); err != nil || operational != 0 {
		t.Fatalf("operational roles hold corporate permissions count=%d err=%v", operational, err)
	}
	// Analyst is read-only over corporate accounts.
	var analystWrites int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_role_permissions rp
		JOIN vendor_permissions p ON p.id=rp.permission_id
		WHERE rp.role_id='role_analyst_auditor' AND p.code LIKE 'vendor.crm.corporate.%'
		  AND p.code <> 'vendor.crm.corporate.read'`).Scan(&analystWrites); err != nil || analystWrites != 0 {
		t.Fatalf("analyst holds a corporate write permission count=%d err=%v", analystWrites, err)
	}

	// Section 8G stores a relationship, not a ledger and not a contact card.
	// The absent columns are the boundary: no money, no commercial document, no
	// personal contact detail duplicated out of the Section 8A profile.
	var suspicious int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema='public' AND table_name IN
		 ('corporate_customer_profiles','corporate_customer_branch_scopes','corporate_customer_locations',
		  'corporate_customer_cost_centers','corporate_customer_contacts','corporate_customer_memberships')
		  AND (column_name ~ '(email|phone|first_name|last_name|password|date_of_birth|government_id|device_token)'
			OR column_name ~ '(budget|credit|invoice|order|payment|wallet|balance|amount|price|kobo|minor|currency|discount|tax)')`).
		Scan(&suspicious); err != nil || suspicious != 0 {
		t.Fatalf("the corporate schema holds personal or financial columns count=%d err=%v", suspicious, err)
	}
	// Nor is there a corporate ordering, invoicing or credit table hiding
	// alongside the relationship tables.
	var forbiddenTables int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables
		WHERE table_schema='public' AND table_name ~ '^corporate_.*(order|invoice|credit|budget|contract|quotation|payment|wallet)'`).
		Scan(&forbiddenTables); err != nil || forbiddenTables != 0 {
		t.Fatalf("a corporate commerce or finance table exists count=%d err=%v", forbiddenTables, err)
	}
}
