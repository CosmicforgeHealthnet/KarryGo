package main

import (
	"context"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/app"
	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
)

// startExecutionStartWorker runs the execution-start worker built by the
// shared composition root (app.StartExecutionStartWorker), so this dedicated
// process and the API binary's in-process option use the identical execution
// set -- Dispatch, Hauling and Taxi -- and the identical gates.
//
// It returns a stop function that blocks until the loop has exited (the loop
// itself stops when ctx is cancelled), so the process does not terminate while
// a claimed operation is mid-flight.
func startExecutionStartWorker(ctx context.Context, cfg config.Config,
	pool *pgxpool.Pool, metrics *observability.Metrics) func() {
	var wg sync.WaitGroup
	app.StartExecutionStartWorker(ctx, cfg, pool, metrics, &wg)
	return wg.Wait
}
