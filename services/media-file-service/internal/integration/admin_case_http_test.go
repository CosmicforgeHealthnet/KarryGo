package integration_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	mediahttp "cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/http"
	"cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/usecases"
	"cosmicforge/logistics/shared/go/adminmedia"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
)

func wireCaseActor(a usecases.CaseActor) adminmedia.Actor {
	d := a.Delegation
	return adminmedia.Actor{Product: d.Product, Staff: d.Staff, Roles: d.Roles, Permissions: d.Permissions, Markets: d.Markets, Zones: d.Zones, Organizations: d.Organizations, Tenants: d.Tenants, ExpiresAt: a.ExpiresAt}
}
func TestPostgresMediaCasePrivateHTTPAndTypedClientEnforceActorScopeAndReplay(t *testing.T) {
	f := newCaseFixture(t)
	key := bytes.Repeat([]byte{87}, 32)
	actor := wireCaseActor(f.actor)
	h := mediahttp.NewAdminCaseHandler(f.access, f.media.pool, key, true, true)
	engine := gin.New()
	h.Register(engine)
	server := httptest.NewServer(engine)
	defer server.Close()
	client, err := adminmedia.NewClient(server.URL, key)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	grant, err := client.Issue(context.Background(), actor, f.request)
	if err != nil {
		t.Fatal(err)
	}
	// Section 19: a copied grant link/token used by unrelated staff or under
	// another tenant is refused BEFORE first use and does not spend the
	// grant; the legitimate holder then redeems it exactly once.
	other := actor
	other.Staff = "staff-2"
	if body, err := client.Read(context.Background(), other, f.request, grant.ID, grant.Token); err == nil || len(body) > 0 {
		t.Fatal("another staff redeemed private case grant")
	}
	q := f.request
	q.Scope.Tenant = "another-tenant"
	if body, err := client.Read(context.Background(), actor, q, grant.ID, grant.Token); err == nil || len(body) > 0 {
		t.Fatal("tenant argument changed native authority")
	}
	if body, err := client.Read(context.Background(), actor, f.request, grant.ID, grant.Token); err != nil || len(body) == 0 {
		t.Fatal("private case transport failed", err)
	}
	if body, err := client.Read(context.Background(), actor, f.request, grant.ID, grant.Token); !errors.Is(err, adminmedia.ErrNotFound) || len(body) > 0 {
		t.Fatal("one-time private case grant redeemed twice over transport", err)
	}
	badActor := actor
	badActor.Permissions = []string{adminmedia.Permission}
	if _, err := client.Issue(context.Background(), badActor, f.request); !errors.Is(err, adminmedia.ErrDenied) {
		t.Fatal("missing source permission reached object", err)
	}
	raw, _ := json.Marshal(adminmedia.IssueRequest{Actor: actor, Request: f.request})
	request := func(body, secret []byte, caller, query string) *http.Request {
		t.Helper()
		r := httptest.NewRequest("POST", adminmedia.GrantPath+query, bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if err := serviceauth.SignRequest(r, caller, secret, body, time.Now()); err != nil {
			t.Fatal(err)
		}
		return r
	}
	call := func(r *http.Request) int {
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, r)
		if w.Code != 200 && bytes.Contains(w.Body.Bytes(), []byte("token")) {
			t.Error("denied transport disclosed credential")
		}
		return w.Code
	}
	r := request(raw, key, adminmedia.Caller, "")
	replay := r.Clone(context.Background())
	replay.Body = io.NopCloser(bytes.NewReader(raw))
	if code := call(r); code != 200 {
		t.Fatal(code)
	}
	if code := call(replay); code != 409 {
		t.Fatal("durable replay accepted", code)
	}
	if code := call(request(raw, []byte(testSecret), adminmedia.Caller, "")); code != 401 {
		t.Fatal("owner key acquired case audience", code)
	}
	if code := call(request(raw, key, testCaller, "")); code != 401 {
		t.Fatal("owner persona acquired case audience", code)
	}
	if code := call(request(raw, key, adminmedia.Caller, "?tenant=another")); code != 400 {
		t.Fatal("query scope accepted", code)
	}
	duplicate := append(append([]byte{}, raw[:len(raw)-1]...), []byte(`,"actor":{}}`)...)
	alias := append(append([]byte{}, raw[:len(raw)-1]...), []byte(`,"Actor":{}}`)...)
	unknown := append(append([]byte{}, raw[:len(raw)-1]...), []byte(`,"owner_url":"https://untrusted.invalid"}`)...)
	for _, body := range [][]byte{duplicate, alias, unknown} {
		if code := call(request(body, key, adminmedia.Caller, "")); code != 400 {
			t.Fatal("ambiguous/unknown private input accepted", code)
		}
	}
	if code := call(request(bytes.Repeat([]byte{'x'}, adminmedia.MaxBody+1), key, adminmedia.Caller, "")); code != 413 {
		t.Fatal("unbounded body accepted", code)
	}
	// A read kill does not prevent authenticated withdrawal of one's own grant.
	killed := mediahttp.NewAdminCaseHandler(f.access, f.media.pool, key, false, true)
	killedEngine := gin.New()
	killed.Register(killedEngine)
	killServer := httptest.NewServer(killedEngine)
	defer killServer.Close()
	killClient, err := adminmedia.NewClient(killServer.URL, key)
	if err != nil {
		t.Fatal(err)
	}
	defer killClient.Close()
	unspent, err := client.Issue(context.Background(), actor, f.request)
	if err != nil {
		t.Fatal(err)
	}
	if body, err := killClient.Read(context.Background(), actor, f.request, unspent.ID, unspent.Token); err == nil || len(body) > 0 {
		t.Fatal("transport read kill bypassed")
	}
	f.authority.denied = true
	if err := killClient.Revoke(context.Background(), actor, unspent.ID, "TEST_ONLY_WITHDRAWAL"); err != nil {
		t.Fatal("closed case prevented withdrawal", err)
	}
	f.authority.denied = false
	if body, err := client.Read(context.Background(), actor, f.request, unspent.ID, unspent.Token); err == nil || len(body) > 0 {
		t.Fatal("withdrawn case token redeemed")
	}
}
func TestPostgresMediaCaseRuntimeSeparatesKeysAndCannotInventAdaptersOrPolicy(t *testing.T) {
	f := newCaseFixture(t)
	key := bytes.Repeat([]byte{88}, 32)
	file := filepath.Join(t.TempDir(), "case.key")
	if err := os.WriteFile(file, []byte(base64.RawURLEncoding.EncodeToString(key)), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MEDIA_ADMIN_CASE_READ_ENABLED", "true")
	t.Setenv("MEDIA_ADMIN_CASE_REVOKE_ENABLED", "true")
	t.Setenv("MEDIA_ADMIN_CASE_SECRET_FILE", file)
	t.Setenv("MEDIA_ADMIN_CASE_POLICY_FILE", "")
	t.Setenv("MEDIA_ADMIN_CASE_POLICY_KEYS_FILE", "")
	h, err := mediahttp.AdminCaseRuntime(context.Background(), f.media.service, f.media.pool, nil, nil, nil)
	if err != nil {
		t.Fatal("missing approvals should block capability", err)
	}
	engine := gin.New()
	h.Register(engine)
	server := httptest.NewServer(engine)
	defer server.Close()
	client, err := adminmedia.NewClient(server.URL, key)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.Issue(context.Background(), wireCaseActor(f.actor), f.request); !errors.Is(err, adminmedia.ErrUnavailable) {
		t.Fatal("runtime manufactured policy/source adapter", err)
	}
	if _, err := mediahttp.AdminCaseRuntime(context.Background(), f.media.service, f.media.pool, serviceauth.Secrets{testCaller: key}, nil, nil); err == nil {
		t.Fatal("owner workload secret reused for HQ case access")
	}
	if _, err := mediahttp.AdminCaseRuntime(context.Background(), f.media.service, f.media.pool, nil, key, nil); err == nil {
		t.Fatal("download signing key reused for case audience")
	}
}
func TestMediaCaseRuntimeOffReadsNoCredentialOrPolicyFiles(t *testing.T) {
	t.Setenv("MEDIA_ADMIN_CASE_READ_ENABLED", "false")
	t.Setenv("MEDIA_ADMIN_CASE_REVOKE_ENABLED", "false")
	t.Setenv("MEDIA_ADMIN_CASE_SECRET_FILE", "missing")
	h, err := mediahttp.AdminCaseRuntime(context.Background(), nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	h.Register(engine)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest("POST", adminmedia.GrantPath, nil))
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
}
