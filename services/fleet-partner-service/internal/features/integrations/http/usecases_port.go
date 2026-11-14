package http

import (
	"context"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/integrations/models"
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
