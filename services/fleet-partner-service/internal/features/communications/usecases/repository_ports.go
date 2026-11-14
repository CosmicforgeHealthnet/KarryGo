package usecases

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/communications/models"
)

// Store is the persistence the communication workflows need.
// repositories.Repository satisfies it; a nil Querier means the pool.
type Store interface {
	WithinTx(ctx context.Context, f func(pgx.Tx) error) error

	InsertThread(ctx context.Context, q database.Querier, id, organizationID, actorID string, in models.ThreadInput) error
	InsertParticipant(ctx context.Context, q database.Querier, threadID, organizationID, actorID string, p models.ParticipantInput) error
	IsUserParticipant(ctx context.Context, organizationID, threadID, userID string) (bool, error)
	GetThread(ctx context.Context, organizationID, id string) (map[string]any, error)
	ListThreadsForUser(ctx context.Context, organizationID, userID string, branchIDs []string) ([]map[string]any, error)
	LockThread(ctx context.Context, q database.Querier, organizationID, id string) (string, int64, error)
	UpdateThread(ctx context.Context, q database.Querier, organizationID, id, title, ownerUserID, status string) (int64, error)
	CloseThread(ctx context.Context, q database.Querier, organizationID, id string) (int64, error)
	InsertThreadHistory(ctx context.Context, q database.Querier, organizationID, threadID, previousStatus, newStatus string, snapshot []byte, actorID string, version int64) error
	LockParticipantThread(ctx context.Context, q database.Querier, organizationID, threadID, userID string) (string, string, error)
	ExternalRecipients(ctx context.Context, q database.Querier, organizationID, threadID string) ([]models.Recipient, error)
	MarkThreadPendingResponse(ctx context.Context, q database.Querier, organizationID, threadID string) error

	MessageByIdempotencyKey(ctx context.Context, organizationID, key string) (map[string]any, error)
	GetMessage(ctx context.Context, organizationID, id string) (map[string]any, error)
	InsertMessage(ctx context.Context, q database.Querier, id, organizationID, threadID, actorID string, in models.MessageInput, media []byte, deliveryStatus, key string) error
	ListMessages(ctx context.Context, organizationID, threadID string) ([]map[string]any, error)

	LockRule(ctx context.Context, q database.Querier, organizationID, eventType, recipientScope string) (string, int64, error)
	UpdateRule(ctx context.Context, q database.Querier, organizationID, id string, channels, quiet json.RawMessage, timezone, severity string, escalation json.RawMessage, enabled bool) error
	InsertRule(ctx context.Context, q database.Querier, id, organizationID, eventType, recipientScope string, channels, quiet json.RawMessage, timezone, severity string, escalation json.RawMessage, enabled bool) error
	GetRule(ctx context.Context, organizationID, id string) (map[string]any, error)
	ListRules(ctx context.Context, organizationID string) ([]map[string]any, error)

	InsertIntent(ctx context.Context, q database.Querier, id string, in models.IntentInput) error
	ActiveOrganizations(ctx context.Context) ([]string, error)
	DueIntents(ctx context.Context, organizationID string) ([]string, error)
	GetIntent(ctx context.Context, organizationID, id string) (models.Intent, error)
	RecordDeliveryAttempt(ctx context.Context, q database.Querier, organizationID, intentID string, attemptNumber int, outcome, reference, code string, started time.Time, status string) error
}
