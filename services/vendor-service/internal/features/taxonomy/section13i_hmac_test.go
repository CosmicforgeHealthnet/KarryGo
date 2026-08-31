package taxonomy_test

// Section 13I HMAC-v2 hard gate.
//
// This exercises the real production router, the real serviceauth verifier and
// the real vendor_service_nonces table. Nothing is stubbed and no signature is
// computed by hand except where a negative case requires a deliberately wrong
// one: every positive request is signed with serviceauth.SignRequest, which is
// the same function the platform's real callers use.
//
// The point is not that HMAC works - shared/go/serviceauth has its own tests.
// The point is that Section 13's platform taxonomy surface is actually behind
// it, with the caller allowlist and the durable nonce consumption in the chain,
// and that each individual binding (method, path, query, body) is really part
// of the signature rather than assumed to be.

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/database"
	"cosmicforge/logistics/services/vendor-service/internal/features/taxonomy"
	"cosmicforge/logistics/services/vendor-service/internal/platform/servicenonce"
	"cosmicforge/logistics/shared/go/serviceauth"
)

const (
	hmacAllowedCaller = "admin-backoffice-service"
	hmacDeniedCaller  = "analytics-service"
)

var (
	hmacAllowedSecret = []byte("section13i-taxonomy-admin-secret")
	hmacDeniedSecret  = []byte("section13i-analytics-secret")
)

// section13IHarness is a live internal taxonomy router over a real database.
type section13IHarness struct {
	router *gin.Engine
	pool   *pgxpool.Pool
}

func newSection13IHarness(t *testing.T) (*section13IHarness, context.Context, func()) {
	t.Helper()

	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping Section 13I HMAC hard gate")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if err = database.ApplyMigrations(ctx, pool, "../../../migrations"); err != nil {
		pool.Close()
		cancel()
		t.Fatalf("apply Vendor migrations: %v", err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()

	flags := taxonomy.Flags{TaxonomyEnabled: true, WritesEnabled: true}
	service := taxonomy.NewService(taxonomy.NewRepository(pool), nil)

	// Both callers hold a VALID configured secret. Only one is on the
	// allowlist. That is the distinction the allowlist exists to make: a
	// service can be authentic and still have no business publishing taxonomy.
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{
		hmacAllowedCaller: hmacAllowedSecret,
		hmacDeniedCaller:  hmacDeniedSecret,
	}, 5*time.Minute)

	taxonomy.RegisterInternalRoutes(
		router,
		taxonomy.NewInternalHandler(
			service,
			[]string{hmacAllowedCaller},
			servicenonce.NewStore(pool),
			flags,
		),
		verifier,
		flags,
	)

	return &section13IHarness{router: router, pool: pool}, ctx, func() {
		pool.Close()
		cancel()
	}
}

// signedRequest builds a request signed exactly as a real caller would.
func signedRequest(
	t *testing.T,
	method string,
	target string,
	body []byte,
	caller string,
	secret []byte,
	signedAt time.Time,
) *http.Request {
	t.Helper()
	request := httptest.NewRequest(method, target, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if method != http.MethodGet {
		request.Header.Set("Idempotency-Key", "section13i-"+strconv.FormatInt(time.Now().UnixNano(), 10))
	}
	if err := serviceauth.SignRequest(request, caller, secret, body, signedAt); err != nil {
		t.Fatalf("sign request: %v", err)
	}
	return request
}

func (h *section13IHarness) do(request *http.Request) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	h.router.ServeHTTP(recorder, request)
	return recorder
}

const listVersionsPath = "/internal/vendor/v1/taxonomy/namespaces/FOOD/versions"

// TestSection13IValidSignedRequestSucceeds is the positive control. Without it
// every negative case below could pass for the wrong reason - a route that
// 401s unconditionally rejects forged requests too.
func TestSection13IValidSignedRequestSucceeds(t *testing.T) {
	harness, _, cleanup := newSection13IHarness(t)
	defer cleanup()

	request := signedRequest(t, http.MethodGet, listVersionsPath, nil,
		hmacAllowedCaller, hmacAllowedSecret, time.Now())

	if response := harness.do(request); response.Code != http.StatusOK {
		t.Fatalf("valid signed request status=%d body=%s", response.Code, response.Body.String())
	}
}

