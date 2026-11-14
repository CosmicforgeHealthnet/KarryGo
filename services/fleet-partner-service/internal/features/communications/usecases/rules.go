package usecases

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/communications/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

// SaveNotificationRule creates or (at the expected version) updates the rule
// for one event type and recipient scope.
func (s *Service) SaveNotificationRule(ctx context.Context, t tenant.Context, in models.NotificationRuleInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.NotificationRules, true); err != nil {
		return nil, err
	}
	if in.EventType == "" || in.RecipientScope == "" || in.Timezone == "" {
		return nil, apperrors.Validation("Notification event, recipient scope and timezone are required.", nil)
	}
	if _, err := time.LoadLocation(in.Timezone); err != nil {
		return nil, apperrors.Validation("Timezone must be a valid IANA name.", nil)
	}
	if err := models.ValidateQuietHours(in.QuietHours); err != nil {
		return nil, err
	}
	channels := models.RawOr(in.Channels, "[]")
	quiet := models.RawOr(in.QuietHours, "{}")
	escalation := models.RawOr(in.EscalationPolicy, "{}")
	severity := strings.ToUpper(in.SeverityThreshold)
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	id := ids.New("ntr")
	err := s.o.Store.WithinTx(ctx, func(tx pgx.Tx) error {
		existingID, version, err := s.o.Store.LockRule(ctx, tx, t.OrganizationID, in.EventType, in.RecipientScope)
		if err == nil {
			if in.ExpectedVersion <= 0 || version != in.ExpectedVersion {
				return apperrors.Conflict("Notification rule version changed.", nil)
			}
			id = existingID
			if err := s.o.Store.UpdateRule(ctx, tx, t.OrganizationID, id, channels, quiet, in.Timezone, severity, escalation, enabled); err != nil {
				return err
			}
			version++
			return s.record(ctx, tx, t, "notification_rule.updated.v1", "notification_rule", id, version, map[string]any{"notificationRuleId": id, "eventType": in.EventType, "enabled": enabled})
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err := s.o.Store.InsertRule(ctx, tx, id, t.OrganizationID, in.EventType, in.RecipientScope, channels, quiet, in.Timezone, severity, escalation, enabled); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "notification_rule.created.v1", "notification_rule", id, 1, map[string]any{"notificationRuleId": id, "eventType": in.EventType, "enabled": enabled})
	})
	if err != nil {
		return nil, err
	}
	return s.o.Store.GetRule(ctx, t.OrganizationID, id)
}

// ListNotificationRules lists the organization's rules.
func (s *Service) ListNotificationRules(ctx context.Context, t tenant.Context) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.NotificationRules, false); err != nil {
		return nil, err
	}
	return s.o.Store.ListRules(ctx, t.OrganizationID)
}
