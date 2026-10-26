package adminsnapshot

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"cosmicforge/logistics/services/verification-compliance-service/internal/testdb"
	"cosmicforge/logistics/shared/go/adminevents"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func TestSnapshotDisabledByDefaultAndRequiresDedicatedSecret(t *testing.T) {
	t.Setenv("VERIFICATION_COMPLIANCE_ADMIN_EVENTS_SNAPSHOT_ENABLED", "")
	h, err := FromEnvironment(nil)
	if err != nil || h.Enabled {
		t.Fatalf("snapshot must default off: %+v %v", h, err)
	}
	t.Setenv("VERIFICATION_COMPLIANCE_ADMIN_EVENTS_SNAPSHOT_ENABLED", "true")
	t.Setenv("VERIFICATION_COMPLIANCE_ADMIN_EVENTS_SNAPSHOT_SECRET_FILE", filepath.Join(t.TempDir(), "missing"))
	if _, err := FromEnvironment(nil); err == nil {
		t.Fatal("enabled snapshot without secret accepted")
	}
	router := gin.New()
	(&Handler{}).Register(router)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, Path, nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("disabled snapshot served %d", w.Code)
	}
}

func signed(t *testing.T, router http.Handler, caller string, secret []byte, query, nonce string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, Path+query, nil)
	req.Header.Set(serviceauth.HeaderNonce, nonce)
	if err := serviceauth.SignRequest(req, caller, secret, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestSnapshotBackfillPagesValidatedMinimizedEvents(t *testing.T) {
	pool := testdb.Fresh(t)
	ctx := context.Background()
	for _, business := range []string{"biz-s1", "biz-s2", "biz-s3", "biz with a space"} {
		if _, err := pool.Exec(ctx, `INSERT INTO vendor_verification_cases(client_reference,business_id,dimension,subject_type,subject_ref,document_refs,request_fingerprint,idempotency_key,reason_code) VALUES($1,$2,'FOOD_KITCHEN','BUSINESS','subject-secret','["doc-secret"]','fp',$1,'free text reason')`, uuid.NewString(), business); err != nil {
			t.Fatal(err)
		}
	}
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	file := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(file, []byte(base64.RawURLEncoding.EncodeToString(raw)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VERIFICATION_COMPLIANCE_ADMIN_EVENTS_SNAPSHOT_ENABLED", "true")
	t.Setenv("VERIFICATION_COMPLIANCE_ADMIN_EVENTS_SNAPSHOT_SECRET_FILE", file)
	h, err := FromEnvironment(pool)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	h.Register(router)

	if w := signed(t, router, "vendor-service", raw, "", uuid.NewString()); w.Code == http.StatusOK {
		t.Fatal("non-admin caller served")
	}
	other := []byte(strings.Repeat("x", 32))
	h.Verifier = serviceauth.NewVerifier(serviceauth.Secrets{Caller: raw, "vendor-service": other}, 5*time.Minute)
	if w := signed(t, router, "vendor-service", other, "", uuid.NewString()); w.Code != http.StatusForbidden {
		t.Fatalf("wrong caller=%d", w.Code)
	}
	if w := signed(t, router, Caller, raw, "?limit=201", uuid.NewString()); w.Code != http.StatusBadRequest {
		t.Fatalf("limit above 200=%d", w.Code)
	}
	seen := map[string]bool{}
	cursor := ""
	for pages := 0; pages < 5; pages++ {
		query := "?limit=2"
		if cursor != "" {
			query += "&cursor=" + cursor
		}
		nonce := uuid.NewString()
		w := signed(t, router, Caller, raw, query, nonce)
		if w.Code != http.StatusOK {
			t.Fatalf("snapshot=%d %s", w.Code, w.Body.String())
		}
		if replay := signed(t, router, Caller, raw, query, nonce); replay.Code != http.StatusUnauthorized {
			t.Fatalf("replayed nonce=%d", replay.Code)
		}
		if strings.Contains(w.Body.String(), "doc-secret") || strings.Contains(w.Body.String(), "subject-secret") || strings.Contains(w.Body.String(), "free text") {
			t.Fatalf("snapshot leaks: %s", w.Body.String())
		}
		var page adminevents.SnapshotPage
		if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		if err := page.Validate(Source); err != nil || page.EntityType != EntityType {
			t.Fatalf("page invalid: %v", err)
		}
		for _, ev := range page.Events {
			encoded, _ := json.Marshal(ev)
			decoded, err := adminevents.Decode(encoded)
			if err != nil {
				t.Fatal(err)
			}
			adapter, _ := adminevents.Lookup(decoded.SourceService, decoded.EventType)
			if _, err := adapter.Minimize(decoded); err != nil {
				t.Fatalf("minimize: %v", err)
			}
			seen[ev.EntityRef] = true
		}
		if page.Complete {
			break
		}
		cursor = page.NextCursor
	}
	if len(seen) != 3 {
		t.Fatalf("snapshot covered %d exportable cases, want 3", len(seen))
	}
}
