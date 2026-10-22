package bridge

import (
	"bytes"
	"context"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/config"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/persistence"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/testutil"
	"cosmicforge/logistics/shared/go/admincase"
	"cosmicforge/logistics/shared/go/adminmedia"
	"cosmicforge/logistics/shared/go/adminpolicy"
	policyfixture "cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	"cosmicforge/logistics/shared/go/serviceauth"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"github.com/google/uuid"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type mediaBridgeFixture struct {
	client    *adminmedia.Client
	policy    *policyfixture.Fixture
	body      []byte
	grant     adminmedia.Grant
	calls     atomic.Int32
	malformed atomic.Bool
	outage    atomic.Bool
	auditRepo *fakeRepo
}

func newMediaBridgeFixture(t *testing.T) *mediaBridgeFixture {
	t.Helper()
	scope := adminpolicy.Scope{Product: "karryam", Market: "NG", Zone: "Z1", Organization: "org-1", Tenant: "org-1"}
	spec, _ := admincase.Find("customer-service", "review-work-items")
	p := policyfixture.New(t, scope, spec.Owner, spec.Permission, spec.SourceContract)
	p.Document.ApprovedOwners = append(p.Document.ApprovedOwners, "media-file-service")
	rule := p.Document.Rules[0]
	rule.Owner = "media-file-service"
	rule.Capability = adminmedia.Permission
	rule.Permission = adminmedia.Permission
	rule.SourceContract = adminmedia.Contract
	p.Document.Rules = append(p.Document.Rules, rule)
	p.Document.Media = &adminpolicy.MediaDisclosure{MaxGrantSeconds: 60, SourceCapabilities: []string{spec.Permission}}
	p.Sign(t)
	f := &mediaBridgeFixture{policy: p, grant: adminmedia.Grant{ContractVersion: adminmedia.Contract, ID: uuid.NewString(), MediaObjectID: "mobj-native", Token: strings.Repeat("c", 43)}}
	f.body, _ = json.Marshal(mediaCaseInput{CaseOwner: spec.Owner, CaseKind: spec.Kind, CaseReference: "native-case", SubjectType: "CUSTOMER", SubjectReference: "native-customer", MediaObjectID: f.grant.MediaObjectID})
	key := bytes.Repeat([]byte("m"), 32)
	owner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		raw, err := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(raw))
		if err != nil {
			t.Error(err)
		}
		if _, err := serviceauth.NewVerifier(serviceauth.Secrets{adminmedia.Caller: key}, time.Minute).VerifyRequest(r); err != nil {
			t.Error("native Media credential", err)
		}
		if r.Method != "POST" || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "" || r.Header.Get("X-HQ-Key-ID") != "" {
			t.Error("HQ credential/query forwarded")
		}
		if f.outage.Load() {
			w.WriteHeader(503)
			return
		}
		var actor adminmedia.Actor
		var request adminmedia.Request
		switch r.URL.Path {
		case adminmedia.GrantPath:
			var input adminmedia.IssueRequest
			if adminmedia.Decode(raw, &input) != nil {
				t.Error("invalid native issue wire")
			}
			actor, request = input.Actor, input.Request
		case adminmedia.GrantPath + "/" + f.grant.ID + "/content":
			var input adminmedia.ReadRequest
			if adminmedia.Decode(raw, &input) != nil || input.Token != f.grant.Token {
				t.Error("invalid native content wire")
			}
			actor, request = input.Actor, input.Request
		case adminmedia.GrantPath + "/" + f.grant.ID + "/revocations":
			var input adminmedia.RevokeRequest
			if adminmedia.Decode(raw, &input) != nil || input.ReasonCode != "CASE_CLOSED" {
				t.Error("invalid native revoke wire")
			}
			actor = input.Actor
		default:
			t.Error("arbitrary native path", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		if actor.Product != "karryam" || actor.Staff != "staff-1" || !actor.Valid(time.Now()) || actor.ExpiresAt.After(time.Now().Add(time.Minute+time.Second)) || !config.Contains(actor.Tenants, "org-1") || !config.Contains(actor.Zones, "Z1") {
			t.Error("native actor was not derived from verified HQ assertion", actor)
		}
		if strings.HasSuffix(r.URL.Path, "/revocations") {
			json.NewEncoder(w).Encode(map[string]string{"status": "REVOKED"})
			return
		}
		if request.Scope != scope || request.SourceCapability != spec.Permission || request.Purpose != "case-1" || request.SubjectReference != "native-customer" {
			w.WriteHeader(404)
			return
		}
		if f.malformed.Load() {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"private_url":"https://test.invalid/private"}`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/content") {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("X-Media-Case-Contract", adminmedia.Contract)
			w.Header().Set("X-Media-Object-ID", f.grant.MediaObjectID)
			w.Write([]byte("TEST_ONLY_PRIVATE_EVIDENCE"))
			return
		}
		grant := f.grant
		grant.ExpiresAt = time.Now().Add(20 * time.Second)
		json.NewEncoder(w).Encode(grant)
	}))
	t.Cleanup(owner.Close)
	client, err := adminmedia.NewClient(owner.URL, key)
	if err != nil {
		t.Fatal(err)
	}
	f.client = client
	t.Cleanup(client.Close)
	return f
}
func (m *mediaBridgeFixture) configure(t *testing.T, f *fixture) {
	t.Helper()
	f.claims.Zones = []string{"Z1"}
	f.claims.Tenants = []string{"org-1"}
	f.claims.Permissions = append(f.claims.Permissions, "customer.case.evidence.read")
	if err := f.b.ConfigureMediaCases(m.client, m.policy.Engine, true, true); err != nil {
		t.Fatal(err)
	}
}

const mediaQuery = "?market=NG&zone=Z1&organization=org-1&tenant=org-1&purpose=case-1"

func (m *mediaBridgeFixture) readBody(t *testing.T) []byte {
	t.Helper()
	var raw map[string]any
	if json.Unmarshal(m.body, &raw) != nil {
		t.Fatal("fixture")
	}
	raw["token"] = m.grant.Token
	out, _ := json.Marshal(raw)
	return out
}

func TestPostgresMediaCaseBridgeMTLSDelegationAndDisclosureAudit(t *testing.T) {
	m := newMediaBridgeFixture(t)
	f := newFixture(t)
	m.configure(t, f)
	db := testutil.Database(t)
	f.b.repo = &persistence.Store{Pool: db}
	pair, leaf, roots := workloadCertificate(t, f.b.trust.WorkloadURI, false)
	f.cert = leaf
	sum := sha256.Sum256(leaf.Raw)
	if _, err := db.Exec(context.Background(), `INSERT INTO admin_remote_principals(principal,certificate_sha256,workload_uri,product,audience,expires_at)VALUES($1,$2,$3,'karryam',$4,$5)`, config.HQPrincipal, hex.EncodeToString(sum[:]), f.b.trust.WorkloadURI, f.b.trust.Audience, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(f.b)
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots}
	server.StartTLS()
	defer server.Close()
	hq := server.Client()
	hq.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{pair}
	call := func(path string, body []byte) (int, []byte, http.Header) {
		t.Helper()
		r := f.request(t, "POST", path+mediaQuery, body)
		address, _ := url.Parse(server.URL)
		r.URL.Scheme, r.URL.Host = address.Scheme, address.Host
		r.RequestURI = ""
		response, err := hq.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		raw, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		return response.StatusCode, raw, response.Header
	}
	status, raw, _ := call("/media/case-grants", m.body)
	if status != 200 {
		t.Fatal(status, string(raw))
	}
	var envelope struct {
		Result adminmedia.Grant `json:"result"`
	}
	if json.Unmarshal(raw, &envelope) != nil || envelope.Result.Token != m.grant.Token {
		t.Fatal("grant contract", string(raw))
	}
	if dir := os.Getenv("ADMIN_BRIDGE_CONTRACT_CAPTURE_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "media-case-grant.json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	contentPath := "/media/case-grants/" + m.grant.ID + "/content"
	status, raw, headers := call(contentPath, m.readBody(t))
	if status != 200 || string(raw) != "TEST_ONLY_PRIVATE_EVIDENCE" || headers.Get("Content-Type") != "application/octet-stream" || headers.Get("Cache-Control") != "no-store" || headers.Get("Content-Disposition") == "" || headers.Get("X-Media-Object-ID") != m.grant.MediaObjectID {
		t.Fatal("private evidence response", status, string(raw), headers)
	}
	// Fail the second Bridge audit only: native content may be fetched into
	// memory, but no sensitive bytes can be released to HQ without the commit.
	if _, err := db.Exec(context.Background(), `CREATE FUNCTION fail_media_disclosure_audit()RETURNS trigger LANGUAGE plpgsql AS $$BEGIN IF NEW.request_type='media.case.content.read' AND NEW.outcome='CONFIRMED' THEN RAISE EXCEPTION 'test audit failure';END IF;RETURN NEW;END$$;CREATE TRIGGER fail_media_disclosure_audit BEFORE INSERT ON admin_audit_entries FOR EACH ROW EXECUTE FUNCTION fail_media_disclosure_audit()`); err != nil {
		t.Fatal(err)
	}
	status, raw, _ = call(contentPath, m.readBody(t))
	if status != 503 || bytes.Contains(raw, []byte("TEST_ONLY_PRIVATE_EVIDENCE")) {
		t.Fatal("Bridge audit failure disclosed bytes", status, string(raw))
	}
	if _, err := db.Exec(context.Background(), `DROP TRIGGER fail_media_disclosure_audit ON admin_audit_entries;DROP FUNCTION fail_media_disclosure_audit()`); err != nil {
		t.Fatal(err)
	}
	var audits string
	if err := db.QueryRow(context.Background(), `SELECT COALESCE(string_agg(row_to_json(a)::text,''),'')FROM admin_audit_entries a`).Scan(&audits); err != nil || strings.Contains(audits, m.grant.Token) || strings.Contains(audits, "TEST_ONLY_PRIVATE_EVIDENCE") {
		t.Fatal("credential/content entered Bridge audit", err)
	}
	f.b.mediaReads = false
	f.b.mediaPolicy = nil
	status, raw, _ = call("/media/case-grants/"+m.grant.ID+"/revocations", []byte(`{"reason_code":"CASE_CLOSED"}`))
	if status != 200 || !bytes.Contains(raw, []byte(`"status":"REVOKED"`)) {
		t.Fatal("own-grant revocation blocked by read/policy kill", status, string(raw))
	}
}

func TestMediaCaseBridgeDeniesUnapprovedScopeActorInjectionAndFailures(t *testing.T) {
	m := newMediaBridgeFixture(t)
	for _, mode := range []string{"zone", "tenant", "organization", "market", "permission", "source_permission", "role", "policy", "disclosure_policy", "read_kill", "actor_injection", "scope_injection", "capability_injection", "subject", "unknown_case", "duplicate_json", "query_actor", "query_duplicate", "query_unknown", "no_mtls", "audit", "rate", "native_outage", "malformed_native"} {
		t.Run(mode, func(t *testing.T) {
			f := newFixture(t)
			m.configure(t, f)
			path := "/media/case-grants" + mediaQuery
			body := m.body
			want := 403
			expectCall := false
			switch mode {
			case "zone":
				f.claims.Zones = []string{"Z2"}
			case "tenant":
				f.claims.Tenants = []string{"foreign"}
			case "organization":
				f.claims.Organizations = []string{"foreign"}
			case "market":
				f.claims.Markets = []string{"foreign"}
			case "permission":
				f.claims.Permissions = []string{"customer.case.evidence.read"}
			case "source_permission":
				f.claims.Permissions = []string{adminmedia.Permission}
			case "role":
				f.claims.Roles = []string{"other"}
			case "policy":
				f.b.mediaPolicy = nil
				want = 503
			case "disclosure_policy":
				p := policyfixture.New(t, m.policy.Document.Rules[0].Scopes[0], "media-file-service", adminmedia.Permission, adminmedia.Contract)
				f.b.mediaPolicy = p.Engine
				want = 503
			case "read_kill":
				f.b.mediaReads = false
				want = 503
			case "actor_injection", "scope_injection", "capability_injection", "duplicate_json":
				extra := `"actor":{"staff":"forged"}`
				if mode == "scope_injection" {
					extra = `"scope":{"tenant":"foreign"}`
				}
				if mode == "capability_injection" {
					extra = `"source_capability":"other"`
				}
				if mode == "duplicate_json" {
					extra = `"case_owner":"customer-service"`
				}
				body = append(append([]byte{}, body[:len(body)-1]...), []byte(","+extra+"}")...)
				want = 400
			case "subject":
				body = bytes.ReplaceAll(body, []byte("native-customer"), []byte("foreign-customer"))
				want = 404
				expectCall = true
			case "unknown_case":
				body = bytes.ReplaceAll(body, []byte("review-work-items"), []byte("invented-case"))
				want = 400
			case "query_actor":
				path += "&staff=forged"
				want = 400
			case "query_duplicate":
				path += "&tenant=org-1"
				want = 400
			case "query_unknown":
				path += "&origin=https://test.invalid"
				want = 400
			case "no_mtls":
				want = 401
			case "audit":
				f.repo.auditUnavailable = true
				want = 503
			case "rate":
				f.repo.rateDenied = true
				want = 429
			case "native_outage":
				m.outage.Store(true)
				defer m.outage.Store(false)
				want = 503
				expectCall = true
			case "malformed_native":
				m.malformed.Store(true)
				defer m.malformed.Store(false)
				want = 503
				expectCall = true
			}
			request := f.request(t, "POST", path, body)
			if mode == "no_mtls" {
				request.TLS = nil
			}
			before := m.calls.Load()
			out := invoke(f.b, request)
			if out.Code != want || m.calls.Load() > before && !expectCall || bytes.Contains(out.Body.Bytes(), []byte("test.invalid")) || bytes.Contains(out.Body.Bytes(), []byte(m.grant.Token)) {
				t.Fatal(mode, out.Code, out.Body.String(), "native calls", m.calls.Load()-before)
			}
		})
	}
	f := newFixture(t)
	m.configure(t, f)
	request := f.request(t, "POST", "/media/case-grants"+mediaQuery, m.body)
	copy := request.Clone(context.Background())
	copy.Body = io.NopCloser(bytes.NewReader(m.body))
	if out := invoke(f.b, request); out.Code != 200 {
		t.Fatal(out.Code, out.Body.String())
	}
	before := m.calls.Load()
	if out := invoke(f.b, copy); out.Code != 409 || m.calls.Load() != before {
		t.Fatal("HQ replay called Media", out.Code)
	}
}
