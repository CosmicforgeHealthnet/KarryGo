package integration_test

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/integrations"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

type section55Resolver struct{}

func (section55Resolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
}

func newSection55Service(t *testing.T, pool *pgxpool.Pool) *integrations.Service {
	t.Helper()
	if pool == nil {
		t.Fatal("invalid test pool")
	}
	validator := &integrations.DestinationValidator{
		Resolver: section55Resolver{}, Production: false, AllowHTTP: true,
	}
	return newSection55ServiceWith(t, pool, validator, nil, 3)
}

func newSection55ServiceWith(t *testing.T, pool *pgxpool.Pool, validator *integrations.DestinationValidator, client *http.Client, disableAfter int) *integrations.Service {
	t.Helper()
	return integrations.NewService(integrations.Options{
		Pool: pool,
		Config: config.Config{
			AppEnv:                        "test",
			OrganizationAPIClientsEnabled: true,
			WebhooksEnabled:               true,
			WebhookDeliveryWorkerEnabled:  true,
			APIClientHashKey:              []byte("0123456789abcdef0123456789abcdef"),
			WebhookSecretEncryptionKey:    []byte("abcdef0123456789abcdef0123456789"),
			WebhookRotationOverlapMax:     time.Hour,
			WebhookDeliveryTimeout:        time.Second,
			WebhookDeliveryMaxAttempts:    5,
			WebhookDisableAfterFailures:   disableAfter,
			WebhookFanoutBatchSize:        25,
			WebhookFanoutInterval:         time.Second,
		},
		StepUp:               &fakeStepUpVerifier{validReference: "step_ok"},
		DestinationValidator: validator,
		HTTPClient:           client,
	})
}

