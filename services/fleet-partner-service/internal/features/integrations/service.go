package integrations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/stepup"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

const (
	PermissionIntegrationsRead   = "integrations.read"
	PermissionIntegrationsManage = "integrations.manage"
	PermissionWebhooksManage     = "webhooks.manage"

	ScopeIntegrationContextRead = "integration.context.read"

	actionAPIClientManage = "integrations.manage"
	actionWebhookManage   = "webhooks.manage"
)

var allowedClientScopes = map[string]bool{ScopeIntegrationContextRead: true}

type SecurityPolicyReader interface {
	MFARequiredForSensitiveActions(context.Context, string) (bool, error)
}

type Options struct {
	Pool                 *pgxpool.Pool
	Config               config.Config
	Metrics              *observability.Metrics
	StepUp               stepup.Verifier
	SecurityPolicy       SecurityPolicyReader
	DestinationValidator *DestinationValidator
	HTTPClient           *http.Client
}

type Service struct {
	pool              *pgxpool.Pool
	cfg               config.Config
	metrics           *observability.Metrics
	recorder          *audit.Recorder
	publisher         *events.DomainPublisher
	queue             *jobs.Queue
	stepUp            stepup.Verifier
	securityPolicy    SecurityPolicyReader
	destination       *DestinationValidator
	httpClient        *http.Client
	initializationErr error
	workerRegistered  atomic.Bool
	fanoutStarted     atomic.Bool
}

func NewService(opts Options) *Service {
	destination := opts.DestinationValidator
	var initErr error
	if destination == nil {
		destination, initErr = NewDestinationValidator(opts.Config.AppEnv == "production",
			opts.Config.WebhookAllowHTTP, opts.Config.WebhookPrivateDestinationAllowlist)
	}
	s := &Service{pool: opts.Pool, cfg: opts.Config, metrics: opts.Metrics,
		recorder: audit.NewRecorder(), publisher: events.NewDomainPublisher(events.NewOutboxRepository(opts.Pool)),
		queue: jobs.NewQueue(opts.Pool), stepUp: opts.StepUp, securityPolicy: opts.SecurityPolicy,
		destination: destination, httpClient: opts.HTTPClient, initializationErr: initErr}
	if s.httpClient == nil && destination != nil {
		s.httpClient = secureHTTPClient(destination, opts.Config.WebhookDeliveryTimeout)
	}
	return s
}

