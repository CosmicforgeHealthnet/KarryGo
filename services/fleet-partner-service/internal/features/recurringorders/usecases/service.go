// Package usecases implements recurring-template commands and generation.
package usecases

import (
	"context"
	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/recurringorders/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/recurringorders/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
	"time"
)

type OrderInput struct {
	BranchID, CustomerID, CustomerReference, ServiceCatalogItemID, ContractID, Timezone, Priority, SpecialInstructions, SourceReference string
	ServiceFields                                                                                                                       map[string]any
	ScheduleStart                                                                                                                       time.Time
}
type OrdersPort interface {
	CreateRecurring(context.Context, tenant.Context, OrderInput) (string, error)
}
type EventPublisher interface {
	Publish(context.Context, pgx.Tx, string, string, string, int64, string, string, any) error
}
type Flags struct{ Enabled, WritesEnabled, WorkerEnabled, KillSwitch bool }
type Options struct {
	Pool       *pgxpool.Pool
	Repository *repositories.Repository
	Orders     OrdersPort
	Events     EventPublisher
	Audit      *audit.Recorder
	Flags      Flags
	Now        func() time.Time
}
type Service struct{ opts Options }

func NewService(o Options) *Service {
	if o.Audit == nil {
		o.Audit = audit.NewRecorder()
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	return &Service{o}
}
func (s *Service) gate(w bool) error {
	if s.opts.Flags.KillSwitch {
		return errorcodes.Unavailable("recurring_orders_disabled", "Recurring orders are unavailable.")
	}
	if !s.opts.Flags.Enabled {
		return errorcodes.NotFound("recurring_order_not_found", "Recurring order not found.")
	}
	if w && !s.opts.Flags.WritesEnabled {
		return errorcodes.Forbidden("recurring_order_invalid_schedule", "Recurring-order writes are disabled.")
	}
	return nil
}
func (s *Service) tx(ctx context.Context, fn func(pgx.Tx) error) error {
	return database.WithinTx(ctx, s.opts.Pool, fn)
}

type CreateInput struct {
	BranchID, CustomerID, ServiceCatalogItemID, ContractID, RecurrenceType, RRule, Timezone, MissedRunPolicy, GeneratedOrderInitialState string
	StartDate                                                                                                                            time.Time
	EndDate                                                                                                                              *time.Time
	Data                                                                                                                                 models.TemplateData
}

func (s *Service) Create(ctx context.Context, t tenant.Context, in CreateInput) (models.Template, error) {
	if e := s.gate(true); e != nil {
		return models.Template{}, e
	}
	if in.BranchID == "" || !t.BranchPermitted(in.BranchID) || in.CustomerID == "" || in.ServiceCatalogItemID == "" {
		return models.Template{}, apperrors.BadRequest("branchId, customerId and serviceCatalogItemId are required.", nil)
	}
	loc, e := time.LoadLocation(in.Timezone)
	if e != nil {
		return models.Template{}, errorcodes.UnprocessableEntity("recurring_order_invalid_schedule", "timezone must be a valid IANA timezone.")
	}
	freq, interval, rule, e := models.ParseRule(in.RecurrenceType, in.RRule)
	if e != nil {
		return models.Template{}, errorcodes.UnprocessableEntity("recurring_order_invalid_schedule", e.Error())
	}
	start := in.StartDate.In(loc)
	if in.StartDate.IsZero() {
		start = s.opts.Now().In(loc)
	}
	next := time.Date(start.Year(), start.Month(), start.Day(), start.Hour(), start.Minute(), 0, 0, loc).UTC()
	_ = freq
	_ = interval
	missed := in.MissedRunPolicy
	if missed == "" {
		missed = "GENERATE_NEXT"
	}
	initial := in.GeneratedOrderInitialState
	if initial == "" {
		initial = "DRAFT"
	}
	out := models.Template{ID: ids.New(repositories.PrefixTemplate), OrganizationID: t.OrganizationID, BranchID: in.BranchID, CustomerID: in.CustomerID, ServiceCatalogItemID: in.ServiceCatalogItemID, ContractID: in.ContractID, Data: in.Data, RecurrenceType: strings.ToUpper(in.RecurrenceType), NormalizedRRule: rule, Timezone: in.Timezone, StartDate: start, EndDate: in.EndDate, NextRunAt: next, MissedRunPolicy: missed, GeneratedOrderInitialState: initial, Status: models.StatusDraft, CreatedBy: t.ActorID}
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		out, x = s.opts.Repository.Create(ctx, tx, out)
		if x != nil {
			return x
		}
		if x = s.opts.Audit.Record(ctx, tx, audit.EntryFromTenant(t, "recurring_orders.created", "recurring_order", out.ID)); x != nil {
			return x
		}
		if s.opts.Events != nil {
			return s.opts.Events.Publish(ctx, tx, "recurring_order.created.v1", "recurring_order", out.ID, out.Version, t.OrganizationID, t.CorrelationID, map[string]any{"templateId": out.ID})
		}
		return nil
	})
	return out, e
}
func (s *Service) Get(ctx context.Context, t tenant.Context, id string) (models.Template, error) {
	if e := s.gate(false); e != nil {
		return models.Template{}, e
	}
	v, e := s.opts.Repository.Get(ctx, nil, t.OrganizationID, id)
	if errors.Is(e, repositories.ErrNotFound) || e == nil && !t.BranchPermitted(v.BranchID) {
		return models.Template{}, errorcodes.NotFound("recurring_order_not_found", "Recurring order not found.")
	}
	return v, e
}
func (s *Service) List(ctx context.Context, t tenant.Context, status string) ([]models.Template, error) {
	if e := s.gate(false); e != nil {
		return nil, e
	}
	return s.opts.Repository.List(ctx, t.OrganizationID, status)
}
func (s *Service) SetActive(ctx context.Context, t tenant.Context, id string, version int64, active bool) (models.Template, error) {
	if e := s.gate(true); e != nil {
		return models.Template{}, e
	}
	if version <= 0 {
		return models.Template{}, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	status := models.StatusInactive
	event := "recurring_order.deactivated.v1"
	if active {
		status = models.StatusActive
		event = "recurring_order.activated.v1"
	}
	var out models.Template
	e := s.tx(ctx, func(tx pgx.Tx) error {
		cur, x := s.opts.Repository.Get(ctx, tx, t.OrganizationID, id)
		if x != nil || !t.BranchPermitted(cur.BranchID) {
			return errorcodes.NotFound("recurring_order_not_found", "Recurring order not found.")
		}
		if active && cur.Status != models.StatusDraft && cur.Status != models.StatusInactive || !active && cur.Status != models.StatusActive {
			return errorcodes.Conflict("recurring_order_invalid_schedule", "Template state does not allow this command.")
		}
		out, x = s.opts.Repository.SetStatus(ctx, tx, t.OrganizationID, id, version, status, t.ActorID)
		if x != nil {
			return x
		}
		if x = s.opts.Audit.Record(ctx, tx, audit.EntryFromTenant(t, "recurring_orders.status_changed", "recurring_order", id)); x != nil {
			return x
		}
		if s.opts.Events != nil {
			return s.opts.Events.Publish(ctx, tx, event, "recurring_order", id, out.Version, t.OrganizationID, t.CorrelationID, map[string]any{"templateId": id, "status": status})
		}
		return nil
	})
	return out, e
}

func (s *Service) Replay(ctx context.Context, t tenant.Context, id string) error {
	if e := s.gate(true); e != nil {
		return e
	}
	tpl, err := s.opts.Repository.Get(ctx, nil, t.OrganizationID, id)
	if err != nil || !t.BranchPermitted(tpl.BranchID) {
		return errorcodes.NotFound("recurring_order_not_found", "Recurring order not found.")
	}
	if err = s.opts.Repository.Replay(ctx, t.OrganizationID, id); err != nil {
		return errorcodes.Conflict("recurring_occurrence_conflict", "Only a dead-lettered occurrence may be replayed.")
	}
	return nil
}
