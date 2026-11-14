// Package http serves the Section 55 integration management routes, the
// organization API-client routes and the client credential middleware.
package http

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/integrations/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Permissions guarding the management routes.
const (
	PermissionIntegrationsRead   = "integrations.read"
	PermissionIntegrationsManage = "integrations.manage"
	PermissionWebhooksManage     = "webhooks.manage"
)

// Service is the slice of the integration use cases the handler calls.
type Service interface {
	ListAPIClients(ctx context.Context, organizationID string) ([]models.APIClient, error)
	CreateAPIClient(ctx context.Context, actor models.Actor, in models.CreateAPIClientInput) (models.APIClient, string, bool, error)
	RotateAPIClient(ctx context.Context, actor models.Actor, id string, expected int64, idempotencyKey string, overlap time.Duration) (models.APIClient, string, bool, error)
	RevokeAPIClient(ctx context.Context, actor models.Actor, id string, expected int64, key string) (models.APIClient, bool, error)
	ListWebhooks(ctx context.Context, org string) ([]models.WebhookSubscription, error)
	CreateWebhook(ctx context.Context, actor models.Actor, in models.CreateWebhookInput) (models.WebhookSubscription, string, bool, error)
	UpdateWebhook(ctx context.Context, actor models.Actor, id string, in models.UpdateWebhookInput) (models.WebhookSubscription, bool, error)
	RotateWebhookSecret(ctx context.Context, actor models.Actor, id string, expected int64, key string, overlap time.Duration) (models.WebhookSubscription, string, bool, error)
	ListDeliveries(ctx context.Context, org, subscriptionID string) ([]models.WebhookDelivery, error)
	ReplayDelivery(ctx context.Context, actor models.Actor, deliveryID, key string) (models.WebhookDelivery, bool, error)
}

// Authenticator is the use case the client credential middleware calls.
type Authenticator interface {
	Authenticate(ctx context.Context, clientID, secret, sourceIP, requiredScope, requestID, correlationID string) (models.AuthenticatedClient, error)
	RecordAuthOutcome(class string)
}

// Handler serves the integration routes.
type Handler struct {
	service    Service
	authorizer *accesscontrol.Authorizer
}

// NewHandler builds the handler.
func NewHandler(service Service, authorizer *accesscontrol.Authorizer) *Handler {
	return &Handler{service: service, authorizer: authorizer}
}

func (h *Handler) RegisterTenantRoutes(g *gin.RouterGroup) {
	g.GET("/integrations/api-clients", h.listAPIClients)
	g.POST("/integrations/api-clients", h.createAPIClient)
	g.POST("/integrations/api-clients/:id/rotate", h.rotateAPIClient)
	g.POST("/integrations/api-clients/:id/revoke", h.revokeAPIClient)
	g.GET("/integrations/webhooks", h.listWebhooks)
	g.POST("/integrations/webhooks", h.createWebhook)
	g.PATCH("/integrations/webhooks/:id", h.updateWebhook)
	g.POST("/integrations/webhooks/:id/rotate-secret", h.rotateWebhook)
	g.GET("/integrations/webhooks/:id/deliveries", h.listDeliveries)
	g.POST("/integrations/webhook-deliveries/:id/replay", h.replayDelivery)
}
func (h *Handler) RegisterOrganizationAPIRoutes(g *gin.RouterGroup) {
	g.GET("/context", h.organizationContext)
}

