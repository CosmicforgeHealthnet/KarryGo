// Package usecases implements the Section 23 engagement workflows: support
// case projections, internal quality cases, SLA measurements and escalations.
// Delivery and platform support lifecycles remain owned by their authoritative
// services; escalation notifications are queued through the communications
// feature's notification intents (NotificationIntents port).
package usecases

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

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

// SupportPort is support-dispute-service's case contract.
type SupportPort interface {
	Create(context.Context, string, string, models.SupportCreateInput) (models.SupportProjection, error)
	Read(context.Context, string, string) (models.SupportProjection, error)
	Configured() bool
}

// EscalationNotice is the notification raised for a due escalation.
type EscalationNotice struct {
	OrganizationID     string
	EscalationID       string
	RecipientType      string
	RecipientReference string
	Channels           json.RawMessage
	SafePayload        json.RawMessage
	IdempotencyKey     string
}

// NotificationIntents queues a durable notification intent in the caller's
// transaction and returns the intent ID. The communications feature owns the
// intents and their delivery; the composition root adapts it.
type NotificationIntents interface {
	QueueEscalationNotice(ctx context.Context, tx pgx.Tx, notice EscalationNotice) (string, error)
}

// Options configures the service.
type Options struct {
	Store         Store
	Events        Events
	Audit         AuditRecorder
	Queue         Enqueuer
	Support       SupportPort
	Notifications NotificationIntents
	Flags         models.Flags
	Now           func() time.Time
}

// Service implements the engagement workflows.
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