// TestSection13IRejectsUnauthenticatedAndForgedRequests walks every way a
// caller can fail to prove itself. Each case mutates exactly one thing about an
// otherwise-valid request, so a pass means that one binding is really enforced
// rather than incidentally covered by another check.
func TestSection13IRejectsUnauthenticatedAndForgedRequests(t *testing.T) {
	harness, _, cleanup := newSection13IHarness(t)
	defer cleanup()

	body := []byte(`{}`)

	cases := []struct {
		name    string
		mutate  func(*http.Request)
		method  string
		target  string
		caller  string
		secret  []byte
		signAt  time.Time
		wantAny []int
	}{
		{
			name:    "missing service name",
			mutate:  func(r *http.Request) { r.Header.Del(serviceauth.HeaderServiceName) },
			wantAny: []int{http.StatusUnauthorized, http.StatusForbidden},
		},
		{
			name:    "missing signature",
			mutate:  func(r *http.Request) { r.Header.Del(serviceauth.HeaderSignature) },
			wantAny: []int{http.StatusUnauthorized},
		},
		{
			name:    "missing nonce",
			mutate:  func(r *http.Request) { r.Header.Del(serviceauth.HeaderNonce) },
			wantAny: []int{http.StatusUnauthorized},
		},
		{
			name:    "missing timestamp",
			mutate:  func(r *http.Request) { r.Header.Del(serviceauth.HeaderTimestamp) },
			wantAny: []int{http.StatusUnauthorized},
		},
		{
			name: "invalid signature",
			mutate: func(r *http.Request) {
				r.Header.Set(serviceauth.HeaderSignature, "0000000000000000000000000000000000000000000000000000000000000000")
			},
			wantAny: []int{http.StatusUnauthorized},
		},
		{
			name: "signature from a different secret",
			// Signed correctly, but with the wrong service's key while
			// claiming to be the allowlisted one.
			mutate: func(r *http.Request) {
				_ = serviceauth.SignRequest(r, hmacAllowedCaller, hmacDeniedSecret, body, time.Now())
			},
			wantAny: []int{http.StatusUnauthorized},
		},
		{
			name:    "stale timestamp outside the skew window",
			signAt:  time.Now().Add(-30 * time.Minute),
			wantAny: []int{http.StatusUnauthorized},
		},
		{
			name:    "future timestamp outside the skew window",
			signAt:  time.Now().Add(30 * time.Minute),
			wantAny: []int{http.StatusUnauthorized},
		},
		{
			name:    "authentic caller that is not allowlisted",
			caller:  hmacDeniedCaller,
			secret:  hmacDeniedSecret,
			wantAny: []int{http.StatusForbidden},
		},
		{
			name:    "unknown caller with an unconfigured secret",
			caller:  "not-a-configured-service",
			secret:  []byte("whatever"),
			wantAny: []int{http.StatusForbidden, http.StatusUnauthorized},
		},
		{
			name: "body altered after signing",
			// The signature covers a SHA-256 of the body, so swapping the body
			// after signing must not verify.
			mutate: func(r *http.Request) {
				r.Body = http.NoBody
				r.Body = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{"tampered":true}`))).Body
			},
			method:  http.MethodPost,
			target:  "/internal/vendor/v1/taxonomy/namespaces/FOOD/versions",
			wantAny: []int{http.StatusUnauthorized},
		},
		{
			name: "method altered after signing",
			mutate: func(r *http.Request) {
				r.Method = http.MethodPost
			},
			wantAny: []int{http.StatusUnauthorized, http.StatusNotFound},
		},
		{
			name: "path altered after signing",
			mutate: func(r *http.Request) {
				altered := *r.URL
				altered.Path = "/internal/vendor/v1/taxonomy/namespaces/SHOP/versions"
				r.URL = &altered
			},
			wantAny: []int{http.StatusUnauthorized},
		},
		{
			name: "query string altered after signing",
			// RequestURI is signed, so appending a query parameter to a signed
			// request breaks the signature. This is what stops a captured read
			// being re-aimed at different data.
			mutate: func(r *http.Request) {
				altered := *r.URL
				altered.RawQuery = "target_version_id=ctv_someone_elses"
				r.URL = &altered
			},
			wantAny: []int{http.StatusUnauthorized},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			method := testCase.method
			if method == "" {
				method = http.MethodGet
			}
			target := testCase.target
			if target == "" {
				target = listVersionsPath
			}
			caller := testCase.caller
			secret := testCase.secret
			if caller == "" {
				caller, secret = hmacAllowedCaller, hmacAllowedSecret
			}
			signAt := testCase.signAt
			if signAt.IsZero() {
				signAt = time.Now()
			}

			var requestBody []byte
			if method != http.MethodGet {
				requestBody = body
			}

			request := signedRequest(t, method, target, requestBody, caller, secret, signAt)
			if testCase.mutate != nil {
				testCase.mutate(request)
			}

			response := harness.do(request)
			for _, want := range testCase.wantAny {
				if response.Code == want {
					return
				}
			}
			t.Fatalf("status=%d want one of %v body=%s",
				response.Code, testCase.wantAny, response.Body.String())
		})
	}
}

// TestSection13INonceReplayIsRejectedAndDurable proves the replay guard is a
// database fact rather than in-process memory.
//
// The same signed request is sent twice. The first is accepted; the second is
// refused because the nonce row already exists. The test then reads
// vendor_service_nonces directly, because "the second call 401'd" would also be
// true of an in-memory guard that a restart would forget - and a replay window
// that reopens on deploy is not a replay guard.
func TestSection13INonceReplayIsRejectedAndDurable(t *testing.T) {
	harness, ctx, cleanup := newSection13IHarness(t)
	defer cleanup()

	request := signedRequest(t, http.MethodGet, listVersionsPath, nil,
		hmacAllowedCaller, hmacAllowedSecret, time.Now())
	nonce := request.Header.Get(serviceauth.HeaderNonce)
	if nonce == "" {
		t.Fatal("signed request carries no nonce")
	}

	if response := harness.do(request); response.Code != http.StatusOK {
		t.Fatalf("first request status=%d body=%s", response.Code, response.Body.String())
	}

	var stored int
	if err := harness.pool.QueryRow(
		ctx,
		`SELECT count(*) FROM vendor_service_nonces WHERE caller=$1 AND nonce=$2`,
		hmacAllowedCaller, nonce,
	).Scan(&stored); err != nil {
		t.Fatalf("read nonce evidence: %v", err)
	}
	if stored != 1 {
		t.Fatalf("nonce rows=%d want=1", stored)
	}

	// Replay the byte-identical signed request.
	replay := signedRequest(t, http.MethodGet, listVersionsPath, nil,
		hmacAllowedCaller, hmacAllowedSecret, time.Now())
	replay.Header.Set(serviceauth.HeaderNonce, nonce)
	// Re-sign so the signature matches the reused nonce: this is exactly what
	// an attacker replaying a captured request has.
	if err := serviceauth.SignRequest(replay, hmacAllowedCaller, hmacAllowedSecret, nil, time.Now()); err != nil {
		t.Fatalf("re-sign replay: %v", err)
	}

	if response := harness.do(replay); response.Code != http.StatusUnauthorized {
		t.Fatalf("replayed nonce status=%d want=401 body=%s", response.Code, response.Body.String())
	}
}

// TestSection13IDisabledFlagRegistersNoInternalRoute proves the rollout switch
// removes the surface rather than merely refusing at it.
func TestSection13IDisabledFlagRegistersNoInternalRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	disabled := taxonomy.Flags{TaxonomyEnabled: false, WritesEnabled: false}
	taxonomy.RegisterInternalRoutes(
		router,
		taxonomy.NewInternalHandler(
			taxonomy.NewService(taxonomy.NewRepository(nil), nil),
			[]string{hmacAllowedCaller},
			nil,
			disabled,
		),
		serviceauth.NewVerifier(serviceauth.Secrets{hmacAllowedCaller: hmacAllowedSecret}, time.Minute),
		disabled,
	)

	for _, route := range router.Routes() {
		t.Fatalf("disabled taxonomy still registered %s %s", route.Method, route.Path)
	}
}
