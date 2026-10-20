package adminprivacy

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/analyticsadmin"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
)

func TestScopedAnalyticsRuntimeOffDoesNotReadFiles(t *testing.T) {
	t.Setenv("ANALYTICS_ADMIN_SCOPED_NETWORK_READ_ENABLED", "false")
	t.Setenv("ANALYTICS_ADMIN_SCOPED_SECRET_FILE", "missing")
	h, err := Runtime(context.Background(), nil, nil)
	if err != nil || h.Enabled {
		t.Fatal(err)
	}
	engine := gin.New()
	h.Register(engine)
	r := httptest.NewRecorder()
	engine.ServeHTTP(r, httptest.NewRequest("GET", analyticsadmin.MissionSuccessPath, nil))
	if r.Code != 503 {
		t.Fatal("off endpoint not denied", r.Code)
	}
}
func TestPostgresScopedAnalyticsNonceStrictQueryAndPolicyBoundary(t *testing.T) {
	db := privacyDB(t)
	f, _, q, _ := privacyFixture(t)
	key := bytes.Repeat([]byte{82}, 32)
	h := &Handler{Enabled: true, Secret: key, Service: &Service{DB: db, Policy: f.Engine}}
	engine := gin.New()
	h.Register(engine)
	meta := adminread.Query{Scope: adminread.Scope{Market: q.Scope.Market, Zone: q.Scope.Zone, Organization: q.Scope.Organization, Tenant: q.Scope.Tenant}, Staff: "staff-1", Role: "staff", Purpose: q.Purpose, PolicyID: f.Engine.ID(), PolicyVersion: f.Engine.Version(), PolicyChecksum: f.Engine.Checksum(), Start: q.Start, End: q.End, Limit: 1}
	values, err := analyticsadmin.Values(meta, analyticsadmin.Filters{Authority: "TAXI", ProductCode: "TAXI_PASSENGER"})
	if err != nil {
		t.Fatal(err)
	}
	request := func(query string, secret []byte) *http.Request {
		t.Helper()
		r := httptest.NewRequest("GET", analyticsadmin.MissionSuccessPath+"?"+query, nil)
		r.Body = http.NoBody
		if err := serviceauth.SignRequest(r, adminread.Caller, secret, nil, time.Now()); err != nil {
			t.Fatal(err)
		}
		return r
	}
	call := func(r *http.Request) int {
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, r)
		if strings.Contains(w.Body.String(), `"cells"`) {
			t.Error("denied request disclosed cells")
		}
		return w.Code
	}
	r := request(values.Encode(), key)
	replay := r.Clone(context.Background())
	replay.Body = http.NoBody
	// This fixture has no report-specific policy/source; authentic requests
	// must remain unavailable and nevertheless consume the durable nonce.
	if code := call(r); code != 503 {
		t.Fatal(code)
	}
	if code := call(replay); code != 409 {
		t.Fatal("nonce replay", code)
	}
	if code := call(request(values.Encode(), bytes.Repeat([]byte{83}, 32))); code != 401 {
		t.Fatal("legacy/wrong key accepted", code)
	}
	for _, extra := range []string{"&staff=forged", "&authority=TAXI", "&unknown=1", "&cursor=forged"} {
		if code := call(request(values.Encode()+extra, key)); code != 400 {
			t.Fatal(extra, code)
		}
	}
}
func TestPostgresScopedAnalyticsRuntimeCredentialSeparationAndNoInventedApproval(t *testing.T) {
	db := lineageDB(t)
	key := bytes.Repeat([]byte{84}, 32)
	file := filepath.Join(t.TempDir(), "source.key")
	if err := os.WriteFile(file, []byte(base64.RawURLEncoding.EncodeToString(key)), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ANALYTICS_ADMIN_SCOPED_NETWORK_READ_ENABLED", "true")
	t.Setenv("ANALYTICS_ADMIN_SCOPED_SECRET_FILE", file)
	for _, name := range []string{"ANALYTICS_ADMIN_SCOPED_POLICY_FILE", "ANALYTICS_ADMIN_SCOPED_POLICY_KEYS_FILE", "ANALYTICS_ADMIN_SOURCE_CONTEXT_KEYS_FILE", "ANALYTICS_ADMIN_MISSION_SUCCESS_UNIT_FIELD"} {
		t.Setenv(name, "")
	}
	h, err := Runtime(context.Background(), db, nil)
	if err != nil || !h.Enabled || h.Service.Policy != nil {
		t.Fatal("missing policy became an approval or stopped independent source work", err)
	}
	if _, err := Runtime(context.Background(), db, serviceauth.Secrets{"legacy": key}); err == nil {
		t.Fatal("legacy credential acquired scoped authority")
	}
	for _, raw := range []string{`{"a":{},"a":{}}`, `null`, `{"owner":{"key":"value","key":"other"}}`} {
		d := json.NewDecoder(strings.NewReader(raw))
		if uniqueKeyMap(d, 0) == nil {
			t.Fatal("ambiguous source keys accepted")
		}
	}
}
