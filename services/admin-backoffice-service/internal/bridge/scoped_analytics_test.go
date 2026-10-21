package bridge

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cosmicforge/logistics/services/admin-backoffice-service/internal/config"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/persistence"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/testutil"
	"cosmicforge/logistics/shared/go/adminpolicy"
	policyfixture "cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/analyticsadmin"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func TestPostgresScopedAnalyticsBridgeMTLSFiveScopeAndSuppressedWire(t *testing.T) {
	f := newFixture(t)
	f.claims.Zones = []string{"Z1"}
	f.claims.Tenants = []string{"org-1"}
	f.b.ConfigureDomainFlags(config.DomainFlags{AnalyticsReads: true})
	db := testutil.Database(t)
	f.b.repo = &persistence.Store{Pool: db}
	pair, leaf, roots := workloadCertificate(t, f.b.trust.WorkloadURI, false)
	f.cert = leaf
	sum := sha256.Sum256(leaf.Raw)
	if _, err := db.Exec(context.Background(), `INSERT INTO admin_remote_principals(principal,certificate_sha256,workload_uri,product,audience,expires_at) VALUES($1,$2,$3,'karryam',$4,$5)`, config.HQPrincipal, hex.EncodeToString(sum[:]), f.b.trust.WorkloadURI, f.b.trust.Audience, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	scope := adminpolicy.Scope{Product: "karryam", Market: "NG", Zone: "Z1", Organization: "org-1", Tenant: "org-1"}
	policy := policyfixture.New(t, scope, "analytics-service", analyticsadmin.MissionSuccessPermission, "analytics.scoped-mission-success.v1:provider_id")
	key := []byte(strings.Repeat("a", 32))
	var calls atomic.Int32
	owner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != analyticsadmin.MissionSuccessPath || r.Header.Get("Authorization") != "" || r.Header.Get("X-HQ-Key-ID") != "" {
			t.Error("unscoped path/HQ credential forwarding")
		}
		if _, err := serviceauth.NewVerifier(serviceauth.Secrets{adminread.Caller: key}, time.Minute).VerifyRequest(r); err != nil {
			t.Error(err)
		}
		values := r.URL.Query()
		values.Del("authority")
		values.Del("product_code")
		q, err := adminread.Parse(values, time.Now())
		if err != nil {
			t.Error(err)
		}
		if q.Scope.Tenant != scope.Tenant || q.Scope.Zone != scope.Zone || q.Staff != f.claims.Subject || q.PolicyChecksum != policy.Engine.Checksum() {
			t.Error("scope/delegation lost")
		}
		out := analyticsadmin.Release{PolicyID: q.PolicyID, PolicyVersion: q.PolicyVersion, PolicyChecksum: q.PolicyChecksum, Scope: scope, Metric: "mission-success", SourceVersion: strings.Repeat("b", 64), AsOf: time.Now().UTC(), Freshness: "LIVE", Coverage: "PARTIAL_COVERAGE", Cells: []analyticsadmin.Cell{}}
		for _, name := range []string{"completed", "failed", "cancelled", "guarantee_breaches"} {
			out.Cells = append(out.Cells, analyticsadmin.Cell{Key: name, Suppression: "PARTIAL_COVERAGE"})
		}
		json.NewEncoder(w).Encode(out)
	}))
	defer owner.Close()
	client, err := analyticsadmin.NewClient(owner.URL, key)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := f.b.ConfigureScopedAnalytics(client, policy.Engine); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(f.b)
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots}
	server.StartTLS()
	defer server.Close()
	hq := server.Client()
	hq.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{pair}
	values := adminread.Query{Scope: adminread.Scope{Market: "NG", Zone: "Z1", Organization: "org-1", Tenant: "org-1"}, Purpose: "case-1", Start: time.Now().UTC().Add(-time.Hour).Truncate(time.Second), End: time.Now().UTC().Truncate(time.Second)}.Values()
	for _, name := range []string{"staff", "role", "policy_id", "policy_version", "policy_checksum", "limit", "cursor"} {
		values.Del(name)
	}
	values.Set("authority", "TAXI")
	values.Set("product_code", "TAXI_PASSENGER")
	path := "/analytics/scoped/network/mission-success?" + values.Encode()
	request := f.request(t, "GET", path, nil)
	request.URL.Scheme = "https"
	request.URL.Host = strings.TrimPrefix(server.URL, "https://")
	request.RequestURI = ""
	response, err := hq.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != 200 {
		t.Fatal(response.StatusCode, string(raw), err)
	}
	if !strings.Contains(string(raw), `"coverage":"PARTIAL_COVERAGE"`) || strings.Contains(string(raw), "cohort_size") {
		t.Fatal("invalid suppression wire", string(raw))
	}
	if dir := os.Getenv("ADMIN_BRIDGE_CONTRACT_CAPTURE_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "scoped-analytics.json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, mode := range []string{"zone", "tenant", "permission", "role", "policy", "read_kill", "actor_injection", "pagination", "filter_injection"} {
		t.Run(mode, func(t *testing.T) {
			other := newFixture(t)
			other.claims.Zones = []string{"Z1"}
			other.claims.Tenants = []string{"org-1"}
			other.b.ConfigureDomainFlags(config.DomainFlags{AnalyticsReads: true})
			other.b.ConfigureScopedAnalytics(client, policy.Engine)
			target := path
			want := 403
			switch mode {
			case "zone":
				other.claims.Zones = []string{"Z2"}
			case "tenant":
				other.claims.Tenants = []string{"foreign"}
			case "permission":
				other.claims.Permissions = []string{"analytics.network.read"}
			case "role":
				other.claims.Roles = []string{"other"}
			case "policy":
				other.b.scopedAnalyticsPolicy = nil
				want = 503
			case "read_kill":
				other.b.ConfigureDomainFlags(config.DomainFlags{})
				want = 503
			case "actor_injection":
				target += "&staff=forged"
				want = 400
			case "pagination":
				target += "&limit=100"
				want = 400
			case "filter_injection":
				target += "&authority=HAULING"
				want = 400
			}
			before := calls.Load()
			out := invoke(other.b, other.request(t, "GET", target, nil))
			if out.Code != want || calls.Load() != before {
				t.Fatal(mode, out.Code, out.Body.String(), "owner called", calls.Load() != before)
			}
		})
	}
	if out := invoke(f.b, f.request(t, "GET", "/analytics/network/mission-success"+scopeQuery, nil)); out.Code != 503 {
		t.Fatal("legacy source became remotely available", out.Code)
	}
}
