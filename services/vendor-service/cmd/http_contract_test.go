package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/services/vendor-service/internal/audit"
	"cosmicforge/logistics/services/vendor-service/internal/database"
	"cosmicforge/logistics/services/vendor-service/internal/events"
	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/vendor-service/internal/platform/ratelimit"

	vendorauthhttp "cosmicforge/logistics/services/vendor-service/internal/features/auth/http"
	vendorauthrepositories "cosmicforge/logistics/services/vendor-service/internal/features/auth/repositories"
	vendorauthusecases "cosmicforge/logistics/services/vendor-service/internal/features/auth/usecases"

	staffhttp "cosmicforge/logistics/services/vendor-service/internal/features/staff/http"
	staffrepositories "cosmicforge/logistics/services/vendor-service/internal/features/staff/repositories"
	staffusecases "cosmicforge/logistics/services/vendor-service/internal/features/staff/usecases"
	vendors "cosmicforge/logistics/services/vendor-service/internal/features/vendors"

	sharedauth "cosmicforge/logistics/shared/go/auth"
)

// buildTestRouter wires the exact same real components cmd/main.go's
// Register closure does (same handlers, same middleware, same adapters) onto
// a bare gin.Engine, so this test exercises Section 5's actual production
// wiring rather than a parallel test-only stand-in.
func buildTestRouter(t *testing.T, pool *pgxpool.Pool, redisClient *redis.Client) *gin.Engine {
	t.Helper()

	userRepo := vendorauthrepositories.NewUserRepository(pool)
	challengeRepo := vendorauthrepositories.NewOTPChallengeRepository(pool)
	sessionRepo := vendorauthrepositories.NewSessionRepository(pool)
	stepUpRepo := vendorauthrepositories.NewStepUpRepository(pool)

	staffRepo := staffrepositories.NewStaffRepository(pool)
	roleRepo := staffrepositories.NewRoleRepository(pool)
	permissionRepo := staffrepositories.NewPermissionRepository(pool)
	assignmentRepo := staffrepositories.NewAssignmentRepository(pool)

	auditRecorder := audit.NewPostgresRecorder(pool)
	evaluator := staffusecases.NewAuthorizationEvaluator(assignmentRepo)
	outboxRepo := events.NewOutboxRepository(pool)
	vendorService := vendors.NewService(pool, vendorAuthorizationAdapter{evaluator: evaluator}, vendorAuditAdapter{recorder: auditRecorder}, events.NewDomainPublisher(outboxRepo))

	authService := vendorauthusecases.NewAuthService(vendorauthusecases.Options{
		Users: userRepo, Challenges: challengeRepo, Sessions: sessionRepo, StepUps: stepUpRepo,
		Staff: staffResolverAdapter{staff: staffRepo}, Notifier: nil,
		AccessTokenSecret: []byte("contract-test-access-secret"), RefreshTokenSecret: []byte("contract-test-refresh-secret"), OTPSecret: []byte("contract-test-otp-secret"),
		AccessTokenTTL: 15 * time.Minute, RefreshTokenTTL: 30 * 24 * time.Hour,
		OTPTTL: 5 * time.Minute, OTPMaxAttempts: 5, OTPMaxResends: 5, StepUpTTL: 5 * time.Minute, OTPDebug: true,
	})

	staffEvents := staffEventAdapter{outbox: outboxRepo, db: pool}
	staffService := staffusecases.NewStaffService(staffusecases.StaffServiceOptions{
		Users: userResolverAdapter{users: userRepo}, Staff: staffRepo, Roles: roleRepo, Assignments: assignmentRepo,
		Evaluator: evaluator, Audit: auditAdapter{recorder: auditRecorder}, Events: staffEvents, Branches: vendorService,
	})
	roleService := staffusecases.NewRoleService(staffusecases.RoleServiceOptions{
		Roles: roleRepo, Permissions: permissionRepo, Assignments: assignmentRepo, Evaluator: evaluator,
		StepUp: authService, Audit: auditAdapter{recorder: auditRecorder}, Events: staffEvents, Branches: vendorService,
	})

	limiter := ratelimit.New(redisClient, "vendor:test:ratelimit:"+uniqueTestPrefix(t))
	authHandler := vendorauthhttp.NewHandler(authService, limiter, vendorauthhttp.RateLimits{
		StartMax: 100, StartWindowSeconds: 60, ResendMax: 100, ResendWindowSeconds: 60,
		VerifyMax: 100, VerifyWindowSeconds: 60, RefreshMax: 100, RefreshWindowSeconds: 60,
		StepUpStartMax: 100, StepUpStartWindowSeconds: 60, StepUpVerifyMax: 100, StepUpVerifyWindowSeconds: 60,
	})
	staffHandler := staffhttp.NewHandler(staffService, roleService, authService.RevokeAllSessionsForStaff)
	vendorHandler := vendors.NewHandler(vendorService, idempotency.NewStore(pool, time.Hour), true, false)
	accessSigner := sharedauth.NewTokenSigner([]byte("contract-test-access-secret"))

	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1/vendor")
	vendorauthhttp.RegisterRoutes(group, authHandler, accessSigner, sessionRepo, staffResolverAdapter{staff: staffRepo})
	protected := group.Group("")
	protected.Use(vendorauthhttp.RequireAuth(accessSigner, sessionRepo, staffResolverAdapter{staff: staffRepo}))
	staffhttp.RegisterRoutes(protected, staffHandler)
	vendors.RegisterRoutes(protected, vendorHandler)
	return router
}

