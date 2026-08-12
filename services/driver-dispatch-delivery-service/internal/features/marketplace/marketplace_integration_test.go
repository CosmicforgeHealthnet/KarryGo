package marketplace_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/marketplace"
)

const (
	fleetService = "fleet-partner-service"
	fleetSecret  = "test-fleet-secret-value"
)

// testPool connects to the disposable dispatch database.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DISPATCH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("DISPATCH_TEST_DATABASE_URL not set; skipping dispatch PostgreSQL tests (CI must set it)")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect dispatch test database: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("ping dispatch test database: %v", err)
	}
	// Clean only the marketplace tables plus the fixtures this suite creates.
	if _, err := pool.Exec(context.Background(), `
		TRUNCATE dispatch_marketplace_outbox_deliveries, dispatch_marketplace_outbox,
			dispatch_marketplace_command_operations, dispatch_organization_reservations,
			dispatch_fleet_relay_operations, trips, request_broadcasts, bikes, providers CASCADE`); err != nil {
		t.Fatalf("clean dispatch test database: %v", err)
	}
	return pool
}

// seedBroadcast inserts a claimable canonical request.
func seedBroadcast(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO request_broadcasts (booking_id, broadcast_radius_km, expires_at, booking_payload)
		VALUES (gen_random_uuid(), 5, now() + interval '30 minutes', '{}'::jsonb)
		RETURNING id`).Scan(&id)
	if err != nil {
		t.Fatalf("seed broadcast: %v", err)
	}
	return id
}

// newServer builds a real gin server with real service authentication.
func newServer(t *testing.T, pool *pgxpool.Pool, flags marketplace.Flags) *httptest.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(httpx.RequestID(), httpx.ErrorHandler())

	repo := marketplace.NewRepository(pool)
	service := marketplace.NewService(repo, flags, nil)
	handler := marketplace.NewHandler(service, flags)
	verifier := serviceauth.NewVerifier(
		serviceauth.Secrets{fleetService: []byte(fleetSecret)}, time.Minute)

	marketplace.RegisterInternalRoutes(
		engine.Group("/api/v1/internal/marketplace"), handler, verifier, []string{fleetService})

	server := httptest.NewServer(engine)
	t.Cleanup(server.Close)
	return server
}

// signedCall performs a properly signed internal request.
func signedCall(t *testing.T, server *httptest.Server, method string, path string,
	body any, idempotencyKey string, serviceName string, secret string) (int, map[string]any) {
	t.Helper()
	var encoded []byte
	if body != nil {
		var err error
		encoded, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req, err := http.NewRequest(method, server.URL+path, bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if idempotencyKey != "" {
		req.Header.Set(marketplace.HeaderIdempotencyKey, idempotencyKey)
	}
	if serviceName != "" {
		if err := serviceauth.SignRequest(req, serviceName, []byte(secret), encoded, time.Now()); err != nil {
			t.Fatalf("sign request: %v", err)
		}
	}

	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("perform request: %v", err)
	}
	defer resp.Body.Close()

	var decoded map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&decoded)
	return resp.StatusCode, decoded
}

func enabledFlags() marketplace.Flags {
	return marketplace.Flags{
		InternalAPIEnabled: true,
		ReservationEnabled: true,
		OutboxEnabled:      true,
	}
}

func dataOf(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("response has no data object: %v", body)
	}
	return data
}

func errorCodeOf(body map[string]any) string {
	errObj, ok := body["error"].(map[string]any)
	if !ok {
		return ""
	}
	code, _ := errObj["code"].(string)
	return code
}

func TestReservationRequiresServiceAuthentication(t *testing.T) {
	pool := testPool(t)
	server := newServer(t, pool, enabledFlags())
	requestID := seedBroadcast(t, pool)
	body := map[string]any{
		"requestId": requestID, "organizationId": "org_A",
		"acceptanceSource": "ORGANIZATION_DISPATCHER",
	}

	// Unsigned.
	status, _ := signedCall(t, server, http.MethodPost, "/api/v1/internal/marketplace/reservations",
		body, "idem-unsigned", "", "")
	if status != http.StatusUnauthorized {
		t.Fatalf("unsigned call status = %d, want 401", status)
	}

	// Signed with the wrong secret.
	status, _ = signedCall(t, server, http.MethodPost, "/api/v1/internal/marketplace/reservations",
		body, "idem-badsecret", fleetService, "the-wrong-secret")
	if status != http.StatusUnauthorized {
		t.Fatalf("bad-signature status = %d, want 401", status)
	}

	// Signed by a service that is not on the allowlist.
	status, _ = signedCall(t, server, http.MethodPost, "/api/v1/internal/marketplace/reservations",
		body, "idem-stranger", "customer-service", fleetSecret)
	if status != http.StatusForbidden && status != http.StatusUnauthorized {
		t.Fatalf("unknown-caller status = %d, want 401 or 403", status)
	}

	// Nothing was created by any of the rejected calls.
	var count int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM dispatch_organization_reservations`).Scan(&count); err != nil {
		t.Fatalf("count reservations: %v", err)
	}
	if count != 0 {
		t.Fatalf("rejected calls created %d reservations", count)
	}
}

