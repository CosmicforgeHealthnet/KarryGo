// Package stacke2e drives the Sections 9-11 journey-sharing surfaces against a
// locally RUNNING service stack (docs/validation/sections911-local-stack.ps1).
//
// It is black-box: every read, share lifecycle step and provider location
// ingestion goes over real HTTP to the running Customer, Taxi, Dispatch,
// Hauling and Vendor processes. SQL is used only to place the authority-owned
// journey records (bookings/trips/missions) that the booking and matching
// flows would otherwise create; those flows are out of scope for Sections 9-11.
//
// The test skips unless SECTIONS911_STACK_E2E=1. Every credential it uses is a
// disposable local fixture read from the stack's own runtime config.
package stacke2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/journeyshare"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	customerURL = "http://127.0.0.1:58101"
	taxiURL     = "http://127.0.0.1:58102"
	dispatchURL = "http://127.0.0.1:58103"
	haulingURL  = "http://127.0.0.1:58104"
	vendorURL   = "http://127.0.0.1:58112"
	viewer      = "http://127.0.0.1:58101"
	dsnBase     = "postgres://karry_test:karry_test@127.0.0.1:55432/"
	// Seeded by the Dispatch dev seed (seeds/dev_seed.sql); only rider 2 has no
	// active trip, so a single current binding exists for it.
	dispatchRider      = "d2000000-0000-4000-8000-000000000002"
	dispatchRiderPhone = "+2348077770002"
)

type stack struct {
	t        *testing.T
	secret   []byte
	signer   *auth.TokenSigner
	dbs      map[string]*pgxpool.Pool
	http     *http.Client
	evidence map[string]any
	mu       sync.Mutex
}

func newStack(t *testing.T) *stack {
	t.Helper()
	if os.Getenv("SECTIONS911_STACK_E2E") != "1" {
		t.Skip("SECTIONS911_STACK_E2E=1 and a running local stack are required")
	}
	secret := strings.TrimSpace(os.Getenv("SECTIONS911_STACK_SECRET"))
	if secret == "" {
		t.Fatal("SECTIONS911_STACK_SECRET is required")
	}
	s := &stack{t: t, secret: []byte(secret), signer: auth.NewTokenSigner([]byte(secret)), dbs: map[string]*pgxpool.Pool{}, http: &http.Client{Timeout: 10 * time.Second, Transport: pooledTransport()}, evidence: map[string]any{}}
	for _, name := range []string{"customer", "taxi", "dispatch", "hauling", "vendor"} {
		pool, err := pgxpool.New(context.Background(), dsnBase+"closure_stack_"+name+"?sslmode=disable")
		if err != nil {
			t.Fatal(err)
		}
		s.dbs[name] = pool
		t.Cleanup(pool.Close)
	}
	return s
}

// pooledTransport keeps the load generator itself from exhausting ports.
func pooledTransport() http.RoundTripper {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConns = 256
	t.MaxIdleConnsPerHost = 64
	return t
}

func (s *stack) exec(db, sql string, args ...any) {
	s.t.Helper()
	if _, err := s.dbs[db].Exec(context.Background(), sql, args...); err != nil {
		s.t.Fatalf("%s fixture: %v\n%s", db, err, sql)
	}
}

// customer creates a real Customer-owned account record (shares FK to it).
func (s *stack) customer() string {
	s.t.Helper()
	id := uuid.NewString()
	s.exec("customer", `INSERT INTO customers(id,email) VALUES($1,$2)`, id, "e2e-"+id+"@example.test")
	return id
}

func (s *stack) token(subject, role, service string) string {
	s.t.Helper()
	tok, err := s.signer.Sign(auth.Claims{Subject: subject, Role: role, Service: service, SessionID: uuid.NewString(), Type: "access", ExpiresAt: time.Now().Add(time.Hour).Unix()})
	if err != nil {
		s.t.Fatal(err)
	}
	return tok
}

type reply struct {
	Code int
	Raw  string
	Body map[string]any
}

func (r reply) data() map[string]any {
	d, _ := r.Body["data"].(map[string]any)
	return d
}
func (r reply) errCode() string {
	e, _ := r.Body["error"].(map[string]any)
	c, _ := e["code"].(string)
	return c
}