func (s *Service) Ready() error {
	if s == nil || s.pool == nil {
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

type Actor struct {
	Tenant          tenant.Context
	IdentityID      string
	SessionID       string
	StepUpReference string
}

func (s *Service) verifyStepUp(ctx context.Context, actor Actor, action string) error {
	required := true
	if s.securityPolicy != nil {
		if value, err := s.securityPolicy.MFARequiredForSensitiveActions(ctx, actor.Tenant.OrganizationID); err == nil {
			required = value
		}
	}
	if !required {
		return nil
	}
	if s.stepUp == nil {
		return apperrors.Unavailable("Step-up verification is not configured.", nil)
	}
	if strings.TrimSpace(actor.StepUpReference) == "" {
		return apperrors.Forbidden("A valid step-up grant is required.", nil)
	}
	return s.stepUp.Verify(ctx, actor.StepUpReference, actor.IdentityID, actor.SessionID,
		actor.Tenant.OrganizationID, action)
}

type CreateAPIClientInput struct {
	Name, IdempotencyKey string
	Scopes               []string
	IPAllowlist          []string
	ExpiresAt            *time.Time
}

type RotateAPIClientInput struct {
	IdempotencyKey, StepUpReference string
	ExpectedVersion                 int64
	Overlap                         time.Duration
}

func (s *Service) CreateAPIClient(ctx context.Context, actor Actor, in CreateAPIClientInput) (APIClient, string, bool, error) {
	if !s.cfg.OrganizationAPIClientsEnabled || s.cfg.IntegrationsKillSwitch {
		return APIClient{}, "", false, apperrors.Unavailable("Organization API clients are disabled.", nil)
	}
	if err := s.verifyStepUp(ctx, actor, actionAPIClientManage); err != nil {
		return APIClient{}, "", false, err
	}
	name, scopes, cidrs, err := validateAPIClientInput(in)
	if err != nil {
		return APIClient{}, "", false, err
	}
	secret, err := randomToken(32)
	if err != nil {
		return APIClient{}, "", false, err
	}
	clientID, err := generateClientID()
	if err != nil {
		return APIClient{}, "", false, err
	}
	client := APIClient{ID: ids.New("apc"), OrganizationID: actor.Tenant.OrganizationID,
		Name: name, ClientID: clientID, Scopes: scopes, IPAllowlist: cidrs,
		Status: APIClientStatusActive, ExpiresAt: in.ExpiresAt,
		CreatedBy: actor.Tenant.OrganizationUserID, SecretVersion: 1, Version: 1}
	fingerprint := fingerprintOf(struct {
		Name          string
		Scopes, CIDRs []string
		ExpiresAt     *time.Time
	}{name, scopes, cidrs, in.ExpiresAt})
	replay := false
	err = database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		existing, wasReplay, claimErr := claimCommand(ctx, tx, client.OrganizationID, "api_client.create",
			in.IdempotencyKey, fingerprint, client.ID)
		if claimErr != nil {
			return claimErr
		}
		if wasReplay {
			replay = true
			loaded, loadErr := scanAPIClient(tx.QueryRow(ctx, apiClientSelect+` WHERE organization_id=$1 AND id=$2`, client.OrganizationID, existing))
			client = loaded
			return loadErr
		}
		_, insertErr := tx.Exec(ctx, `INSERT INTO fleet_partner_organization_api_clients
			(id,organization_id,name,client_id,secret_hash,secret_version,scopes,ip_allowlist,status,expires_at,created_by)
			VALUES($1,$2,$3,$4,$5,1,$6,$7,'ACTIVE',$8,$9)`, client.ID, client.OrganizationID, client.Name,
			client.ClientID, hashSecret(s.cfg.APIClientHashKey, secret), client.Scopes, client.IPAllowlist,
			client.ExpiresAt, client.CreatedBy)
		if insertErr != nil {
			return fmt.Errorf("integrations: create API client: %w", insertErr)
		}
		if err := s.recordAndPublish(ctx, tx, actor, "api_client.created", "api_client.created.v1",
			"organization_api_client", client.ID, 1, map[string]any{"status": client.Status, "scopes": client.Scopes}); err != nil {
			return err
		}
		loaded, loadErr := scanAPIClient(tx.QueryRow(ctx, apiClientSelect+` WHERE organization_id=$1 AND id=$2`, client.OrganizationID, client.ID))
		client = loaded
		return loadErr
	})
	if err != nil {
		return APIClient{}, "", false, err
	}
	if replay {
		secret = ""
	}
	return client, secret, replay, nil
}

func validateAPIClientInput(in CreateAPIClientInput) (string, []string, []string, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > 120 || strings.ContainsAny(name, "\r\n\x00") {
		return "", nil, nil, apperrors.BadRequest("API client name is invalid.", nil)
	}
	scopes, err := exactAllowlist(in.Scopes, allowedClientScopes, "scope")
	if err != nil {
		return "", nil, nil, err
	}
	var cidrs []string
	seen := map[string]bool{}
	for _, raw := range in.IPAllowlist {
		_, network, parseErr := net.ParseCIDR(strings.TrimSpace(raw))
		if parseErr != nil {
			return "", nil, nil, apperrors.BadRequest("API client IP allowlist contains an invalid CIDR.", nil)
		}
		canonical := network.String()
		if !seen[canonical] {
			seen[canonical] = true
			cidrs = append(cidrs, canonical)
		}
	}
	sort.Strings(cidrs)
	if in.ExpiresAt != nil && !in.ExpiresAt.After(time.Now().UTC()) {
		return "", nil, nil, apperrors.BadRequest("API client expiry must be in the future.", nil)
	}
	return name, scopes, cidrs, nil
}