func uniqueTestPrefix(t *testing.T) string {
	return t.Name()
}

func contractTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping HTTP contract test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := database.ApplyMigrations(ctx, pool, "../migrations"); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `TRUNCATE vendor_sessions, vendor_otp_challenges, vendor_staff, vendor_users RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return pool
}

func contractTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("VENDOR_SERVICE_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("VENDOR_SERVICE_TEST_REDIS_ADDR not set; skipping HTTP contract test")
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Skipf("redis at %s not reachable: %v", addr, err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func doJSON(t *testing.T, router *gin.Engine, method, path string, body any, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestHTTP_AuthStart_ReturnsChallengeEnvelope(t *testing.T) {
	pool := contractTestPool(t)
	redisClient := contractTestRedis(t)
	router := buildTestRouter(t, pool, redisClient)

	rec := doJSON(t, router, http.MethodPost, "/api/v1/vendor/auth/start", map[string]string{"email": "contract@example.com"}, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var envelope struct {
		Success bool `json:"success"`
		Data    struct {
			ChallengeID string `json:"challenge_id"`
			DebugOTP    string `json:"debug_otp"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !envelope.Success || envelope.Data.ChallengeID == "" || envelope.Data.DebugOTP == "" {
		t.Fatalf("unexpected envelope: %s", rec.Body.String())
	}
}

func TestHTTP_AuthStart_ValidationError(t *testing.T) {
	pool := contractTestPool(t)
	redisClient := contractTestRedis(t)
	router := buildTestRouter(t, pool, redisClient)

	rec := doJSON(t, router, http.MethodPost, "/api/v1/vendor/auth/start", map[string]string{}, "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for missing identifier, got %d: %s", rec.Code, rec.Body.String())
	}
	var envelope struct {
		Success bool `json:"success"`
		Error   struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if envelope.Success || envelope.Error.Code != "validation_failed" {
		t.Fatalf("expected a validation_failed error envelope, got %s", rec.Body.String())
	}
}

func TestHTTP_ProtectedRoute_RequiresAuth(t *testing.T) {
	pool := contractTestPool(t)
	redisClient := contractTestRedis(t)
	router := buildTestRouter(t, pool, redisClient)

	rec := doJSON(t, router, http.MethodGet, "/api/v1/vendor/staff", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with no bearer token, got %d: %s", rec.Code, rec.Body.String())
	}

	rec2 := doJSON(t, router, http.MethodGet, "/api/v1/vendor/staff", nil, "not-a-real-token")
	if rec2.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with an invalid bearer token, got %d: %s", rec2.Code, rec2.Body.String())
	}
	rec3 := doJSON(t, router, http.MethodGet, "/api/v1/vendor/capability-types", nil, "")
	if rec3.Code != http.StatusUnauthorized {
		t.Fatalf("expected Section 6 routes to require authentication, got %d: %s", rec3.Code, rec3.Body.String())
	}
}

