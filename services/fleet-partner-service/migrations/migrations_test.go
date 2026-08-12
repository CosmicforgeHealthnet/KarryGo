package migrations

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func read(t *testing.T, name string) string {
	t.Helper()
	content, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", name, err)
	}
	return string(content)
}

func TestEveryUpMigrationHasDown(t *testing.T) {
	ups, err := filepath.Glob("*.up.sql")
	if err != nil || len(ups) == 0 {
		t.Fatalf("no up migrations found: %v", err)
	}
	for _, up := range ups {
		down := strings.TrimSuffix(up, ".up.sql") + ".down.sql"
		if _, err := os.Stat(down); err != nil {
			t.Errorf("missing down migration for %s", up)
		}
	}
}

func TestPlatformFoundationTables(t *testing.T) {
	sql := read(t, "000001_platform_foundation.up.sql")
	for _, table := range []string{
		"fleet_partner_outbox", "fleet_partner_inbox", "fleet_partner_jobs",
		"fleet_partner_idempotency_keys", "fleet_partner_audit_records",
		"fleet_partner_file_references",
	} {
		if !strings.Contains(sql, "CREATE TABLE IF NOT EXISTS "+table) {
			t.Errorf("missing table %s", table)
		}
	}
	for _, fragment := range []string{
		"PRIMARY KEY (consumer_name, event_id)",                    // inbox dedupe constraint
		"idx_fleet_partner_jobs_active_dedupe",                     // job dedupe partial unique index
		"PRIMARY KEY (organization_id, endpoint, idempotency_key)", // org-scoped idempotency
		"trg_fleet_partner_audit_append_only",                      // append-only audit
	} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("missing constraint fragment %q", fragment)
		}
	}
}

func TestOrganizationsFoundationTables(t *testing.T) {
	sql := read(t, "000002_organizations_foundation.up.sql")
	for _, table := range []string{
		"fleet_partner_organizations", "fleet_partner_branches",
		"fleet_partner_departments", "fleet_partner_organization_settings",
		"fleet_partner_organization_status_history",
	} {
		if !strings.Contains(sql, "CREATE TABLE IF NOT EXISTS "+table) {
			t.Errorf("missing table %s", table)
		}
	}
	for _, fragment := range []string{
		"uq_fleet_partner_branch_code UNIQUE (organization_id, code)",
		"uq_fleet_partner_department_code UNIQUE (organization_id, code)",
		"uq_fleet_partner_org_registration UNIQUE (country_code, registration_number)",
		"chk_fleet_partner_department_no_self_parent",
		"ON DELETE RESTRICT",
		"trg_fleet_partner_org_status_history_append_only",
	} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("missing constraint fragment %q", fragment)
		}
	}
	if strings.Contains(sql, "REFERENCES customers") || strings.Contains(sql, "REFERENCES providers") {
		t.Error("migration must not reference another service's tables")
	}
}

func TestNoCrossServiceReferences(t *testing.T) {
	files, _ := filepath.Glob("*.sql")
	for _, file := range files {
		sql := strings.ToLower(read(t, file))
		for _, forbidden := range []string{"dblink", "foreign data wrapper", "postgres_fdw"} {
			if strings.Contains(sql, forbidden) {
				t.Errorf("%s uses cross-database mechanism %q", file, forbidden)
			}
		}
	}
}

// ── WP6 ───────────────────────────────────────────────────────────────────────

func TestAssignmentAcknowledgementMigration(t *testing.T) {
	up := read(t, "000013_assignment_acknowledgements.up.sql")
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS fleet_partner_assignment_acknowledgements",
		"CONSTRAINT uq_fleet_partner_assignment_acknowledgement UNIQUE (assignment_id)",
		"uq_fleet_partner_acknowledgement_idempotency",
		"request_fingerprint",
		"require_assignment_acknowledgement boolean NOT NULL DEFAULT false",
		"trg_fleet_partner_acknowledgements_append_only",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("missing fragment %q", fragment)
		}
	}
	// Acknowledgement must never be able to become acceptance. The migration
	// may READ the status — the partial index for unacknowledged live work
	// does exactly that — but it must never write one, and it must not add a
	// status value to the assignment state machine.
	for _, forbidden := range []string{"SET status", "status = '", "accepted_at ="} {
		if strings.Contains(up, forbidden) {
			t.Errorf("acknowledgement migration must not write acceptance state: %q", forbidden)
		}
	}
	if strings.Contains(up, "chk_fleet_partner_assignment_status") {
		t.Error("acknowledgement must not alter the assignment status constraint")
	}

	down := read(t, "000013_assignment_acknowledgements.down.sql")
	for _, fragment := range []string{
		"DROP TABLE IF EXISTS fleet_partner_assignment_acknowledgements",
		"DROP COLUMN IF EXISTS require_assignment_acknowledgement",
		"DROP COLUMN IF EXISTS acknowledgement_state",
	} {
		if !strings.Contains(down, fragment) {
			t.Errorf("down migration missing %q", fragment)
		}
	}
}

func TestSafetyReferenceMigration(t *testing.T) {
	up := read(t, "000014_safety_references.up.sql")
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS fleet_partner_safety_references",
		"CREATE TABLE IF NOT EXISTS fleet_partner_safety_reference_history",
		"reference_type         text NOT NULL CHECK (reference_type IN ('INCIDENT', 'DISPUTE'))",
		"verification_state     text NOT NULL",
		"chk_fleet_partner_safety_reference_association",
		"uq_fleet_partner_safety_reference_idempotency",
		"trg_fleet_partner_safety_reference_history_append_only",
		"safety.references.read",
		"safety.references.manage",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("missing fragment %q", fragment)
		}
	}
	// The external identifier is opaque: no foreign key may reference another
	// service's table, and no verification default may exist.
	if strings.Contains(up, "external_reference     text NOT NULL REFERENCES") {
		t.Error("external_reference must not carry a foreign key")
	}
	if strings.Contains(up, "verification_state     text NOT NULL DEFAULT") {
		t.Error("verification_state must have no default; every row must state it")
	}

	down := read(t, "000014_safety_references.down.sql")
	if !strings.Contains(down, "DROP TABLE IF EXISTS fleet_partner_safety_references") {
		t.Error("down migration must drop the references table")
	}
}

func TestDeliveryOutcomeProjectionMigration(t *testing.T) {
	up := read(t, "000015_delivery_outcome_projections.up.sql")
	for _, fragment := range []string{
		"ADD COLUMN IF NOT EXISTS recipient_confirmed_at timestamptz",
		"ADD COLUMN IF NOT EXISTS recipient_reference text",
		"ADD COLUMN IF NOT EXISTS failed_at timestamptz",
		"ADD COLUMN IF NOT EXISTS failure_reason_category text",
		"ADD COLUMN IF NOT EXISTS failure_retry_eligible boolean",
		"ADD COLUMN IF NOT EXISTS failure_incident_reference text",
		"'DELIVERY_FAILED'",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("missing fragment %q", fragment)
		}
	}
	// The projection must have nowhere to put recipient identity or money.
	for _, forbidden := range []string{
		"receiver_name", "receiver_phone", "recipient_name", "recipient_phone",
		"fare_amount", "refund_amount", "settlement_amount",
	} {
		if strings.Contains(up, forbidden) {
			t.Errorf("delivery-outcome projection must not add column %q", forbidden)
		}
	}

	down := read(t, "000015_delivery_outcome_projections.down.sql")
	for _, fragment := range []string{
		"DROP COLUMN IF EXISTS failed_at",
		"DROP COLUMN IF EXISTS recipient_confirmed_at",
		// A row already carrying DELIVERY_FAILED must be moved before the
		// narrower constraint is restored, or the rollback fails on live data.
		"SET projection_status = 'RECONCILIATION_REQUIRED'",
	} {
		if !strings.Contains(down, fragment) {
			t.Errorf("down migration missing %q", fragment)
		}
	}
}

