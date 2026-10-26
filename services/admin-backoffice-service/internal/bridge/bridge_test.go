package bridge

import (
	"bytes"
	"context"
	contract "cosmicforge/logistics/services/admin-backoffice-service/docs/openapi"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/config"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/features/operations"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/persistence"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/security"
	"cosmicforge/logistics/shared/go/serviceauth"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeRepo struct {
	mu                                        sync.Mutex
	claims                                    map[string]bool
	audits                                    []persistence.Audit
	revoked, principalDenied                  bool
	unavailable, auditUnavailable, rateDenied bool
	records                                   int
	operation                                 operations.Operation
}

func (f *fakeRepo) Ready(context.Context) error {
	if f.unavailable {
		return errors.New("database secret must not escape")
	}
	return nil
}
func (f *fakeRepo) Principal(context.Context, string, string, string, time.Time) error {
	if f.unavailable {
		return errors.New("private topology")
	}
	if f.principalDenied {
		return security.ErrDenied
	}
	return nil
}
func (f *fakeRepo) Revoked(context.Context, security.Claims) (bool, error) { return f.revoked, nil }
func (f *fakeRepo) Claim(_ context.Context, n, j string, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.claims[n] || f.claims[j] {
		return persistence.ErrReplay
	}
	f.claims[n] = true
	f.claims[j] = true
	return nil
}
func (f *fakeRepo) Rate(context.Context, string, time.Time, int) (bool, error) {
	return !f.rateDenied, nil
}
func (f *fakeRepo) Audit(_ context.Context, a persistence.Audit) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.auditUnavailable {
		return errors.New("audit secret")
	}
	f.audits = append(f.audits, a)
	return nil
}
func (f *fakeRepo) RecordRejected(_ context.Context, c operations.Command, _ string, _ string) (operations.Operation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.records++
	return operations.Operation{ID: c.CommandID, Status: "REJECTED"}, nil
}
func (f *fakeRepo) Operation(_ context.Context, _ string, staff string, scope security.Scope) (operations.Operation, error) {
	if staff != "staff-1" || scope != f.operation.Scope {
		return operations.Operation{}, operations.ErrNotFound
	}
	return f.operation, nil
}

