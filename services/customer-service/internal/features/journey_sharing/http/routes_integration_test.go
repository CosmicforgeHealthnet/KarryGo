package sharehttp

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	r "cosmicforge/logistics/services/customer-service/internal/features/journey_sharing/repositories"
	u "cosmicforge/logistics/services/customer-service/internal/features/journey_sharing/usecases"
	"cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/journeyshare"
	"cosmicforge/logistics/shared/go/journeyshare/journeysharetest"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type scopedSource struct{}

func (scopedSource) Read(_ context.Context, authority, ref, owner string) (journeyshare.Projection, error) {
	return journeyshare.Projection{Authority: authority, SourceRef: ref, CustomerID: owner, ProductCode: "RIDE", Eligible: true, SourceVersion: 1, PublicStatus: "IN_PROGRESS", Location: journeyshare.Location{State: "UNAVAILABLE"}}, nil
}

// Persistence, bearer boundary, strict input, public policy and Redis limits are
// real. The source reader is deliberately a fixture; this is not a full-service E2E.
func TestHTTPPersistentCommandsAndPublicSecurity(t *testing.T) {
	dsn := os.Getenv("CUSTOMER_TEST_DATABASE_URL")
	addr := os.Getenv("JOURNEY_TEST_REDIS_ADDR")
	if dsn == "" || addr == "" {
		t.Skip("requires isolated Customer PostgreSQL and Redis")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rc := redis.NewClient(&redis.Options{Addr: addr})
	defer rc.Close()
	owner := uuid.NewString()
	_, err = db.Exec(ctx, `INSERT INTO customers(id,email) VALUES($1,$2)`, owner, owner+"@test.invalid")
	if err != nil {
		t.Fatal(err)
	}
	signer := auth.NewTokenSigner([]byte("http-test"))
	bearer, _ := signer.Sign(auth.Claims{Subject: owner, Role: "customer", Service: "customer", Type: "access", SessionID: uuid.NewString(), ExpiresAt: time.Now().Add(time.Hour).Unix()})
	svc := &u.Service{Repository: &r.Postgres{DB: db}, Reader: scopedSource{}, Enabled: true}
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(httpx.ErrorHandler())
	Register(engine.Group("/api/v1/customer"), svc, signer, journeyshare.RedisLimiter{Client: rc}, "https://viewer.test")
	request := func(method, path, body, authorization, key, origin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.RemoteAddr = "[2001:db8:" + owner[:4] + ":" + owner[4:8] + "::1]:5555"
		req.Header.Set("Authorization", authorization)
		req.Header.Set("Idempotency-Key", key)
		req.Header.Set("Origin", origin)
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, req)
		return response
	}
	base := "/api/v1/customer/journey-shares"
	input := `{"authority":"TAXI","source_ref":"` + uuid.NewString() + `","product_code":"RIDE"}`
	if w := request("POST", base, input, "", "create", ""); w.Code != 401 {
		t.Fatal("anonymous command", w.Code)
	}
	if w := request("POST", base, strings.TrimSuffix(input, "}")+`,"customer_id":"spoof"}`, "Bearer "+bearer, "spoof", ""); w.Code != 400 {
		t.Fatal("spoof", w.Code, w.Body.String())
	}
	w := request("POST", base, input, "Bearer "+bearer, "create", "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var result struct{ Data u.Result }
	if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	token := result.Data.Token
	id := result.Data.Session.ID
	for _, path := range []string{base, base + "/" + id} {
		w = request("GET", path, "", "Bearer "+bearer, "", "")
		if w.Code != 200 || strings.Contains(w.Body.String(), token) {
			t.Fatal("metadata", w.Code, w.Body.String())
		}
	}
	replay := request("POST", base, input, "Bearer "+bearer, "create", "")
	if replay.Code != 200 || strings.Contains(replay.Body.String(), token) {
		t.Fatal("replay", replay.Code)
	}
	public := "/api/v1/customer/public/journey-shares/" + token
	w = request("GET", public, "", "", "", "https://evil.test")
	if w.Code != 403 {
		t.Fatal("CORS", w.Code)
	}
	w = request("GET", public, "", "", "", "https://viewer.test")
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Robots-Tag") != "noindex, nofollow" || w.Header().Get("Access-Control-Allow-Origin") != "https://viewer.test" {
		t.Fatal("public policy", w.Code, w.Header(), w.Body.String())
	}
	for _, secret := range []string{owner, id, token, "source_ref", "token_hash", "customer_id", "otp", "payment"} {
		if strings.Contains(w.Body.String(), secret) {
			t.Fatal("public leak", secret)
		}
	}

	// Forty bounded reads fit the token's sixty-read minute budget.
	journeysharetest.Exercise(t, "N06 public policy and digest lookup", 40, func() error {
		out := request("GET", public, "", "", "", "")
		if out.Code != 200 {
			return fmt.Errorf("status %d", out.Code)
		}
		return nil
	})
	w = request("POST", base+"/"+id+"/rotate", `{"expected_version":1}`, "Bearer "+bearer, "rotate", "")
	if w.Code != 200 {
		t.Fatal("rotate", w.Code, w.Body.String())
	}
	if w = request("GET", public, "", "", "", ""); w.Code != 404 {
		t.Fatal("old token survived", w.Code)
	}
	var rotated struct{ Data u.Result }
	_ = json.Unmarshal(request("POST", base+"/"+id+"/rotate", `{"expected_version":1}`, "Bearer "+bearer, "rotate", "").Body.Bytes(), &rotated)
	if rotated.Data.Token != "" || !rotated.Data.Replayed {
		t.Fatal("rotation retry")
	}
	w = request("POST", base+"/"+id+"/revoke", `{"expected_version":2}`, "Bearer "+bearer, "revoke", "")
	if w.Code != 200 {
		t.Fatal("revoke", w.Code, w.Body.String())
	}
	// Distinct IP isolates this exercise from earlier reads; token digest is unique.
	limited := false
	for i := 0; i < 62; i++ {
		w = request("GET", public, "", "", "", "")
		if w.Code == 429 {
			limited = true
			break
		}
	}
	if !limited {
		t.Fatal("public token rate limit absent")
	}
	svc.Enabled = false
	if w = request("GET", base, "", "Bearer "+bearer, "", ""); w.Code != 503 {
		t.Fatal("feature gate", w.Code)
	}
}

func TestMountedOpenAPIParity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	Register(engine.Group("/api/v1/customer"), &u.Service{}, auth.NewTokenSigner([]byte("test")), nil, "")
	raw, err := os.ReadFile("../../../../docs/openapi/journey-sharing.openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Paths map[string]map[string]json.RawMessage
	}
	if err = json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	paths := map[string]bool{}
	for path, methods := range spec.Paths {
		for method := range methods {
			paths[strings.ToUpper(method)+" "+path] = true
		}
	}
	for _, route := range engine.Routes() {
		path := strings.ReplaceAll(strings.ReplaceAll(route.Path, ":shareId", "{shareId}"), ":token", "{token}")
		key := route.Method + " " + path
		if !paths[key] {
			t.Fatal("undocumented", key)
		}
		delete(paths, key)
	}
	if len(paths) > 0 {
		t.Fatal("phantom routes", paths)
	}
}
