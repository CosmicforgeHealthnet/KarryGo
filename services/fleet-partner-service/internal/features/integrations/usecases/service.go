// Package usecases implements the Section 55 integration workflows:
// organization API clients and their authentication, webhook subscriptions,
// transactional fanout of public events and durable webhook delivery.
package usecases

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/integrations/clients"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/integrations/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/stepup"
)

const (
	actionAPIClientManage = "integrations.manage"
	actionWebhookManage   = "webhooks.manage"
)

type SecurityPolicyReader interface {
	MFARequiredForSensitiveActions(context.Context, string) (bool, error)
}

// Publisher publishes domain events inside the caller's transaction.
type Publisher interface {
	Publish(ctx context.Context, tx pgx.Tx, eventType string, aggregateType string, aggregateID string, aggregateVersion int64, organizationID string, correlationID string, payload any) error
}

// Enqueuer schedules durable jobs inside the caller's transaction.
type Enqueuer interface {
	Enqueue(ctx context.Context, q database.Querier, request jobs.EnqueueRequest) (string, error)
}

// HTTPDoer sends webhook deliveries.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// Options configures the service.
type Options struct {
	Store                Store
	Publisher            Publisher
	Config               config.Config
	Metrics              *observability.Metrics
	StepUp               stepup.Verifier
	SecurityPolicy       SecurityPolicyReader
	DestinationValidator *clients.DestinationValidator
	HTTPClient           *http.Client
}

// Service implements the integration workflows.

type Service struct {
	store             Store
	cfg               config.Config
	metrics           *observability.Metrics
	recorder          *audit.Recorder
	publisher         Publisher
	queue             Enqueuer
	stepUp            stepup.Verifier
	securityPolicy    SecurityPolicyReader
	destination       *clients.DestinationValidator
	httpClient        HTTPDoer
	initializationErr error
	workerRegistered  atomic.Bool
	fanoutStarted     atomic.Bool
}

func NewService(opts Options) *Service {
	destination := opts.DestinationValidator
	var initErr error
	if destination == nil {
		destination, initErr = clients.NewDestinationValidator(opts.Config.AppEnv == "production",
			opts.Config.WebhookAllowHTTP, opts.Config.WebhookPrivateDestinationAllowlist)
	}
	s := &Service{store: opts.Store, cfg: opts.Config, metrics: opts.Metrics,
		recorder: audit.NewRecorder(), publisher: opts.Publisher,
		queue: jobs.NewQueue(nil), stepUp: opts.StepUp, securityPolicy: opts.SecurityPolicy,
		destination: destination, initializationErr: initErr}
	if opts.HTTPClient != nil {
		s.httpClient = opts.HTTPClient
	} else if destination != nil {
		s.httpClient = clients.SecureHTTPClient(destination, opts.Config.WebhookDeliveryTimeout)
	}
	return s
}

func (s *Service) Ready() error {
	if s == nil || s.store == nil || !s.store.Configured() {
		return errors.New("integrations: database is not configured")
	}
	if s.initializationErr != nil {
		return s.initializationErr
	}
	if s.cfg.OrganizationAPIClientsEnabled && len(s.cfg.APIClientHashKey) < 32 {
		return errors.New("integrations: API client hash key is not configured")
	}
	if s.cfg.WebhooksEnabled && len(s.cfg.WebhookSecretEncryptionKey) != 32 {
		return errors.New("integrations: webhook encryption key is not configured")
	}
	if s.cfg.WebhookDeliveryWorkerEnabled && s.httpClient == nil {
		return errors.New("integrations: webhook delivery transport is not configured")
	}
	if s.cfg.WebhookDeliveryWorkerEnabled && s.cfg.WorkerEnabled && !s.cfg.IntegrationsKillSwitch && (!s.workerRegistered.Load() || !s.fanoutStarted.Load()) {
		return errors.New("integrations: webhook worker or fanout coordinator is not wired")
	}
	return nil
}