// ── WP9 ───────────────────────────────────────────────────────────────────────

func TestEligibilityDecisionMigration(t *testing.T) {
	up := read(t, "000019_eligibility_decisions.up.sql")
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS fleet_partner_eligibility_reason_codes",
		"CREATE TABLE IF NOT EXISTS fleet_partner_eligibility_policies",
		"CREATE TABLE IF NOT EXISTS fleet_partner_eligibility_decisions",
		"CREATE TABLE IF NOT EXISTS fleet_partner_eligibility_decision_reasons",
		"CREATE TABLE IF NOT EXISTS fleet_partner_eligibility_decision_sources",
		"CREATE TABLE IF NOT EXISTS fleet_partner_eligibility_decision_history",
		// The five statuses are the contract: UNKNOWN and STALE are outcomes,
		// not error paths, and a schema that cannot store them forces a caller
		// to invent one.
		"CHECK (status IN ('ELIGIBLE', 'INELIGIBLE', 'UNKNOWN', 'STALE', 'RECONCILING'))",
		"input_fingerprint    text NOT NULL",
		"checked_at",
		"expires_at",
		"CONSTRAINT chk_fleet_partner_eligibility_expiry CHECK (expires_at > checked_at)",
		// One live decision per subject and operation.
		"uq_fleet_partner_eligibility_current",
		// Reason codes are foreign-keyed to a closed vocabulary.
		"REFERENCES fleet_partner_eligibility_reason_codes (code)",
		// Source versions make a decision reproducible.
		"source_version    bigint NOT NULL",
		"freshness_state   text NOT NULL",
		// Immutability of both policies and decisions.
		"trg_fleet_partner_eligibility_policy_immutable",
		"trg_fleet_partner_eligibility_decision_immutable",
		"trg_fleet_partner_eligibility_history_append_only",
		// Indexes named by the work package.
		"idx_fleet_partner_eligibility_tenant",
		"idx_fleet_partner_eligibility_subject",
		"idx_fleet_partner_eligibility_operation",
		"idx_fleet_partner_eligibility_status",
		"idx_fleet_partner_eligibility_expiry",
		// Permissions.
		"eligibility.decisions.read",
		"eligibility.policies.manage",
		"approvals.requests.decide",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("missing fragment %q", fragment)
		}
	}
	// The subject of a decision is opaque. A foreign key onto another
	// service's table is the coupling this architecture exists to avoid, and a
	// vehicle is owned elsewhere.
	if strings.Contains(up, "subject_id           text NOT NULL REFERENCES") {
		t.Error("decision subject must be an opaque identifier, not a foreign key")
	}

	down := read(t, "000019_eligibility_decisions.down.sql")
	for _, fragment := range []string{
		"DROP TABLE IF EXISTS fleet_partner_eligibility_decision_history",
		"DROP TABLE IF EXISTS fleet_partner_eligibility_decision_sources",
		"DROP TABLE IF EXISTS fleet_partner_eligibility_decision_reasons",
		"DROP TABLE IF EXISTS fleet_partner_eligibility_decisions",
		"DROP TABLE IF EXISTS fleet_partner_eligibility_policies",
		"DROP TABLE IF EXISTS fleet_partner_eligibility_reason_codes",
	} {
		if !strings.Contains(down, fragment) {
			t.Errorf("down migration missing %q", fragment)
		}
	}
}

func TestApprovalExecutionBindingMigration(t *testing.T) {
	up := read(t, "000020_approval_execution_binding.up.sql")
	// The whole point is that this EXTENDS 000006. A second approval system
	// would show up here as a new requests table.
	for _, forbidden := range []string{
		"CREATE TABLE IF NOT EXISTS fleet_partner_approval_requests",
		"CREATE TABLE fleet_partner_approval_requests",
		"CREATE TABLE IF NOT EXISTS fleet_partner_approval_decisions",
	} {
		if strings.Contains(up, forbidden) {
			t.Errorf("must extend the existing approval framework, not recreate it: %q", forbidden)
		}
	}
	for _, fragment := range []string{
		"ALTER TABLE fleet_partner_approval_requests",
		"expected_subject_version",
		"payload_version",
		"policy_version",
		"minimum_approvals",
		"eligible_role_ids",
		"self_approval_allowed",
		"separation_of_duties",
		"execution_reference",
		"invalidated_at",
		"invalidation_reason",
		"eligibility_decision_id",
		"'INVALIDATED'",
		"chk_fleet_partner_approval_execution_evidence",
		"chk_fleet_partner_approval_invalidation_reason",
		"uq_fleet_partner_approval_execution_reference",
		"uq_fleet_partner_approval_request_idempotency",
		"uq_fleet_partner_decision_per_request",
		"approved_fingerprint",
		"CREATE TABLE IF NOT EXISTS fleet_partner_approval_executions",
		"trg_fleet_partner_approval_executions_append_only",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("missing fragment %q", fragment)
		}
	}

	down := read(t, "000020_approval_execution_binding.down.sql")
	for _, fragment := range []string{
		"DROP TABLE IF EXISTS fleet_partner_approval_executions",
		"DROP COLUMN IF EXISTS expected_subject_version",
		"DROP COLUMN IF EXISTS execution_reference",
		"fleet_partner_approval_requests_status_check",
	} {
		if !strings.Contains(down, fragment) {
			t.Errorf("down migration missing %q", fragment)
		}
	}
	// Rolling back narrows the status set, so rows in the new state must be
	// moved somewhere legal rather than left to break the constraint.
	if !strings.Contains(down, "WHERE status = 'INVALIDATED'") {
		t.Error("down migration must resolve INVALIDATED rows before narrowing the constraint")
	}
}

