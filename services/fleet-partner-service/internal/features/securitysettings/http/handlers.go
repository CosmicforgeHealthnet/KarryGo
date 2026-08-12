package http

import (
	"errors"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/auth"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/securitysettings/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/securitysettings/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/securitysettings/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/concurrency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Permissions guarding the routes.
const (
	PermissionRead   = "security.settings.read"
	PermissionManage = "security.settings.manage"
)

// Handler serves the corporate security-settings routes.
type Handler struct {
	service    *usecases.Service
	authorizer *accesscontrol.Authorizer
}

// NewHandler builds the handler.
func NewHandler(service *usecases.Service, authorizer *accesscontrol.Authorizer) *Handler {
	return &Handler{service: service, authorizer: authorizer}
}

// RegisterTenantRoutes mounts the security-settings surface.
func (h *Handler) RegisterTenantRoutes(group *gin.RouterGroup) {
	group.GET("/security-settings", h.get)
	group.PUT("/security-settings", h.update)
}

func requireTenant(c *gin.Context) (tenant.Context, bool) {
	tenantCtx, ok := middleware.TenantFromGin(c)
	if !ok {
		respond.Error(c, apperrors.Forbidden("Organization scope is required.", nil))
		return tenant.Context{}, false
	}
	return tenantCtx, true
}

func (h *Handler) authorize(c *gin.Context, t tenant.Context, permission string) bool {
	if h.authorizer == nil {
		respond.Error(c, apperrors.Forbidden("You do not have permission to perform this action.", nil))
		return false
	}
	if err := h.authorizer.Require(c.Request.Context(), t.OrganizationID, t.OrganizationUserID,
		roleVersionFrom(c), permission, accesscontrol.Target{}); err != nil {
		respond.Error(c, err)
		return false
	}
	return true
}

func roleVersionFrom(c *gin.Context) int64 {
	if value, ok := c.Get(middleware.TenantRoleVersionKey); ok {
		if version, ok := value.(int64); ok {
			return version
		}
	}
	return 1
}

// SettingsView is the wire response shape.
type SettingsView struct {
	MFARequiredForSensitiveActions   bool   `json:"mfaRequiredForSensitiveActions"`
	StepUpFreshnessSeconds           int    `json:"stepUpFreshnessSeconds"`
	SessionRevocationPolicy          string `json:"sessionRevocationPolicy"`
	AllowedProviderAccessPolicy      string `json:"allowedProviderAccessPolicy"`
	SensitiveExportThresholdKobo     int64  `json:"sensitiveExportThresholdKobo"`
	RestrictedDocumentStepUpRequired bool   `json:"restrictedDocumentStepUpRequired"`
	AuditCapturePolicy               string `json:"auditCapturePolicy"`
	RetentionPreferenceDays          int    `json:"retentionPreferenceDays"`
	Version                          int64  `json:"version"`
}

func newSettingsView(s models.Settings) SettingsView {
	return SettingsView{
		MFARequiredForSensitiveActions: s.MFARequiredForSensitiveActions, StepUpFreshnessSeconds: s.StepUpFreshnessSeconds,
		SessionRevocationPolicy: s.SessionRevocationPolicy, AllowedProviderAccessPolicy: s.AllowedProviderAccessPolicy,
		SensitiveExportThresholdKobo: s.SensitiveExportThresholdKobo, RestrictedDocumentStepUpRequired: s.RestrictedDocumentStepUpRequired,
		AuditCapturePolicy: s.AuditCapturePolicy, RetentionPreferenceDays: s.RetentionPreferenceDays, Version: s.Version,
	}
}

func (h *Handler) get(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionRead) {
		return
	}
	if h.service == nil {
		respond.Error(c, apperrors.Unavailable("Security settings are not available.", nil))
		return
	}
	settings, err := h.service.Get(c.Request.Context(), t.OrganizationID)
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, newSettingsView(settings))
}