func (s *Service) verifyStepUp(ctx context.Context, actor models.Actor, action string) error {
	required := true
	if s.securityPolicy != nil {
		if value, err := s.securityPolicy.MFARequiredForSensitiveActions(ctx, actor.Tenant.OrganizationID); err == nil {
			required = value
		}
	}
	if !required {
		return nil
	}
	return s.requireStepUp(ctx, actor, action)
}

// verifySecretStepUp guards actions that issue, rotate or revoke an API or
// webhook secret. Spec 56.2 and 64.1 make step-up mandatory for secret
// changes, so the organization's mfaRequiredForSensitiveActions setting does
// not waive it (it still governs webhook updates and delivery replays).
func (s *Service) verifySecretStepUp(ctx context.Context, actor models.Actor, action string) error {
	return s.requireStepUp(ctx, actor, action)
}

func (s *Service) requireStepUp(ctx context.Context, actor models.Actor, action string) error {
	if s.stepUp == nil {
		return apperrors.Unavailable("Step-up verification is not configured.", nil)
	}
	if strings.TrimSpace(actor.StepUpReference) == "" {
		return apperrors.Forbidden("A valid step-up grant is required.", nil)
	}
	return s.stepUp.Verify(ctx, actor.StepUpReference, actor.IdentityID, actor.SessionID,
		actor.Tenant.OrganizationID, action)
}

func (s *Service) CreateAPIClient(ctx context.Context, actor models.Actor, in models.CreateAPIClientInput) (models.APIClient, string, bool, error) {
	if !s.cfg.OrganizationAPIClientsEnabled || s.cfg.IntegrationsKillSwitch {
		return models.APIClient{}, "", false, apperrors.Unavailable("Organization API clients are disabled.", nil)
	}
	if err := s.verifySecretStepUp(ctx, actor, actionAPIClientManage); err != nil {
		return models.APIClient{}, "", false, err
	}
	name, scopes, cidrs, err := models.ValidateAPIClientInput(in)
	if err != nil {
		return models.APIClient{}, "", false, err
	}
	secret, err := models.RandomToken(32)
	if err != nil {
		return models.APIClient{}, "", false, err
	}
	clientID, err := models.GenerateClientID()
	if err != nil {
		return models.APIClient{}, "", false, err
	}
	client := models.APIClient{ID: ids.New("apc"), OrganizationID: actor.Tenant.OrganizationID,
		Name: name, ClientID: clientID, Scopes: scopes, IPAllowlist: cidrs,
		Status: models.APIClientStatusActive, ExpiresAt: in.ExpiresAt,
		CreatedBy: actor.Tenant.OrganizationUserID, SecretVersion: 1, Version: 1}
	fingerprint := models.Fingerprint(struct {
		Name          string
		Scopes, CIDRs []string
		ExpiresAt     *time.Time
	}{name, scopes, cidrs, in.ExpiresAt})
	replay := false
	err = s.store.WithinTx(ctx, func(tx pgx.Tx) error {
		existing, wasReplay, claimErr := s.claimCommand(ctx, tx, client.OrganizationID, "api_client.create",
			in.IdempotencyKey, fingerprint, client.ID)
		if claimErr != nil {
			return claimErr
		}
		if wasReplay {
			replay = true
			loaded, loadErr := s.store.GetAPIClient(ctx, tx, client.OrganizationID, existing)
			client = loaded
			return loadErr
		}
		if insertErr := s.store.InsertAPIClient(ctx, tx, client, models.HashSecret(s.cfg.APIClientHashKey, secret)); insertErr != nil {
			return fmt.Errorf("integrations: create API client: %w", insertErr)
		}
		if err := s.recordAndPublish(ctx, tx, actor, "api_client.created", "api_client.created.v1",
			"organization_api_client", client.ID, 1, map[string]any{"status": client.Status, "scopes": client.Scopes}); err != nil {
			return err
		}
		loaded, loadErr := s.store.GetAPIClient(ctx, tx, client.OrganizationID, client.ID)
		client = loaded
		return loadErr
	})
	if err != nil {
		return models.APIClient{}, "", false, err
	}
	if replay {
		secret = ""
	}
	return client, secret, replay, nil
}

