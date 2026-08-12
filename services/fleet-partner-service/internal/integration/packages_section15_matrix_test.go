package integration_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/app"
	packagesmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/packages/models"
	packagesrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/packages/repositories"
	packagesusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/packages/usecases"
	shipmentsrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/repositories"
	shipmentsusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

type trustedDeviceStub struct{ valid bool }

func (s trustedDeviceStub) ValidateTrustedDevice(context.Context, tenant.Context, string) (bool, error) {
	return s.valid, nil
}

type providerScopeStub struct{ valid bool }

func (s providerScopeStub) ValidatePackageScope(context.Context, tenant.Context, string, string) (bool, error) {
	return s.valid, nil
}

type evidenceStub struct{ valid bool }

func (s evidenceStub) ValidateEvidence(context.Context, tenant.Context, string) (bool, error) {
	return s.valid, nil
}

type packagesMatrixFixture struct {
	*shipmentsFixture
	service    *packagesusecases.Service
	repository *packagesrepos.Repository
	shipmentID string
}

func newPackagesMatrixFixture(t *testing.T) *packagesMatrixFixture {
	t.Helper()
	f := newShipmentsFixture(t)
	ctx := context.Background()
	customer := f.customer(t, "Package Matrix Co")
	item := f.activeCatalogItem(t, "PACKAGE_MATRIX")
	order, pickupID, deliveryID := f.confirmedOrderWithStops(t, customer.ID, item.ID)
	shipment, err := f.shipments.Create(ctx, f.tenant, shipmentsusecases.CreateInput{
		OrderID: order.ID, OriginStopID: pickupID, DestinationStopID: deliveryID,
		DeclaredValueCurrency: "NGN"})
	if err != nil {
		t.Fatal(err)
	}
	repo := packagesrepos.NewRepository(f.pool)
	service := packagesusecases.NewService(packagesusecases.Options{
		Pool: f.pool, Repository: repo,
		Shipments: app.NewPackagesShipmentsAdapter(shipmentsrepos.NewRepository(f.pool)),
		Devices:   trustedDeviceStub{valid: true}, ProviderScope: providerScopeStub{valid: true},
		Media: evidenceStub{valid: true}, Audit: audit.NewRecorder(),
		Flags: packagesusecases.Flags{Enabled: true, WritesEnabled: true, ScanningEnabled: true},
	})
	return &packagesMatrixFixture{shipmentsFixture: f, service: service, repository: repo, shipmentID: shipment.ID}
}

func packageInput(shipmentID string) packagesusecases.CreateInput {
	return packagesusecases.CreateInput{ShipmentID: shipmentID, PackageType: "PARCEL",
		Length: "10", Width: "11", Height: "12", DimensionUnit: "CM", Weight: "2.5",
		WeightUnit: "KG", DeclaredValueCurrency: "NGN"}
}

func (f *packagesMatrixFixture) createPackage(t *testing.T) packagesmodels.Package {
	t.Helper()
	p, err := f.service.Create(context.Background(), f.tenant, packageInput(f.shipmentID))
	if err != nil {
		t.Fatalf("create package: %v", err)
	}
	return p
}