type updateSettingsRequest struct {
	ExpectedVersion                  *int64 `json:"expectedVersion"`
	MFARequiredForSensitiveActions   bool   `json:"mfaRequiredForSensitiveActions"`
	StepUpFreshnessSeconds           int    `json:"stepUpFreshnessSeconds"`
	SessionRevocationPolicy          string `json:"sessionRevocationPolicy"`
	AllowedProviderAccessPolicy      string `json:"allowedProviderAccessPolicy"`
	SensitiveExportThresholdKobo     int64  `json:"sensitiveExportThresholdKobo"`
	RestrictedDocumentStepUpRequired bool   `json:"restrictedDocumentStepUpRequired"`
	AuditCapturePolicy               string `json:"auditCapturePolicy"`
	RetentionPreferenceDays          int    `json:"retentionPreferenceDays"`
	StepUpReference                  string `json:"stepUpReference"`
}

// sessionIDFrom resolves the caller's session ID from the verified access
// token claims -- tenant.Context does not carry it, so it is re-read here
// directly from the claims ResolveTenant already consumed.
func sessionIDFrom(c *gin.Context) string {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		return ""
	}
	return claims.SessionID
}

func (h *Handler) update(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionManage) {
		return
	}
	if h.service == nil {
		respond.Error(c, apperrors.Unavailable("Security settings are not available.", nil))
		return
	}
	var body updateSettingsRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body is invalid.", nil))
		return
	}
	// expectedVersion 0 is a legitimate value here (it means "create"), so
	// If-Match is consulted first but a body value of exactly 0 is honored
	// rather than treated as "absent" the way concurrency.ExpectedVersion's
	// normal >=1 convention does elsewhere in this service.
	version := int64(0)
	if ifMatch := c.GetHeader("If-Match"); ifMatch != "" {
		parsed, err := concurrency.ParseIfMatch(ifMatch)
		if err != nil {
			respond.Error(c, err)
			return
		}
		version = parsed
	} else if body.ExpectedVersion != nil {
		version = *body.ExpectedVersion
	}

	settings, validationErrs, err := h.service.Update(c.Request.Context(), usecases.UpdateInput{
		Actor:           usecases.Actor{OrganizationID: t.OrganizationID, ActorType: "organization_user", ActorID: t.OrganizationUserID},
		IdentityID:      t.ActorID,
		SessionID:       sessionIDFrom(c),
		StepUpReference: body.StepUpReference,
		Settings: models.Settings{
			MFARequiredForSensitiveActions: body.MFARequiredForSensitiveActions, StepUpFreshnessSeconds: body.StepUpFreshnessSeconds,
			SessionRevocationPolicy: body.SessionRevocationPolicy, AllowedProviderAccessPolicy: body.AllowedProviderAccessPolicy,
			SensitiveExportThresholdKobo: body.SensitiveExportThresholdKobo, RestrictedDocumentStepUpRequired: body.RestrictedDocumentStepUpRequired,
			AuditCapturePolicy: body.AuditCapturePolicy, RetentionPreferenceDays: body.RetentionPreferenceDays,
		},
		ExpectedVersion: version, RequestID: t.RequestID, CorrelationID: t.CorrelationID,
	})
	if err != nil {
		respond.Error(c, mapUpdateError(err))
		return
	}
	if len(validationErrs) > 0 {
		fields := make([]apperrors.FieldViolation, 0, len(validationErrs))
		for _, e := range validationErrs {
			fields = append(fields, apperrors.FieldViolation{Field: e.Field, Message: e.Message})
		}
		respond.Error(c, apperrors.Validation("Security settings are invalid.", fields))
		return
	}
	respond.OK(c, newSettingsView(settings))
}

func mapUpdateError(err error) error {
	if errors.Is(err, repositories.ErrVersionMismatch) {
		return apperrors.Conflict("Security settings were modified by someone else, or already exist. Reload and retry.", err)
	}
	return err
}
