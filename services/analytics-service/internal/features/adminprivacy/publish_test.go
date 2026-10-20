package adminprivacy

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/admincontext"
	contextfixture "cosmicforge/logistics/shared/go/admincontext/testfixture"
	"cosmicforge/logistics/shared/go/adminowner"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/analyticsadmin"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
)

func TestPostgresAnalyticsProducerPublicationBindsNativeFactsAndRevokes(t *testing.T) {
	db := lineageDB(t)
	ctx := context.Background()
	owner := "driver-taxi-service"
	issuer := contextfixture.New(t, owner)
	id := "published-native-completed"
	ingestLineage(t, db, id, "taxi.booking.completed", owner, "booking", id, 1, map[string]any{"provider_id": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"})
	reader := LineageReader{Owner: owner}
	kind, _ := LineageKind("mission-success", "provider_id")
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := reader.Binding(ctx, tx, kind, id)
	tx.Rollback(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, _, q, _ := privacyFixture(t)
	q.Capability = analyticsadmin.MissionSuccessPermission
	q.Start = time.Now().UTC().Add(-time.Hour)
	q.End = time.Now().UTC().Add(time.Hour)
	q.Filters["product_code"] = "TAXI_PASSENGER"
	approval := issuer.Approval("published-context", kind, id, "analytics.cohorts.mission-success.export", binding, q.Scope)
	key := bytes.Repeat([]byte{91}, 32)
	store := &admincontext.Store{DB: db, Reader: reader, Authority: issuer.Authority}
	p := &LineagePublisher{handlers: map[string]*adminowner.Handler{owner: {Owner: owner, DB: db, PublishEnabled: true, PublishSecret: key, PreparationReader: reader, Definitions: lineageDefinitions(), Contexts: store}}}
	engine := gin.New()
	p.Register(engine)
	request := func(path string, raw, secret []byte, caller string) *http.Request {
		t.Helper()
		r := httptest.NewRequest("POST", path, bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		if err := serviceauth.SignRequest(r, caller, secret, raw, time.Now()); err != nil {
			t.Fatal(err)
		}
		return r
	}
	call := func(r *http.Request) int {
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, r)
		if bytes.Contains(w.Body.Bytes(), []byte("binding_fingerprint")) {
			t.Error("private context leaked")
		}
		return w.Code
	}
	prepare := func(caller string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest("GET", lineagePublishBase+owner+"/bindings/"+kind+"/"+id+"?purpose=TEST_ONLY_REVIEW", nil)
		if err := serviceauth.SignRequest(r, caller, key, nil, time.Now()); err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, r)
		return w
	}
	if w := prepare(adminread.Caller); w.Code != 401 {
		t.Fatal("read caller prepared lineage", w.Code)
	}
	prepared := prepare(adminowner.ContextPublisher)
	if prepared.Code != 200 || !bytes.Contains(prepared.Body.Bytes(), []byte(binding.Fingerprint)) || bytes.Contains(prepared.Body.Bytes(), []byte("provider_id\":")) {
		t.Fatal("native binding preparation", prepared.Code, prepared.Body.String())
	}
	if dir := os.Getenv("ADMIN_OWNER_CUSTODY_CAPTURE_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "scoped-analytics-binding.json"), prepared.Body.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, sql := range []string{`UPDATE admin_owner_context_preparation_audit SET purpose_code='rewrite'`, `DELETE FROM admin_owner_context_preparation_audit`, `TRUNCATE admin_owner_context_preparation_audit`} {
		if _, err := db.Exec(ctx, sql); err == nil {
			t.Fatal("preparation evidence mutable")
		}
	}
	if _, err := db.Exec(ctx, `CREATE FUNCTION fail_lineage_preparation() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test-only'; END $$; CREATE TRIGGER fail_lineage_preparation BEFORE INSERT ON admin_owner_context_preparation_audit FOR EACH STATEMENT EXECUTE FUNCTION fail_lineage_preparation()`); err != nil {
		t.Fatal(err)
	}
	if w := prepare(adminowner.ContextPublisher); w.Code != 503 || bytes.Contains(w.Body.Bytes(), []byte("fingerprint")) {
		t.Fatal("preparation disclosed despite audit failure", w.Code)
	}
	if _, err := db.Exec(ctx, `DROP TRIGGER fail_lineage_preparation ON admin_owner_context_preparation_audit`); err != nil {
		t.Fatal(err)
	}
	raw := issuer.Sign(t, approval)
	path := lineagePublishBase + owner
	r := request(path, raw, key, adminowner.ContextPublisher)
	replay := r.Clone(ctx)
	replay.Body = http.NoBody
	if code := call(r); code != 200 {
		t.Fatal("signed native context rejected", code)
	}
	// Reconstruct the original signed request body; the exact nonce cannot be reused.
	replay.Body = io.NopCloser(bytes.NewReader(raw))
	if code := call(replay); code != 409 {
		t.Fatal("replay accepted", code)
	}
	source := &MissionSuccessSource{Authorities: map[string]admincontext.Authority{owner: issuer.Authority}, UnitField: "provider_id"}
	snapshot := func() error {
		t.Helper()
		tx, err := db.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		out, err := source.Snapshot(ctx, tx, q)
		if err == nil && !out.Partial {
			t.Fatal("publication invented complete coverage")
		}
		return err
	}
	if err := snapshot(); err != nil {
		t.Fatal("published native membership unavailable", err)
	}
	for _, tc := range []struct {
		name, path, caller string
		raw, key           []byte
		want               int
	}{
		{"read persona", path, adminread.Caller, raw, key, 401},
		{"wrong key", path, adminowner.ContextPublisher, raw, bytes.Repeat([]byte{92}, 32), 401},
		{"different owner", lineagePublishBase + "driver-hauling-service", adminowner.ContextPublisher, raw, key, 503},
		{"query injection", path + "?tenant=other", adminowner.ContextPublisher, raw, key, 400},
		{"oversize", path, adminowner.ContextPublisher, bytes.Repeat([]byte{'x'}, (32<<10)+1), key, 413},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if code := call(request(tc.path, tc.raw, tc.key, tc.caller)); code != tc.want {
				t.Fatal(code)
			}
		})
	}
	changed := approval
	changed.Revision = 2
	changed.BindingFingerprint = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if code := call(request(path, issuer.Sign(t, changed), key, adminowner.ContextPublisher)); code != 403 {
		t.Fatal("fabricated native binding accepted", code)
	}
	// History is the immutable publication evidence. An insert failure must
	// roll back the head update rather than partially accepting a revocation.
	if _, err := db.Exec(ctx, `CREATE FUNCTION reject_publication_insert() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test publication evidence failure'; END $$; CREATE TRIGGER fail_publication_history BEFORE INSERT ON admin_owner_context_history FOR EACH STATEMENT EXECUTE FUNCTION reject_publication_insert()`); err != nil {
		t.Fatal(err)
	}
	failedRevoke := approval
	failedRevoke.Revision = 2
	failedRevoke.State = "REVOKED"
	if code := call(request(path, issuer.Sign(t, failedRevoke), key, adminowner.ContextPublisher)); code != 403 {
		t.Fatal("history failure accepted", code)
	}
	if err := snapshot(); err != nil {
		t.Fatal("failed evidence insert changed live head", err)
	}
	if _, err := db.Exec(ctx, `DROP TRIGGER fail_publication_history ON admin_owner_context_history`); err != nil {
		t.Fatal(err)
	}
	revoked := approval
	revoked.Revision = 2
	revoked.State = "REVOKED"
	if code := call(request(path, issuer.Sign(t, revoked), key, adminowner.ContextPublisher)); code != 200 {
		t.Fatal("revocation rejected", code)
	}
	if err := snapshot(); err == nil {
		t.Fatal("revoked published allocation disclosed source membership")
	}
	if code := call(request(path, raw, key, adminowner.ContextPublisher)); code != 403 {
		t.Fatal("stale approval resurrected revoked context", code)
	}
	var n int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM admin_owner_context_history").Scan(&n); err != nil || n != 2 {
		t.Fatal("immutable publication evidence", n, err)
	}
}

