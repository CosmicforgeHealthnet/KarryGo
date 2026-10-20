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
	"cosmicforge/logistics/services/admin-backoffice-service/internal/features/customerreviewops"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/persistence"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/testutil"
	"cosmicforge/logistics/shared/go/adminpolicy"
	policyfixture "cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/customerreview"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/google/uuid"
)

func TestPostgresCustomerReviewBridgeTypedOwnerBoundaryAndRecovery(t *testing.T) {
	db := testutil.Database(t)
	f := newFixture(t)
	f.b.ConfigureDomainFlags(config.DomainFlags{RemoteWrites: true, CustomerWrites: true})
	f.claims.Zones = []string{"Z1"}
	f.claims.Tenants = []string{"org-1"}
	pair, leaf, roots := workloadCertificate(t, f.b.trust.WorkloadURI, false)
	f.cert = leaf
	f.b.repo = &persistence.Store{Pool: db}
	fingerprint := sha256.Sum256(leaf.Raw)
	if _, err := db.Exec(context.Background(), `INSERT INTO admin_remote_principals(principal,certificate_sha256,workload_uri,product,audience,expires_at) VALUES($1,$2,$3,'karryam',$4,$5)`, config.HQPrincipal, hex.EncodeToString(fingerprint[:]), f.b.trust.WorkloadURI, f.b.trust.Audience, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	bridgeServer := httptest.NewUnstartedServer(f.b)
	bridgeServer.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots}
	bridgeServer.StartTLS()
	defer bridgeServer.Close()
	hqClient := bridgeServer.Client()
	hqClient.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{pair}
	call := func(method, path string, body []byte) *httptest.ResponseRecorder {
		t.Helper()
		request := f.request(t, method, path, body)
		request.URL.Scheme = "https"
		request.URL.Host = strings.TrimPrefix(bridgeServer.URL, "https://")
		request.RequestURI = ""
		response, err := hqClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		raw, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		out := httptest.NewRecorder()
		out.WriteHeader(response.StatusCode)
		out.Write(raw)
		return out
	}
	sc := adminpolicy.Scope{Product: "karryam", Market: "NG", Zone: "Z1", Organization: "org-1", Tenant: "org-1"}
	p := policyfixture.New(t, sc, customerreview.Owner, customerreview.RequestCapability, customerreview.Contract)
	rule := p.Document.Rules[0]
	rule.Capability = customerreview.ReadCapability
	rule.Permission = customerreview.ReadCapability
	rule.SourceContract = customerreview.StateContract
	p.Document.Rules = append(p.Document.Rules, rule)
	p.Sign(t)
	writeKey := []byte(strings.Repeat("w", 32))
	readKey := []byte(strings.Repeat("r", 32))
	var posts, gets atomic.Int32
	var receipt customerreview.Receipt
	owner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := readKey
		if r.Method == "POST" {
			key = writeKey
		}
		if _, err := serviceauth.NewVerifier(serviceauth.Secrets{adminread.Caller: key}, time.Minute).VerifyRequest(r); err != nil {
			t.Error(err)
			w.WriteHeader(401)
			return
		}
		if r.Header.Get("Authorization") != "" || r.Header.Get("X-HQ-Key-ID") != "" {
			t.Error("HQ credentials leaked")
		}
		q, err := adminread.Parse(r.URL.Query(), time.Now())
		if err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		if q.Scope.Tenant != "org-1" || q.Scope.Zone != "Z1" || q.Staff != "staff-1" || q.PolicyChecksum != p.Engine.Checksum() {
			t.Error("delegation lost")
		}
		if r.Method == "POST" {
			posts.Add(1)
			var input customerreview.Request
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Error(err)
			}
			receipt = customerreview.Receipt{ContractVersion: customerreview.Contract, Status: customerreview.Recorded, RequestID: input.RequestID, CustomerID: input.CustomerID, ReviewRequestID: uuid.NewString(), IdempotencyKey: input.IdempotencyKey, IntentDigest: input.Digest(q.Staff, q.Purpose, sc), RecordedReviewRevision: input.ExpectedReviewRevision + 1, Scope: sc, RecordedAt: time.Now().UTC()}
			// The native source committed; its response was lost.
			w.WriteHeader(503)
			return
		}
		gets.Add(1)
		if r.URL.Path != customerreview.Path+"/"+receipt.CustomerID+"/"+receipt.IdempotencyKey {
			t.Error("readback changed original identity")
		}
		json.NewEncoder(w).Encode(receipt)
	}))
	defer owner.Close()
	client, err := customerreview.NewClient(owner.URL, writeKey, readKey)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	service := &customerreviewops.Service{DB: db, Owner: client, Policy: p.Engine, WriteEnabled: true, ReadEnabled: true}
	if err := f.b.ConfigureCustomerReview(service); err != nil {
		t.Fatal(err)
	}
	availability := map[string]bool{}
	f.b.customerReviewAvailability(availability, f.claims.Roles, f.claims.Zones, f.claims.Tenants)
	if !availability[customerreview.RequestCapability] || !availability[customerreview.ReadCapability] {
		t.Fatal("approved test-only rollout not discoverable")
	}
	q := adminread.Query{Scope: adminread.Scope{Market: "NG", Zone: "Z1", Organization: "org-1", Tenant: "org-1"}, Purpose: "case-1", Start: time.Now().UTC().Add(-time.Hour).Truncate(time.Second), End: time.Now().UTC().Truncate(time.Second), Limit: 1}
	values := q.Values()
	for _, key := range []string{"staff", "role", "policy_id", "policy_version", "policy_checksum"} {
		values.Del(key)
	}
	suffix := "?" + values.Encode()
	input := customerreview.Request{ContractVersion: customerreview.Contract, RequestID: uuid.NewString(), CustomerID: uuid.NewString(), ExpectedReviewRevision: 1, IdempotencyKey: uuid.NewString(), ReasonCode: "ACCOUNT_REVIEW", RequestedAt: time.Now().UTC().Add(-time.Second)}
	body, _ := json.Marshal(input)
	w := call("POST", "/customer/review-requests"+suffix, body)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"UNKNOWN_OUTCOME"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	if dir := os.Getenv("ADMIN_BRIDGE_CONTRACT_CAPTURE_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "customer-review-unknown.json"), w.Body.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
	service.WriteEnabled = false
	f.b.ConfigureDomainFlags(config.DomainFlags{})
	f.b.customerReviewAvailability(availability, f.claims.Roles, f.claims.Zones, f.claims.Tenants)
	if availability[customerreview.RequestCapability] || !availability[customerreview.ReadCapability] {
		t.Fatal("write kill hid original readback or left writes discoverable")
	}
	w = call("GET", "/customer/review-requests/"+input.RequestID+suffix, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"status":"CONFIRMED"`) {
		t.Fatal(w.Code, w.Body.String())
	}
	if posts.Load() != 1 || gets.Load() != 1 {
		t.Fatal("automatic resubmission")
	}
	if dir := os.Getenv("ADMIN_BRIDGE_CONTRACT_CAPTURE_DIR"); dir != "" {
		if err := os.WriteFile(filepath.Join(dir, "customer-review-confirmed.json"), w.Body.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"zone", "tenant", "staff_injection", "permission", "policy", "disabled", "global_write_kill", "customer_write_kill"} {
		t.Run(name, func(t *testing.T) {
			other := newFixture(t)
			other.b.ConfigureDomainFlags(config.DomainFlags{RemoteWrites: true, CustomerWrites: true})
			other.claims.Zones = []string{"Z1"}
			other.claims.Tenants = []string{"org-1"}
			clone := *service
			clone.WriteEnabled = true
			if err := other.b.ConfigureCustomerReview(&clone); err != nil {
				t.Fatal(err)
			}
			target := "/customer/review-requests" + suffix
			want := 403
			switch name {
			case "zone":
				other.claims.Zones = []string{"Z2"}
			case "tenant":
				other.claims.Tenants = []string{"org-2"}
			case "staff_injection":
				target += "&staff=forged"
				want = 400
			case "permission":
				other.claims.Permissions = []string{"bridge.health.read"}
			case "policy":
				clone.Policy = nil
				want = 503
			case "disabled":
				clone.WriteEnabled = false
				want = 503
			case "global_write_kill":
				other.b.ConfigureDomainFlags(config.DomainFlags{CustomerWrites: true})
				want = 503
			case "customer_write_kill":
				other.b.ConfigureDomainFlags(config.DomainFlags{RemoteWrites: true})
				want = 503
			}
			res := invoke(other.b, other.request(t, "POST", target, body))
			if res.Code != want {
				t.Fatal(name, res.Code, res.Body.String())
			}
			if posts.Load() != 1 {
				t.Fatal("denied caller reached source")
			}
		})
	}
}
