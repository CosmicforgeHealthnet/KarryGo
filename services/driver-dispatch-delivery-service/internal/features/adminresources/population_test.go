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

// Customer bookings (including unmatched ones with no trip), delivery missions
// and commerce missions are enrolled exactly like trips and riders: bindings
// come only from the owner's own rows, organization/tenant only where the row
// records one, and scope membership is proven per record by a signed context.
func TestDispatchConsumerPopulationReadsBindOwnRowsWithoutInventingScope(t *testing.T) {
	db := fixture.Database(t, "../../../migrations", true)
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := db.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	var booking string
	if err := db.QueryRow(ctx, `INSERT INTO dispatch_bookings(customer_id,status,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone,package_desc) VALUES($1,'unmatched','PRIVATE_ADDRESS',6.52,3.37,'PRIVATE_ADDRESS',0,0,100,'PRIVATE_NAME','PRIVATE_PHONE','PRIVATE_NOTE') RETURNING id::text`, uuid.NewString()).Scan(&booking); err != nil {
		t.Fatal(err)
	}
	org, group, mission, commerce := uuid.NewString(), uuid.NewString(), uuid.NewString(), "dcm_"+strings.ReplaceAll(uuid.NewString(), "-", "")
	exec(`INSERT INTO dispatch_delivery_groups(id,source_type,source_ref,organization_id) VALUES($1,'PARTNER_API','PRIVATE_SOURCE',$2)`, group, org)
	exec(`INSERT INTO dispatch_delivery_missions(id,delivery_group_id,source_type,source_ref) VALUES($1,$2,'PARTNER_API','PRIVATE_SOURCE')`, mission, group)
	exec(`INSERT INTO commerce_missions(mission_ref,parent_order_ref,vendor_order_refs,customer_id,booking_id,agreed_fee_minor,status,operation_ref,canonical_ref) VALUES('PRIVATE_MISSION','PRIVATE_ORDER','{PRIVATE_VENDOR}',$1,$2,500,'accepted','PRIVATE_OPERATION',$3)`, uuid.NewString(), booking, commerce)

	partner := uuid.NewString()
	exec(`INSERT INTO request_broadcasts(booking_id,broadcast_radius_km,expires_at,booking_payload) VALUES($1,5,now()+interval '1 minute','{"receiver_phone":"PRIVATE_PHONE","pickup_address":"PRIVATE_ADDRESS"}'),($2,5,now()+interval '1 minute','{}')`, partner, booking)

	source := Source()
	read := func(kind, ref string) (adminowner.Resource, admincontext.RecordBinding) {
		t.Helper()
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		resource, err := source.Read(ctx, tx, kind, ref, true)
		if err != nil {
			t.Fatal(kind, err)
		}
		if err := adminowner.ValidateResource(resource, source.Specs[kind].Definition); err != nil {
			t.Fatal(kind, err)
		}
		raw, _ := json.Marshal(resource)
		if strings.Contains(string(raw), "PRIVATE_") || strings.Contains(string(raw), "6.52") {
			t.Fatal(kind, "private fields leaked", string(raw))
		}
		binding, err := source.Binding(ctx, tx, kind, ref)
		if err != nil {
			t.Fatal(kind, err)
		}
		return resource, binding
	}
	empty := adminpolicy.Scope{Product: "karryam"}
	if _, b := read("bookings", booking); b.Canonical != empty {
		t.Fatal("booking scope invented", b.Canonical)
	}
	if _, b := read("commerce-missions", commerce); b.Canonical != empty {
		t.Fatal("commerce mission scope invented from vendor references", b.Canonical)
	}
	if _, b := read("partner-requests", partner); b.Canonical != empty {
		t.Fatal("partner request scope invented", b.Canonical)
	}
	// A customer booking's broadcast is the booking, never a partner request.
	tx, _ := db.Begin(ctx)
	if _, err := source.Read(ctx, tx, "partner-requests", booking, false); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("customer booking broadcast served as a partner request", err)
	}
	tx.Rollback(ctx)
	_, missionBinding := read("delivery-missions", mission)
	if want := (adminpolicy.Scope{Product: "karryam", Organization: org, Tenant: org}); missionBinding.Canonical != want {
		t.Fatal("delivery mission did not take its group's recorded organization", missionBinding.Canonical)
	}

	issuer := contextfixture.New(t, Owner)
	store := admincontext.Store{DB: db, Reader: source, Authority: issuer.Authority}
	scope := adminpolicy.Scope{Product: "karryam", Market: "lagos", Zone: "island", Organization: org, Tenant: org}
	foreign := scope
	foreign.Organization, foreign.Tenant = "other-org", "other-org"
	if err := store.Import(ctx, issuer.Sign(t, issuer.Approval("mission-foreign", "delivery-missions", mission, "dispatch.delivery_missions.read", missionBinding, foreign)), time.Now()); err == nil {
		t.Fatal("delivery mission attested into another organization")
	}
	for kind, ref := range map[string]string{"bookings": booking, "delivery-missions": mission, "commerce-missions": commerce, "partner-requests": partner} {
		_, binding := read(kind, ref)
		capability := source.Specs[kind].Definition.Capability
		if err := store.Import(ctx, issuer.Sign(t, issuer.Approval("ctx-"+kind, kind, ref, capability, binding, scope)), time.Now()); err != nil {
			t.Fatal(kind, err)
		}
		tx, _ := db.Begin(ctx)
		if _, err := store.Resolve(ctx, tx, kind, ref, capability, scope, time.Now()); err != nil {
			t.Fatal(kind, "signed membership not resolved", err)
		}
		if _, err := store.Resolve(ctx, tx, kind, ref, capability, foreign, time.Now()); !errors.Is(err, admincontext.ErrUnmapped) {
			t.Fatal(kind, "unsigned scope resolved", err)
		}
		tx.Rollback(ctx)
	}
	// A changed identity link withdraws the signed membership.
	for kind, change := range map[string]string{
		"bookings":          `UPDATE dispatch_bookings SET customer_id=gen_random_uuid() WHERE id::text='` + booking + `'`,
		"delivery-missions": `UPDATE dispatch_delivery_groups SET organization_id=NULL WHERE id::text='` + group + `'`,
		"commerce-missions": `UPDATE commerce_missions SET parent_order_ref='ANOTHER_ORDER' WHERE canonical_ref='` + commerce + `'`,
		"partner-requests":  `UPDATE request_broadcasts SET service_type='other' WHERE booking_id::text='` + partner + `'`,
	} {
		ref := map[string]string{"bookings": booking, "delivery-missions": mission, "commerce-missions": commerce, "partner-requests": partner}[kind]
		tx, _ := db.Begin(ctx)
		if _, err := tx.Exec(ctx, change); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Resolve(ctx, tx, kind, ref, source.Specs[kind].Definition.Capability, scope, time.Now()); !errors.Is(err, admincontext.ErrUnmapped) {
			t.Fatal(kind, "membership survived a changed source link", err)
		}
		tx.Rollback(ctx)
	}
}
