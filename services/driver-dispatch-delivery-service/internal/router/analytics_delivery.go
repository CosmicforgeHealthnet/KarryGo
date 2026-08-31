package router

import (
	"context"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/config"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/marketplace"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/routeintegrity"
	"github.com/jackc/pgx/v5/pgxpool"
)

func startAnalyticsOutboxWorker(ctx context.Context, cfg config.Config, db *pgxpool.Pool) func() {
	if !cfg.AnalyticsEventDeliveryEnabled {
		return func() {}
	}
	transport, ready := marketplace.NewHTTPEventDeliverer(marketplace.DelivererConfig{BaseURL: cfg.AnalyticsURL, Path: "/internal/analytics/v1/commerce-events", ServiceName: cfg.ServiceName, Secret: string(cfg.AnalyticsSecret), Timeout: cfg.ExternalServiceTimeout})
	if !ready {
		return func() {}
	}
	w := marketplace.NewOutboxWorker(marketplace.NewRepository(db), routeintegrity.AnalyticsDeliverer{Transport: transport}, marketplace.OutboxWorkerConfig{Owner: cfg.ServiceName + "-analytics", SourceService: cfg.ServiceName, Target: "analytics-service", Lease: cfg.EventDeliveryLease, BatchSize: cfg.EventDeliveryBatchSize, MaxAttempts: cfg.EventDeliveryRetryLimit, BaseBackoff: cfg.EventDeliveryBackoffBase, MaxBackoff: cfg.EventDeliveryBackoffMax})
	run, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { defer close(done); _ = w.Run(run, cfg.EventDeliveryInterval) }()
	return func() { cancel(); <-done }
}