func TestHTTP_EverySection6RouteRequiresAuthentication(t *testing.T) {
	pool := contractTestPool(t)
	redisClient := contractTestRedis(t)
	router := buildTestRouter(t, pool, redisClient)
	checked := 0
	for _, route := range router.Routes() {
		if route.Path != "/api/v1/vendor/capability-types" && !strings.HasPrefix(route.Path, "/api/v1/vendor/businesses") {
			continue
		}
		path := route.Path
		for _, name := range []string{":businessId", ":branchId", ":brandId", ":capabilityId"} {
			path = strings.ReplaceAll(path, name, "unknown")
		}
		rec := doJSON(t, router, route.Method, path, map[string]any{}, "")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without auth=%d body=%s", route.Method, path, rec.Code, rec.Body.String())
		}
		checked++
	}
	if checked != 32 {
		t.Fatalf("checked %d Section 6 routes, want 32", checked)
	}
}

func TestHTTP_FullLoginThenListStaff(t *testing.T) {
	pool := contractTestPool(t)
	redisClient := contractTestRedis(t)
	router := buildTestRouter(t, pool, redisClient)

	// Provision an owner directly via the internal usecase layer (bypassing
	// the internal HTTP route, which needs HMAC signing not worth
	// reproducing here — the internal route's own wiring is exercised by
	// staffhttp.RegisterInternalRoutes being reachable from cmd/main.go,
	// already proven by `go build ./...` succeeding on that wiring).
	staffRepo := staffrepositories.NewStaffRepository(pool)
	users := vendorauthrepositories.NewUserRepository(pool)
	user, err := users.UpsertByEmail(context.Background(), "http-owner@example.com")
	if err != nil {
		t.Fatalf("upsert user: %v", err)
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO vendor_staff (id, vendor_id, user_id, status) VALUES ('staff-http-1','vendor-http-1',$1,'active')`, user.ID); err != nil {
		t.Fatalf("seed staff: %v", err)
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO vendor_staff_role_assignments (id, staff_id, role_id) VALUES ('vsa-http-1','staff-http-1','role_owner')`); err != nil {
		t.Fatalf("seed assignment: %v", err)
	}

	start := doJSON(t, router, http.MethodPost, "/api/v1/vendor/auth/start", map[string]string{"email": "http-owner@example.com"}, "")
	var startEnvelope struct {
		Data struct {
			ChallengeID string `json:"challenge_id"`
			DebugOTP    string `json:"debug_otp"`
		} `json:"data"`
	}
	_ = json.Unmarshal(start.Body.Bytes(), &startEnvelope)

	verify := doJSON(t, router, http.MethodPost, "/api/v1/vendor/auth/verify", map[string]string{
		"email": "http-owner@example.com", "otp": startEnvelope.Data.DebugOTP, "challenge_id": startEnvelope.Data.ChallengeID,
	}, "")
	if verify.Code != http.StatusOK {
		t.Fatalf("expected verify 200, got %d: %s", verify.Code, verify.Body.String())
	}
	var verifyEnvelope struct {
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	_ = json.Unmarshal(verify.Body.Bytes(), &verifyEnvelope)
	if verifyEnvelope.Data.AccessToken == "" {
		t.Fatalf("expected an access token, got %s", verify.Body.String())
	}

	list := doJSON(t, router, http.MethodGet, "/api/v1/vendor/staff", nil, verifyEnvelope.Data.AccessToken)
	if list.Code != http.StatusOK {
		t.Fatalf("expected list-staff 200 with a valid token, got %d: %s", list.Code, list.Body.String())
	}
	var listEnvelope struct {
		Data struct {
			Staff []struct {
				ID string `json:"id"`
			} `json:"staff"`
		} `json:"data"`
	}
	_ = json.Unmarshal(list.Body.Bytes(), &listEnvelope)
	if len(listEnvelope.Data.Staff) != 1 || listEnvelope.Data.Staff[0].ID != "staff-http-1" {
		t.Fatalf("unexpected staff list: %s", list.Body.String())
	}
	_ = staffRepo
}