func TestReserveThenIdempotentReplay(t *testing.T) {
	pool := testPool(t)
	server := newServer(t, pool, enabledFlags())
	requestID := seedBroadcast(t, pool)
	body := map[string]any{
		"requestId": requestID, "organizationId": "org_A",
		"acceptanceSource": "ORGANIZATION_DISPATCHER",
	}

	status, first := signedCall(t, server, http.MethodPost,
		"/api/v1/internal/marketplace/reservations", body, "idem-1", fleetService, fleetSecret)
	if status != http.StatusCreated {
		t.Fatalf("reserve status = %d, want 201 (%v)", status, first)
	}
	firstData := dataOf(t, first)
	if firstData["status"] != "RESERVED" {
		t.Fatalf("reservation status = %v, want RESERVED", firstData["status"])
	}

	// The same key replays to the same reservation. This is what makes a lost
	// response safe rather than a double claim.
	status, second := signedCall(t, server, http.MethodPost,
		"/api/v1/internal/marketplace/reservations", body, "idem-1", fleetService, fleetSecret)
	if status != http.StatusCreated {
		t.Fatalf("replay status = %d (%v)", status, second)
	}
	if dataOf(t, second)["reservationId"] != firstData["reservationId"] {
		t.Fatal("replay returned a different reservation")
	}

	var count int
	_ = pool.QueryRow(context.Background(),
		`SELECT count(*) FROM dispatch_organization_reservations`).Scan(&count)
	if count != 1 {
		t.Fatalf("reservation rows = %d, want exactly 1", count)
	}

	// Read-back by idempotency key resolves an unknown outcome.
	status, readBack := signedCall(t, server, http.MethodGet,
		"/api/v1/internal/marketplace/reservations/by-idempotency/idem-1", nil, "", fleetService, fleetSecret)
	if status != http.StatusOK {
		t.Fatalf("read-back status = %d (%v)", status, readBack)
	}
	if dataOf(t, readBack)["reservationId"] != firstData["reservationId"] {
		t.Fatal("read-back returned a different reservation")
	}
}

