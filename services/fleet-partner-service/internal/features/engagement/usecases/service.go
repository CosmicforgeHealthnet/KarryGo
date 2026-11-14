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

// Store is the persistence the engagement workflows need.
// repositories.Repository satisfies it; a nil Querier means the pool.
type Store interface {
	WithinTx(ctx context.Context, f func(pgx.Tx) error) error
	ActiveOrganizations(ctx context.Context) ([]string, error)

	SupportCaseByCreateKey(ctx context.Context, organizationID, key string) (map[string]any, error)
	InsertCreatedSupportCase(ctx context.Context, q database.Querier, id, organizationID string, in models.SupportCreateInput, p models.SupportProjection, key string) error
	GetSupportCase(ctx context.Context, organizationID, caseID string) (map[string]any, error)
	ListSupportCases(ctx context.Context, organizationID string) ([]map[string]any, error)
	LockSupportCase(ctx context.Context, q database.Querier, organizationID, caseID string) (string, int64, error)
	InsertEventSupportCase(ctx context.Context, q database.Querier, id string, in models.SupportEvent) error
	InsertSupportInbox(ctx context.Context, q database.Querier, in models.SupportEvent, result string) error
	ApplySupportEvent(ctx context.Context, q database.Querier, in models.SupportEvent, gap string) error
	SupportCasesNeedingReconcile(ctx context.Context, organizationID string) ([]string, error)
	ReconcileSupportCase(ctx context.Context, organizationID, caseID string, p models.SupportProjection) error

	InsertQualityCase(ctx context.Context, q database.Querier, id, organizationID, actorID string, in models.QualityInput) error
	GetQualityCase(ctx context.Context, organizationID, id string) (map[string]any, error)
	ListQualityCases(ctx context.Context, organizationID string, branchIDs []string) ([]map[string]any, error)
	LockQualityCase(ctx context.Context, q database.Querier, organizationID, id string) (string, int64, error)
	UpdateQualityCase(ctx context.Context, q database.Querier, organizationID, id string, in models.QualityInput, severity, next, actorID string) (int64, error)
	InsertQualityHistory(ctx context.Context, q database.Querier, organizationID, id, previous, next string, in models.QualityInput, actorID string, version int64) error
	OverdueSevereQualityCases(ctx context.Context, organizationID string) ([]string, error)

	InsertMeasurement(ctx context.Context, q database.Querier, id string, in models.MeasurementInput, measured any, status string) (string, bool, error)
	GetMeasurement(ctx context.Context, organizationID, id string) (map[string]any, error)
	ListMeasurements(ctx context.Context, organizationID string) ([]map[string]any, error)
	LatestSLAObservations(ctx context.Context, organizationID string) ([]models.SLAObservation, error)

	InsertEscalation(ctx context.Context, q database.Querier, id, organizationID string, in models.EscalationInput) (string, bool, error)
	InsertBreachEscalation(ctx context.Context, q database.Querier, id, organizationID, measurementID string) error
	GetEscalation(ctx context.Context, organizationID, id string) (map[string]any, error)
	ListEscalations(ctx context.Context, organizationID string) ([]map[string]any, error)
	LockEscalation(ctx context.Context, q database.Querier, organizationID, id string) (string, int64, error)
	UpdateEscalationStatus(ctx context.Context, q database.Querier, organizationID, id, next string) (int64, error)
	InsertEscalationHistory(ctx context.Context, q database.Querier, organizationID, id, previous, next, actorID string, version int64) error
	DueEscalations(ctx context.Context, organizationID string) ([]models.DueEscalation, error)
	MarkEscalationNotified(ctx context.Context, q database.Querier, organizationID, id, intentID string) error
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
