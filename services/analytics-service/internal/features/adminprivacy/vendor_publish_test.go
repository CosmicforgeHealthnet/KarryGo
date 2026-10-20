package adminprivacy

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/admincontext"
	contextfixture "cosmicforge/logistics/shared/go/admincontext/testfixture"
	"cosmicforge/logistics/shared/go/adminowner"
	"cosmicforge/logistics/shared/go/analyticsadmin"
	"cosmicforge/logistics/shared/go/networkanalytics"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
)

func TestPostgresVendorReadinessPublicationUsesIndependentNativeBranchBinding(t *testing.T) {
	db := lineageDB(t)
	ctx := context.Background()
	const owner = "vendor-service"
	const id = "native-vendor-readiness"
	const branch = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	ingestLineage(t, db, id, networkanalytics.EventBranchReadiness, owner, "vendor_order", "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", 1, map[string]any{"business_id": "cccccccc-cccc-4ccc-8ccc-cccccccccccc", "branch_id": branch, "on_time": true, "customer_name": "PRIVATE_CUSTOMER", "zone": "UNAPPROVED_OPERATIONAL_ZONE"})
	issuer := contextfixture.New(t, owner)
	key := bytes.Repeat([]byte{101}, 32)
	dir := t.TempDir()
	keysFile, secretsFile := filepath.Join(dir, "keys.json"), filepath.Join(dir, "secrets.json")
	keys := map[string]map[string]string{owner: {}}
	for id, k := range issuer.Authority.Keys {
		keys[owner][id] = base64.RawURLEncoding.EncodeToString(k)
	}
	write := func(path string, v any) {
		t.Helper()
		raw, _ := json.Marshal(v)
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(keysFile, keys)
	write(secretsFile, map[string]string{owner: base64.RawURLEncoding.EncodeToString(key)})
	t.Setenv("ANALYTICS_ADMIN_CONTEXT_PUBLISH_ENABLED", "true")
	t.Setenv("ANALYTICS_ADMIN_SOURCE_CONTEXT_KEYS_FILE", keysFile)
	t.Setenv("ANALYTICS_ADMIN_CONTEXT_PUBLISH_SECRETS_FILE", secretsFile)
	publisher, err := PublisherRuntime(ctx, db, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// The runtime pins production authority; the TEST_ONLY context below must
	// first fail before this isolated test installs its ephemeral authority.
	engine := gin.New()
	publisher.Register(engine)
	server := httptest.NewServer(engine)
	defer server.Close()
	client, err := analyticsadmin.NewPublisherClient(server.URL, owner, key)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	kind, _ := LineageKind("readiness", "branch_id")
	binding, err := client.PrepareBinding(ctx, kind, id, "TEST_ONLY_READINESS_REVIEW")
	if err != nil {
		t.Fatal(err)
	}
	if binding.Canonical.Product != "karryam" || binding.Canonical.Market != "" || binding.Canonical.Zone != "" || binding.Canonical.Organization != "" || binding.Canonical.Tenant != "" {
		t.Fatal("operational hints became authorization", binding)
	}
	_, _, q, _ := privacyFixture(t)
	approval := issuer.Approval("TEST_ONLY_VENDOR_ALLOCATION", kind, id, "analytics.cohorts.readiness.export", binding, q.Scope)
	raw := issuer.Sign(t, approval)

	if err := client.Publish(ctx, raw); err == nil {
		t.Fatal("production Vendor runtime accepted TEST_ONLY allocation")
	}
	publisher.handlers[owner].Contexts.Authority = issuer.Authority
	ingestLineage(t, db, "fabricated-vendor-branch-snapshot", networkanalytics.EventBranchReadiness, owner, "branch", branch, 1, map[string]any{"branch_id": branch})
	if _, err := client.PrepareBinding(ctx, kind, "fabricated-vendor-branch-snapshot", "TEST_ONLY_REVIEW"); err == nil {
		t.Fatal("non-native readiness entity accepted")
	}
	foreignSigner := contextfixture.New(t, "driver-taxi-service")
	if err := client.Publish(ctx, foreignSigner.Sign(t, approval)); err == nil {
		t.Fatal("driver signature authorized Vendor branch")
	}
	if err := client.Publish(ctx, raw); err != nil {
		t.Fatal(err)
	}
	resolve := func() error {
		tx, err := db.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		_, err = publisher.handlers[owner].Contexts.Resolve(ctx, tx, kind, id, "analytics.cohorts.readiness.export", q.Scope, time.Now())
		return err
	}
	if err := resolve(); err != nil {
		t.Fatal(err)
	}
	// A driver audience cannot prepare or publish a Vendor event.
	driver, err := analyticsadmin.NewPublisherClient(server.URL, "driver-taxi-service", key)
	if err != nil {
		t.Fatal(err)
	}
	defer driver.Close()
	if _, err := driver.PrepareBinding(ctx, kind, id, "TEST_ONLY_REVIEW"); err == nil {
		t.Fatal("driver audience prepared Vendor branch")
	}
	if err := driver.Publish(ctx, raw); !errors.Is(err, analyticsadmin.ErrInvalid) {
		t.Fatal("client accepted foreign owner", err)
	}
	if _, err := client.PrepareBinding(ctx, "mission-success:branch_id", id, "TEST_ONLY_REVIEW"); err == nil {
		t.Fatal("readiness event accepted as driver outcome")
	}
	if _, err := client.PrepareBinding(ctx, "readiness:provider_id", id, "TEST_ONLY_REVIEW"); err == nil {
		t.Fatal("missing provider invented")
	}
	wrong := approval
	wrong.ID = "TEST_ONLY_WRONG_SIGNER"
	wrong.Owner = "driver-taxi-service"
	if err := client.Publish(ctx, issuer.Sign(t, wrong)); !errors.Is(err, analyticsadmin.ErrInvalid) {
		t.Fatal("cross-owner envelope accepted", err)
	}
	approval.Revision = 2
	approval.State = "REVOKED"
	if err := client.Publish(ctx, issuer.Sign(t, approval)); err != nil {
		t.Fatal(err)
	}
	if err := resolve(); !errors.Is(err, admincontext.ErrUnmapped) {
		t.Fatal("Vendor revocation not enforced", err)
	}
	if err := client.Publish(ctx, raw); err == nil {
		t.Fatal("stale allocation resurrected Vendor branch")
	}
	if _, err := PublisherRuntime(ctx, db, serviceauth.Secrets{"legacy": key}, nil); err == nil {
		t.Fatal("legacy Vendor key acquired publication authority")
	}
	if _, err := PublisherRuntime(ctx, db, nil, key); err == nil {
		t.Fatal("read key acquired Vendor publication authority")
	}

	driverIssuer := contextfixture.New(t, "driver-taxi-service")
	keys["driver-taxi-service"] = map[string]string{}
	for id, k := range driverIssuer.Authority.Keys {
		keys["driver-taxi-service"][id] = base64.RawURLEncoding.EncodeToString(k)
	}
	write(keysFile, keys)
	write(secretsFile, map[string]string{owner: base64.RawURLEncoding.EncodeToString(key), "driver-taxi-service": base64.RawURLEncoding.EncodeToString(key)})
	if _, err := PublisherRuntime(ctx, db, nil, nil); err == nil {
		t.Fatal("driver and Vendor reused publication audience")
	}
	// With publication killed, the fixed Vendor routes still deny by default.
	t.Setenv("ANALYTICS_ADMIN_CONTEXT_PUBLISH_ENABLED", "false")
	disabled, err := PublisherRuntime(ctx, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	root := gin.New()
	disabled.Register(root)
	recorder := httptest.NewRecorder()
	root.ServeHTTP(recorder, httptest.NewRequest("POST", analyticsadmin.SourceContextPath+owner, nil))
	if recorder.Code != 503 {
		t.Fatal("Vendor path enabled by registration", recorder.Code)
	}
	if adminowner.ContextPublisher != analyticsadmin.ProducerPublisherCaller {
		t.Fatal("publisher contract drift")
	}
}
