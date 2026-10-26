package secureadmin_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"testing"
	"time"

	"cosmicforge/logistics/services/support-dispute-service/internal/features/adminfeed"
	supportmodels "cosmicforge/logistics/services/support-dispute-service/internal/features/support/models"
	supportusecases "cosmicforge/logistics/services/support-dispute-service/internal/features/support/usecases"
	"cosmicforge/logistics/shared/go/adminevents"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestSupportAdminEventSnapshotBackfill(t *testing.T) {
	h := newHarness(t, nil)
	var created []string
	for i := 0; i < 5; i++ {
		c, err := h.support.CreateComplaint(h.ctx, supportusecases.CreateComplaintInput{ComplainantType: supportmodels.ComplainantCustomer, ComplainantID: "cust-" + uuid.NewString(), ServiceType: supportmodels.ServiceTypeDispatch, Subject: "Secret subject 123456", Description: "Secret description"})
		if err != nil {
			t.Fatal(err)
		}
		created = append(created, c.ID)
	}
	sort.Strings(created)
	secret := bytes.Repeat([]byte("s"), 32)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := &adminfeed.SnapshotHandler{DB: h.pool, Secret: secret, Enabled: true}
	handler.Register(router)
	do := func(query string, key []byte, caller string, nonce string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, adminfeed.SnapshotPath+query, nil)
		if nonce != "" {
			req.Header.Set(serviceauth.HeaderNonce, nonce)
		}
		if err := serviceauth.SignRequest(req, caller, key, nil, time.Now()); err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	var seen []string
	cursor := ""
	for pages := 0; ; pages++ {
		if pages > 5 {
			t.Fatal("pagination did not terminate")
		}
		query := "?limit=2"
		if cursor != "" {
			query += "&cursor=" + cursor
		}
		rec := do(query, secret, adminfeed.Caller, "")
		if rec.Code != 200 {
			t.Fatalf("page status=%d body=%s", rec.Code, rec.Body.String())
		}
		if bytes.Contains(rec.Body.Bytes(), []byte("Secret")) {
			t.Fatal("snapshot leaks case text")
		}
		var page adminevents.SnapshotPage
		if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		if err := page.Validate(adminfeed.SourceService); err != nil || page.EntityType != "support_case" {
			t.Fatalf("invalid page: %v", err)
		}
		for _, ev := range page.Events {
			raw, _ := json.Marshal(ev)
			if _, err := adminevents.Decode(raw); err != nil {
				t.Fatalf("snapshot event does not decode: %v", err)
			}
			seen = append(seen, ev.EntityRef)
		}
		if page.Complete {
			break
		}
		cursor = page.NextCursor
	}
	if len(seen) != len(created) {
		t.Fatalf("snapshot saw %d cases want %d", len(seen), len(created))
	}
	for i := range created {
		if seen[i] != created[i] {
			t.Fatalf("snapshot order/content mismatch at %d", i)
		}
	}

	nonce := "replay-" + uuid.NewString()
	if rec := do("?limit=1", secret, adminfeed.Caller, nonce); rec.Code != 200 {
		t.Fatalf("first nonce use: %d", rec.Code)
	}
	if rec := do("?limit=1", secret, adminfeed.Caller, nonce); rec.Code != http.StatusConflict {
		t.Fatalf("replayed nonce accepted: %d", rec.Code)
	}
	if rec := do("?limit=1", bytes.Repeat([]byte("x"), 32), adminfeed.Caller, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong secret accepted: %d", rec.Code)
	}
	if rec := do("?limit=1", secret, "fleet-partner-service", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong caller accepted: %d", rec.Code)
	}
	for _, bad := range []string{"?limit=201", "?limit=0", "?cursor=not-a-uuid", "?other=1"} {
		if rec := do(bad, secret, adminfeed.Caller, ""); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s accepted: %d", bad, rec.Code)
		}
	}
	unsigned := httptest.NewRequest(http.MethodGet, adminfeed.SnapshotPath, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, unsigned)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned accepted: %d", rec.Code)
	}

	disabled := gin.New()
	(&adminfeed.SnapshotHandler{DB: h.pool}).Register(disabled)
	rec = httptest.NewRecorder()
	disabled.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, adminfeed.SnapshotPath, nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("disabled snapshot served: %d", rec.Code)
	}
}

func TestSupportAdminEventSnapshotConfiguration(t *testing.T) {
	pool := newHarness(t, nil).pool
	dir := t.TempDir()
	write := func(name string, key []byte) string {
		path := dir + "/" + name
		if err := os.WriteFile(path, []byte(base64.RawURLEncoding.EncodeToString(key)+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	ctx := context.Background()
	// Default off: nothing is read, the handler answers 503.
	h, err := adminfeed.SnapshotFromEnvironment(ctx, pool)
	if err != nil || h.Enabled {
		t.Fatalf("default must be disabled: %+v %v", h, err)
	}
	t.Setenv("SUPPORT_DISPUTE_ADMIN_EVENTS_SNAPSHOT_ENABLED", "true")
	if _, err := adminfeed.SnapshotFromEnvironment(ctx, pool); err == nil {
		t.Fatal("enabled without a secret file")
	}
	t.Setenv("SUPPORT_DISPUTE_ADMIN_EVENTS_SNAPSHOT_SECRET_FILE", write("short", []byte("too-short")))
	if _, err := adminfeed.SnapshotFromEnvironment(ctx, pool); err == nil {
		t.Fatal("short secret accepted")
	}
	snapshotKey := bytes.Repeat([]byte("k"), 32)
	t.Setenv("SUPPORT_DISPUTE_ADMIN_EVENTS_SNAPSHOT_SECRET_FILE", write("snapshot", snapshotKey))
	t.Setenv("SUPPORT_DISPUTE_ADMIN_RESOURCE_SECRET_FILE", write("resource", snapshotKey))
	if _, err := adminfeed.SnapshotFromEnvironment(ctx, pool); err == nil {
		t.Fatal("snapshot key shared with the resource key")
	}
	t.Setenv("SUPPORT_DISPUTE_ADMIN_RESOURCE_SECRET_FILE", write("resource", bytes.Repeat([]byte("r"), 32)))
	h, err = adminfeed.SnapshotFromEnvironment(ctx, pool)
	if err != nil || !h.Enabled || !bytes.Equal(h.Secret, snapshotKey) {
		t.Fatalf("valid configuration refused: %v", err)
	}
}
