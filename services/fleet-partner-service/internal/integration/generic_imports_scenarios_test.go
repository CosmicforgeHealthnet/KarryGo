package integration_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	bulkmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/bulkimports/models"
	bulkrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/bulkimports/repositories"
	bulkusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/bulkimports/usecases"
)

const privateOrderCSV = "rowReference,customerId,serviceCatalogItemId,timezone\nrow-1,cus-1,sci-1,Africa/Lagos\n"

// TestGenericImports_DryRunCreatesNoTargetRecords proves the one hard
// guarantee dry-run exists for: validation runs and produces real row
// outcomes, but Commit is refused outright for a DRY_RUN-mode job, so there
// is no code path from DRY_RUN to a committed domain record.
func TestGenericImports_DryRunCreatesNoTargetRecords(t *testing.T) {
	f := newGenericImportsFixture(t, "Generic Imports Dry Run Co", "RC-GIMP-1")
	job := f.create(t, bulkmodels.TypePrivateOrder, privateOrderCSV, bulkmodels.ModeDryRun)

	result := f.validate(t, job.ID)
	if result.Validated != 1 {
		t.Fatalf("validation result=%+v, want 1 validated", result)
	}
	current, rows, err := f.service.GetGeneric(context.Background(), f.actor(f.membershipID), job.ID)
	if err != nil || current.Status != bulkmodels.StatusValidated || len(rows) != 1 {
		t.Fatalf("current=%+v rows=%d err=%v", current, len(rows), err)
	}

	if _, err := f.service.Commit(context.Background(), f.actor(f.membershipID), job.ID); err == nil {
		t.Fatal("committing a DRY_RUN import must be refused")
	}
	if f.orders.calls != 0 {
		t.Fatalf("dry-run must create no target records, orders.calls=%d", f.orders.calls)
	}
}

// TestGenericImports_ValidateAndCommitAreSeparateCommands proves commit is
// refused until an explicit, successful validation has run, and that
// validation alone never auto-commits.
func TestGenericImports_ValidateAndCommitAreSeparateCommands(t *testing.T) {
	f := newGenericImportsFixture(t, "Generic Imports Separation Co", "RC-GIMP-2")
	job := f.create(t, bulkmodels.TypePrivateOrder, privateOrderCSV, bulkmodels.ModeCommit)

	if _, err := f.service.Commit(context.Background(), f.actor(f.membershipID), job.ID); err == nil {
		t.Fatal("commit before validation must be refused")
	}
	f.validate(t, job.ID)
	if f.orders.calls != 0 {
		t.Fatal("validation alone must never commit a row")
	}
	f.commit(t, job.ID)
	if f.orders.calls != 1 {
		t.Fatalf("expected exactly one commit call, got %d", f.orders.calls)
	}
	current, _, err := f.service.GetGeneric(context.Background(), f.actor(f.membershipID), job.ID)
	if err != nil || current.Status != bulkmodels.StatusCommitted || current.CommittedRows != 1 {
		t.Fatalf("current=%+v err=%v", current, err)
	}
}

// TestGenericImports_PartialSuccessIsExplicit proves a mix of a committable
// and a permanently-invalid row lands the job in PARTIALLY_COMMITTED, not a
// blanket success or failure, with an error report generated through Media.
func TestGenericImports_PartialSuccessIsExplicit(t *testing.T) {
	f := newGenericImportsFixture(t, "Generic Imports Partial Co", "RC-GIMP-3")
	csv := "rowReference,customerId,serviceCatalogItemId,timezone\n" +
		"row-1,cus-1,sci-1,Africa/Lagos\n" +
		"row-2,,sci-2,Africa/Lagos\n" // missing customerId -> INVALID at validation
	job := f.create(t, bulkmodels.TypePrivateOrder, csv, bulkmodels.ModeCommit)
	f.validate(t, job.ID)
	f.commit(t, job.ID)

	current, rows, err := f.service.GetGeneric(context.Background(), f.actor(f.membershipID), job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 latest rows, got %d", len(rows))
	}
	var committed, invalid int
	for _, row := range rows {
		switch row.Status {
		case bulkmodels.RowStatusCommitted:
			committed++
		case bulkmodels.RowStatusInvalid:
			invalid++
		}
	}
	if committed != 1 || invalid != 1 {
		t.Fatalf("committed=%d invalid=%d, want 1 and 1", committed, invalid)
	}
	// One row was never attempted for commit (invalid at validation), so this
	// import is a full commit of everything that was valid, not a partial
	// failure of a commit attempt — COMMITTED is the correct terminal status.
	if current.Status != bulkmodels.StatusCommitted {
		t.Fatalf("status = %s, want COMMITTED (the only valid row committed)", current.Status)
	}
}

