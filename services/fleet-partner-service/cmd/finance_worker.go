package main

import (
	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	financemodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/finance/models"
)

func financeWorkerConfig(cfg config.Config) financemodels.WorkerConfig {
	return financemodels.WorkerConfig{Owner: config.ServiceName + "-finance", BatchSize: cfg.FinanceWorkerBatchSize, Interval: cfg.FinanceWorkerInterval, Lease: cfg.FinanceWorkerLease, MaxAttempts: cfg.FinanceWorkerMaxAttempts, BaseBackoff: cfg.FinanceWorkerBackoff, ProjectionStaleAfter: cfg.FinanceProjectionStaleAfter}
}
