package router

import (
	"context"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/config"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/marketplace"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/networkobserver"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/routeintegrity"
	"cosmicforge/logistics/shared/go/networkanalytics"
	"github.com/jackc/pgx/v5/pgxpool"
)

func startAnalyticsOutboxWorker(ctx context.Context, cfg config.Config, db *pgxpool.Pool, live networkobserver.Locator) func() {
	if !cfg.AnalyticsEventDeliveryEnabled {
		return func() {}
	}
	transport, ready := marketplace.NewHTTPEventDeliverer(marketplace.DelivererConfig{BaseURL: cfg.AnalyticsURL, Path: "/internal/analytics/v1/commerce-events", ServiceName: cfg.ServiceName, Secret: string(cfg.AnalyticsSecret), Timeout: cfg.ExternalServiceTimeout})
	if !ready {
		return func() {}
	}
	// Section 13: the same credentials reach the network ingest.
	network, _ := marketplace.NewHTTPEventDeliverer(marketplace.DelivererConfig{BaseURL: cfg.AnalyticsURL, Path: networkanalytics.IngestPath, ServiceName: cfg.ServiceName, Secret: string(cfg.AnalyticsSecret), Timeout: cfg.ExternalServiceTimeout})
	w := marketplace.NewOutboxWorker(marketplace.NewRepository(db), routeintegrity.AnalyticsDeliverer{Transport: transport, Network: network}, marketplace.OutboxWorkerConfig{Owner: cfg.ServiceName + "-analytics", SourceService: cfg.ServiceName, Target: "analytics-service", Lease: cfg.EventDeliveryLease, BatchSize: cfg.EventDeliveryBatchSize, MaxAttempts: cfg.EventDeliveryRetryLimit, BaseBackoff: cfg.EventDeliveryBackoffBase, MaxBackoff: cfg.EventDeliveryBackoffMax})
	run, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { defer close(done); _ = w.Run(run, cfg.EventDeliveryInterval) }()
	// Section 13: supply and demand, observed once per bucket from the
	// matcher's own state. Only started alongside delivery -- with nowhere to
	// deliver, the facts would only accumulate.
	observerDone := make(chan struct{})
	go func() { defer close(observerDone); (&networkobserver.Observer{DB: db, Locator: live}).Run(run) }()
	return func() { cancel(); <-done; <-observerDone }
}
