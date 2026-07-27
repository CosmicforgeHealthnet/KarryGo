package marketplace_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	authmodels "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/models"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/marketplace"
)

func delegatedFlags() marketplace.Flags {
	flags := providerFlags()
	flags.ReservationEnabled = true
	flags.ProviderEligibilityEnabled = true
	flags.OutboxEnabled = true
	return flags
}

// seedEligibleProvider creates a provider dispatch considers eligible.
func seedEligibleProvider(t *testing.T, pool *pgxpool.Pool, phone string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO providers (id, phone, verification_status, is_active, onboarding_complete, operation_type)
		VALUES (gen_random_uuid(), $1, 'verified', true, true, 'fleet') RETURNING id`, phone).Scan(&id)
	if err != nil {
		t.Fatalf("seed provider: %v", err)
	}
	return id
}

func delegatedBody(clientResponseID string, requestID string) map[string]any {
	return map[string]any{
		"clientResponseId":         clientResponseID,
		"requestId":                requestID,
		"organizationId":           "org_A",
		"organizationMembershipId": "mem_1",
	}
}

// TestDelegatedAcceptanceCommitsReservationAndRelayTogether is the atomicity
// proof: both rows exist and both name the accepting provider.
func TestDelegatedAcceptanceCommitsReservationAndRelayTogether(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	requestID := seedBroadcast(t, pool)
	provider := seedEligibleProvider(t, pool, "+2348070000001")

	server := newProviderServer(t, pool, acceptingResponder(),
		providerSession(provider, "sess-1", authmodels.RoleDispatchProvider), delegatedFlags())

	status, body := providerCall(t, server, http.MethodPost,
		"/api/v1/provider/organization-offers/off-del-1/accept",
		delegatedBody("c-del-1", requestID))
	if status != http.StatusOK {
		t.Fatalf("delegated accept = %d (%v)", status, body)
	}

	// Exactly one reservation, PROVIDER_DIRECT, naming this provider.
	var reservations int
	_ = pool.QueryRow(ctx,
		`SELECT count(*) FROM dispatch_organization_reservations`).Scan(&reservations)
	if reservations != 1 {
		t.Fatalf("reservations = %d, want exactly 1", reservations)
	}
	var source, acceptingProvider string
	if err := pool.QueryRow(ctx, `
		SELECT acceptance_source, COALESCE(accepting_provider_id::text, '')
		FROM dispatch_organization_reservations`).Scan(&source, &acceptingProvider); err != nil {
		t.Fatalf("load reservation: %v", err)
	}
	if source != "PROVIDER_DIRECT" || acceptingProvider != provider {
		t.Fatalf("reservation source=%s provider=%s", source, acceptingProvider)
	}

	// Exactly one relay, of the offer-accept kind, for the same provider.
	var relays int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM dispatch_fleet_relay_operations`).Scan(&relays)
	if relays != 1 {
		t.Fatalf("relays = %d, want exactly 1", relays)
	}
	var operationType, relayProvider string
	_ = pool.QueryRow(ctx,
		`SELECT operation_type, provider_id FROM dispatch_fleet_relay_operations`).
		Scan(&operationType, &relayProvider)
	if operationType != marketplace.RelayProviderOfferAccept || relayProvider != provider {
		t.Fatalf("relay type=%s provider=%s", operationType, relayProvider)
	}
}

