package adminresources

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/admincontext"
	contextfixture "cosmicforge/logistics/shared/go/admincontext/testfixture"
	"cosmicforge/logistics/shared/go/adminowner"
	fixture "cosmicforge/logistics/shared/go/adminowner/testfixture"
	"cosmicforge/logistics/shared/go/adminpolicy"
	policyfixture "cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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
	if _, err := db.Exec(ctx, `UPDATE dispatch_delivery_incidents SET recovery_result='UNKNOWN',recovery_attempts=2,recovery_next_at=now()+interval '1 minute',recovery_deadline=now()+interval '1 hour',recovery_error_code='PRIVATE_ERROR',recovery_lease_owner='PRIVATE_LEASE' WHERE id=$1`, incident); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `UPDATE dispatch_return_workflows SET assignment_state='UNKNOWN',assignment_attempt_count=3,assignment_next_attempt_at=now()+interval '1 minute',assignment_last_error_code='PRIVATE_ERROR',assignment_lease_owner='PRIVATE_LEASE' WHERE id=$1`, returned); err != nil {
		t.Fatal(err)
	}
	source := Source()
	for kind, ref := range map[string]string{"incidents": incident, "delivery-recoveries": recovery, "returns": returned} {
		t.Run("case-evidence-"+kind, func(t *testing.T) { fixture.CaseBoundary(t, db, source, Owner, kind, ref, true) })
	}
	issuer := contextfixture.New(t, Owner)
	store := admincontext.Store{DB: db, Reader: source, Authority: issuer.Authority}
	scope := adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: org, Tenant: org}
	for kind, ref := range map[string]string{"incidents": incident, "delivery-recoveries": recovery, "returns": returned, "incident-recovery-status": incident, "return-execution-status": returned} {
		t.Run(kind, func(t *testing.T) {
			tx, err := db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
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
			if kind == "incident-recovery-status" || kind == "return-execution-status" {
				foundUnknown := false
				for _, d := range resource.Dimensions {
					if d.Name == "recovery_result" || d.Name == "assignment_state" {
						foundUnknown = d.Value != nil && *d.Value == "UNKNOWN"
					}
				}
				if !foundUnknown {
					t.Fatal("native uncertainty became an inferred successful/replayable result")
				}
				counts := map[string]int64{}
				for _, c := range resource.Counters {
					if c.Value != nil {
						counts[c.Name] = *c.Value
					}
				}
				if kind == "incident-recovery-status" && (counts["captured_trip_revision"] != 9 || counts["recovery_attempts"] != 2) {
					t.Fatal("native incident queue observations lost", counts)
				}
				if kind == "return-execution-status" && counts["assignment_attempts"] != 3 {
					t.Fatal("native return attempts lost", counts)
				}
				definition := source.Specs[kind].Definition
				policy := policyfixture.New(t, scope, Owner, definition.Capability, definition.SourceContract)
				policy.Document.ID = "test-only-execution-" + kind
				policy.Sign(t)
				profile, _ := adminowner.ForOwner(Owner)
				key := []byte(strings.Repeat("e", 32))
				h := &adminowner.Handler{Owner: Owner, Path: profile.Path, DB: db, Source: source, Contexts: &store, Policy: policy.Engine, Secret: key, Enabled: true, Definitions: profile.Definitions}
				router := gin.New()
				h.Register(router)
				q := adminread.Query{Scope: adminread.Scope{Market: scope.Market, Zone: scope.Zone, Organization: scope.Organization, Tenant: scope.Tenant}, Staff: "staff-1", Role: "staff", Purpose: "case-1", PolicyID: policy.Engine.ID(), PolicyVersion: policy.Engine.Version(), PolicyChecksum: policy.Engine.Checksum(), Start: time.Now().Add(-time.Hour), End: time.Now().Add(time.Second), Limit: 10}
				request := httptest.NewRequest("GET", profile.Path+"/resources/"+kind+"/"+ref+"?"+q.Values().Encode(), nil)
				if err := serviceauth.SignRequest(request, adminread.Caller, key, nil, time.Now()); err != nil {
					t.Fatal(err)
				}
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				if response.Code != 200 || strings.Contains(response.Body.String(), "PRIVATE_") {
					t.Fatal("native execution-status wire", response.Code, response.Body.String())
				}
				if dir := os.Getenv("ADMIN_OWNER_CUSTODY_CAPTURE_DIR"); dir != "" {
					if err := os.MkdirAll(dir, 0700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(dir, "dispatch-"+kind+".json"), response.Body.Bytes(), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
		})
	}
	// Native createReturnWorkflow links a same-customer booking, return:<trip>
	// mission and execution link atomically. Validate that actual graph; neither
	// a UUID FK nor an old draft-row allocation authorizes unrelated children.
	groupID, returnMission, returnBooking := uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO dispatch_delivery_groups(id,customer_id,source_type,source_ref,booking_id,trip_id,organization_id)VALUES($1,$2,'CUSTOMER',$3,$4,$5,$6)`, groupID, customer, "return:"+bookings[0], bookings[0], trips[0], org); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO dispatch_bookings(id,customer_id,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone)SELECT $1,customer_id,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,0,receiver_name,receiver_phone FROM dispatch_bookings WHERE id=$2`, returnBooking, bookings[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO dispatch_delivery_missions(id,delivery_group_id,source_type,source_ref,booking_id,logical_mission_ref)VALUES($1,$2,'CUSTOMER',$3,$4,$3)`, returnMission, groupID, "return:"+trips[0], returnBooking); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO dispatch_mission_execution_links(mission_id,booking_id,source_type,source_ref,source_version,operation_ref,request_fingerprint)VALUES($1,$2,'CUSTOMER',$3,1,'PRIVATE_RETURN_OPERATION','PRIVATE_FINGERPRINT')`, returnMission, returnBooking, "return:"+trips[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `UPDATE dispatch_return_workflows SET delivery_group_id=$2,return_mission_id=$3,return_booking_id=$4 WHERE id=$1`, returned, groupID, returnMission, returnBooking); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"returns", "return-execution-status"} {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		item, err := source.Read(ctx, tx, kind, returned, true)
		if err != nil || len(item.References) != 4 {
			t.Fatal("genuine native return links unavailable", err, item.References)
		}
		if _, err := store.Resolve(ctx, tx, kind, returned, source.Specs[kind].Definition.Capability, scope, time.Now()); !errors.Is(err, admincontext.ErrUnmapped) {
			t.Fatal("old draft allocation followed new child identities", err)
		}
		binding, err := source.Binding(ctx, tx, kind, returned)
		tx.Rollback(ctx)
		if err != nil {
			t.Fatal(err)
		}
		approval := issuer.Approval("workflow-"+kind, kind, returned, source.Specs[kind].Definition.Capability, binding, scope)
		approval.Revision = 2
		if err := store.Import(ctx, issuer.Sign(t, approval), time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"booking", "mission-parent", "execution-link", "organization", "customer", "partial-children"} {
		t.Run("return-child-"+name, func(t *testing.T) {
			tx, err := db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			switch name {
			case "booking":
				_, err = tx.Exec(ctx, `UPDATE dispatch_return_workflows SET return_booking_id=$2 WHERE id=$1`, returned, bookings[1])
			case "mission-parent":
				_, err = tx.Exec(ctx, `UPDATE dispatch_delivery_missions SET source_ref='return:foreign-trip' WHERE id=$1`, returnMission)
			case "execution-link":
				_, err = tx.Exec(ctx, `UPDATE dispatch_mission_execution_links SET booking_id=$2 WHERE mission_id=$1`, returnMission, bookings[1])
			case "organization":
				_, err = tx.Exec(ctx, `UPDATE dispatch_delivery_groups SET organization_id=$2 WHERE id=$1`, groupID, otherOrg)
			case "customer":
				_, err = tx.Exec(ctx, `UPDATE dispatch_delivery_groups SET customer_id=gen_random_uuid() WHERE id=$1`, groupID)
			case "partial-children":
				_, err = tx.Exec(ctx, `UPDATE dispatch_return_workflows SET return_mission_id=NULL WHERE id=$1`, returned)
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, kind := range []string{"returns", "return-execution-status"} {
				if _, err := source.Read(ctx, tx, kind, returned, true); !errors.Is(err, pgx.ErrNoRows) {
					t.Fatal("foreign/partial native return children disclosed", name, kind, err)
				}
				if _, err := store.Resolve(ctx, tx, kind, returned, source.Specs[kind].Definition.Capability, scope, time.Now()); !errors.Is(err, admincontext.ErrUnmapped) {
					t.Fatal("old return allocation survived changed child authority", name, kind, err)
				}
			}
		})
	}
	// A valid FK alone does not establish the same workflow subject. Corrupted
	// historical cross-links must disappear, including from dependent returns.
	if _, err := db.Exec(ctx, `UPDATE dispatch_delivery_recovery_actions SET booking_id=$2 WHERE id=$1`, recovery, bookings[1]); err != nil {
		t.Fatal(err)
	}
	for kind, ref := range map[string]string{"delivery-recoveries": recovery, "returns": returned, "return-execution-status": returned} {
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
	if _, err := store.Resolve(ctx, tx, "incident-recovery-status", incident, "dispatch.incident_recovery.read", scope, time.Now()); !errors.Is(err, admincontext.ErrUnmapped) {
		t.Fatal("execution observation allocation survived changed incident parent", err)
	}
}
