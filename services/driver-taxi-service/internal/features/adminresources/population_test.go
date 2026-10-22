package adminresources

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/admincontext"
	contextfixture "cosmicforge/logistics/shared/go/admincontext/testfixture"
	"cosmicforge/logistics/shared/go/adminowner"
	fixture "cosmicforge/logistics/shared/go/adminowner/testfixture"
	"cosmicforge/logistics/shared/go/adminpolicy"
)

// Goods missions and corporate rides are enrolled exactly like independent
// rides: the binding comes only from the owner's own row, and membership in a
// scope is proven per record by an owner-signed context.
func TestTaxiGoodsAndCorporateRideReadsBindOwnRowsWithoutInventingScope(t *testing.T) {
	db := fixture.Database(t, "../../../migrations")
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := db.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	market, zone, quoteMarket, quoteZone := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	exec(`INSERT INTO markets(id,country,name,code) VALUES($1,'TEST','PRIVATE_NAME','M1'),($2,'TEST','PRIVATE_NAME','QM')`, market, quoteMarket)
	exec(`INSERT INTO operating_zones(id,market_id,name,code,center_lat,center_lng) VALUES($1,$2,'PRIVATE_ZONE','Z1',10,20),($3,$4,'PRIVATE_ZONE','QZ',30,40)`, zone, market, quoteZone, quoteMarket)
	corporate, independent, quote, goods := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	exec(`INSERT INTO taxi_bookings(id,customer_id,market_id,operating_zone_id,organization_id,is_corporate_managed,membership_id,fleet_assignment_id,status,source_context,ride_notes) VALUES($1,$2,$3,$4,'org-1',true,'PRIVATE_MEMBERSHIP','PRIVATE_ASSIGNMENT','completed','FLEET_ASSIGNMENT','PRIVATE_NOTES')`, corporate, uuid.NewString(), market, zone)
	exec(`INSERT INTO taxi_bookings(id,customer_id,market_id,operating_zone_id,status,ride_notes) VALUES($1,$2,$3,$4,'completed','PRIVATE_NOTES')`, independent, uuid.NewString(), market, zone)
	exec(`INSERT INTO taxi_goods_quotes(id,source_service,quote_reference,revision,request_fingerprint,request_payload,serviceable,reason_code,market_id,operating_zone_id,currency,expires_at) VALUES($1,'vendor-service','PRIVATE_QUOTE',1,'PRIVATE_HASH','{}',true,'SERVICEABLE',$2,$3,'NGN',now()+interval '15 minutes')`, quote, quoteMarket, quoteZone)
	exec(`INSERT INTO taxi_goods_missions(id,source_service,mission_ref,operation_ref,request_fingerprint,parent_order_ref,vendor_order_refs,customer_ref,quote_id,pickup,dropoff,load,agreed_fee_minor,currency,requested_at,receiver_name,receiver_phone,pickup_evidence_ref,delivery_evidence_ref) VALUES($1,'vendor-service','PRIVATE_MISSION','PRIVATE_OPERATION','PRIVATE_HASH','PRIVATE_ORDER','{PRIVATE_VENDOR}','PRIVATE_CUSTOMER',$2,'{"latitude":51}','{"longitude":10}','{}',1000,'NGN',now(),'PRIVATE_NAME','PRIVATE_PHONE','PRIVATE_MEDIA','PRIVATE_MEDIA')`, goods, quote)
	source := Source()
	read := func(kind, ref string) (adminowner.Resource, admincontext.RecordBinding, error) {
		t.Helper()
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		resource, err := source.Read(ctx, tx, kind, ref, true)
		if err != nil {
			return resource, admincontext.RecordBinding{}, err
		}
		binding, err := source.Binding(ctx, tx, kind, ref)
		return resource, binding, err
	}

	// Goods missions: no dimension is taken from the quote's geography or
	// from vendor/customer strings.
	resource, goodsBinding, err := read("goods-missions", goods)
	if err != nil {
		t.Fatal(err)
	}
	if err := adminowner.ValidateResource(resource, source.Specs["goods-missions"].Definition); err != nil {
		t.Fatal(err)
	}
	if goodsBinding.Canonical != (adminpolicy.Scope{Product: "karryam"}) {
		t.Fatal("goods mission scope inferred from quote or strings", goodsBinding.Canonical)
	}
	raw, _ := json.Marshal(resource)
	if strings.Contains(string(raw), "PRIVATE_") || strings.Contains(string(raw), "latitude") {
		t.Fatal("goods mission private fields leaked", string(raw))
	}

	// Corporate rides: market and zone from the booking's own FKs, the
	// organization the row records, tenant = organization.
	resource, corporateBinding, err := read("corporate-rides", corporate)
	if err != nil {
		t.Fatal(err)
	}
	if err := adminowner.ValidateResource(resource, source.Specs["corporate-rides"].Definition); err != nil {
		t.Fatal(err)
	}
	want := adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: "org-1", Tenant: "org-1"}
	if corporateBinding.Canonical != want {
		t.Fatal("corporate ride canonical scope", corporateBinding.Canonical)
	}
	raw, _ = json.Marshal(resource)
	if strings.Contains(string(raw), "PRIVATE_") {
		t.Fatal("corporate ride private fields leaked", string(raw))
	}
	// The two passenger kinds partition the same table exactly.
	if _, _, err := read("independent-rides", corporate); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("corporate ride served as independent", err)
	}
	if _, _, err := read("corporate-rides", independent); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("independent ride served as corporate", err)
	}

	// Membership needs a signature; a foreign organization is refused, and a
	// changed identity link withdraws the signed membership.
	issuer := contextfixture.New(t, Owner)
	store := admincontext.Store{DB: db, Reader: source, Authority: issuer.Authority}
	foreign := want
	foreign.Organization, foreign.Tenant = "org-2", "org-2"
	if err := store.Import(ctx, issuer.Sign(t, issuer.Approval("corporate-foreign", "corporate-rides", corporate, "taxi.corporate_rides.read", corporateBinding, foreign)), time.Now()); err == nil {
		t.Fatal("corporate ride attested into another organization")
	}
	scope := adminpolicy.Scope{Product: "karryam", Market: "M9", Zone: "Z9", Organization: "o9", Tenant: "t9"}
	if err := store.Import(ctx, issuer.Sign(t, issuer.Approval("goods-ctx", "goods-missions", goods, "taxi.goods_missions.read", goodsBinding, scope)), time.Now()); err != nil {
		t.Fatal(err)
	}
	tx, _ := db.Begin(ctx)
	if _, err := store.Resolve(ctx, tx, "goods-missions", goods, "taxi.goods_missions.read", scope, time.Now()); err != nil {
		t.Fatal("signed goods membership not resolved", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE taxi_goods_missions SET customer_ref='ANOTHER_CUSTOMER' WHERE id=$1`, goods); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Resolve(ctx, tx, "goods-missions", goods, "taxi.goods_missions.read", scope, time.Now()); !errors.Is(err, admincontext.ErrUnmapped) {
		t.Fatal("goods membership survived a changed identity link", err)
	}
	tx.Rollback(ctx)
	tx, _ = db.Begin(ctx)
	defer tx.Rollback(ctx)
	if _, err := store.Resolve(ctx, tx, "goods-missions", goods, "taxi.goods_missions.read", foreign, time.Now()); !errors.Is(err, admincontext.ErrUnmapped) {
		t.Fatal("unsigned scope resolved", err)
	}
}
