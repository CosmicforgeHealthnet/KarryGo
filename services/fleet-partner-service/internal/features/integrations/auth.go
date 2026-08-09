package integrations

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"

	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

func (s *Service) Authenticate(ctx *gin.Context, clientID, secret, sourceIP, requiredScope string) (AuthenticatedClient, error) {
	if !s.cfg.OrganizationAPIClientsEnabled || s.cfg.IntegrationsKillSwitch {
		s.authMetric("feature_disabled")
		return AuthenticatedClient{}, apperrors.Unauthorized("Client authentication failed.", nil)
	}
	var result AuthenticatedClient
	var status string
	var expiresAt, previousValidUntil *time.Time
	var currentHash, previousHash []byte
	var previousVersion *int
	var allowlist []string
	err := s.pool.QueryRow(ctx, `SELECT id,client_id,organization_id,status,expires_at,scopes,ip_allowlist::text[],
		secret_hash,secret_version,previous_secret_hash,previous_secret_version,previous_secret_valid_until
		FROM fleet_partner_organization_api_clients WHERE client_id=$1`, clientID).Scan(
		&result.APIClientID, &result.ClientID, &result.OrganizationID, &status, &expiresAt, &result.Scopes,
		&allowlist, &currentHash, &result.SecretVersion, &previousHash, &previousVersion, &previousValidUntil)
	if err != nil {
		s.authMetric("unknown_client")
		return AuthenticatedClient{}, apperrors.Unauthorized("Client authentication failed.", nil)
	}
	now := time.Now().UTC()
	if status != APIClientStatusActive {
		s.authMetric("revoked")
		return AuthenticatedClient{}, apperrors.Unauthorized("Client authentication failed.", nil)
	}
	if expiresAt != nil && !expiresAt.After(now) {
		s.authMetric("expired")
		return AuthenticatedClient{}, apperrors.Unauthorized("Client authentication failed.", nil)
	}
	matched := secretMatches(s.cfg.APIClientHashKey, secret, currentHash)
	if !matched && len(previousHash) > 0 && previousValidUntil != nil && previousValidUntil.After(now) {
		matched = secretMatches(s.cfg.APIClientHashKey, secret, previousHash)
		if matched && previousVersion != nil {
			result.SecretVersion = *previousVersion
		}
	}
	if !matched {
		s.authMetric("invalid_secret")
		return AuthenticatedClient{}, apperrors.Unauthorized("Client authentication failed.", nil)
	}
	if !containsExact(result.Scopes, requiredScope) {
		s.authMetric("scope_denied")
		return AuthenticatedClient{}, apperrors.Forbidden("Client scope does not permit this operation.", nil)
	}
	if len(allowlist) > 0 && !ipAllowed(sourceIP, allowlist) {
		s.authMetric("ip_denied")
		return AuthenticatedClient{}, apperrors.Forbidden("Client source IP is not permitted.", nil)
	}
	if _, err = s.pool.Exec(ctx, `UPDATE fleet_partner_organization_api_clients SET last_used_at=now()
		WHERE id=$1 AND status='ACTIVE' AND (last_used_at IS NULL OR last_used_at < now()-interval '30 seconds')`, result.APIClientID); err != nil {
		return AuthenticatedClient{}, apperrors.Unavailable("Client authentication could not be finalized.", err)
	}
	entry := audit.Entry{OrganizationID: result.OrganizationID, ActorType: string(tenant.ActorTypeOrganizationAPIClient),
		ActorID: result.APIClientID, Action: "api_client.request_authenticated", ResourceType: "organization_api_client",
		ResourceID: result.APIClientID, RequestID: httpx.GetRequestID(ctx), CorrelationID: middleware.GetCorrelationID(ctx),
		NetworkMeta: map[string]string{"sourceIp": sourceIP}, NewValues: map[string]any{"scope": requiredScope}}
	if err = s.recorder.Record(ctx, s.pool, entry); err != nil {
		return AuthenticatedClient{}, apperrors.Unavailable("Client authentication audit could not be recorded.", err)
	}
	s.authMetric("success")
	return result, nil
}

func (s *Service) authMetric(class string) {
	if s.metrics != nil {
		s.metrics.IncCounter("fleet_partner_api_client_auth_total", map[string]string{"result": class}, 1)
	}
}

func containsExact(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func ipAllowed(raw string, cidrs []string) bool {
	ip := net.ParseIP(strings.TrimSpace(raw))
	if ip == nil {
		return false
	}
	for _, rawCIDR := range cidrs {
		_, network, err := net.ParseCIDR(rawCIDR)
		if err == nil && network.Contains(ip) {
			return true
		}
	}
	return false
}

// ClientAuthMiddleware resolves the organization exclusively from the API
// credential. Organization identifiers supplied in headers, query or JSON
// bodies are rejected rather than allowed to override that trusted scope.
func (s *Service) ClientAuthMiddleware(requiredScope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		clientID, secret, ok := c.Request.BasicAuth()
		if !ok || strings.TrimSpace(clientID) == "" || secret == "" {
			s.authMetric("malformed_authorization")
			respond.Error(c, apperrors.Unauthorized("Client authentication is required.", nil))
			return
		}
		if c.GetHeader("X-Organization-ID") != "" || c.Query("organizationId") != "" || bodyContainsOrganizationID(c) {
			s.authMetric("organization_override_denied")
			respond.Error(c, apperrors.Forbidden("Organization scope is derived from the client credential.", nil))
			return
		}
		resolved, err := s.Authenticate(c, clientID, secret, c.ClientIP(), requiredScope)
		if err != nil {
			respond.Error(c, err)
			return
		}
		t := tenant.Context{OrganizationID: resolved.OrganizationID, ActorType: tenant.ActorTypeOrganizationAPIClient,
			ActorID: resolved.APIClientID, RequestID: httpx.GetRequestID(c), CorrelationID: middleware.GetCorrelationID(c)}
		c.Set(middleware.TenantContextKey, t)
		c.Set(middleware.TenantOrganizationIDKey, t.OrganizationID)
		c.Set("organization_api_client_id", resolved.APIClientID)
		c.Set("organization_api_client_scopes", resolved.Scopes)
		c.Request = c.Request.WithContext(tenant.Into(c.Request.Context(), t))
		c.Next()
	}
}

func bodyContainsOrganizationID(c *gin.Context) bool {
	if c.Request.Body == nil || c.Request.ContentLength == 0 {
		return false
	}
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return true
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	var value map[string]json.RawMessage
	if json.Unmarshal(raw, &value) != nil {
		return false
	}
	_, exists := value["organizationId"]
	return exists
}
