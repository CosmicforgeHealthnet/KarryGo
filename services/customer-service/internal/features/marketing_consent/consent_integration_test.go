package marketingconsent_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	marketingconsent "cosmicforge/logistics/services/customer-service/internal/features/marketing_consent"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/journeyshare"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// isolatedPool applies the actual customer migrations into a fresh schema of
// the disposable CUSTOMER_TEST_DATABASE_URL database. It never falls back to
// a default DSN.
func isolatedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("CUSTOMER_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("NOT EXECUTED: CUSTOMER_TEST_DATABASE_URL (isolated disposable PostgreSQL) is required")
	}
	ctx := context.Background()
	root, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "mc_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := root.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = root.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
		root.Close()
	})
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(file), "..", "..", "..", "migrations")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(raw)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
	return pool
}

func newCustomer(t *testing.T, db *pgxpool.Pool, status string) string {
	t.Helper()
	id := uuid.NewString()
	if _, err := db.Exec(context.Background(), `INSERT INTO customers(id,email,status) VALUES($1,$2,$3)`, id, "mc-"+id+"@example.invalid", status); err != nil {
		t.Fatal(err)
	}
	return id
}

func opt(t *testing.T, store *marketingconsent.Store, id, channel string, in bool, market string) {
	t.Helper()
	var m *string
	if market != "" {
		m = &market
	}
	if _, _, err := store.Set(context.Background(), marketingconsent.SetInput{CustomerID: id, Channel: channel, OptedIn: in, Market: m}); err != nil {
		t.Fatal(err)
	}
}

type customerAPI struct {
	engine *gin.Engine
	signer *sharedauth.TokenSigner
}

