package vendors

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/audit"
	"cosmicforge/logistics/services/vendor-service/internal/database"
	"cosmicforge/logistics/services/vendor-service/internal/events"
)

type allowAll struct{}

func (allowAll) Authorize(context.Context, string, string, *string) (bool, error) { return true, nil }

type integrationAudit struct{ recorder audit.Recorder }

func (a integrationAudit) Record(ctx context.Context, e AuditEntry) error {
	return a.recorder.Record(ctx, audit.Entry{ActorType: "vendor_staff", ActorID: e.ActorID, Action: e.Action, AggregateType: e.AggregateType, AggregateID: e.AggregateID, VendorID: e.VendorID, BranchID: e.BranchID, Before: e.Before, After: e.After, Reason: e.Reason})
}

func TestSection6AggregatePersistsAndScopesToAuthenticatedBusiness(t *testing.T) {
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping real-Postgres Section 6 aggregate test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = database.ApplyMigrations(ctx, pool, "../../../migrations"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err = pool.Exec(ctx, `TRUNCATE vendor_businesses,vendor_outbox,vendor_audit_log,vendor_staff_role_assignments,vendor_staff,vendor_users RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO vendor_users(id,email) VALUES('vu_s6','s6@example.test'); INSERT INTO vendor_staff(id,vendor_id,user_id,status) VALUES('vs_s6','vendor_s6','vu_s6','active')`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	svc := NewService(pool, allowAll{}, integrationAudit{recorder: audit.NewPostgresRecorder(pool)}, events.NewDomainPublisher(events.NewOutboxRepository(pool)))
	actor := Actor{StaffID: "vs_s6", VendorID: "vendor_s6"}
	business, err := svc.CreateBusiness(ctx, actor, Business{LegalName: "KarryGo Kitchen Limited", TradingName: "Karry Kitchen", Currency: "NGN", Timezone: "Africa/Lagos"})
	if err != nil {
		t.Fatalf("create business: %v", err)
	}
	if business.ID != "vendor_s6" || business.OwnerRef != "vs_s6" || business.Status != BusinessDraft {
		t.Fatalf("business scope/lifecycle mismatch: %+v", business)
	}
	capability, err := svc.CreateCapability(ctx, actor, business.ID, CapabilityRestaurant)
	if err != nil {
		t.Fatalf("create capability: %v", err)
	}
	if _, err = svc.CreateCapability(ctx, actor, business.ID, CapabilityRestaurant); err == nil {
		t.Fatal("duplicate capability unexpectedly created")
	}
	if _, err = svc.CreateCapability(ctx, actor, business.ID, CapabilityType("PHARMACY")); err == nil {
		t.Fatal("unsupported capability unexpectedly created")
	}
	brand, err := svc.CreateBrand(ctx, actor, business.ID, Brand{DisplayName: "Karry Kitchen", MediaRefs: []MediaReference{{Kind: "LOGO", MediaRef: "media/logo_s6"}}})
	if err != nil {
		t.Fatalf("create brand: %v", err)
	}
	if brand.VerificationState != VerificationNotStarted {
		t.Fatalf("brand self-verified: %s", brand.VerificationState)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO vendor_users(id,email) VALUES('vu_s6_b','s6-b@example.test'); INSERT INTO vendor_staff(id,vendor_id,user_id,status) VALUES('vs_s6_b','vendor_s6_b','vu_s6_b','active')`); err != nil {
		t.Fatalf("seed second business staff: %v", err)
	}
	actorB := Actor{StaffID: "vs_s6_b", VendorID: "vendor_s6_b"}
	if _, err = svc.CreateBusiness(ctx, actorB, Business{LegalName: "Other Vendor Limited", Currency: "NGN", Timezone: "Africa/Lagos"}); err != nil {
		t.Fatalf("create other business: %v", err)
	}
	brandB, err := svc.CreateBrand(ctx, actorB, actorB.VendorID, Brand{DisplayName: "Other Brand"})
	if err != nil {
		t.Fatalf("create other brand: %v", err)
	}
	if _, err = svc.CreateBranch(ctx, actor, business.ID, Branch{BrandID: brandB.ID, DisplayName: "Invalid Cross Brand", Address: Address{Line1: "2 Admiralty Way", City: "Lagos", Region: "Lagos", CountryCode: "NG"}, Latitude: 6.44, Longitude: 3.47, Timezone: "Africa/Lagos"}); err == nil {
		t.Fatal("cross-business brand attachment unexpectedly succeeded")
	}
	branch, err := svc.CreateBranch(ctx, actor, business.ID, Branch{BrandID: brand.ID, DisplayName: "Lekki Branch", Address: Address{Line1: "1 Admiralty Way", City: "Lagos", Region: "Lagos", CountryCode: "NG"}, Latitude: 6.4474, Longitude: 3.4723, Timezone: "Africa/Lagos"})
	if err != nil {
		t.Fatalf("create branch: %v", err)
	}
	branchTwo, err := svc.CreateBranch(ctx, actor, business.ID, Branch{BrandID: brand.ID, DisplayName: "Ikeja Branch", Address: Address{Line1: "10 Allen Avenue", City: "Lagos", Region: "Lagos", CountryCode: "NG"}, Latitude: 6.6018, Longitude: 3.3515, Timezone: "Africa/Lagos"})
	if err != nil {
		t.Fatalf("create second branch: %v", err)
	}
	hours, err := svc.PutHours(ctx, actor, business.ID, branch.ID, []OperatingWindow{{Weekday: 1, OpensAt: "09:00", ClosesAt: "12:00"}, {Weekday: 1, OpensAt: "13:00", ClosesAt: "18:00"}}, branch.Version)
	if err != nil || len(hours) != 2 {
		t.Fatalf("put hours: len=%d err=%v", len(hours), err)
	}
	area, err := svc.PutServiceArea(ctx, actor, business.ID, branch.ID, ServiceArea{Mode: "RADIUS", CenterLatitude: 6.4474, CenterLongitude: 3.4723, RadiusMeters: 5000}, 0)
	if err != nil || area.Version != 1 {
		t.Fatalf("put service area: %+v err=%v", area, err)
	}
	settings, err := svc.PutSettings(ctx, actor, business.ID, branch.ID, OperationsSettings{SchemaVersion: 1, AcceptingNewOrders: true, PickupEnabled: true}, 0)
	if err != nil || settings.Version != 1 {
		t.Fatalf("put settings: %+v err=%v", settings, err)
	}
	configured, err := svc.PutBranchCapability(ctx, actor, business.ID, branch.ID, capability.ID, currentBranchCapabilityVersion(t, pool, ctx, business.ID, branch.ID, capability.ID))
	if err != nil {
		t.Fatalf("configure branch capability: %v", err)
	}
	if configured.Status != BranchCapabilityDraft || configured.Effective {
		t.Fatalf("branch capability self-activated: %+v", configured)
	}
	configuredAtBranchTwo, err := svc.PutBranchCapability(ctx, actor, business.ID, branchTwo.ID, capability.ID, currentBranchCapabilityVersion(t, pool, ctx, business.ID, branchTwo.ID, capability.ID))
	if err != nil {
		t.Fatalf("configure second branch: %v", err)
	}
	pastry, err := svc.CreateCapability(ctx, actor, business.ID, CapabilityPastryBakery)
	if err != nil {
		t.Fatalf("create second capability: %v", err)
	}
	configuredPastry, err := svc.PutBranchCapability(ctx, actor, business.ID, branch.ID, pastry.ID, currentBranchCapabilityVersion(t, pool, ctx, business.ID, branch.ID, pastry.ID))
	if err != nil {
		t.Fatalf("configure second branch capability: %v", err)
	}
	if _, err = pool.Exec(ctx, `WITH activate_business AS (UPDATE vendor_businesses SET status='ACTIVE' WHERE id=$1 RETURNING id), activate_branches AS (UPDATE vendor_branches SET status='ACTIVE' WHERE business_id=$1 RETURNING id), activate_capabilities AS (UPDATE vendor_capabilities SET status='ACTIVE' WHERE business_id=$1 RETURNING id) UPDATE vendor_branch_capabilities SET status='ACTIVE' WHERE business_id=$1`, business.ID); err != nil {
		t.Fatalf("operator activation seed: %v", err)
	}
	branchTwoBefore, err := svc.ListBranchCapabilities(ctx, actor, business.ID, branchTwo.ID)
	if err != nil {
		t.Fatal(err)
	}
	paused, err := svc.ChangeBranchCapabilityStatus(ctx, actor, business.ID, branch.ID, capability.ID, BranchCapabilityPaused, configured.Version)
	if err != nil {
		t.Fatalf("pause one branch capability: %v", err)
	}
	if paused.Status != BranchCapabilityPaused {
		t.Fatalf("pause status=%s", paused.Status)
	}
	list, err := svc.ListBranchCapabilities(ctx, actor, business.ID, branch.ID)
	if err != nil {
		t.Fatalf("list branch capabilities: %v", err)
	}
	states := map[CapabilityType]BranchCapability{}
	for _, item := range list {
		states[item.CapabilityType] = item
	}
	if states[CapabilityPastryBakery].Status != BranchCapabilityActive || !states[CapabilityPastryBakery].Effective {
		t.Fatalf("pausing restaurant changed pastry: %+v", states)
	}
	branchTwoStates, err := svc.ListBranchCapabilities(ctx, actor, business.ID, branchTwo.ID)
	if err != nil {
		t.Fatalf("list second branch: %v", err)
	}
	if len(branchTwoBefore) != 2 || len(branchTwoStates) != len(branchTwoBefore) {
		t.Fatalf("unexpected auto-linked capability inventory: before=%+v after=%+v", branchTwoBefore, branchTwoStates)
	}
	beforeByID := map[string]BranchCapability{}
	for _, c := range branchTwoBefore {
		beforeByID[c.ID] = c
	}
	for _, c := range branchTwoStates {
		b, ok := beforeByID[c.ID]
		if !ok || c.Status != b.Status || c.Version != b.Version || c.Effective != b.Effective {
			t.Fatalf("pausing branch one changed branch two: %+v", branchTwoStates)
		}
	}

	if _, err = pool.Exec(ctx, `UPDATE vendor_businesses SET status='PAUSED' WHERE id=$1`, business.ID); err != nil {
		t.Fatal(err)
	}
	list, err = svc.ListBranchCapabilities(ctx, actor, business.ID, branch.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range list {
		if item.Status == BranchCapabilityActive && item.Effective {
			t.Fatalf("business pause did not gate child: %+v", item)
		}
	}
	_ = configuredPastry
	_ = configuredAtBranchTwo
	other := Actor{StaffID: "vs_s6", VendorID: "vendor_other"}
	if _, err = svc.GetBranch(ctx, other, business.ID, branch.ID); err == nil {
		t.Fatal("cross-business access unexpectedly succeeded")
	}
	var outboxCount, auditCount int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_outbox WHERE vendor_id=$1`, business.ID).Scan(&outboxCount); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vendor_audit_log WHERE vendor_id=$1`, business.ID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if outboxCount < 8 || auditCount < 8 {
		t.Fatalf("expected durable outbox/audit coverage, outbox=%d audit=%d", outboxCount, auditCount)
	}
}
