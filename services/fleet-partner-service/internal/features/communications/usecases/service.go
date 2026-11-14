// Package usecases implements the Section 23 communication workflows:
// threads, messages, notification rules and the delivery of durable
// notification intents to notification-service.
package usecases

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/communications/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
	"cosmicforge/logistics/shared/go/apperrors"
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

// Events publishes domain events inside the caller's transaction.
type Events interface {
	Publish(context.Context, pgx.Tx, string, string, string, int64, string, string, any) error
}

// Enqueuer schedules durable jobs inside the caller's transaction.
type Enqueuer interface {
	Enqueue(ctx context.Context, q database.Querier, request jobs.EnqueueRequest) (string, error)
}

// AuditRecorder writes audit records inside the caller's transaction.
type AuditRecorder interface {
	Record(ctx context.Context, q database.Querier, entry audit.Entry) error
}

// NotificationPort is notification-service's delivery contract.
type NotificationPort interface {
	Submit(context.Context, string, string, json.RawMessage) (models.DeliveryOutcome, error)
	Read(context.Context, string, string) (models.DeliveryOutcome, error)
	Configured() bool
}

// Options configures the service.
type Options struct {
	Store        Store
	Events       Events
	Audit        AuditRecorder
	Queue        Enqueuer
	Media        platformclients.MediaContract
	Notification NotificationPort
	Flags        models.Flags
	Now          func() time.Time
}

// Service implements the communication workflows.
type Service struct{ o Options }

// NewService builds the service.
func NewService(o Options) *Service {
	if o.Audit == nil {
		o.Audit = audit.NewRecorder()
	}
	if o.Queue == nil {
		o.Queue = jobs.NewQueue(nil)
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	return &Service{o: o}
}

func (s *Service) gate(on, write bool) error {
	if s.o.Flags.KillSwitch {
		return apperrors.Unavailable("Fleet integrations are disabled.", nil)
	}
	if !on {
		if write {
			return apperrors.Forbidden("This engagement capability is disabled.", nil)
		}
		return apperrors.NotFound("Resource not found.", nil)
	}
	return nil
}

func (s *Service) record(ctx context.Context, tx pgx.Tx, t tenant.Context, event, kind, id string, v int64, p any) error {
	if e := s.o.Audit.Record(ctx, tx, audit.EntryFromTenant(t, strings.TrimSuffix(event, ".v1"), kind, id)); e != nil {
		return e
	}
	if s.o.Events != nil {
		return s.o.Events.Publish(ctx, tx, event, kind, id, v, t.OrganizationID, t.CorrelationID, p)
	}
	return nil
}

func expectedRequired(v int64) error {
	if v <= 0 {
		return apperrors.Conflict("expectedVersion is required.", nil)
	}
	return nil
}

func branchAllowed(t tenant.Context, b string) bool { return b == "" || t.BranchPermitted(b) }

func (s *Service) validateMedia(ctx context.Context, t tenant.Context, fileIDs []string) error {
	if len(fileIDs) == 0 {
		return nil
	}
	if s.o.Media == nil || !s.o.Media.Configured() {
		return apperrors.Unavailable("Media validation is unavailable.", nil)
	}
	for _, id := range fileIDs {
		m, e := s.o.Media.GetFile(ctx, platformclients.CallMeta{OrganizationID: t.OrganizationID, CorrelationID: t.CorrelationID}, id)
		if e != nil {
			return apperrors.Unavailable("Media validation failed.", e)
		}
		if !m.Ready() || m.OwnerID != t.OrganizationID {
			return apperrors.Forbidden("Media is not owned and safe for this organization.", nil)
		}
	}
	return nil
}