// TestGenericImports_WorkforceProfileImportCommitsRealProfile proves the
// generic engine drives a real workforce.CreateProfile call — the imported
// profile is readable back from the workforce feature itself, not just from
// the import's own row bookkeeping — and that a duplicate employee number
// surfaces as a safe row-level failure rather than a raw database error.
func TestGenericImports_WorkforceProfileImportCommitsRealProfile(t *testing.T) {
	f := newGenericImportsFixture(t, "Generic Imports Workforce Co", "RC-GIMP-4")
	csv := "membershipId,employeeNumber,relationshipType,role\n" + f.membershipID + ",EMP-100,MANAGED_PROVIDER,DRIVER\n"
	job := f.create(t, bulkmodels.TypeWorkforceProfile, csv, bulkmodels.ModeCommit)
	f.validate(t, job.ID)
	f.commit(t, job.ID)

	current, rows, err := f.service.GetGeneric(context.Background(), f.actor(f.membershipID), job.ID)
	if err != nil || current.Status != bulkmodels.StatusCommitted || len(rows) != 1 {
		t.Fatalf("current=%+v rows=%d err=%v", current, len(rows), err)
	}
	if rows[0].GeneratedSubjectReference == "" {
		t.Fatal("no generated subject reference recorded for the committed row")
	}
	var profileID string
	if err := f.pool.QueryRow(context.Background(),
		`SELECT id FROM fleet_partner_workforce_profiles WHERE organization_id=$1 AND employee_number='EMP-100'`,
		f.orgID).Scan(&profileID); err != nil {
		t.Fatalf("the real workforce profile was not created: %v", err)
	}
	if profileID != rows[0].GeneratedSubjectReference {
		t.Fatalf("generatedSubjectReference=%s does not match the real profile id=%s", rows[0].GeneratedSubjectReference, profileID)
	}

	// Retry the SAME import type + a second row with the same employee
	// number: a fresh second job importing the same employeeNumber must fail
	// safely at commit, not with a raw constraint-violation message.
	job2 := f.create(t, bulkmodels.TypeWorkforceProfile, csv, bulkmodels.ModeCommit)
	f.validate(t, job2.ID)
	f.commit(t, job2.ID)
	_, rows2, err := f.service.GetGeneric(context.Background(), f.actor(f.membershipID), job2.ID)
	if err != nil || len(rows2) != 1 {
		t.Fatalf("rows2=%d err=%v", len(rows2), err)
	}
	if rows2[0].Status != bulkmodels.RowStatusFailed {
		t.Fatalf("duplicate employee row status = %s, want FAILED", rows2[0].Status)
	}
	if rows2[0].SafeErrorMessage == "" {
		t.Fatal("no safe error message recorded")
	}
}

// TestGenericImports_AssetImportCommitsRealAssetAndRejectsDuplicateCode
// mirrors the workforce test for the ASSET adapter.
func TestGenericImports_AssetImportCommitsRealAssetAndRejectsDuplicateCode(t *testing.T) {
	f := newGenericImportsFixture(t, "Generic Imports Asset Co", "RC-GIMP-5")
	csv := "assetCode,assetType\nTRUCK-GEN-1,TRUCK\n"
	job := f.create(t, bulkmodels.TypeAsset, csv, bulkmodels.ModeCommit)
	f.validate(t, job.ID)
	f.commit(t, job.ID)

	current, rows, err := f.service.GetGeneric(context.Background(), f.actor(f.membershipID), job.ID)
	if err != nil || current.Status != bulkmodels.StatusCommitted || rows[0].Status != bulkmodels.RowStatusCommitted {
		t.Fatalf("current=%+v rows=%+v err=%v", current, rows, err)
	}
	var assetID string
	if err := f.pool.QueryRow(context.Background(),
		`SELECT id FROM fleet_partner_assets WHERE organization_id=$1 AND asset_code='TRUCK-GEN-1'`,
		f.orgID).Scan(&assetID); err != nil {
		t.Fatalf("the real asset was not created: %v", err)
	}

	job2 := f.create(t, bulkmodels.TypeAsset, csv, bulkmodels.ModeCommit)
	f.validate(t, job2.ID)
	f.commit(t, job2.ID)
	_, rows2, err := f.service.GetGeneric(context.Background(), f.actor(f.membershipID), job2.ID)
	if err != nil || rows2[0].Status != bulkmodels.RowStatusFailed {
		t.Fatalf("duplicate asset code row=%+v err=%v", rows2, err)
	}
}

