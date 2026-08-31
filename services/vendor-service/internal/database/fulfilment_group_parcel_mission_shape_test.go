package database

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fulfilmentAggregateMigration = "000100_commerce_fulfilment_group_parcel_mission"

func readFulfilmentMigration(t *testing.T, suffix string) string {
	t.Helper()
	path := filepath.Join("..", "..", "migrations", fulfilmentAggregateMigration+suffix)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return strings.ToLower(string(raw))
}

func TestFulfilmentAggregateMigrationShape(t *testing.T) {
	sql := readFulfilmentMigration(t, ".up.sql")

	for _, table := range []string{
		"commerce_fulfilment_groups",
		"commerce_fulfilment_units",
		"commerce_fulfilment_parcels",
		"commerce_fulfilment_plan_versions",
		"commerce_fulfilment_plan_missions",
		"commerce_fulfilment_plan_stops",
		"commerce_fulfilment_plan_mission_parcels",
		"commerce_platform_mission_refs",
		"commerce_vendor_missions",
		"commerce_parcel_custody_events",
		"commerce_parcel_returns",
	} {
		if !strings.Contains(sql, "create table "+table) {
			t.Errorf("migration is missing table %q", table)
		}
	}
}

// The invariants that matter under concurrency are database constraints, not
// handler checks. A handler check is not enforced against a second writer.
func TestFulfilmentAggregateInvariantsAreConstraints(t *testing.T) {
	sql := readFulfilmentMigration(t, ".up.sql")

	cases := []struct {
		fragment string
		why      string
	}{
		{"unique (parent_order_id)",
			"one enduring fulfilment group per commerce order"},
		{"unique (vendor_order_id)",
			"one pickup obligation per vendor order"},
		{"unique (group_id, version)",
			"two concurrent replans cannot both become the same plan version"},
		{"unique (group_id, idempotency_key)",
			"a retried replan reads back its own version instead of appending another"},
		{"unique (unit_id, idempotency_key)",
			"a retried packing worker cannot create a second physical parcel"},
		{"unique (unit_id, sequence)",
			"parcels within a unit are distinguishable"},
		{"unique (plan_id, slot_ref)",
			"a mission slot is stable within a plan version"},
		{"unique (plan_mission_id, sequence)",
			"the route order is persisted, not derived from unordered rows"},
		{"unique (parcel_id, source, source_event_id)",
			"at-least-once custody delivery is safe to replay"},
		{"unique (plan_mission_id)",
			"one vendor mission per vendor-owned slot"},
	}
	for _, testCase := range cases {
		if !strings.Contains(sql, testCase.fragment) {
			t.Errorf("missing constraint %q — %s", testCase.fragment, testCase.why)
		}
	}

	for _, index := range []struct {
		fragment string
		why      string
	}{
		{"commerce_fulfilment_plan_versions_one_active_idx",
			"at most one plan version is in force per group"},
		{"commerce_fulfilment_plan_stops_one_dropoff_idx",
			"exactly one receiver destination per mission"},
		{"commerce_platform_mission_refs_one_live_slot_idx",
			"one live canonical mission per plan mission slot"},
		{"commerce_platform_mission_refs_external_idx",
			"the same canonical mission is never linked twice under one authority"},
		{"commerce_parcel_returns_one_open_idx",
			"one open return per parcel"},
	} {
		if !strings.Contains(sql, index.fragment) {
			t.Errorf("missing partial index %q — %s", index.fragment, index.why)
		}
	}
}

// A vendor-owned mission is executed by the vendor and a platform mission never
// is. Collapsing the two would make responsibility unanswerable.
func TestPlanMissionOwnerAndAuthorityAgree(t *testing.T) {
	sql := readFulfilmentMigration(t, ".up.sql")

	if !strings.Contains(sql, "commerce_fulfilment_plan_missions_owner_authority_chk") {
		t.Fatal("missing the owner/authority agreement constraint")
	}
	if !strings.Contains(sql, "owner = 'vendor' and authority = 'vendor'") {
		t.Error("a vendor-owned mission must be executed by the vendor authority")
	}
	if !strings.Contains(sql, "owner = 'platform' and authority in ('dispatch','hauling','taxi')") {
		t.Error("a platform mission must name a platform authority")
	}
	// HYBRID is a planning behaviour, never an executor. A mission with it as
	// owner would have nobody responsible for it.
	if strings.Contains(sql, "owner in ('platform','vendor','hybrid')") {
		t.Error("HYBRID must not be a mission owner")
	}
}

