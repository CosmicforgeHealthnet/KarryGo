package bridge

import (
	"context"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/config"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/features/operations"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/persistence"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/security"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/testutil"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"github.com/google/uuid"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestRealPostgresMTLSIntegration(t *testing.T) {
	f := newFixture(t)
	pool := testutil.Database(t)
	ctx := context.Background()
	pair, leaf, roots := workloadCertificate(t, f.b.trust.WorkloadURI, false)
	f.cert = leaf
	hash := sha256.Sum256(leaf.Raw)
	if _, err := pool.Exec(ctx, `INSERT INTO admin_remote_principals(principal,certificate_sha256,workload_uri,product,audience,expires_at) VALUES($1,$2,$3,'karryam',$4,$5)`, config.HQPrincipal, hex.EncodeToString(hash[:]), f.b.trust.WorkloadURI, f.b.trust.Audience, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	b, err := New(&persistence.Store{Pool: pool}, f.b.trust, true)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(b)
	srv.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots}
	srv.StartTLS()
	defer srv.Close()
	client := srv.Client()
	client.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{pair}
	target, _ := url.Parse(srv.URL)
	call := func(r *http.Request) int {
		r.URL.Scheme = target.Scheme
		r.URL.Host = target.Host
		r.RequestURI = ""
		res, e := client.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		return res.StatusCode
	}
	request := f.request(t, "GET", "/capabilities"+scopeQuery, nil)
	duplicate := request.Clone(ctx)
	duplicate.Body = http.NoBody
	if status := call(request); status != 200 {
		t.Fatal("complete authenticated read failed", status)
	}
	if status := call(duplicate); status != 409 {
		t.Fatal("durable replay admitted", status)
	}
	command := operations.Command{ContractVersion: "1.0", CommandID: uuid.NewString(), Product: "karryam", Action: "dispatch.provider.suspension.request", Target: operations.Target{Type: "dispatch_provider", ID: "provider-1", ExpectedVersion: 7}, Scope: security.Scope{Market: "NG", Organization: "org-1"}, ReasonCode: "SAFETY_REVIEW", Reason: "Case reference only", CaseReference: "case-1", IdempotencyKey: "idem-1", RequestedAt: time.Now().Add(-time.Second)}
	body, _ := json.Marshal(command)
	for i := 0; i < 2; i++ {
		if status := call(f.request(t, "POST", "/operations"+scopeQuery, body)); status != 503 {
			t.Fatal("unsupported command reported successful", status)
		}
	}
	var status string
	var observed *int64
	var count int
	if err = pool.QueryRow(ctx, `SELECT status,source_version FROM admin_delegated_operations WHERE id=$1`, command.CommandID).Scan(&status, &observed); err != nil || status != "REJECTED" || observed != nil {
		t.Fatal("fabricated owner state/version", status, observed, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM admin_delegated_operations`).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate command", count, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE admin_remote_principals SET revoked_at=now()`); err != nil {
		t.Fatal(err)
	}
	if status := call(f.request(t, "GET", "/capabilities"+scopeQuery, nil)); status != 401 {
		t.Fatal("certificate revocation missed on open connection", status)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM admin_audit_entries`).Scan(&count); err != nil || count != 5 {
		t.Fatal("privileged request missing audit", count, err)
	}
}