type fixture struct {
	b      *Bridge
	repo   *fakeRepo
	key    ed25519.PrivateKey
	secret []byte
	cert   *x509.Certificate
	claims security.Claims
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	now := time.Now()
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	secret := bytes.Repeat([]byte{42}, 32)
	trust := config.Trust{Issuer: "hq-test", Audience: "karryam-test", WorkloadURI: "spiffe://test/karryam/hq", PolicyVersion: "test-policy-v1", AssertionLifetimeSeconds: 60, SessionLifetimeSeconds: 300, AllowedACR: []string{"mfa"}, StrongACR: []string{"mfa"}, RequestsPerMinute: 100, StaffKeys: map[string]string{"staff-1": base64.RawURLEncoding.EncodeToString(pub)}, ServiceKeys: map[string]string{"workload-1": base64.RawURLEncoding.EncodeToString(secret)}, StaffKeyExpires: map[string]int64{"staff-1": now.Add(time.Hour).Unix()}, ServiceKeyExpires: map[string]int64{"workload-1": now.Add(time.Hour).Unix()}}
	repo := &fakeRepo{claims: map[string]bool{}}
	b, err := New(repo, trust, true)
	if err != nil {
		t.Fatal(err)
	}
	uri, _ := url.Parse(trust.WorkloadURI)
	cert := &x509.Certificate{Raw: []byte("test-verified-certificate"), URIs: []*url.URL{uri}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour)}
	permissions := []string{}
	// The default fixture retains its existing domains. Resource/command tests
	// delegate only their specific permissions; the production 128-entry bound
	// must not grow with the capability catalogue.
	for _, c := range contract.Matrix() {
		if strings.HasPrefix(c.Permission, "vendor.") || strings.HasPrefix(c.Permission, "wallet.") || strings.HasPrefix(c.Permission, "referral.") || strings.HasPrefix(c.Permission, "support.") || strings.HasPrefix(c.Permission, "verification.") || strings.HasPrefix(c.Permission, "hq.") {
			continue
		}
		if !config.Contains(permissions, c.Permission) {
			permissions = append(permissions, c.Permission)
		}
	}
	c := security.Claims{Issuer: trust.Issuer, Subject: "staff-1", Audience: trust.Audience, Product: "karryam", Roles: []string{"staff"}, Permissions: permissions, Markets: []string{"NG"}, Organizations: []string{"org-1"}, SessionID: "session-1", AuthTime: now.Add(-time.Minute).Unix(), ACR: "mfa", IssuedAt: now.Unix(), NotBefore: now.Unix(), Expires: now.Add(time.Minute).Unix()}
	return &fixture{b: b, repo: repo, key: key, secret: secret, cert: cert, claims: c}
}
func (f *fixture) request(t *testing.T, method, path string, body []byte) *http.Request {
	t.Helper()
	r := httptest.NewRequest(method, Base+path, bytes.NewReader(body))
	r.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{f.cert}, VerifiedChains: [][]*x509.Certificate{{f.cert}}}
	c := f.claims
	c.JTI = uuid.NewString()
	c.RequestDigest = security.Digest(r, body)
	h := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"EdDSA","typ":"JWT","kid":"staff-1"}`))
	raw, _ := json.Marshal(c)
	payload := h + "." + base64.RawURLEncoding.EncodeToString(raw)
	token := payload + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(f.key, []byte(payload)))
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("X-HQ-Key-ID", "workload-1")
	r.Header.Set("Content-Type", "application/json")
	if err := serviceauth.SignRequest(r, config.HQPrincipal, f.secret, body, time.Now()); err != nil {
		t.Fatal(err)
	}
	return r
}

const scopeQuery = "?market=NG&organization=org-1&purpose=CASE-123"

func TestRolloutFlagsCannotCreateOwnerCapabilities(t *testing.T) {
	f := newFixture(t)
	f.b.ConfigureDomainFlags(config.DomainFlags{RemoteWrites: true, CustomerWrites: true, TaxiWrites: true, DispatchRecovery: true, HaulingRecovery: true, AnalyticsReads: true, CustomerSensitiveReads: true})
	if w := invoke(f.b, f.request(t, "GET", "/analytics/network/mission-success"+scopeQuery, nil)); w.Code != 503 {
		t.Fatalf("flag enabled unscoped legacy report: %d", w.Code)
	}
	command := operations.Command{ContractVersion: "1.0", CommandID: uuid.NewString(), Product: "karryam", Action: "dispatch.provider.suspension.request", Target: operations.Target{Type: "dispatch_provider", ID: "provider-1", ExpectedVersion: 7}, Scope: security.Scope{Market: "NG", Organization: "org-1"}, ReasonCode: "SAFETY_REVIEW", Reason: "test-only reason", CaseReference: "case-1", IdempotencyKey: "test-flag-gate", RequestedAt: time.Now().UTC().Add(-time.Second)}
	body, _ := json.Marshal(command)
	w := invoke(f.b, f.request(t, "POST", "/operations"+scopeQuery, body))
	if w.Code != 503 || f.repo.records != 1 || !strings.Contains(w.Body.String(), "capability_blocked") {
		t.Fatalf("flag created executable command: %d %s", w.Code, w.Body)
	}
	w = invoke(f.b, f.request(t, "GET", "/health/aggregate"+scopeQuery, nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"remote_writes":"DISABLED"`) {
		t.Fatalf("unimplemented writes advertised available: %d %s", w.Code, w.Body)
	}
}

func invoke(b *Bridge, r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	b.ServeHTTP(w, r)
	return w
}