// ListAPIClients lists the organization's API clients.
func (s *Service) ListAPIClients(ctx context.Context, organizationID string) ([]models.APIClient, error) {
	return s.store.ListAPIClients(ctx, organizationID)
}

func (s *Service) RotateAPIClient(ctx context.Context, actor models.Actor, id string, expected int64,
	idempotencyKey string, overlap time.Duration) (models.APIClient, string, bool, error) {
	if !s.cfg.OrganizationAPIClientsEnabled || s.cfg.IntegrationsKillSwitch {
		return models.APIClient{}, "", false, apperrors.Unavailable("Organization API clients are disabled.", nil)
	}
	if err := s.verifySecretStepUp(ctx, actor, actionAPIClientManage); err != nil {
		return models.APIClient{}, "", false, err
	}
	if expected < 1 || overlap < 0 || overlap > s.cfg.WebhookRotationOverlapMax {
		return models.APIClient{}, "", false, apperrors.BadRequest("Expected version and a bounded overlap are required.", nil)
	}
	secret, err := models.RandomToken(32)
	if err != nil {
		return models.APIClient{}, "", false, err
	}
	fingerprint := models.Fingerprint(struct {
		ID      string
		Version int64
		Overlap int64
	}{id, expected, int64(overlap.Seconds())})
	var client models.APIClient
	replay := false
	err = s.store.WithinTx(ctx, func(tx pgx.Tx) error {
		existing, wasReplay, claimErr := s.claimCommand(ctx, tx, actor.Tenant.OrganizationID, "api_client.rotate:"+id,
			idempotencyKey, fingerprint, id)
		if claimErr != nil {
			return claimErr
		}
		if wasReplay {
			replay = true
			var loadErr error
			client, loadErr = s.store.GetAPIClient(ctx, tx, actor.Tenant.OrganizationID, existing)
			return loadErr
		}
		previous, previousVersion, err := s.store.LockActiveAPIClientSecret(ctx, tx, actor.Tenant.OrganizationID, id, expected)
		if err != nil {
			return apperrors.Conflict("API client was not found, revoked, or changed.", err)
		}
		var prevHash any
		var prevVersion any
		var prevUntil any
		if overlap > 0 {
			prevHash = previous
			prevVersion = previousVersion
			prevUntil = time.Now().UTC().Add(overlap)
		}
		changed, updateErr := s.store.RotateAPIClientSecret(ctx, tx, actor.Tenant.OrganizationID, id, expected, models.HashSecret(s.cfg.APIClientHashKey, secret), prevHash, prevVersion, prevUntil)
		if updateErr != nil || !changed {
			return apperrors.Conflict("API client was changed.", updateErr)
		}
		client, updateErr = s.store.GetAPIClient(ctx, tx, actor.Tenant.OrganizationID, id)
		if updateErr != nil {
			return updateErr
		}
		return s.recordAndPublish(ctx, tx, actor, "api_client.rotated", "api_client.rotated.v1", "organization_api_client", id,
			client.Version, map[string]any{"status": client.Status, "scopes": client.Scopes})
	})
	if err != nil {
		return models.APIClient{}, "", false, err
	}
	if replay {
		secret = ""
	}
	return client, secret, replay, nil
}

