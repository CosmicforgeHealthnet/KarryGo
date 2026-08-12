package integrations

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/auth"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

type Handler struct {
	service    *Service
	authorizer *accesscontrol.Authorizer
}

func NewHandler(service *Service, authorizer *accesscontrol.Authorizer) *Handler {
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
func actorFrom(c *gin.Context, t tenant.Context, step string) Actor {
	claims, _ := auth.ClaimsFromContext(c)
	return Actor{Tenant: t, IdentityID: claims.Subject, SessionID: claims.SessionID, StepUpReference: step}
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

func apiClientResponse(c APIClient) map[string]any {
	return map[string]any{"id": c.ID, "organizationId": c.OrganizationID, "name": c.Name, "clientId": c.ClientID, "scopes": c.Scopes, "ipAllowlist": c.IPAllowlist, "status": c.Status, "expiresAt": c.ExpiresAt, "lastUsedAt": c.LastUsedAt, "rotatedAt": c.RotatedAt, "revokedAt": c.RevokedAt, "secretVersion": c.SecretVersion, "version": c.Version, "createdAt": c.CreatedAt, "updatedAt": c.UpdatedAt}
}
func webhookResponse(s WebhookSubscription) map[string]any {
	return map[string]any{"id": s.ID, "organizationId": s.OrganizationID, "endpointUrl": s.EndpointURL, "eventTypes": s.EventTypes, "status": s.Status, "failureCount": s.FailureCount, "disabledAt": s.DisabledAt, "secretVersion": s.SecretVersion, "version": s.Version, "createdAt": s.CreatedAt, "updatedAt": s.UpdatedAt}
}
func deliveryResponse(d WebhookDelivery) map[string]any {
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
	item, secret, replay, err := h.service.CreateAPIClient(c, actorFrom(c, t, body.StepUpReference), CreateAPIClientInput{Name: body.Name, Scopes: body.Scopes, IPAllowlist: body.IPAllowlist, ExpiresAt: body.ExpiresAt, IdempotencyKey: c.GetHeader("Idempotency-Key")})
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
	item, secret, replay, err := h.service.CreateWebhook(c, actorFrom(c, t, body.StepUpReference), CreateWebhookInput{EndpointURL: body.EndpointURL, EventTypes: body.EventTypes, IdempotencyKey: c.GetHeader("Idempotency-Key")})
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
	item, _, err := h.service.UpdateWebhook(c, actorFrom(c, t, body.StepUpReference), c.Param("id"), UpdateWebhookInput{EndpointURL: body.EndpointURL, EventTypes: body.EventTypes, Status: body.Status, ExpectedVersion: versionFrom(c, body.ExpectedVersion), IdempotencyKey: c.GetHeader("Idempotency-Key")})
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