func (s *Service) ListAPIClients(ctx context.Context, organizationID string) ([]APIClient, error) {
	rows, err := s.pool.Query(ctx, apiClientSelect+` WHERE organization_id=$1 ORDER BY created_at DESC`, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []APIClient
	for rows.Next() {
		item, scanErr := scanAPIClient(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) RotateAPIClient(ctx context.Context, actor Actor, id string, expected int64,
	idempotencyKey string, overlap time.Duration) (APIClient, string, bool, error) {
	if !s.cfg.OrganizationAPIClientsEnabled || s.cfg.IntegrationsKillSwitch {
		return APIClient{}, "", false, apperrors.Unavailable("Organization API clients are disabled.", nil)
	}
	if err := s.verifyStepUp(ctx, actor, actionAPIClientManage); err != nil {
		return APIClient{}, "", false, err
	}
	if expected < 1 || overlap < 0 || overlap > s.cfg.WebhookRotationOverlapMax {
		return APIClient{}, "", false, apperrors.BadRequest("Expected version and a bounded overlap are required.", nil)
	}
	secret, err := randomToken(32)
	if err != nil {
		return APIClient{}, "", false, err
	}
	fingerprint := fingerprintOf(struct {
		ID      string
		Version int64
		Overlap int64
	}{id, expected, int64(overlap.Seconds())})
	var client APIClient
	replay := false
	err = database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		existing, wasReplay, claimErr := claimCommand(ctx, tx, actor.Tenant.OrganizationID, "api_client.rotate:"+id,
			idempotencyKey, fingerprint, id)
		if claimErr != nil {
			return claimErr
		}
		if wasReplay {
			replay = true
			var loadErr error
			client, loadErr = scanAPIClient(tx.QueryRow(ctx, apiClientSelect+` WHERE organization_id=$1 AND id=$2`, actor.Tenant.OrganizationID, existing))
			return loadErr
		}
		var previous []byte
		var previousVersion int
		if err := tx.QueryRow(ctx, `SELECT secret_hash,secret_version FROM fleet_partner_organization_api_clients
			WHERE organization_id=$1 AND id=$2 AND status='ACTIVE' AND version=$3 FOR UPDATE`, actor.Tenant.OrganizationID, id, expected).Scan(&previous, &previousVersion); err != nil {
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
		tag, updateErr := tx.Exec(ctx, `UPDATE fleet_partner_organization_api_clients SET
			secret_hash=$1,secret_version=secret_version+1,previous_secret_hash=$2,previous_secret_version=$3,
			previous_secret_valid_until=$4,rotated_at=now(),updated_at=now(),version=version+1
			WHERE organization_id=$5 AND id=$6 AND status='ACTIVE' AND version=$7`, hashSecret(s.cfg.APIClientHashKey, secret),
			prevHash, prevVersion, prevUntil, actor.Tenant.OrganizationID, id, expected)
		if updateErr != nil || tag.RowsAffected() != 1 {
			return apperrors.Conflict("API client was changed.", updateErr)
		}
		client, updateErr = scanAPIClient(tx.QueryRow(ctx, apiClientSelect+` WHERE organization_id=$1 AND id=$2`, actor.Tenant.OrganizationID, id))
		if updateErr != nil {
			return updateErr
		}
		return s.recordAndPublish(ctx, tx, actor, "api_client.rotated", "api_client.rotated.v1", "organization_api_client", id,
			client.Version, map[string]any{"status": client.Status, "scopes": client.Scopes})
	})
	if err != nil {
		return APIClient{}, "", false, err
	}
	if replay {
		secret = ""
	}
	return client, secret, replay, nil
}

func (s *Service) RevokeAPIClient(ctx context.Context, actor Actor, id string, expected int64, key string) (APIClient, bool, error) {
	if !s.cfg.OrganizationAPIClientsEnabled || s.cfg.IntegrationsKillSwitch {
		return APIClient{}, false, apperrors.Unavailable("Organization API clients are disabled.", nil)
	}
	if err := s.verifyStepUp(ctx, actor, actionAPIClientManage); err != nil {
		return APIClient{}, false, err
	}
	if expected < 1 {
		return APIClient{}, false, apperrors.BadRequest("Expected version is required.", nil)
	}
	fingerprint := fingerprintOf(struct {
		ID      string
		Version int64
	}{id, expected})
	replay := false
	var client APIClient
	revokedHash := hashSecret(s.cfg.APIClientHashKey, ids.New("revoked"))
	err := database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		existing, wasReplay, claimErr := claimCommand(ctx, tx, actor.Tenant.OrganizationID, "api_client.revoke:"+id, key, fingerprint, id)
		if claimErr != nil {
			return claimErr
		}
		if wasReplay {
			replay = true
			var loadErr error
			client, loadErr = scanAPIClient(tx.QueryRow(ctx, apiClientSelect+` WHERE organization_id=$1 AND id=$2`, actor.Tenant.OrganizationID, existing))
			return loadErr
		}
		tag, updateErr := tx.Exec(ctx, `UPDATE fleet_partner_organization_api_clients SET status='REVOKED',revoked_at=now(),
			secret_hash=$1,previous_secret_hash=NULL,
			previous_secret_version=NULL,previous_secret_valid_until=NULL,updated_at=now(),version=version+1
			WHERE organization_id=$2 AND id=$3 AND status='ACTIVE' AND version=$4`, revokedHash, actor.Tenant.OrganizationID, id, expected)
		if updateErr != nil || tag.RowsAffected() != 1 {
			return apperrors.Conflict("API client was not found, revoked, or changed.", updateErr)
		}
		client, updateErr = scanAPIClient(tx.QueryRow(ctx, apiClientSelect+` WHERE organization_id=$1 AND id=$2`, actor.Tenant.OrganizationID, id))
		if updateErr != nil {
			return updateErr
		}
		return s.recordAndPublish(ctx, tx, actor, "api_client.revoked", "api_client.revoked.v1", "organization_api_client", id, client.Version, map[string]any{"status": client.Status, "scopes": client.Scopes})
	})
	return client, replay, err
}