func (h *Handler) tenantAndAuthorize(c *gin.Context, permission string) (tenant.Context, bool) {
	t, ok := middleware.TenantFromGin(c)
	if !ok {
		respond.Error(c, apperrors.Forbidden("Organization scope is required.", nil))
		return tenant.Context{}, false
	}
	if h.authorizer == nil {
		respond.Error(c, apperrors.Forbidden("You do not have permission to perform this action.", nil))
		return tenant.Context{}, false
	}
	version := int64(1)
	if value, exists := c.Get(middleware.TenantRoleVersionKey); exists {
		if v, valid := value.(int64); valid {
			version = v
		}
	}
	if err := h.authorizer.Require(c, t.OrganizationID, t.OrganizationUserID, version, permission, accesscontrol.Target{}); err != nil {
		respond.Error(c, err)
		return tenant.Context{}, false
	}
	return t, true
}
func actorFrom(c *gin.Context, t tenant.Context, step string) models.Actor {
	claims, _ := auth.ClaimsFromContext(c)
	return models.Actor{Tenant: t, IdentityID: claims.Subject, SessionID: claims.SessionID, StepUpReference: step}
}
func bind(c *gin.Context, value any) bool {
	if c.ShouldBindJSON(value) != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
		return false
	}
	return true
}
func versionFrom(c *gin.Context, body int64) int64 {
	if raw := strings.Trim(strings.TrimSpace(c.GetHeader("If-Match")), `"`); raw != "" {
		if v, err := strconv.ParseInt(raw, 10, 64); err == nil {
			return v
		}
	}
	return body
}

func apiClientResponse(c models.APIClient) map[string]any {
	return map[string]any{"id": c.ID, "organizationId": c.OrganizationID, "name": c.Name, "clientId": c.ClientID, "scopes": c.Scopes, "ipAllowlist": c.IPAllowlist, "status": c.Status, "expiresAt": c.ExpiresAt, "lastUsedAt": c.LastUsedAt, "rotatedAt": c.RotatedAt, "revokedAt": c.RevokedAt, "secretVersion": c.SecretVersion, "version": c.Version, "createdAt": c.CreatedAt, "updatedAt": c.UpdatedAt}
}
func webhookResponse(s models.WebhookSubscription) map[string]any {
	return map[string]any{"id": s.ID, "organizationId": s.OrganizationID, "endpointUrl": s.EndpointURL, "eventTypes": s.EventTypes, "status": s.Status, "failureCount": s.FailureCount, "disabledAt": s.DisabledAt, "secretVersion": s.SecretVersion, "version": s.Version, "createdAt": s.CreatedAt, "updatedAt": s.UpdatedAt}
}
func deliveryResponse(d models.WebhookDelivery) map[string]any {
	return map[string]any{"id": d.ID, "organizationId": d.OrganizationID, "subscriptionId": d.SubscriptionID, "eventId": d.SourceEventID, "eventType": d.EventType, "attempt": d.AttemptNo, "status": d.Status, "responseStatus": d.ResponseStatus, "responseExcerpt": d.ResponseExcerpt, "scheduledAt": d.ScheduledAt, "deliveredAt": d.DeliveredAt, "nextAttemptAt": d.NextAttemptAt, "errorClass": d.ErrorClass, "errorCode": d.ErrorCode, "correlationId": d.CorrelationID, "replayOfDeliveryId": d.ReplayOfDeliveryID, "version": d.Version, "createdAt": d.CreatedAt, "updatedAt": d.UpdatedAt}
}

