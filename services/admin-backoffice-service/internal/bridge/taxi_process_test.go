package bridge

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/services/admin-backoffice-service/internal/clients"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/config"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/persistence"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/testutil"
	"cosmicforge/logistics/shared/go/admincontext"
	contextfixture "cosmicforge/logistics/shared/go/admincontext/testfixture"
	"cosmicforge/logistics/shared/go/adminowner"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This test starts the actual Taxi main binary. Its owner queries, HMAC nonce
// store, audit and migrations run against a separate real PostgreSQL schema.
// The HQ side uses an actual mTLS connection to the bridge and its real ledger.
func TestTaxiOwnerProcessPostgresMTLS(t *testing.T) {
	binary := os.Getenv("ADMIN_TAXI_TEST_BINARY")
	dsn := os.Getenv("TAXI_ADMIN_TEST_DATABASE_URL")
	if binary == "" || dsn == "" {
		t.Skip("ADMIN_TAXI_TEST_BINARY and TAXI_ADMIN_TEST_DATABASE_URL required for real owner process certification")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil || !strings.Contains(cfg.ConnConfig.Database, "test") {
		t.Fatal("disposable Taxi database required")
	}
	root, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	schema := "taxi_bridge_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = root.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer root.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
	ownerURL, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	params := ownerURL.Query()
	params.Set("search_path", schema+",public")
	ownerURL.RawQuery = params.Encode()
	owner, err := pgxpool.New(ctx, ownerURL.String())
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	_, file, _, _ := runtime.Caller(0)
	taxiRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "../../../driver-taxi-service"))
	paths, err := filepath.Glob(filepath.Join(taxiRoot, "migrations/*.sql"))
	if err != nil || len(paths) == 0 {
		t.Fatal("owner migrations not found")
	}
	sort.Strings(paths)
	for _, p := range paths {
		if strings.HasSuffix(p, ".down.sql") {
			continue
		}
		raw, e := os.ReadFile(p)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = owner.Exec(ctx, string(raw)); e != nil {
			t.Fatalf("owner migration %s: %v", filepath.Base(p), e)
		}
	}
	var mid, zid string
	if err = owner.QueryRow(ctx, `INSERT INTO markets(country,name,code) VALUES('test','test','NG') RETURNING id::text`).Scan(&mid); err != nil {
		t.Fatal(err)
	}
	if err = owner.QueryRow(ctx, `INSERT INTO operating_zones(market_id,name,code,center_lat,center_lng) VALUES($1,'test','Z1',0,0) RETURNING id::text`, mid).Scan(&zid); err != nil {
		t.Fatal(err)
	}
	id := uuid.NewString()
	at := time.Now().UTC().Add(-time.Hour)
	if _, err = owner.Exec(ctx, `INSERT INTO taxi_bookings(id,customer_id,market_id,operating_zone_id,organization_id,is_corporate_managed,membership_id,fleet_assignment_id,status,source_context,created_at,updated_at,ride_notes) VALUES($1::uuid,$2,$3,$4,'org-1',true,'member-test',$1::text,'completed','FLEET_ASSIGNMENT',$5,$5,'SECRET_NOTES')`, id, uuid.NewString(), mid, zid, at); err != nil {
		t.Fatal(err)
	}
	secret := bytes.Repeat([]byte{51}, 32)
	secretFile := filepath.Join(t.TempDir(), "owner-secret")
	if err = os.WriteFile(secretFile, []byte(base64.RawURLEncoding.EncodeToString(secret)), 0600); err != nil {
		t.Fatal(err)
	}
	policy := testfixture.NewTaxi(t, adminpolicy.Scope{Product: "karryam", Market: "NG", Zone: "Z1", Organization: "org-1", Tenant: "org-1"})
	providerPolicy := testfixture.New(t, policy.Document.Rules[0].Scopes[0], "driver-taxi-service", "taxi.providers.read", "taxi.admin-resources.v1")
	policy.Document.Rules = append(policy.Document.Rules, providerPolicy.Document.Rules[0])
	policy.Sign(t)
	sourceIssuer := contextfixture.New(t, "driver-taxi-service")
	sourceKeys := map[string]string{}
	for id, key := range sourceIssuer.Authority.Keys {
		sourceKeys[id] = base64.RawURLEncoding.EncodeToString(key)
	}
	sourceKeysRaw, _ := json.Marshal(sourceKeys)
	sourceKeysFile := filepath.Join(t.TempDir(), "test-only-source-owner-keys.json")
	if err = os.WriteFile(sourceKeysFile, sourceKeysRaw, 0600); err != nil {
		t.Fatal(err)
	}
	publisherSecret := bytes.Repeat([]byte{52}, 32)
	publisherSecretFile := filepath.Join(t.TempDir(), "test-only-owner-publisher-secret")
	if err = os.WriteFile(publisherSecretFile, []byte(base64.RawURLEncoding.EncodeToString(publisherSecret)), 0600); err != nil {
		t.Fatal(err)
	}
	providerID := uuid.NewString()
	if _, err = owner.Exec(ctx, `INSERT INTO taxi_providers(id,first_name,license_expiry_date) VALUES($1,'PRIVATE_PROVIDER_NAME','2030-01-01')`, providerID); err != nil {
		t.Fatal(err)
	}
	policyFile := filepath.Join(t.TempDir(), "test-only-policy.json")
	keysFile := filepath.Join(t.TempDir(), "test-only-policy-keys.json")
	if err = os.WriteFile(policyFile, policy.Envelope, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(keysFile, policy.Keys, 0600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	listener.Close()
	logFile, err := os.CreateTemp(t.TempDir(), "owner-log")
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()
	cmd := exec.CommandContext(ctx, binary)
	cmd.Dir = taxiRoot
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = append(os.Environ(), "APP_ENV=test", "HTTP_ADDR="+addr, "TAXI_DATABASE_URL="+ownerURL.String(), "TAXI_MIGRATION=false", "TAXI_ADMIN_READ_ENABLED=true", "TAXI_ADMIN_READ_SECRET_FILE="+secretFile, "TAXI_PROVIDER_TOKEN_SECRET=owner-fixture-only-token", "TAXI_PROVIDER_REFRESH_SECRET=owner-fixture-only-refresh", "TAXI_PROVIDER_OTP_SECRET=owner-fixture-only-otp", "TAXI_CUSTOMER_TOKEN_SECRET=owner-fixture-only-customer")
	cmd.Env = append(cmd.Env, "TAXI_ADMIN_ACCESS_POLICY_FILE="+policyFile, "TAXI_ADMIN_ACCESS_POLICY_KEYS_FILE="+keysFile)
	cmd.Env = append(cmd.Env, "TAXI_ADMIN_RESOURCE_READ_ENABLED=true", "TAXI_ADMIN_RESOURCE_SECRET_FILE="+secretFile, "TAXI_ADMIN_CONTEXT_KEYS_FILE="+sourceKeysFile, "TAXI_ADMIN_CONTEXT_PUBLISH_ENABLED=true", "TAXI_ADMIN_CONTEXT_PUBLISH_SECRET_FILE="+publisherSecretFile)
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cmd.Process.Kill(); cmd.Wait() }()
	// Bounded readiness polling is local test synchronization, not a time-based
	// assertion of lifecycle completion. Failure includes only the test log.
	origin := "http://" + addr
	ready := false
	probe := http.Client{Timeout: 200 * time.Millisecond}
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); {
		res, e := probe.Get(origin + "/health")
		if e == nil {
			io.Copy(io.Discard, res.Body)
			res.Body.Close()
			if res.StatusCode == 200 {
				ready = true
				break
			}
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}
	if !ready {
		log, _ := os.ReadFile(logFile.Name())
		t.Fatalf("Taxi did not start: %s", log)
	}
	f := newFixture(t)
	f.claims.Zones = []string{"Z1"}
	f.claims.Tenants = []string{"org-1"}
	pool := testutil.Database(t)
	pair, leaf, roots := workloadCertificate(t, f.b.trust.WorkloadURI, false)
	f.cert = leaf
	fingerprint := sha256.Sum256(leaf.Raw)
	if _, err = pool.Exec(ctx, `INSERT INTO admin_remote_principals(principal,certificate_sha256,workload_uri,product,audience,expires_at) VALUES($1,$2,$3,'karryam',$4,$5)`, config.HQPrincipal, hex.EncodeToString(fingerprint[:]), f.b.trust.WorkloadURI, f.b.trust.Audience, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	b, err := New(&persistence.Store{Pool: pool}, f.b.trust, true)
	if err != nil {
		t.Fatal(err)
	}
	taxi, err := clients.NewTaxi(origin, secret)
	if err != nil {
		t.Fatal(err)
	}
	defer taxi.Close()
	if err = adminpolicy.PinRevision(ctx, pool, "admin_policy_revisions", policy.Engine); err != nil {
		t.Fatal(err)
	}
	if err = b.ConfigureTaxi(taxi, []string{"staff"}, policy.Engine); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(b)
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots}
	server.StartTLS()
	defer server.Close()
	hq := server.Client()
	hq.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{pair}
	target, _ := url.Parse(server.URL)
	q := adminread.Query{Scope: adminread.Scope{Market: "NG", Zone: "Z1", Organization: "org-1", Tenant: "org-1"}, Start: at.Add(-time.Hour).Truncate(time.Second), End: at.Add(time.Hour).Truncate(time.Second), Limit: 10, Purpose: "case-1"}
	values := q.Values()
	for _, key := range []string{"staff", "role", "policy_id", "policy_version", "policy_checksum"} {
		values.Del(key)
	}
	request := f.request(t, "GET", "/taxi/rides/"+id+"?"+values.Encode(), nil)
	request.URL.Scheme = target.Scheme
	request.URL.Host = target.Host
	request.RequestURI = ""
	res, err := hq.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || strings.Contains(string(raw), "SECRET") {
		t.Fatalf("HQ read=%d %s", res.StatusCode, raw)
	}
	var body struct {
		Result adminread.TaxiSnapshot `json:"result"`
	}
	if json.Unmarshal(raw, &body) != nil || len(body.Result.Items) != 1 || body.Result.Items[0].ID != id {
		t.Fatalf("owner source row missing: %s", raw)
	}
	var sourceAudit, bridgeAudit int
	if err = owner.QueryRow(ctx, `SELECT count(*) FROM taxi_admin_read_audit WHERE staff_ref='staff-1' AND target_ref=$1`, id).Scan(&sourceAudit); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM admin_audit_entries WHERE staff_ref='staff-1' AND zone='Z1' AND target_ref=$1 AND outcome='AUTHORIZED'`, id).Scan(&bridgeAudit); err != nil {
		t.Fatal(err)
	}
	if sourceAudit != 1 || bridgeAudit != 1 {
		t.Fatalf("source/bridge audit=%d/%d", sourceAudit, bridgeAudit)
	}
	if output := os.Getenv("ADMIN_BRIDGE_CONTRACT_CAPTURE_DIR"); output != "" {
		if err = os.MkdirAll(output, 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(output, "taxi.json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	// Complete the new owner workflow through the real binary: source-fact
	// preparation, independent owner approval, publication, HQ read, revocation.
	ownerCall := func(method, path string, payload []byte) (int, []byte) {
		t.Helper()
		req, e := http.NewRequestWithContext(ctx, method, origin+path, bytes.NewReader(payload))
		if e != nil {
			t.Fatal(e)
		}
		if method == "POST" {
			req.Header.Set("Content-Type", "application/json")
		}
		if e = serviceauth.SignRequest(req, adminowner.ContextPublisher, publisherSecret, payload, time.Now()); e != nil {
			t.Fatal(e)
		}
		res, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		raw, e := io.ReadAll(res.Body)
		if e != nil {
			t.Fatal(e)
		}
		return res.StatusCode, raw
	}
	status, prepared := ownerCall("GET", "/internal/taxi/v1/admin/authorization-contexts/bindings/providers/"+providerID+"?purpose=case-1", nil)
	var preparation struct {
		State   string                     `json:"state"`
		Binding admincontext.RecordBinding `json:"binding"`
	}
	if status != 200 || json.Unmarshal(prepared, &preparation) != nil || preparation.State != "DRAFT_PENDING_OWNER_APPROVAL" || preparation.Binding.Canonical.Organization != "" || preparation.Binding.Canonical.Tenant != "" {
		t.Fatalf("source preparation invented authority: %d %s", status, prepared)
	}
	resources, e := clients.NewResources("driver-taxi-service", origin, secret)
	if e != nil {
		t.Fatal(e)
	}
	defer resources.Close()
	if e = b.ConfigureResources(resources, policy.Engine); e != nil {
		t.Fatal(e)
	}
	resourceRead := func(want int) []byte {
		t.Helper()
		req := f.request(t, "GET", "/taxi/resources/providers/"+providerID+"?"+values.Encode(), nil)
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.RequestURI = ""
		res, e := hq.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		raw, e := io.ReadAll(res.Body)
		if e != nil {
			t.Fatal(e)
		}
		if res.StatusCode != want || strings.Contains(string(raw), "PRIVATE_PROVIDER_NAME") {
			t.Fatalf("source-context resource read: %d %s", res.StatusCode, raw)
		}
		return raw
	}
	resourceRead(404)
	approval := sourceIssuer.Approval("test-only-provider-context", "providers", providerID, "taxi.providers.read", preparation.Binding, policy.Document.Rules[0].Scopes[0])
	if status, raw := ownerCall("POST", "/internal/taxi/v1/admin/authorization-contexts", sourceIssuer.Sign(t, approval)); status != 200 {
		t.Fatalf("owner publication: %d %s", status, raw)
	}
	resourceWire := resourceRead(200)
	if output := os.Getenv("ADMIN_BRIDGE_CONTRACT_CAPTURE_DIR"); output != "" {
		if e = os.WriteFile(filepath.Join(output, "resources.json"), resourceWire, 0600); e != nil {
			t.Fatal(e)
		}
	}
	approval.Revision++
	approval.State = "REVOKED"
	if status, raw := ownerCall("POST", "/internal/taxi/v1/admin/authorization-contexts", sourceIssuer.Sign(t, approval)); status != 200 {
		t.Fatalf("owner revocation: %d %s", status, raw)
	}
	resourceRead(404)
}