type CreateWebhookInput struct {
	EndpointURL, IdempotencyKey string
	EventTypes                  []string
}

func (s *Service) CreateWebhook(ctx context.Context, actor Actor, in CreateWebhookInput) (WebhookSubscription, string, bool, error) {
	if !s.cfg.WebhooksEnabled || s.cfg.IntegrationsKillSwitch {
		return WebhookSubscription{}, "", false, apperrors.Unavailable("Webhooks are disabled.", nil)
	}
	if err := s.verifyStepUp(ctx, actor, actionWebhookManage); err != nil {
		return WebhookSubscription{}, "", false, err
	}
	endpoint, eventsList, err := s.validateWebhookConfig(ctx, in.EndpointURL, in.EventTypes)
	if err != nil {
		return WebhookSubscription{}, "", false, err
	}
	secret, err := randomToken(32)
	if err != nil {
		return WebhookSubscription{}, "", false, err
	}
	ciphertext, err := encryptSecret(s.cfg.WebhookSecretEncryptionKey, secret)
	if err != nil {
		return WebhookSubscription{}, "", false, err
	}
	sub := WebhookSubscription{ID: ids.New("whs"), OrganizationID: actor.Tenant.OrganizationID, EndpointURL: endpoint, EventTypes: eventsList, Status: WebhookStatusActive, CreatedBy: actor.Tenant.OrganizationUserID, SecretVersion: 1, Version: 1}
	fingerprint := fingerprintOf(struct {
		Endpoint string
		Events   []string
	}{endpoint, eventsList})
	replay := false
	err = database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		existing, wasReplay, claimErr := claimCommand(ctx, tx, sub.OrganizationID, "webhook.create", in.IdempotencyKey, fingerprint, sub.ID)
		if claimErr != nil {
			return claimErr
		}
		if wasReplay {
			replay = true
			var loadErr error
			sub, loadErr = scanWebhook(tx.QueryRow(ctx, webhookSelect+` WHERE organization_id=$1 AND id=$2`, sub.OrganizationID, existing))
			return loadErr
		}
		_, insertErr := tx.Exec(ctx, `INSERT INTO fleet_partner_webhook_subscriptions
			(id,organization_id,endpoint_url,event_types,secret_ciphertext,secret_version,status,created_by)
			VALUES($1,$2,$3,$4,$5,1,'ACTIVE',$6)`, sub.ID, sub.OrganizationID, sub.EndpointURL, sub.EventTypes, ciphertext, sub.CreatedBy)
		if insertErr != nil {
			return insertErr
		}
		if eventErr := s.recordAndPublish(ctx, tx, actor, "webhook.subscription_created", "webhook.subscription_created.v1", "webhook_subscription", sub.ID, 1, map[string]any{"status": sub.Status, "eventTypes": sub.EventTypes}); eventErr != nil {
			return eventErr
		}
		sub, insertErr = scanWebhook(tx.QueryRow(ctx, webhookSelect+` WHERE organization_id=$1 AND id=$2`, sub.OrganizationID, sub.ID))
		return insertErr
	})
	if err != nil {
		return WebhookSubscription{}, "", false, err
	}
	if replay {
		secret = ""
	}
	return sub, secret, replay, nil
}