func (h *Handler) listAPIClients(c *gin.Context) {
	t, ok := h.tenantAndAuthorize(c, PermissionIntegrationsRead)
	if !ok {
		return
	}
	items, err := h.service.ListAPIClients(c, t.OrganizationID)
	if err != nil {
		respond.Error(c, err)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, apiClientResponse(item))
	}
	respond.OK(c, out)
}
func (h *Handler) createAPIClient(c *gin.Context) {
	t, ok := h.tenantAndAuthorize(c, PermissionIntegrationsManage)
	if !ok {
		return
	}
	var body struct {
		Name            string     `json:"name"`
		Scopes          []string   `json:"scopes"`
		IPAllowlist     []string   `json:"ipAllowlist"`
		ExpiresAt       *time.Time `json:"expiresAt"`
		StepUpReference string     `json:"stepUpReference"`
	}
	if !bind(c, &body) {
		return
	}
	item, secret, replay, err := h.service.CreateAPIClient(c, actorFrom(c, t, body.StepUpReference), models.CreateAPIClientInput{Name: body.Name, Scopes: body.Scopes, IPAllowlist: body.IPAllowlist, ExpiresAt: body.ExpiresAt, IdempotencyKey: c.GetHeader("Idempotency-Key")})
	if err != nil {
		respond.Error(c, err)
		return
	}
	view := apiClientResponse(item)
	view["secret"] = secret
	view["secretShownOnce"] = !replay
	respond.Created(c, view)
}
func (h *Handler) rotateAPIClient(c *gin.Context) {
	t, ok := h.tenantAndAuthorize(c, PermissionIntegrationsManage)
	if !ok {
		return
	}
	var body struct {
		ExpectedVersion    int64  `json:"expectedVersion"`
		GracePeriodSeconds int    `json:"gracePeriodSeconds"`
		StepUpReference    string `json:"stepUpReference"`
	}
	if !bind(c, &body) {
		return
	}
	item, secret, replay, err := h.service.RotateAPIClient(c, actorFrom(c, t, body.StepUpReference), c.Param("id"), versionFrom(c, body.ExpectedVersion), c.GetHeader("Idempotency-Key"), time.Duration(body.GracePeriodSeconds)*time.Second)
	if err != nil {
		respond.Error(c, err)
		return
	}
	view := apiClientResponse(item)
	view["secret"] = secret
	view["secretShownOnce"] = !replay
	respond.OK(c, view)
}
func (h *Handler) revokeAPIClient(c *gin.Context) {
	t, ok := h.tenantAndAuthorize(c, PermissionIntegrationsManage)
	if !ok {
		return
	}
	var body struct {
		ExpectedVersion int64  `json:"expectedVersion"`
		StepUpReference string `json:"stepUpReference"`
	}
	if !bind(c, &body) {
		return
	}
	item, _, err := h.service.RevokeAPIClient(c, actorFrom(c, t, body.StepUpReference), c.Param("id"), versionFrom(c, body.ExpectedVersion), c.GetHeader("Idempotency-Key"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, apiClientResponse(item))
}
func (h *Handler) listWebhooks(c *gin.Context) {
	t, ok := h.tenantAndAuthorize(c, PermissionIntegrationsRead)
	if !ok {
		return
	}
	items, err := h.service.ListWebhooks(c, t.OrganizationID)
	if err != nil {
		respond.Error(c, err)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, webhookResponse(item))
	}
	respond.OK(c, out)
}
func (h *Handler) createWebhook(c *gin.Context) {
	t, ok := h.tenantAndAuthorize(c, PermissionWebhooksManage)
	if !ok {
		return
	}
	var body struct {
		EndpointURL     string   `json:"endpointUrl"`
		EventTypes      []string `json:"eventTypes"`
		StepUpReference string   `json:"stepUpReference"`
	}
	if !bind(c, &body) {
		return
	}
	item, secret, replay, err := h.service.CreateWebhook(c, actorFrom(c, t, body.StepUpReference), models.CreateWebhookInput{EndpointURL: body.EndpointURL, EventTypes: body.EventTypes, IdempotencyKey: c.GetHeader("Idempotency-Key")})
	if err != nil {
		respond.Error(c, err)
		return
	}
	view := webhookResponse(item)
	view["secret"] = secret
	view["secretShownOnce"] = !replay
	respond.Created(c, view)
}
func (h *Handler) updateWebhook(c *gin.Context) {
	t, ok := h.tenantAndAuthorize(c, PermissionWebhooksManage)
	if !ok {
		return
	}
	var body struct {
		EndpointURL     string   `json:"endpointUrl"`
		Status          string   `json:"status"`
		StepUpReference string   `json:"stepUpReference"`
		EventTypes      []string `json:"eventTypes"`
		ExpectedVersion int64    `json:"expectedVersion"`
	}
	if !bind(c, &body) {
		return
	}
	item, _, err := h.service.UpdateWebhook(c, actorFrom(c, t, body.StepUpReference), c.Param("id"), models.UpdateWebhookInput{EndpointURL: body.EndpointURL, EventTypes: body.EventTypes, Status: body.Status, ExpectedVersion: versionFrom(c, body.ExpectedVersion), IdempotencyKey: c.GetHeader("Idempotency-Key")})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, webhookResponse(item))
}
func (h *Handler) rotateWebhook(c *gin.Context) {
	t, ok := h.tenantAndAuthorize(c, PermissionWebhooksManage)
	if !ok {
		return
	}
	var body struct {
		ExpectedVersion    int64  `json:"expectedVersion"`
		GracePeriodSeconds int    `json:"gracePeriodSeconds"`
		StepUpReference    string `json:"stepUpReference"`
	}
	if !bind(c, &body) {
		return
	}
	item, secret, replay, err := h.service.RotateWebhookSecret(c, actorFrom(c, t, body.StepUpReference), c.Param("id"), versionFrom(c, body.ExpectedVersion), c.GetHeader("Idempotency-Key"), time.Duration(body.GracePeriodSeconds)*time.Second)
	if err != nil {
		respond.Error(c, err)
		return
	}
	view := webhookResponse(item)
	view["secret"] = secret
	view["secretShownOnce"] = !replay
	respond.OK(c, view)
}
func (h *Handler) listDeliveries(c *gin.Context) {
	t, ok := h.tenantAndAuthorize(c, PermissionIntegrationsRead)
	if !ok {
		return
	}
	items, err := h.service.ListDeliveries(c, t.OrganizationID, c.Param("id"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, deliveryResponse(item))
	}
	respond.OK(c, out)
}
func (h *Handler) replayDelivery(c *gin.Context) {
	t, ok := h.tenantAndAuthorize(c, PermissionWebhooksManage)
	if !ok {
		return
	}
	var body struct {
		Reason          string `json:"reason"`
		StepUpReference string `json:"stepUpReference"`
	}
	if !bind(c, &body) {
		return
	}
	item, _, err := h.service.ReplayDelivery(c, actorFrom(c, t, body.StepUpReference), c.Param("id"), c.GetHeader("Idempotency-Key"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.Created(c, deliveryResponse(item))
}
func (h *Handler) organizationContext(c *gin.Context) {
	t, ok := middleware.TenantFromGin(c)
	if !ok || t.ActorType != tenant.ActorTypeOrganizationAPIClient {
		respond.Error(c, apperrors.Forbidden("Organization API client scope is required.", nil))
		return
	}
	respond.OK(c, map[string]any{"organizationId": t.OrganizationID, "apiClientId": t.ActorID, "scopes": c.MustGet("organization_api_client_scopes")})
}

// ClientAuthMiddleware resolves the organization exclusively from the API
// credential. Organization identifiers supplied in headers, query or JSON
// bodies are rejected rather than allowed to override that trusted scope.
func ClientAuthMiddleware(s Authenticator, requiredScope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		clientID, secret, ok := c.Request.BasicAuth()
		if !ok || strings.TrimSpace(clientID) == "" || secret == "" {
			s.RecordAuthOutcome("malformed_authorization")
			respond.Error(c, apperrors.Unauthorized("Client authentication is required.", nil))
			return
		}
		if c.GetHeader("X-Organization-ID") != "" || c.Query("organizationId") != "" || bodyContainsOrganizationID(c) {
			s.RecordAuthOutcome("organization_override_denied")
			respond.Error(c, apperrors.Forbidden("Organization scope is derived from the client credential.", nil))
			return
		}
		resolved, err := s.Authenticate(c, clientID, secret, c.ClientIP(), requiredScope, httpx.GetRequestID(c), middleware.GetCorrelationID(c))
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