func TestPackages_PostgreSQLIdentifiersConcurrencyAndTenantScope(t *testing.T) {
	f := newPackagesMatrixFixture(t)
	ctx := context.Background()
	first := f.createPackage(t)
	for name, column := range map[string]string{
		"package-code": "package_code", "barcode": "barcode_value", "qr": "qr_value",
	} {
		t.Run(name+"-uniqueness", func(t *testing.T) {
			query := `INSERT INTO fleet_partner_packages(id,organization_id,branch_id,shipment_id,
				package_code,barcode_value,qr_value,package_type,length,width,height,dimension_unit,weight,
				weight_unit,declared_value_currency,created_by)
				SELECT $1,organization_id,branch_id,shipment_id,
				CASE WHEN $2='package_code' THEN package_code ELSE package_code||$1 END,
				CASE WHEN $2='barcode_value' THEN barcode_value ELSE barcode_value||$1 END,
				CASE WHEN $2='qr_value' THEN qr_value ELSE qr_value||$1 END,
				package_type,length,width,height,dimension_unit,weight,weight_unit,declared_value_currency,created_by
				FROM fleet_partner_packages WHERE id=$3`
			if _, err := f.pool.Exec(ctx, query, "pkg_duplicate_"+column, column, first.ID); err == nil {
				t.Fatalf("%s uniqueness was not enforced", column)
			}
		})
	}

	var wg sync.WaitGroup
	ids := make(chan string, 2)
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, err := f.service.Create(ctx, f.tenant, packageInput(f.shipmentID))
			if err != nil {
				errs <- err
				return
			}
			ids <- p.ID
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	if err := <-errs; err != nil {
		t.Fatalf("concurrent create: %v", err)
	}
	seen := map[string]bool{}
	for id := range ids {
		if seen[id] {
			t.Fatal("concurrent creation reused a package identifier")
		}
		seen[id] = true
	}
	if len(seen) != 2 {
		t.Fatalf("created identifiers=%d want=2", len(seen))
	}
	if _, err := f.service.Get(ctx, f.other, first.ID); err == nil {
		t.Fatal("cross-tenant package read succeeded")
	}
	if _, err := f.service.Transition(ctx, f.other, first.ID, packagesusecases.TransitionInput{
		NewStatus: packagesmodels.StatusReceived, ExpectedVersion: first.Version}); err == nil {
		t.Fatal("cross-tenant package mutation succeeded")
	}
	branchScoped := f.tenant
	branchScoped.PermittedBranchIDs = []string{"brn_not_permitted"}
	if _, err := f.service.Get(ctx, branchScoped, first.ID); err == nil {
		t.Fatal("cross-branch package read succeeded")
	}
}

func TestPackages_TrustedDeviceProviderScopeOfflineAndReview(t *testing.T) {
	f := newPackagesMatrixFixture(t)
	ctx := context.Background()
	p := f.createPackage(t)
	now := time.Now().UTC()
	base := packagesusecases.ScanInput{ScanType: "RECEIVED", Barcode: p.BarcodeValue,
		DeviceReference: "device-1", ClientScanID: "scan-client-1", OccurredAt: now}
	first, err := f.service.Scan(ctx, f.tenant, p.ID, base)
	if err != nil || first.Disposition != "APPLIED" {
		t.Fatalf("offline scan=%+v err=%v", first, err)
	}
	duplicate, err := f.service.Scan(ctx, f.tenant, p.ID, base)
	if err != nil || duplicate.ID != first.ID {
		t.Fatalf("duplicate scan=%+v err=%v", duplicate, err)
	}
	changed := base
	changed.ScanType = "SORTED"
	if _, err = f.service.Scan(ctx, f.tenant, p.ID, changed); ordersErrorCode(err) != "package_scan_conflict" {
		t.Fatalf("changed scan fingerprint err=%v", err)
	}
	outOfOrder := packagesusecases.ScanInput{ScanType: "SORTED", Barcode: p.BarcodeValue,
		DeviceReference: "device-1", ClientScanID: "scan-client-2", OccurredAt: now.Add(-time.Minute)}
	review, err := f.service.Scan(ctx, f.tenant, p.ID, outOfOrder)
	if err != nil || review.Disposition != "REVIEW_REQUIRED" {
		t.Fatalf("out-of-order scan=%+v err=%v", review, err)
	}
	if _, err = f.service.Scan(ctx, f.tenant, p.ID, packagesusecases.ScanInput{
		ScanType: "DELIVERED", Barcode: p.BarcodeValue, DeviceReference: "device-1",
		ClientScanID: "scan-delivery", OccurredAt: now}); err == nil {
		t.Fatal("scan-only delivery was accepted")
	}

	untrusted := packagesusecases.NewService(packagesusecases.Options{Pool: f.pool,
		Repository: f.repository, Shipments: app.NewPackagesShipmentsAdapter(shipmentsrepos.NewRepository(f.pool)),
		Devices: trustedDeviceStub{valid: false}, Flags: packagesusecases.Flags{Enabled: true,
			WritesEnabled: true, ScanningEnabled: true}})
	if _, err = untrusted.Scan(ctx, f.tenant, p.ID, packagesusecases.ScanInput{ScanType: "SORTED",
		Barcode: p.BarcodeValue, DeviceReference: "revoked", ClientScanID: "scan-revoked",
		OccurredAt: now}); ordersErrorCode(err) != "package_scan_device_untrusted" {
		t.Fatalf("untrusted device err=%v", err)
	}
	provider := f.tenant
	provider.ActorType, provider.ActorID = tenant.ActorTypePlatformService, "provider-out-of-scope"
	denied := packagesusecases.NewService(packagesusecases.Options{Pool: f.pool,
		Repository: f.repository, Shipments: app.NewPackagesShipmentsAdapter(shipmentsrepos.NewRepository(f.pool)),
		Devices: trustedDeviceStub{valid: true}, ProviderScope: providerScopeStub{valid: false},
		Flags: packagesusecases.Flags{Enabled: true, WritesEnabled: true, ScanningEnabled: true}})
	if _, err = denied.Scan(ctx, provider, p.ID, packagesusecases.ScanInput{ScanType: "SORTED",
		Barcode: p.BarcodeValue, DeviceReference: "provider-device", ClientScanID: "scan-provider",
		OccurredAt: now.Add(time.Minute)}); ordersErrorCode(err) != "package_not_in_provider_scope" {
		t.Fatalf("provider scope err=%v", err)
	}
}

