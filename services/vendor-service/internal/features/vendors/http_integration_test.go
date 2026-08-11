package vendors

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/database"
	"cosmicforge/logistics/services/vendor-service/internal/platform/authctx"
	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
)

func TestHTTPCreateBusinessIdempotencyReplayAndMismatch(t *testing.T) {
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping real-Postgres HTTP idempotency test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = database.ApplyMigrations(ctx, pool, "../../../migrations"); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `TRUNCATE vendor_businesses,vendor_idempotency_keys,vendor_staff_role_assignments,vendor_staff,vendor_users RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO vendor_users(id,email) VALUES('vu_http','http@example.test'); INSERT INTO vendor_staff(id,vendor_id,user_id,status) VALUES('vs_http','vendor_http','vu_http','active')`); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1/vendor")
	group.Use(func(c *gin.Context) {
		authctx.Set(c, authctx.StaffContext{StaffID: "vs_http", VendorID: "vendor_http", SessionID: "session"})
		c.Next()
	})
	RegisterRoutes(group, NewHandler(NewService(pool, allowAll{}, nil, nil), idempotency.NewStore(pool, time.Hour), true, false))
	body := []byte(`{"legal_name":"HTTP Kitchen Limited","currency":"NGN","timezone":"Africa/Lagos"}`)
	first := requestSection6(t, router, http.MethodPost, "/api/v1/vendor/businesses", body, "business-create-1")
	if first.Code != http.StatusCreated {
		t.Fatalf("first status=%d body=%s", first.Code, first.Body.String())
	}
	second := requestSection6(t, router, http.MethodPost, "/api/v1/vendor/businesses", body, "business-create-1")
	if second.Code != first.Code || second.Body.String() != first.Body.String() {
		t.Fatalf("replay differs: first=%d %s second=%d %s", first.Code, first.Body.String(), second.Code, second.Body.String())
	}
	changed := []byte(`{"legal_name":"Different Kitchen Limited","currency":"NGN","timezone":"Africa/Lagos"}`)
	mismatch := requestSection6(t, router, http.MethodPost, "/api/v1/vendor/businesses", changed, "business-create-1")
	if mismatch.Code != http.StatusConflict {
		t.Fatalf("mismatch status=%d body=%s", mismatch.Code, mismatch.Body.String())
	}
}

func requestSection6(t *testing.T, router *gin.Engine, method, path string, body []byte, key string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}