func TestSection55APIClientLifecycleTenantIsolationAndSecretStorage(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	orgA := createOrganization(t, pool, "Section55 API A", "RC-S55-API-A")
	orgB := createOrganization(t, pool, "Section55 API B", "RC-S55-API-B")
	svc := newSection55Service(t, pool)
	if err := svc.Ready(); err != nil {
		t.Fatalf("enabled Section 55 service is not ready: %v", err)
	}
	actor := integrations.Actor{Tenant: tenant.Context{OrganizationID: orgA.ID, OrganizationUserID: "ou_s55_a", ActorType: tenant.ActorTypeOrganizationUser, ActorID: "ou_s55_a", CorrelationID: "cor_s55_api"}, IdentityID: "idn_s55_a", SessionID: "ses_s55_a", StepUpReference: "step_ok"}

	client, secret, replay, err := svc.CreateAPIClient(ctx, actor, integrations.CreateAPIClientInput{
		Name: "Warehouse connector", Scopes: []string{integrations.ScopeIntegrationContextRead},
		IPAllowlist: []string{"203.0.113.0/24"}, IdempotencyKey: "idem-api-create",
	})
	if err != nil || replay || secret == "" {
		t.Fatalf("create API client: replay=%v secret=%t err=%v", replay, secret != "", err)
	}
	var storedHash []byte
	if err = pool.QueryRow(ctx, `SELECT secret_hash FROM fleet_partner_organization_api_clients WHERE id=$1`, client.ID).Scan(&storedHash); err != nil {
		t.Fatal(err)
	}
	if string(storedHash) == secret {
		t.Fatal("API client plaintext secret was persisted")
	}
	_, replaySecret, replay, err := svc.CreateAPIClient(ctx, actor, integrations.CreateAPIClientInput{
		Name: "Warehouse connector", Scopes: []string{integrations.ScopeIntegrationContextRead},
		IPAllowlist: []string{"203.0.113.0/24"}, IdempotencyKey: "idem-api-create",
	})
	if err != nil || !replay || replaySecret != "" {
		t.Fatalf("idempotent replay leaked or regenerated secret: replay=%v secret=%q err=%v", replay, replaySecret, err)
	}

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	ginCtx.Request = httptest.NewRequest("GET", "/fleet-partner/integrations/v1/context", nil)
	if _, err = svc.Authenticate(ginCtx, client.ClientID, secret, "203.0.113.42", integrations.ScopeIntegrationContextRead); err != nil {
		t.Fatalf("valid client authentication failed: %v", err)
	}
	if _, err = svc.Authenticate(ginCtx, client.ClientID, "wrong", "203.0.113.42", integrations.ScopeIntegrationContextRead); err == nil {
		t.Fatal("wrong secret authenticated")
	}
	if _, err = svc.Authenticate(ginCtx, client.ClientID, secret, "198.51.100.1", integrations.ScopeIntegrationContextRead); err == nil {
		t.Fatal("IP allowlist bypassed")
	}

	if items, listErr := svc.ListAPIClients(ctx, orgB.ID); listErr != nil || len(items) != 0 {
		t.Fatalf("cross-tenant API client visible: count=%d err=%v", len(items), listErr)
	}
	rotated, newSecret, replay, err := svc.RotateAPIClient(ctx, actor, client.ID, client.Version, "idem-api-rotate", 10*time.Minute)
	if err != nil || replay || newSecret == "" || newSecret == secret {
		t.Fatalf("rotate API client: replay=%v err=%v", replay, err)
	}
	if _, err = svc.Authenticate(ginCtx, client.ClientID, secret, "203.0.113.42", integrations.ScopeIntegrationContextRead); err != nil {
		t.Fatalf("previous secret was not honored during overlap: %v", err)
	}
	if _, err = svc.Authenticate(ginCtx, client.ClientID, newSecret, "203.0.113.42", integrations.ScopeIntegrationContextRead); err != nil {
		t.Fatalf("rotated secret did not authenticate: %v", err)
	}
	if _, err = svc.Authenticate(ginCtx, client.ClientID, newSecret, "203.0.113.42", "integration.context.write"); err == nil {
		t.Fatal("missing exact scope was accepted")
	}
	if _, err = pool.Exec(ctx, `UPDATE fleet_partner_organization_api_clients SET previous_secret_valid_until=now()-interval '1 second' WHERE id=$1`, client.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Authenticate(ginCtx, client.ClientID, secret, "203.0.113.42", integrations.ScopeIntegrationContextRead); err == nil {
		t.Fatal("previous secret authenticated after the overlap expired")
	}
	actorB := actor
	actorB.Tenant.OrganizationID = orgB.ID
	actorB.Tenant.OrganizationUserID = "ou_s55_b"
	if _, _, _, err = svc.RotateAPIClient(ctx, actorB, client.ID, rotated.Version, "idem-api-cross-tenant", 0); err == nil {
		t.Fatal("cross-tenant API client rotation succeeded")
	}
	revoked, replay, err := svc.RevokeAPIClient(ctx, actor, client.ID, rotated.Version, "idem-api-revoke")
	if err != nil || replay || revoked.Status != integrations.APIClientStatusRevoked {
		t.Fatalf("revoke API client: status=%s replay=%v err=%v", revoked.Status, replay, err)
	}
	if _, err = svc.Authenticate(ginCtx, client.ClientID, newSecret, "203.0.113.42", integrations.ScopeIntegrationContextRead); err == nil {
		t.Fatal("revoked credential authenticated")
	}
	var lastUsed *time.Time
	if err = pool.QueryRow(ctx, `SELECT last_used_at FROM fleet_partner_organization_api_clients WHERE id=$1`, client.ID).Scan(&lastUsed); err != nil || lastUsed == nil {
		t.Fatalf("last-used timestamp was not persisted: %v %v", lastUsed, err)
	}
	var audits, lifecycleEvents int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_audit_records WHERE organization_id=$1 AND resource_id=$2`, orgA.ID, client.ID).Scan(&audits)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_outbox WHERE organization_id=$1 AND aggregate_id=$2`, orgA.ID, client.ID).Scan(&lifecycleEvents)
	if audits < 4 || lifecycleEvents != 3 {
		t.Fatalf("audit/outbox evidence incomplete: audits=%d events=%d", audits, lifecycleEvents)
	}
}