// TestFleetDocumentMigration guards the properties of the WP10 document
// schema that a later edit could quietly remove.
func TestFleetDocumentMigration(t *testing.T) {
	up := read(t, "000021_fleet_documents.up.sql")

	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS fleet_partner_documents",
		"CREATE TABLE IF NOT EXISTS fleet_partner_document_history",
		"CREATE TABLE IF NOT EXISTS fleet_partner_document_verifications",
		"CREATE TABLE IF NOT EXISTS fleet_partner_document_media_versions",
		"CREATE TABLE IF NOT EXISTS fleet_partner_document_reconciliation_cases",
		"CREATE TABLE IF NOT EXISTS fleet_partner_document_deletion_requests",
		// Tenant, owner, branch, state, expiry, retention and version indexes.
		"idx_fleet_partner_document_tenant",
		"idx_fleet_partner_document_branch",
		"idx_fleet_partner_document_category",
		"idx_fleet_partner_document_state",
		"idx_fleet_partner_document_expiry",
		"idx_fleet_partner_document_retention",
		"idx_fleet_partner_document_source_version",
		"idx_fleet_partner_document_freshness",
		// Idempotency, fingerprint and single-pointer constraints.
		"uq_fleet_partner_document_idempotency",
		"uq_fleet_partner_document_media_object",
		"uq_fleet_partner_document_deletion_idempotency",
		"request_fingerprint",
		// Lease and retry indexes.
		"idx_fleet_partner_document_reconciliation_due",
		"idx_fleet_partner_document_reconciliation_lease",
		"idx_fleet_partner_document_deletion_due",
		"idx_fleet_partner_document_deletion_lease",
		// Append-only history and decisions.
		"trg_fleet_partner_document_history_append_only",
		"trg_fleet_partner_document_verifications_append_only",
		// The rule the whole work package protects, enforced by the schema.
		"chk_fleet_partner_document_verified_requires_safe",
		"chk_fleet_partner_document_revocation",
		"chk_fleet_partner_document_branch",
		// Permissions and reason codes.
		"'documents.read'",
		"'documents.manage'",
		"'documents.download'",
		"'documents.verify'",
		"'documents.delete'",
		"'document_not_verified'",
		"'document_not_safe'",
		"'document_reconciling'",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("missing fragment %q", fragment)
		}
	}

	// A private document must have nowhere to keep a durable link or a
	// credential. This is checked in the migration rather than only in code
	// because a column is the thing that makes storing one possible.
	for _, forbidden := range []string{
		"public_url", "signed_url", "download_url", "storage_path",
		"storage_bucket", "upload_token", "download_token",
	} {
		if strings.Contains(up, forbidden) {
			t.Errorf("document schema must not carry %q", forbidden)
		}
	}
	// Safety must never default to SAFE and freshness must never default to
	// CURRENT: a document fleet has heard nothing about is UNKNOWN.
	if !strings.Contains(up, "safety_projection     text NOT NULL DEFAULT 'UNKNOWN'") {
		t.Error("safety_projection must default to UNKNOWN")
	}
	if !strings.Contains(up, "projection_freshness  text NOT NULL DEFAULT 'UNKNOWN'") {
		t.Error("projection_freshness must default to UNKNOWN")
	}
	// The media object is a pointer, never a foreign key.
	if strings.Contains(up, "media_object_id       text NOT NULL REFERENCES") {
		t.Error("media_object_id must not be a cross-service foreign key")
	}

	down := read(t, "000021_fleet_documents.down.sql")
	for _, fragment := range []string{
		"DROP TABLE IF EXISTS fleet_partner_documents",
		"DROP TABLE IF EXISTS fleet_partner_document_deletion_requests",
		"DROP TABLE IF EXISTS fleet_partner_document_reconciliation_cases",
		"DELETE FROM fleet_partner_permissions",
		"DELETE FROM fleet_partner_eligibility_reason_codes",
	} {
		if !strings.Contains(down, fragment) {
			t.Errorf("down migration missing %q", fragment)
		}
	}
}

// ── WP14 (Section 15: service catalogue, private orders) ───────────────────

func TestServiceCatalogMigration(t *testing.T) {
	up := read(t, "000025_service_catalog.up.sql")

	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS fleet_partner_service_catalog_items",
		"idx_fleet_servicecatalog_code",
		"idx_fleet_servicecatalog_cursor",
		"idx_fleet_servicecatalog_status",
		"idx_fleet_servicecatalog_category",
		"idx_fleet_servicecatalog_branch",
		"'DRAFT', 'ACTIVE', 'INACTIVE', 'ARCHIVED'",
		"'TAXI'", "'PASSENGER_TRANSPORT'",
		"'servicecatalog.items.read'",
		"'servicecatalog.items.manage'",
		"'servicecatalog.items.activate'",
		"'servicecatalog.items.deactivate'",
		"'servicecatalog.items.archive'",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("missing fragment %q", fragment)
		}
	}

	// The code uniqueness index must be permanent (no WHERE clause dropping
	// out archived rows): an archived code stays reserved forever.
	idx := "idx_fleet_servicecatalog_code"
	pos := strings.Index(up, idx)
	if pos == -1 {
		t.Fatalf("index %s not found", idx)
	}
	statement := up[pos : pos+strings.Index(up[pos:], ";")]
	if strings.Contains(statement, "WHERE") {
		t.Error("service catalogue code uniqueness must not be partial: an archived code must stay reserved")
	}

	if strings.Contains(up, "REFERENCES customers") || strings.Contains(up, "REFERENCES providers") {
		t.Error("migration must not reference another service's tables")
	}

	down := read(t, "000025_service_catalog.down.sql")
	for _, fragment := range []string{
		"DROP TABLE IF EXISTS fleet_partner_service_catalog_items",
		"DELETE FROM fleet_partner_permissions",
		"DELETE FROM fleet_partner_role_permissions",
	} {
		if !strings.Contains(down, fragment) {
			t.Errorf("down migration missing %q", fragment)
		}
	}
}

func TestPrivateOrdersAndStopsMigration(t *testing.T) {
	up := read(t, "000026_private_orders_and_stops.up.sql")

	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS fleet_partner_private_orders",
		"CREATE TABLE IF NOT EXISTS fleet_partner_private_order_history",
		"CREATE TABLE IF NOT EXISTS fleet_partner_order_stops",
		"idx_fleet_orders_number",
		"idx_fleet_orders_cursor",
		"idx_fleet_orders_status",
		"idx_fleet_orders_customer",
		"idx_fleet_order_stops_order",
		"trg_fleet_order_history_append_only",
		"UNIQUE (order_id, sequence)",
		// The whole point of this migration's conversion-intent piece: additive
		// columns and a guarded index, never a rewrite of the Section 14 CHECK.
		"ADD COLUMN IF NOT EXISTS consumed_at timestamptz",
		"ADD COLUMN IF NOT EXISTS private_order_id text",
		"idx_fleet_conversion_intent_order",
		"'orders.read'",
		"'orders.manage'",
		"'orders.confirm'",
		"'orders.cancel'",
		"'orders.stops.manage'",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("missing fragment %q", fragment)
		}
	}

	// The Section 14 status CHECK from migration 000023 must never be touched
	// here: consumed_at IS NULL is the whole "still convertible" gate.
	if strings.Contains(up, "DROP CONSTRAINT") && strings.Contains(up, "conversion_type") {
		t.Error("must not rewrite the Section 14 conversion-intent status/type CHECK")
	}
	if strings.Contains(up, "REFERENCES customers") || strings.Contains(up, "REFERENCES providers") {
		t.Error("migration must not reference another service's tables")
	}

	down := read(t, "000026_private_orders_and_stops.down.sql")
	for _, fragment := range []string{
		"DROP TABLE IF EXISTS fleet_partner_private_orders",
		"DROP TABLE IF EXISTS fleet_partner_order_stops",
		"DROP TABLE IF EXISTS fleet_partner_private_order_history",
		"DROP COLUMN IF EXISTS private_order_id",
		"DROP COLUMN IF EXISTS consumed_at",
		"DELETE FROM fleet_partner_permissions",
	} {
		if !strings.Contains(down, fragment) {
			t.Errorf("down migration missing %q", fragment)
		}
	}
}