func TestPostgresAnalyticsPublisherRuntimeIsIndependentAndSeparatesCredentials(t *testing.T) {
	t.Setenv("ANALYTICS_ADMIN_CONTEXT_PUBLISH_ENABLED", "false")
	t.Setenv("ANALYTICS_ADMIN_CONTEXT_PUBLISH_SECRETS_FILE", "missing")
	p, err := PublisherRuntime(context.Background(), nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	p.Register(engine)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest("POST", lineagePublishBase+lineageOwners[0], nil))
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
	db := lineageDB(t)
	issuer := contextfixture.New(t, lineageOwners[0])
	key := bytes.Repeat([]byte{93}, 32)
	dir := t.TempDir()
	keys := filepath.Join(dir, "keys.json")
	secrets := filepath.Join(dir, "secrets.json")
	public := map[string]map[string]string{lineageOwners[0]: {}}
	for id, k := range issuer.Authority.Keys {
		public[lineageOwners[0]][id] = base64.RawURLEncoding.EncodeToString(k)
	}
	write := func(path string, v any) {
		t.Helper()
		raw, _ := json.Marshal(v)
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(keys, public)
	write(secrets, map[string]string{lineageOwners[0]: base64.RawURLEncoding.EncodeToString(key)})
	t.Setenv("ANALYTICS_ADMIN_CONTEXT_PUBLISH_ENABLED", "true")
	t.Setenv("ANALYTICS_ADMIN_SOURCE_CONTEXT_KEYS_FILE", keys)
	t.Setenv("ANALYTICS_ADMIN_CONTEXT_PUBLISH_SECRETS_FILE", secrets)
	p, err = PublisherRuntime(context.Background(), db, nil, nil)
	if err != nil || len(p.handlers) != 1 {
		t.Fatal(err)
	}
	if _, err := PublisherRuntime(context.Background(), db, serviceauth.Secrets{"legacy": key}, nil); err == nil {
		t.Fatal("legacy key acquired publication authority")
	}
	if _, err := PublisherRuntime(context.Background(), db, nil, key); err == nil {
		t.Fatal("read key acquired publication authority")
	}
	write(secrets, map[string]string{"unknown-owner": base64.RawURLEncoding.EncodeToString(key)})
	if _, err := PublisherRuntime(context.Background(), db, nil, nil); err == nil {
		t.Fatal("mismatched publisher owner accepted")
	}
}