type UpdateWebhookInput struct {
	EndpointURL, Status, IdempotencyKey string
	EventTypes                          []string
	ExpectedVersion                     int64
}

func (s *Service) UpdateWebhook(ctx context.Context, actor Actor, id string, in UpdateWebhookInput) (WebhookSubscription, bool, error) {
	if !s.cfg.WebhooksEnabled || s.cfg.IntegrationsKillSwitch {
		return WebhookSubscription{}, false, apperrors.Unavailable("Webhooks are disabled.", nil)
	}
	if err := s.verifyStepUp(ctx, actor, actionWebhookManage); err != nil {
		return WebhookSubscription{}, false, err
	}
	if in.ExpectedVersion < 1 {
		return WebhookSubscription{}, false, apperrors.BadRequest("Expected version is required.", nil)
	}
	endpoint, eventsList, err := s.validateWebhookConfig(ctx, in.EndpointURL, in.EventTypes)
	if err != nil {
		return WebhookSubscription{}, false, err
	}
	status := strings.ToUpper(strings.TrimSpace(in.Status))
	if status == "" {
		status = WebhookStatusActive
	}
	if status != WebhookStatusActive && status != WebhookStatusDisabled {
		return WebhookSubscription{}, false, apperrors.BadRequest("Webhook status is invalid.", nil)
	}
	fingerprint := fingerprintOf(struct {
		ID, Endpoint, Status string
		Events               []string
		Version              int64
	}{id, endpoint, status, eventsList, in.ExpectedVersion})
	replay := false
	var sub WebhookSubscription
	err = database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		existing, wasReplay, claimErr := claimCommand(ctx, tx, actor.Tenant.OrganizationID, "webhook.update:"+id, in.IdempotencyKey, fingerprint, id)
		if claimErr != nil {
			return claimErr
		}
		if wasReplay {
			replay = true
			var loadErr error
			sub, loadErr = scanWebhook(tx.QueryRow(ctx, webhookSelect+` WHERE organization_id=$1 AND id=$2`, actor.Tenant.OrganizationID, existing))
			return loadErr
		}
		tag, updateErr := tx.Exec(ctx, `UPDATE fleet_partner_webhook_subscriptions SET endpoint_url=$1,event_types=$2,status=$3,
			disabled_at=CASE WHEN $3='DISABLED' THEN COALESCE(disabled_at,now()) ELSE NULL END,updated_at=now(),version=version+1
			WHERE organization_id=$4 AND id=$5 AND version=$6`, endpoint, eventsList, status, actor.Tenant.OrganizationID, id, in.ExpectedVersion)
		if updateErr != nil || tag.RowsAffected() != 1 {
			return apperrors.Conflict("Webhook was not found or changed.", updateErr)
		}
		sub, updateErr = scanWebhook(tx.QueryRow(ctx, webhookSelect+` WHERE organization_id=$1 AND id=$2`, actor.Tenant.OrganizationID, id))
		if updateErr != nil {
			return updateErr
		}
		eventType, action := "webhook.subscription_updated.v1", "webhook.subscription_updated"
		if status == WebhookStatusDisabled {
			eventType, action = "webhook.disabled.v1", "webhook.disabled"
		}
		return s.recordAndPublish(ctx, tx, actor, action, eventType, "webhook_subscription", id, sub.Version, map[string]any{"status": sub.Status, "eventTypes": sub.EventTypes})
	})
	return sub, replay, err
}