func TestSameKeyDifferentPayloadIsIdempotencyConflict(t *testing.T) {
	pool := testPool(t)
	server := newServer(t, pool, enabledFlags())
	requestID := seedBroadcast(t, pool)

	status, _ := signedCall(t, server, http.MethodPost, "/api/v1/internal/marketplace/reservations",
		map[string]any{"requestId": requestID, "organizationId": "org_A",
			"acceptanceSource": "ORGANIZATION_DISPATCHER"}, "idem-x", fleetService, fleetSecret)
	if status != http.StatusCreated {
		t.Fatalf("first reserve status = %d", status)
	}

	// Same key, different organization: this must not be treated as a replay.
	status, body := signedCall(t, server, http.MethodPost, "/api/v1/internal/marketplace/reservations",
		map[string]any{"requestId": requestID, "organizationId": "org_B",
			"acceptanceSource": "ORGANIZATION_DISPATCHER"}, "idem-x", fleetService, fleetSecret)
	if status != http.StatusConflict || errorCodeOf(body) != "idempotency_conflict" {
		t.Fatalf("conflict status = %d code = %q, want 409 idempotency_conflict", status, errorCodeOf(body))
	}
}

func TestTwoOrganizationsRaceExactlyOneWins(t *testing.T) {
	pool := testPool(t)
	server := newServer(t, pool, enabledFlags())
	requestID := seedBroadcast(t, pool)

	const racers = 6
	var wg sync.WaitGroup
	statuses := make([]int, racers)
	codes := make([]string, racers)

	for index := 0; index < racers; index++ {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			org := fmt.Sprintf("org_%d", slot)
			statuses[slot], _ = func() (int, map[string]any) {
				status, body := signedCall(t, server, http.MethodPost,
					"/api/v1/internal/marketplace/reservations",
					map[string]any{"requestId": requestID, "organizationId": org,
						"acceptanceSource": "ORGANIZATION_DISPATCHER"},
					fmt.Sprintf("idem-race-%d", slot), fleetService, fleetSecret)
				codes[slot] = errorCodeOf(body)
				return status, body
			}()
		}(index)
	}
	wg.Wait()

	winners, claimed := 0, 0
	for index := 0; index < racers; index++ {
		switch statuses[index] {
		case http.StatusCreated:
			winners++
		case http.StatusConflict:
			if codes[index] == "request_already_claimed" {
				claimed++
			}
		}
	}
	if winners != 1 {
		t.Fatalf("winners = %d, want exactly 1 (statuses=%v codes=%v)", winners, statuses, codes)
	}
	if claimed != racers-1 {
		t.Fatalf("request_already_claimed = %d, want %d (statuses=%v codes=%v)",
			claimed, racers-1, statuses, codes)
	}

	var live int
	_ = pool.QueryRow(context.Background(),
		`SELECT count(*) FROM dispatch_organization_reservations
		 WHERE status IN ('PENDING','RESERVED')`).Scan(&live)
	if live != 1 {
		t.Fatalf("live reservations = %d, want exactly 1", live)
	}
}

func TestReserveRejectsStaleVersionAndExpiredRequest(t *testing.T) {
	pool := testPool(t)
	server := newServer(t, pool, enabledFlags())
	requestID := seedBroadcast(t, pool)

	status, body := signedCall(t, server, http.MethodPost,
		"/api/v1/internal/marketplace/reservations",
		map[string]any{"requestId": requestID, "organizationId": "org_A",
			"acceptanceSource": "ORGANIZATION_DISPATCHER", "expectedRequestVersion": 99},
		"idem-stale", fleetService, fleetSecret)
	if status != http.StatusConflict || errorCodeOf(body) != "source_version_mismatch" {
		t.Fatalf("stale version = %d %q, want 409 source_version_mismatch", status, errorCodeOf(body))
	}

	// An expired request cannot be claimed.
	expiredID := seedBroadcast(t, pool)
	if _, err := pool.Exec(context.Background(),
		`UPDATE request_broadcasts SET expires_at = now() - interval '1 minute' WHERE id = $1`,
		expiredID); err != nil {
		t.Fatalf("expire request: %v", err)
	}
	status, body = signedCall(t, server, http.MethodPost,
		"/api/v1/internal/marketplace/reservations",
		map[string]any{"requestId": expiredID, "organizationId": "org_A",
			"acceptanceSource": "ORGANIZATION_DISPATCHER"}, "idem-expired", fleetService, fleetSecret)
	if status != http.StatusConflict || errorCodeOf(body) != "request_expired" {
		t.Fatalf("expired request = %d %q, want 409 request_expired", status, errorCodeOf(body))
	}
}

