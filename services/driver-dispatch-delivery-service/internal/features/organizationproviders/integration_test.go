package organizationproviders_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"

	authhttp "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/http"
	authusecases "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/usecases"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/authority"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/organizationproviders"
)

// The organization-managed rider contract against real PostgreSQL. Run with
// DISPATCH_TEST_DATABASE_URL pointing at a database that has the rider-auth
// files (001-003) and every *.up.sql migration applied.

const (
	fleetCaller = "fleet-partner-service"
	fleetSecret = "organization-providers-test-secret"
	tokenSecret = "organization-providers-token-secret"
	base        = "/api/v1/internal/organization-providers"
)

type fixture struct {
	pool   *pgxpool.Pool
	server *httptest.Server
	tokens *authusecases.TokenUsecase
}

func newFixture(t *testing.T, enabled bool) *fixture {
	t.Helper()
	dsn := os.Getenv("DISPATCH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("DISPATCH_TEST_DATABASE_URL not set; skipping dispatch PostgreSQL tests")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(context.Background(), `TRUNCATE dispatch_organization_provider_operations,
		dispatch_organization_provider_link_history, dispatch_organization_provider_links,
		dispatch_service_request_nonces, dispatch_rider_identities, providers CASCADE`); err != nil {
		t.Fatalf("clean: %v", err)
	}
	tokens := authusecases.NewTokenUsecase([]byte(tokenSecret), time.Hour, time.Hour)
	handler := organizationproviders.NewHandler(organizationproviders.NewService(
		organizationproviders.NewRepository(pool), organizationproviders.Flags{Enabled: enabled}))

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(httpx.RequestID(), httpx.ErrorHandler())
	organizationproviders.RegisterInternal(engine.Group(base), handler,
		authority.ServiceBoundary(serviceauth.NewVerifier(serviceauth.Secrets{fleetCaller: []byte(fleetSecret)}, time.Minute),
			authority.NewPostgresNonceClaimer(pool), []string{fleetCaller})...)
	organizationproviders.RegisterProvider(engine.Group("/api/v1/provider/organization-associations"), handler,
		authhttp.DispatchRiderID, authhttp.DispatchRiderAuthRequired(tokens))
	server := httptest.NewServer(engine)
	t.Cleanup(server.Close)
	return &fixture{pool: pool, server: server, tokens: tokens}
}

type response struct {
	status int
	data   map[string]any
	code   string
}

func (f *fixture) do(t *testing.T, req *http.Request) response {
	t.Helper()
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	var envelope struct {
		Data  map[string]any `json:"data"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &envelope)
	return response{status: res.StatusCode, data: envelope.Data, code: envelope.Error.Code}
}

