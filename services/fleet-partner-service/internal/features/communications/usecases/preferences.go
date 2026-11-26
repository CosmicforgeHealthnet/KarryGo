package usecases

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/communications/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

// PreferenceStore is the persistence of notification preferences.
// repositories.Repository satisfies it.
type PreferenceStore interface {
	UpsertPreference(ctx context.Context, q database.Querier, id, organizationID, subjectType, subjectID, eventType string, channels, quiet []byte, enabled bool, expected int64) (int64, error)
	ListPreferences(ctx context.Context, organizationID, userID string) ([]map[string]any, error)
	PreferenceFor(ctx context.Context, q database.Querier, organizationID, userID, eventType string) ([]byte, bool, bool, error)
}

func (s *Service) preferences() (PreferenceStore, bool) {
	p, ok := s.o.Store.(PreferenceStore)
	return p, ok
}

// SavePreference sets the caller's own preference for one event type, or
// the organization default when organizationDefault is true (that route
// requires the organization-preference permission). A new preference is
// created with expectedVersion 0; an existing one needs its current version.
func (s *Service) SavePreference(ctx context.Context, t tenant.Context, in models.PreferenceInput, organizationDefault bool) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.Preferences, true); err != nil {
		return nil, err
	}
	store, ok := s.preferences()
	if !ok {
		return nil, apperrors.Unavailable("Notification preferences are unavailable.", nil)
	}
	in.EventType = strings.TrimSpace(in.EventType)
	if in.EventType == "" || len(in.EventType) > 120 {
		return nil, apperrors.Validation("eventType is required.", nil)
	}
	for _, c := range in.Channels {
		if !models.ValidChannel(c) {
			return nil, apperrors.Validation("Unsupported channel: "+c, nil)
		}
	}
	if err := models.ValidateQuietHours(in.QuietHours); err != nil {
		return nil, err
	}
	subjectType, subjectID := "USER", t.OrganizationUserID
	if organizationDefault {
		subjectType, subjectID = "ORGANIZATION", t.OrganizationID
	}
	if subjectID == "" {
		return nil, apperrors.Forbidden("An organization user is required.", nil)
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	channels := make([]string, 0, len(in.Channels))
	for _, c := range in.Channels {
		channels = append(channels, strings.ToUpper(c))
	}
	rawChannels, _ := json.Marshal(channels)
	quiet := models.RawOr(in.QuietHours, "{}")
	err := s.o.Store.WithinTx(ctx, func(tx pgx.Tx) error {
		v, err := store.UpsertPreference(ctx, tx, ids.New("npf"), t.OrganizationID, subjectType, subjectID, in.EventType, rawChannels, quiet, enabled, in.ExpectedVersion)
		if err != nil {
			return err
		}
		if v == 0 {
			return apperrors.New(http.StatusConflict, "version_conflict", "Notification preference version changed.", nil)
		}
		return s.record(ctx, tx, t, "notification_preference.updated.v1", "notification_preference", subjectType+":"+subjectID+":"+in.EventType, v, map[string]any{"subjectType": subjectType, "eventType": in.EventType, "enabled": enabled, "channels": channels})
	})
	if err != nil {
		return nil, err
	}
	return store.ListPreferences(ctx, t.OrganizationID, t.OrganizationUserID)
}

// ListPreferences lists the organization defaults and the caller's own
// preferences; another user's preferences are never visible.
func (s *Service) ListPreferences(ctx context.Context, t tenant.Context) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.Preferences, false); err != nil {
		return nil, err
	}
	store, ok := s.preferences()
	if !ok {
		return nil, apperrors.Unavailable("Notification preferences are unavailable.", nil)
	}
	return store.ListPreferences(ctx, t.OrganizationID, t.OrganizationUserID)
}

// applyPreference narrows an intent's channels to the recipient's
// preference when the recipient is an organization user.
func (s *Service) applyPreference(ctx context.Context, tx pgx.Tx, in *models.IntentInput) error {
	if !s.o.Flags.Preferences || in.RecipientType != "ORGANIZATION_USER" {
		return nil
	}
	store, ok := s.preferences()
	if !ok {
		return nil
	}
	raw, enabled, found, err := store.PreferenceFor(ctx, tx, in.OrganizationID, in.RecipientReference, in.TemplateCode)
	if err != nil || !found {
		return err
	}
	var requested, allowed []string
	_ = json.Unmarshal(in.Channels, &requested)
	_ = json.Unmarshal(raw, &allowed)
	narrowed, _ := json.Marshal(models.NarrowChannels(requested, &models.Preference{Channels: allowed, Enabled: enabled}))
	in.Channels = narrowed
	return nil
}