func TestProviderDirectReservationRequiresProvider(t *testing.T) {
	pool := testPool(t)
	server := newServer(t, pool, enabledFlags())
	requestID := seedBroadcast(t, pool)

	// Delegated acceptance without naming the provider is unattributable.
	status, _ := signedCall(t, server, http.MethodPost,
		"/api/v1/internal/marketplace/reservations",
		map[string]any{"requestId": requestID, "organizationId": "org_A",
			"acceptanceSource": "PROVIDER_DIRECT"}, "idem-pd", fleetService, fleetSecret)
	if status != http.StatusBadRequest {
		t.Fatalf("provider-direct without provider = %d, want 400", status)
	}

	// A dispatcher reservation must NOT name a provider: the dispatcher has
	// not made any provider's decision for them.
	status, _ = signedCall(t, server, http.MethodPost,
		"/api/v1/internal/marketplace/reservations",
		map[string]any{"requestId": requestID, "organizationId": "org_A",
			"acceptanceSource": "ORGANIZATION_DISPATCHER", "acceptingProviderId": "prov_1"},
		"idem-dispatcher-provider", fleetService, fleetSecret)
	if status != http.StatusBadRequest {
		t.Fatalf("dispatcher reservation naming a provider = %d, want 400", status)
	}
}

func TestReleaseReservationAndIdempotentReplay(t *testing.T) {
	pool := testPool(t)
	server := newServer(t, pool, enabledFlags())
	requestID := seedBroadcast(t, pool)

	_, created := signedCall(t, server, http.MethodPost,
		"/api/v1/internal/marketplace/reservations",
		map[string]any{"requestId": requestID, "organizationId": "org_A",
			"acceptanceSource": "ORGANIZATION_DISPATCHER"}, "idem-rel", fleetService, fleetSecret)
	reservationID, _ := dataOf(t, created)["reservationId"].(string)

	path := "/api/v1/internal/marketplace/reservations/" + reservationID + "/release"
	status, released := signedCall(t, server, http.MethodPost, path,
		map[string]any{"reasonCode": "ASSIGNMENT_EXPIRED"}, "idem-rel-1", fleetService, fleetSecret)
	if status != http.StatusOK {
		t.Fatalf("release status = %d (%v)", status, released)
	}
	if dataOf(t, released)["status"] != "RELEASED" {
		t.Fatalf("status after release = %v", dataOf(t, released)["status"])
	}

	// Replaying the same release returns the original result.
	status, replay := signedCall(t, server, http.MethodPost, path,
		map[string]any{"reasonCode": "ASSIGNMENT_EXPIRED"}, "idem-rel-1", fleetService, fleetSecret)
	if status != http.StatusOK || dataOf(t, replay)["status"] != "RELEASED" {
		t.Fatalf("release replay = %d %v", status, replay)
	}

	// An unsupported reason is refused.
	status, _ = signedCall(t, server, http.MethodPost, path,
		map[string]any{"reasonCode": "BECAUSE_I_SAID_SO"}, "idem-rel-2", fleetService, fleetSecret)
	if status != http.StatusBadRequest {
		t.Fatalf("invalid release reason = %d, want 400", status)
	}

	// After release the request is claimable again by another organization.
	status, _ = signedCall(t, server, http.MethodPost,
		"/api/v1/internal/marketplace/reservations",
		map[string]any{"requestId": requestID, "organizationId": "org_B",
			"acceptanceSource": "ORGANIZATION_DISPATCHER"}, "idem-rel-3", fleetService, fleetSecret)
	if status != http.StatusCreated {
		t.Fatalf("reclaim after release = %d, want 201", status)
	}
}