func TestAuthorizedDiscovery(t *testing.T) {
	f := newFixture(t)
	w := invoke(f.b, f.request(t, "GET", "/capabilities"+scopeQuery, nil))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"READ_ONLY"`) || !strings.Contains(w.Body.String(), `"BLOCKED_DEPENDENCY"`) {
		t.Fatal(w.Body.String())
	}
	if len(f.repo.audits) != 1 || f.repo.audits[0].Staff != "staff-1" {
		t.Fatal("missing verified audit")
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("browser CORS present")
	}
}

func TestAuthenticationDenialsBeforeOperation(t *testing.T) {
	tests := map[string]func(*fixture, *http.Request){
		"missing_certificate":    func(f *fixture, r *http.Request) { r.TLS = nil },
		"unverified_certificate": func(f *fixture, r *http.Request) { r.TLS.VerifiedChains = nil },
		"expired_certificate":    func(f *fixture, r *http.Request) { f.cert.NotAfter = time.Now().Add(-time.Second) },
		"wrong_workload":         func(f *fixture, r *http.Request) { f.cert.URIs[0], _ = url.Parse("spiffe://test/healthnet/hq") },
		"revoked_certificate":    func(f *fixture, r *http.Request) { f.repo.principalDenied = true },
		"invalid_signature":      func(f *fixture, r *http.Request) { r.Header.Set(serviceauth.HeaderSignature, "bad") },
		"wrong_service": func(f *fixture, r *http.Request) {
			r.Header.Set(serviceauth.HeaderServiceName, "admin-backoffice-service")
		},
		"body_modified": func(f *fixture, r *http.Request) {
			r.Body = io.NopCloser(strings.NewReader(`{"performed_by":"admin"}`))
		},
		"uri_modified": func(f *fixture, r *http.Request) { r.URL.RawQuery = "market=IT&organization=org-1&purpose=CASE-123" },
		"timestamp_expired": func(f *fixture, r *http.Request) {
			_ = serviceauth.SignRequest(r, config.HQPrincipal, f.secret, nil, time.Now().Add(-time.Hour))
		},
		"jwt_tampered":         func(f *fixture, r *http.Request) { r.Header.Set("Authorization", r.Header.Get("Authorization")+"x") },
		"jwt_missing":          func(f *fixture, r *http.Request) { r.Header.Del("Authorization") },
		"jwt_duplicated":       func(f *fixture, r *http.Request) { r.Header.Add("Authorization", r.Header.Get("Authorization")) },
		"revoked_staff":        func(f *fixture, r *http.Request) { f.repo.revoked = true },
		"identity_unavailable": func(f *fixture, r *http.Request) { f.repo.unavailable = true },
		"service_key_retired":  func(f *fixture, r *http.Request) { delete(f.b.verifiers, "workload-1") },
		"service_key_expired": func(f *fixture, r *http.Request) {
			f.b.trust.ServiceKeyExpires["workload-1"] = time.Now().Add(-time.Second).Unix()
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			r := f.request(t, "POST", "/operations"+scopeQuery, nil)
			mutate(f, r)
			w := invoke(f.b, r)
			if w.Code < 400 {
				t.Fatal("denial failed", w.Code)
			}
			if f.repo.records != 0 {
				t.Fatal("unauthorized owner-side effect")
			}
			if len(f.repo.audits) != 1 {
				t.Fatal("denial was not audited")
			}
			if strings.Contains(w.Body.String(), "private topology") {
				t.Fatal("internal details leaked")
			}
		})
	}
}

func TestActorScopeAndStrengthDenials(t *testing.T) {
	for name, mutate := range map[string]func(*security.Claims){"wrong_audience": func(c *security.Claims) { c.Audience = "healthnet" }, "wrong_product": func(c *security.Claims) { c.Product = "healthnet" }, "session_expired": func(c *security.Claims) { c.AuthTime = time.Now().Add(-time.Hour).Unix() }, "missing_permission": func(c *security.Claims) { c.Permissions = []string{"other.read"} }, "cross_market": func(c *security.Claims) { c.Markets = []string{"IT"} }, "cross_tenant": func(c *security.Claims) { c.Organizations = []string{"org-2"} }, "mfa": func(c *security.Claims) { c.ACR = "password" }} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			mutate(&f.claims)
			w := invoke(f.b, f.request(t, "POST", "/operations"+scopeQuery, nil))
			if w.Code < 400 || f.repo.records != 0 {
				t.Fatal("unauthorized side effect", w.Code)
			}
		})
	}
}

func TestNonceAndJTIReplay(t *testing.T) {
	f := newFixture(t)
	r := f.request(t, "GET", "/capabilities"+scopeQuery, nil)
	r2 := r.Clone(r.Context())
	r2.Body = http.NoBody
	if invoke(f.b, r).Code != 200 {
		t.Fatal("first request failed")
	}
	if w := invoke(f.b, r2); w.Code != 409 {
		t.Fatal("nonce replay admitted", w.Code)
	}
	r3 := f.request(t, "GET", "/capabilities"+scopeQuery, nil)
	r3.Header.Set("Authorization", r.Header.Get("Authorization"))
	if w := invoke(f.b, r3); w.Code != 409 {
		t.Fatal("JTI replay admitted", w.Code)
	}
}

func TestRejectedSubmission(t *testing.T) {
	f := newFixture(t)
	c := operations.Command{ContractVersion: "1.0", CommandID: uuid.NewString(), Product: "karryam", Action: "dispatch.provider.suspension.request", Target: operations.Target{Type: "dispatch_provider", ID: "provider-1", ExpectedVersion: 7}, Scope: security.Scope{Market: "NG", Organization: "org-1"}, ReasonCode: "SAFETY_REVIEW", Reason: "PRIVATE_EVIDENCE_DO_NOT_LOG", CaseReference: "case-1", IdempotencyKey: "idem-1", RequestedAt: time.Now().UTC().Add(-time.Second)}
	body, _ := json.Marshal(c)
	w := invoke(f.b, f.request(t, "POST", "/operations"+scopeQuery, body))
	if w.Code != 503 || !strings.Contains(w.Body.String(), `"retryable":false`) || f.repo.records != 1 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, a := range f.repo.audits {
		raw, _ := json.Marshal(a)
		if strings.Contains(string(raw), c.Reason) {
			t.Fatal("private evidence leaked")
		}
	}
}

func TestDisabledRoutes(t *testing.T) {
	for _, c := range contract.Matrix() {
		if c.State == "READ_ONLY" || c.ID == "bridge.operations.submit" {
			continue
		}
		f := newFixture(t)
		f.claims.Permissions = []string{c.Permission}
		path := c.BridgeEndpoint
		for strings.Contains(path, "{") {
			start := strings.Index(path, "{")
			end := strings.Index(path[start:], "}")
			path = path[:start] + "test" + path[start+end+1:]
		}
		w := invoke(f.b, f.request(t, c.Method, path+scopeQuery, nil))
		if w.Code != 503 || f.repo.records != 0 {
			t.Errorf("disabled route %s executed: %d", c.ID, w.Code)
		}
	}
}
func TestOperationScope(t *testing.T) {
	f := newFixture(t)
	f.repo.operation = operations.Operation{ID: uuid.NewString(), Scope: security.Scope{Market: "NG", Organization: "org-2"}, Status: "UNKNOWN_OUTCOME"}
	w := invoke(f.b, f.request(t, "GET", "/operations/"+f.repo.operation.ID+scopeQuery, nil))
	if w.Code != 404 {
		t.Fatal("cross-tenant operation leaked", w.Code)
	}
}
func TestHealthDegraded(t *testing.T) {
	f := newFixture(t)
	f.repo.unavailable = true
	w := httptest.NewRecorder()
	f.b.Probe(w, httptest.NewRequest("GET", "/ready", nil))
	if w.Code != 503 {
		t.Fatal("degraded readiness passed")
	}
	f.repo.unavailable = false
	f.b.enabled = false
	if w = invoke(f.b, f.request(t, "GET", "/health/aggregate"+scopeQuery, nil)); w.Code != 503 {
		t.Fatal("disabled boundary accepted")
	}
}
func TestAuditAndRateFailClosed(t *testing.T) {
	f := newFixture(t)
	f.repo.auditUnavailable = true
	if w := invoke(f.b, f.request(t, "GET", "/capabilities"+scopeQuery, nil)); w.Code != 503 || strings.Contains(w.Body.String(), "audit secret") {
		t.Fatal("audit outage exposed data")
	}
	f.repo.auditUnavailable = false
	f.repo.rateDenied = true
	if w := invoke(f.b, f.request(t, "GET", "/capabilities"+scopeQuery, nil)); w.Code != 429 {
		t.Fatal("rate limit bypass")
	}
}
func TestMalformedInputs(t *testing.T) {
	for _, path := range []string{"/capabilities?market=NG&market=IT&organization=org-1&purpose=case", "/capabilities?market=NG&organization=org-1&purpose=case&limit=1000000", "/capabilities?market=NG&organization=org-1", "/operations/not-a-uuid" + scopeQuery} {
		f := newFixture(t)
		if w := invoke(f.b, f.request(t, "GET", path, nil)); w.Code != 400 {
			t.Errorf("unbounded query accepted: %s = %d", path, w.Code)
		}
	}
	f := newFixture(t)
	if w := invoke(f.b, f.request(t, "POST", "/operations"+scopeQuery, bytes.Repeat([]byte("x"), 32769))); w.Code != 413 {
		t.Fatal("oversized request accepted", w.Code)
	}
}
