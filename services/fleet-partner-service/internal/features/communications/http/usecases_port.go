package http

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/communications/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Service is the slice of the communication use cases the handler calls.
type Service interface {
	ListThreads(ctx context.Context, t tenant.Context) ([]map[string]any, error)
	CreateThread(ctx context.Context, t tenant.Context, in models.ThreadInput) (map[string]any, error)
	GetThread(ctx context.Context, t tenant.Context, id string) (map[string]any, error)
	UpdateThread(ctx context.Context, t tenant.Context, id string, in models.ThreadInput) (map[string]any, error)
	CloseThread(ctx context.Context, t tenant.Context, id string, expected int64) (map[string]any, error)
	ListMessages(ctx context.Context, t tenant.Context, threadID string) ([]map[string]any, error)
	SendMessage(ctx context.Context, t tenant.Context, threadID, key string, in models.MessageInput) (map[string]any, error)
	ListNotificationRules(ctx context.Context, t tenant.Context) ([]map[string]any, error)
	SaveNotificationRule(ctx context.Context, t tenant.Context, in models.NotificationRuleInput) (map[string]any, error)
}
