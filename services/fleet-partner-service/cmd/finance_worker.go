package main

import (
	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	financefeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/finance"
)

func financeWorkerConfig(cfg config.Config) financefeature.WorkerConfig {
	return financefeature.WorkerConfig{Owner: config.ServiceName + "-finance", BatchSize: cfg.FinanceWorkerBatchSize, Interval: cfg.FinanceWorkerInterval, Lease: cfg.FinanceWorkerLease, MaxAttempts: cfg.FinanceWorkerMaxAttempts, BaseBackoff: cfg.FinanceWorkerBackoff, ProjectionStaleAfter: cfg.FinanceProjectionStaleAfter}
}