func (s *Service) RotateWebhookSecret(ctx context.Context, actor Actor, id string, expected int64, key string, overlap time.Duration) (WebhookSubscription, string, bool, error) {
	if !s.cfg.WebhooksEnabled || s.cfg.IntegrationsKillSwitch {
		return WebhookSubscription{}, "", false, apperrors.Unavailable("Webhooks are disabled.", nil)
	}
	if err := s.verifyStepUp(ctx, actor, actionWebhookManage); err != nil {
		return WebhookSubscription{}, "", false, err
	}
	if expected < 1 || overlap < 0 || overlap > s.cfg.WebhookRotationOverlapMax {
		return WebhookSubscription{}, "", false, apperrors.BadRequest("Expected version and a bounded overlap are required.", nil)
	}
	secret, err := randomToken(32)
	if err != nil {
		return WebhookSubscription{}, "", false, err
	}
	ciphertext, err := encryptSecret(s.cfg.WebhookSecretEncryptionKey, secret)
	if err != nil {
		return WebhookSubscription{}, "", false, err
	}
	fingerprint := fingerprintOf(struct {
		ID               string
		Version, Overlap int64
	}{id, expected, int64(overlap.Seconds())})
	replay := false
	var sub WebhookSubscription
	err = database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		existing, wasReplay, claimErr := claimCommand(ctx, tx, actor.Tenant.OrganizationID, "webhook.rotate:"+id, key, fingerprint, id)
		if claimErr != nil {
			return claimErr
		}
		if wasReplay {
			replay = true
			var loadErr error
			sub, loadErr = scanWebhook(tx.QueryRow(ctx, webhookSelect+` WHERE organization_id=$1 AND id=$2`, actor.Tenant.OrganizationID, existing))
			return loadErr
		}
		var previous []byte
		var previousVersion int
		if scanErr := tx.QueryRow(ctx, `SELECT secret_ciphertext,secret_version FROM fleet_partner_webhook_subscriptions WHERE organization_id=$1 AND id=$2 AND status='ACTIVE' AND version=$3 FOR UPDATE`, actor.Tenant.OrganizationID, id, expected).Scan(&previous, &previousVersion); scanErr != nil {
			return apperrors.Conflict("Webhook was not found, disabled, or changed.", scanErr)
		}
		var prevCipher, prevVersion, prevUntil any
		if overlap > 0 {
			prevCipher = previous
			prevVersion = previousVersion
			prevUntil = time.Now().UTC().Add(overlap)
		}
		tag, updateErr := tx.Exec(ctx, `UPDATE fleet_partner_webhook_subscriptions SET secret_ciphertext=$1,secret_version=secret_version+1,
			previous_secret_ciphertext=$2,previous_secret_version=$3,previous_secret_valid_until=$4,updated_at=now(),version=version+1
			WHERE organization_id=$5 AND id=$6 AND status='ACTIVE' AND version=$7`, ciphertext, prevCipher, prevVersion, prevUntil, actor.Tenant.OrganizationID, id, expected)
		if updateErr != nil || tag.RowsAffected() != 1 {
			return apperrors.Conflict("Webhook was changed.", updateErr)
		}
		sub, updateErr = scanWebhook(tx.QueryRow(ctx, webhookSelect+` WHERE organization_id=$1 AND id=$2`, actor.Tenant.OrganizationID, id))
		if updateErr != nil {
			return updateErr
		}
		return s.recordAndPublish(ctx, tx, actor, "webhook.secret_rotated", "webhook.secret_rotated.v1", "webhook_subscription", id, sub.Version, map[string]any{"status": sub.Status, "eventTypes": sub.EventTypes})
	})
	if err != nil {
		return WebhookSubscription{}, "", false, err
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
	list, err := exactAllowlist(eventTypes, allowed, "event type")
	if err != nil {
		return "", nil, err
	}
	return u.String(), list, nil
}

