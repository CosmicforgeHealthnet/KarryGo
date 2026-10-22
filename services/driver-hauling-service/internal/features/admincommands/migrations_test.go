package admincommands

import (
	"context"
	"os"
	"strings"
	"testing"

	"cosmicforge/logistics/shared/go/admincommand"
	fixture "cosmicforge/logistics/shared/go/adminowner/testfixture"
	"github.com/google/uuid"
)

const migrationDir = "../../../migrations/"

func TestCommandLedgerMigrationEmbedsSharedTemplateVerbatim(t *testing.T) {
	raw, err := os.ReadFile(migrationDir + "029_admin_command_ledger.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, admincommand.Schema) || !strings.Contains(text, "-- shared-template-sha256: "+admincommand.SchemaSHA256()) {
		t.Fatal("hauling command ledger drifted from shared/go/admincommand/schema.sql")
	}
}

func TestPostgresAdminMigrationsRollBackOnlyWhenEmptyAndReapply(t *testing.T) {
	db := fixture.Database(t, "../../../migrations")
	ctx := context.Background()
	read := func(name string) string {
		raw, err := os.ReadFile(migrationDir + name)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	exists := func(table string) bool {
		var ok bool
		if err := db.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, table).Scan(&ok); err != nil {
			t.Fatal(err)
		}
		return ok
	}
	// Populated evidence refuses rollback.
	if _, err := db.Exec(ctx, `INSERT INTO hauling_admin_intervention_heads(subject_kind,subject_id,version,state) VALUES('trucks',$1,1,'ACTIVE')`, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, read("030_admin_inspections_interventions.down.sql")); err == nil {
		t.Fatal("populated intervention evidence dropped")
	}
	if _, err := db.Exec(ctx, `DELETE FROM hauling_admin_intervention_heads`); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"030_admin_inspections_interventions.down.sql", "029_admin_command_ledger.down.sql"} {
		if _, err := db.Exec(ctx, read(name)); err != nil {
			t.Fatal(name, err)
		}
	}
	for _, table := range []string{"admin_owner_command_decisions", "hauling_admin_inspections", "hauling_admin_inspection_evidence", "hauling_admin_legacy_inspection_mappings", "hauling_cargo_suitability_decisions", "hauling_admin_intervention_heads"} {
		if exists(table) {
			t.Fatal("rollback left", table)
		}
	}
	for _, name := range []string{"029_admin_command_ledger.up.sql", "030_admin_inspections_interventions.up.sql"} {
		if _, err := db.Exec(ctx, read(name)); err != nil {
			t.Fatal("reapply", name, err)
		}
	}
	if !exists("admin_owner_command_decisions") || !exists("hauling_admin_inspections") {
		t.Fatal("reapply incomplete")
	}
	// A populated ledger refuses rollback.
	if _, err := db.Exec(ctx, `INSERT INTO admin_owner_command_decisions(idempotency_key,operation_id,decision_id,owner,contract,subject_kind,subject_id,action,version_scope,version_key,recorded_version,operation_digest,intent_digest,staff_ref,purpose_code,product,market,zone,organization_ref,tenant_ref,policy_id,policy_version,policy_checksum,receipt)
 VALUES('k',gen_random_uuid(),gen_random_uuid(),'driver-hauling-service','c','trucks','t','RESTRICT_TRUCK','SUBJECT','',1,$1,$1,'s','p','karryam','m','z','o','t','pol',1,$1,'{}')`, strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, read("029_admin_command_ledger.down.sql")); err == nil {
		t.Fatal("populated command ledger dropped")
	}
}
