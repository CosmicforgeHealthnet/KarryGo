package analyticsproducer

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/adminresources"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/booking"
	"cosmicforge/logistics/shared/go/admincontext"
	contextfixture "cosmicforge/logistics/shared/go/admincontext/testfixture"
	fixture "cosmicforge/logistics/shared/go/adminowner/testfixture"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/analyticsadmin"
	"cosmicforge/logistics/shared/go/networkanalytics"
)

func insertBooking(t *testing.T, db *pgxpool.Pool) string {
	t.Helper()
	var id string
	if err := db.QueryRow(context.Background(), `INSERT INTO dispatch_bookings(customer_id,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone) VALUES($1,'Marina',6.52,3.37,'b',0,0,100,'Ada','+2348000000000') RETURNING id::text`, uuid.NewString()).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func unmatchedFacts(t *testing.T, db *pgxpool.Pool, booking string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(context.Background(), `SELECT count(*) FROM dispatch_marketplace_outbox WHERE event_type=$1 AND aggregate_id=$2`, networkanalytics.EventDispatchBookingUnmatched, booking).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Migration 000076: one unmatched demand fact per authoritative unmatched
// outcome, written in the booking's own compare-and-set transaction, whatever
// the number of duplicate no_provider_found events, retries or racing workers.
func TestPostgresDispatchUnmatchedFactProducedExactlyOncePerAuthoritativeOutcome(t *testing.T) {
	db := fixture.Database(t, "../../../migrations", true)
	ctx := context.Background()
	repo := booking.NewPostgresRepository(db)
	service := booking.NewService(repo, nil, nil, nil, nil, nil, 0)

	// Duplicate and retried no_provider_found events.
	duplicate := insertBooking(t, db)
	for i := 0; i < 3; i++ {
		if err := service.HandleNoProviderFound(ctx, duplicate); err != nil {
			t.Fatal(err)
		}
	}
	if ok, err := repo.SetUnmatched(ctx, duplicate, time.Now()); err != nil || ok {
		t.Fatal("retried CAS re-applied", ok, err)
	}
	if _, err := db.Exec(ctx, `UPDATE dispatch_bookings SET status='unmatched',updated_at=now() WHERE id=$1`, duplicate); err != nil {
		t.Fatal(err)
	}
	if n := unmatchedFacts(t, db, duplicate); n != 1 {
		t.Fatal("duplicate/retried outcome produced", n, "facts")
	}

	// Concurrent workers handling the same broadcast outcome.
	concurrent := insertBooking(t, db)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- service.HandleNoProviderFound(ctx, concurrent)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if n := unmatchedFacts(t, db, concurrent); n != 1 {
		t.Fatal("concurrent outcome produced", n, "facts")
	}

	// A matched booking and a cancelled booking are never unmatched demand,
	// even when a late no_provider_found event arrives.
	matched := insertBooking(t, db)
	if _, err := db.Exec(ctx, `UPDATE dispatch_bookings SET status='accepted' WHERE id=$1`, matched); err != nil {
		t.Fatal(err)
	}
	cancelled := insertBooking(t, db)
	if ok, err := repo.SetTerminal(ctx, cancelled, booking.StatusCancelled, booking.PaymentUnpaid, "customer_cancelled", time.Now()); err != nil || !ok {
		t.Fatal(ok, err)
	}
	for _, id := range []string{matched, cancelled} {
		if err := service.HandleNoProviderFound(ctx, id); err != nil {
			t.Fatal(err)
		}
		if n := unmatchedFacts(t, db, id); n != 0 {
			t.Fatal("non-unmatched booking produced an unmatched fact", id, n)
		}
	}
	var status string
	if err := db.QueryRow(ctx, `SELECT status FROM dispatch_bookings WHERE id=$1`, matched).Scan(&status); err != nil || status != "accepted" {
		t.Fatal("late event clobbered a matched booking", status, err)
	}

	// The fact is analytical only, classified as the authoritative outcome,
	// and identified by the booking itself.
	var aggregateType, target string
	var version int64
	var raw []byte
	if err := db.QueryRow(ctx, `SELECT aggregate_type,aggregate_version,target_service,payload FROM dispatch_marketplace_outbox WHERE event_type=$1 AND aggregate_id=$2`, networkanalytics.EventDispatchBookingUnmatched, duplicate).Scan(&aggregateType, &version, &target, &raw); err != nil {
		t.Fatal(err)
	}
	if aggregateType != "analytics:dispatch_booking" || version != 1 || target != "analytics-service" {
		t.Fatal("unexpected fact identity", aggregateType, version, target)
	}
	for _, leak := range []string{"Marina", "Ada", "6.52", "+234"} {
		if strings.Contains(string(raw), leak) {
			t.Fatalf("unmatched fact leaked %q: %s", leak, raw)
		}
	}
	var payload map[string]any
	if json.Unmarshal(raw, &payload) != nil || payload["booking_id"] != duplicate || payload["product_code"] != networkanalytics.ProductDispatchParcel {
		t.Fatal("payload", string(raw))
	}
	if authority, kind, ok := networkanalytics.Classify(networkanalytics.EventDispatchBookingUnmatched, ""); !ok || authority != networkanalytics.AuthorityDispatch || kind != networkanalytics.KindMissionFailed {
		t.Fatal("unmatched fact not classified as the authoritative outcome")
	}
}

func payloadOf(t *testing.T, db *pgxpool.Pool, event, aggregateType, id string) map[string]any {
	t.Helper()
	var raw []byte
	var hash string
	if err := db.QueryRow(context.Background(), `SELECT payload,payload_hash FROM dispatch_marketplace_outbox WHERE target_service='analytics-service' AND event_type=$1 AND aggregate_type=$2 AND aggregate_id=$3`, event, aggregateType, id).Scan(&raw, &hash); err != nil {
		t.Fatal(event, aggregateType, err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func commerceBooking(t *testing.T, db *pgxpool.Pool) (string, string) {
	t.Helper()
	ctx := context.Background()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var id, mission string
	if err := tx.QueryRow(ctx, `INSERT INTO dispatch_bookings(customer_id,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone) VALUES($1,'a',0,0,'b',0,0,100,'r','p') RETURNING id::text`, uuid.NewString()).Scan(&id); err != nil {
		t.Fatal(err)
	}
	ref := "dcm_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	// As commercemission does: the mission names the booking after inserting
	// it, and records the booking as assignment attempt 1.
	if err := tx.QueryRow(ctx, `INSERT INTO commerce_missions(mission_ref,parent_order_ref,vendor_order_refs,customer_id,booking_id,agreed_fee_minor,status,operation_ref,canonical_ref) VALUES($1,'order','{vendor-order}',$2,$3,500,'accepted',$1,$4) RETURNING id::text`, "m-"+id, uuid.NewString(), id, ref).Scan(&mission); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO commerce_mission_assignment_attempts(commerce_mission_id,attempt_number,booking_id,state) VALUES($1,1,$2,'SEARCHING')`, mission, id); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO dispatch_marketplace_outbox(event_type,aggregate_type,aggregate_id,aggregate_version,payload,payload_hash,target_service) VALUES('commerce.mission.created.v1','COMMERCE_MISSION',$1,1,'{"state":"SEARCHING"}'::jsonb,'h','vendor-service')`, ref); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return id, ref
}

// reassignCommerce moves a commerce mission to a new booking, as
// commercemission's reassignment does (attempt N+1 on a new booking).
func reassignCommerce(t *testing.T, db *pgxpool.Pool, ref string) string {
	t.Helper()
	ctx := context.Background()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var id string
	if err := tx.QueryRow(ctx, `INSERT INTO dispatch_bookings(customer_id,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone) VALUES($1,'a',0,0,'b',0,0,100,'r','p') RETURNING id::text`, uuid.NewString()).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE commerce_missions SET booking_id=$2::uuid WHERE canonical_ref=$1`, ref, id); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO commerce_mission_assignment_attempts(commerce_mission_id,attempt_number,booking_id,state) SELECT id,2,$2::uuid,'SEARCHING' FROM commerce_missions WHERE canonical_ref=$1`, ref, id); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return id
}

func insertTrip(t *testing.T, db *pgxpool.Pool, booking, rider string) string {
	t.Helper()
	var trip string
	if err := db.QueryRow(context.Background(), `INSERT INTO trips(booking_id,provider_id,customer_id,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone) VALUES($1,$2,$3,'a',0,0,'b',0,0,100,'r','p') RETURNING id::text`, booking, rider, uuid.NewString()).Scan(&trip); err != nil {
		t.Fatal(err)
	}
	return trip
}

// tripEvent writes a trip lifecycle event as the trip feature does (to Fleet);
// migration 000064 copies it to analytics.
func tripEvent(t *testing.T, db *pgxpool.Pool, event, trip string, version int64, rider string) {
	t.Helper()
	if _, err := db.Exec(context.Background(), `INSERT INTO dispatch_marketplace_outbox(event_type,aggregate_type,aggregate_id,aggregate_version,payload,payload_hash,target_service) VALUES($1,'TRIP',$2,$3,$4::jsonb,'h','fleet-partner-service')`, event, trip, version, `{"provider_id":"`+rider+`"}`); err != nil {
		t.Fatal(err)
	}
}

func partnerRequest(t *testing.T, db *pgxpool.Pool) string {
	t.Helper()
	id := uuid.NewString()
	if _, err := db.Exec(context.Background(), `INSERT INTO request_broadcasts(booking_id,broadcast_radius_km,expires_at,booking_payload) VALUES($1,5,now()+interval '1 minute',$2::jsonb)`, id, `{"customer_id":"`+uuid.NewString()+`","receiver_phone":"+2348000000000"}`); err != nil {
		t.Fatal(err)
	}
	return id
}

// kindCounts counts the analytics facts of each lifecycle kind recorded for
// the given subjects (booking, commerce mission, partner request or trip ids).
func kindCounts(t *testing.T, db *pgxpool.Pool, product string, ids ...string) map[networkanalytics.Kind]int {
	t.Helper()
	rows, err := db.Query(context.Background(), `SELECT event_type,aggregate_type,payload FROM dispatch_marketplace_outbox WHERE target_service='analytics-service' AND aggregate_id=ANY($1)`, ids)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[networkanalytics.Kind]int{}
	for rows.Next() {
		var event, aggregate string
		var raw []byte
		if err := rows.Scan(&event, &aggregate, &raw); err != nil {
			t.Fatal(err)
		}
		_, kind, ok := networkanalytics.Classify(event, "")
		if !ok {
			continue
		}
		var payload map[string]any
		_ = json.Unmarshal(raw, &payload)
		if got := networkanalytics.ProductFor(event, strings.TrimPrefix(aggregate, "analytics:"), payload); got != product {
			t.Fatalf("%s on %s attributed to %s, want %s", event, aggregate, got, product)
		}
		out[kind]++
	}
	return out
}

func wantKinds(t *testing.T, name string, got map[networkanalytics.Kind]int, want map[networkanalytics.Kind]int) {
	t.Helper()
	for _, kind := range []networkanalytics.Kind{networkanalytics.KindMissionCommitted, networkanalytics.KindMissionAssigned, networkanalytics.KindMissionCompleted, networkanalytics.KindMissionFailed, networkanalytics.KindMissionCancelled} {
		if got[kind] != want[kind] {
			t.Errorf("%s: %s = %d, want %d (all %v)", name, kind, got[kind], want[kind], got)
		}
	}
}

// Every Dispatch mission is recorded once per lifecycle kind, whatever path
// executes it: customer parcel bookings, Vendor commerce missions (including
// reassignment onto new bookings) and partner-API requests. Commerce work
// carries DISPATCH_COMMERCE; parcel facts are unchanged.
func TestPostgresDispatchLifecycleFactsOncePerMissionAndKind(t *testing.T) {
	db := fixture.Database(t, "../../../migrations", true)
	ctx := context.Background()
	service := booking.NewService(booking.NewPostgresRepository(db), nil, nil, nil, nil, nil, 0)
	repo := booking.NewPostgresRepository(db)
	rider := uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO providers(id,phone,full_name) VALUES($1,'P1','N1')`, rider); err != nil {
		t.Fatal(err)
	}
	const (
		committed = networkanalytics.KindMissionCommitted
		assigned  = networkanalytics.KindMissionAssigned
		completed = networkanalytics.KindMissionCompleted
		failed    = networkanalytics.KindMissionFailed
		cancelled = networkanalytics.KindMissionCancelled
	)

	// Parcel: committed, matched (booking accepted + organization trip.created),
	// completed.
	parcel := insertBooking(t, db)
	if _, err := db.Exec(ctx, `INSERT INTO request_broadcasts(booking_id,broadcast_radius_km,expires_at,booking_payload) VALUES($1,5,now()+interval '1 minute','{}')`, parcel); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `UPDATE dispatch_bookings SET status='accepted' WHERE id=$1`, parcel); err != nil {
		t.Fatal(err)
	}
	parcelTrip := insertTrip(t, db, parcel, rider)
	tripEvent(t, db, "trip.created.v1", parcelTrip, 1, rider)
	tripEvent(t, db, "trip.completed.v1", parcelTrip, 5, rider)
	wantKinds(t, "parcel", kindCounts(t, db, networkanalytics.ProductDispatchParcel, parcel, parcelTrip), map[networkanalytics.Kind]int{committed: 1, assigned: 1, completed: 1})

	// Commerce: one request, assigned, reassigned onto a new booking, completed.
	first, ref := commerceBooking(t, db)
	if _, err := db.Exec(ctx, `UPDATE dispatch_bookings SET status='accepted' WHERE id=$1`, first); err != nil {
		t.Fatal(err)
	}
	firstTrip := insertTrip(t, db, first, rider)
	tripEvent(t, db, "trip.cancelled.v1", firstTrip, 3, rider)
	second := reassignCommerce(t, db, ref)
	if _, err := db.Exec(ctx, `UPDATE dispatch_bookings SET status='accepted' WHERE id=$1`, second); err != nil {
		t.Fatal(err)
	}
	secondTrip := insertTrip(t, db, second, rider)
	tripEvent(t, db, "trip.completed.v1", secondTrip, 4, rider)
	wantKinds(t, "commerce", kindCounts(t, db, networkanalytics.ProductDispatchCommerce, ref, first, second, firstTrip, secondTrip), map[networkanalytics.Kind]int{committed: 1, assigned: 1, completed: 1, cancelled: 1})

	// Partner-API: no dispatch_bookings row; its broadcast is the request.
	partner := partnerRequest(t, db)
	partnerTrip := insertTrip(t, db, partner, rider)
	if _, err := db.Exec(ctx, `UPDATE request_broadcasts SET status='accepted' WHERE booking_id=$1`, partner); err != nil {
		t.Fatal(err)
	}
	tripEvent(t, db, "trip.created.v1", partnerTrip, 1, rider)
	tripEvent(t, db, "trip.completed.v1", partnerTrip, 6, rider)
	wantKinds(t, "partner", kindCounts(t, db, networkanalytics.ProductDispatchParcel, partner, partnerTrip), map[networkanalytics.Kind]int{committed: 1, assigned: 1, completed: 1})
	partnerUnmatched := partnerRequest(t, db)
	for i := 0; i < 2; i++ {
		if _, err := db.Exec(ctx, `UPDATE request_broadcasts SET status='no_provider_found' WHERE booking_id=$1`, partnerUnmatched); err != nil {
			t.Fatal(err)
		}
	}
	wantKinds(t, "partner unmatched", kindCounts(t, db, networkanalytics.ProductDispatchParcel, partnerUnmatched), map[networkanalytics.Kind]int{committed: 1, failed: 1})

	// Unmatched customer booking: committed and failed, never assigned.
	unmatched := insertBooking(t, db)
	if err := service.HandleNoProviderFound(ctx, unmatched); err != nil {
		t.Fatal(err)
	}
	wantKinds(t, "unmatched", kindCounts(t, db, networkanalytics.ProductDispatchParcel, unmatched), map[networkanalytics.Kind]int{committed: 1, failed: 1})

	// Cancelled before matching: committed and cancelled.
	cancelledBooking := insertBooking(t, db)
	if ok, err := repo.SetTerminal(ctx, cancelledBooking, booking.StatusCancelled, booking.PaymentUnpaid, "customer_cancelled", time.Now()); err != nil || !ok {
		t.Fatal(ok, err)
	}
	wantKinds(t, "cancelled", kindCounts(t, db, networkanalytics.ProductDispatchParcel, cancelledBooking), map[networkanalytics.Kind]int{committed: 1, cancelled: 1})

	// Cancelled after matching: the trip's cancellation is the one fact.
	late := insertBooking(t, db)
	lateTrip := insertTrip(t, db, late, rider)
	if ok, err := repo.SetTerminal(ctx, late, booking.StatusCancelled, booking.PaymentUnpaid, "customer_cancelled", time.Now()); err != nil || !ok {
		t.Fatal(ok, err)
	}
	tripEvent(t, db, "trip.cancelled.v1", lateTrip, 2, rider)
	wantKinds(t, "cancelled after match", kindCounts(t, db, networkanalytics.ProductDispatchParcel, late, lateTrip), map[networkanalytics.Kind]int{committed: 1, assigned: 1, cancelled: 1})

	// trip.created.v1 never reaches analytics: the trip row is the assignment.
	var copies int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM dispatch_marketplace_outbox WHERE target_service='analytics-service' AND event_type='trip.created.v1'`).Scan(&copies); err != nil || copies != 0 {
		t.Fatal("trip.created copied to analytics", copies, err)
	}
	var hash, text string
	if err := db.QueryRow(ctx, `SELECT payload_hash,encode(digest(payload::text,'sha256'),'hex') FROM dispatch_marketplace_outbox WHERE target_service='analytics-service' AND aggregate_id=$1`, secondTrip).Scan(&hash, &text); err != nil || hash != text {
		t.Fatal("rewritten commerce trip payload hash", hash, text, err)
	}

	// Commerce delivery-mission facts carry the commerce product too.
	group, mission := uuid.NewString(), uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO dispatch_delivery_groups(id,source_type,source_ref) VALUES($1,'VENDOR_COMMERCE','order')`, group); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO dispatch_delivery_missions(id,delivery_group_id,source_type,source_ref) VALUES($1,$2,'VENDOR_COMMERCE','order')`, mission, group); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO dispatch_marketplace_outbox(event_type,aggregate_type,aggregate_id,aggregate_version,payload,payload_hash,target_service) VALUES('dispatch.offer.declined.v1','delivery_mission',$1,2,'{"reason":"BUSY"}'::jsonb,'h','vendor-service')`, mission); err != nil {
		t.Fatal(err)
	}
	if got := payloadOf(t, db, "dispatch.offer.declined.v1", "analytics:delivery_mission", mission)["product_code"]; got != networkanalytics.ProductDispatchCommerce {
		t.Fatal("commerce delivery-mission decline product", got)
	}
}

// The producer anchors every Dispatch population to its enrolled kind: trips
// ('TRIP' lifecycle), customer bookings (including unmatched ones), partner
// requests, delivery missions and commerce missions. Only records with an owner-signed context
// enter a scope; nothing is declared unattested any more.
func TestPostgresDispatchProducerAnchorsEveryPopulationToSignedContexts(t *testing.T) {
	db := fixture.Database(t, "../../../migrations", true)
	ctx := context.Background()
	service := booking.NewService(booking.NewPostgresRepository(db), nil, nil, nil, nil, nil, 0)
	unmatched := insertBooking(t, db)
	if err := service.HandleNoProviderFound(ctx, unmatched); err != nil {
		t.Fatal(err)
	}
	unsigned := insertBooking(t, db)
	accepted := insertBooking(t, db)
	if _, err := db.Exec(ctx, `UPDATE dispatch_bookings SET status='accepted' WHERE id=$1`, accepted); err != nil {
		t.Fatal(err)
	}
	rider, org := uuid.NewString(), uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO providers(id,phone,full_name) VALUES($1,'P1','N1')`, rider); err != nil {
		t.Fatal(err)
	}
	var trip string
	if err := db.QueryRow(ctx, `INSERT INTO trips(booking_id,provider_id,customer_id,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone,organization_id) VALUES($1,$2,$3,'a',0,0,'b',0,0,100,'r','p',$4) RETURNING id::text`, accepted, rider, uuid.NewString(), org).Scan(&trip); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO dispatch_marketplace_outbox(event_type,aggregate_type,aggregate_id,aggregate_version,payload,payload_hash,target_service) VALUES('trip.completed.v1','TRIP',$1,5,$2::jsonb,'h','fleet-partner-service')`, trip, `{"provider_id":"`+rider+`"}`); err != nil {
		t.Fatal(err)
	}
	group, mission := uuid.NewString(), uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO dispatch_delivery_groups(id,source_type,source_ref,organization_id) VALUES($1,'PARTNER_API','src',$2)`, group, org); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO dispatch_delivery_missions(id,delivery_group_id,source_type,source_ref) VALUES($1,$2,'PARTNER_API','src')`, mission, group); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO dispatch_marketplace_outbox(event_type,schema_version,aggregate_type,aggregate_id,aggregate_version,payload,payload_hash,target_service) VALUES('dispatch.capability_mismatch.v1',1,'delivery_mission',$1,1,'{"reason_codes":"CAPACITY"}'::jsonb,'h','analytics-service')`, mission); err != nil {
		t.Fatal(err)
	}
	commerceRef := "dcm_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := db.Exec(ctx, `INSERT INTO commerce_missions(mission_ref,parent_order_ref,vendor_order_refs,customer_id,booking_id,agreed_fee_minor,status,operation_ref,canonical_ref) VALUES('m1','order','{vo}',$1,$2,500,'accepted','m1',$3)`, uuid.NewString(), insertBooking(t, db), commerceRef); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO dispatch_marketplace_outbox(event_type,aggregate_type,aggregate_id,aggregate_version,payload,payload_hash,target_service) VALUES('commerce.mission.created.v1','COMMERCE_MISSION',$1,1,'{"state":"SEARCHING"}'::jsonb,'h','vendor-service')`, commerceRef); err != nil {
		t.Fatal(err)
	}

	partner := partnerRequest(t, db)
	incident := insertIncident(t, db, trip, rider)
	scope := adminpolicy.Scope{Product: "karryam", Market: "lagos", Zone: "island", Organization: org, Tenant: org}
	issuer := contextfixture.New(t, Owner)
	store := &admincontext.Store{DB: db, Reader: adminresources.Source(), Authority: issuer.Authority}
	for kind, ref := range map[string]string{"missions": trip, "bookings": unmatched, "delivery-missions": mission, "commerce-missions": commerceRef, "partner-requests": partner, "incidents": incident} {
		tx, _ := db.Begin(ctx)
		binding, err := adminresources.Source().Binding(ctx, tx, kind, ref)
		tx.Rollback(ctx)
		if err != nil {
			t.Fatal(kind, err)
		}
		if err := store.Import(ctx, issuer.Sign(t, issuer.Approval("ctx-"+kind, kind, ref, analyticsadmin.CoverageExportCapability, binding, scope)), time.Now()); err != nil {
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
	if err != nil {
		t.Fatal(err)
	}
	if result.Unattested != 0 {
		t.Fatal("a Dispatch population is still unattested", result)
	}
	if result.Unscoped == 0 {
		t.Fatal("unsigned bookings must stay out of every scope", result)
	}
	got := map[string]bool{}
	for _, body := range publisher.bodies {
		v, err := analyticsadmin.VerifyManifest(authority, body, window.Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		if v.Approval.Scope != scope {
			t.Fatal("manifest for unexpected scope", v.Approval.Scope)
		}
		if v.Family != analyticsadmin.FamilyNetworkEvents {
			continue
		}
		for _, m := range v.Members {
			if m.EntityID == unsigned || m.EntityID == accepted {
				t.Fatal("unsigned booking attested", m)
			}
			got[m.EntityType+"|"+m.EventName] = true
		}
	}
	for _, want := range []string{"TRIP|trip.completed.v1", "dispatch_booking|" + networkanalytics.EventDispatchBookingUnmatched, "dispatch_booking|dispatch.booking.committed.v1", "delivery_mission|dispatch.capability_mismatch.v1", "COMMERCE_MISSION|commerce.mission.created.v1", "partner_request|dispatch.booking.committed.v1", "delivery_incident|dispatch.incident_recovery.started.v1"} {
		if !got[want] {
			t.Fatal("signed population fact not attested", want, got)
		}
	}
}
