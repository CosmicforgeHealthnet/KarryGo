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
	"github.com/jackc/pgx/v5"
)

func TestDispatchHandoverStateReadsBindFrozenNativeParentsAndExcludeCredentials(t *testing.T) {
	db := fixture.Database(t, "../../../migrations", true)
	ctx := context.Background()
	const provider = "11111111-1111-4111-8111-111111111111"
	const customer = "22222222-2222-4222-8222-222222222222"
	const org = "33333333-3333-4333-8333-333333333333"
	const otherOrg = "44444444-4444-4444-8444-444444444444"
	if _, err := db.Exec(ctx, `INSERT INTO providers(id,phone,full_name) VALUES($1,'PRIVATE_PHONE','PRIVATE_NAME')`, provider); err != nil {
		t.Fatal(err)
	}
	var bookings, trips, groups, missions, stops []string
	for _, organization := range []string{org, otherOrg} {
		var booking, trip, group, mission, stop string
		if err := db.QueryRow(ctx, `INSERT INTO dispatch_bookings(customer_id,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone) VALUES($1,'PRIVATE_ADDRESS',0,0,'PRIVATE_ADDRESS',0,0,100,'PRIVATE_RECEIVER','PRIVATE_PHONE') RETURNING id::text`, customer).Scan(&booking); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow(ctx, `INSERT INTO trips(booking_id,provider_id,customer_id,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone,organization_id) VALUES($1,$2,$3,'PRIVATE_ADDRESS',0,0,'PRIVATE_ADDRESS',0,0,100,'PRIVATE_RECEIVER','PRIVATE_PHONE',$4) RETURNING id::text`, booking, provider, customer, organization).Scan(&trip); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow(ctx, `INSERT INTO dispatch_delivery_groups(customer_id,source_type,source_ref,booking_id,trip_id,organization_id) VALUES($1,'CUSTOMER',$2::uuid::text,$3,$2::uuid,$4) RETURNING id::text`, customer, trip, booking, organization).Scan(&group); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow(ctx, `INSERT INTO dispatch_delivery_missions(delivery_group_id,source_type,source_ref,booking_id,trip_id) VALUES($1,'CUSTOMER',$2::uuid::text,$3,$2::uuid) RETURNING id::text`, group, trip, booking).Scan(&mission); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow(ctx, `INSERT INTO dispatch_mission_stops(mission_id,sequence,stop_type,stop_ref,location_snapshot) VALUES($1,1,'PICKUP','pickup-native','{"private":"PRIVATE_COORDINATES"}') RETURNING id::text`, mission).Scan(&stop); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(ctx, `INSERT INTO dispatch_parcel_manifests(mission_id,manifest_ref,version) VALUES($1::uuid,'manifest-native-'||$1::uuid::text,4)`, mission); err != nil {
			t.Fatal(err)
		}
		bookings = append(bookings, booking)
		trips = append(trips, trip)
		groups = append(groups, group)
		missions = append(missions, mission)
		stops = append(stops, stop)
	}
	insert := func(planned bool) string {
		t.Helper()
		var id string
		var group, mission, stop any
		missionRef, stopRef, manifestRef, version := "trip:"+trips[0], "pickup", "legacy-trip:"+trips[0], 1
		if planned {
			group, mission, stop = groups[0], missions[0], stops[0]
			missionRef, stopRef, manifestRef, version = missions[0], "pickup-native", "manifest-native-"+missions[0], 4
		}
		if err := db.QueryRow(ctx, `INSERT INTO dispatch_handover_challenges(challenge_ref,context_key,version,stage,delivery_group_id,mission_id,mission_ref,stop_id,stop_ref,trip_id,booking_id,manifest_ref,manifest_version,assigned_provider_id,intended_recipient_ref,recipient_type,masked_destination,otp_hash,qr_token_hash,expires_at,max_attempts,created_by_source,source_version) VALUES('PRIVATE_CHALLENGE_'||gen_random_uuid(),'PRIVATE_CONTEXT_'||gen_random_uuid(),1,'PICKUP',$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'PRIVATE_RECIPIENT','SENDER','PRIVATE_CONTACT','PRIVATE_OTP_HASH','PRIVATE_QR_HASH',now()+interval '10 minutes',5,'TEST_ONLY_SOURCE',9) RETURNING id::text`, group, mission, missionRef, stop, stopRef, trips[0], bookings[0], manifestRef, version, provider).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	legacy, planned := insert(false), insert(true)
	source := Source()
	profile, _ := adminowner.ForOwner(Owner)
	def := profile.Definitions["handover-states"]
	if def.SourceContract != "dispatch.admin-handover-state.v1" {
		t.Fatal("handover reused broad resource approval")
	}
	scope := adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: org, Tenant: org}
	issuer := contextfixture.New(t, Owner)
	store := &admincontext.Store{DB: db, Reader: source, Authority: issuer.Authority}
	for _, ref := range []string{legacy, planned} {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		binding, err := source.Binding(ctx, tx, "handover-states", ref)
		if err != nil {
			t.Fatal(err)
		}
		r, err := source.Read(ctx, tx, "handover-states", ref, true)
		tx.Rollback(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if r.SourceVersion != nil {
			t.Fatal("partial optimistic counter advertised as full revision")
		}
		if binding.Canonical.Organization != org || binding.Canonical.Tenant != org {
			t.Fatal("Trip canonical tenant lost")
		}
		if err := adminowner.ValidateResource(r, def, true); err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(r)
		if strings.Contains(string(raw), "PRIVATE_") || strings.Contains(string(raw), "otp") || strings.Contains(string(raw), "qr_token") {
			t.Fatal("secret handover material leaked")
		}
		a := issuer.Approval("handover-"+ref, "handover-states", ref, def.Capability, binding, scope)
		old := a
		old.SourceContract = "dispatch.admin-resources.v1"
		if err := store.Import(ctx, issuer.Sign(t, old), time.Now()); err == nil {
			t.Fatal("broad old approval authorized new handover resource")
		}
		bad := a
		bad.Scope.Tenant = otherOrg
		if err := store.Import(ctx, issuer.Sign(t, bad), time.Now()); err == nil {
			t.Fatal("supplied tenant overrode canonical Trip")
		}
		if err := store.Import(ctx, issuer.Sign(t, a), time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	policy := policyfixture.New(t, scope, Owner, def.Capability, def.SourceContract)
	key := []byte(strings.Repeat("h", 32))
	h := &adminowner.Handler{Owner: Owner, Path: profile.Path, DB: db, Source: source, Contexts: store, Policy: policy.Engine, Secret: key, Enabled: true, Definitions: profile.Definitions}
	engine := gin.New()
	h.Register(engine)
	query := adminread.Query{Scope: adminread.Scope{Market: scope.Market, Zone: scope.Zone, Organization: scope.Organization, Tenant: scope.Tenant}, Staff: "staff-1", Role: "staff", Purpose: "case-1", PolicyID: policy.Engine.ID(), PolicyVersion: policy.Engine.Version(), PolicyChecksum: policy.Engine.Checksum(), Start: time.Now().UTC().Add(-time.Hour), End: time.Now().UTC().Add(time.Second), Limit: 10}
	request := httptest.NewRequest("GET", profile.Path+"/resources/handover-states/"+planned+"?"+query.Values().Encode(), nil)
	if err := serviceauth.SignRequest(request, adminread.Caller, key, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != 200 || strings.Contains(response.Body.String(), "PRIVATE_") {
		t.Fatal("signed handover read", response.Code, response.Body.String())
	}
	if dir := os.Getenv("ADMIN_OWNER_CUSTODY_CAPTURE_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "dispatch-handover-state.json"), response.Body.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
	changes := map[string]string{
		"foreign booking":    `UPDATE dispatch_handover_challenges SET booking_id=$2 WHERE id=$1`,
		"foreign mission":    `UPDATE dispatch_handover_challenges SET mission_id=$2 WHERE id=$1`,
		"foreign stop":       `UPDATE dispatch_handover_challenges SET stop_id=$2 WHERE id=$1`,
		"wrong group tenant": `UPDATE dispatch_delivery_groups SET organization_id=$2 WHERE id=$1`,
	}
	for name, sql := range changes {
		t.Run(name, func(t *testing.T) {
			tx, err := db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			args := []any{planned}
			switch name {
			case "foreign booking":
				args = append(args, bookings[1])
			case "foreign mission":
				args = append(args, missions[1])
			case "foreign stop":
				args = append(args, stops[1])
			case "wrong group tenant":
				args = []any{groups[0], otherOrg}
			}
			if _, err := tx.Exec(ctx, sql, args...); err != nil {
				t.Fatal(err)
			}
			if _, err := source.Read(ctx, tx, "handover-states", planned, true); !errors.Is(err, pgx.ErrNoRows) {
				t.Fatal("contradictory native link disclosed", err)
			}
			if _, err := store.Resolve(ctx, tx, "handover-states", planned, def.Capability, scope, time.Now()); !errors.Is(err, admincontext.ErrUnmapped) {
				t.Fatal("old allocation survived contradictory native link", err)
			}
		})
	}
	t.Run("unknown frozen manifest", func(t *testing.T) {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		var badRef string
		if err := tx.QueryRow(ctx, `INSERT INTO dispatch_handover_challenges SELECT (jsonb_populate_record(NULL::dispatch_handover_challenges,to_jsonb(h)||jsonb_build_object('id',gen_random_uuid(),'challenge_ref',h.challenge_ref||'_bad','context_key',h.context_key||'_bad','manifest_version',5,'status','INVALIDATED','invalidated_at',now()))).* FROM dispatch_handover_challenges h WHERE h.id=$1 RETURNING id::text`, planned).Scan(&badRef); err != nil {
			t.Fatal(err)
		}
		if _, err := source.Read(ctx, tx, "handover-states", badRef, true); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatal("unproven frozen manifest disclosed", err)
		}
	})
	t.Run("later manifest cannot replace frozen version", func(t *testing.T) {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err := tx.Exec(ctx, `UPDATE dispatch_parcel_manifests SET status='SUPERSEDED' WHERE mission_id=$1;`, missions[0]); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO dispatch_parcel_manifests(mission_id,manifest_ref,version) VALUES($1::uuid,'manifest-native-'||$1::uuid::text,5)`, missions[0]); err != nil {
			t.Fatal(err)
		}
		if _, err := source.Read(ctx, tx, "handover-states", planned, true); err != nil {
			t.Fatal("new manifest replaced captured history", err)
		}
		if _, err := store.Resolve(ctx, tx, "handover-states", planned, def.Capability, scope, time.Now()); err != nil {
			t.Fatal("unchanged frozen context invalidated", err)
		}
	})

}
