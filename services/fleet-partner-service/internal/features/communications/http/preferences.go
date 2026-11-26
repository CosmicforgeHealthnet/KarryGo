package http

import (
	"context"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/communications/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenantcmd"
	"cosmicforge/logistics/shared/go/apperrors"
)

// PreferenceService is the notification-preference slice of the
// communications use cases.
type PreferenceService interface {
	SavePreference(ctx context.Context, t tenant.Context, in models.PreferenceInput, organizationDefault bool) ([]map[string]any, error)
	ListPreferences(ctx context.Context, t tenant.Context) ([]map[string]any, error)
}

func preferenceRoutes() []Route {
	const f = "NOTIFICATION_PREFERENCES_ENABLED"
	return []Route{
		{"GET", "/notification-preferences", "listNotificationPreferences", "notification_preferences.manage", f, "corporate", false, false},
		{"PATCH", "/notification-preferences", "saveNotificationPreference", "notification_preferences.manage", f, "corporate", true, false},
		{"PATCH", "/notification-preferences/organization", "saveOrganizationNotificationPreference", "notification_preferences.manage_organization", f, "corporate", true, false},
	}
}

func (h *Handler) preferenceOps(c *gin.Context, t tenant.Context, r Route) (any, error, bool) {
	s, ok := h.s.(PreferenceService)
	if !ok {
		for _, x := range preferenceRoutes() {
			if x.OperationID == r.OperationID {
				return nil, apperrors.Unavailable("Notification preferences are unavailable.", nil), true
			}
		}
		return nil, nil, false
	}
	switch r.OperationID {
	case "listNotificationPreferences":
		v, err := s.ListPreferences(c, t)
		return v, err, true
	case "saveNotificationPreference", "saveOrganizationNotificationPreference":
		var in models.PreferenceInput
		if !tenantcmd.Bind(c, &in) {
			return nil, tenantcmd.ErrBound, true
		}
		in.ExpectedVersion = tenantcmd.ExpectedVersion(c, in.ExpectedVersion)
		v, err := s.SavePreference(c, t, in, r.OperationID == "saveOrganizationNotificationPreference")
		return v, err, true
	}
	return nil, nil, false
}
