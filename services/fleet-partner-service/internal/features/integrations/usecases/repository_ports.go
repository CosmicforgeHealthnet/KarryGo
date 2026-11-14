package usecases

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/integrations/models"
)

// Store is the persistence the integration workflows need.
// repositories.Repository satisfies it; a nil Querier means the pool.
type Store interface {
	WithinTx(ctx context.Context, f func(pgx.Tx) error) error
	Configured() bool
	ClaimCommand(ctx context.Context, q database.Querier, org, command, key, fingerprint, resourceID string) (string, bool, bool, error)

	GetAPIClient(ctx context.Context, q database.Querier, org, id string) (models.APIClient, error)
	InsertAPIClient(ctx context.Context, q database.Querier, c models.APIClient, secretHash []byte) error
	ListAPIClients(ctx context.Context, org string) ([]models.APIClient, error)
	LockActiveAPIClientSecret(ctx context.Context, q database.Querier, org, id string, expected int64) ([]byte, int, error)
	RotateAPIClientSecret(ctx context.Context, q database.Querier, org, id string, expected int64, secretHash []byte, prevHash, prevVersion, prevUntil any) (bool, error)
	RevokeAPIClient(ctx context.Context, q database.Querier, org, id string, expected int64, revokedHash []byte) (bool, error)
	ClientCredential(ctx context.Context, clientID string) (models.ClientCredential, error)
	TouchAPIClient(ctx context.Context, id string) error

	GetWebhook(ctx context.Context, q database.Querier, org, id string) (models.WebhookSubscription, error)
	InsertWebhook(ctx context.Context, q database.Querier, s models.WebhookSubscription, ciphertext []byte) error
	UpdateWebhook(ctx context.Context, q database.Querier, org, id, endpoint string, eventTypes []string, status string, expected int64) (bool, error)
	LockActiveWebhookSecret(ctx context.Context, q database.Querier, org, id string, expected int64) ([]byte, int, error)
	RotateWebhookSecret(ctx context.Context, q database.Querier, org, id string, expected int64, ciphertext []byte, prevCipher, prevVersion, prevUntil any) (bool, error)
	ListWebhooks(ctx context.Context, org string) ([]models.WebhookSubscription, error)

	GetDelivery(ctx context.Context, q database.Querier, org, id string) (models.WebhookDelivery, error)
	ListDeliveries(ctx context.Context, org, subscriptionID string) ([]models.WebhookDelivery, error)
	LockFailedDelivery(ctx context.Context, q database.Querier, org, id string) (models.WebhookDelivery, error)
	NextAttempt(ctx context.Context, q database.Querier, subscriptionID, eventID string) (int, error)
	InsertReplayDelivery(ctx context.Context, q database.Querier, id, org string, original models.WebhookDelivery, attempt int, replayOf string) error
	SetDeliveryJob(ctx context.Context, q database.Querier, id, jobID string) error
	PendingPublicEvents(ctx context.Context, eventTypes []string, limit int) ([]models.OutboxEvent, error)
	ClaimFanout(ctx context.Context, q database.Querier, eventID, org, eventType string) (bool, error)
	ActiveSubscriptionsFor(ctx context.Context, q database.Querier, org, eventType string) ([]string, error)
	InsertFirstDelivery(ctx context.Context, q database.Querier, id, org, subscriptionID, eventID, eventType, correlationID string) (bool, error)
	DeliveryContext(ctx context.Context, org, id string) (models.DeliveryContext, error)
	MarkDelivering(ctx context.Context, org, id string) error
	MarkDeliverySucceeded(ctx context.Context, q database.Querier, org, id string, status int, excerpt string) (bool, error)
	ResetFailureCount(ctx context.Context, q database.Querier, org, deliveryID string) error
	MarkDeliveryFailed(ctx context.Context, q database.Querier, org, id, status string, responseStatus int, next any, class, code string) error
	RecordSubscriptionFailure(ctx context.Context, q database.Querier, threshold int, org, deliveryID string) (string, int64, error)
	WebhookAggregates(ctx context.Context) (int64, *time.Time, int64, int64, error)
}