func (s *Service) RevokeAPIClient(ctx context.Context, actor models.Actor, id string, expected int64, key string) (models.APIClient, bool, error) {
	if !s.cfg.OrganizationAPIClientsEnabled || s.cfg.IntegrationsKillSwitch {
		return models.APIClient{}, false, apperrors.Unavailable("Organization API clients are disabled.", nil)
	}
	if err := s.verifySecretStepUp(ctx, actor, actionAPIClientManage); err != nil {
		return models.APIClient{}, false, err
	}
	if expected < 1 {
		return models.APIClient{}, false, apperrors.BadRequest("Expected version is required.", nil)
	}
	fingerprint := models.Fingerprint(struct {
		ID      string
		Version int64
	}{id, expected})
	replay := false
	var client models.APIClient
	revokedHash := models.HashSecret(s.cfg.APIClientHashKey, ids.New("revoked"))
	err := s.store.WithinTx(ctx, func(tx pgx.Tx) error {
		existing, wasReplay, claimErr := s.claimCommand(ctx, tx, actor.Tenant.OrganizationID, "api_client.revoke:"+id, key, fingerprint, id)
		if claimErr != nil {
			return claimErr
		}
		if wasReplay {
			replay = true
			var loadErr error
			client, loadErr = s.store.GetAPIClient(ctx, tx, actor.Tenant.OrganizationID, existing)
			return loadErr
		}
		changed, updateErr := s.store.RevokeAPIClient(ctx, tx, actor.Tenant.OrganizationID, id, expected, revokedHash)
		if updateErr != nil || !changed {
			return apperrors.Conflict("API client was not found, revoked, or changed.", updateErr)
		}
		client, updateErr = s.store.GetAPIClient(ctx, tx, actor.Tenant.OrganizationID, id)
		if updateErr != nil {
			return updateErr
		}
		return s.recordAndPublish(ctx, tx, actor, "api_client.revoked", "api_client.revoked.v1", "organization_api_client", id, client.Version, map[string]any{"status": client.Status, "scopes": client.Scopes})
	})
	return client, replay, err
}

func (s *Service) CreateWebhook(ctx context.Context, actor models.Actor, in models.CreateWebhookInput) (models.WebhookSubscription, string, bool, error) {
	if !s.cfg.WebhooksEnabled || s.cfg.IntegrationsKillSwitch {
		return models.WebhookSubscription{}, "", false, apperrors.Unavailable("Webhooks are disabled.", nil)
	}
	if err := s.verifySecretStepUp(ctx, actor, actionWebhookManage); err != nil {
		return models.WebhookSubscription{}, "", false, err
	}
	endpoint, eventsList, err := s.validateWebhookConfig(ctx, in.EndpointURL, in.EventTypes)
	if err != nil {
		return models.WebhookSubscription{}, "", false, err
	}
	secret, err := models.RandomToken(32)
	if err != nil {
		return models.WebhookSubscription{}, "", false, err
	}
	ciphertext, err := models.EncryptSecret(s.cfg.WebhookSecretEncryptionKey, secret)
	if err != nil {
		return models.WebhookSubscription{}, "", false, err
	}
	sub := models.WebhookSubscription{ID: ids.New("whs"), OrganizationID: actor.Tenant.OrganizationID, EndpointURL: endpoint, EventTypes: eventsList, Status: models.WebhookStatusActive, CreatedBy: actor.Tenant.OrganizationUserID, SecretVersion: 1, Version: 1}
	fingerprint := models.Fingerprint(struct {
		Endpoint string
		Events   []string
	}{endpoint, eventsList})
	replay := false
	err = s.store.WithinTx(ctx, func(tx pgx.Tx) error {
		existing, wasReplay, claimErr := s.claimCommand(ctx, tx, sub.OrganizationID, "webhook.create", in.IdempotencyKey, fingerprint, sub.ID)
		if claimErr != nil {
			return claimErr
		}
		if wasReplay {
			replay = true
			var loadErr error
			sub, loadErr = s.store.GetWebhook(ctx, tx, sub.OrganizationID, existing)
			return loadErr
		}
		insertErr := s.store.InsertWebhook(ctx, tx, sub, ciphertext)
		if insertErr != nil {
			return insertErr
		}
		if eventErr := s.recordAndPublish(ctx, tx, actor, "webhook.subscription_created", "webhook.subscription_created.v1", "webhook_subscription", sub.ID, 1, map[string]any{"status": sub.Status, "eventTypes": sub.EventTypes}); eventErr != nil {
			return eventErr
		}
		sub, insertErr = s.store.GetWebhook(ctx, tx, sub.OrganizationID, sub.ID)
		return insertErr
	})
	if err != nil {
		return models.WebhookSubscription{}, "", false, err
	}
	if replay {
		secret = ""
	}
	return sub, secret, replay, nil
}