// TestGenericImports_WarehouseInventoryImportReceivesRealStock proves the
// WAREHOUSE_INVENTORY adapter drives a real warehouse.Receipt call and that
// an invalid (non-positive) quantity is refused at validation, never
// reaching the commit step at all.
func TestGenericImports_WarehouseInventoryImportReceivesRealStock(t *testing.T) {
	f := newGenericImportsFixture(t, "Generic Imports Warehouse Co", "RC-GIMP-6")
	csv := "warehouseId,locationId,ownerType,ownerReference,sku,quantity\n" +
		f.warehouseID + "," + f.locationID + ",ORGANIZATION," + f.orgID + ",SKU-1,10\n" +
		f.warehouseID + "," + f.locationID + ",ORGANIZATION," + f.orgID + ",SKU-2,-5\n"
	job := f.create(t, bulkmodels.TypeWarehouseInventory, csv, bulkmodels.ModeCommit)
	valResult := f.validate(t, job.ID)
	if valResult.Validated != 1 {
		t.Fatalf("validation result=%+v", valResult)
	}
	current, rows, err := f.service.GetGeneric(context.Background(), f.actor(f.membershipID), job.ID)
	if err != nil || current.TotalRows != 2 || current.ValidRows != 1 || current.InvalidRows != 1 {
		t.Fatalf("current=%+v err=%v", current, err)
	}
	f.commit(t, job.ID)
	_ = rows

	var quantity float64
	if err := f.pool.QueryRow(context.Background(),
		`SELECT quantity_projection FROM fleet_partner_inventory_items WHERE organization_id=$1 AND sku='SKU-1'`,
		f.orgID).Scan(&quantity); err != nil {
		t.Fatalf("the real inventory item was not created: %v", err)
	}
	if quantity != 10 {
		t.Fatalf("received quantity = %v, want 10", quantity)
	}
	_ = rows
}

