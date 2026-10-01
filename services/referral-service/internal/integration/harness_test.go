// Package integration_test is referral-service's PostgreSQL-backed suite,
// gated on REFERRAL_TEST_DATABASE_URL (skipped when unset; CI must set it).
package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/referral-service/internal/database"
	referralhttp "cosmicforge/logistics/services/referral-service/internal/features/referrals/http"
	referralmodels "cosmicforge/logistics/services/referral-service/internal/features/referrals/models"
	referralrepositories "cosmicforge/logistics/services/referral-service/internal/features/referrals/repositories"
	referralusecases "cosmicforge/logistics/services/referral-service/internal/features/referrals/usecases"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/outbox"
	"cosmicforge/logistics/shared/go/referralcontract"
	"cosmicforge/logistics/shared/go/serviceauth"
)

const migrationsDir = "../../migrations"

var (
	migrateOnce sync.Once
	migrateErr  error
)

// testPool connects to the disposable test database, applies migrations once
// per process and truncates every table so each test starts clean.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	rawURL := os.Getenv("REFERRAL_TEST_DATABASE_URL")
	if rawURL == "" {
		t.Skip("REFERRAL_TEST_DATABASE_URL not set; skipping PostgreSQL integration tests (CI must set it)")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || !strings.Contains(strings.ToLower(parsed.Path), "test") {
		t.Fatalf("refusing to run against %q: the database name must contain \"test\"", parsed.Path)
	}
	pool, err := database.NewPool(context.Background(), rawURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	migrateOnce.Do(func() { migrateErr = database.ApplyMigrations(context.Background(), pool, migrationsDir) })
	if migrateErr != nil {
		t.Fatalf("apply migrations: %v", migrateErr)
	}
	rows, err := pool.Query(context.Background(), `
		SELECT tablename FROM pg_tables
		WHERE schemaname = 'public' AND tablename <> 'referral_service_schema_migrations'`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	var tables []string
	for rows.Next() {
		var name string
		_ = rows.Scan(&name)
		tables = append(tables, pgx.Identifier{name}.Sanitize())
	}
	rows.Close()
	if len(tables) > 0 {
		if _, err := pool.Exec(context.Background(), "TRUNCATE "+strings.Join(tables, ", ")+" CASCADE"); err != nil {
			t.Fatalf("truncate: %v", err)
		}
	}
	return pool
}

// defaultRules are the approved launch values.
func defaultRules() referralmodels.Rules {
	return referralmodels.Rules{
		Activity:                  referralmodels.ActivityRules{CustomerThreshold: 5, ProviderThreshold: 10, Window: 30 * 24 * time.Hour},
		CustomerReferralBonusKobo: 50_000,
		CustomerWelcomeCreditKobo: 20_000,
		FirstBookingCreditKobo:    100_000,
		ProviderWelcomeCreditKobo: 50_000,
		ProviderCommissionBPS:     1_000,
		ProviderActivationJobs:    5,
		ProviderWindow:            90 * 24 * time.Hour,
		Milestone5CreditKobo:      150_000,
		Milestone10BonusKobo:      750_000,
		ActivityDecisionDelay:     time.Hour,
		ReversalReviewThreshold:   3,
	}
}

func newService(t *testing.T, pool *pgxpool.Pool) *referralusecases.Service {
	t.Helper()
	return newServiceWith(t, pool, defaultRules(), nil)
}

func referralStore(pool *pgxpool.Pool) *referralrepositories.Store {
	return referralrepositories.NewStore(pool)
}

func newServiceWith(t *testing.T, pool *pgxpool.Pool, rules referralmodels.Rules, now func() time.Time) *referralusecases.Service {
	t.Helper()
	return referralusecases.NewService(referralusecases.Options{
		Store:         referralrepositories.NewStore(pool),
		Rules:         rules,
		ShareLinkBase: "https://example.test/r/",
		Now:           now,
	})
}

// envelopeJSON wraps a payload the way a producer's outbox would.
func envelopeJSON(t *testing.T, producer, eventType, aggregateID string, payload any) []byte {
	t.Helper()
	envelope, err := outbox.NewEnvelope(producer, eventType, "test_aggregate", aggregateID, 1, payload)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// jobSettled is a paid-out taxi job for customer/driver at completedAt.
func jobSettled(customerID, driverID string, completedAt time.Time) referralcontract.JobSettled {
	return referralcontract.JobSettled{
		PaymentIntentID: "pi_" + uuid.NewString(), PaymentReference: "ref_" + uuid.NewString(),
		SourceService: "taxi-service", SourceReference: "booking_" + uuid.NewString(),
		CustomerID: customerID, ProviderID: driverID, ProviderType: "taxi",
		AmountKobo: 1_000_000, PlatformFeeKobo: 150_000, ProviderNetKobo: 850_000, Currency: "NGN",
		JobCompletedAt: completedAt, JobCompletedAtSource: referralcontract.CompletedAtFromProvider, SettledAt: completedAt,
	}
}

func mustHandle(t *testing.T, svc *referralusecases.Service, caller string, raw []byte) referralusecases.Outcome {
	t.Helper()
	outcome, err := svc.HandleEvent(context.Background(), caller, raw)
	if err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}
	return outcome
}

func countRows(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count (%s): %v", query, err)
	}
	return n
}

// signedRouter mounts the real internal routes with a verifier that knows
// the given callers' secrets.
func signedRouter(t *testing.T, pool *pgxpool.Pool, svc *referralusecases.Service, secrets serviceauth.Secrets) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(httpx.ErrorHandler())
	referralhttp.RegisterInternalRoutes(router, referralhttp.InternalOptions{
		Service:                svc,
		Verifier:               serviceauth.NewVerifier(secrets, 5*time.Minute),
		NonceStore:             pool,
		AdminCallers:           []string{"admin-backoffice-service"},
		WithdrawalCheckCallers: []string{"payment-wallet-service"},
	})
	return router
}

// signedRequest builds an HMAC-signed request as the given caller.
func signedRequest(t *testing.T, method, path, caller string, secret []byte, body []byte) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if err := serviceauth.SignRequest(req, caller, secret, body, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	return req
}
