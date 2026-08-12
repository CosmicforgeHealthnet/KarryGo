package integration_test

import (
	"context"
	"sync"
	"testing"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	manifestmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/manifests/models"
	manifestrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/manifests/repositories"
	manifestusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/manifests/usecases"
	shipmentsusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
)

type manifestResourcesStub struct {
	itemID, shipmentID, branchID string
}

func (s manifestResourcesStub) Resolve(_ context.Context, _ database.Querier, _ string,
	_ string, id string) (manifestusecases.ResourceInfo, error) {
	if id != s.itemID {
		return manifestusecases.ResourceInfo{}, manifestrepos.ErrNotFound
	}
	return manifestusecases.ResourceInfo{ID: id, ShipmentID: s.shipmentID, BranchID: s.branchID}, nil
}

type manifestAuthorityStub struct{ valid bool }

func (s manifestAuthorityStub) Validate(context.Context, string, string, string) (bool, error) {
	return s.valid, nil
}

type manifestMatrixFixture struct {
	*shipmentsFixture
	service    *manifestusecases.Service
	repository *manifestrepos.Repository
	branchID   string
	shipmentID string
}

func newManifestMatrixFixture(t *testing.T, authority manifestusecases.AuthorityPort) *manifestMatrixFixture {
	t.Helper()
	f := newShipmentsFixture(t)
	ctx := context.Background()
	branchID := "brn_manifest_matrix"
	if _, err := f.pool.Exec(ctx, `INSERT INTO fleet_partner_branches(id,organization_id,code,name,timezone)
		VALUES($1,$2,'MANIFEST-MATRIX','Manifest Matrix','Africa/Lagos')`, branchID,
		f.tenant.OrganizationID); err != nil {
		t.Fatal(err)
	}
	customer := f.customer(t, "Manifest Matrix Co")
	item := f.activeCatalogItem(t, "MANIFEST_MATRIX")
	order, pickupID, deliveryID := f.confirmedOrderWithStops(t, customer.ID, item.ID)
	shipment, err := f.shipments.Create(ctx, f.tenant, shipmentsusecases.CreateInput{
		OrderID: order.ID, OriginStopID: pickupID, DestinationStopID: deliveryID,
		DeclaredValueCurrency: "NGN"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(ctx, `UPDATE fleet_partner_shipments SET branch_id=$1 WHERE id=$2`, branchID, shipment.ID); err != nil {
		t.Fatal(err)
	}
	f.tenant.PermittedBranchIDs = []string{branchID}
	repo := manifestrepos.NewRepository(f.pool)
	service := manifestusecases.NewService(manifestusecases.Options{Pool: f.pool, Repository: repo,
		Resources: manifestResourcesStub{itemID: shipment.ID, shipmentID: shipment.ID, branchID: branchID},
		Authority: authority, Audit: audit.NewRecorder(),
		Flags: manifestusecases.Flags{Enabled: true, WritesEnabled: true}})
	return &manifestMatrixFixture{shipmentsFixture: f, service: service, repository: repo,
		branchID: branchID, shipmentID: shipment.ID}
}

func (f *manifestMatrixFixture) manifestWithItem(t *testing.T, suffix string,
	provider, vehicle string) manifestmodels.Manifest {
	t.Helper()
	m, err := f.service.Create(context.Background(), f.tenant, manifestusecases.CreateInput{
		BranchID: f.branchID, RouteReference: "route-" + suffix, ProviderMembershipID: provider,
		VehicleReference: vehicle, SealNumber: "SEAL-" + suffix})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.AddItem(context.Background(), f.tenant, m.ID,
		manifestusecases.AddItemInput{ItemType: "SHIPMENT", ItemID: f.shipmentID}); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestManifests_TenantBranchMembershipAndAuthority(t *testing.T) {
	f := newManifestMatrixFixture(t, manifestAuthorityStub{valid: false})
	ctx := context.Background()
	m := f.manifestWithItem(t, "scope", "membership-invalid", "vehicle-invalid")
	if _, _, err := f.service.Get(ctx, f.other, m.ID); err == nil {
		t.Fatal("cross-tenant manifest access succeeded")
	}
	wrongBranch := f.tenant
	wrongBranch.PermittedBranchIDs = []string{"brn_other"}
	if _, _, err := f.service.Get(ctx, wrongBranch, m.ID); err == nil {
		t.Fatal("cross-branch manifest access succeeded")
	}
	if _, _, err := f.service.Close(ctx, f.tenant, m.ID, manifestusecases.CloseInput{
		ExpectedVersion: m.Version, Fingerprint: "authority"}); ordersErrorCode(err) != "manifest_not_closeable" {
		t.Fatalf("invalid provider/vehicle authority err=%v", err)
	}

	second, err := f.service.Create(ctx, f.tenant, manifestusecases.CreateInput{BranchID: f.branchID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.AddItem(ctx, f.tenant, second.ID, manifestusecases.AddItemInput{
		ItemType: "SHIPMENT", ItemID: f.shipmentID}); ordersErrorCode(err) != "manifest_item_conflict" {
		t.Fatalf("duplicate active membership err=%v", err)
	}
	mismatchService := manifestusecases.NewService(manifestusecases.Options{Pool: f.pool,
		Repository: f.repository, Resources: manifestResourcesStub{itemID: "shipment-mismatch",
			shipmentID: f.shipmentID, branchID: "brn_wrong"}, Audit: audit.NewRecorder(),
		Flags: manifestusecases.Flags{Enabled: true, WritesEnabled: true}})
	third, err := mismatchService.Create(ctx, f.tenant, manifestusecases.CreateInput{BranchID: f.branchID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = mismatchService.AddItem(ctx, f.tenant, third.ID, manifestusecases.AddItemInput{
		ItemType: "SHIPMENT", ItemID: "shipment-mismatch"}); ordersErrorCode(err) != "manifest_item_conflict" {
		t.Fatalf("package/shipment branch mismatch err=%v", err)
	}
}

func TestManifests_ConcurrentCloseReplaySnapshotAndMutationGuards(t *testing.T) {
	f := newManifestMatrixFixture(t, manifestAuthorityStub{valid: true})
	ctx := context.Background()
	m := f.manifestWithItem(t, "close", "", "")
	type closeResult struct {
		manifest manifestmodels.Manifest
		snapshot manifestmodels.Snapshot
		err      error
	}
	results := make(chan closeResult, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			closed, snapshot, err := f.service.Close(ctx, f.tenant, m.ID,
				manifestusecases.CloseInput{ExpectedVersion: m.Version, Fingerprint: "same-close"})
			results <- closeResult{closed, snapshot, err}
		}()
	}
	wg.Wait()
	close(results)
	var closed manifestmodels.Manifest
	var snapshotID string
	for result := range results {
		if result.err != nil {
			t.Fatalf("identical concurrent close failed: %v", result.err)
		}
		closed = result.manifest
		if snapshotID != "" && snapshotID != result.snapshot.ID {
			t.Fatal("identical close replay created another snapshot")
		}
		snapshotID = result.snapshot.ID
	}
	if closed.Status != manifestmodels.StatusClosed || snapshotID == "" {
		t.Fatalf("closed=%+v snapshot=%s", closed, snapshotID)
	}
	if _, _, err := f.service.Close(ctx, f.tenant, m.ID, manifestusecases.CloseInput{
		ExpectedVersion: m.Version, Fingerprint: "changed-close"}); ordersErrorCode(err) != "manifest_already_closed" {
		t.Fatalf("changed close fingerprint err=%v", err)
	}
	if _, err := f.service.Update(ctx, f.tenant, m.ID, manifestusecases.UpdateInput{
		ExpectedVersion: closed.Version, SealNumber: "ILLEGAL"}); ordersErrorCode(err) != "manifest_invalid_state" {
		t.Fatalf("closed manifest edit err=%v", err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE fleet_partner_manifest_snapshots SET snapshot='{}' WHERE id=$1`, snapshotID); err == nil {
		t.Fatal("snapshot immutability trigger allowed update")
	}
	amended, err := f.service.AmendSeal(ctx, f.tenant, m.ID, manifestusecases.SealAmendmentInput{
		SealNumber: "SEAL-REPLACED", ReasonCode: "BROKEN_SEAL", ExpectedVersion: closed.Version})
	if err != nil || amended.SealNumber != "SEAL-REPLACED" {
		t.Fatalf("seal amendment=%+v err=%v", amended, err)
	}
	if _, err = f.repository.SetStatus(ctx, nil, f.tenant.OrganizationID, m.ID, amended.Version,
		manifestmodels.StatusDispatched, f.tenant.ActorID); err != nil {
		t.Fatal(err)
	}
	current, err := f.repository.Get(ctx, nil, f.tenant.OrganizationID, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Update(ctx, f.tenant, m.ID, manifestusecases.UpdateInput{
		ExpectedVersion: current.Version, RouteReference: "mutated"}); ordersErrorCode(err) != "manifest_invalid_state" {
		t.Fatalf("dispatched manifest mutation err=%v", err)
	}
}

func TestManifests_CloseVersusEditAndCancellationPreservesSnapshotHistory(t *testing.T) {
	f := newManifestMatrixFixture(t, manifestAuthorityStub{valid: true})
	ctx := context.Background()
	m := f.manifestWithItem(t, "race", "", "")
	results := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _, err := f.service.Close(ctx, f.tenant, m.ID,
			manifestusecases.CloseInput{ExpectedVersion: m.Version, Fingerprint: "race-close"})
		results <- err
	}()
	go func() {
		defer wg.Done()
		_, err := f.service.Update(ctx, f.tenant, m.ID,
			manifestusecases.UpdateInput{ExpectedVersion: m.Version, RouteReference: "race-edit"})
		results <- err
	}()
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("close-versus-edit successes=%d want=1", successes)
	}

	current, _, err := f.service.Get(ctx, f.tenant, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != manifestmodels.StatusClosed {
		current, _, err = f.service.Close(ctx, f.tenant, m.ID, manifestusecases.CloseInput{
			ExpectedVersion: current.Version, Fingerprint: "race-close-final"})
		if err != nil {
			t.Fatal(err)
		}
	}
	cancelled, err := f.service.Cancel(ctx, f.tenant, m.ID, manifestusecases.CancelInput{
		ExpectedVersion: current.Version, ReasonCode: "ROUTE_CANCELLED"})
	if err != nil || cancelled.Status != manifestmodels.StatusCancelled {
		t.Fatalf("cancelled=%+v err=%v", cancelled, err)
	}
	var snapshots, histories int
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_manifest_snapshots WHERE manifest_id=$1`, m.ID).Scan(&snapshots); err != nil || snapshots != 1 {
		t.Fatalf("snapshots=%d err=%v", snapshots, err)
	}
	if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_manifest_history WHERE manifest_id=$1`, m.ID).Scan(&histories); err != nil || histories < 3 {
		t.Fatalf("history=%d err=%v", histories, err)
	}
}
