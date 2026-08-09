package integration_test

import (
	"context"
	"net/http"
	"testing"

	bulkmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/bulkimports/models"
	bulkusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/bulkimports/usecases"
)

// TestGenericImports_CorrectedRowFullLifecycle proves the whole flow the
// spec asks for: the original row is preserved unchanged, the corrected row
// references it, the correction validates through the same adapter, it
// commits a real domain record, and a second commit pass creates no
// duplicate.
func TestGenericImports_CorrectedRowFullLifecycle(t *testing.T) {
	f := newGenericImportsFixture(t, "Generic Imports Correction Co", "RC-GIMP-13")
	// assetType left blank -> INVALID at validation.
	csv := "assetCode,assetType\nTRUCK-COR-1,\n"
	job := f.create(t, bulkmodels.TypeAsset, csv, bulkmodels.ModeCommit)
	f.validate(t, job.ID)

	_, rows, err := f.service.GetGeneric(context.Background(), f.actor(f.membershipID), job.ID)
	if err != nil || len(rows) != 1 || rows[0].Status != bulkmodels.RowStatusInvalid {
		t.Fatalf("rows=%+v err=%v, want 1 INVALID row", rows, err)
	}
	original := rows[0]

	corrected, err := f.service.SubmitCorrection(context.Background(), f.actor(f.membershipID), bulkusecases.SubmitCorrectionInput{
		ImportID: job.ID, RowID: original.ID, ExpectedVersion: original.RowVersion,
		RowData: map[string]string{"assetCode": "TRUCK-COR-1", "assetType": "TRUCK"},
	})
	if err != nil {
		t.Fatalf("submit correction: %v", err)
	}
	if corrected.Status != bulkmodels.RowStatusValid {
		t.Fatalf("corrected row status = %s, want VALID", corrected.Status)
	}
	if corrected.ReplacesRowID != original.ID {
		t.Fatalf("corrected.ReplacesRowID = %s, want %s", corrected.ReplacesRowID, original.ID)
	}
	if corrected.ExternalReference != original.ExternalReference {
		t.Fatal("corrected row must share the original's external reference")
	}
	if corrected.RowVersion != original.RowVersion+1 {
		t.Fatalf("corrected.RowVersion = %d, want %d", corrected.RowVersion, original.RowVersion+1)
	}

	// The original row is untouched.
	var origStatus string
	var origVersion int
	if err := f.pool.QueryRow(context.Background(),
		`SELECT status, row_version FROM fleet_partner_bulk_import_rows WHERE id=$1`, original.ID).
		Scan(&origStatus, &origVersion); err != nil {
		t.Fatal(err)
	}
	if origStatus != bulkmodels.RowStatusInvalid || origVersion != original.RowVersion {
		t.Fatalf("original row was mutated: status=%s version=%d", origStatus, origVersion)
	}

	// Commit: the corrected row is what gets picked up (it's the latest
	// version and it's VALID); the original INVALID version is not.
	result, err := f.service.RunCommitPass(context.Background(), bulkusecases.GenericWorkerConfig{Owner: "test-correction-commit", MaxAttempts: 3})
	if err != nil || result.Committed != 1 {
		t.Fatalf("commit pass result=%+v err=%v", result, err)
	}
	var assetID string
	if err := f.pool.QueryRow(context.Background(),
		`SELECT id FROM fleet_partner_assets WHERE organization_id=$1 AND asset_code='TRUCK-COR-1'`,
		f.orgID).Scan(&assetID); err != nil {
		t.Fatalf("the real asset was not created: %v", err)
	}

	// A second commit pass must create no duplicate: the corrected row is
	// now COMMITTED, so ListLatestGenericRows(status=VALID) finds nothing.
	result, err = f.service.RunCommitPass(context.Background(), bulkusecases.GenericWorkerConfig{Owner: "test-correction-commit-2", MaxAttempts: 3})
	if err != nil {
		t.Fatalf("second commit pass: %v", err)
	}
	if result.Claimed != 0 {
		t.Fatalf("second commit pass claimed %d jobs, want 0 (nothing left to commit)", result.Claimed)
	}
	var assetCount int
	if err := f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM fleet_partner_assets WHERE organization_id=$1 AND asset_code='TRUCK-COR-1'`,
		f.orgID).Scan(&assetCount); err != nil {
		t.Fatal(err)
	}
	if assetCount != 1 {
		t.Fatalf("asset_code TRUCK-COR-1 has %d rows, want exactly 1 (no duplicate)", assetCount)
	}
}

// TestGenericImports_CorrectionRejectsStaleVersionAndCommittedRow covers the
// two hard refusals the spec calls out explicitly.
func TestGenericImports_CorrectionRejectsStaleVersionAndCommittedRow(t *testing.T) {
	f := newGenericImportsFixture(t, "Generic Imports Correction Refusal Co", "RC-GIMP-14")
	csv := "assetCode,assetType\nTRUCK-COR-2,\n"
	job := f.create(t, bulkmodels.TypeAsset, csv, bulkmodels.ModeCommit)
	f.validate(t, job.ID)
	_, rows, err := f.service.GetGeneric(context.Background(), f.actor(f.membershipID), job.ID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
	invalidRow := rows[0]

	if _, err := f.service.SubmitCorrection(context.Background(), f.actor(f.membershipID), bulkusecases.SubmitCorrectionInput{
		ImportID: job.ID, RowID: invalidRow.ID, ExpectedVersion: invalidRow.RowVersion + 1,
		RowData: map[string]string{"assetCode": "TRUCK-COR-2", "assetType": "TRUCK"},
	}); err == nil {
		t.Fatal("a stale expected version must be refused")
	}

	// A committed row cannot be corrected: build a second, valid, committed
	// row and try to "correct" it.
	csv2 := "assetCode,assetType\nTRUCK-COR-3,TRUCK\n"
	job2 := f.create(t, bulkmodels.TypeAsset, csv2, bulkmodels.ModeCommit)
	f.validate(t, job2.ID)
	f.commit(t, job2.ID)
	_, rows2, err := f.service.GetGeneric(context.Background(), f.actor(f.membershipID), job2.ID)
	if err != nil || rows2[0].Status != bulkmodels.RowStatusCommitted {
		t.Fatalf("rows2=%+v err=%v, want a COMMITTED row", rows2, err)
	}
	if _, err := f.service.SubmitCorrection(context.Background(), f.actor(f.membershipID), bulkusecases.SubmitCorrectionInput{
		ImportID: job2.ID, RowID: rows2[0].ID, ExpectedVersion: rows2[0].RowVersion,
		RowData: map[string]string{"assetCode": "TRUCK-COR-3", "assetType": "VAN"},
	}); err == nil {
		t.Fatal("a committed row must never be correctable")
	}
}

// TestGenericImports_CorrectionHTTPTenantIsolationAndIdempotency exercises
// the real route: cross-tenant access is refused, a retried identical
// correction replays the same result, and the same idempotency key with
// different row data is a conflict.
func TestGenericImports_CorrectionHTTPTenantIsolationAndIdempotency(t *testing.T) {
	f := newGenericImportsFixture(t, "Generic Imports Correction HTTP Co", "RC-GIMP-15")
	csv := "assetCode,assetType\nTRUCK-COR-4,\n"
	job := f.create(t, bulkmodels.TypeAsset, csv, bulkmodels.ModeCommit)
	f.validate(t, job.ID)
	_, rows, err := f.service.GetGeneric(context.Background(), f.actor(f.membershipID), job.ID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
	rowID := rows[0].ID

	owner := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "owner")
	router := f.router(t, owner)
	path := "/fleet-partner/v1/imports/" + job.ID + "/rows/" + rowID + "/corrections"

	// A member of a DIFFERENT organization (same database, same fixture's
	// pool — see routerForOrg's doc comment for why this isn't a second
	// newGenericImportsFixture call) must not reach this row at all.
	otherOrg := createOrganization(t, f.pool, "Generic Imports Correction Other Co", "RC-GIMP-15B")
	otherOwner := seedApprovalMember(t, f.pool, otherOrg.ID, "rol_system_owner", "otherowner")
	otherRouter := f.routerForOrg(t, otherOrg.ID, otherOwner)
	if status, _ := documentCall(t, otherRouter, http.MethodPost, path,
		map[string]any{"expectedVersion": rows[0].RowVersion, "rowData": map[string]string{"assetCode": "TRUCK-COR-4", "assetType": "TRUCK"}},
		map[string]string{"Idempotency-Key": "corr-cross-tenant"}); status != http.StatusNotFound {
		t.Fatalf("cross-tenant correction status = %d, want 404", status)
	}

	body := map[string]any{"expectedVersion": rows[0].RowVersion, "rowData": map[string]string{"assetCode": "TRUCK-COR-4", "assetType": "TRUCK"}}
	status, response := documentCall(t, router, http.MethodPost, path, body, map[string]string{"Idempotency-Key": "corr-key-1"})
	if status != http.StatusCreated {
		t.Fatalf("correction status = %d body=%v, want 201", status, response)
	}
	correctedID, _ := documentData(t, response)["ID"].(string)
	if correctedID == "" {
		t.Fatalf("correction response carries no id: %v", response)
	}

	// Identical retry replays the same result.
	status, replay := documentCall(t, router, http.MethodPost, path, body, map[string]string{"Idempotency-Key": "corr-key-1"})
	if status != http.StatusCreated || documentData(t, replay)["ID"] != correctedID {
		t.Fatalf("retry status=%d body=%v, want the same corrected row replayed", status, replay)
	}

	// Same key, different payload: conflict.
	differentBody := map[string]any{"expectedVersion": rows[0].RowVersion, "rowData": map[string]string{"assetCode": "TRUCK-COR-4", "assetType": "VAN"}}
	status, _ = documentCall(t, router, http.MethodPost, path, differentBody, map[string]string{"Idempotency-Key": "corr-key-1"})
	if status != http.StatusConflict {
		t.Fatalf("same key different payload status = %d, want 409", status)
	}
}