func (s *Service) UpdateWebhook(ctx context.Context, actor models.Actor, id string, in models.UpdateWebhookInput) (models.WebhookSubscription, bool, error) {
	if !s.cfg.WebhooksEnabled || s.cfg.IntegrationsKillSwitch {
		return models.WebhookSubscription{}, false, apperrors.Unavailable("Webhooks are disabled.", nil)
	}
	if err := s.verifyStepUp(ctx, actor, actionWebhookManage); err != nil {
		return models.WebhookSubscription{}, false, err
	}
	if in.ExpectedVersion < 1 {
		return models.WebhookSubscription{}, false, apperrors.BadRequest("Expected version is required.", nil)
	}
	endpoint, eventsList, err := s.validateWebhookConfig(ctx, in.EndpointURL, in.EventTypes)
	if err != nil {
		return models.WebhookSubscription{}, false, err
	}
	status := strings.ToUpper(strings.TrimSpace(in.Status))
	if status == "" {
		status = models.WebhookStatusActive
	}
	if status != models.WebhookStatusActive && status != models.WebhookStatusDisabled {
		return models.WebhookSubscription{}, false, apperrors.BadRequest("Webhook status is invalid.", nil)
	}
	fingerprint := models.Fingerprint(struct {
		ID, Endpoint, Status string
		Events               []string
		Version              int64
	}{id, endpoint, status, eventsList, in.ExpectedVersion})
	replay := false
	var sub models.WebhookSubscription
	err = s.store.WithinTx(ctx, func(tx pgx.Tx) error {
		existing, wasReplay, claimErr := s.claimCommand(ctx, tx, actor.Tenant.OrganizationID, "webhook.update:"+id, in.IdempotencyKey, fingerprint, id)
		if claimErr != nil {
			return claimErr
		}
		if wasReplay {
			replay = true
			var loadErr error
			sub, loadErr = s.store.GetWebhook(ctx, tx, actor.Tenant.OrganizationID, existing)
			return loadErr
		}
		changed, updateErr := s.store.UpdateWebhook(ctx, tx, actor.Tenant.OrganizationID, id, endpoint, eventsList, status, in.ExpectedVersion)
		if updateErr != nil || !changed {
			return apperrors.Conflict("Webhook was not found or changed.", updateErr)
		}
		sub, updateErr = s.store.GetWebhook(ctx, tx, actor.Tenant.OrganizationID, id)
		if updateErr != nil {
			return updateErr
		}
		eventType, action := "webhook.subscription_updated.v1", "webhook.subscription_updated"
		if status == models.WebhookStatusDisabled {
			eventType, action = "webhook.disabled.v1", "webhook.disabled"
		}
		return s.recordAndPublish(ctx, tx, actor, action, eventType, "webhook_subscription", id, sub.Version, map[string]any{"status": sub.Status, "eventTypes": sub.EventTypes})
	})
	return sub, replay, err
}