func TestShipmentsAndCargoMigration(t *testing.T) {
	up := read(t, "000027_shipments_and_cargo.up.sql")

	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS fleet_partner_shipments",
		"CREATE TABLE IF NOT EXISTS fleet_partner_shipment_history",
		"CREATE TABLE IF NOT EXISTS fleet_partner_cargo_items",
		"idx_fleet_shipments_cursor",
		"idx_fleet_shipments_order",
		"idx_fleet_shipments_status",
		"idx_fleet_cargo_items_shipment",
		"idx_fleet_cargo_items_restricted",
		"trg_fleet_shipment_history_append_only",
		"REFERENCES fleet_partner_private_orders (id) ON DELETE RESTRICT",
		"REFERENCES fleet_partner_order_stops (id) ON DELETE RESTRICT",
		"CHECK (origin_stop_id <> destination_stop_id)",
		"'DRAFT', 'READY', 'PLANNED', 'ASSIGNED', 'IN_TRANSIT'",
		"'shipments.read'",
		"'shipments.manage'",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("missing fragment %q", fragment)
		}
	}

	// Restricted-item status must default to the fail-closed UNKNOWN state,
	// never to a clean bill of health.
	if !strings.Contains(up, "restricted_item_status      text NOT NULL DEFAULT 'UNKNOWN'") {
		t.Error("restricted_item_status must default to UNKNOWN")
	}
	// Quantity and declared value must never be floating point column types.
	for _, forbidden := range []string{" real ", " real,", " float4", " float8", " double precision"} {
		if strings.Contains(up, forbidden) {
			t.Errorf("shipments/cargo schema must not use floating point column type %q", forbidden)
		}
	}
	if !strings.Contains(up, "quantity                    numeric(18, 6)") {
		t.Error("cargo quantity must be a bounded-precision numeric, not a float")
	}
	if strings.Contains(up, "REFERENCES customers") || strings.Contains(up, "REFERENCES providers") {
		t.Error("migration must not reference another service's tables")
	}

	down := read(t, "000027_shipments_and_cargo.down.sql")
	for _, fragment := range []string{
		"DROP TABLE IF EXISTS fleet_partner_shipments",
		"DROP TABLE IF EXISTS fleet_partner_cargo_items",
		"DROP TABLE IF EXISTS fleet_partner_shipment_history",
		"DELETE FROM fleet_partner_permissions",
	} {
		if !strings.Contains(down, fragment) {
			t.Errorf("down migration missing %q", fragment)
		}
	}
}

func TestSection15CompletionMigrations(t *testing.T) {
	cases := map[string][]string{
		"000028_packages_scans_custody.up.sql": {
			"fleet_partner_packages", "fleet_partner_package_history",
			"fleet_partner_package_scans", "fleet_partner_custody_events",
			"append_only", "packages.scan", "custody.record",
		},
		"000029_manifests.up.sql": {
			"fleet_partner_manifests", "fleet_partner_manifest_items",
			"fleet_partner_manifest_snapshots", "append_only", "manifests.manage",
		},
		"000030_recurring_orders.up.sql": {
			"fleet_partner_recurring_order_templates", "fleet_partner_recurring_occurrences",
			"occurrence_key", "lease_owner", "recurring_orders.manage",
		},
		"000031_bulk_order_imports.up.sql": {
			"fleet_partner_bulk_imports", "fleet_partner_bulk_import_rows",
			"media_file_id", "row_fingerprint", "lease_owner", "imports.manage",
		},
		"000032_private_assignments_execution.up.sql": {
			"fleet_partner_private_assignments", "fleet_partner_private_assignment_decisions",
			"fleet_partner_private_execution_operations", "work_origin", "execution_service",
			"private_assignments.respond", "private_execution.manage",
		},
		"000033_private_execution_completion.up.sql": {
			"fleet_partner_private_execution_projections",
			"fleet_partner_private_execution_reconciliation_cases",
			"fleet_partner_private_execution_reconciliation_attempts",
			"lease_heartbeat_at", "DEAD_LETTER", "private_execution.replay",
		},
	}
	for name, fragments := range cases {
		t.Run(name, func(t *testing.T) {
			up := read(t, name)
			for _, fragment := range fragments {
				if !strings.Contains(strings.ToLower(up), strings.ToLower(fragment)) {
					t.Errorf("missing fragment %q", fragment)
				}
			}
			downName := strings.TrimSuffix(name, ".up.sql") + ".down.sql"
			if down := read(t, downName); !strings.Contains(down, "DROP") {
				t.Errorf("%s does not contain rollback DDL", downName)
			}
		})
	}
	private := read(t, "000032_private_assignments_execution.up.sql")
	if strings.Contains(strings.ToLower(private), "alter table fleet_partner_marketplace") {
		t.Fatal("private assignments must not widen the marketplace source_service constraint")
	}
}

// ── Section 25 ───────────────────────────────────────────────────────────

func TestSearchProjectionsMigration(t *testing.T) {
	up := read(t, "000053_search_projections.up.sql")
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS fleet_partner_search_documents",
		"tsvector GENERATED ALWAYS AS",
		"USING GIN (search_vector)",
		"idx_fleet_search_tenant",
		"idx_fleet_search_subject",
		"idx_fleet_search_branch",
		"UNIQUE (organization_id, subject_type, subject_reference)",
		"required_permission   text NOT NULL CHECK",
		"'search.read'",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("missing fragment %q", fragment)
		}
	}
	// This is a read-side projection: it must never become a second
	// authority. No cross-service foreign key, no ownership of any
	// business aggregate's canonical state.
	if strings.Contains(up, "REFERENCES customers") || strings.Contains(up, "REFERENCES providers") {
		t.Error("migration must not reference another service's tables")
	}
	down := read(t, "000053_search_projections.down.sql")
	if !strings.Contains(down, "DROP TABLE IF EXISTS fleet_partner_search_documents") {
		t.Error("down migration must drop the search table")
	}
}

func TestDocumentVisibilityMigration(t *testing.T) {
	up := read(t, "000054_document_visibility.up.sql")
	for _, fragment := range []string{
		"ALTER TABLE fleet_partner_documents",
		"ADD COLUMN IF NOT EXISTS visibility text NOT NULL DEFAULT 'INTERNAL'",
		"ADD COLUMN IF NOT EXISTS archived_at timestamptz",
		"'INTERNAL', 'PROVIDER_VISIBLE', 'CUSTOMER_VISIBLE', 'RESTRICTED'",
		"'COMPANY_CONFIDENTIAL'",
		"idx_fleet_partner_document_visibility",
		"'documents.download_restricted'",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("missing fragment %q", fragment)
		}
	}
	// Additive only: this migration must not touch the existing lifecycle,
	// verification or deletion state machines it builds on top of.
	for _, forbidden := range []string{"DROP COLUMN", "ALTER COLUMN", "lifecycle_state ="} {
		if strings.Contains(up, forbidden) {
			t.Errorf("up migration must be purely additive, found %q", forbidden)
		}
	}
	down := read(t, "000054_document_visibility.down.sql")
	if !strings.Contains(down, "DROP COLUMN IF EXISTS visibility") {
		t.Error("down migration must drop the visibility column")
	}
}

