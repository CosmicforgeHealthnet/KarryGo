package database

// Migration 000121 widens the capability vocabulary to CATERING and
// MANUFACTURER.
//
// It gets its own test because of what a mistake here would cost. The
// constraint it replaces guards vendor_capabilities, the row that says what a
// business is allowed to sell; dropping a value, or rolling back over live
// rows, would take a vendor's livelihood with it. So this proves three things:
// every original value still works, the two new ones now work, and the
// rollback refuses rather than deletes when a real caterer exists.

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestCapabilityVocabularyMigrationRollbackAndReapply(t *testing.T) {
	pool := groupingMigrationPool(t)
	ctx := context.Background()

	// A business that existed before the migration, with an original
	// capability. Nothing about it may change.
	legacyBusiness := "vendor_legacy_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	legacyStaff, legacyUser := "vs_"+legacyBusiness, "vu_"+legacyBusiness
	legacyCapability := "vc_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	t.Cleanup(func() {
		clean := context.Background()
		_, _ = pool.Exec(clean, `DELETE FROM vendor_capabilities WHERE business_id=$1`, legacyBusiness)
		_, _ = pool.Exec(clean, `DELETE FROM vendor_businesses WHERE id=$1`, legacyBusiness)
		_, _ = pool.Exec(clean, `DELETE FROM vendor_staff WHERE id=$1`, legacyStaff)
		_, _ = pool.Exec(clean, `DELETE FROM vendor_users WHERE id=$1`, legacyUser)
	})
	if _, err := pool.Exec(ctx, `INSERT INTO vendor_users(id,email) VALUES($1,$2)`,
		legacyUser, legacyUser+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO vendor_staff(id,vendor_id,user_id,status) VALUES($1,$2,$3,'active')`,
		legacyStaff, legacyBusiness, legacyUser); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO vendor_businesses(id,legal_name,owner_ref,currency,timezone,status)
		VALUES($1,'Legacy Wholesaler Limited',$2,'NGN','Africa/Lagos','DRAFT')`,
		legacyBusiness, legacyStaff); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO vendor_capabilities(id,business_id,capability_type,status)
		VALUES($1,$2,'WHOLESALE','ACTIVE')`, legacyCapability, legacyBusiness); err != nil {
		t.Fatal(err)
	}

	accepts := func(capability string) bool {
		t.Helper()
		id := "vc_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		_, err := pool.Exec(ctx, `
			INSERT INTO vendor_capabilities(id,business_id,capability_type,status)
			VALUES($1,$2,$3,'DRAFT')`, id, legacyBusiness, capability)
		if err == nil {
			_, _ = pool.Exec(ctx, `DELETE FROM vendor_capabilities WHERE id=$1`, id)
			return true
		}
		return false
	}

	// After apply: the original seven and the two new ones, and nothing else.
	for _, capability := range []string{"RESTAURANT", "FAST_FOOD", "CLOUD_KITCHEN", "PASTRY_BAKERY",
		"SHOP_GROCERY", "MARKET", "CATERING", "MANUFACTURER"} {
		if !accepts(capability) {
			t.Errorf("%s is not accepted after apply", capability)
		}
	}
	for _, rejected := range []string{"PHARMACY", "CATERER", "FACTORY", "catering"} {
		if accepts(rejected) {
			t.Errorf("%q was accepted; the vocabulary is not closed", rejected)
		}
	}

	// The legacy row is exactly as it was.
	var storedType, storedStatus string
	if err := pool.QueryRow(ctx, `SELECT capability_type,status FROM vendor_capabilities WHERE id=$1`,
		legacyCapability).Scan(&storedType, &storedStatus); err != nil {
		t.Fatalf("legacy capability disappeared: %v", err)
	}
	if storedType != "WHOLESALE" || storedStatus != "ACTIVE" {
		t.Fatalf("legacy capability was rewritten: %s/%s", storedType, storedStatus)
	}

	// With a live CATERING row the rollback must refuse, not delete.
	liveCatering := "vc_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := pool.Exec(ctx, `
		INSERT INTO vendor_capabilities(id,business_id,capability_type,status)
		VALUES($1,$2,'CATERING','ACTIVE')`, liveCatering, legacyBusiness); err != nil {
		t.Fatalf("seed a live caterer: %v", err)
	}
	// Unwind anything a later section added so the next rollback targets
	// 000121 itself, rather than assuming it is the last migration.
	for unwound := 0; ; unwound++ {
		var newer bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vendor_service_schema_migrations WHERE version >= '000122')`).Scan(&newer); err != nil {
			t.Fatal(err)
		}
		if !newer {
			break
		}
		if unwound >= knownMigrationCount(t) {
			t.Fatal("unwound the full known migration chain and 000121 is still not the tip")
		}
		if err := RollbackLastMigration(ctx, pool, "../../migrations"); err != nil {
			t.Fatalf("unwind towards 000121: %v", err)
		}
	}
	err := RollbackLastMigration(ctx, pool, "../../migrations")
	if err == nil {
		t.Fatal("the rollback dropped a live caterer's capability instead of refusing")
	}
	if !strings.Contains(err.Error(), "cannot roll back 000121") {
		t.Fatalf("rollback failed for the wrong reason: %v", err)
	}
	var survivors int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_capabilities WHERE id=$1`, liveCatering).Scan(&survivors); err != nil {
		t.Fatal(err)
	}
	if survivors != 1 {
		t.Fatal("the refused rollback deleted the caterer anyway")
	}

	// With the caterer retired through the ordinary path, the rollback is
	// exact: the vocabulary narrows and the original values still work.
	if _, err = pool.Exec(ctx, `DELETE FROM vendor_capabilities WHERE id=$1`, liveCatering); err != nil {
		t.Fatal(err)
	}
	if err = RollbackLastMigration(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("rollback with no new-type rows: %v", err)
	}
	if accepts("CATERING") || accepts("MANUFACTURER") {
		t.Fatal("the new values survived the rollback; the CHECK was not narrowed")
	}
	// WHOLESALE is the legacy row's own type and vendor_capabilities is unique
	// per business and type, so the check uses types this business does not
	// already hold.
	if !accepts("RESTAURANT") || !accepts("SHOP_GROCERY") {
		t.Fatal("the rollback took an original value with it")
	}

	// And reapplying restores them.
	if err = ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatalf("reapply 000121: %v", err)
	}
	if !accepts("CATERING") || !accepts("MANUFACTURER") {
		t.Fatal("CATERING/MANUFACTURER are not accepted after reapply")
	}
	if !accepts("RESTAURANT") || !accepts("SHOP_GROCERY") {
		t.Fatal("an original value is not accepted after reapply")
	}
}
