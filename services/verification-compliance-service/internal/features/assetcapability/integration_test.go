package assetcapability

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/verification-compliance-service/internal/database"
	"cosmicforge/logistics/shared/go/mediaclient"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// These tests run against a real PostgreSQL. Several of the guarantees Section
// 12 rests on are enforced by the database and cannot be observed any other
// way: the append-only trigger on decisions, the partial unique index that
// stops an asset holding two current documents of one type, the CHECK that
// refuses an ELIGIBLE certification with no reference, and the version guard
// that decides which of two concurrent reviewers wins.
//
// They share one database with the other packages' integration tests, and each
// drops the public schema on entry, so the suite must run serialized
// (go test -p 1 ./...).

const (
	taxiCaller     = "driver-taxi-service"
	dispatchCaller = "driver-dispatch-delivery-service"
	haulingCaller  = "driver-hauling-service"
	adminCaller    = AdminService
	fleetCaller    = "fleet-partner-service"
	vendorCaller   = "vendor-service"
)

var secrets = serviceauth.Secrets{
	taxiCaller:     []byte("taxi-secret"),
	dispatchCaller: []byte("dispatch-secret"),
	haulingCaller:  []byte("hauling-secret"),
	adminCaller:    []byte("admin-secret"),
	fleetCaller:    []byte("fleet-secret"),
	vendorCaller:   []byte("vendor-secret"),
}

type harness struct {
	pool    *pgxpool.Pool
	router  http.Handler
	service *Service
	media   *recordingMedia
	nonce   atomic.Int64
}

// recordingMedia stands in for media-file-service. Every asset it holds is
// owned by the authority that would really have uploaded it, so binding is
// exercised for real rather than stubbed into always-yes.
type recordingMedia struct {
	assets map[string]mediaclient.MediaAsset
}

func (m *recordingMedia) GetByID(_ context.Context, id string) (mediaclient.MediaAsset, error) {
	asset, ok := m.assets[id]
	if !ok {
		return mediaclient.MediaAsset{}, fmt.Errorf("media: %s not found", id)
	}
	return asset, nil
}