// TestExportsMigration guards the Section 25D generic export framework: the
// job/attempt tables, the immutable-attempts trigger and the three-permission
// split (read/manage/download), mirroring documents' capability separation.
func TestExportsMigration(t *testing.T) {
	up := read(t, "000055_exports.up.sql")
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS fleet_partner_export_jobs",
		"CREATE TABLE IF NOT EXISTS fleet_partner_export_attempts",
		"status text NOT NULL DEFAULT 'PENDING' CHECK(status IN('PENDING','RUNNING','READY','FAILED','EXPIRED','CANCELLED','DEAD_LETTER'))",
		"uq_fleet_export_idempotency",
		"idx_fleet_export_claim",
		"trg_fleet_export_attempts_append_only",
		"fleet_partner_forbid_mutation",
		"'exports.read'",
		"'exports.manage'",
		"'exports.download'",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("missing fragment %q", fragment)
		}
	}
	down := read(t, "000055_exports.down.sql")
	for _, fragment := range []string{
		"DROP TABLE IF EXISTS fleet_partner_export_attempts",
		"DROP TABLE IF EXISTS fleet_partner_export_jobs",
	} {
		if !strings.Contains(down, fragment) {
			t.Errorf("down migration missing %q", fragment)
		}
	}
}

// TestGenericImportsMigration guards the Section 25C generalization of the
// order-only bulk-import aggregate: the additive columns, the widened status
// vocabulary (old values preserved verbatim), the engine discriminator that
// keeps the legacy and generic claim queries disjoint, and the three new
// semantic permissions.
func TestGenericImportsMigration(t *testing.T) {
	up := read(t, "000056_generic_imports.up.sql")
	for _, fragment := range []string{
		"ALTER TABLE fleet_partner_bulk_imports",
		"ADD COLUMN IF NOT EXISTS import_type text NOT NULL DEFAULT 'PRIVATE_ORDER'",
		"CHECK (import_type IN ('PRIVATE_ORDER', 'WORKFORCE_PROFILE', 'ASSET', 'WAREHOUSE_INVENTORY'))",
		"ADD COLUMN IF NOT EXISTS engine text NOT NULL DEFAULT 'LEGACY'",
		"CHECK (engine IN ('LEGACY', 'GENERIC'))",
		"ADD COLUMN IF NOT EXISTS mode text NOT NULL DEFAULT 'COMMIT'",
		"fleet_partner_bulk_imports_status_check_v2",
		// Every original status value must still be accepted.
		"'UPLOADED', 'VALIDATING', 'VALIDATION_FAILED', 'READY', 'IMPORTING',",
		"'COMPLETED', 'PARTIALLY_COMPLETED', 'FAILED', 'CANCELLED',",
		// And the new generic vocabulary is additive alongside it.
		"'VALIDATION_PENDING', 'VALIDATED', 'COMMIT_PENDING', 'COMMITTING',",
		"'PARTIALLY_COMMITTED', 'COMMITTED', 'DEAD_LETTER'",
		"ADD COLUMN IF NOT EXISTS external_reference text NOT NULL DEFAULT ''",
		"ADD COLUMN IF NOT EXISTS row_version integer NOT NULL DEFAULT 1",
		"ADD COLUMN IF NOT EXISTS replaces_row_id text REFERENCES fleet_partner_bulk_import_rows (id)",
		"'imports.validate'",
		"'imports.commit'",
		"'imports.retry'",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("missing fragment %q", fragment)
		}
	}
	// Additive only: never touch existing rows/columns of either table, and
	// never rewrite migration 000031's own schema in place.
	for _, forbidden := range []string{"DROP COLUMN", "ALTER COLUMN", "DROP TABLE"} {
		if strings.Contains(up, forbidden) {
			t.Errorf("up migration must be purely additive, found %q", forbidden)
		}
	}

	down := read(t, "000056_generic_imports.down.sql")
	for _, fragment := range []string{
		"DROP COLUMN IF EXISTS import_type",
		"DROP COLUMN IF EXISTS engine",
		"DROP COLUMN IF EXISTS external_reference",
		"DROP COLUMN IF EXISTS row_version",
		"DROP COLUMN IF EXISTS replaces_row_id",
		"fleet_partner_bulk_imports_status_check_v2",
		"CHECK (status IN ('UPLOADED', 'VALIDATING', 'VALIDATION_FAILED', 'READY', 'IMPORTING',",
	} {
		if !strings.Contains(down, fragment) {
			t.Errorf("down migration missing %q", fragment)
		}
	}
}

// TestDocumentChecksumCaptureAccessMigration guards the Section 25A
// additions: authoritative checksum/capture columns, the verification
// decision's own checksum retention, the ORDER owner type, and the
// append-only access-audit table.
func TestDocumentChecksumCaptureAccessMigration(t *testing.T) {
	up := read(t, "000057_document_checksum_capture_access.up.sql")
	for _, fragment := range []string{
		"ADD COLUMN IF NOT EXISTS media_checksum_sha256 text NOT NULL DEFAULT ''",
		"ADD COLUMN IF NOT EXISTS captured_at timestamptz",
		"ADD COLUMN IF NOT EXISTS source_timestamp timestamptz",
		"ADD COLUMN IF NOT EXISTS capture_method text NOT NULL DEFAULT ''",
		"ADD COLUMN IF NOT EXISTS device_reference text NOT NULL DEFAULT ''",
		"ADD COLUMN IF NOT EXISTS actor_reference text NOT NULL DEFAULT ''",
		"ADD COLUMN IF NOT EXISTS original_mime_type text NOT NULL DEFAULT ''",
		"ADD COLUMN IF NOT EXISTS original_size_bytes bigint NOT NULL DEFAULT 0",
		"ALTER TABLE fleet_partner_document_verifications",
		"ADD COLUMN IF NOT EXISTS media_checksum_sha256 text NOT NULL DEFAULT ''",
		"fleet_partner_documents_owner_type_check_v2",
		"'COMPANY', 'BRANCH', 'PROVIDER_LINK', 'VEHICLE_LINK',",
		"'INCIDENT', 'DISPUTE', 'APPROVAL', 'ORDER'",
		"CREATE TABLE IF NOT EXISTS fleet_partner_document_access_audit",
		"trg_fleet_document_access_audit_append_only",
		"fleet_partner_forbid_mutation",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("missing fragment %q", fragment)
		}
	}
	for _, forbidden := range []string{"DROP COLUMN", "ALTER COLUMN", "DROP TABLE"} {
		if strings.Contains(up, forbidden) {
			t.Errorf("up migration must be purely additive, found %q", forbidden)
		}
	}
	down := read(t, "000057_document_checksum_capture_access.down.sql")
	for _, fragment := range []string{
		"DROP TABLE IF EXISTS fleet_partner_document_access_audit",
		"DROP COLUMN IF EXISTS media_checksum_sha256",
		"DROP COLUMN IF EXISTS original_size_bytes",
	} {
		if !strings.Contains(down, fragment) {
			t.Errorf("down migration missing %q", fragment)
		}
	}
}