func (s *Service) ListWebhooks(ctx context.Context, org string) ([]WebhookSubscription, error) {
	rows, err := s.pool.Query(ctx, webhookSelect+` WHERE organization_id=$1 ORDER BY created_at DESC`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WebhookSubscription
	for rows.Next() {
		item, scanErr := scanWebhook(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) ListDeliveries(ctx context.Context, org, subscriptionID string) ([]WebhookDelivery, error) {
	rows, err := s.pool.Query(ctx, deliverySelect+` WHERE organization_id=$1 AND subscription_id=$2 ORDER BY created_at DESC LIMIT 200`, org, subscriptionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WebhookDelivery
	for rows.Next() {
		item, scanErr := scanDelivery(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) ReplayDelivery(ctx context.Context, actor Actor, deliveryID, key string) (WebhookDelivery, bool, error) {
	if !s.cfg.WebhooksEnabled || s.cfg.IntegrationsKillSwitch {
		return WebhookDelivery{}, false, apperrors.Unavailable("Webhooks are disabled.", nil)
	}
	if err := s.verifyStepUp(ctx, actor, actionWebhookManage); err != nil {
		return WebhookDelivery{}, false, err
	}
	fingerprint := fingerprintOf(struct{ ID string }{deliveryID})
	replay := false
	var created WebhookDelivery
	err := database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		newID := ids.New("whd")
		existing, wasReplay, claimErr := claimCommand(ctx, tx, actor.Tenant.OrganizationID, "webhook.delivery.replay:"+deliveryID, key, fingerprint, newID)
		if claimErr != nil {
			return claimErr
		}
		if wasReplay {
			replay = true
			var loadErr error
			created, loadErr = scanDelivery(tx.QueryRow(ctx, deliverySelect+` WHERE organization_id=$1 AND id=$2`, actor.Tenant.OrganizationID, existing))
			return loadErr
		}
		var subID, eventID, eventType, correlation string
		var attempt int
		if scanErr := tx.QueryRow(ctx, `SELECT subscription_id,source_event_id,event_type,correlation_id,attempt_no FROM fleet_partner_webhook_deliveries WHERE organization_id=$1 AND id=$2 AND status IN('FAILED','DEAD_LETTERED') FOR UPDATE`, actor.Tenant.OrganizationID, deliveryID).Scan(&subID, &eventID, &eventType, &correlation, &attempt); scanErr != nil {
			return apperrors.NotFound("Failed webhook delivery was not found.", scanErr)
		}
		if err := tx.QueryRow(ctx, `SELECT COALESCE(max(attempt_no),0)+1 FROM fleet_partner_webhook_deliveries WHERE subscription_id=$1 AND source_event_id=$2`, subID, eventID).Scan(&attempt); err != nil {
			return err
		}
		_, insertErr := tx.Exec(ctx, `INSERT INTO fleet_partner_webhook_deliveries(id,organization_id,subscription_id,source_event_id,event_type,attempt_no,status,scheduled_at,correlation_id,replay_of_delivery_id) VALUES($1,$2,$3,$4,$5,$6,'PENDING',now(),$7,$8)`, newID, actor.Tenant.OrganizationID, subID, eventID, eventType, attempt, correlation, deliveryID)
		if insertErr != nil {
			return insertErr
		}
		jobID, enqueueErr := s.queue.Enqueue(ctx, tx, jobs.EnqueueRequest{OrganizationID: actor.Tenant.OrganizationID, JobType: JobTypeWebhookDelivery, DedupeKey: "webhook:" + newID, IdempotencyKey: key, Payload: map[string]any{"organizationId": actor.Tenant.OrganizationID, "deliveryId": newID}, CorrelationID: correlation, CausationID: eventID, MaxAttempts: s.cfg.WebhookDeliveryMaxAttempts})
		if enqueueErr != nil {
			return enqueueErr
		}
		if _, updateErr := tx.Exec(ctx, `UPDATE fleet_partner_webhook_deliveries SET job_id=$1 WHERE id=$2`, jobID, newID); updateErr != nil {
			return updateErr
		}
		created, insertErr = scanDelivery(tx.QueryRow(ctx, deliverySelect+` WHERE organization_id=$1 AND id=$2`, actor.Tenant.OrganizationID, newID))
		if insertErr != nil {
			return insertErr
		}
		return s.recordAndPublish(ctx, tx, actor, "webhook.delivery_replayed", "webhook.delivery_replayed.v1", "webhook_delivery", newID, 1, map[string]any{"status": created.Status, "eventId": eventID, "attempt": attempt})
	})
	return created, replay, err
}

func exactAllowlist(values []string, allowed map[string]bool, label string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" || !allowed[value] {
			return nil, apperrors.BadRequest("Unsupported "+label+".", nil)
		}
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	if len(out) == 0 {
		return nil, apperrors.BadRequest("At least one "+label+" is required.", nil)
	}
	sort.Strings(out)
	return out, nil
}

func fingerprintOf(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func claimCommand(ctx context.Context, tx pgx.Tx, org, command, key, fingerprint, resourceID string) (string, bool, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", false, apperrors.BadRequest("Idempotency-Key header is required.", nil)
	}
	var inserted string
	err := tx.QueryRow(ctx, `INSERT INTO fleet_partner_integration_commands(organization_id,command_type,idempotency_key,request_fingerprint,resource_id) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING RETURNING resource_id`, org, command, key, fingerprint, resourceID).Scan(&inserted)
	if err == nil {
		return inserted, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", false, err
	}
	var storedFingerprint string
	if err = tx.QueryRow(ctx, `SELECT request_fingerprint,resource_id FROM fleet_partner_integration_commands WHERE organization_id=$1 AND command_type=$2 AND idempotency_key=$3`, org, command, key).Scan(&storedFingerprint, &inserted); err != nil {
		return "", false, err
	}
	if storedFingerprint != fingerprint {
		return "", false, apperrors.Conflict("This idempotency key was already used with different input.", nil)
	}
	return inserted, true, nil
}

func (s *Service) recordAndPublish(ctx context.Context, tx pgx.Tx, actor Actor, action, eventType, aggregateType, id string, version int64, values map[string]any) error {
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

const apiClientSelect = `SELECT id,organization_id,name,client_id,scopes,ip_allowlist::text[],status,expires_at,last_used_at,rotated_at,revoked_at,created_by,secret_version,version,created_at,updated_at FROM fleet_partner_organization_api_clients`

type scanner interface{ Scan(...any) error }

func scanAPIClient(row scanner) (APIClient, error) {
	var c APIClient
	err := row.Scan(&c.ID, &c.OrganizationID, &c.Name, &c.ClientID, &c.Scopes, &c.IPAllowlist, &c.Status, &c.ExpiresAt, &c.LastUsedAt, &c.RotatedAt, &c.RevokedAt, &c.CreatedBy, &c.SecretVersion, &c.Version, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

const webhookSelect = `SELECT id,organization_id,endpoint_url,event_types,status,failure_count,disabled_at,created_by,secret_version,version,created_at,updated_at FROM fleet_partner_webhook_subscriptions`

func scanWebhook(row scanner) (WebhookSubscription, error) {
	var s WebhookSubscription
	err := row.Scan(&s.ID, &s.OrganizationID, &s.EndpointURL, &s.EventTypes, &s.Status, &s.FailureCount, &s.DisabledAt, &s.CreatedBy, &s.SecretVersion, &s.Version, &s.CreatedAt, &s.UpdatedAt)
	return s, err
}

const deliverySelect = `SELECT id,organization_id,subscription_id,source_event_id,event_type,attempt_no,status,COALESCE(response_status,0),COALESCE(response_body_redacted,''),scheduled_at,delivered_at,next_attempt_at,COALESCE(error_class,''),COALESCE(error_code,''),correlation_id,COALESCE(job_id,''),COALESCE(replay_of_delivery_id,''),version,created_at,updated_at FROM fleet_partner_webhook_deliveries`

func scanDelivery(row scanner) (WebhookDelivery, error) {
	var d WebhookDelivery
	err := row.Scan(&d.ID, &d.OrganizationID, &d.SubscriptionID, &d.SourceEventID, &d.EventType, &d.AttemptNo, &d.Status, &d.ResponseStatus, &d.ResponseExcerpt, &d.ScheduledAt, &d.DeliveredAt, &d.NextAttemptAt, &d.ErrorClass, &d.ErrorCode, &d.CorrelationID, &d.JobID, &d.ReplayOfDeliveryID, &d.Version, &d.CreatedAt, &d.UpdatedAt)
	return d, err
}