func (m *recordingMedia) add(id, ownerService, ownerID, purpose, status string) {
	m.assets[id] = mediaclient.MediaAsset{ID: id, OwnerService: ownerService,
		OwnerID: ownerID, Purpose: purpose, Status: status}
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	dsn := os.Getenv("VERIFICATION_COMPLIANCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VERIFICATION_COMPLIANCE_TEST_DATABASE_URL is required")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.Contains(strings.ToLower(parsed.Path), "test") {
		t.Fatal("refusing a database whose name does not identify it as a test database")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err = pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	if err = database.Apply(ctx, pool, "../../../migrations"); err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	media := &recordingMedia{assets: map[string]mediaclient.MediaAsset{}}
	service := NewService(Options{
		Repository: NewRepository(pool),
		Evidence:   NewEvidenceValidator(media),
		Enabled:    true,
	})
	router := gin.New()
	Register(router.Group("/api/v1/verification-compliance"),
		NewHandler(service, pool, []string{fleetCaller, vendorCaller}),
		serviceauth.NewVerifier(secrets, 5*time.Minute))
	return &harness{pool: pool, router: router, service: service, media: media}
}

type response struct {
	Code int
	Body string
}

func (r response) contains(fragment string) bool { return strings.Contains(r.Body, fragment) }

// sameJSON compares two response bodies by content rather than by bytes.
//
// The recorded idempotent response is stored in a jsonb column, which does not
// preserve key order or whitespace. Comparing raw bytes would therefore fail
// for a replay that is in fact identical, and "fix" it by storing the body as
// opaque text -- losing the ability to query recorded responses for no gain.
func sameJSON(t *testing.T, left, right string) bool {
	t.Helper()
	var first, second any
	if err := json.Unmarshal([]byte(left), &first); err != nil {
		t.Fatalf("left body is not JSON: %v", err)
	}
	if err := json.Unmarshal([]byte(right), &second); err != nil {
		t.Fatalf("right body is not JSON: %v", err)
	}
	canonical := func(value any) string {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	return canonical(first) == canonical(second)
}

func (h *harness) call(t *testing.T, caller, method, path string, body any, key string) response {
	t.Helper()
	var raw []byte
	if body != nil {
		var err error
		if raw, err = json.Marshal(body); err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequest(method, "/api/v1/verification-compliance"+path, bytes.NewReader(raw))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(serviceauth.HeaderNonce, fmt.Sprintf("nonce-%d", h.nonce.Add(1)))
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	secret, ok := secrets[caller]
	if !ok {
		t.Fatalf("no secret configured for %s", caller)
	}
	if err := serviceauth.SignRequest(request, caller, secret, raw, time.Now()); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	h.router.ServeHTTP(recorder, request)
	return response{Code: recorder.Code, Body: recorder.Body.String()}
}

// verifyProvider records the provider standing a VERIFIED decision requires.
func (h *harness) verifyProvider(t *testing.T, authority Authority, providerID string) {
	t.Helper()
	got := h.call(t, adminCaller, http.MethodPut,
		fmt.Sprintf("/internal/asset-capability/providers/%s/%s", authority, providerID),
		map[string]any{"status": "VERIFIED", "source_version": 1}, "")
	if got.Code != http.StatusOK {
		t.Fatalf("verify provider = %d %s", got.Code, got.Body)
	}
}

// openCase opens a case and returns its reference.
func (h *harness) openCase(t *testing.T, caller string, body map[string]any, key string) (string, response) {
	t.Helper()
	got := h.call(t, caller, http.MethodPost, "/internal/asset-capability/cases", body, key)
	if got.Code != http.StatusCreated {
		return "", got
	}
	var envelope struct {
		Data struct {
			Case Case `json:"case"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(got.Body), &envelope); err != nil {
		t.Fatal(err)
	}
	return envelope.Data.Case.CaseRef, got
}

func caseBody(authority Authority, providerID, assetID, capability, product string) map[string]any {
	return map[string]any{
		"authority": string(authority), "provider_id": providerID, "asset_id": assetID,
		"capability_code": capability, "product_code": product,
	}
}

// TestEachAuthorityMayOpenCasesOnlyForItsOwnAssets covers mandatory tests 1-6.
//
// The three positive cases and the three negative ones belong together: the
// rule is not "these callers are allowed", it is "each caller is allowed
// exactly one authority". Testing only the positives would pass just as well
// with no check at all.
func TestEachAuthorityMayOpenCasesOnlyForItsOwnAssets(t *testing.T) {
	h := newHarness(t)

	owners := []struct {
		caller    string
		authority Authority
		asset     string
		product   string
	}{
		{taxiCaller, AuthorityTaxi, "veh-1", "TAXI_COMFORT"},
		{dispatchCaller, AuthorityDispatch, "bike-1", "DISPATCH_COLD"},
		{haulingCaller, AuthorityHauling, "truck-1", "HAUL_REEFER"},
	}
	for _, owner := range owners {
		ref, got := h.openCase(t, owner.caller,
			caseBody(owner.authority, "prv-1", owner.asset, "PHYSICAL_FIT", owner.product),
			"open-"+string(owner.authority))
		if got.Code != http.StatusCreated || ref == "" {
			t.Fatalf("%s opening its own case = %d %s", owner.caller, got.Code, got.Body)
		}
	}

	// Test 4: an authority may not speak for another authority's assets.
	if got := h.call(t, taxiCaller, http.MethodPost, "/internal/asset-capability/cases",
		caseBody(AuthorityDispatch, "prv-1", "bike-2", "PHYSICAL_FIT", "DISPATCH_COLD"),
		"cross-authority"); got.Code != http.StatusForbidden {
		t.Fatalf("taxi opening a dispatch case = %d %s, want 403", got.Code, got.Body)
	}

	// Test 5: Fleet consumes certification; it must not be able to manufacture
	// the certification it wants to consume.
	if got := h.call(t, fleetCaller, http.MethodPost, "/internal/asset-capability/cases",
		caseBody(AuthorityTaxi, "prv-1", "veh-2", "PHYSICAL_FIT", "TAXI_COMFORT"),
		"fleet-impersonation"); got.Code != http.StatusForbidden {
		t.Fatalf("fleet opening a taxi case = %d %s, want 403", got.Code, got.Body)
	}

	// Test 6: same for Vendor.
	if got := h.call(t, vendorCaller, http.MethodPost, "/internal/asset-capability/cases",
		caseBody(AuthorityHauling, "prv-1", "truck-2", "PHYSICAL_FIT", "HAUL_REEFER"),
		"vendor-case"); got.Code != http.StatusForbidden {
		t.Fatalf("vendor opening a hauling case = %d %s, want 403", got.Code, got.Body)
	}
}

// TestDecisionAuthorizationIdempotencyAndVersioning covers mandatory tests
// 7, 9, 10 and 11.
func TestDecisionAuthorizationIdempotencyAndVersioning(t *testing.T) {
	h := newHarness(t)
	h.verifyProvider(t, AuthorityTaxi, "prv-1")
	ref, got := h.openCase(t, taxiCaller,
		caseBody(AuthorityTaxi, "prv-1", "veh-1", "PREMIUM_CONDITION", "TAXI_COMFORT"), "open-1")
	if ref == "" {
		t.Fatalf("open = %d %s", got.Code, got.Body)
	}
	path := "/internal/asset-capability/cases/" + ref + "/decisions"

	// Test 7: only Admin decides. The owning authority may ask the question and
	// may not answer it, which is the whole point of an external certifier.
	decision := map[string]any{"decision": "VERIFIED", "reviewer": "reviewer-1",
		"expected_source_version": 1}
	if refused := h.call(t, taxiCaller, http.MethodPost, path, decision, "self-decide"); refused.Code != http.StatusForbidden {
		t.Fatalf("authority self-deciding = %d %s, want 403", refused.Code, refused.Body)
	}

	// Test 9: a reviewer acting on a version that has moved loses.
	stale := map[string]any{"decision": "VERIFIED", "reviewer": "reviewer-1",
		"expected_source_version": 99}
	if conflict := h.call(t, adminCaller, http.MethodPost, path, stale, "stale-version"); conflict.Code != http.StatusConflict {
		t.Fatalf("stale version = %d %s, want 409", conflict.Code, conflict.Body)
	}

	accepted := h.call(t, adminCaller, http.MethodPost, path, decision, "decide-1")
	if accepted.Code != http.StatusOK {
		t.Fatalf("decision = %d %s", accepted.Code, accepted.Body)
	}

	// Test 10: the same key and the same request replays the original answer
	// rather than deciding twice. The comparison is semantic, not byte-level:
	// the recorded response lives in a jsonb column, so PostgreSQL normalises
	// key order on the way back out.
	replay := h.call(t, adminCaller, http.MethodPost, path, decision, "decide-1")
	if replay.Code != http.StatusOK {
		t.Fatalf("replay = %d %s", replay.Code, replay.Body)
	}
	if !sameJSON(t, accepted.Body, replay.Body) {
		t.Fatalf("replay returned a different answer:\n first: %s\nreplay: %s", accepted.Body, replay.Body)
	}

	// Test 11: the same key carrying a different conclusion is a conflict, not
	// a second decision. Otherwise whichever retry arrived first would silently
	// decide which conclusion was recorded.
	different := map[string]any{"decision": "SUSPENDED", "reviewer": "reviewer-1",
		"reason_code": "RECHECK", "expected_source_version": 1}
	if conflict := h.call(t, adminCaller, http.MethodPost, path, different, "decide-1"); conflict.Code != http.StatusConflict {
		t.Fatalf("same key different decision = %d %s, want 409", conflict.Code, conflict.Body)
	}

	// Exactly one decision row exists for the case.
	var decisions int
	if err := h.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM asset_capability_decisions WHERE case_id::text=$1`, ref).Scan(&decisions); err != nil {
		t.Fatal(err)
	}
	if decisions != 1 {
		t.Fatalf("decision rows = %d, want exactly 1", decisions)
	}
}

// TestDecisionsAreAppendOnly is mandatory test 8, enforced by the database.
func TestDecisionsAreAppendOnly(t *testing.T) {
	h := newHarness(t)
	h.verifyProvider(t, AuthorityTaxi, "prv-1")
	ref, _ := h.openCase(t, taxiCaller,
		caseBody(AuthorityTaxi, "prv-1", "veh-1", "PREMIUM_CONDITION", "TAXI_COMFORT"), "open-1")
	if got := h.call(t, adminCaller, http.MethodPost,
		"/internal/asset-capability/cases/"+ref+"/decisions",
		map[string]any{"decision": "VERIFIED", "reviewer": "reviewer-1", "expected_source_version": 1},
		"decide-1"); got.Code != http.StatusOK {
		t.Fatalf("decision = %d %s", got.Code, got.Body)
	}

	ctx := context.Background()
	if _, err := h.pool.Exec(ctx,
		`UPDATE asset_capability_decisions SET reason_code='rewritten'`); err == nil {
		t.Fatal("a decision was updated; the history a suspension rests on must not be editable")
	}
	if _, err := h.pool.Exec(ctx, `DELETE FROM asset_capability_decisions`); err == nil {
		t.Fatal("a decision was deleted; append-only must mean append-only")
	}
}

// TestVerifiedDecisionProducesEligibilityAndAnEvent covers mandatory tests
// 15 and 19, and the producer half of 20.
func TestVerifiedDecisionProducesEligibilityAndAnEvent(t *testing.T) {
	h := newHarness(t)
	h.verifyProvider(t, AuthorityHauling, "prv-1")
	ref, _ := h.openCase(t, haulingCaller,
		caseBody(AuthorityHauling, "prv-1", "truck-1", "REFRIGERATION", "HAUL_REEFER"), "open-1")
	if got := h.call(t, adminCaller, http.MethodPost,
		"/internal/asset-capability/cases/"+ref+"/decisions",
		map[string]any{"decision": "VERIFIED", "reviewer": "inspector-1",
			"policy_version": "p1", "expected_source_version": 1}, "decide-1"); got.Code != http.StatusOK {
		t.Fatalf("decision = %d %s", got.Code, got.Body)
	}

	ctx := context.Background()
	var status, certRef string
	var sourceVersion int64
	if err := h.pool.QueryRow(ctx,
		`SELECT status, certification_ref, source_version FROM service_tier_certifications
		  WHERE authority='HAULING' AND asset_id='truck-1' AND product_code='HAUL_REEFER'`).
		Scan(&status, &certRef, &sourceVersion); err != nil {
		t.Fatal(err)
	}
	if status != "ELIGIBLE" {
		t.Fatalf("certification status = %q, want ELIGIBLE", status)
	}
	if certRef == "" {
		t.Fatal("an eligible certification with no reference gives a consumer nothing to trace")
	}
	// The PENDING row written when the case opened was version 1, so the
	// decision must read as strictly newer or a consumer could not order them.
	if sourceVersion <= 1 {
		t.Fatalf("source_version = %d, want strictly greater than the PENDING row's", sourceVersion)
	}

	// Exactly one deliverable event, addressed to the owning authority.
	var events int
	var target, deliveryStatus string
	if err := h.pool.QueryRow(ctx,
		`SELECT count(*), max(delivery_target), max(delivery_status) FROM verification_outbox
		  WHERE event_type=$1`, EventServiceTierStatusChanged).
		Scan(&events, &target, &deliveryStatus); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Fatalf("service-tier events = %d, want exactly 1", events)
	}
	if target != "HAULING" || deliveryStatus != "PENDING" {
		t.Fatalf("event target/%s status/%s, want HAULING/PENDING", target, deliveryStatus)
	}

	// Test 19's producer half: the outbox's uniqueness on
	// (aggregate, type, version) makes a re-published identical event a no-op
	// rather than a second delivery.
	var certificationID string
	if err := h.pool.QueryRow(ctx,
		`SELECT id::text FROM service_tier_certifications WHERE asset_id='truck-1'`).
		Scan(&certificationID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.pool.Exec(ctx,
		`INSERT INTO verification_outbox(aggregate_id,event_type,event_version,payload)
		 VALUES($1::uuid,$2,$3,'{}') ON CONFLICT DO NOTHING`,
		certificationID, EventServiceTierStatusChanged, sourceVersion); err != nil {
		t.Fatal(err)
	}
	if err := h.pool.QueryRow(ctx,
		`SELECT count(*) FROM verification_outbox WHERE event_type=$1`,
		EventServiceTierStatusChanged).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Fatalf("duplicate event was stored; events = %d", events)
	}
}

// TestRejectedDecisionDoesNotMakeAnAssetEligible is mandatory test 16.
func TestRejectedDecisionDoesNotMakeAnAssetEligible(t *testing.T) {
	h := newHarness(t)
	ref, _ := h.openCase(t, dispatchCaller,
		caseBody(AuthorityDispatch, "prv-1", "bike-1", "COLD_BOX", "DISPATCH_COLD"), "open-1")
	if got := h.call(t, adminCaller, http.MethodPost,
		"/internal/asset-capability/cases/"+ref+"/decisions",
		map[string]any{"decision": "REJECTED", "reviewer": "inspector-1",
			"reason_code": "BOX_NOT_SEALED", "expected_source_version": 1},
		"decide-1"); got.Code != http.StatusOK {
		t.Fatalf("decision = %d %s", got.Code, got.Body)
	}
	var status, certRef string
	if err := h.pool.QueryRow(context.Background(),
		`SELECT status, certification_ref FROM service_tier_certifications WHERE asset_id='bike-1'`).
		Scan(&status, &certRef); err != nil {
		t.Fatal(err)
	}
	if status != "REJECTED" {
		t.Fatalf("status = %q, want REJECTED", status)
	}
	if certRef != "" {
		t.Fatal("a rejected asset must hold no certification reference")
	}
}

// TestVerifiedDecisionRequiresAVerifiedProvider is the provider-verification
// integration: a certified vehicle operated by an unverified provider is a
// vehicle nobody should be dispatching, and this is the last place that can
// still refuse it.
func TestVerifiedDecisionRequiresAVerifiedProvider(t *testing.T) {
	h := newHarness(t)
	ref, _ := h.openCase(t, taxiCaller,
		caseBody(AuthorityTaxi, "unverified-prv", "veh-1", "PREMIUM_CONDITION", "TAXI_COMFORT"), "open-1")
	path := "/internal/asset-capability/cases/" + ref + "/decisions"
	decision := map[string]any{"decision": "VERIFIED", "reviewer": "reviewer-1",
		"expected_source_version": 1}

	refused := h.call(t, adminCaller, http.MethodPost, path, decision, "decide-1")
	if refused.Code != http.StatusUnprocessableEntity || !refused.contains(ReasonProviderNotVerified) {
		t.Fatalf("unverified provider = %d %s, want 422 %s", refused.Code, refused.Body, ReasonProviderNotVerified)
	}

	// The same decision succeeds once the provider is verified, proving the
	// refusal was about the provider and not about the case.
	h.verifyProvider(t, AuthorityTaxi, "unverified-prv")
	if accepted := h.call(t, adminCaller, http.MethodPost, path, decision, "decide-2"); accepted.Code != http.StatusOK {
		t.Fatalf("verified provider = %d %s", accepted.Code, accepted.Body)
	}
}

// TestSuspensionIsExplicitAndProductScoped covers mandatory tests 17 and 21.
//
// Test 21 is the one that is easy to get wrong by being helpful: suspending a
// vehicle's Comfort certification must not also strip its Standard one. They
// are separate admissions resting on separate evidence, and collapsing them
// would take a car off the road for failing a premium check it never needed to
// pass for the work it was doing.
func TestSuspensionIsExplicitAndProductScoped(t *testing.T) {
	h := newHarness(t)
	h.verifyProvider(t, AuthorityTaxi, "prv-1")

	refs := map[string]string{}
	for _, product := range []string{"TAXI_STANDARD", "TAXI_COMFORT"} {
		ref, got := h.openCase(t, taxiCaller,
			caseBody(AuthorityTaxi, "prv-1", "veh-1", "PREMIUM_CONDITION", product), "open-"+product)
		if ref == "" {
			t.Fatalf("open %s = %d %s", product, got.Code, got.Body)
		}
		refs[product] = ref
		if decided := h.call(t, adminCaller, http.MethodPost,
			"/internal/asset-capability/cases/"+ref+"/decisions",
			map[string]any{"decision": "VERIFIED", "reviewer": "reviewer-1",
				"expected_source_version": 1}, "verify-"+product); decided.Code != http.StatusOK {
			t.Fatalf("verify %s = %d %s", product, decided.Code, decided.Body)
		}
	}

	// Suspend Comfort only.
	if got := h.call(t, adminCaller, http.MethodPost,
		"/internal/asset-capability/cases/"+refs["TAXI_COMFORT"]+"/decisions",
		map[string]any{"decision": "SUSPENDED", "reviewer": "reviewer-1",
			"reason_code": "UPHOLSTERY_DAMAGE", "expected_source_version": 2},
		"suspend-comfort"); got.Code != http.StatusOK {
		t.Fatalf("suspend = %d %s", got.Code, got.Body)
	}

	ctx := context.Background()
	statuses := map[string]string{}
	rows, err := h.pool.Query(ctx,
		`SELECT product_code, status FROM service_tier_certifications WHERE asset_id='veh-1'`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var product, status string
		if err = rows.Scan(&product, &status); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		statuses[product] = status
	}
	rows.Close()
	if statuses["TAXI_COMFORT"] != "SUSPENDED" {
		t.Fatalf("Comfort = %q, want SUSPENDED", statuses["TAXI_COMFORT"])
	}
	if statuses["TAXI_STANDARD"] != "ELIGIBLE" {
		t.Fatalf("Standard = %q, want ELIGIBLE: suspending one product must not withdraw another",
			statuses["TAXI_STANDARD"])
	}

	// Test 17: the suspension is published, not merely stored.
	var suspensionEvents int
	if err = h.pool.QueryRow(ctx,
		`SELECT count(*) FROM verification_outbox
		  WHERE event_type=$1 AND payload->>'status'='SUSPENDED' AND delivery_target='TAXI'`,
		EventServiceTierStatusChanged).Scan(&suspensionEvents); err != nil {
		t.Fatal(err)
	}
	if suspensionEvents != 1 {
		t.Fatalf("suspension events = %d, want 1: a suspension nobody is told about is not a suspension",
			suspensionEvents)
	}
}

// TestExpirySweepPublishesAnExplicitExpiry is mandatory test 18.
func TestExpirySweepPublishesAnExplicitExpiry(t *testing.T) {
	h := newHarness(t)
	h.verifyProvider(t, AuthorityTaxi, "prv-1")
	ref, _ := h.openCase(t, taxiCaller,
		caseBody(AuthorityTaxi, "prv-1", "veh-1", "PREMIUM_CONDITION", "TAXI_COMFORT"), "open-1")
	past := time.Now().UTC().Add(-time.Hour)
	if got := h.call(t, adminCaller, http.MethodPost,
		"/internal/asset-capability/cases/"+ref+"/decisions",
		map[string]any{"decision": "VERIFIED", "reviewer": "reviewer-1",
			"effective_at": past.Add(-time.Hour), "expires_at": past,
			"expected_source_version": 1}, "decide-1"); got.Code != http.StatusOK {
		t.Fatalf("decision = %d %s", got.Code, got.Body)
	}

	result, err := h.service.RunExpirySweep(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Certifications != 1 {
		t.Fatalf("sweep expired %d certifications, want 1", result.Certifications)
	}

	ctx := context.Background()
	var status string
	if err = h.pool.QueryRow(ctx,
		`SELECT status FROM service_tier_certifications WHERE asset_id='veh-1'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "EXPIRED" {
		t.Fatalf("status = %q, want EXPIRED", status)
	}
	var expiryEvents int
	if err = h.pool.QueryRow(ctx,
		`SELECT count(*) FROM verification_outbox
		  WHERE event_type=$1 AND payload->>'status'='EXPIRED' AND delivery_target='TAXI'`,
		EventServiceTierStatusChanged).Scan(&expiryEvents); err != nil {
		t.Fatal(err)
	}
	if expiryEvents != 1 {
		t.Fatalf("expiry events = %d, want 1: eligibility must not lapse silently", expiryEvents)
	}

	// A second sweep finds nothing: expiry is not republished every tick.
	repeat, err := h.service.RunExpirySweep(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if repeat.Certifications != 0 {
		t.Fatalf("second sweep expired %d certifications, want 0", repeat.Certifications)
	}
}

// TestReinspectionReturnsALapsedAssetToPendingThenEligible is mandatory test 22.
//
// It also pins the inverse rule, which matters just as much: opening a case
// against a certification that is still live must NOT knock it back to PENDING.
// A vehicle does not stop being a Comfort vehicle because somebody started next
// year's renewal early.
func TestReinspectionReturnsALapsedAssetToPendingThenEligible(t *testing.T) {
	h := newHarness(t)
	h.verifyProvider(t, AuthorityTaxi, "prv-1")
	body := caseBody(AuthorityTaxi, "prv-1", "veh-1", "PREMIUM_CONDITION", "TAXI_COMFORT")

	first, _ := h.openCase(t, taxiCaller, body, "open-1")
	if got := h.call(t, adminCaller, http.MethodPost,
		"/internal/asset-capability/cases/"+first+"/decisions",
		map[string]any{"decision": "SUSPENDED", "reviewer": "reviewer-1",
			"reason_code": "TYRE_WEAR", "expected_source_version": 1}, "suspend"); got.Code != http.StatusOK {
		t.Fatalf("suspend = %d %s", got.Code, got.Body)
	}

	ctx := context.Background()
	// Re-inspection: a new case moves the lapsed certification back to PENDING.
	second, opened := h.openCase(t, taxiCaller, body, "open-2")
	if second == "" {
		t.Fatalf("reinspection case = %d %s", opened.Code, opened.Body)
	}
	var status string
	if err := h.pool.QueryRow(ctx,
		`SELECT status FROM service_tier_certifications WHERE asset_id='veh-1'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "PENDING" {
		t.Fatalf("after reinspection status = %q, want PENDING", status)
	}

	if got := h.call(t, adminCaller, http.MethodPost,
		"/internal/asset-capability/cases/"+second+"/decisions",
		map[string]any{"decision": "VERIFIED", "reviewer": "reviewer-1",
			"expected_source_version": 1}, "reverify"); got.Code != http.StatusOK {
		t.Fatalf("reverify = %d %s", got.Code, got.Body)
	}
	if err := h.pool.QueryRow(ctx,
		`SELECT status FROM service_tier_certifications WHERE asset_id='veh-1'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "ELIGIBLE" {
		t.Fatalf("after reverification status = %q, want ELIGIBLE", status)
	}

	// The inverse: a further case against a live certification leaves it alone.
	if third, got := h.openCase(t, taxiCaller, body, "open-3"); third == "" {
		t.Fatalf("renewal case = %d %s", got.Code, got.Body)
	}
	if err := h.pool.QueryRow(ctx,
		`SELECT status FROM service_tier_certifications WHERE asset_id='veh-1'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "ELIGIBLE" {
		t.Fatalf("a renewal case knocked a live certification to %q; eligibility must survive until decided", status)
	}
}

// TestDocumentsAreEvidenceAndNeverAdmission is mandatory test 23 end to end.
//
// Three assertions, in order of how the mistake would actually be made:
// registering perfectly valid documents produces no certification at all; an
// expired document may not be leaned on; and only the reviewed decision path
// ever writes ELIGIBLE.
func TestDocumentsAreEvidenceAndNeverAdmission(t *testing.T) {
	h := newHarness(t)
	h.verifyProvider(t, AuthorityTaxi, "prv-1")
	h.media.add("media-reg", taxiCaller, "veh-1", "document_file", "active")
	h.media.add("media-ins", taxiCaller, "veh-1", "document_file", "active")

	future := time.Now().UTC().Add(365 * 24 * time.Hour)
	past := time.Now().UTC().Add(-24 * time.Hour)

	valid := h.call(t, adminCaller, http.MethodPost, "/internal/asset-capability/vehicle-documents",
		map[string]any{"authority": "TAXI", "provider_id": "prv-1", "asset_id": "veh-1",
			"document_type": "REGISTRATION", "media_asset_id": "media-reg",
			"status": "VALID", "expires_at": future}, "doc-1")
	if valid.Code != http.StatusCreated {
		t.Fatalf("register document = %d %s", valid.Code, valid.Body)
	}
	expiredDoc := h.call(t, adminCaller, http.MethodPost, "/internal/asset-capability/vehicle-documents",
		map[string]any{"authority": "TAXI", "provider_id": "prv-1", "asset_id": "veh-1",
			"document_type": "INSURANCE", "media_asset_id": "media-ins",
			"status": "VALID", "expires_at": past}, "doc-2")
	if expiredDoc.Code != http.StatusCreated {
		t.Fatalf("register lapsed document = %d %s", expiredDoc.Code, expiredDoc.Body)
	}

	// Holding valid paperwork certifies nothing.
	ctx := context.Background()
	var certifications int
	if err := h.pool.QueryRow(ctx,
		`SELECT count(*) FROM service_tier_certifications WHERE asset_id='veh-1'`).Scan(&certifications); err != nil {
		t.Fatal(err)
	}
	if certifications != 0 {
		t.Fatalf("valid documents produced %d certifications; a document is evidence, never admission",
			certifications)
	}

	var documentRefs []string
	rows, err := h.pool.Query(ctx,
		`SELECT id::text FROM vehicle_documents WHERE asset_id='veh-1' ORDER BY document_type`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		documentRefs = append(documentRefs, id)
	}
	rows.Close()
	if len(documentRefs) != 2 {
		t.Fatalf("documents = %d, want 2", len(documentRefs))
	}
	// Ordered by document_type: INSURANCE (lapsed) then REGISTRATION (valid).
	lapsedRef, validRef := documentRefs[0], documentRefs[1]

	ref, _ := h.openCase(t, taxiCaller,
		caseBody(AuthorityTaxi, "prv-1", "veh-1", "PREMIUM_CONDITION", "TAXI_COMFORT"), "open-1")
	path := "/internal/asset-capability/cases/" + ref + "/decisions"

	// A reviewer may not lean on a lapsed document.
	refused := h.call(t, adminCaller, http.MethodPost, path,
		map[string]any{"decision": "VERIFIED", "reviewer": "reviewer-1",
			"document_refs": []string{lapsedRef}, "expected_source_version": 1}, "cite-lapsed")
	if refused.Code != http.StatusUnprocessableEntity || !refused.contains(ReasonDocumentNotValid) {
		t.Fatalf("citing a lapsed document = %d %s, want 422 %s", refused.Code, refused.Body, ReasonDocumentNotValid)
	}

	// A document belonging to another asset is refused too.
	h.media.add("media-other", taxiCaller, "veh-9", "document_file", "active")
	other := h.call(t, adminCaller, http.MethodPost, "/internal/asset-capability/vehicle-documents",
		map[string]any{"authority": "TAXI", "provider_id": "prv-9", "asset_id": "veh-9",
			"document_type": "REGISTRATION", "media_asset_id": "media-other",
			"status": "VALID", "expires_at": future}, "doc-3")
	if other.Code != http.StatusCreated {
		t.Fatalf("register other asset document = %d %s", other.Code, other.Body)
	}
	var otherRef string
	if err = h.pool.QueryRow(ctx,
		`SELECT id::text FROM vehicle_documents WHERE asset_id='veh-9'`).Scan(&otherRef); err != nil {
		t.Fatal(err)
	}
	wrongAsset := h.call(t, adminCaller, http.MethodPost, path,
		map[string]any{"decision": "VERIFIED", "reviewer": "reviewer-1",
			"document_refs": []string{otherRef}, "expected_source_version": 1}, "cite-other")
	if wrongAsset.Code != http.StatusUnprocessableEntity || !wrongAsset.contains(ReasonDocumentWrongAsset) {
		t.Fatalf("citing another asset's document = %d %s, want 422 %s",
			wrongAsset.Code, wrongAsset.Body, ReasonDocumentWrongAsset)
	}

	// Only the reviewed decision produces admission.
	accepted := h.call(t, adminCaller, http.MethodPost, path,
		map[string]any{"decision": "VERIFIED", "reviewer": "reviewer-1",
			"document_refs": []string{validRef}, "expected_source_version": 1}, "cite-valid")
	if accepted.Code != http.StatusOK {
		t.Fatalf("decision citing a valid document = %d %s", accepted.Code, accepted.Body)
	}
	var status string
	if err = h.pool.QueryRow(ctx,
		`SELECT status FROM service_tier_certifications WHERE asset_id='veh-1'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "ELIGIBLE" {
		t.Fatalf("status = %q, want ELIGIBLE", status)
	}
}

// TestCertificationReadIsScopedToTheOwningAuthority covers Section 5.11's
// consumer rule and the read-back in 5.9.
func TestCertificationReadIsScopedToTheOwningAuthority(t *testing.T) {
	h := newHarness(t)
	h.verifyProvider(t, AuthorityDispatch, "prv-1")
	ref, _ := h.openCase(t, dispatchCaller,
		caseBody(AuthorityDispatch, "prv-1", "bike-1", "COLD_BOX", "DISPATCH_COLD"), "open-1")
	if got := h.call(t, adminCaller, http.MethodPost,
		"/internal/asset-capability/cases/"+ref+"/decisions",
		map[string]any{"decision": "VERIFIED", "reviewer": "reviewer-1",
			"expected_source_version": 1}, "decide-1"); got.Code != http.StatusOK {
		t.Fatalf("decision = %d %s", got.Code, got.Body)
	}

	owner := h.call(t, dispatchCaller, http.MethodGet,
		"/internal/asset-capability/assets/DISPATCH/bike-1/certifications", nil, "")
	if owner.Code != http.StatusOK || !owner.contains(`"status":"ELIGIBLE"`) {
		t.Fatalf("owning authority read = %d %s", owner.Code, owner.Body)
	}

	// Taxi may not read a Dispatch asset's certifications.
	stranger := h.call(t, taxiCaller, http.MethodGet,
		"/internal/asset-capability/assets/DISPATCH/bike-1/certifications", nil, "")
	if stranger.Code != http.StatusForbidden {
		t.Fatalf("cross-authority read = %d %s, want 403", stranger.Code, stranger.Body)
	}

	// Read-back of the case returns its decision history.
	detail := h.call(t, dispatchCaller, http.MethodGet,
		"/internal/asset-capability/cases/"+ref, nil, "")
	if detail.Code != http.StatusOK || !detail.contains(`"decision":"VERIFIED"`) {
		t.Fatalf("case read-back = %d %s", detail.Code, detail.Body)
	}
}

// TestNonceReplayIsRefused proves the replay window is actually closed.
func TestNonceReplayIsRefused(t *testing.T) {
	h := newHarness(t)
	raw, err := json.Marshal(caseBody(AuthorityTaxi, "prv-1", "veh-1", "PREMIUM_CONDITION", "TAXI_COMFORT"))
	if err != nil {
		t.Fatal(err)
	}
	send := func() int {
		request := httptest.NewRequest(http.MethodPost,
			"/api/v1/verification-compliance/internal/asset-capability/cases", bytes.NewReader(raw))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set(serviceauth.HeaderNonce, "fixed-nonce")
		request.Header.Set("Idempotency-Key", "replay-key")
		if err := serviceauth.SignRequest(request, taxiCaller, secrets[taxiCaller], raw, time.Now()); err != nil {
			t.Fatal(err)
		}
		recorder := httptest.NewRecorder()
		h.router.ServeHTTP(recorder, request)
		return recorder.Code
	}
	if first := send(); first != http.StatusCreated {
		t.Fatalf("first send = %d", first)
	}
	if second := send(); second != http.StatusUnauthorized {
		t.Fatalf("replayed nonce = %d, want 401", second)
	}
}

// TestOneLiveDocumentPerTypePerAsset pins the partial unique index.
func TestOneLiveDocumentPerTypePerAsset(t *testing.T) {
	h := newHarness(t)
	h.media.add("media-a", taxiCaller, "veh-1", "document_file", "active")
	h.media.add("media-b", taxiCaller, "veh-1", "document_file", "active")
	body := func(media, key string) (map[string]any, string) {
		return map[string]any{"authority": "TAXI", "provider_id": "prv-1", "asset_id": "veh-1",
			"document_type": "REGISTRATION", "media_asset_id": media, "status": "VALID"}, key
	}
	first, key := body("media-a", "doc-a")
	if got := h.call(t, adminCaller, http.MethodPost,
		"/internal/asset-capability/vehicle-documents", first, key); got.Code != http.StatusCreated {
		t.Fatalf("first document = %d %s", got.Code, got.Body)
	}
	second, key := body("media-b", "doc-b")
	got := h.call(t, adminCaller, http.MethodPost,
		"/internal/asset-capability/vehicle-documents", second, key)
	if got.Code != http.StatusBadRequest {
		t.Fatalf("second current registration = %d %s, want a refusal: "+
			"two current registrations make \"is it registered\" depend on row order",
			got.Code, got.Body)
	}
}