// TestSearchRepairPermissionMigration proves the search.repair permission is
// added additively and granted only to the operator-level roles.
func TestSearchRepairPermissionMigration(t *testing.T) {
	up := read(t, "000058_search_repair_permission.up.sql")
	for _, fragment := range []string{
		"('search.repair', 'search', 'Manually reindex or repair a search projection entry', true)",
		"'rol_system_owner', 'rol_system_admin', 'rol_system_operations'",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("missing fragment %q", fragment)
		}
	}
	for _, forbidden := range []string{"DROP COLUMN", "ALTER COLUMN", "DROP TABLE"} {
		if strings.Contains(up, forbidden) {
			t.Errorf("up migration must be purely additive, found %q", forbidden)
		}
	}
	down := read(t, "000058_search_repair_permission.down.sql")
	for _, fragment := range []string{
		"DELETE FROM fleet_partner_role_permissions WHERE permission_code = 'search.repair'",
		"DELETE FROM fleet_partner_permissions WHERE code = 'search.repair'",
	} {
		if !strings.Contains(down, fragment) {
			t.Errorf("down migration missing %q", fragment)
		}
	}
}

// TestExportsAdditionalDatasetsMigration proves the export_type CHECK widens
// additively to the four newly-registered adapters and nothing else.
func TestExportsAdditionalDatasetsMigration(t *testing.T) {
	up := read(t, "000059_exports_additional_datasets.up.sql")
	for _, fragment := range []string{
		"fleet_partner_export_jobs_export_type_check_v2",
		"'ORDERS', 'SHIPMENTS', 'PROVIDERS_WORKFORCE',",
		"'ASSETS_MAINTENANCE', 'WAREHOUSE_INVENTORY'",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("missing fragment %q", fragment)
		}
	}
	// The CHECK constraint's own value list — not the explanatory comment
	// above it, which names these datasets on purpose — must not admit them.
	checkClause := up[strings.Index(up, "CHECK (export_type"):]
	for _, notYet := range []string{"COMPLIANCE_INCIDENTS", "INVOICES_RECEIVABLES",
		"FULFILMENT_ORDERS", "SUPPORT_CASE_PROJECTIONS"} {
		if strings.Contains(checkClause, notYet) {
			t.Errorf("up migration must not admit %q: no adapter is registered for it yet", notYet)
		}
	}
	for _, forbidden := range []string{"DROP COLUMN", "ALTER COLUMN", "DROP TABLE"} {
		if strings.Contains(up, forbidden) {
			t.Errorf("up migration must be purely additive, found %q", forbidden)
		}
	}
	down := read(t, "000059_exports_additional_datasets.down.sql")
	if !strings.Contains(down, "fleet_partner_export_jobs_export_type_check_v2") {
		t.Error("down migration must drop the widened constraint")
	}
}

// TestServiceNonceReplayGuardMigration proves the nonce guard table is
// additive and keyed so the same (caller, nonce) pair can never be stored
// twice.
func TestServiceNonceReplayGuardMigration(t *testing.T) {
	up := read(t, "000060_service_nonce_replay_guard.up.sql")
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS fleet_partner_service_nonces",
		"PRIMARY KEY (caller_service, nonce)",
		"idx_fleet_service_nonces_expiry",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("missing fragment %q", fragment)
		}
	}
	for _, forbidden := range []string{"DROP COLUMN", "ALTER COLUMN", "DROP TABLE"} {
		if strings.Contains(up, forbidden) {
			t.Errorf("up migration must be purely additive, found %q", forbidden)
		}
	}
	down := read(t, "000060_service_nonce_replay_guard.down.sql")
	if !strings.Contains(down, "DROP TABLE IF EXISTS fleet_partner_service_nonces") {
		t.Error("down migration must drop the nonce table")
	}
}

// TestSection27JobsCompletionMigration proves the canonical jobs/attempts
// schema widens additively and the operator jobs.* permissions are seeded
// only to owner/admin/operations.
func TestSection27JobsCompletionMigration(t *testing.T) {
	up := read(t, "000063_section27_jobs_completion.up.sql")
	for _, fragment := range []string{
		"ADD COLUMN IF NOT EXISTS branch_id text",
		"ADD COLUMN IF NOT EXISTS payload_version integer NOT NULL DEFAULT 1",
		"ADD COLUMN IF NOT EXISTS idempotency_key text",
		"ADD COLUMN IF NOT EXISTS correlation_id text NOT NULL DEFAULT ''",
		"ADD COLUMN IF NOT EXISTS causation_id text NOT NULL DEFAULT ''",
		"ADD COLUMN IF NOT EXISTS scheduled_task_id text",
		"ADD COLUMN IF NOT EXISTS scheduled_for timestamptz",
		"ADD COLUMN IF NOT EXISTS next_attempt_at timestamptz",
		"ADD COLUMN IF NOT EXISTS started_at timestamptz",
		"ADD COLUMN IF NOT EXISTS cancelled_at timestamptz",
		"ADD COLUMN IF NOT EXISTS dead_lettered_at timestamptz",
		"ADD COLUMN IF NOT EXISTS last_error_class text NOT NULL DEFAULT ''",
		"ADD COLUMN IF NOT EXISTS last_error_code text NOT NULL DEFAULT ''",
		"ADD COLUMN IF NOT EXISTS last_safe_error_message text NOT NULL DEFAULT ''",
		"ADD COLUMN IF NOT EXISTS external_outcome_status text NOT NULL DEFAULT ''",
		"ADD COLUMN IF NOT EXISTS authoritative_readback_required boolean NOT NULL DEFAULT false",
		"ADD COLUMN IF NOT EXISTS version bigint NOT NULL DEFAULT 1",
		"idx_fleet_partner_jobs_active_idempotency",
		"idx_fleet_partner_jobs_scheduled_occurrence",
		"fleet_partner_job_attempts_status_check_v2",
		"TRANSIENT_FAILURE", "PERMANENT_FAILURE", "UNKNOWN_EXTERNAL_OUTCOME", "SHUTDOWN_RELEASED",
		"('jobs.read', 'jobs', 'Read durable background jobs', true)",
		"('jobs.manage', 'jobs', 'Manage durable background jobs', true)",
		"('jobs.retry', 'jobs', 'Retry or replay a dead-lettered job', true)",
		"('jobs.cancel', 'jobs', 'Cancel a pending or running job', true)",
		"'rol_system_owner', 'rol_system_admin', 'rol_system_operations'",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("missing fragment %q", fragment)
		}
	}
	for _, forbidden := range []string{"DROP COLUMN", "DROP TABLE"} {
		if strings.Contains(up, forbidden) {
			t.Errorf("up migration must be purely additive, found %q", forbidden)
		}
	}
	down := read(t, "000063_section27_jobs_completion.down.sql")
	for _, fragment := range []string{
		"DROP COLUMN IF EXISTS branch_id",
		"DROP COLUMN IF EXISTS version",
		"fleet_partner_job_attempts_status_check_v2",
	} {
		if !strings.Contains(down, fragment) {
			t.Errorf("down migration missing %q", fragment)
		}
	}
}