func (s *Service) RotateWebhookSecret(ctx context.Context, actor models.Actor, id string, expected int64, key string, overlap time.Duration) (models.WebhookSubscription, string, bool, error) {
	if !s.cfg.WebhooksEnabled || s.cfg.IntegrationsKillSwitch {
		return models.WebhookSubscription{}, "", false, apperrors.Unavailable("Webhooks are disabled.", nil)
	}
	if err := s.verifySecretStepUp(ctx, actor, actionWebhookManage); err != nil {
		return models.WebhookSubscription{}, "", false, err
	}
	if expected < 1 || overlap < 0 || overlap > s.cfg.WebhookRotationOverlapMax {
		return models.WebhookSubscription{}, "", false, apperrors.BadRequest("Expected version and a bounded overlap are required.", nil)
	}
	secret, err := models.RandomToken(32)
	if err != nil {
		return models.WebhookSubscription{}, "", false, err
	}
	ciphertext, err := models.EncryptSecret(s.cfg.WebhookSecretEncryptionKey, secret)
	if err != nil {
		return models.WebhookSubscription{}, "", false, err
	}
	fingerprint := models.Fingerprint(struct {
		ID               string
		Version, Overlap int64
	}{id, expected, int64(overlap.Seconds())})
	replay := false
	var sub models.WebhookSubscription
	err = s.store.WithinTx(ctx, func(tx pgx.Tx) error {
		existing, wasReplay, claimErr := s.claimCommand(ctx, tx, actor.Tenant.OrganizationID, "webhook.rotate:"+id, key, fingerprint, id)
		if claimErr != nil {
			return claimErr
		}
		if wasReplay {
			replay = true
			var loadErr error
			sub, loadErr = s.store.GetWebhook(ctx, tx, actor.Tenant.OrganizationID, existing)
			return loadErr
		}
		previous, previousVersion, scanErr := s.store.LockActiveWebhookSecret(ctx, tx, actor.Tenant.OrganizationID, id, expected)
		if scanErr != nil {
			return apperrors.Conflict("Webhook was not found, disabled, or changed.", scanErr)
		}
		var prevCipher, prevVersion, prevUntil any
		if overlap > 0 {
			prevCipher = previous
			prevVersion = previousVersion
			prevUntil = time.Now().UTC().Add(overlap)
		}
		changed, updateErr := s.store.RotateWebhookSecret(ctx, tx, actor.Tenant.OrganizationID, id, expected, ciphertext, prevCipher, prevVersion, prevUntil)
		if updateErr != nil || !changed {
			return apperrors.Conflict("Webhook was changed.", updateErr)
		}
		sub, updateErr = s.store.GetWebhook(ctx, tx, actor.Tenant.OrganizationID, id)
		if updateErr != nil {
			return updateErr
		}
		return s.recordAndPublish(ctx, tx, actor, "webhook.secret_rotated", "webhook.secret_rotated.v1", "webhook_subscription", id, sub.Version, map[string]any{"status": sub.Status, "eventTypes": sub.EventTypes})
	})
	if err != nil {
		return models.WebhookSubscription{}, "", false, err
	}
	if replay {
		secret = ""
	}
	return sub, secret, replay, nil
}

func (s *Service) validateWebhookConfig(ctx context.Context, raw string, eventTypes []string) (string, []string, error) {
	if s.destination == nil {
		return "", nil, apperrors.Unavailable("Webhook destination validation is unavailable.", nil)
	}
	u, _, err := s.destination.Validate(ctx, raw)
	if err != nil {
		return "", nil, apperrors.BadRequest("Webhook destination is not permitted.", err)
	}
	allowed := map[string]bool{}
	for _, eventType := range events.PublicIntegrationTypes() {
		allowed[eventType] = true
	}
	list, err := models.ExactAllowlist(eventTypes, allowed, "event type")
	if err != nil {
		return "", nil, err
	}
	return u.String(), list, nil
}

func (s *Service) ListWebhooks(ctx context.Context, org string) ([]models.WebhookSubscription, error) {
	return s.store.ListWebhooks(ctx, org)
}

func (s *Service) ListDeliveries(ctx context.Context, org, subscriptionID string) ([]models.WebhookDelivery, error) {
	return s.store.ListDeliveries(ctx, org, subscriptionID)
}

