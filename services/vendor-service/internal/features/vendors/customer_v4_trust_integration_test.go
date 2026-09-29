package vendors

import (
	"cosmicforge/logistics/services/vendor-service/internal/audit"
	"cosmicforge/logistics/services/vendor-service/internal/events"
	"cosmicforge/logistics/services/vendor-service/internal/publictrust"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestCustomerV4PublicTrustLifecycle(t *testing.T) {
	ctx, pool := section7Pool(t)
	_, err := pool.Exec(ctx, `INSERT INTO vendor_users(id,email) VALUES('vu_trust','trust@example.test'); INSERT INTO vendor_staff(id,vendor_id,user_id,status) VALUES('vs_trust','vendor_trust','vu_trust','active')`)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(pool, allowAll{}, integrationAudit{recorder: audit.NewPostgresRecorder(pool)}, events.NewDomainPublisher(events.NewOutboxRepository(pool)))
	actor := Actor{StaffID: "vs_trust", VendorID: "vendor_trust"}
	business, err := svc.CreateBusiness(ctx, actor, Business{LegalName: "Trust Test", Currency: "NGN", Timezone: "Africa/Lagos"})
	if err != nil {
		t.Fatal(err)
	}
	brand, err := svc.CreateBrand(ctx, actor, business.ID, Brand{DisplayName: "Trust Brand"})
	if err != nil {
		t.Fatal(err)
	}
	branch, err := svc.CreateBranch(ctx, actor, business.ID, Branch{BrandID: brand.ID, DisplayName: "Trust Branch", Address: Address{Line1: "1 Road", City: "Lagos", Region: "Lagos", CountryCode: "NG"}, Latitude: 6.4, Longitude: 3.4, Timezone: "Africa/Lagos"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `UPDATE vendor_businesses SET status='ACTIVE' WHERE id=$1;`, business.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `UPDATE vendor_branches SET status='ACTIVE' WHERE id=$1`, branch.ID)
	if err != nil {
		t.Fatal(err)
	}
	read := func() []publictrust.Fact {
		t.Helper()
		f, e := publictrust.Read(ctx, pool, business.ID, branch.ID)
		if e != nil {
			t.Fatal(e)
		}
		return f
	}
	dimensions := []struct {
		d        VerificationDimension
		typ, ref string
	}{{DimensionBusiness, "BUSINESS", business.ID}, {DimensionLocation, "BRANCH", branch.ID}, {DimensionBrand, "BRAND", brand.ID}, {DimensionFoodKitchen, "BRANCH", branch.ID}}
	for _, d := range dimensions {
		r := verified("private-case-"+string(d.d), 1, nil)
		r.ReasonCode = "private-reason"
		r.Remediation = "private-note"
		if _, err = svc.ApplyVerificationEvent(ctx, "verification-compliance-service", "event-"+string(d.d), business.ID, r, d.d, d.typ, d.ref); err != nil {
			t.Fatal(err)
		}
	}
	if len(read()) != 4 {
		t.Fatalf("missing supported dimensions: %+v", read())
	}
	raw, _ := json.Marshal(read())
	for _, s := range []string{"private-", "source_ref", "source_version", "reason_code", "remediation", "document", "score", "reviewer"} {
		if strings.Contains(string(raw), s) {
			t.Fatalf("leak %s: %s", s, raw)
		}
	}
	if f, e := publictrust.Read(ctx, pool, "another-vendor", branch.ID); e != nil || len(f) != 0 {
		t.Fatalf("cross vendor leakage: %+v %v", f, e)
	}
	for i, status := range []VerificationState{VerificationSuspended, VerificationRejected, VerificationVerified, VerificationExpired} {
		r := verified("private-case-BUSINESS", int64(i+2), nil)
		r.Status = status
		if _, err = svc.ApplyVerificationEvent(ctx, "verification-compliance-service", "transition-"+string(status), business.ID, r, DimensionBusiness, "BUSINESS", business.ID); err != nil {
			t.Fatal(err)
		}
		for _, f := range read() {
			if f.Dimension == "BUSINESS" && f.Status != string(status) {
				t.Fatalf("stale badge: %+v", f)
			}
		}
	}
	if _, err = svc.ApplyVerificationEvent(ctx, "verification-compliance-service", "old-event", business.ID, verified("private-case-BUSINESS", 1, nil), DimensionBusiness, "BUSINESS", business.ID); err == nil {
		t.Fatal("stale version accepted")
	}
	old := verified("old-case", 99, nil)
	old.ObservedAt = time.Now().Add(-time.Hour)
	if _, err = svc.ApplyVerificationEvent(ctx, "verification-compliance-service", "old-case-event", business.ID, old, DimensionBusiness, "BUSINESS", business.ID); err == nil {
		t.Fatal("older case replaced latest")
	}
	past := time.Now().Add(-time.Second)
	r := verified("private-case-BUSINESS", 6, &past)
	if _, err = svc.ApplyVerificationEvent(ctx, "verification-compliance-service", "expired-by-clock", business.ID, r, DimensionBusiness, "BUSINESS", business.ID); err != nil {
		t.Fatal(err)
	}
	for _, f := range read() {
		if f.Dimension == "BUSINESS" && f.Status != "EXPIRED" {
			t.Fatalf("time expiry failed: %+v", f)
		}
	}
	_, err = pool.Exec(ctx, `UPDATE vendor_verification_projections SET updated_at=now()-interval '6 minutes' WHERE business_id=$1`, business.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(read()) != 0 {
		t.Fatal("badge survived freshness bound")
	}
	// Equal-version callbacks do not extend freshness; successful read-back does.
	if _, err = svc.ApplyVerificationEvent(ctx, "verification-compliance-service", "equal-event", business.ID, r, DimensionBusiness, "BUSINESS", business.ID); err != nil {
		t.Fatal(err)
	}
	if len(read()) != 0 {
		t.Fatal("equal callback renewed stale badge")
	}
	if _, err = svc.applyProjection(ctx, actor, "verification-compliance-service", r, DimensionBusiness, "BUSINESS", business.ID, ""); err != nil {
		t.Fatal(err)
	}
	if len(read()) != 1 {
		t.Fatal("read-back did not renew projection")
	}
}
