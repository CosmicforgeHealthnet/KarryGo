package productionmaster

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/features/inventory"
)

// Inventory-controlled ingredients and packaging get a branch stock item
// (with a balance row) on save, and listed balances carry the material name.
func TestInventoryControlledMaterialsProvisionNamedStock(t *testing.T) {
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	suffix := time.Now().Format("150405.000000")
	businessID := "vb_prov_" + suffix
	branchID := "vbr_prov_" + suffix
	staffID := "vs_prov_" + suffix
	if _, err = pool.Exec(ctx, `INSERT INTO vendor_users(id,email) VALUES($1,$2)`, "vu_prov_"+suffix, "prov"+suffix+"@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO vendor_staff(id,vendor_id,user_id) VALUES($1,$2,$3)`,
		staffID, businessID, "vu_prov_"+suffix); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO vendor_businesses(id,legal_name,owner_ref,currency,timezone)
	    VALUES($1,'Prov Kitchen',$2,'NGN','Africa/Lagos')`, businessID, staffID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO vendor_branches(id,business_id,display_name,address_line1,city,region,country_code,latitude,longitude,timezone)
	    VALUES($1,$2,'Ikeja','1 Allen','Ikeja','Lagos','NG',6.6,3.35,'Africa/Lagos')`, branchID, businessID); err != nil {
		t.Fatal(err)
	}

	svc := NewService(pool, nil)
	actor := Actor{StaffID: staffID, BusinessID: businessID}

	rice, err := svc.CreateIngredient(ctx, actor, CreateIngredientInput{
		Code: "RICE", Name: "Rice", MeasurementFamily: "MASS",
		DefaultUnitCode: "KG", InventoryControlMode: InventoryControlled,
	})
	if err != nil {
		t.Fatal(err)
	}
	box, err := svc.CreatePackagingMaterial(ctx, actor, CreatePackagingMaterialInput{
		Code: "BOX", Name: "Food box", MeasurementFamily: "COUNT",
		DefaultUnitCode: "UNIT", InventoryControlMode: InventoryControlled,
	})
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := svc.PutIngredientBranchConfig(ctx, actor, rice.ID, branchID,
		PutBranchConfigInput{Enabled: true, ProductionEligible: true})
	if err != nil {
		t.Fatalf("ingredient config: %v", err)
	}
	if cfg.InventoryItemID == "" {
		t.Fatal("expected a provisioned inventory item")
	}
	again, err := svc.PutIngredientBranchConfig(ctx, actor, rice.ID, branchID,
		PutBranchConfigInput{Enabled: true, ProductionEligible: true})
	if err != nil {
		t.Fatalf("second save: %v", err)
	}
	if again.InventoryItemID != cfg.InventoryItemID {
		t.Fatalf("re-save minted a second item: %s vs %s", again.InventoryItemID, cfg.InventoryItemID)
	}
	if _, err = svc.PutPackagingBranchConfig(ctx, actor, box.ID, branchID,
		PutBranchConfigInput{Enabled: true}); err != nil {
		t.Fatalf("packaging config: %v", err)
	}

	balances, err := inventory.QueryBranchBalances(ctx, pool, businessID, branchID, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, b := range balances {
		names[b.Name] = true
	}
	if len(balances) != 2 || !names["Rice"] || !names["Food box"] {
		t.Fatalf("unexpected balances: %+v", balances)
	}
}