func TestCanonicalRequestReadExcludesCommercialData(t *testing.T) {
	pool := testPool(t)
	server := newServer(t, pool, enabledFlags())
	requestID := seedBroadcast(t, pool)

	status, body := signedCall(t, server, http.MethodGet,
		"/api/v1/internal/marketplace/requests/"+requestID, nil, "", fleetService, fleetSecret)
	if status != http.StatusOK {
		t.Fatalf("canonical read status = %d (%v)", status, body)
	}

	// The whole response, including nesting, must be free of commercial data.
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	for _, forbidden := range []string{
		"fare_amount", "fareAmount", "customerFare", "customerPrice", "quotedPrice",
		"organizationSettlement", "companySettlement", "platformCommission",
		"commissionPercentage", "profit", "margin", "contractTerms",
		"customerWallet", "organizationWallet",
	} {
		if bytes.Contains(encoded, []byte(forbidden)) {
			t.Fatalf("canonical request read leaked %q: %s", forbidden, encoded)
		}
	}
}

func TestReservationDisabledFailsClosed(t *testing.T) {
	pool := testPool(t)
	// Flags all false: an unconfigured deployment must refuse, not half-work.
	server := newServer(t, pool, marketplace.Flags{})
	requestID := seedBroadcast(t, pool)

	status, _ := signedCall(t, server, http.MethodPost,
		"/api/v1/internal/marketplace/reservations",
		map[string]any{"requestId": requestID, "organizationId": "org_A",
			"acceptanceSource": "ORGANIZATION_DISPATCHER"}, "idem-off", fleetService, fleetSecret)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("disabled reservation = %d, want 503", status)
	}

	var count int
	_ = pool.QueryRow(context.Background(),
		`SELECT count(*) FROM dispatch_organization_reservations`).Scan(&count)
	if count != 0 {
		t.Fatalf("disabled flag still created %d reservations", count)
	}
}

func TestReservationWritesOutboxEventInSameTransaction(t *testing.T) {
	pool := testPool(t)
	server := newServer(t, pool, enabledFlags())
	requestID := seedBroadcast(t, pool)

	status, _ := signedCall(t, server, http.MethodPost,
		"/api/v1/internal/marketplace/reservations",
		map[string]any{"requestId": requestID, "organizationId": "org_A",
			"acceptanceSource": "ORGANIZATION_DISPATCHER"}, "idem-outbox", fleetService, fleetSecret)
	if status != http.StatusCreated {
		t.Fatalf("reserve status = %d", status)
	}

	var eventType, payloadHash string
	err := pool.QueryRow(context.Background(),
		`SELECT event_type, payload_hash FROM dispatch_marketplace_outbox
		 WHERE aggregate_id = $1`, requestID).Scan(&eventType, &payloadHash)
	if err != nil {
		t.Fatalf("outbox row missing after reservation: %v", err)
	}
	if eventType != "platform.request.reserved.v1" {
		t.Fatalf("event type = %q", eventType)
	}
	if payloadHash == "" {
		t.Fatal("outbox payload hash must be set for delivery integrity")
	}

	// A rejected reservation must leave no event behind.
	status, _ = signedCall(t, server, http.MethodPost,
		"/api/v1/internal/marketplace/reservations",
		map[string]any{"requestId": requestID, "organizationId": "org_B",
			"acceptanceSource": "ORGANIZATION_DISPATCHER"}, "idem-outbox-2", fleetService, fleetSecret)
	if status != http.StatusConflict {
		t.Fatalf("second claim = %d, want 409", status)
	}
	var events int
	_ = pool.QueryRow(context.Background(),
		`SELECT count(*) FROM dispatch_marketplace_outbox WHERE aggregate_id = $1`,
		requestID).Scan(&events)
	if events != 1 {
		t.Fatalf("outbox events = %d, want 1; a rejected command must not emit an event", events)
	}
}
