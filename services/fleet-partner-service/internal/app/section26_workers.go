package app

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/synchronization"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
)

var synchronizationEvents = []string{"platform.request.offered.v1", "platform.request.cancelled.v1", "provider.availability.changed.v1", "provider.verification.changed.v1", "trip.created.v1", "trip.status_changed.v1", "trip.completed.v1", "vehicle.verification.changed.v1", "wallet.entry.posted.v1", "settlement.created.v1", "settlement.completed.v1", "support.case.changed.v1", "media.asset.status_changed.v1"}

func StartSection26Workers(ctx context.Context, cfg config.Config, components *Components, redisClient *redis.Client, metrics *observability.Metrics, workers *sync.WaitGroup) {
	if components == nil || components.Synchronization == nil || workers == nil || !cfg.SynchronizationEnabled || cfg.SynchronizationKillSwitch || cfg.IntegrationsKillSwitch {
		return
	}
	if redisClient != nil {
		consumer := events.NewStreamConsumer(redisClient, components.Pool, config.ServiceName+"-section26", cfg.OutboxStreamKey, events.DefaultConsumerGroup, metrics).
			WithTransactionPool(components.Pool).WithEnvironment(cfg.AppEnv)
		for _, eventType := range synchronizationEvents {
			eventType := eventType
			consumer.HandleTransactional(eventType, func(runCtx context.Context, tx pgx.Tx, envelope events.Envelope) error {
				switch envelope.EventType {
				case "platform.request.offered.v1", "platform.request.cancelled.v1", "trip.created.v1", "trip.status_changed.v1", "trip.completed.v1":
					return components.Marketplace.ApplyStreamEventTx(runCtx, tx, envelope)
				case "media.asset.status_changed.v1":
					return components.Documents.ApplyMediaStreamEventTx(runCtx, tx, envelope)
				case "support.case.changed.v1":
					if err := components.Synchronization.ApplyTx(runCtx, tx, envelope); err != nil {
						return err
					}
					var payload events.ContractPayload
					if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
						return err
					}
					caseID := payload.CaseID
					if caseID == "" {
						caseID = payload.SubjectReference
					}
					return components.Engagement.ApplySupportEventTx(runCtx, tx, engagement.SupportEvent{
						EventID: envelope.EventID, OrganizationID: envelope.OrganizationID,
						CaseID: caseID, EventType: envelope.EventType, Status: payload.Status,
						Priority: payload.Priority, OwnerReference: payload.OwnerReference,
						SourceVersion: payload.SourceVersion, SafePayload: envelope.Payload,
					})
				default:
					return components.Synchronization.ApplyTx(runCtx, tx, envelope)
				}
			})
		}
		workers.Add(1)
		go func() { defer workers.Done(); consumer.Run(ctx, 2*time.Second) }()
	}
	if cfg.SynchronizationWorkerEnabled {
		workers.Add(1)
		go func() {
			defer workers.Done()
			ticker := time.NewTicker(cfg.JobPollInterval)
			defer ticker.Stop()
			for {
				_, _ = components.Synchronization.RunReconciliationPass(ctx, synchronization.WorkerConfig{Owner: config.ServiceName + "-synchronization", BatchSize: 20, MaxAttempts: 8, Lease: cfg.JobLeaseDuration})
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}()
	}
}