func (a customerAPI) do(t *testing.T, method, customer, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, "/api/v1/customer/consents/marketing", strings.NewReader(body))
	if customer != "" {
		token, err := a.signer.Sign(sharedauth.Claims{Subject: customer, Role: "customer", Service: "customer", SessionID: "s", ExpiresAt: time.Now().Add(time.Hour).Unix()})
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	a.engine.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestConsentRoutesCaptureWithdrawIdempotentAndHistory(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := isolatedPool(t)
	store := marketingconsent.NewStore(db)
	signer := sharedauth.NewTokenSigner([]byte("customer-test-access-secret"))
	build := func(flags marketingconsent.Flags) customerAPI {
		e := gin.New()
		e.Use(httpx.ErrorHandler())
		marketingconsent.RegisterCustomerRoutes(e.Group("/api/v1/customer"), store, signer, flags)
		return customerAPI{e, signer}
	}
	off, on := build(marketingconsent.Flags{}), build(marketingconsent.Flags{CaptureEnabled: true})
	id := newCustomer(t, db, "active")

	if code, _ := on.do(t, http.MethodGet, "", ""); code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated read: %d", code)
	}
	code, out := on.do(t, http.MethodGet, id, "")
	list := out["data"].(map[string]any)["consents"].([]any)
	if code != 200 || len(list) != 3 || list[0].(map[string]any)["status"] != "NOT_RECORDED" {
		t.Fatalf("initial read: %d %v", code, out)
	}
	if code, _ := off.do(t, http.MethodPut, id, `{"channel":"push","opted_in":true,"market":"NG-LAG"}`); code != http.StatusServiceUnavailable {
		t.Fatalf("capture while disabled: %d", code)
	}
	for _, bad := range []string{`{"channel":"sms","opted_in":true}`, `{"channel":"push"}`, `{"channel":"push","opted_in":true,"market":"has space"}`, `{"channel":"push","opted_in":true,"extra":1}`} {
		if code, _ := on.do(t, http.MethodPut, id, bad); code != http.StatusUnprocessableEntity && code != http.StatusBadRequest {
			t.Fatalf("invalid body %s accepted: %d", bad, code)
		}
	}
	code, out = on.do(t, http.MethodPut, id, `{"channel":"push","opted_in":true,"market":"NG-LAG"}`)
	consent := out["data"].(map[string]any)["consent"].(map[string]any)
	if code != 200 || consent["status"] != "OPTED_IN" || consent["version"].(float64) != 1 || consent["market"] != "NG-LAG" || out["data"].(map[string]any)["changed"] != true {
		t.Fatalf("opt-in: %d %v", code, out)
	}
	code, out = on.do(t, http.MethodPut, id, `{"channel":"push","opted_in":true,"market":"NG-LAG"}`)
	if code != 200 || out["data"].(map[string]any)["changed"] != false || out["data"].(map[string]any)["consent"].(map[string]any)["version"].(float64) != 1 {
		t.Fatalf("repeat opt-in must be idempotent: %v", out)
	}
	// Withdrawal is allowed even with capture disabled, and is idempotent.
	code, out = off.do(t, http.MethodPut, id, `{"channel":"push","opted_in":false}`)
	consent = out["data"].(map[string]any)["consent"].(map[string]any)
	if code != 200 || consent["status"] != "WITHDRAWN" || consent["version"].(float64) != 2 || consent["market"] != "NG-LAG" || consent["withdrawn_at"] == nil {
		t.Fatalf("withdrawal: %d %v", code, out)
	}
	if code, out = off.do(t, http.MethodPut, id, `{"channel":"push","opted_in":false}`); code != 200 || out["data"].(map[string]any)["changed"] != false {
		t.Fatalf("repeat withdrawal: %v", out)
	}
	// An explicit refusal of a never-opted channel is recorded, not invented.
	if code, out = off.do(t, http.MethodPut, id, `{"channel":"email","opted_in":false}`); code != 200 || out["data"].(map[string]any)["consent"].(map[string]any)["status"] != "WITHDRAWN" {
		t.Fatalf("refusal: %v", out)
	}
	var history int
	if err := db.QueryRow(context.Background(), `SELECT count(*) FROM customer_marketing_consent_history WHERE customer_id=$1`, id).Scan(&history); err != nil || history != 3 {
		t.Fatalf("history rows: %d %v", history, err)
	}
	// History is append-only.
	for _, stmt := range []string{`UPDATE customer_marketing_consent_history SET status='OPTED_IN'`, `DELETE FROM customer_marketing_consent_history`, `TRUNCATE customer_marketing_consent_history`} {
		if _, err := db.Exec(context.Background(), stmt); err == nil || !strings.Contains(err.Error(), "append-only") {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	// A customer that does not exist cannot opt in.
	if code, _ := on.do(t, http.MethodPut, uuid.NewString(), `{"channel":"push","opted_in":true}`); code != http.StatusNotFound {
		t.Fatalf("unknown customer: %d", code)
	}
}

func TestAudiencePaginationAndMarketFilter(t *testing.T) {
	db := isolatedPool(t)
	store := marketingconsent.NewStore(db)
	ctx := context.Background()
	var lagos []string
	for i := 0; i < 5; i++ {
		id := newCustomer(t, db, "active")
		opt(t, store, id, "push", true, "NG-LAG")
		lagos = append(lagos, id)
	}
	sort.Strings(lagos)
	for i := 0; i < 2; i++ {
		opt(t, store, newCustomer(t, db, "active"), "push", true, "NG-ABJ")
	}
	opt(t, store, newCustomer(t, db, "active"), "push", true, "")        // unknown market
	opt(t, store, newCustomer(t, db, "active"), "email", true, "NG-LAG") // other channel
	withdrawn := newCustomer(t, db, "active")
	opt(t, store, withdrawn, "push", true, "NG-LAG")
	opt(t, store, withdrawn, "push", false, "")
	opt(t, store, newCustomer(t, db, "suspended"), "push", true, "NG-LAG") // inactive account

	var got []string
	cursor, pages := "", 0
	for {
		page, err := store.Audience(ctx, "push", "NG-LAG", cursor, 2)
		if err != nil {
			t.Fatal(err)
		}
		pages++
		if page.AsOf.IsZero() || len(page.Members) > 2 {
			t.Fatalf("page: %+v", page)
		}
		for _, m := range page.Members {
			if m.ConsentVersion != 1 {
				t.Fatalf("version %+v", m)
			}
			got = append(got, m.CustomerID)
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	if fmt.Sprint(got) != fmt.Sprint(lagos) || pages != 3 {
		t.Fatalf("Lagos audience %v (pages %d), want %v", got, pages, lagos)
	}
	abuja, err := store.Audience(ctx, "push", "NG-ABJ", "", 500)
	if err != nil || len(abuja.Members) != 2 {
		t.Fatalf("Abuja audience: %+v %v", abuja, err)
	}
	for _, bad := range [][2]string{{"push", ""}, {"sms", "NG-LAG"}, {"push", "bad market"}} {
		if _, err := store.Audience(ctx, bad[0], bad[1], "", 10); err == nil {
			t.Fatalf("invalid audience query %v accepted", bad)
		}
	}
}

func TestEligibilityCheckReflectsCurrentState(t *testing.T) {
	db := isolatedPool(t)
	store := marketingconsent.NewStore(db)
	ctx := context.Background()
	in := newCustomer(t, db, "active")
	opt(t, store, in, "email", true, "NG-LAG")
	opt(t, store, in, "push", true, "NG-LAG")
	out := newCustomer(t, db, "active")
	opt(t, store, out, "email", true, "NG-LAG")
	opt(t, store, out, "email", false, "")
	inactive := newCustomer(t, db, "suspended")
	opt(t, store, inactive, "email", true, "NG-LAG")
	noEmail := uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO customers(id,phone) VALUES($1,$2)`, noEmail, "+23480"+noEmail[:8]); err != nil {
		t.Fatal(err)
	}
	opt(t, store, noEmail, "email", true, "NG-LAG")
	missing := uuid.NewString()

	res, err := store.Check(ctx, "email", []string{in, out, inactive, noEmail, missing, in})
	if err != nil || len(res.Results) != 5 || res.AsOf.IsZero() {
		t.Fatalf("check: %+v %v", res, err)
	}
	want := map[string]string{in: "OPTED_IN", out: "NOT_OPTED_IN", inactive: "ACCOUNT_INACTIVE", noEmail: "NO_EMAIL_ADDRESS", missing: "ACCOUNT_NOT_FOUND"}
	for _, r := range res.Results {
		if r.Reason != want[r.CustomerID] || r.Eligible != (r.CustomerID == in) {
			t.Fatalf("%s: %+v", r.CustomerID, r)
		}
		if (r.Email != nil) != (r.CustomerID == in) {
			t.Fatalf("email disclosed for ineligible customer: %+v", r)
		}
	}
	push, err := store.Check(ctx, "push", []string{in})
	if err != nil || !push.Results[0].Eligible || push.Results[0].Email != nil {
		t.Fatalf("push check must not disclose email: %+v %v", push, err)
	}
	if _, err := store.Check(ctx, "email", []string{"not-a-uuid"}); err == nil {
		t.Fatal("invalid id accepted")
	}
	tooMany := make([]string, 501)
	for i := range tooMany {
		tooMany[i] = uuid.NewString()
	}
	if _, err := store.Check(ctx, "email", tooMany); err == nil {
		t.Fatal("unbounded batch accepted")
	}
}

func TestInternalRoutesCallerFlagAndReplay(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := isolatedPool(t)
	store := marketingconsent.NewStore(db)
	rdb := redis.NewClient(&redis.Options{Addr: envOr("CUSTOMER_TEST_REDIS_ADDR", "127.0.0.1:6379")})
	t.Cleanup(func() { _ = rdb.Close() })
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Skip("NOT EXECUTED: Redis is required for replay protection")
	}
	secrets := serviceauth.Secrets{"notification-service": []byte("notification-test-secret-000000000000"), "vendor-service": []byte("vendor-test-secret-0000000000000000000")}
	verifier := serviceauth.NewVerifier(secrets, 5*time.Minute)
	build := func(flags marketingconsent.Flags) *gin.Engine {
		e := gin.New()
		e.Use(httpx.ErrorHandler())
		marketingconsent.RegisterInternalRoutes(e, store, flags, journeyshare.RedisBoundary(verifier, rdb, marketingconsent.AudienceCaller))
		return e
	}
	enabled, disabled := build(marketingconsent.Flags{AudienceEnabled: true}), build(marketingconsent.Flags{})
	id := newCustomer(t, db, "active")
	opt(t, store, id, "push", true, "NG-LAG")
	call := func(e *gin.Engine, caller, nonce, method, target, body string) int {
		req := httptest.NewRequest(method, target, bytes.NewReader([]byte(body)))
		req.Header.Set(serviceauth.HeaderNonce, nonce)
		if err := serviceauth.SignRequest(req, caller, secrets[caller], []byte(body), time.Now()); err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec.Code
	}
	audience := marketingconsent.AudiencePath + "?channel=push&market=NG-LAG&limit=10"
	if code := call(enabled, "vendor-service", uuid.NewString(), http.MethodGet, audience, ""); code != http.StatusForbidden {
		t.Fatalf("other caller: %d", code)
	}
	if code := call(disabled, "notification-service", uuid.NewString(), http.MethodGet, audience, ""); code != http.StatusServiceUnavailable {
		t.Fatalf("disabled audience: %d", code)
	}
	nonce := uuid.NewString()
	if code := call(enabled, "notification-service", nonce, http.MethodGet, audience, ""); code != http.StatusOK {
		t.Fatalf("audience: %d", code)
	}
	if code := call(enabled, "notification-service", nonce, http.MethodGet, audience, ""); code != http.StatusConflict {
		t.Fatalf("replayed nonce: %d", code)
	}
	body := `{"channel":"push","customer_ids":["` + id + `"]}`
	if code := call(enabled, "notification-service", uuid.NewString(), http.MethodPost, marketingconsent.CheckPath, body); code != http.StatusOK {
		t.Fatalf("check: %d", code)
	}
	if code := call(enabled, "notification-service", uuid.NewString(), http.MethodGet, marketingconsent.AudiencePath+"?channel=push&market=NG-LAG&limit=501", ""); code != http.StatusUnprocessableEntity {
		t.Fatalf("over-limit page: %d", code)
	}
	req := httptest.NewRequest(http.MethodGet, audience, nil)
	rec := httptest.NewRecorder()
	enabled.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned: %d", rec.Code)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