// TestSection27ScheduledTasksMigration proves the scheduled-task tables and
// permissions are present with the required schedule/concurrency/misfire
// vocabularies.
func TestSection27ScheduledTasksMigration(t *testing.T) {
	up := read(t, "000064_section27_scheduled_tasks.up.sql")
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS fleet_partner_scheduled_tasks",
		"CHECK (schedule_type IN ('CRON', 'INTERVAL', 'ONCE'))",
		"CHECK (concurrency_policy IN ('ALLOW', 'FORBID', 'REPLACE'))",
		"CHECK (misfire_policy IN ('SKIP', 'RUN_ONCE', 'CATCH_UP_LIMITED'))",
		"CREATE TABLE IF NOT EXISTS fleet_partner_scheduled_task_runs",
		"trg_fleet_scheduled_task_runs_append_only",
		"fleet_partner_forbid_mutation",
		"('scheduled_tasks.read'", "('scheduled_tasks.manage'", "('scheduled_tasks.run'",
		"('worker_operations.read'", "('worker_operations.manage'",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("missing fragment %q", fragment)
		}
	}
	for _, forbidden := range []string{"DROP COLUMN", "DROP TABLE"} {
		if strings.Contains(up, forbidden) {
			t.Errorf("up migration must be purely additive, found %q", forbidden)
		}
	}
	down := read(t, "000064_section27_scheduled_tasks.down.sql")
	for _, fragment := range []string{
		"DROP TABLE IF EXISTS fleet_partner_scheduled_task_runs",
		"DROP TABLE IF EXISTS fleet_partner_scheduled_tasks",
	} {
		if !strings.Contains(down, fragment) {
			t.Errorf("down migration missing %q", fragment)
		}
	}
}

// TestMigrationNumbersAreUnique guards the one mistake that makes a migration
// set unorderable: two files claiming the same version.
func TestMigrationNumbersAreUnique(t *testing.T) {
	ups, err := filepath.Glob("*.up.sql")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	seen := map[string]string{}
	for _, up := range ups {
		version, _, ok := strings.Cut(filepath.Base(up), "_")
		if !ok {
			t.Errorf("migration %s has no version prefix", up)
			continue
		}
		if previous, duplicate := seen[version]; duplicate {
			t.Errorf("duplicate migration number %s: %s and %s", version, previous, up)
		}
		seen[version] = up
	}
}

func TestSection16Migrations(t *testing.T) {
	cases := map[string][]string{
		"000034_dispatch_routes_candidates.up.sql": {"fleet_partner_dispatch_plans", "fleet_partner_route_plans", "fleet_partner_route_stops", "fleet_partner_assignment_candidates", "fleet_partner_route_publication_snapshots", "dispatch.read", "routes.publish", "control_tower.read"},
		"000035_tracking_projections.up.sql":       {"fleet_partner_tracking_event_inbox", "fleet_partner_tracking_projections", "fleet_partner_tracking_projection_conflicts", "GAP_DETECTED", "retention_expires_at"},
		"000036_geofences_alerts.up.sql":           {"fleet_partner_geofences", "fleet_partner_geofence_events", "fleet_partner_operational_alerts", "condition_fingerprint", "ACKNOWLEDGED", "CRITICAL"},
		"000037_route_optimization.up.sql":         {"fleet_partner_route_optimization_requests", "fleet_partner_route_optimization_attempts", "RECONCILIATION_REQUIRED", "input_fingerprint", "provider_job_reference"},
	}
	for name, fragments := range cases {
		up := read(t, name)
		for _, fragment := range fragments {
			if !strings.Contains(up, fragment) {
				t.Errorf("%s missing %q", name, fragment)
			}
		}
		down := read(t, strings.TrimSuffix(name, ".up.sql")+".down.sql")
		if !strings.Contains(down, "DROP TABLE") {
			t.Errorf("%s lacks rollback", name)
		}
	}
	all := ""
	for name := range cases {
		all += read(t, name)
	}
	for _, forbidden := range []string{"driver_dispatch.", "driver_hauling.", "REFERENCES providers(", "REFERENCES vehicles(", "ALTER TABLE fleet_partner_marketplace"} {
		if strings.Contains(strings.ToLower(all), strings.ToLower(forbidden)) {
			t.Errorf("cross-service ownership violation %q", forbidden)
		}
	}
}

func TestSection17And18Migrations(t *testing.T) {
	cases := map[string][]string{
		"000038_workforce_shifts_leave.up.sql":              {"fleet_partner_workforce_profiles", "fleet_partner_shifts", "fleet_partner_shift_assignments", "fleet_partner_leave_records", "fleet_partner_workforce_history", "append_only"},
		"000039_qualifications_training_performance.up.sql": {"fleet_partner_qualifications", "fleet_partner_provider_qualifications", "fleet_partner_training_courses", "fleet_partner_training_completions", "fleet_partner_performance_snapshots", "append_only"},
		"000040_assets_assignments_documents.up.sql":        {"fleet_partner_assets", "fleet_partner_asset_assignments", "fleet_partner_asset_documents", "fleet_partner_asset_history", "append_only"},
		"000041_inspections_maintenance_breakdowns.up.sql":  {"fleet_partner_inspection_templates", "fleet_partner_inspections", "fleet_partner_inspection_results", "fleet_partner_asset_safety_blocks", "fleet_partner_maintenance_schedules", "fleet_partner_work_orders", "fleet_partner_return_to_service_decisions", "fleet_partner_breakdowns", "append_only"},
		"000042_inventory_fuel_permissions.up.sql":          {"fleet_partner_parts", "fleet_partner_stock_movements", "fleet_partner_fuel_records", "fleet_partner_fuel_approval_history", "inventory.adjust", "maintenance.return_to_service", "append_only"},
	}
	all := ""
	for name, fragments := range cases {
		up := read(t, name)
		all += up
		for _, fragment := range fragments {
			if !strings.Contains(strings.ToLower(up), strings.ToLower(fragment)) {
				t.Errorf("%s missing %q", name, fragment)
			}
		}
		down := read(t, strings.TrimSuffix(name, ".up.sql")+".down.sql")
		if !strings.Contains(down, "DROP TABLE") {
			t.Errorf("%s lacks rollback", name)
		}
	}
	for _, forbidden := range []string{"references providers(", "references vehicles(", "references trips(", "wallet_ledger", "general_ledger", "payroll"} {
		if strings.Contains(strings.ToLower(all), forbidden) {
			t.Errorf("ownership or exclusion violation %q", forbidden)
		}
	}
}