func TestPackages_CustodyContinuityCorrectionEvidenceAndAppendOnly(t *testing.T) {
	f := newPackagesMatrixFixture(t)
	ctx := context.Background()
	p := f.createPackage(t)
	now := time.Now().UTC()
	first, err := f.service.RecordCustody(ctx, f.tenant, p.ID, packagesusecases.CustodyInput{
		ToActorType: "WAREHOUSE", ToActorReference: "warehouse-1", Condition: "GOOD",
		OccurredAt: now, EvidenceMediaID: "media-owned", Source: "TEST"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.RecordCustody(ctx, f.tenant, p.ID, packagesusecases.CustodyInput{
		FromActorType: "PROVIDER", FromActorReference: "wrong-holder", ToActorType: "WAREHOUSE",
		ToActorReference: "warehouse-2", Condition: "GOOD", OccurredAt: now.Add(time.Minute), Source: "TEST"}); ordersErrorCode(err) != "custody_sequence_invalid" {
		t.Fatalf("impossible custody sequence err=%v", err)
	}
	correction, err := f.service.RecordCustody(ctx, f.tenant, p.ID, packagesusecases.CustodyInput{
		FromActorType: "", FromActorReference: "", ToActorType: "WAREHOUSE",
		ToActorReference: "warehouse-corrected", Condition: "GOOD", OccurredAt: now,
		CorrectionOfEventID: first.ID, Source: "CORRECTION"})
	if err != nil || correction.CorrectionOfEventID != first.ID {
		t.Fatalf("correction=%+v err=%v", correction, err)
	}
	if _, err = f.service.RecordCustody(ctx, f.tenant, p.ID, packagesusecases.CustodyInput{
		ToActorType: "WAREHOUSE", ToActorReference: "invalid", Condition: "GOOD", OccurredAt: now,
		CorrectionOfEventID: "cue_other", Source: "CORRECTION"}); ordersErrorCode(err) != "custody_correction_invalid" {
		t.Fatalf("invalid correction err=%v", err)
	}
	if _, err = f.pool.Exec(ctx, `UPDATE fleet_partner_custody_events SET condition='DAMAGED' WHERE id=$1`, first.ID); err == nil {
		t.Fatal("custody append-only trigger allowed update")
	}
	invalidMedia := packagesusecases.NewService(packagesusecases.Options{Pool: f.pool,
		Repository: f.repository, Shipments: app.NewPackagesShipmentsAdapter(shipmentsrepos.NewRepository(f.pool)),
		Media: evidenceStub{valid: false}, Devices: trustedDeviceStub{valid: true},
		Flags: packagesusecases.Flags{Enabled: true, WritesEnabled: true, ScanningEnabled: true}})
	if _, err = invalidMedia.RecordCustody(ctx, f.tenant, p.ID, packagesusecases.CustodyInput{
		ToActorType: "WAREHOUSE", ToActorReference: "warehouse", Condition: "GOOD",
		OccurredAt: now.Add(2 * time.Minute), EvidenceMediaID: "foreign-media"}); err == nil {
		t.Fatal("foreign custody evidence was accepted")
	}
}

func TestPackages_ConcurrentTransitionSingleWinner(t *testing.T) {
	f := newPackagesMatrixFixture(t)
	p := f.createPackage(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, status := range []string{packagesmodels.StatusReceived, packagesmodels.StatusDamaged} {
		wg.Add(1)
		go func(next string) {
			defer wg.Done()
			_, err := f.service.Transition(ctx, f.tenant, p.ID, packagesusecases.TransitionInput{
				NewStatus: next, ReasonCode: "CONCURRENT_TEST", ExpectedVersion: p.Version})
			results <- err
		}(status)
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent transition successes=%d want=1", successes)
	}
}
