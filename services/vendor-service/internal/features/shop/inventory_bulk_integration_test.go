package shop

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func bulkJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestStructuredCatalogueImportAtomicApply(t *testing.T) {
	f := section12Fixture(t)
	ctx := context.Background()
	scope := Scope{Global: true}
	if _, err := f.service.SetOperatingModels(ctx, f.actorA, scope, f.branchShop, SetOperatingModelsInput{PrimaryModel: ModelSupermarket, Models: []string{ModelSupermarket}}); err != nil {
		t.Fatal(err)
	}
	suffix := uuid.NewString()[:8]
	categoryID := "scat_bulk_" + suffix
	productID := "spr_bulk_" + suffix
	variantID := "svar_bulk_" + suffix
	sellUnitID := "ssu_bulk_" + suffix
	barcodeID := "spi_bulk_" + suffix
	listingID := "sbl_bulk_" + suffix
	breakID := "sqb_bulk_" + suffix
	price := int64(1250)
	breakPrice := int64(1100)
	rows := []json.RawMessage{
		bulkJSON(t, map[string]any{"entity_type": "QUANTITY_BREAK", "id": breakID, "listing_id": listingID, "sell_unit_id": sellUnitID, "min_quantity": 10, "unit_price_minor": breakPrice}),
		bulkJSON(t, map[string]any{"entity_type": "BARCODE", "id": barcodeID, "product_id": productID, "variant_id": variantID, "identifier_type": "BARCODE", "identifier_value": "BULK" + suffix}),
		bulkJSON(t, map[string]any{"entity_type": "LISTING", "id": listingID, "branch_id": f.branchShop, "variant_id": variantID, "operating_model": ModelSupermarket, "sell_unit_ids": []string{sellUnitID}}),
		bulkJSON(t, map[string]any{"entity_type": "SELL_UNIT", "id": sellUnitID, "product_id": productID, "variant_id": variantID, "measure_id": "smd_piece", "display_name": "each", "measurement_mode": ModeFixed, "retail_price_minor": price}),
		bulkJSON(t, map[string]any{"entity_type": "VARIANT", "id": variantID, "product_id": productID, "name": "Bulk each", "package_level": "EACH", "retail_price_minor": price}),
		bulkJSON(t, map[string]any{"entity_type": "PRODUCT", "id": productID, "category_id": categoryID, "name": "Bulk Product", "product_class": "PACKAGED", "operating_models": []string{ModelSupermarket}}),
		bulkJSON(t, map[string]any{"entity_type": "CATEGORY", "id": categoryID, "name": "Bulk Category " + suffix}),
	}
	job, err := f.service.SubmitBulkOperation(ctx, f.actorA, scope, BulkOperationInput{OperationType: "CATALOGUE_IMPORT", Rows: rows})
	if err != nil {
		t.Fatal(err)
	}
	job, err = f.service.ApplyBulkOperation(ctx, f.actorA, scope, job.ID)
	if err != nil || job.Status != "COMPLETED" {
		t.Fatalf("catalogue import job=%+v err=%v", job, err)
	}
	for table, id := range map[string]string{
		"shop_categories": categoryID, "shop_products": productID, "shop_variants": variantID,
		"shop_sell_units": sellUnitID, "shop_product_identifiers": barcodeID,
		"shop_branch_listings": listingID, "shop_quantity_breaks": breakID,
	} {
		var count int
		if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE id=$1 AND business_id=$2`, id, f.actorA.VendorID).Scan(&count); err != nil || count != 1 {
			t.Fatalf("%s id=%s count=%d err=%v", table, id, count, err)
		}
	}
	var priceEvidence int
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM shop_price_versions WHERE business_id=$1 AND subject_ref=ANY($2)`, f.actorA.VendorID, []string{variantID, sellUnitID, breakID}).Scan(&priceEvidence); err != nil || priceEvidence != 3 {
		t.Fatalf("baseline price evidence=%d err=%v", priceEvidence, err)
	}
}

