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

func TestPostgresScopedMissionRateRequiresBothPermissionsAndPreservesSuppression(t *testing.T) {
	f := newFixture(t)
	f.claims.Zones = []string{"Z1"}
	f.claims.Tenants = []string{"org-1"}
	f.claims.Permissions = []string{analyticsadmin.MissionSuccessPermission, analyticsadmin.MissionRatePermission}
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
	rateRule := policy.Document.Rules[0]
	rateRule.Capability = analyticsadmin.MissionRatePermission
	rateRule.Permission = analyticsadmin.MissionRatePermission
	rateRule.SourceContract = analyticsadmin.MissionRateSourceContract(rateRule.SourceContract)
	policy.Document.Rules = append(policy.Document.Rules, rateRule)
	policy.Sign(t)
	key := []byte(strings.Repeat("a", 32))
	var calls atomic.Int32
	var complete atomic.Bool
	// Complete data is a TEST_ONLY owner response for the projection boundary.
	// The real native source remains obligatorily PARTIAL_COVERAGE.
	owner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != analyticsadmin.MissionSuccessPath {
			t.Error("rate bypassed scoped owner source")
		}
		if _, err := serviceauth.NewVerifier(serviceauth.Secrets{adminread.Caller: key}, time.Minute).VerifyRequest(r); err != nil {
			t.Error(err)
		}
		out := analyticsadmin.Release{PolicyID: policy.Engine.ID(), PolicyVersion: policy.Engine.Version(), PolicyChecksum: policy.Engine.Checksum(), Scope: scope, Metric: "mission-success", SourceVersion: "TEST_ONLY_RATE_SOURCE", AsOf: time.Now().UTC(), Freshness: "LIVE", Coverage: "PARTIAL_COVERAGE", Cells: []analyticsadmin.Cell{}}
		values := []int64{3, 1, 0, 0}
		for i, name := range []string{"completed", "failed", "cancelled", "guarantee_breaches"} {
			cell := analyticsadmin.Cell{Key: name, Suppression: "PARTIAL_COVERAGE"}
			if complete.Load() {
				cell.Value = &values[i]
				cell.Suppression = "NONE"
			}
			out.Cells = append(out.Cells, cell)
		}
		if complete.Load() {
			out.Coverage = "COMPLETE"
		}
		json.NewEncoder(w).Encode(out)
	}))
	defer owner.Close()
	client, err := analyticsadmin.NewClient(owner.URL, key)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := f.b.ConfigureScopedAnalytics(client, policy.Engine, true); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(f.b)
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots}
	server.StartTLS()
	defer server.Close()
	hq := server.Client()
	hq.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{pair}
	values := adminread.Query{Scope: adminread.Scope{Market: scope.Market, Zone: scope.Zone, Organization: scope.Organization, Tenant: scope.Tenant}, Purpose: "case-1", Start: time.Now().UTC().Add(-time.Hour).Truncate(time.Second), End: time.Now().UTC().Truncate(time.Second)}.Values()
	for _, name := range []string{"staff", "role", "policy_id", "policy_version", "policy_checksum", "limit", "cursor"} {
		values.Del(name)
	}
	values.Set("authority", "TAXI")
	values.Set("product_code", "TAXI_PASSENGER")
	path := "/analytics/scoped/network/mission-success/rate?" + values.Encode()
	call := func() (int, []byte) {
		t.Helper()
		r := f.request(t, "GET", path, nil)
		r.URL.Scheme = "https"
		r.URL.Host = strings.TrimPrefix(server.URL, "https://")
		r.RequestURI = ""
		res, err := hq.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		raw, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		return res.StatusCode, raw
	}
	status, raw := call()
	if status != 200 {
		t.Fatal(status, string(raw))
	}
	var envelope struct {
		Result analyticsadmin.RateRelease `json:"result"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope.Result.Rate != nil || envelope.Result.Suppression != "PARTIAL_COVERAGE" {
		t.Fatal("partial owner data became numeric rate", err)
	}
	if dir := os.Getenv("ADMIN_BRIDGE_CONTRACT_CAPTURE_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "scoped-analytics-rate.json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	complete.Store(true)
	status, raw = call()
	if status != 200 {
		t.Fatal(status, string(raw))
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope.Result.Rate == nil || *envelope.Result.Rate != .75 {
		t.Fatal("TEST_ONLY rate calculation failed", err)
	}
	for _, name := range []string{"counts permission", "rate permission", "rate flag", "rate contract", "scope"} {
		t.Run(name, func(t *testing.T) {
			before := calls.Load()
			originalClaims := f.claims
			originalPolicy := f.b.scopedAnalyticsPolicy
			flag := f.b.scopedAnalyticsRate
			defer func() {
				f.claims = originalClaims
				f.b.scopedAnalyticsPolicy = originalPolicy
				f.b.scopedAnalyticsRate = flag
			}()
			switch name {
			case "counts permission":
				f.claims.Permissions = []string{analyticsadmin.MissionRatePermission}
			case "rate permission":
				f.claims.Permissions = []string{analyticsadmin.MissionSuccessPermission}
			case "rate flag":
				f.b.scopedAnalyticsRate = false
			case "rate contract":
				bad := policyfixture.New(t, scope, "analytics-service", analyticsadmin.MissionSuccessPermission, "analytics.scoped-mission-success.v1:provider_id")
				f.b.scopedAnalyticsPolicy = bad.Engine
			case "scope":
				f.claims.Tenants = []string{"foreign"}
			}
			status, _ := call()
			if status == 200 || calls.Load() != before {
				t.Fatal("rate boundary reached owner", status, calls.Load()-before)
			}
		})
	}
}