// TestDelegatedAcceptanceLeavesNoOrphanWhenReservationLost proves the other
// half of atomicity: a provider who loses the reservation race must not leave
// a relay claiming a decision on a request their organization never got.
func TestDelegatedAcceptanceLeavesNoOrphanWhenReservationLost(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	requestID := seedBroadcast(t, pool)
	first := seedEligibleProvider(t, pool, "+2348071000001")
	second := seedEligibleProvider(t, pool, "+2348071000002")

	firstServer := newProviderServer(t, pool, acceptingResponder(),
		providerSession(first, "sess-1", authmodels.RoleDispatchProvider), delegatedFlags())
	if status, resp := providerCall(t, firstServer, http.MethodPost,
		"/api/v1/provider/organization-offers/off-a/accept",
		delegatedBody("c-a", requestID)); status != http.StatusOK {
		t.Fatalf("first accept = %d (%v)", status, resp)
	}

	secondServer := newProviderServer(t, pool, acceptingResponder(),
		providerSession(second, "sess-2", authmodels.RoleDispatchProvider), delegatedFlags())
	status, resp := providerCall(t, secondServer, http.MethodPost,
		"/api/v1/provider/organization-offers/off-b/accept",
		delegatedBody("c-b", requestID))
	if status == http.StatusOK {
		t.Fatalf("a second provider reserved an already-claimed request (%v)", resp)
	}

	var reservations, relays int
	_ = pool.QueryRow(ctx,
		`SELECT count(*) FROM dispatch_organization_reservations`).Scan(&reservations)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM dispatch_fleet_relay_operations`).Scan(&relays)
	if reservations != 1 {
		t.Fatalf("reservations = %d, want 1", reservations)
	}
	if relays != 1 {
		t.Fatalf("relays = %d, want 1; the losing provider left an orphan relay", relays)
	}
}

// TestDelegatedAcceptanceOutageKeepsBothAndReturnsPending proves the outage
// path end to end: both rows survive, the provider is told PENDING, and
// recovery reuses the original key without duplicating anything.
func TestDelegatedAcceptanceOutageKeepsBothAndReturnsPending(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	requestID := seedBroadcast(t, pool)
	provider := seedEligibleProvider(t, pool, "+2348072000001")

	down := &scriptedResponder{configured: true, err: marketplace.ErrFleetUnavailable}
	server := newProviderServer(t, pool, down,
		providerSession(provider, "sess-1", authmodels.RoleDispatchProvider), delegatedFlags())

	status, body := providerCall(t, server, http.MethodPost,
		"/api/v1/provider/organization-offers/off-out/accept",
		delegatedBody("c-out", requestID))
	// The provider's decision and the reservation are both durable, so this is
	// a success with a pending status — not an error they might retry as a new
	// decision.
	if status != http.StatusOK {
		t.Fatalf("outage accept = %d (%v)", status, body)
	}
	data, _ := body["data"].(map[string]any)
	if data["status"] != marketplace.ProviderActionPending {
		t.Fatalf("status = %v, want PENDING", data["status"])
	}

	var reservations int
	_ = pool.QueryRow(ctx,
		`SELECT count(*) FROM dispatch_organization_reservations`).Scan(&reservations)
	if reservations != 1 {
		t.Fatalf("the reservation was lost during the outage (count=%d)", reservations)
	}
	var relayStatus, key string
	if err := pool.QueryRow(ctx,
		`SELECT status, idempotency_key FROM dispatch_fleet_relay_operations`).
		Scan(&relayStatus, &key); err != nil {
		t.Fatalf("the relay was lost during the outage: %v", err)
	}
	if relayStatus != marketplace.RelayUnknownOutcome {
		t.Fatalf("relay status = %s, want UNKNOWN_OUTCOME", relayStatus)
	}
	if key != "c-out" {
		t.Fatalf("idempotency key = %s; the original must be preserved", key)
	}

	// Fleet recovers. The existing worker delivers the SAME decision, and the
	// provider is never asked to accept again.
	if _, err := pool.Exec(ctx,
		`UPDATE dispatch_fleet_relay_operations SET next_attempt_at = now()`); err != nil {
		t.Fatalf("make due: %v", err)
	}
	up := &scriptedResponder{configured: true,
		result: marketplace.RelayDeliveryResult{Accepted: true}}
	worker := marketplace.NewRelayWorker(marketplace.NewRepository(pool), up,
		marketplace.RelayWorkerConfig{BaseBackoff: time.Millisecond})
	if _, err := worker.RunOnce(ctx); err != nil {
		t.Fatalf("worker: %v", err)
	}
	if len(up.seen) != 1 || up.seen[0].IdempotencyKey != "c-out" {
		t.Fatalf("recovery deliveries = %+v; must reuse the original key", up.seen)
	}

	var relays int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM dispatch_fleet_relay_operations`).Scan(&relays)
	if relays != 1 {
		t.Fatalf("relays = %d after recovery; the decision was duplicated", relays)
	}
	_ = pool.QueryRow(ctx,
		`SELECT count(*) FROM dispatch_organization_reservations`).Scan(&reservations)
	if reservations != 1 {
		t.Fatalf("reservations = %d after recovery; the request was reserved twice", reservations)
	}
}

// TestDelegatedDeclineDoesNotReserve proves a decline is not an acceptance:
// it records the decision but claims nothing for the organization.
func TestDelegatedDeclineDoesNotReserve(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	provider := seedEligibleProvider(t, pool, "+2348073000001")
	server := newProviderServer(t, pool, acceptingResponder(),
		providerSession(provider, "sess-1", authmodels.RoleDispatchProvider), delegatedFlags())

	status, body := providerCall(t, server, http.MethodPost,
		"/api/v1/provider/organization-offers/off-dec/decline", map[string]any{
			"clientResponseId": "c-dec", "reasonCode": "EXCESSIVE_DISTANCE",
		})
	if status != http.StatusOK {
		t.Fatalf("decline = %d (%v)", status, body)
	}

	var reservations int
	_ = pool.QueryRow(ctx,
		`SELECT count(*) FROM dispatch_organization_reservations`).Scan(&reservations)
	if reservations != 0 {
		t.Fatalf("a decline reserved the request (%d reservations)", reservations)
	}

	var relays int
	var operationType string
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM dispatch_fleet_relay_operations`).Scan(&relays)
	_ = pool.QueryRow(ctx,
		`SELECT operation_type FROM dispatch_fleet_relay_operations`).Scan(&operationType)
	if relays != 1 || operationType != marketplace.RelayProviderOfferDecline {
		t.Fatalf("relays = %d type = %s", relays, operationType)
	}
}
