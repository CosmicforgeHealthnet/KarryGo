// Package usecases implements the Section 22 finance-operations workflows:
// invoices and receivables, expenses, budgets, claims, compensation rules,
// payroll, subcontractor settlements and the delivery of postings and managed
// settlement instructions to the money authority. Wallet balances, ledger
// entries and payment execution remain external.
package usecases

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/financeops/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
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

// PostingPort is the money authority's posting contract.
type PostingPort interface {
	Submit(context.Context, platformclients.CallMeta, models.PostingRequest) (models.PostingOutcome, error)
	ReadByKey(context.Context, platformclients.CallMeta, models.PostingRequest) (models.PostingOutcome, error)
	Configured(operationType string) bool
}

// Options configures the service.
type Options struct {
	Store  Store
	Events Events
	Audit  AuditRecorder
	Queue  Enqueuer
	Media  platformclients.MediaContract
	Wallet PostingPort
	Flags  models.Flags
	Now    func() time.Time
}

// Service implements the finance-operations workflows.
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

func (s *Service) gate(enabled, write bool) error {
	if s.o.Flags.KillSwitch {
		return apperrors.Unavailable("Fleet integrations are disabled.", nil)
	}
	if !enabled {
		if write {
			return apperrors.Forbidden("This finance capability is disabled.", nil)
		}
		return apperrors.NotFound("Resource not found.", nil)
	}
	return nil
}

func (s *Service) tx(ctx context.Context, f func(pgx.Tx) error) error {
	return s.o.Store.WithinTx(ctx, f)
}

func (s *Service) record(ctx context.Context, tx pgx.Tx, t tenant.Context, event, kind, id string, version int64, payload any) error {
	if err := s.o.Audit.Record(ctx, tx, audit.EntryFromTenant(t, strings.TrimSuffix(event, ".v1"), kind, id)); err != nil {
		return err
	}
	if s.o.Events != nil {
		return s.o.Events.Publish(ctx, tx, event, kind, id, version, t.OrganizationID, t.CorrelationID, payload)
	}
	return nil
}

func branchAllowed(t tenant.Context, branch string) bool {
	return branch == "" || t.BranchPermitted(branch)
}

func (s *Service) validateMedia(ctx context.Context, t tenant.Context, fileID string) error {
	if strings.TrimSpace(fileID) == "" {
		return nil
	}
	if s.o.Media == nil || !s.o.Media.Configured() {
		return apperrors.Unavailable("Media validation is unavailable.", nil)
	}
	meta, err := s.o.Media.GetFile(ctx, platformclients.CallMeta{OrganizationID: t.OrganizationID, CorrelationID: t.CorrelationID}, fileID)
	if err != nil {
		return apperrors.Unavailable("Media validation failed.", err)
	}
	if !meta.Ready() || meta.OwnerID != t.OrganizationID {
		return apperrors.Forbidden("Media is not owned and safe for this organization.", nil)
	}
	return nil
}

func expectedRequired(v int64) error {
	if v <= 0 {
		return apperrors.Conflict("expectedVersion is required.", nil)
	}
	return nil
}

func statusString(v map[string]any) string { x, _ := v["status"].(string); return x }

func int64Value(v map[string]any, key string) int64 {
	switch x := v[key].(type) {
	case int64:
		return x
	case float64:
		return int64(x)
	case json.Number:
		n, _ := x.Int64()
		return n
	}
	return 0
}

func operationMeta(t tenant.Context, key string) platformclients.CallMeta {
	return platformclients.CallMeta{OrganizationID: t.OrganizationID, CorrelationID: t.CorrelationID, IdempotencyKey: key}
}
