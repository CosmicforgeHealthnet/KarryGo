package usecases

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/integrations/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Authenticate resolves an organization API client from its credential,
// enforcing status, expiry, the current or overlapping previous secret, the
// exact scope and the IP allowlist, and audits the authenticated request.
func (s *Service) Authenticate(ctx context.Context, clientID, secret, sourceIP, requiredScope, requestID, correlationID string) (models.AuthenticatedClient, error) {
	if !s.cfg.OrganizationAPIClientsEnabled || s.cfg.IntegrationsKillSwitch {
		s.authMetric("feature_disabled")
		return models.AuthenticatedClient{}, apperrors.Unauthorized("Client authentication failed.", nil)
	}
	credential, err := s.store.ClientCredential(ctx, clientID)
	result := models.AuthenticatedClient{APIClientID: credential.APIClientID, ClientID: credential.ClientID,
		OrganizationID: credential.OrganizationID, Scopes: credential.Scopes, SecretVersion: credential.SecretVersion}
	status, expiresAt, previousValidUntil := credential.Status, credential.ExpiresAt, credential.PreviousValidUntil
	currentHash, previousHash, previousVersion, allowlist := credential.SecretHash, credential.PreviousHash, credential.PreviousVersion, credential.IPAllowlist
	if err != nil {
		s.authMetric("unknown_client")
		return models.AuthenticatedClient{}, apperrors.Unauthorized("Client authentication failed.", nil)
	}
	now := time.Now().UTC()
	if status != models.APIClientStatusActive {
		s.authMetric("revoked")
		return models.AuthenticatedClient{}, apperrors.Unauthorized("Client authentication failed.", nil)
	}
	if expiresAt != nil && !expiresAt.After(now) {
		s.authMetric("expired")
		return models.AuthenticatedClient{}, apperrors.Unauthorized("Client authentication failed.", nil)
	}
	matched := models.SecretMatches(s.cfg.APIClientHashKey, secret, currentHash)
	if !matched && len(previousHash) > 0 && previousValidUntil != nil && previousValidUntil.After(now) {
		matched = models.SecretMatches(s.cfg.APIClientHashKey, secret, previousHash)
		if matched && previousVersion != nil {
			result.SecretVersion = *previousVersion
		}
	}
	if !matched {
		s.authMetric("invalid_secret")
		return models.AuthenticatedClient{}, apperrors.Unauthorized("Client authentication failed.", nil)
	}
	if !models.ContainsExact(result.Scopes, requiredScope) {
		s.authMetric("scope_denied")
		return models.AuthenticatedClient{}, apperrors.Forbidden("Client scope does not permit this operation.", nil)
	}
	if len(allowlist) > 0 && !models.IPAllowed(sourceIP, allowlist) {
		s.authMetric("ip_denied")
		return models.AuthenticatedClient{}, apperrors.Forbidden("Client source IP is not permitted.", nil)
	}
	if err = s.store.TouchAPIClient(ctx, result.APIClientID); err != nil {
		return models.AuthenticatedClient{}, apperrors.Unavailable("Client authentication could not be finalized.", err)
	}
	entry := audit.Entry{OrganizationID: result.OrganizationID, ActorType: string(tenant.ActorTypeOrganizationAPIClient),
		ActorID: result.APIClientID, Action: "api_client.request_authenticated", ResourceType: "organization_api_client",
		ResourceID: result.APIClientID, RequestID: requestID, CorrelationID: correlationID,
		NetworkMeta: map[string]string{"sourceIp": sourceIP}, NewValues: map[string]any{"scope": requiredScope}}
	if err = s.store.WithinTx(ctx, func(tx pgx.Tx) error { return s.recorder.Record(ctx, tx, entry) }); err != nil {
		return models.AuthenticatedClient{}, apperrors.Unavailable("Client authentication audit could not be recorded.", err)
	}
	s.authMetric("success")
	return result, nil
}

// RecordAuthOutcome counts one authentication outcome class.
func (s *Service) RecordAuthOutcome(class string) { s.authMetric(class) }

func (s *Service) authMetric(class string) {
	if s.metrics != nil {
		s.metrics.IncCounter("fleet_partner_api_client_auth_total", map[string]string{"result": class}, 1)
	}
}