// The projection must always name the source that produced it, so a late event
// can be recognised as late rather than applied as new.
func TestMissionProjectionCarriesItsSourceVersion(t *testing.T) {
	sql := readFulfilmentMigration(t, ".up.sql")

	for _, fragment := range []string{
		"status_source_version",
		"commerce_platform_mission_refs_status_chk",
	} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("mission projection is missing %q", fragment)
		}
	}
	if !strings.Contains(sql, "commerce_platform_mission_refs_linked_chk") {
		t.Error("a LINKED mission must carry the authority's own canonical reference")
	}
	if !strings.Contains(sql, "commerce_platform_mission_refs_refused_chk") {
		t.Error("a REFUSED mission must carry a reason a caller can act on")
	}
}

// The vendor mission lifecycle is the delivery feature's existing
// ExecutionState, reused rather than re-invented.
func TestVendorMissionUsesExistingExecutionVocabulary(t *testing.T) {
	sql := readFulfilmentMigration(t, ".up.sql")

	for _, status := range []string{
		"'planned'", "'assigned'", "'ready_for_pickup'", "'custody_transferred'",
		"'out_for_delivery'", "'delivered'", "'failed'", "'return_required'", "'returned'",
	} {
		if !strings.Contains(sql, status) {
			t.Errorf("vendor mission is missing the existing execution state %s", status)
		}
	}
}

// Replan reasons reuse the delivery-policy overflow vocabulary rather than
// duplicating it under new names: the same operational condition must not have
// two spellings.
func TestPlanReasonsReuseOverflowVocabulary(t *testing.T) {
	sql := readFulfilmentMigration(t, ".up.sql")

	for _, reason := range []string{
		"'own_rider_first'", "'distance_overflow'", "'time_overflow'",
		"'capacity_overflow'", "'vehicle_overflow'", "'sla_overflow'",
	} {
		if !strings.Contains(sql, reason) {
			t.Errorf("plan reason vocabulary is missing %s", reason)
		}
	}
	if !strings.Contains(sql, "commerce_fulfilment_plan_versions_initial_chk") {
		t.Error("version 1 must be the initial plan and every later one must supersede")
	}
}

// The down migration must actually drop what the up migration created, and in
// an order the foreign keys allow.
func TestFulfilmentAggregateDownMigrationReverses(t *testing.T) {
	down := readFulfilmentMigration(t, ".down.sql")

	tables := []string{
		"commerce_parcel_returns",
		"commerce_parcel_custody_events",
		"commerce_vendor_missions",
		"commerce_platform_mission_refs",
		"commerce_fulfilment_plan_mission_parcels",
		"commerce_fulfilment_plan_stops",
		"commerce_fulfilment_plan_missions",
		"commerce_fulfilment_plan_versions",
		"commerce_fulfilment_parcels",
		"commerce_fulfilment_units",
		"commerce_fulfilment_groups",
	}

	lastIndex := -1
	for _, table := range tables {
		fragment := "drop table if exists " + table
		index := strings.Index(down, fragment)
		if index < 0 {
			t.Fatalf("down migration does not drop %q", table)
		}
		if index < lastIndex {
			t.Errorf("%q is dropped out of dependency order", table)
		}
		lastIndex = index
	}

	// The group's forward-pointing plan foreign key has to go first: it
	// references a table dropped before the group table itself.
	fkIndex := strings.Index(down, "commerce_fulfilment_groups_current_plan_fk")
	planIndex := strings.Index(down, "drop table if exists commerce_fulfilment_plan_versions")
	if fkIndex < 0 {
		t.Fatal("down migration does not drop the current-plan foreign key")
	}
	if fkIndex > planIndex {
		t.Error("the current-plan foreign key must be dropped before its target table")
	}

	for _, permission := range []string{
		"perm_fulfilment_read", "perm_fulfilment_parcel_manage", "perm_fulfilment_mission_manage",
	} {
		if !strings.Contains(down, permission) {
			t.Errorf("down migration leaves permission %q behind", permission)
		}
	}
}

// Migration files are paired. A migration with no reverse cannot be rolled
// back, which is discovered at exactly the wrong moment.
func TestFulfilmentAggregateMigrationIsPaired(t *testing.T) {
	for _, suffix := range []string{".up.sql", ".down.sql"} {
		path := filepath.Join("..", "..", "migrations", fulfilmentAggregateMigration+suffix)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("missing %s: %v", path, err)
		}
	}
}
