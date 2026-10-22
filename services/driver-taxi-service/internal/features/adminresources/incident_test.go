package adminresources

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"cosmicforge/logistics/shared/go/adminowner"
	fixture "cosmicforge/logistics/shared/go/adminowner/testfixture"
	"github.com/google/uuid"
)

// TestTaxiIncidentReadsAndCaseEvidenceAuthority covers the Section 8
// administrative incident resource: native scope only from an independent
// ride's booking, typed subject references, and Media case evidence granted
// only while the incident is active (OPEN/REMEDIATION_IN_PROGRESS).
func TestTaxiIncidentReadsAndCaseEvidenceAuthority(t *testing.T) {
	db := fixture.Database(t, "../../../migrations")
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := db.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	market, zone, booking, provider := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	exec(`INSERT INTO markets(id,country,name,code) VALUES($1,'TEST','PRIVATE_NAME','M1')`, market)
	exec(`INSERT INTO operating_zones(id,market_id,name,code,center_lat,center_lng) VALUES($1,$2,'PRIVATE_ZONE','Z1',10,20)`, zone, market)
	exec(`INSERT INTO taxi_bookings(id,customer_id,market_id,operating_zone_id,status,ride_notes) VALUES($1,$2,$3,$4,'completed','PRIVATE_NOTES')`, booking, uuid.NewString(), market, zone)
	exec(`INSERT INTO taxi_providers(id,first_name) VALUES($1,'PRIVATE_NAME')`, provider)
	rideCase, providerCase, closedCase := "case-"+uuid.NewString(), "case-"+uuid.NewString(), "case-"+uuid.NewString()
	exec(`INSERT INTO taxi_admin_incidents(case_ref,subject_kind,subject_id,booking_id,category,severity,occurred_on,state,version,opened_at,updated_at) VALUES($1,'independent-rides',$2,$2,'SAFETY','HIGH',current_date,'OPEN',1,now(),now())`, rideCase, booking)
	exec(`INSERT INTO taxi_admin_incidents(case_ref,subject_kind,subject_id,provider_id,category,severity,occurred_on,state,version,opened_at,updated_at) VALUES($1,'providers',$2,$2,'CONDUCT','LOW',current_date,'REMEDIATION_IN_PROGRESS',2,now(),now())`, providerCase, provider)
	exec(`INSERT INTO taxi_admin_incidents(case_ref,subject_kind,subject_id,provider_id,category,severity,occurred_on,state,resolution,version,opened_at,updated_at,resolved_at,closed_at) VALUES($1,'providers',$2,$2,'CONDUCT','LOW',current_date,'CLOSED','NO_FINDING',3,now(),now(),now(),now())`, closedCase, provider)
	source := Source()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rb, err := source.Binding(ctx, tx, "incidents", rideCase)
	if err != nil {
		t.Fatal(err)
	}
	pb, err := source.Binding(ctx, tx, "incidents", providerCase)
	if err != nil {
		t.Fatal(err)
	}
	item, err := source.Read(ctx, tx, "incidents", providerCase, true)
	tx.Rollback(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rb.Canonical.Market != "M1" || rb.Canonical.Zone != "Z1" || rb.Canonical.Tenant != "" {
		t.Fatal("ride incident scope not from native booking", rb.Canonical)
	}
	if pb.Canonical.Market != "" || pb.Canonical.Tenant != "" {
		t.Fatal("provider incident geography/tenant inferred", pb.Canonical)
	}
	if err := adminowner.ValidateResource(item, source.Specs["incidents"].Definition, true); err != nil {
		t.Fatal(err)
	}
	if item.SourceVersion == nil || *item.SourceVersion != 2 || item.Status == nil || *item.Status != "REMEDIATION_IN_PROGRESS" {
		t.Fatal("incident version/state lost", item)
	}
	raw, _ := json.Marshal(item)
	if strings.Contains(string(raw), "PRIVATE_") {
		t.Fatal("private data disclosed", string(raw))
	}
	t.Run("ride-active", func(t *testing.T) { fixture.CaseBoundary(t, db, source, Owner, "incidents", rideCase, true) })
	t.Run("provider-active", func(t *testing.T) { fixture.CaseBoundary(t, db, source, Owner, "incidents", providerCase, true) })
	t.Run("closed", func(t *testing.T) { fixture.CaseBoundary(t, db, source, Owner, "incidents", closedCase, false) })
	// A historically inconsistent native parent (zone outside the booking's
	// market) is refused rather than projected with a guessed scope.
	otherMarket, otherZone := uuid.NewString(), uuid.NewString()
	exec(`INSERT INTO markets(id,country,name,code) VALUES($1,'TEST','PRIVATE_NAME','M2')`, otherMarket)
	exec(`INSERT INTO operating_zones(id,market_id,name,code,center_lat,center_lng) VALUES($1,$2,'PRIVATE_ZONE','Z2',30,40)`, otherZone, otherMarket)
	exec(`UPDATE taxi_bookings SET operating_zone_id=$2 WHERE id=$1`, booking, otherZone)
	tx, err = db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := source.Read(ctx, tx, "incidents", rideCase, false); err == nil {
		t.Fatal("incident with inconsistent native ride scope readable")
	}
}