func TestSection55WebhookFanoutEncryptionAndReplay(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "Section55 Webhook", "RC-S55-WEBHOOK")
	svc := newSection55Service(t, pool)
	actor := integrations.Actor{Tenant: tenant.Context{OrganizationID: org.ID, OrganizationUserID: "ou_s55_w", ActorType: tenant.ActorTypeOrganizationUser, ActorID: "ou_s55_w", CorrelationID: "cor_s55_webhook"}, IdentityID: "idn_s55_w", SessionID: "ses_s55_w", StepUpReference: "step_ok"}
	publicEvent := "organization.status_changed.v1"
	sub, secret, replay, err := svc.CreateWebhook(ctx, actor, integrations.CreateWebhookInput{
		EndpointURL: "http://public.example/webhook", EventTypes: []string{publicEvent}, IdempotencyKey: "idem-webhook-create",
	})
	if err != nil || replay || secret == "" {
		t.Fatalf("create webhook: replay=%v secret=%t err=%v", replay, secret != "", err)
	}
	var ciphertext []byte
	if err = pool.QueryRow(ctx, `SELECT secret_ciphertext FROM fleet_partner_webhook_subscriptions WHERE id=$1`, sub.ID).Scan(&ciphertext); err != nil {
		t.Fatal(err)
	}
	if string(ciphertext) == secret {
		t.Fatal("webhook plaintext secret was persisted")
	}
	if _, _, _, err = svc.CreateWebhook(ctx, actor, integrations.CreateWebhookInput{EndpointURL: "http://169.254.169.254/hook", EventTypes: []string{publicEvent}, IdempotencyKey: "idem-webhook-ssrf"}); err == nil {
		t.Fatal("metadata-service webhook destination accepted")
	}
	if _, _, _, err = svc.CreateWebhook(ctx, actor, integrations.CreateWebhookInput{EndpointURL: "http://public.example/hook", EventTypes: []string{"api_client.created.v1"}, IdempotencyKey: "idem-webhook-internal-event"}); err == nil {
		t.Fatal("internal lifecycle event was exposed to webhook subscription")
	}

	publisher := events.NewDomainPublisher(events.NewOutboxRepository(pool))
	err = database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
		return publisher.Publish(ctx, tx, publicEvent, "organization", org.ID, 2, org.ID, "cor_s55_webhook",
			events.ContractPayload{SubjectReference: org.ID, SourceVersion: 2, Status: "ACTIVE"})
	})
	if err != nil {
		t.Fatalf("publish public integration event: %v", err)
	}
	processed, err := svc.FanoutOnce(ctx, 10)
	if err != nil || processed != 1 {
		t.Fatalf("fanout: processed=%d err=%v", processed, err)
	}
	items, err := svc.ListDeliveries(ctx, org.ID, sub.ID)
	if err != nil || len(items) != 1 || items[0].Status != integrations.DeliveryPending || items[0].JobID == "" {
		t.Fatalf("durable delivery not created: %#v err=%v", items, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE fleet_partner_webhook_deliveries SET status='DEAD_LETTERED' WHERE id=$1`, items[0].ID); err != nil {
		t.Fatal(err)
	}
	replayed, wasReplay, err := svc.ReplayDelivery(ctx, actor, items[0].ID, "idem-webhook-replay")
	if err != nil || wasReplay || replayed.Status != integrations.DeliveryPending || replayed.ReplayOfDeliveryID != items[0].ID {
		t.Fatalf("manual replay failed: %#v replay=%v err=%v", replayed, wasReplay, err)
	}
	replayedAgain, wasReplay, err := svc.ReplayDelivery(ctx, actor, items[0].ID, "idem-webhook-replay")
	if err != nil || !wasReplay || replayedAgain.ID != replayed.ID {
		t.Fatalf("manual replay idempotency failed: %#v replay=%v err=%v", replayedAgain, wasReplay, err)
	}
}

func TestSection55WebhookWorkerSigningRetryAfterSuccessAndDeadLetter(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "Section55 Worker", "RC-S55-WORKER")
	actor := integrations.Actor{Tenant: tenant.Context{OrganizationID: org.ID, OrganizationUserID: "ou_s55_worker", ActorType: tenant.ActorTypeOrganizationUser, ActorID: "ou_s55_worker", CorrelationID: "cor_s55_worker"}, IdentityID: "idn_s55_worker", SessionID: "ses_s55_worker", StepUpReference: "step_ok"}

	var calls atomic.Int32
	var capturedBody []byte
	var capturedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		capturedBody, _ = io.ReadAll(r.Body)
		capturedHeaders = r.Header.Clone()
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	validator := &integrations.DestinationValidator{Resolver: section55Resolver{}, Production: false, AllowHTTP: true, AllowTestLoopback: true}
	svc := newSection55ServiceWith(t, pool, validator, nil, 1)
	publicEvent := "organization.status_changed.v1"
	sub, secret, _, err := svc.CreateWebhook(ctx, actor, integrations.CreateWebhookInput{EndpointURL: server.URL + "/hook", EventTypes: []string{publicEvent}, IdempotencyKey: "idem-worker-webhook"})
	if err != nil {
		t.Fatal(err)
	}
	previousVersion := sub.SecretVersion
	rotatedSub, rotatedSecret, wasReplay, err := svc.RotateWebhookSecret(ctx, actor, sub.ID, sub.Version, "idem-worker-webhook-rotate", 5*time.Minute)
	if err != nil || wasReplay || rotatedSecret == "" || rotatedSecret == secret {
		t.Fatalf("rotate webhook secret: replay=%v err=%v", wasReplay, err)
	}
	sub = rotatedSub
	publisher := events.NewDomainPublisher(events.NewOutboxRepository(pool))
	publish := func(version int64, correlation string) {
		t.Helper()
		if publishErr := database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
			return publisher.Publish(ctx, tx, publicEvent, "organization", org.ID, version, org.ID, correlation,
				events.ContractPayload{SubjectReference: org.ID, SourceVersion: version, Status: "ACTIVE"})
		}); publishErr != nil {
			t.Fatal(publishErr)
		}
	}
	publish(2, "cor_s55_worker_retry")
	if processed, fanoutErr := svc.FanoutOnce(ctx, 10); fanoutErr != nil || processed != 1 {
		t.Fatalf("fanout retry delivery: processed=%d err=%v", processed, fanoutErr)
	}
	deliveries, err := svc.ListDeliveries(ctx, org.ID, sub.ID)
	if err != nil || len(deliveries) != 1 {
		t.Fatalf("load retry delivery: %#v %v", deliveries, err)
	}
	payload, _ := json.Marshal(map[string]string{"organizationId": org.ID, "deliveryId": deliveries[0].ID})
	job := jobs.Job{ID: deliveries[0].JobID, OrganizationID: org.ID, Payload: payload, Attempts: 1, MaxAttempts: 5, CorrelationID: "cor_s55_worker_retry", CausationID: deliveries[0].SourceEventID}
	err = svc.HandleDeliveryJob(ctx, job)
	class, code, _ := jobs.Classify(err)
	if class != jobs.ErrorClassTransient || code != "HTTP_429" || jobs.RetryDelay(err) != 2*time.Second {
		t.Fatalf("429 classification = %s/%s delay=%s err=%v", class, code, jobs.RetryDelay(err), err)
	}
	deliveries, _ = svc.ListDeliveries(ctx, org.ID, sub.ID)
	if deliveries[0].Status != integrations.DeliveryRetryScheduled || deliveries[0].NextAttemptAt == nil {
		t.Fatalf("retry state was not persisted: %#v", deliveries[0])
	}
	job.Attempts = 2
	if err = svc.HandleDeliveryJob(ctx, job); err != nil {
		t.Fatalf("successful retry failed: %v", err)
	}
	deliveries, _ = svc.ListDeliveries(ctx, org.ID, sub.ID)
	if deliveries[0].Status != integrations.DeliverySucceeded || deliveries[0].ResponseStatus != http.StatusNoContent {
		t.Fatalf("success state was not persisted: %#v", deliveries[0])
	}
	timestamp, _ := strconv.ParseInt(capturedHeaders.Get("X-KarryGo-Webhook-Timestamp"), 10, 64)
	if err = integrations.VerifyWebhookSignature(secret, previousVersion, sub.ID, deliveries[0].ID,
		deliveries[0].SourceEventID, timestamp, capturedBody,
		capturedHeaders.Get("X-KarryGo-Webhook-Signature"), time.Unix(timestamp, 0), 5*time.Minute); err != nil {
		t.Fatalf("previous overlap signature failed verification: %v", err)
	}
	if err = integrations.VerifyWebhookSignature(rotatedSecret, sub.SecretVersion, sub.ID, deliveries[0].ID,
		deliveries[0].SourceEventID, timestamp, capturedBody,
		capturedHeaders.Get("X-KarryGo-Webhook-Signature"), time.Unix(timestamp, 0), 5*time.Minute); err != nil {
		t.Fatalf("current rotated signature failed verification: %v", err)
	}
	if capturedHeaders.Get("X-KarryGo-Event-Id") != deliveries[0].SourceEventID || capturedHeaders.Get("Idempotency-Key") != deliveries[0].ID {
		t.Fatalf("stable delivery identities missing: %#v", capturedHeaders)
	}

	deadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadRequest) }))
	defer deadServer.Close()
	deadSub, _, _, err := svc.CreateWebhook(ctx, actor, integrations.CreateWebhookInput{EndpointURL: deadServer.URL + "/hook", EventTypes: []string{publicEvent}, IdempotencyKey: "idem-worker-dead"})
	if err != nil {
		t.Fatal(err)
	}
	publish(3, "cor_s55_worker_dead")
	if processed, fanoutErr := svc.FanoutOnce(ctx, 10); fanoutErr != nil || processed != 1 {
		t.Fatalf("fanout dead delivery: processed=%d err=%v", processed, fanoutErr)
	}
	deadDeliveries, _ := svc.ListDeliveries(ctx, org.ID, deadSub.ID)
	if len(deadDeliveries) != 1 {
		t.Fatalf("dead delivery count = %d", len(deadDeliveries))
	}
	deadPayload, _ := json.Marshal(map[string]string{"organizationId": org.ID, "deliveryId": deadDeliveries[0].ID})
	deadJob := jobs.Job{ID: deadDeliveries[0].JobID, OrganizationID: org.ID, Payload: deadPayload, Attempts: 1, MaxAttempts: 5, CorrelationID: "cor_s55_worker_dead", CausationID: deadDeliveries[0].SourceEventID}
	if err = svc.HandleDeliveryJob(ctx, deadJob); err == nil {
		t.Fatal("permanent 400 response reported success")
	}
	deadDeliveries, _ = svc.ListDeliveries(ctx, org.ID, deadSub.ID)
	if deadDeliveries[0].Status != integrations.DeliveryDeadLettered {
		t.Fatalf("permanent response not dead-lettered: %#v", deadDeliveries[0])
	}
	webhooks, _ := svc.ListWebhooks(ctx, org.ID)
	foundDisabled := false
	for _, webhook := range webhooks {
		if webhook.ID == deadSub.ID && webhook.Status == integrations.WebhookStatusDisabled {
			foundDisabled = true
		}
	}
	if !foundDisabled {
		t.Fatal("threshold failure did not disable subscription")
	}
}
