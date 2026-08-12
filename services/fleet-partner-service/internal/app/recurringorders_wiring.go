package app

import (
	"context"
	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	ordersmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/models"
	ordersusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/usecases"
	recurringrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/recurringorders/repositories"
	recurringusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/recurringorders/usecases"
	searchusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/search/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type recurringOrdersAdapter struct{ orders *ordersusecases.Service }

func (a recurringOrdersAdapter) CreateRecurring(ctx context.Context, t tenant.Context, in recurringusecases.OrderInput) (string, error) {
	start := in.ScheduleStart
	var end *time.Time
	if in.ScheduleStart.IsZero() {
		start = time.Now().UTC()
	}
	order, e := a.orders.Create(ctx, t, ordersusecases.CreateInput{BranchID: in.BranchID, CustomerID: in.CustomerID, CustomerReference: in.CustomerReference, ServiceCatalogItemID: in.ServiceCatalogItemID, ServiceFields: ordersmodels.JSONMap(in.ServiceFields), ContractID: in.ContractID, RequestedScheduleStart: &start, RequestedScheduleEnd: end, Timezone: in.Timezone, Priority: in.Priority, SpecialInstructions: in.SpecialInstructions, Source: ordersmodels.SourceRecurringTemplate, SourceReference: in.SourceReference})
	return order.ID, e
}
func BuildRecurringOrdersService(cfg config.Config, pool *pgxpool.Pool, publisher *events.DomainPublisher,
	search *searchusecases.Service) *recurringusecases.Service {
	orders := BuildOrdersService(cfg, pool, nil, publisher, search)
	return recurringusecases.NewService(recurringusecases.Options{Pool: pool, Repository: recurringrepos.NewRepository(pool), Orders: recurringOrdersAdapter{orders}, Events: publisher, Flags: recurringusecases.Flags{Enabled: cfg.RecurringOrdersEnabled, WritesEnabled: cfg.RecurringOrdersWritesEnabled, WorkerEnabled: cfg.RecurringOrdersWorkerEnabled, KillSwitch: cfg.RecurringOrdersKillSwitch || cfg.IntegrationsKillSwitch}})
}
