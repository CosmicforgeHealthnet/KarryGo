package adminresources

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/admincontext"
	contextfixture "cosmicforge/logistics/shared/go/admincontext/testfixture"
	"cosmicforge/logistics/shared/go/adminowner"
	fixture "cosmicforge/logistics/shared/go/adminowner/testfixture"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"github.com/jackc/pgx/v5"
)

func TestDispatchWorkflowReadsBindCanonicalTripAndRejectCrossLinks(t *testing.T) {
	db := fixture.Database(t, "../../../migrations", true)
	ctx := context.Background()
	const provider = "11111111-1111-4111-8111-111111111111"
	const customer = "22222222-2222-4222-8222-222222222222"
	const org = "33333333-3333-4333-8333-333333333333"
	const otherOrg = "44444444-4444-4444-8444-444444444444"
	const incident = "55555555-5555-4555-8555-555555555555"
	const attempt = "66666666-6666-4666-8666-666666666666"
	const recovery = "77777777-7777-4777-8777-777777777777"
	const returned = "88888888-8888-4888-8888-888888888888"
	var bookings, trips []string
	if _, err := db.Exec(ctx, `INSERT INTO providers(id,phone,full_name) VALUES($1,'PRIVATE_PHONE','PRIVATE_NAME')`, provider); err != nil {
		t.Fatal(err)
	}
	for _, organization := range []string{org, otherOrg} {
		var booking, trip string
		if err := db.QueryRow(ctx, `INSERT INTO dispatch_bookings(customer_id,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone) VALUES($1,'PRIVATE_ADDRESS',0,0,'PRIVATE_ADDRESS',0,0,100,'PRIVATE_RECEIVER','PRIVATE_PHONE') RETURNING id::text`, customer).Scan(&booking); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow(ctx, `INSERT INTO trips(booking_id,provider_id,customer_id,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone,organization_id) VALUES($1,$2,$3,'PRIVATE_ADDRESS',0,0,'PRIVATE_ADDRESS',0,0,100,'PRIVATE_RECEIVER','PRIVATE_PHONE',$4) RETURNING id::text`, booking, provider, customer, organization).Scan(&trip); err != nil {
			t.Fatal(err)
		}
		bookings = append(bookings, booking)
		trips = append(trips, trip)
	}
	if _, err := db.Exec(ctx, `INSERT INTO dispatch_delivery_incidents(id,trip_id,provider_id,reason_code,safe_note,source_version,idempotency_key,request_fingerprint) VALUES($1,$2,$3,'BREAKDOWN','PRIVATE_NOTE',9,'incident-key','PRIVATE_FINGERPRINT')`, incident, trips[0], provider); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO dispatch_delivery_attempts(id,trip_id,provider_id,attempt_number,arrived_at,grace_expires_at,policy_version,source_version,idempotency_key,request_fingerprint) VALUES($1,$2,$3,1,now(),now()+interval '5 minutes','test-policy',9,'attempt-key','PRIVATE_FINGERPRINT')`, attempt, trips[0], provider); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO dispatch_delivery_recovery_actions(id,delivery_attempt_id,trip_id,booking_id,customer_id,action,state,policy_version,expected_trip_version,idempotency_key,request_fingerprint,alternative_receiver_ref,financial_operation_ref) VALUES($1,$2,$3,$4,$5,'RETURN_TO_SENDER','REQUESTED','test-policy',9,'recovery-key','PRIVATE_FINGERPRINT','PRIVATE_RECEIVER_REF','PRIVATE_FINANCIAL_REF')`, recovery, attempt, trips[0], bookings[0], customer); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO dispatch_return_workflows(id,recovery_action_id,original_trip_id,route_reference,route_snapshot,policy_version,wallet_operation_ref) VALUES($1,$2,$3,'PRIVATE_ROUTE','{"private_location":"PRIVATE_COORDINATES"}','test-policy','PRIVATE_WALLET_REF')`, returned, recovery, trips[0]); err != nil {
		t.Fatal(err)
	}
	source := Source()
	issuer := contextfixture.New(t, Owner)
	store := admincontext.Store{DB: db, Reader: source, Authority: issuer.Authority}
	scope := adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: org, Tenant: org}
	for kind, ref := range map[string]string{"incidents": incident, "delivery-recoveries": recovery, "returns": returned} {
		t.Run(kind, func(t *testing.T) {
			tx, err := db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			binding, err := source.Binding(ctx, tx, kind, ref)
			if err != nil {
				t.Fatal(err)
			}
			resource, err := source.Read(ctx, tx, kind, ref, true)
			tx.Rollback(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if binding.Canonical.Organization != org || binding.Canonical.Tenant != org {
				t.Fatal("canonical Trip tenant lost")
			}
			if resource.SourceVersion != nil {
				t.Fatal("captured Trip counter became workflow mutation version")
			}
			if err := adminowner.ValidateResource(resource, source.Specs[kind].Definition, true); err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(resource)
			if strings.Contains(string(raw), "PRIVATE_") {
				t.Fatal("private workflow payload leaked")
			}
			approval := issuer.Approval("workflow-"+kind, kind, ref, source.Specs[kind].Definition.Capability, binding, scope)
			bad := approval
			bad.Scope.Tenant = otherOrg
			if err := store.Import(ctx, issuer.Sign(t, bad), time.Now()); err == nil {
				t.Fatal("signed context overrode canonical Trip tenant")
			}
			if err := store.Import(ctx, issuer.Sign(t, approval), time.Now()); err != nil {
				t.Fatal(err)
			}
			tx, err = db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if _, err := store.Resolve(ctx, tx, kind, ref, source.Specs[kind].Definition.Capability, scope, time.Now()); err != nil {
				t.Fatal(err)
			}
			wrong := scope
			wrong.Tenant = otherOrg
			if _, err := store.Resolve(ctx, tx, kind, ref, source.Specs[kind].Definition.Capability, wrong, time.Now()); !errors.Is(err, admincontext.ErrUnmapped) {
				t.Fatalf("wrong tenant: %v", err)
			}
		})
	}
	// A valid FK alone does not establish the same workflow subject. Corrupted
	// historical cross-links must disappear, including from dependent returns.
	if _, err := db.Exec(ctx, `UPDATE dispatch_delivery_recovery_actions SET booking_id=$2 WHERE id=$1`, recovery, bookings[1]); err != nil {
		t.Fatal(err)
	}
	for kind, ref := range map[string]string{"delivery-recoveries": recovery, "returns": returned} {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		_, err = source.Read(ctx, tx, kind, ref, true)
		tx.Rollback(ctx)
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("%s accepted a foreign booking link: %v", kind, err)
		}
	}
	// Repointing an incident to another real Trip changes its source binding;
	// the previous owner allocation cannot follow it into another organization.
	if _, err := db.Exec(ctx, `UPDATE dispatch_delivery_incidents SET trip_id=$2 WHERE id=$1`, incident, trips[1]); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := store.Resolve(ctx, tx, "incidents", incident, "dispatch.incidents.read", scope, time.Now()); !errors.Is(err, admincontext.ErrUnmapped) {
		t.Fatalf("incident allocation survived changed native parent: %v", err)
	}
}