func (s *stack) call(method, url, bearer string, headers map[string]string, body any) reply {
	s.t.Helper()
	var reader io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		s.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := s.http.Do(req)
	if err != nil {
		s.t.Fatalf("%s %s: %v", method, url, err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := reply{Code: res.StatusCode, Raw: string(raw)}
	_ = json.Unmarshal(raw, &out.Body)
	return out
}

func (s *stack) expect(r reply, code int, what string) reply {
	s.t.Helper()
	if r.Code != code {
		s.t.Fatalf("%s: status %d want %d: %s", what, r.Code, code, r.Raw)
	}
	return r
}

func (s *stack) noLeak(raw string, what string, private ...string) {
	s.t.Helper()
	for _, p := range private {
		if p != "" && strings.Contains(raw, p) {
			s.t.Fatalf("%s leaked %q: %s", what, p, raw)
		}
	}
	for _, word := range []string{"otp", "qr_", "password", "secret", "payment", "receiver_phone", "customer_id", "provider_id", "token_hash"} {
		if strings.Contains(strings.ToLower(raw), word) {
			s.t.Fatalf("%s leaked field %q: %s", what, word, raw)
		}
	}
}

func (s *stack) record(key string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evidence[key] = value
}

// share runs N01 and returns (shareID, version, token).
func (s *stack) share(customer, authority, ref, product string) (string, int64, string) {
	s.t.Helper()
	r := s.expect(s.call("POST", customerURL+"/api/v1/customer/journey-shares", customer, map[string]string{"Idempotency-Key": uuid.NewString()}, map[string]any{"authority": authority, "source_ref": ref, "product_code": product}), 200, "N01 "+authority)
	d := r.data()
	session, _ := d["session"].(map[string]any)
	tok, _ := d["token"].(string)
	id, _ := session["id"].(string)
	version, _ := session["version"].(float64)
	if tok == "" || id == "" {
		s.t.Fatalf("N01 returned no token: %s", r.Raw)
	}
	return id, int64(version), tok
}

func (s *stack) public(tok string) reply {
	return s.call("GET", customerURL+"/api/v1/customer/public/journey-shares/"+tok, "", map[string]string{"Origin": viewer, "X-Forwarded-For": "198.51.100." + fmt.Sprint(time.Now().UnixNano()%250+1)}, nil)
}

func location(r reply) map[string]any {
	l, _ := r.data()["location"].(map[string]any)
	return l
}

// signedRead calls a private projection route directly as the named caller.
func (s *stack) signedRead(base, path, ref, customer, caller string) reply {
	s.t.Helper()
	req, _ := http.NewRequest("GET", base+path+ref+"?customer_id="+customer, nil)
	if err := serviceauth.SignRequest(req, caller, s.secret, nil, time.Now().UTC()); err != nil {
		s.t.Fatal(err)
	}
	res, err := s.http.Do(req)
	if err != nil {
		s.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := reply{Code: res.StatusCode, Raw: string(raw)}
	_ = json.Unmarshal(raw, &out.Body)
	return out
}

// --- fixtures -------------------------------------------------------------

type taxiRide struct{ booking, provider, customer string }

func (s *stack) seedTaxiRide(status string) taxiRide {
	f := taxiRide{booking: uuid.NewString(), provider: uuid.NewString(), customer: s.customer()}
	market, zone, vehicle := uuid.NewString(), uuid.NewString(), uuid.NewString()
	s.exec("taxi", `INSERT INTO markets(id,country,name,code)VALUES($1,'NG','e2e',$2)`, market, "E2E-"+market[:8])
	s.exec("taxi", `INSERT INTO operating_zones(id,market_id,name,code,center_lat,center_lng,match_radius_km,supported_classes)VALUES($1,$2,'e2e',$3,6.5,3.3,10,'{economy}')`, zone, market, "Z-"+zone[:8])
	s.exec("taxi", `INSERT INTO taxi_providers(id,phone,first_name)VALUES($1,$2,'Ada')`, f.provider, "+234"+strings.ReplaceAll(f.provider, "-", "")[:10])
	s.exec("taxi", `INSERT INTO vehicles(id,provider_id,vehicle_class,plate_number,status,make,model)VALUES($1,$2,'economy',$3,'active','Toyota','Corolla')`, vehicle, f.provider, "E2E"+vehicle[:6])
	s.exec("taxi", `INSERT INTO taxi_bookings(id,customer_id,provider_id,vehicle_id,market_id,operating_zone_id,status,accepted_at,pricing_quote_reference)VALUES($1,$2,$3,$4,$5,$6,$7,now()-interval '5 minutes','e2e')`, f.booking, f.customer, f.provider, vehicle, market, zone, status)
	s.exec("taxi", `INSERT INTO taxi_booking_stops(id,booking_id,sequence,type,address,lat,lng)VALUES($1,$2,1,'PICKUP','Pickup E2E',6.5,3.3),($3,$2,2,'DROPOFF','Dropoff E2E',6.6,3.4)`, uuid.NewString(), f.booking, uuid.NewString())
	return f
}

func (s *stack) ingestTaxi(f taxiRide, seq int64, lat, lng float64) reply {
	// Points are spaced in time so the movement stays physically plausible;
	// Taxi flags (and the safe reader ignores) teleporting fixes.
	recorded := time.Now().UTC()
	if seq == 1 {
		recorded = recorded.Add(-20 * time.Second)
	}
	return s.call("POST", taxiURL+"/api/v1/taxi/provider/location/events", s.token(f.provider, "taxi_provider", "taxi"), nil, map[string]any{"points": []map[string]any{{"booking_id": f.booking, "sequence": seq, "lat": lat, "lng": lng, "accuracy": 5, "recorded_at": recorded, "source": "gps", "client_event_id": uuid.NewString()}}})
}

type dispatchTrip struct{ booking, trip, customer string }

func (s *stack) seedDispatchTrip(customer string) dispatchTrip {
	f := dispatchTrip{booking: uuid.NewString(), trip: uuid.NewString(), customer: customer}
	s.dispatchRiderToken() // ensures the rider fixture exists
	s.exec("dispatch", `UPDATE trips SET status='completed',completed_at=now() WHERE provider_id=$1 AND status IN ('assigned','en_route_pickup','arrived_pickup','in_progress')`, dispatchRider)
	s.exec("dispatch", `INSERT INTO dispatch_bookings(id,customer_id,status,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone)VALUES($1,$2,'accepted','Pickup E2E',6.5,3.3,'Dropoff E2E',6.6,3.4,0,'SECRET RECEIVER','+2348000000001')`, f.booking, customer)
	s.exec("dispatch", `INSERT INTO trips(id,booking_id,provider_id,customer_id,status,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone)VALUES($1,$2,$3,$4,'in_progress','Pickup E2E',6.5,3.3,'Dropoff E2E',6.6,3.4,0,'SECRET RECEIVER','+2348000000001')`, f.trip, f.booking, dispatchRider, customer)
	return f
}

var dispatchToken struct {
	once sync.Once
	tok  string
	err  error
}

// dispatchRiderToken uses the real rider OTP login (debug OTP, local only).
func (s *stack) dispatchRiderToken() string {
	s.t.Helper()
	dispatchToken.once.Do(func() {
		// Rider 2 of the Dispatch dev seed: verified, online-capable, one bike.
		for _, q := range []string{
			`INSERT INTO dispatch_rider_identities(id,phone_number,status) VALUES('` + dispatchRider + `','` + dispatchRiderPhone + `','active') ON CONFLICT DO NOTHING`,
			`INSERT INTO providers(id,phone,full_name,state,city,country,operation_type,verification_status,is_active,onboarding_complete) VALUES('` + dispatchRider + `','` + dispatchRiderPhone + `','Bisi Rider','Lagos','Surulere','NG','individual','verified',true,true) ON CONFLICT DO NOTHING`,
			`INSERT INTO provider_availability(provider_id,status,verified_to_go_online) VALUES('` + dispatchRider + `','offline',true) ON CONFLICT DO NOTHING`,
			`INSERT INTO bikes(id,provider_id,bike_type,brand,model,year,color,plate_number,verification_status,is_active,is_primary) VALUES('b2000000-0000-4000-8000-000000000002','` + dispatchRider + `','dispatch_bike','TVS','HLX 125',2024,'Green','LAG-102-DR','verified',true,true) ON CONFLICT DO NOTHING`,
			`INSERT INTO verification_steps(provider_id,step,is_optional,is_auto_confirmed,status,submitted_at,reviewed_at) SELECT '` + dispatchRider + `',s,false,s IN ('guarantor','emergency'),'approved',now(),now() FROM unnest(ARRAY['identity','vehicle','face','guarantor','emergency']) s ON CONFLICT DO NOTHING`,
		} {
			if _, err := s.dbs["dispatch"].Exec(context.Background(), q); err != nil {
				dispatchToken.err = fmt.Errorf("rider fixture: %w", err)
				return
			}
		}
		start := s.call("POST", dispatchURL+"/api/v1/auth/login/start", "", nil, map[string]any{"identifier": dispatchRiderPhone})
		otp, _ := start.data()["debug_otp"].(string)
		if start.Code != 200 || otp == "" {
			dispatchToken.err = fmt.Errorf("rider login start %d %s", start.Code, start.Raw)
			return
		}
		verify := s.call("POST", dispatchURL+"/api/v1/auth/verify", "", nil, map[string]any{"identifier": dispatchRiderPhone, "otp_code": otp, "purpose": "login"})
		tok, _ := verify.data()["access_token"].(string)
		if tok == "" {
			if tokens, ok := verify.data()["tokens"].(map[string]any); ok {
				tok, _ = tokens["access_token"].(string)
			}
		}
		if verify.Code != 200 || tok == "" {
			dispatchToken.err = fmt.Errorf("rider verify %d %s", verify.Code, verify.Raw)
			return
		}
		dispatchToken.tok = tok
	})
	if dispatchToken.err != nil {
		s.t.Fatal(dispatchToken.err)
	}
	return dispatchToken.tok
}

func (s *stack) dispatchOnline() {
	s.t.Helper()
	tok := s.dispatchRiderToken()
	r := s.call("PATCH", dispatchURL+"/api/v1/provider/availability", tok, nil, map[string]any{"status": "online", "lat": 6.5, "lng": 3.3})
	if r.Code != 200 && !strings.Contains(r.Raw, "already") {
		s.t.Fatalf("dispatch go online: %d %s", r.Code, r.Raw)
	}
}

func (s *stack) ingestDispatch(lat, lng float64) reply {
	return s.call("POST", dispatchURL+"/api/v1/provider/location", s.dispatchRiderToken(), nil, map[string]any{"lat": lat, "lng": lng, "accuracy": 5})
}

// ingestDispatchSettled waits out Dispatch's own ping throttle, which a
// previous run (or the load writer) may still be holding.
func (s *stack) ingestDispatchSettled(lat, lng float64) reply {
	r := s.ingestDispatch(lat, lng)
	for i := 0; i < 10 && r.Code == 429; i++ {
		time.Sleep(time.Second)
		r = s.ingestDispatch(lat, lng)
	}
	return r
}

type haulingJob struct{ booking, provider, truck, customer string }

func (s *stack) seedHauling(customer string) haulingJob {
	f := haulingJob{booking: uuid.NewString(), provider: uuid.NewString(), truck: uuid.NewString(), customer: customer}
	s.exec("hauling", `INSERT INTO truck_providers(id,phone,first_name,onboarding_status,status)VALUES($1,$2,'Bayo','complete','active')`, f.provider, "+234"+strings.ReplaceAll(f.provider, "-", "")[:10])
	s.exec("hauling", `INSERT INTO trucks(id,provider_id,truck_type,capacity_kg,plate_number,status,make,model)VALUES($1,$2,'flatbed',10000,$3,'active','MAN','TGX')`, f.truck, f.provider, "HE2E"+f.truck[:5])
	s.exec("hauling", `INSERT INTO haulage_bookings(id,customer_id,provider_id,truck_id,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,cargo_type,cargo_weight_kg,status,accepted_at)VALUES($1,$2,$3,$4,'Pickup E2E',6.5,3.3,'Dropoff E2E',6.6,3.4,'general',10,'picked_up',now())`, f.booking, customer, f.provider, f.truck)
	return f
}

func (s *stack) haulingOnline(f haulingJob) {
	s.t.Helper()
	r := s.call("PUT", haulingURL+"/api/v1/hauling/provider/availability", s.token(f.provider, "truck_provider", "hauling"), nil, map[string]any{"status": "online", "truck_id": f.truck, "lat": 6.5, "lng": 3.3})
	if r.Code != 200 {
		s.t.Fatalf("hauling go online: %d %s", r.Code, r.Raw)
	}
}

func (s *stack) heartbeat(f haulingJob, lat, lng float64) reply {
	return s.call("POST", haulingURL+"/api/v1/hauling/provider/availability/heartbeat", s.token(f.provider, "truck_provider", "hauling"), nil, map[string]any{"lat": lat, "lng": lng, "accuracy": 5, "on_trip": true})
}

// seedCommerceParent mirrors Vendor's canonical parent-order evidence.
func (s *stack) seedCommerceParent(customer, product string) string {
	id := "po_" + uuid.NewString()
	suffix := uuid.NewString()
	cart, checkout, plan, quote := "cart_e2e_"+suffix, "checkout_e2e_"+suffix, "plan_e2e_"+suffix, "quote_e2e_"+suffix
	fp := strings.Repeat(strings.ReplaceAll(uuid.NewString(), "-", ""), 2)
	now := time.Now().UTC()
	exp := now.Add(time.Hour)
	for _, st := range []struct {
		q    string
		args []any
	}{
		{`INSERT INTO commerce_carts(id,customer_id,domain,status,version,expires_at,created_at,updated_at) VALUES($1,$2,'FOOD','CONVERTED',1,$3,$4,$4)`, []any{cart, customer, exp, now}},
		{`INSERT INTO commerce_checkout_sessions(id,customer_id,cart_id,cart_version,domain,status,validation_revision,address_ref,address_snapshotted_at,currency,quoted_items_total_minor,max_authorized_items_total_minor,reservation_ttl_seconds,expires_at,validated_at,created_at,updated_at) VALUES($1,$2,$3,1,'FOOD','VALIDATED',1,'e2e-address',$4,'NGN',0,0,900,$5,$4,$4,$4)`, []any{checkout, customer, cart, now, exp}},
		{`INSERT INTO commerce_fulfilment_plans(id,checkout_session_id,customer_id,checkout_validation_revision,status,mode,plan_revision,currency,delivery_gross_minor,delivery_customer_minor,delivery_vendor_funded_minor,delivery_platform_funded_minor,expires_at,version,planned_at,created_at,updated_at) VALUES($1,$2,$3,1,'PLANNED','SINGLE',1,'NGN',0,0,0,0,$4,1,$5,$5,$5)`, []any{plan, checkout, customer, exp, now}},
		{`INSERT INTO commerce_quotes(id,customer_id,checkout_session_id,checkout_validation_revision,cart_id,cart_version,fulfilment_plan_id,fulfilment_plan_revision,fulfilment_plan_version,quote_version,material_fingerprint,quote_fingerprint,currency,selected_fulfilment_mode,address_snapshot,expires_at,product_subtotal_minor,max_product_subtotal_minor,delivery_subtotal_minor,fee_subtotal_minor,tax_subtotal_minor,discount_total_minor,subsidy_total_minor,final_payable_minor,quoted_total_minor,max_authorized_total_minor,funding_breakdown,price_disclosure_snapshot,created_at) VALUES($1,$2,$3,1,$4,1,$5,1,1,1,$6,$6,'NGN','SINGLE','{}'::jsonb,$7,0,0,0,0,0,0,0,0,0,0,'{}'::jsonb,'{}'::jsonb,$8)`, []any{quote, customer, checkout, cart, plan, fp, exp, now}},
		{`INSERT INTO commerce_parent_orders(id,customer_id,quote_id,quote_version,quote_fingerprint,checkout_session_id,checkout_validation_revision,cart_id,cart_version,fulfilment_plan_id,fulfilment_plan_revision,fulfilment_plan_version,domain,currency,selected_fulfilment_mode,address_snapshot,customer_approved_total_minor,max_authorized_total_minor,product_subtotal_minor,delivery_subtotal_minor,fee_subtotal_minor,tax_subtotal_minor,discount_total_minor,subsidy_total_minor,funding_breakdown,price_disclosure_snapshot,state,vendor_order_count,created_at,updated_at) VALUES($1,$2,$3,1,$4,$5,1,$6,1,$7,1,1,'FOOD','NGN','SINGLE','{}'::jsonb,0,0,0,0,0,0,0,0,'{}'::jsonb,'{}'::jsonb,'CREATED',1,$8,$8)`, []any{id, customer, quote, fp, checkout, cart, plan, now}},
	} {
		s.exec("vendor", strings.ReplaceAll(st.q, "'FOOD'", "'"+product+"'"), st.args...)
	}
	return id
}

func (s *stack) vendorMission(customer, product, service string) (parent, mission string) {
	parent = s.seedCommerceParent(customer, product)
	mission = uuid.NewString()
	s.exec("vendor", `INSERT INTO commerce_order_missions(id,mission_ref,parent_order_id,customer_id,plan_mission_ref,service,vehicle_class,agreed_fee_minor,currency,status,external_ref) VALUES($1,$1,$2,$3,$1,$4,'MOTORCYCLE',0,'NGN','ACCEPTED',$1)`, mission, parent, customer, service)
	return parent, mission
}

// --- scenarios ------------------------------------------------------------

func TestStackRideSelfAndShare(t *testing.T) {
	s := newStack(t)
	f := s.seedTaxiRide("in_progress")
	cust := s.token(f.customer, "customer", "customer")

	// Provider location ingestion through the real Taxi route.
	s.expect(s.ingestTaxi(f, 1, 6.51, 3.31), 200, "taxi ingest")
	n07 := s.expect(s.call("GET", taxiURL+"/api/v1/taxi/customer/bookings/"+f.booking+"/location", cust, nil, nil), 200, "N07")
	if l := location(n07); l["state"] != "FRESH" || l["lat"] != 6.51 {
		t.Fatalf("N07 location %s", n07.Raw)
	}
	s.noLeak(n07.Raw, "N07", f.customer, f.provider)
	s.expect(s.call("GET", taxiURL+"/api/v1/taxi/customer/bookings/"+f.booking+"/location", s.token(uuid.NewString(), "customer", "customer"), nil, nil), 404, "N07 foreign owner")

	// N08 caller boundary on the running service.
	if r := s.signedRead(taxiURL, "/internal/taxi/v1/customer-share/bookings/", f.booking, f.customer, "customer-service"); r.Code != 200 {
		t.Fatalf("N08 %d %s", r.Code, r.Raw)
	}
	if r := s.signedRead(taxiURL, "/internal/taxi/v1/customer-share/bookings/", f.booking, f.customer, "vendor-service"); r.Code != 403 {
		t.Fatalf("N08 wrong caller %d", r.Code)
	}
	if r := s.call("GET", taxiURL+"/internal/taxi/v1/customer-share/bookings/"+f.booking+"?customer_id="+f.customer, cust, nil, nil); r.Code < 400 {
		t.Fatalf("N08 bearer accepted %d", r.Code)
	}

	// Share lifecycle: N01 create, N06 read, live update, N05 rotate, N04 revoke.
	id, version, tok := s.share(cust, "TAXI", f.booking, "RIDE")
	pub := s.expect(s.public(tok), 200, "N06")
	if l := location(pub); l["lat"] != 6.51 {
		t.Fatalf("N06 location %s", pub.Raw)
	}
	s.noLeak(pub.Raw, "N06 ride", f.customer, f.provider, f.booking, tok)
	s.expect(s.ingestTaxi(f, 2, 6.5101, 3.3101), 200, "taxi ingest 2")
	if l := location(s.expect(s.public(tok), 200, "N06 live")); l["lat"] != 6.5101 {
		t.Fatalf("live update not visible: %v", l)
	}
	list := s.expect(s.call("GET", customerURL+"/api/v1/customer/journey-shares?authority=TAXI&source_ref="+f.booking, cust, nil, nil), 200, "N02")
	if !strings.Contains(list.Raw, id) || strings.Contains(list.Raw, tok) {
		t.Fatalf("N02 %s", list.Raw)
	}
	s.expect(s.call("GET", customerURL+"/api/v1/customer/journey-shares/"+id, cust, nil, nil), 200, "N03")
	s.expect(s.call("GET", customerURL+"/api/v1/customer/journey-shares/"+id, s.token(uuid.NewString(), "customer", "customer"), nil, nil), 404, "N03 foreign")

	rot := s.expect(s.call("POST", customerURL+"/api/v1/customer/journey-shares/"+id+"/rotate", cust, map[string]string{"Idempotency-Key": uuid.NewString()}, map[string]any{"expected_version": version}), 200, "N05")
	newTok, _ := rot.data()["token"].(string)
	session, _ := rot.data()["session"].(map[string]any)
	newVersion, _ := session["version"].(float64)
	if newTok == "" || newTok == tok {
		t.Fatalf("rotate token %s", rot.Raw)
	}
	s.expect(s.public(tok), 404, "old token after rotate")
	s.expect(s.public(newTok), 200, "new token after rotate")
	s.expect(s.call("POST", customerURL+"/api/v1/customer/journey-shares/"+id+"/revoke", cust, map[string]string{"Idempotency-Key": uuid.NewString()}, map[string]any{"expected_version": version}), 409, "N04 stale version")
	s.expect(s.call("POST", customerURL+"/api/v1/customer/journey-shares/"+id+"/revoke", cust, map[string]string{"Idempotency-Key": uuid.NewString()}, map[string]any{"expected_version": int64(newVersion)}), 200, "N04")
	s.expect(s.public(newTok), 404, "token after revoke")

	// Completion: a fresh share on the same trip stops exposing location when
	// the authority reaches a terminal state.
	_, _, live := s.share(cust, "TAXI", f.booking, "RIDE")
	s.exec("taxi", `UPDATE taxi_bookings SET status='completed',completed_at=now(),updated_at=now() WHERE id=$1`, f.booking)
	done := s.expect(s.public(live), 200, "N06 after completion")
	if l := location(done); l["available"] == true || done.data()["terminal"] != true {
		t.Fatalf("terminal not suppressed: %s", done.Raw)
	}
	n07done := s.expect(s.call("GET", taxiURL+"/api/v1/taxi/customer/bookings/"+f.booking+"/location", cust, nil, nil), 200, "N07 terminal")
	if l := location(n07done); l["available"] == true {
		t.Fatalf("N07 terminal location exposed: %s", n07done.Raw)
	}
	// Movement projection for the completed ride reaches Customer's Activity.
	s.waitMovement(f.customer, false, 10*time.Second, f.booking)
	s.expect(s.ingestTaxi(f, 3, 6.53, 3.33), 200, "ingest after completion is accepted-or-rejected safely")
	if l := location(s.expect(s.public(live), 200, "N06 post-terminal")); l["available"] == true {
		t.Fatal("post-terminal location exposed")
	}
}

// waitMovement checks the movement projection saw the journey (outbox ->
// signed ingress). Taxi publishes on lifecycle use cases or its periodic
// reconciler; SQL-placed fixtures therefore only appear after a reconcile.
func (s *stack) waitMovement(customer string, required bool, wait time.Duration, refs ...string) bool {
	s.t.Helper()
	deadline := time.Now().Add(wait)
	for {
		var n int
		_ = s.dbs["customer"].QueryRow(context.Background(), `SELECT count(*) FROM customer_movement_history_projection WHERE customer_id::text=$1 AND source_ref=ANY($2)`, customer, refs).Scan(&n)
		if n > 0 {
			return true
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if required {
		s.t.Fatalf("movement projection for %v not observed within %s", refs, wait)
	}
	s.t.Logf("MOVEMENT projection for %v not yet observed within %s (reconciler cadence)", refs, wait)
	return false
}

func TestStackSendShare(t *testing.T) {
	s := newStack(t)
	customer := s.customer()
	f := s.seedDispatchTrip(customer)
	cust := s.token(customer, "customer", "customer")
	s.dispatchOnline()
	s.expect(s.ingestDispatchSettled(6.54, 3.34), 200, "dispatch location")
	n09 := s.signedRead(dispatchURL, "/internal/dispatch/v1/customer-share/trips/", f.trip, customer, "customer-service")
	if n09.Code != 200 || location(n09)["state"] != "FRESH" {
		t.Fatalf("N09 %d %s", n09.Code, n09.Raw)
	}
	if r := s.signedRead(dispatchURL, "/internal/dispatch/v1/customer-share/trips/", f.trip, uuid.NewString(), "customer-service"); r.Code != 404 {
		t.Fatalf("N09 IDOR %d", r.Code)
	}
	if r := s.signedRead(dispatchURL, "/internal/dispatch/v1/customer-share/trips/", f.trip, customer, "vendor-service"); r.Code != 403 && r.Code != 401 {
		t.Fatalf("N09 wrong caller %d", r.Code)
	}
	_, _, tok := s.share(cust, "DISPATCH", f.trip, "SEND")
	pub := s.expect(s.public(tok), 200, "N06 send")
	if location(pub)["lat"] != 6.54 {
		t.Fatalf("send location %s", pub.Raw)
	}
	s.noLeak(pub.Raw, "N06 send", customer, dispatchRider, f.trip, f.booking, "SECRET RECEIVER", "+2348000000001")
	s.expect(s.ingestDispatchSettled(6.55, 3.35), 200, "dispatch location 2")
	if location(s.expect(s.public(tok), 200, "N06 send live"))["lat"] != 6.55 {
		t.Fatal("send live update not visible")
	}
	s.exec("dispatch", `UPDATE trips SET status='completed',completed_at=now(),updated_at=now() WHERE id=$1`, f.trip)
	done := s.expect(s.public(tok), 200, "N06 send completed")
	if location(done)["available"] == true || done.data()["terminal"] != true {
		t.Fatalf("send terminal %s", done.Raw)
	}
	s.waitMovement(customer, true, 60*time.Second, f.booking, f.trip)
}

func TestStackTruckShare(t *testing.T) {
	s := newStack(t)
	customer := s.customer()
	f := s.seedHauling(customer)
	cust := s.token(customer, "customer", "customer")
	s.haulingOnline(f)
	s.expect(s.heartbeat(f, 6.56, 3.36), 200, "hauling heartbeat")
	n10 := s.signedRead(haulingURL, "/internal/hauling/v1/customer-share/bookings/", f.booking, customer, "customer-service")
	if n10.Code != 200 || location(n10)["state"] != "FRESH" {
		t.Fatalf("N10 %d %s", n10.Code, n10.Raw)
	}
	if r := s.signedRead(haulingURL, "/internal/hauling/v1/customer-share/bookings/", f.booking, uuid.NewString(), "customer-service"); r.Code != 404 {
		t.Fatalf("N10 IDOR %d", r.Code)
	}
	_, _, tok := s.share(cust, "HAULING", f.booking, "TRUCK")
	pub := s.expect(s.public(tok), 200, "N06 truck")
	if location(pub)["lat"] != 6.56 {
		t.Fatalf("truck location %s", pub.Raw)
	}
	s.noLeak(pub.Raw, "N06 truck", customer, f.provider, f.booking)
	s.expect(s.heartbeat(f, 6.57, 3.37), 200, "hauling heartbeat 2")
	if location(s.expect(s.public(tok), 200, "N06 truck live"))["lat"] != 6.57 {
		t.Fatal("truck live update not visible")
	}
	s.exec("hauling", `UPDATE haulage_bookings SET status='completed',completed_at=now(),updated_at=now() WHERE id=$1`, f.booking)
	if location(s.expect(s.public(tok), 200, "N06 truck completed"))["available"] == true {
		t.Fatal("truck terminal location exposed")
	}
	s.waitMovement(customer, true, 60*time.Second, f.booking)
}

func TestStackCommerceExecutors(t *testing.T) {
	s := newStack(t)
	for _, product := range []string{"FOOD", "SHOP"} {
		customer := s.customer()
		cust := s.token(customer, "customer", "customer")

		// DISPATCH executor with a mission-bound live point.
		d := s.seedDispatchTrip(customer)
		_, dm := s.vendorMission(customer, product, "DISPATCH")
		s.exec("dispatch", `INSERT INTO commerce_missions(mission_ref,parent_order_ref,vendor_order_refs,customer_id,booking_id,agreed_fee_minor,status,operation_ref,canonical_ref) VALUES($1,'po','{vo}',$2,$3,0,'accepted',$1,$1)`, dm, customer, d.booking)
		s.dispatchOnline()
		s.expect(s.ingestDispatchSettled(6.58, 3.38), 200, "commerce dispatch location")
		s.commerceShare(cust, customer, dm, product, "DISPATCH", true)
		s.exec("dispatch", `UPDATE trips SET status='completed',completed_at=now(),updated_at=now() WHERE id=$1`, d.trip)

		// HAULING executor.
		h := s.seedHauling(customer)
		_, hm := s.vendorMission(customer, product, "HAULING")
		s.exec("hauling", `INSERT INTO commerce_missions(mission_ref,parent_order_ref,vendor_order_refs,customer_id,booking_id,agreed_fee_minor,status,operation_ref) VALUES($1,'po','{vo}',$2,$3,0,'accepted',$1)`, hm, customer, h.booking)
		s.haulingOnline(h)
		s.expect(s.heartbeat(h, 6.59, 3.39), 200, "commerce hauling heartbeat")
		s.commerceShare(cust, customer, hm, product, "HAULING", true)

		// TAXI_GOODS executor: status/provider only; no GPS source exists.
		_, tm := s.vendorMission(customer, product, "TAXI")
		quote, market, zone := uuid.NewString(), uuid.NewString(), uuid.NewString()
		s.exec("taxi", `INSERT INTO markets(id,country,name,code)VALUES($1,'NG','e2e goods',$2)`, market, "G-"+market[:8])
		s.exec("taxi", `INSERT INTO operating_zones(id,market_id,name,code,center_lat,center_lng,match_radius_km,supported_classes)VALUES($1,$2,'g',$3,6.5,3.3,10,'{economy}')`, zone, market, "GZ-"+zone[:8])
		s.exec("taxi", `INSERT INTO taxi_goods_quotes(id,source_service,quote_reference,revision,request_fingerprint,request_payload,serviceable,reason_code,distance_km,estimated_minutes,fee_minor,currency,available_providers,expires_at) VALUES($1,'vendor-service',$2,1,'fp','{}',true,'SERVICEABLE',1,5,1000,'NGN',1,now()+interval '15 minutes')`, quote, "gq-"+quote)
		s.exec("taxi", `INSERT INTO taxi_goods_missions(id,source_service,mission_ref,operation_ref,request_fingerprint,parent_order_ref,vendor_order_refs,customer_ref,quote_id,pickup,dropoff,load,agreed_fee_minor,currency,requested_at) VALUES($1,'vendor-service',$2,$2,'fp','po',ARRAY['vo'],$3,$4,'{}','{}','{}',1000,'NGN',now())`, uuid.NewString(), tm, customer, quote)
		// Passenger telemetry for a ride by the same customer must never leak in.
		ride := s.seedTaxiRide("in_progress")
		s.exec("taxi", `UPDATE taxi_bookings SET customer_id=$1 WHERE id=$2`, customer, ride.booking)
		s.expect(s.ingestTaxi(ride, 1, 6.9, 3.9), 200, "passenger telemetry")
		s.commerceShare(cust, customer, tm, product, "TAXI_GOODS", false)

		// Unknown executor fails closed at Vendor N11.
		_, um := s.vendorMission(customer, product, "DISPATCH")
		s.exec("vendor", `ALTER TABLE commerce_order_missions DISABLE TRIGGER USER`)
		_, uerr := s.dbs["vendor"].Exec(context.Background(), `UPDATE commerce_order_missions SET service='TELEPORT' WHERE id=$1`, um)
		s.exec("vendor", `ALTER TABLE commerce_order_missions ENABLE TRIGGER USER`)
		if uerr == nil {
			if r := s.signedRead(vendorURL, "/internal/vendor/v1/customer-share/missions/", um, customer, "customer-service"); r.Code == 200 {
				t.Fatalf("unknown executor served: %s", r.Raw)
			}
			s.record("unknown_executor_"+product, "persisted and refused")
		} else {
			s.record("unknown_executor_"+product, "unpersistable: "+uerr.Error())
		}
	}
	t.Logf("EVIDENCE %v", s.evidence)
}

func (s *stack) commerceShare(cust, customer, mission, product, executor string, gps bool) {
	s.t.Helper()
	n11 := s.signedRead(vendorURL, "/internal/vendor/v1/customer-share/missions/", mission, customer, "customer-service")
	if n11.Code != 200 {
		s.t.Fatalf("N11 %s %d %s", executor, n11.Code, n11.Raw)
	}
	if n11.data()["authority"] != "VENDOR_COMMERCE" || n11.data()["product_code"] != product {
		s.t.Fatalf("N11 %s projection %s", executor, n11.Raw)
	}
	loc := location(n11)
	if gps && loc["state"] != "FRESH" {
		s.t.Fatalf("N11 %s expected mission-bound location: %s", executor, n11.Raw)
	}
	if !gps && (loc["available"] == true || loc["state"] != "UNAVAILABLE") {
		s.t.Fatalf("N11 %s fabricated location: %s", executor, n11.Raw)
	}
	if r := s.signedRead(vendorURL, "/internal/vendor/v1/customer-share/missions/", mission, uuid.NewString(), "customer-service"); r.Code != 404 {
		s.t.Fatalf("N11 %s IDOR %d", executor, r.Code)
	}
	if r := s.signedRead(vendorURL, "/internal/vendor/v1/customer-share/missions/", mission, customer, "vendor-service"); r.Code != 403 && r.Code != 401 {
		s.t.Fatalf("N11 %s wrong caller %d", executor, r.Code)
	}
	_, _, tok := s.share(cust, "VENDOR_COMMERCE", mission, product)
	pub := s.expect(s.public(tok), 200, "N06 commerce "+executor)
	s.noLeak(pub.Raw, "N06 commerce "+executor, customer, mission)
	if !gps && location(pub)["available"] == true {
		s.t.Fatalf("N06 %s fabricated location", executor)
	}
	s.record("executor_"+executor+"_"+product, map[string]any{"n11": n11.Code, "location_state": loc["state"], "public_status": pub.data()["public_status"]})
}

// TestStackProjectionOutage is orchestrated by sections911-stack-outage.ps1,
// which stops the Taxi process after the share is live and then restarts the
// stack. The public read must fail typed (never stale data, never a leak)
// during the outage and recover with the same capability afterwards.
func TestStackProjectionOutage(t *testing.T) {
	if os.Getenv("SECTIONS911_STACK_OUTAGE") != "1" {
		t.Skip("orchestrated by sections911-stack-outage.ps1")
	}
	s := newStack(t)
	f := s.seedTaxiRide("in_progress")
	cust := s.token(f.customer, "customer", "customer")
	s.expect(s.ingestTaxi(f, 1, 6.51, 3.31), 200, "outage ingest")
	_, _, tok := s.share(cust, "TAXI", f.booking, "RIDE")
	s.expect(s.public(tok), 200, "N06 before outage")
	if err := os.WriteFile(os.Getenv("SECTIONS911_STACK_OUTAGE_READY"), []byte("ready"), 0o600); err != nil {
		t.Fatal(err)
	}
	var outage reply
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		r := s.public(tok)
		if r.Code != 200 {
			outage = r
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	if outage.Code == 0 {
		t.Fatal("Taxi outage was never observed")
	}
	if outage.Code != 503 || outage.errCode() != "service_unavailable" {
		t.Fatalf("outage response not typed 503: %d %s", outage.Code, outage.Raw)
	}
	s.noLeak(outage.Raw, "outage", f.customer, f.provider, f.booking, tok)
	var recovered reply
	deadline = time.Now().Add(180 * time.Second)
	for time.Now().Before(deadline) {
		// The orchestrator restarts the whole stack, so refused connections
		// are expected while Customer itself comes back.
		if res, err := s.http.Get(customerURL + "/api/v1/customer/public/journey-shares/" + tok); err == nil {
			res.Body.Close()
			if res.StatusCode == 200 {
				recovered = reply{Code: 200}
				break
			}
		}
		time.Sleep(time.Second)
	}
	if recovered.Code != 200 {
		t.Fatal("share did not recover after Taxi restart")
	}
	t.Logf("OUTAGE typed=%d/%s recovered=%d", outage.Code, outage.errCode(), recovered.Code)
}

// TestStackSustainedLoad exercises N06-N11 on the running stack while provider
// location writes continue. Duration comes from SECTIONS911_STACK_LOAD
// (default 60s). It reports observations; no production SLO is asserted.
func TestStackSustainedLoad(t *testing.T) {
	s := newStack(t)
	duration := 60 * time.Second
	if v := os.Getenv("SECTIONS911_STACK_LOAD"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			t.Fatal(err)
		}
		duration = d
	}
	concurrency := 16
	ride := s.seedTaxiRide("in_progress")
	rideCust := s.token(ride.customer, "customer", "customer")
	s.expect(s.ingestTaxi(ride, 1, 6.5, 3.3), 200, "load ingest")
	_, _, rideTok := s.share(rideCust, "TAXI", ride.booking, "RIDE")
	sendCust := s.customer()
	send := s.seedDispatchTrip(sendCust)
	s.dispatchOnline()
	s.expect(s.ingestDispatchSettled(6.5, 3.3), 200, "load dispatch")
	truckCust := s.customer()
	truck := s.seedHauling(truckCust)
	s.haulingOnline(truck)
	s.expect(s.heartbeat(truck, 6.5, 3.3), 200, "load heartbeat")
	commerceCust := s.customer()
	_, mission := s.vendorMission(commerceCust, "FOOD", "HAULING")
	s.exec("hauling", `INSERT INTO commerce_missions(mission_ref,parent_order_ref,vendor_order_refs,customer_id,booking_id,agreed_fee_minor,status,operation_ref) VALUES($1,'po','{vo}',$2,$3,0,'accepted',$1)`, mission, commerceCust, s.seedHauling(commerceCust).booking)

	type op struct {
		name string
		do   func(i int) (int, error)
	}
	status := func(r reply) (int, error) { return r.Code, nil }
	ops := []op{
		{"N06", func(i int) (int, error) {
			// Distinct forwarded clients through the trusted local gateway hop;
			// the per-token budget (60/min) still applies and yields typed 429s.
			return status(s.call("GET", customerURL+"/api/v1/customer/public/journey-shares/"+rideTok, "", map[string]string{"X-Forwarded-For": fmt.Sprintf("203.0.113.%d", i%250+1)}, nil))
		}},
		{"N07", func(int) (int, error) {
			return status(s.call("GET", taxiURL+"/api/v1/taxi/customer/bookings/"+ride.booking+"/location", rideCust, nil, nil))
		}},
		{"N08", func(int) (int, error) {
			return status(s.signedRead(taxiURL, "/internal/taxi/v1/customer-share/bookings/", ride.booking, ride.customer, "customer-service"))
		}},
		{"N09", func(int) (int, error) {
			return status(s.signedRead(dispatchURL, "/internal/dispatch/v1/customer-share/trips/", send.trip, sendCust, "customer-service"))
		}},
		{"N10", func(int) (int, error) {
			return status(s.signedRead(haulingURL, "/internal/hauling/v1/customer-share/bookings/", truck.booking, truckCust, "customer-service"))
		}},
		{"N11", func(int) (int, error) {
			return status(s.signedRead(vendorURL, "/internal/vendor/v1/customer-share/missions/", mission, commerceCust, "customer-service"))
		}},
	}
	type stats struct {
		samples               []time.Duration
		ok, typed, unexpected int64
	}
	results := map[string]*stats{}
	for _, o := range ops {
		results[o.name] = &stats{}
	}
	var mu sync.Mutex
	var writes, writeErrors int64
	writeFailures := map[string]int{}
	stop := make(chan struct{})
	var wg sync.WaitGroup
	// Concurrent provider writes while reads run.
	wg.Add(1)
	go func() {
		defer wg.Done()
		seq := int64(2)
		for {
			select {
			case <-stop:
				return
			default:
			}
			seq++
			lat := 6.5 + float64(seq%100)/100000
			for i, r := range []reply{s.ingestTaxi(ride, seq, lat, 3.3), s.ingestDispatch(lat, 3.3), s.heartbeat(truck, lat, 3.3)} {
				atomic.AddInt64(&writes, 1)
				if r.Code != 200 {
					// A 429 is the owning service's own write throttle (Dispatch
					// caps rider pings); it is expected and typed, not a failure.
					if r.Code != 429 || r.errCode() != "rate_limited" {
						atomic.AddInt64(&writeErrors, 1)
					}
					mu.Lock()
					writeFailures[fmt.Sprintf("%s:%d:%s", []string{"taxi", "dispatch", "hauling"}[i], r.Code, r.errCode())]++
					mu.Unlock()
				}
			}
			time.Sleep(200 * time.Millisecond)
		}
	}()
	started := time.Now()
	var counter int64
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for time.Since(started) < duration {
				i := int(atomic.AddInt64(&counter, 1))
				o := ops[i%len(ops)]
				t0 := time.Now()
				code, err := o.do(i)
				elapsed := time.Since(t0)
				mu.Lock()
				st := results[o.name]
				st.samples = append(st.samples, elapsed)
				switch {
				case err == nil && code == 200:
					st.ok++
				case err == nil && code == 429:
					st.typed++
				default:
					st.unexpected++
				}
				mu.Unlock()
			}
		}(w)
	}
	time.Sleep(duration)
	close(stop)
	wg.Wait()
	total := time.Since(started)
	names := make([]string, 0, len(results))
	for n := range results {
		names = append(names, n)
	}
	sort.Strings(names)
	var unexpected int64
	for _, n := range names {
		st := results[n]
		sort.Slice(st.samples, func(i, j int) bool { return st.samples[i] < st.samples[j] })
		c := len(st.samples)
		if c == 0 {
			continue
		}
		pct := func(p int) time.Duration { return st.samples[(c-1)*p/100] }
		t.Logf("LOAD %s requests=%d ok=%d typed_429=%d unexpected=%d p50=%s p95=%s p99=%s max=%s", n, c, st.ok, st.typed, st.unexpected, pct(50), pct(95), pct(99), st.samples[c-1])
		unexpected += st.unexpected
	}
	t.Logf("LOAD summary duration=%s concurrency=%d provider_writes=%d write_errors=%d failures=%v", total.Round(time.Second), concurrency, writes, writeErrors, writeFailures)
	if unexpected > 0 || writeErrors > 0 {
		t.Fatalf("unexpected errors: reads=%d writes=%d", unexpected, writeErrors)
	}
}

var _ = journeyshare.Projection{}