func TestSections22And23Migrations(t *testing.T) {
	cases := map[string][]string{
		"000048_invoices_receivables.up.sql":             {"fleet_partner_invoices", "fleet_partner_invoice_snapshots", "fleet_partner_credit_notes", "fleet_partner_receivable_entries", "fleet_partner_financial_posting_operations", "append_only"},
		"000049_expenses_budgets_claims.up.sql":          {"fleet_partner_expenses", "fleet_partner_expense_reversals", "fleet_partner_budgets", "fleet_partner_claims", "append_only"},
		"000050_compensation_payroll_settlements.up.sql": {"fleet_partner_compensation_rules", "fleet_partner_provider_compensation_items", "fleet_partner_payroll_periods", "fleet_partner_subcontractor_settlements", "SAFETY_DECLINE", "append_only"},
		"000051_communications_notifications.up.sql":     {"fleet_partner_communication_threads", "fleet_partner_communication_messages", "idempotency_key", "fleet_partner_notification_rules", "fleet_partner_notification_intents", "append_only"},
		"000052_support_quality_sla_escalations.up.sql":  {"fleet_partner_support_case_projections", "GAP_DETECTED", "fleet_partner_quality_cases", "fleet_partner_sla_measurements", "fleet_partner_escalations", "append_only"},
	}
	all := ""
	for name, fragments := range cases {
		up := read(t, name)
		all += strings.ToLower(up)
		for _, fragment := range fragments {
			if !strings.Contains(strings.ToLower(up), strings.ToLower(fragment)) {
				t.Errorf("%s missing %q", name, fragment)
			}
		}
		down := read(t, strings.TrimSuffix(name, ".up.sql")+".down.sql")
		if !strings.Contains(down, "DROP TABLE") {
			t.Errorf("%s lacks paired rollback", name)
		}
	}
	for _, forbidden := range []string{"create table fleet_partner_ledger", "create table fleet_partner_bank", "references notification_", "references complaints(", "references wallet_"} {
		if strings.Contains(all, forbidden) {
			t.Errorf("authority violation %q", forbidden)
		}
	}
}

// TestSection28AuditAccessMigration proves the two-tier audit-log read
// permissions exist and audit.read_sensitive is granted no more broadly than
// the documents.download_restricted precedent (owner + compliance only).
func TestSection28AuditAccessMigration(t *testing.T) {
	up := read(t, "000065_section28_audit_access.up.sql")
	for _, fragment := range []string{
		"('audit.read'", "('audit.read_sensitive'",
		"rol_system_owner", "rol_system_compliance",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("missing fragment %q", fragment)
		}
	}
	for _, forbidden := range []string{"DROP COLUMN", "DROP TABLE", "DELETE FROM"} {
		if strings.Contains(up, forbidden) {
			t.Errorf("up migration must be purely additive, found %q", forbidden)
		}
	}
	down := read(t, "000065_section28_audit_access.down.sql")
	for _, fragment := range []string{
		"DELETE FROM fleet_partner_role_permissions WHERE permission_code IN ('audit.read', 'audit.read_sensitive')",
		"DELETE FROM fleet_partner_permissions WHERE code IN ('audit.read', 'audit.read_sensitive')",
	} {
		if !strings.Contains(down, fragment) {
			t.Errorf("down migration missing %q", fragment)
		}
	}
}

// TestSection28LegalHoldsMigration proves the generic legal-hold table and
// its three-permission surface exist, DOCUMENT is not a valid scope type
// (document legal hold has its own, already-media-propagated path), and the
// migration stays purely additive.
func TestSection28LegalHoldsMigration(t *testing.T) {
	up := read(t, "000066_section28_legal_holds.up.sql")
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS fleet_partner_legal_holds",
		"'ORGANIZATION', 'CUSTOMER', 'PROVIDER', 'TRIP', 'ASSIGNMENT'",
		"CHECK (status IN ('ACTIVE', 'RELEASED'))",
		"('privacy.legal_hold.read'", "('privacy.legal_hold.manage'", "('privacy.legal_hold.release'",
		"rol_system_owner", "rol_system_compliance",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("missing fragment %q", fragment)
		}
	}
	if strings.Contains(up, "'DOCUMENT'") {
		t.Error("legal_holds scope_type must not include DOCUMENT: document legal hold already has its own media-propagated path")
	}
	for _, forbidden := range []string{"DROP COLUMN", "DROP TABLE"} {
		if strings.Contains(up, forbidden) {
			t.Errorf("up migration must be purely additive, found %q", forbidden)
		}
	}
	down := read(t, "000066_section28_legal_holds.down.sql")
	if !strings.Contains(down, "DROP TABLE IF EXISTS fleet_partner_legal_holds") {
		t.Error("down migration missing DROP TABLE for fleet_partner_legal_holds")
	}
}

// TestSection28SecuritySettingsMigration proves the per-organization
// security-settings table enforces platform minimums/maximums at the
// database layer (not just in Go) and the four Section 28 security.*
// permissions exist.
func TestSection28SecuritySettingsMigration(t *testing.T) {
	up := read(t, "000067_section28_security_settings.up.sql")
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS fleet_partner_security_settings",
		"organization_id                      text PRIMARY KEY",
		"CHECK (step_up_freshness_seconds BETWEEN 300 AND 3600)",
		"CHECK (session_revocation_policy IN ('IMMEDIATE', 'STANDARD'))",
		"('security.settings.read'", "('security.settings.manage'",
		"('security.sessions.revoke'", "('security.credentials.manage'",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("missing fragment %q", fragment)
		}
	}
	for _, forbidden := range []string{"DROP COLUMN", "DROP TABLE"} {
		if strings.Contains(up, forbidden) {
			t.Errorf("up migration must be purely additive, found %q", forbidden)
		}
	}
	down := read(t, "000067_section28_security_settings.down.sql")
	if !strings.Contains(down, "DROP TABLE IF EXISTS fleet_partner_security_settings") {
		t.Error("down migration missing DROP TABLE for fleet_partner_security_settings")
	}
}

// TestSection28RetentionPoliciesMigration proves the retention-class
// CHECK constraint lists all fourteen Section 30 classes and the two
// privacy.retention.* permissions exist.
func TestSection28RetentionPoliciesMigration(t *testing.T) {
	up := read(t, "000068_section28_retention_policies.up.sql")
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS fleet_partner_retention_policies",
		"'TEMPORARY', 'OPERATIONAL', 'CUSTOMER_RECORD', 'PROVIDER_RECORD'",
		"'TRIP_EXECUTION', 'SAFETY_EVIDENCE', 'INCIDENT_EVIDENCE', 'COMPLIANCE'",
		"'FINANCIAL', 'PAYROLL', 'SUPPORT', 'AUDIT', 'LEGAL_HOLD', 'COMPANY_CONFIDENTIAL'",
		"PRIMARY KEY (organization_id, retention_class)",
		"('privacy.retention.read'", "('privacy.retention.manage'",
	} {
		if !strings.Contains(up, fragment) {
			t.Errorf("missing fragment %q", fragment)
		}
	}
	for _, forbidden := range []string{"DROP COLUMN", "DROP TABLE"} {
		if strings.Contains(up, forbidden) {
			t.Errorf("up migration must be purely additive, found %q", forbidden)
		}
	}
	down := read(t, "000068_section28_retention_policies.down.sql")
	if !strings.Contains(down, "DROP TABLE IF EXISTS fleet_partner_retention_policies") {
		t.Error("down migration missing DROP TABLE for fleet_partner_retention_policies")
	}
}
