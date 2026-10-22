package analyticsproducer

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/google/uuid"

	"cosmicforge/logistics/services/taxi-service/internal/features/adminresources"
	"cosmicforge/logistics/services/taxi-service/internal/outbox"
	"cosmicforge/logistics/shared/go/admincontext"
	contextfixture "cosmicforge/logistics/shared/go/admincontext/testfixture"
	fixture "cosmicforge/logistics/shared/go/adminowner/testfixture"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/analyticsadmin"
)

// Goods missions and corporate rides are anchored to their enrolled kinds,
// chosen from the booking's own row. Each enters a scope only with an
// owner-signed context; an unsigned goods mission stays out of every scope.
func TestPostgresTaxiProducerAnchorsGoodsMissionsAndCorporateRides(t *testing.T) {
	db := fixture.Database(t, "../../../migrations")
	ctx := context.Background()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	market, zone, ride, corporate, goods, unsigned := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	exec(`INSERT INTO markets(id,country,name,code) VALUES($1,'TEST','N','M1')`, market)
	exec(`INSERT INTO operating_zones(id,market_id,name,code,center_lat,center_lng) VALUES($1,$2,'Z','Z1',10,20)`, zone, market)
	exec(`INSERT INTO taxi_bookings(id,customer_id,market_id,operating_zone_id,status) VALUES($1,$2,$3,$4,'completed')`, ride, uuid.NewString(), market, zone)
	exec(`INSERT INTO taxi_bookings(id,customer_id,market_id,operating_zone_id,organization_id,is_corporate_managed,membership_id,fleet_assignment_id,status,source_context) VALUES($1,$2,$3,$4,'o1',true,'m','a','completed','FLEET_ASSIGNMENT')`, corporate, uuid.NewString(), market, zone)
	for _, id := range []string{goods, unsigned} {
		exec(`INSERT INTO taxi_goods_missions(id,source_service,mission_ref,operation_ref,request_fingerprint,parent_order_ref,vendor_order_refs,customer_ref,plan_ref,plan_version,pickup_count,plan_binding_mode,pickup,dropoff,load,agreed_fee_minor,currency,requested_at) VALUES($1::uuid,'vendor-service',$2,$2,'h','o','{v}','c','plan',1,2,'PLAN_BOUND','{}','{}','{}',100,'NGN',now())`, id, "ref-"+id)
	}
	for _, id := range []string{ride, corporate} {
		if err := outbox.Write(ctx, db, "booking", id, "taxi.booking.completed", []byte(`{}`)); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{goods, unsigned} {
		for _, event := range []string{"taxi.goods.mission.committed.v1", "taxi.goods.completed.v1"} {
			if err := outbox.Write(ctx, db, "taxi_goods_mission", id, event, []byte(`{}`)); err != nil {
				t.Fatal(err)
			}
		}
	}
	scope := adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: "o1", Tenant: "o1"}
	issuer := contextfixture.New(t, Owner)
	store := &admincontext.Store{DB: db, Reader: adminresources.Source(), Authority: issuer.Authority}
	for kind, ref := range map[string]string{"independent-rides": ride, "corporate-rides": corporate, "goods-missions": goods} {
		tx, _ := db.Begin(ctx)
		binding, err := adminresources.Source().Binding(ctx, tx, kind, ref)
		tx.Rollback(ctx)
		if err != nil {
			t.Fatal(kind, err)
		}
		if err := store.Import(ctx, issuer.Sign(t, issuer.Approval("ctx-"+ref, kind, ref, analyticsadmin.CoverageExportCapability, binding, scope)), time.Now()); err != nil {
			t.Fatal(kind, err)
		}
	}
	window := time.Now().UTC().Truncate(time.Hour)
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	authority := admincontext.Authority{Owner: Owner, Environment: adminpolicy.TestOnly, Keys: map[string]ed25519.PublicKey{"k": pub}}
	publisher := &capture{}
	p := &analyticsadmin.Producer{Owner: Owner, Environment: adminpolicy.TestOnly, DB: db, Source: Facts{DB: db}, Resolver: &analyticsadmin.ContextScopeResolver{DB: db, Store: store}, AnchorKinds: AnchorKinds(), Signer: analyticsadmin.Signer{KeyID: "k", Key: key}, Publisher: publisher,
		Period: time.Hour, Lookback: time.Hour, TTL: 72 * time.Hour, ProvenanceRef: "TEST_ONLY", ApprovalRef: "TEST_ONLY", Now: func() time.Time { return window.Add(time.Hour) }}
	result, err := p.RunOnce(ctx)
	if err != nil || result.Unattested != 0 || result.Unscoped != 2 {
		t.Fatal("expected only the unsigned goods mission's two facts out of scope", result, err)
	}
	members := map[string]int{}
	for _, body := range publisher.bodies {
		v, err := analyticsadmin.VerifyManifest(authority, body, window.Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		if v.Family != analyticsadmin.FamilyNetworkEvents {
			continue
		}
		for _, m := range v.Members {
			members[m.EntityID]++
		}
	}
	if members[ride] != 1 || members[corporate] != 1 || members[goods] != 2 || members[unsigned] != 0 {
		t.Fatal("anchored populations", members)
	}
}