func TestCatalogueImportRollbackAndBulkPriceHistory(t *testing.T) {
	f := section12Fixture(t)
	ctx := context.Background()
	scope := Scope{Global: true}
	suffix := uuid.NewString()[:8]
	categoryID := "scat_rollback_" + suffix
	job, err := f.service.SubmitBulkOperation(ctx, f.actorA, scope, BulkOperationInput{OperationType: "CATALOGUE_IMPORT", Rows: []json.RawMessage{
		bulkJSON(t, map[string]any{"entity_type": "CATEGORY", "id": categoryID, "name": "Rollback Category"}),
		bulkJSON(t, map[string]any{"entity_type": "PRODUCT", "id": "spr_rollback_" + suffix, "category_id": "missing", "name": "Invalid Product", "product_class": "PACKAGED"}),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.ApplyBulkOperation(ctx, f.actorA, scope, job.ID); err == nil {
		t.Fatal("invalid catalogue import applied")
	}
	var count int
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM shop_categories WHERE id=$1`, categoryID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed import partially applied category count=%d err=%v", count, err)
	}

	basis := createInventoryBasis(t, f, "Bulk Price", 1)
	var version int64
	var previous int64
	if err = f.pool.QueryRow(ctx, `SELECT version,retail_price_minor FROM shop_variants WHERE id=$1`, basis.VariantRef).Scan(&version, &previous); err != nil {
		t.Fatal(err)
	}
	next := previous + 333
	priceJob, err := f.service.SubmitBulkOperation(ctx, f.actorA, scope, BulkOperationInput{OperationType: "PRICE_UPDATE", Rows: []json.RawMessage{bulkJSON(t, map[string]any{"variant_id": basis.VariantRef, "retail_price_minor": next, "expected_version": version})}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.ApplyBulkOperation(ctx, f.actorA, scope, priceJob.ID); err != nil {
		t.Fatal(err)
	}
	var recordedPrevious, recordedNext int64
	if err = f.pool.QueryRow(ctx, `SELECT previous_amount_minor,amount_minor FROM shop_price_versions WHERE business_id=$1 AND subject_type='VARIANT_RETAIL' AND subject_ref=$2 ORDER BY revision DESC LIMIT 1`, f.actorA.VendorID, basis.VariantRef).Scan(&recordedPrevious, &recordedNext); err != nil {
		t.Fatal(err)
	}
	if recordedPrevious != previous || recordedNext != next {
		t.Fatalf("bulk price history previous=%d next=%d want=%d/%d", recordedPrevious, recordedNext, previous, next)
	}
}

func TestBulkWorkerLeaseCompletesDryRun(t *testing.T) {
	f := section12Fixture(t)
	ctx := context.Background()
	scope := Scope{Global: true}
	job, err := f.service.SubmitBulkOperation(ctx, f.actorA, scope, BulkOperationInput{OperationType: "CATALOGUE_IMPORT", DryRun: true, Rows: []json.RawMessage{bulkJSON(t, map[string]any{"entity_type": "CATEGORY", "id": "scat_dry_" + uuid.NewString()[:8], "name": "Dry Run Category"})}})
	if err != nil {
		t.Fatal(err)
	}
	var status string
	var leaseOwner *string
	for attempt := 0; attempt < 10; attempt++ {
		if _, err = f.service.RunBulkOperationPass(ctx, 25); err != nil {
			t.Fatal(err)
		}
		if err = f.pool.QueryRow(ctx, `SELECT status,lease_owner FROM shop_bulk_operation_jobs WHERE id=$1`, job.ID).Scan(&status, &leaseOwner); err != nil {
			t.Fatal(err)
		}
		if status == "COMPLETED" {
			break
		}
	}
	if status != "COMPLETED" || leaseOwner != nil {
		t.Fatalf("worker job status=%s lease=%v", status, leaseOwner)
	}
	invalid, err := f.service.SubmitBulkOperation(ctx, f.actorA, scope, BulkOperationInput{OperationType: "CATALOGUE_IMPORT", DryRun: true, Rows: []json.RawMessage{bulkJSON(t, map[string]any{"entity_type": "CATEGORY", "name": "Missing stable id"})}})
	if err != nil {
		t.Fatal(err)
	}
	if invalid.ErrorCount != 1 {
		t.Fatalf("invalid dry run error_count=%d", invalid.ErrorCount)
	}
	if _, err = f.service.ApplyBulkOperation(ctx, f.actorA, scope, invalid.ID); err != nil {
		t.Fatal(err)
	}
	detail, err := f.service.GetBulkOperation(ctx, f.actorA, scope, invalid.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Status != "COMPLETED" || len(detail.Rows) != 1 || detail.Rows[0].Status != "ERROR" || string(detail.Rows[0].ValidationErrors) == "[]" {
		t.Fatalf("dry-run error report=%+v", detail)
	}
}
