package integration_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"cosmicforge/logistics/services/media-file-service/internal/database"
)

// TestEvidenceMigrationPreservesLegacyRows certifies 000005 against a row that
// existed before it: rolled back to 000004, a legacy object is written, then
// 000005 is applied over it. That row must read as unbound ACTIVE evidence --
// exactly what it meant before -- and the migration must reapply cleanly.
func TestEvidenceMigrationPreservesLegacyRows(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	if err := database.ApplyMigrations(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if err := database.RollbackLastMigration(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("rollback 000005: %v", err)
	}
	t.Cleanup(func() {
		if err := database.ApplyMigrations(context.Background(), pool, migrationsDir); err != nil {
			t.Errorf("cleanup reapply: %v", err)
		}
	})

	id := "mob_legacy_" + uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO media_objects (id, organization_id, owner_service, upload_purpose, storage_object_key)
		VALUES ($1, 'org_legacy', 'verification-compliance-service', 'VEHICLE_DOCUMENT', $1)`,
		id); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}

	if err := database.ApplyMigrations(ctx, pool, migrationsDir); err != nil {
		t.Fatalf("apply 000005 over legacy data: %v", err)
	}
	var bindingType, bindingID, evidenceState string
	if err := pool.QueryRow(ctx, `
		SELECT binding_type, binding_id, evidence_state FROM media_objects WHERE id=$1`, id).
		Scan(&bindingType, &bindingID, &evidenceState); err != nil {
		t.Fatalf("read legacy row: %v", err)
	}
	if bindingType != "" || bindingID != "" || evidenceState != "ACTIVE" {
		t.Fatalf("legacy row changed meaning: binding=(%q,%q) state=%q",
			bindingType, bindingID, evidenceState)
	}

	// A half-binding is refused by the database, not only by the use case.
	if _, err := pool.Exec(ctx,
		`UPDATE media_objects SET binding_type='MISSION' WHERE id=$1`, id); err == nil {
		t.Fatal("a binding type with no id was stored")
	}
}