// fleet sends a signed private call the way fleet-partner-service does.
func (f *fixture) fleet(t *testing.T, method, path, org, key string, body any) response {
	t.Helper()
	var payload []byte
	if body != nil {
		payload, _ = json.Marshal(body)
	}
	req, _ := http.NewRequest(method, f.server.URL+base+path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(authority.HeaderServiceNonce, uuid.NewString())
	if org != "" {
		req.Header.Set("X-Organization-ID", org)
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	req.Header.Set("X-Correlation-ID", "cor-test")
	if err := serviceauth.SignRequest(req, fleetCaller, []byte(fleetSecret), payload, time.Now()); err != nil {
		t.Fatal(err)
	}
	return f.do(t, req)
}

func (f *fixture) rider(t *testing.T, method, path, riderID string, body any) response {
	t.Helper()
	var payload []byte
	if body != nil {
		payload, _ = json.Marshal(body)
	}
	req, _ := http.NewRequest(method, f.server.URL+"/api/v1/provider/organization-associations"+path, bytes.NewReader(payload))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	token, _, err := f.tokens.GenerateAccessToken(riderID, "+2348000000000", uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return f.do(t, req)
}

// seedRider creates an independent rider with a sign-in identity.
func (f *fixture) seedRider(t *testing.T, phone, email, verification string) string {
	t.Helper()
	id := uuid.NewString()
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `INSERT INTO dispatch_rider_identities (id, phone_number, email, status) VALUES ($1,$2,$3,'active')`, id, phone, email); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO providers (id, phone, email, full_name, verification_status, is_active, onboarding_complete)
		VALUES ($1,$2,$3,'Existing Rider',$4,true,true)`, id, phone, email, verification); err != nil {
		t.Fatal(err)
	}
	return id
}

func sponsorBody(org, email, phone string) map[string]any {
	return map[string]any{"organizationId": org, "legalName": "Ada Okafor", "emailNormalized": email,
		"phoneE164": phone, "serviceCategory": "DISPATCH", "onboardingMode": "ORGANIZATION_SPONSORED",
		"providerMode": "ORGANIZATION_MANAGED"}
}

func TestPrivateSurfaceRequiresSignedAllowlistedCaller(t *testing.T) {
	f := newFixture(t, true)
	req, _ := http.NewRequest(http.MethodPost, f.server.URL+base+"/providers/resolve", bytes.NewReader([]byte(`{}`)))
	if got := f.do(t, req); got.status != http.StatusUnauthorized {
		t.Fatalf("unsigned call: got %d", got.status)
	}
	payload := []byte(`{"emailNormalized":"a@b.co","phoneE164":"+2348011112222"}`)
	req, _ = http.NewRequest(http.MethodPost, f.server.URL+base+"/providers/resolve", bytes.NewReader(payload))
	req.Header.Set(authority.HeaderServiceNonce, uuid.NewString())
	_ = serviceauth.SignRequest(req, "vendor-service", []byte(fleetSecret), payload, time.Now())
	if got := f.do(t, req); got.status != http.StatusUnauthorized && got.status != http.StatusForbidden {
		t.Fatalf("unknown caller: got %d", got.status)
	}
}

func TestSponsoredRiderLifecycleThroughProviderAcceptance(t *testing.T) {
	f := newFixture(t, true)
	org := "org_" + uuid.NewString()[:8]
	email, phone := "ada.sponsored@example.com", "+2348031110001"

	got := f.fleet(t, http.MethodPost, "/providers/resolve", "", "", map[string]any{"emailNormalized": email, "phoneE164": phone})
	if got.status != 200 || got.data["outcome"] != "NOT_FOUND" || got.data["serviceCategory"] != "DISPATCH" {
		t.Fatalf("resolve before sponsor: %+v", got)
	}

	key := "sponsor-" + uuid.NewString()
	created := f.fleet(t, http.MethodPost, "/providers/sponsored", org, key, sponsorBody(org, email, phone))
	if created.status != http.StatusCreated || created.data["accountState"] != "PENDING_VERIFICATION" {
		t.Fatalf("sponsor: %+v", created)
	}
	providerID := created.data["providerId"].(string)

	replayed := f.fleet(t, http.MethodPost, "/providers/sponsored", org, key, sponsorBody(org, email, phone))
	if replayed.data["providerId"] != providerID {
		t.Fatalf("replay returned a different provider: %+v", replayed)
	}
	if mismatch := f.fleet(t, http.MethodPost, "/providers/sponsored", org, key, sponsorBody(org, email, "+2348031110009")); mismatch.status != http.StatusConflict || mismatch.code != "IDEMPOTENCY_CONFLICT" {
		t.Fatalf("key reuse with a different body: %+v", mismatch)
	}

	// The sponsored rider can sign in by email OTP (identity carries the email)
	// and skips the independent onboarding screens, but is NOT verified.
	var identityEmail, mode, managedBy, verification string
	var onboarded bool
	if err := f.pool.QueryRow(context.Background(), `SELECT i.email, p.onboarding_mode, p.managed_by_organization_id,
		p.verification_status, p.onboarding_complete FROM dispatch_rider_identities i JOIN providers p ON p.id=i.id WHERE i.id=$1`, providerID).
		Scan(&identityEmail, &mode, &managedBy, &verification, &onboarded); err != nil {
		t.Fatal(err)
	}
	if identityEmail != email || mode != "ORGANIZATION_SPONSORED" || managedBy != org || verification != "unverified" || !onboarded {
		t.Fatalf("sponsored identity: email=%s mode=%s org=%s verification=%s onboarded=%v", identityEmail, mode, managedBy, verification, onboarded)
	}

	matched := f.fleet(t, http.MethodPost, "/providers/resolve", "", "", map[string]any{"emailNormalized": email, "phoneE164": phone})
	if matched.data["outcome"] != "MATCHED" || matched.data["providerId"] != providerID || matched.data["managedByOrgId"] != org {
		t.Fatalf("resolve after sponsor: %+v", matched)
	}
	if read := f.fleet(t, http.MethodGet, "/providers/"+providerID, "", "", nil); read.data["accountState"] != "PENDING_VERIFICATION" {
		t.Fatalf("read provider: %+v", read)
	}

	membership := "pmb_" + uuid.NewString()[:8]
	linkKey := "assoc:" + membership
	linkBody := map[string]any{"organizationId": org, "providerId": providerID, "membershipId": membership}
	if wrongOrg := f.fleet(t, http.MethodPost, "/provider-links", "org_other", linkKey, linkBody); wrongOrg.status != http.StatusNotFound {
		t.Fatalf("body/header organization mismatch: %+v", wrongOrg)
	}
	link := f.fleet(t, http.MethodPost, "/provider-links", org, linkKey, linkBody)
	if link.status != http.StatusCreated || link.data["status"] != "REQUESTED" {
		t.Fatalf("create link: %+v", link)
	}
	linkID := link.data["linkId"].(string)
	if again := f.fleet(t, http.MethodPost, "/provider-links", org, linkKey, linkBody); again.status != http.StatusOK || again.data["linkId"] != linkID {
		t.Fatalf("link replay: %+v", again)
	}
	if other := f.fleet(t, http.MethodPost, "/provider-links", "org_second", "assoc:x", map[string]any{"organizationId": "org_second", "providerId": providerID, "membershipId": "pmb_x"}); other.status != http.StatusUnprocessableEntity {
		t.Fatalf("a rider sponsored by one organization must not be linkable by another: %+v", other)
	}
	if hidden := f.fleet(t, http.MethodGet, "/provider-links/"+linkID, "org_second", "", nil); hidden.status != http.StatusNotFound {
		t.Fatalf("cross-organization read: %+v", hidden)
	}
	if byProvider := f.fleet(t, http.MethodGet, "/provider-links/by-provider/"+providerID, org, "", nil); byProvider.data["linkId"] != linkID {
		t.Fatalf("by provider: %+v", byProvider)
	}

	// The organization can never accept on the rider's behalf.
	if forced := f.fleet(t, http.MethodPost, "/provider-links/"+linkID+"/transitions", org, "force-"+linkID,
		map[string]any{"state": "ACTIVE", "expectedVersion": 1, "organizationId": org}); forced.status != http.StatusConflict {
		t.Fatalf("organization accepted for the rider: %+v", forced)
	}

	// Only the rider sees and decides their association.
	list := f.rider(t, http.MethodGet, "", providerID, nil)
	associations, _ := list.data["associations"].([]any)
	if list.status != 200 || len(associations) != 1 {
		t.Fatalf("rider list: %+v", list)
	}
	stranger := f.seedRider(t, "+2348031110099", "stranger@example.com", "verified")
	if theirs := f.rider(t, http.MethodPost, "/"+linkID+"/accept", stranger, nil); theirs.status != http.StatusNotFound {
		t.Fatalf("another rider accepted: %+v", theirs)
	}
	if stale := f.rider(t, http.MethodPost, "/"+linkID+"/accept", providerID, map[string]any{"expectedVersion": 7}); stale.status != http.StatusConflict {
		t.Fatalf("stale version accepted: %+v", stale)
	}
	accepted := f.rider(t, http.MethodPost, "/"+linkID+"/accept", providerID, map[string]any{"expectedVersion": 1})
	if accepted.status != 200 || accepted.data["status"] != "ACTIVE" || accepted.data["decidedAt"] == nil {
		t.Fatalf("accept: %+v", accepted)
	}
	if again := f.rider(t, http.MethodPost, "/"+linkID+"/accept", providerID, nil); again.status != 200 || again.data["status"] != "ACTIVE" {
		t.Fatalf("repeated accept must be idempotent: %+v", again)
	}
	if flip := f.rider(t, http.MethodPost, "/"+linkID+"/decline", providerID, nil); flip.status != http.StatusConflict {
		t.Fatalf("decline after accept: %+v", flip)
	}

	// Fleet reads the decision back.
	readBack := f.fleet(t, http.MethodGet, "/provider-links/"+linkID, org, "", nil)
	if readBack.data["status"] != "ACTIVE" || readBack.data["sourceVersion"] != float64(2) {
		t.Fatalf("read back: %+v", readBack)
	}
	var actor string
	if err := f.pool.QueryRow(context.Background(), `SELECT actor_type FROM dispatch_organization_provider_link_history
		WHERE link_id=$1 AND new_state='ACTIVE'`, linkID).Scan(&actor); err != nil || actor != "PROVIDER" {
		t.Fatalf("consent must be recorded as the provider's: %q %v", actor, err)
	}

	// Organization lifecycle under optimistic concurrency, replay-safe.
	suspend := map[string]any{"state": "SUSPENDED", "expectedVersion": 2, "organizationId": org, "reasonCode": "TEST"}
	suspended := f.fleet(t, http.MethodPost, "/provider-links/"+linkID+"/transitions", org, "suspend-"+linkID, suspend)
	if suspended.status != 200 || suspended.data["status"] != "SUSPENDED" {
		t.Fatalf("suspend: %+v", suspended)
	}
	if replay := f.fleet(t, http.MethodPost, "/provider-links/"+linkID+"/transitions", org, "suspend-"+linkID, suspend); replay.status != 200 || replay.data["sourceVersion"] != float64(3) {
		t.Fatalf("transition replay: %+v", replay)
	}
	if stale := f.fleet(t, http.MethodPost, "/provider-links/"+linkID+"/transitions", org, "revoke-stale",
		map[string]any{"state": "REVOKED", "expectedVersion": 2, "organizationId": org}); stale.status != http.StatusConflict {
		t.Fatalf("stale transition: %+v", stale)
	}
}

func TestResolveNeverMergesAndSponsorRefusesTakenContacts(t *testing.T) {
	f := newFixture(t, true)
	first := f.seedRider(t, "+2348031120001", "one@example.com", "verified")
	f.seedRider(t, "+2348031120002", "two@example.com", "verified")

	split := f.fleet(t, http.MethodPost, "/providers/resolve", "", "", map[string]any{"emailNormalized": "one@example.com", "phoneE164": "+2348031120002"})
	if split.data["outcome"] != "CONFLICT" || split.data["providerId"] != nil {
		t.Fatalf("split identity must be a conflict without an id: %+v", split)
	}
	single := f.fleet(t, http.MethodPost, "/providers/resolve", "", "", map[string]any{"emailNormalized": "ONE@example.com", "phoneE164": "+2348039999999"})
	if single.data["outcome"] != "MATCHED" || single.data["providerId"] != first || single.data["platformStatus"] != "ACTIVE" {
		t.Fatalf("single match: %+v", single)
	}
	if bad := f.fleet(t, http.MethodPost, "/providers/resolve", "", "", map[string]any{"emailNormalized": "nope", "phoneE164": "+2348031120002"}); bad.status != http.StatusBadRequest {
		t.Fatalf("invalid email: %+v", bad)
	}
	taken := f.fleet(t, http.MethodPost, "/providers/sponsored", "org_a", "sponsor-taken", sponsorBody("org_a", "fresh@example.com", "+2348031120001"))
	if taken.status != http.StatusConflict || taken.code != "PROVIDER_IDENTITY_CONFLICT" {
		t.Fatalf("sponsoring a taken phone: %+v", taken)
	}

	// An existing independent rider can be associated; declining is final.
	link := f.fleet(t, http.MethodPost, "/provider-links", "org_a", "assoc:pmb_1", map[string]any{"organizationId": "org_a", "providerId": first, "membershipId": "pmb_1"})
	if link.status != http.StatusCreated {
		t.Fatalf("link existing rider: %+v", link)
	}
	declined := f.rider(t, http.MethodPost, "/"+link.data["linkId"].(string)+"/decline", first, nil)
	if declined.data["status"] != "REJECTED" {
		t.Fatalf("decline: %+v", declined)
	}
	// Declining frees the rider for a new request.
	if again := f.fleet(t, http.MethodPost, "/provider-links", "org_b", "assoc:pmb_2", map[string]any{"organizationId": "org_b", "providerId": first, "membershipId": "pmb_2"}); again.status != http.StatusCreated {
		t.Fatalf("relink after decline: %+v", again)
	}
	if dup := f.fleet(t, http.MethodPost, "/provider-links", "org_c", "assoc:pmb_3", map[string]any{"organizationId": "org_c", "providerId": first, "membershipId": "pmb_3"}); dup.status != http.StatusConflict || dup.code != "LINK_CONFLICT" {
		t.Fatalf("second live link: %+v", dup)
	}
}

func TestSuspendedRiderIsIneligibleAndWritesRespectTheFlag(t *testing.T) {
	f := newFixture(t, false)
	if off := f.fleet(t, http.MethodPost, "/providers/sponsored", "org_a", "k", sponsorBody("org_a", "x@example.com", "+2348031130001")); off.status != http.StatusServiceUnavailable {
		t.Fatalf("disabled sponsor: %+v", off)
	}
	f = newFixture(t, true)
	suspended := f.seedRider(t, "+2348031130002", "s@example.com", "suspended")
	if got := f.fleet(t, http.MethodPost, "/provider-links", "org_a", "assoc:s", map[string]any{"organizationId": "org_a", "providerId": suspended, "membershipId": "pmb_s"}); got.status != http.StatusUnprocessableEntity {
		t.Fatalf("suspended rider linked: %+v", got)
	}
}

func TestConcurrentSponsorshipCreatesOneRider(t *testing.T) {
	f := newFixture(t, true)
	body := sponsorBody("org_a", "race@example.com", "+2348031140001")
	var wg sync.WaitGroup
	ids := make([]string, 6)
	for i := range ids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			got := f.fleet(t, http.MethodPost, "/providers/sponsored", "org_a", "sponsor-race", body)
			if id, ok := got.data["providerId"].(string); ok {
				ids[i] = id
			} else {
				ids[i] = fmt.Sprintf("status=%d code=%s", got.status, got.code)
			}
		}(i)
	}
	wg.Wait()
	for _, id := range ids[1:] {
		if id != ids[0] {
			t.Fatalf("concurrent sponsorship diverged: %v", ids)
		}
	}
	var count int
	_ = f.pool.QueryRow(context.Background(), `SELECT count(*) FROM providers WHERE email='race@example.com'`).Scan(&count)
	if count != 1 {
		t.Fatalf("providers created: %d", count)
	}
}