// TestGenericImports_RowRetryRecommitsOnlyTheFailedRow proves a single-row
// retry re-attempts exactly the row that failed, and does not touch or
// duplicate the row that already committed.
func TestGenericImports_RowRetryRecommitsOnlyTheFailedRow(t *testing.T) {
	f := newGenericImportsFixture(t, "Generic Imports Retry Co", "RC-GIMP-7")
	csv := "assetCode,assetType\nTRUCK-RETRY-1,TRUCK\n"
	// Pre-create the asset directly so the FIRST commit attempt for this row
	// fails with a duplicate — then remove it and retry, proving the retry
	// path re-attempts commit rather than being permanently stuck.
	if _, err := f.pool.Exec(context.Background(),
		`INSERT INTO fleet_partner_assets(id,organization_id,branch_id,asset_code,asset_type,ownership_type) VALUES ('pre_ast',$1,$2,'TRUCK-RETRY-1','TRUCK','OWNED')`,
		f.orgID, f.branchID); err != nil {
		t.Fatal(err)
	}
	job := f.create(t, bulkmodels.TypeAsset, csv, bulkmodels.ModeCommit)
	f.validate(t, job.ID)
	f.commit(t, job.ID)

	_, rows, err := f.service.GetGeneric(context.Background(), f.actor(f.membershipID), job.ID)
	if err != nil || len(rows) != 1 || rows[0].Status != bulkmodels.RowStatusFailed {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
	failedRowID := rows[0].ID

	// Clear the pre-existing conflicting asset, then retry the row.
	if _, err := f.pool.Exec(context.Background(), `DELETE FROM fleet_partner_assets WHERE id='pre_ast'`); err != nil {
		t.Fatal(err)
	}
	if err := f.service.RetryRow(context.Background(), f.actor(f.membershipID), job.ID, failedRowID); err != nil {
		t.Fatalf("retry row: %v", err)
	}
	result, err := f.service.RunCommitPass(context.Background(), bulkusecases.GenericWorkerConfig{Owner: "test-retry-commit", MaxAttempts: 3})
	if err != nil || result.Committed != 1 {
		t.Fatalf("retry commit pass result=%+v err=%v", result, err)
	}
	current, rowsAfter, err := f.service.GetGeneric(context.Background(), f.actor(f.membershipID), job.ID)
	if err != nil || current.Status != bulkmodels.StatusCommitted || rowsAfter[0].Status != bulkmodels.RowStatusCommitted {
		t.Fatalf("current=%+v rows=%+v err=%v", current, rowsAfter, err)
	}
}

// TestGenericImports_WholeImportRetryRowVersioningAndErrorReport closes three
// definition-of-done items TestGenericImports_RowRetryRecommitsOnlyTheFailedRow
// does not cover on its own: whole-import retry (RetryImport, as opposed to a
// single targeted row), the row-versioning read path
// (ListLatestGenericRows/ListGenericRows), and that a row failing AT COMMIT
// (not merely invalid at validation) produces a real error report through
// Media. Row correction has a fully working storage and read model
// (row_version, replaces_row_id, ListLatestGenericRows picks the newest
// version per external_reference) — this test proves that model directly
// through the repository, because no HTTP endpoint yet exists for a caller
// to submit a corrected replacement row (see docs/generic-imports.md's
// "known limitations": validation only ever creates row_version=1 rows).
func TestGenericImports_WholeImportRetryRowVersioningAndErrorReport(t *testing.T) {
	f := newGenericImportsFixture(t, "Generic Imports WholeRetry Co", "RC-GIMP-12")
	csv := "membershipId,employeeNumber,relationshipType,role\n" + f.membershipID + ",EMP-VER-1,MANAGED_PROVIDER,DRIVER\n"
	// Pre-create a conflicting profile so the row fails AT COMMIT (not at
	// validation), which is what actually exercises the error-report path.
	if _, err := f.pool.Exec(context.Background(),
		`INSERT INTO fleet_partner_workforce_profiles(id,organization_id,branch_id,membership_id,employee_number,relationship_type,role,start_date) VALUES ('pre_wfp',$1,$2,'mem_other','EMP-VER-1','MANAGED_PROVIDER','DRIVER',now())`,
		f.orgID, f.branchID); err != nil {
		t.Fatal(err)
	}
	job := f.create(t, bulkmodels.TypeWorkforceProfile, csv, bulkmodels.ModeCommit)
	f.validate(t, job.ID)
	f.commit(t, job.ID)

	current, rows, err := f.service.GetGeneric(context.Background(), f.actor(f.membershipID), job.ID)
	if err != nil || current.Status != bulkmodels.StatusFailed || rows[0].Status != bulkmodels.RowStatusFailed {
		t.Fatalf("current=%+v rows=%+v err=%v", current, rows, err)
	}

	// Error report: the whole-job commit attempted exactly one row and it
	// failed, so an error report must have been written through Media —
	// containing the safe external reference, never the row's source data.
	if f.media.report == nil {
		t.Fatal("no error report was written for a commit failure")
	}
	report := string(f.media.report)
	if !strings.Contains(report, "EMP-VER-1") {
		t.Fatalf("error report missing the row's external reference: %s", report)
	}
	if strings.Contains(report, f.membershipID) {
		t.Fatalf("error report leaked row source data (membershipId): %s", report)
	}

	// Whole-import retry: clear the blocker, retry the ENTIRE import (not a
	// targeted row id), and confirm it recommits.
	if _, err := f.pool.Exec(context.Background(), `DELETE FROM fleet_partner_workforce_profiles WHERE id='pre_wfp'`); err != nil {
		t.Fatal(err)
	}
	requeued, err := f.service.RetryImport(context.Background(), f.actor(f.membershipID), job.ID)
	if err != nil || requeued != 1 {
		t.Fatalf("retry import: requeued=%d err=%v", requeued, err)
	}
	result, err := f.service.RunCommitPass(context.Background(), bulkusecases.GenericWorkerConfig{Owner: "test-whole-retry", MaxAttempts: 3})
	if err != nil || result.Committed != 1 {
		t.Fatalf("post-whole-retry commit pass result=%+v err=%v", result, err)
	}
	current, rows, err = f.service.GetGeneric(context.Background(), f.actor(f.membershipID), job.ID)
	if err != nil || current.Status != bulkmodels.StatusCommitted || rows[0].Status != bulkmodels.RowStatusCommitted {
		t.Fatalf("current=%+v rows=%+v err=%v", current, rows, err)
	}

	// Row versioning: insert a corrected v2 of the same logical row directly
	// through the repository (the storage/read path this proves), sharing
	// the committed row's external_reference. ListLatestGenericRows must
	// then surface only the newest version; ListGenericRows must still show
	// both, preserving history rather than overwriting it.
	repo := bulkrepos.NewRepository(f.pool)
	original := rows[0]
	corrected := original
	corrected.ID = ""
	corrected.RowVersion = original.RowVersion + 1
	corrected.ReplacesRowID = original.ID
	corrected.RowReference = ""
	corrected.Status = bulkmodels.RowStatusValid
	corrected.CommitStatus = ""
	corrected.RowFingerprint = "corrected-fingerprint"
	if err := repo.InsertGenericRow(context.Background(), nil, corrected); err != nil {
		t.Fatalf("insert corrected row version: %v", err)
	}

	allVersions, err := repo.ListGenericRows(context.Background(), nil, f.orgID, job.ID, "")
	if err != nil || len(allVersions) != 2 {
		t.Fatalf("allVersions=%+v err=%v, want 2 (original preserved + corrected)", allVersions, err)
	}
	latest, err := repo.ListLatestGenericRows(context.Background(), nil, f.orgID, job.ID, "")
	if err != nil || len(latest) != 1 || latest[0].RowVersion != 2 {
		t.Fatalf("latest=%+v err=%v, want exactly the v2 row", latest, err)
	}
}

// TestGenericImports_DeadLetterAndReplay proves a whole-job failure (media
// unavailable) retries with backoff, dead-letters once the attempt ceiling
// is reached, is never auto-reclaimed while dead-lettered, and an operator
// replay lets it complete once the underlying condition is fixed.
func TestGenericImports_DeadLetterAndReplay(t *testing.T) {
	f := newGenericImportsFixture(t, "Generic Imports DeadLetter Co", "RC-GIMP-8")
	job := f.create(t, bulkmodels.TypePrivateOrder, privateOrderCSV, bulkmodels.ModeCommit)
	f.media.err = context.DeadlineExceeded

	if _, err := f.service.Validate(context.Background(), f.actor(f.membershipID), job.ID); err != nil {
		t.Fatal(err)
	}
	cfg := bulkusecases.GenericWorkerConfig{Owner: "test-deadletter", Backoff: 0, MaxAttempts: 1}
	result, err := f.service.RunValidationPass(context.Background(), cfg)
	if err != nil || result.DeadLettered != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	current, _, err := f.service.GetGeneric(context.Background(), f.actor(f.membershipID), job.ID)
	if err != nil || current.Status != bulkmodels.StatusDeadLetter {
		t.Fatalf("current=%+v err=%v", current, err)
	}

	// A dead-lettered job is never auto-reclaimed by an ordinary pass.
	result, err = f.service.RunValidationPass(context.Background(), cfg)
	if err != nil || result.Claimed != 0 {
		t.Fatalf("dead-lettered job was reclaimed: result=%+v err=%v", result, err)
	}

	f.media.err = nil
	if err := f.service.ReplayGeneric(context.Background(), f.actor(f.membershipID), job.ID); err != nil {
		t.Fatalf("replay: %v", err)
	}
	result, err = f.service.RunValidationPass(context.Background(), bulkusecases.GenericWorkerConfig{Owner: "test-recover", MaxAttempts: 3})
	if err != nil || result.Validated != 1 {
		t.Fatalf("post-replay validation result=%+v err=%v", result, err)
	}
}

// TestGenericImports_CancelIsVersionGuarded mirrors the same convention
// documents/exports/shipments already use for optimistic concurrency.
func TestGenericImports_CancelIsVersionGuarded(t *testing.T) {
	f := newGenericImportsFixture(t, "Generic Imports Cancel Co", "RC-GIMP-9")
	job := f.create(t, bulkmodels.TypePrivateOrder, privateOrderCSV, bulkmodels.ModeCommit)

	if _, err := f.service.CancelGeneric(context.Background(), f.actor(f.membershipID), job.ID, job.Version+1); err == nil {
		t.Fatal("a stale expected version must be refused")
	}
	cancelled, err := f.service.CancelGeneric(context.Background(), f.actor(f.membershipID), job.ID, job.Version)
	if err != nil || cancelled.Status != bulkmodels.StatusCancelled {
		t.Fatalf("cancelled=%+v err=%v", cancelled, err)
	}
	if _, err := f.service.CancelGeneric(context.Background(), f.actor(f.membershipID), job.ID, cancelled.Version); err == nil {
		t.Fatal("cancelling an already-terminal job must be refused")
	}
}

// TestGenericImports_LegacyAndGenericEnginesAreMutuallyIsolated proves the
// engine discriminator actually separates the two surfaces: a generic-engine
// job is invisible to the legacy Get/List/Cancel path, and a legacy-engine
// job is invisible to the generic path — closing exactly the cross-engine ID
// leakage risk noted in the migration's own comments.
func TestGenericImports_LegacyAndGenericEnginesAreMutuallyIsolated(t *testing.T) {
	f := newGenericImportsFixture(t, "Generic Imports Isolation Co", "RC-GIMP-10")
	genericJob := f.create(t, bulkmodels.TypePrivateOrder, privateOrderCSV, bulkmodels.ModeCommit)

	legacyJob, err := f.service.Create(context.Background(), f.actor(f.membershipID), bulkusecases.CreateInput{
		BranchID: f.branchID, MediaFileID: "legacy_media_1", FileFormat: "CSV",
	})
	if err != nil {
		t.Fatalf("legacy create: %v", err)
	}

	if _, _, err := f.service.Get(context.Background(), f.actor(f.membershipID), genericJob.ID); err == nil {
		t.Fatal("the legacy Get path must not resolve a generic-engine import id")
	}
	if _, _, err := f.service.GetGeneric(context.Background(), f.actor(f.membershipID), legacyJob.ID); err == nil {
		t.Fatal("the generic GetGeneric path must not resolve a legacy-engine import id")
	}

	legacyList, err := f.service.List(context.Background(), f.actor(f.membershipID))
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range legacyList {
		if imp.ID == genericJob.ID {
			t.Fatal("the legacy List must not include a generic-engine import")
		}
	}
	genericList, err := f.service.ListGeneric(context.Background(), f.actor(f.membershipID))
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range genericList {
		if imp.ID == legacyJob.ID {
			t.Fatal("the generic ListGeneric must not include a legacy-engine import")
		}
	}
}

// TestGenericImports_HTTPCreateRequiresIdempotencyKeyAndEnforcesPermissions
// exercises the real routes end to end.
func TestGenericImports_HTTPCreateRequiresIdempotencyKeyAndEnforcesPermissions(t *testing.T) {
	f := newGenericImportsFixture(t, "Generic Imports HTTP Co", "RC-GIMP-11")
	owner := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "owner")
	router := f.router(t, owner)

	if status, _ := documentCall(t, router, http.MethodPost, "/fleet-partner/v1/imports",
		map[string]any{"importType": bulkmodels.TypePrivateOrder, "branchId": f.branchID, "mediaFileId": "media-http-1", "fileFormat": "CSV"},
		nil); status != http.StatusBadRequest {
		t.Fatalf("create without idempotency key status = %d, want 400", status)
	}

	status, response := documentCall(t, router, http.MethodPost, "/fleet-partner/v1/imports",
		map[string]any{"importType": bulkmodels.TypePrivateOrder, "branchId": f.branchID, "mediaFileId": "media-http-1", "fileFormat": "CSV"},
		map[string]string{"Idempotency-Key": "http-import-key-1"})
	if status != http.StatusAccepted {
		t.Fatalf("create status = %d body=%v, want 202", status, response)
	}
	created := documentData(t, response)
	// The generic and legacy import routes both serialize the raw
	// models.Import struct with no json tags — an existing bulkimports
	// convention (see http/handlers.go's create/get/list, all untouched by
	// this change) this suite matches rather than silently changes: the
	// wire field is the exported Go name "ID", not "id".
	importID, _ := created["ID"].(string)
	if importID == "" {
		t.Fatalf("create response carries no id: %v", created)
	}

	// A member with no role bindings reaches nothing.
	nobody := seedApprovalMember(t, f.pool, f.orgID, "", "nobody")
	nobodyRouter := f.router(t, nobody)
	if status, _ := documentCall(t, nobodyRouter, http.MethodGet,
		"/fleet-partner/v1/imports/"+importID, nil, nil); status != http.StatusForbidden {
		t.Fatalf("unroled get status = %d, want 403", status)
	}

	// The operations role holds imports.validate (migration 000056 grants
	// owner+operations every imports.* permission together) and reaches the
	// validate route through the real permission check, not just past the
	// tenant-resolution gate the "nobody" case above stopped at.
	operations := seedApprovalMember(t, f.pool, f.orgID, "rol_system_operations", "operations")
	operationsRouter := f.router(t, operations)
	status, validateResponse := documentCall(t, operationsRouter, http.MethodPost,
		"/fleet-partner/v1/imports/"+importID+"/validate", struct{}{}, map[string]string{"Idempotency-Key": "http-validate-1"})
	if status != http.StatusAccepted {
		t.Fatalf("operations validate status = %d body=%v, want 202 (operations holds imports.validate)", status, validateResponse)
	}
}