func (s *Service) ReplayDelivery(ctx context.Context, actor models.Actor, deliveryID, key string) (models.WebhookDelivery, bool, error) {
	if !s.cfg.WebhooksEnabled || s.cfg.IntegrationsKillSwitch {
		return models.WebhookDelivery{}, false, apperrors.Unavailable("Webhooks are disabled.", nil)
	}
	if err := s.verifyStepUp(ctx, actor, actionWebhookManage); err != nil {
		return models.WebhookDelivery{}, false, err
	}
	fingerprint := models.Fingerprint(struct{ ID string }{deliveryID})
	replay := false
	var created models.WebhookDelivery
	err := s.store.WithinTx(ctx, func(tx pgx.Tx) error {
		newID := ids.New("whd")
		existing, wasReplay, claimErr := s.claimCommand(ctx, tx, actor.Tenant.OrganizationID, "webhook.delivery.replay:"+deliveryID, key, fingerprint, newID)
		if claimErr != nil {
			return claimErr
		}
		if wasReplay {
			replay = true
			var loadErr error
			created, loadErr = s.store.GetDelivery(ctx, tx, actor.Tenant.OrganizationID, existing)
			return loadErr
		}
		original, scanErr := s.store.LockFailedDelivery(ctx, tx, actor.Tenant.OrganizationID, deliveryID)
		if scanErr != nil {
			return apperrors.NotFound("Failed webhook delivery was not found.", scanErr)
		}
		eventID, correlation := original.SourceEventID, original.CorrelationID
		attempt, err := s.store.NextAttempt(ctx, tx, original.SubscriptionID, eventID)
		if err != nil {
			return err
		}
		insertErr := s.store.InsertReplayDelivery(ctx, tx, newID, actor.Tenant.OrganizationID, original, attempt, deliveryID)
		if insertErr != nil {
			return insertErr
		}
		jobID, enqueueErr := s.queue.Enqueue(ctx, tx, jobs.EnqueueRequest{OrganizationID: actor.Tenant.OrganizationID, JobType: models.JobTypeWebhookDelivery, DedupeKey: "webhook:" + newID, IdempotencyKey: key, Payload: map[string]any{"organizationId": actor.Tenant.OrganizationID, "deliveryId": newID}, CorrelationID: correlation, CausationID: eventID, MaxAttempts: s.cfg.WebhookDeliveryMaxAttempts})
		if enqueueErr != nil {
			return enqueueErr
		}
		if updateErr := s.store.SetDeliveryJob(ctx, tx, newID, jobID); updateErr != nil {
			return updateErr
		}
		created, insertErr = s.store.GetDelivery(ctx, tx, actor.Tenant.OrganizationID, newID)
		if insertErr != nil {
			return insertErr
		}
		return s.recordAndPublish(ctx, tx, actor, "webhook.delivery_replayed", "webhook.delivery_replayed.v1", "webhook_delivery", newID, 1, map[string]any{"status": created.Status, "eventId": eventID, "attempt": attempt})
	})
	return created, replay, err
}

func (s *Service) claimCommand(ctx context.Context, tx pgx.Tx, org, command, key, fingerprint, resourceID string) (string, bool, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", false, apperrors.BadRequest("Idempotency-Key header is required.", nil)
	}
	resource, replay, mismatch, err := s.store.ClaimCommand(ctx, tx, org, command, key, fingerprint, resourceID)
	if err != nil {
		return "", false, err
	}
	if mismatch {
		return "", false, apperrors.Conflict("This idempotency key was already used with different input.", nil)
	}
	return resource, replay, nil
}

func (s *Service) recordAndPublish(ctx context.Context, tx pgx.Tx, actor models.Actor, action, eventType, aggregateType, id string, version int64, values map[string]any) error {
	entry := audit.EntryFromTenant(actor.Tenant, action, aggregateType, id)
	if actor.Tenant.OrganizationUserID != "" {
		entry.ActorID = actor.Tenant.OrganizationUserID
	}
	entry.NewValues = values
	entry.StepUpReference = actor.StepUpReference
	if err := s.recorder.Record(ctx, tx, entry); err != nil {
		return err
	}
	payload := events.IntegrationContractPayload{SubjectReference: id, SourceVersion: version, Status: stringValue(values["status"])}
	if scopes, ok := values["scopes"].([]string); ok {
		payload.Scopes = scopes
	}
	if eventTypes, ok := values["eventTypes"].([]string); ok {
		payload.EventTypes = eventTypes
	}
	if eventID, ok := values["eventId"].(string); ok {
		payload.EventID = eventID
	}
	if attempt, ok := values["attempt"].(int); ok {
		payload.Attempt = attempt
	}
	return s.publisher.Publish(ctx, tx, eventType, aggregateType, id, version, actor.Tenant.OrganizationID, actor.Tenant.CorrelationID, payload)
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}
