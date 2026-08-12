package main

import (
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/logging"

	"cosmicforge/logistics/services/fleet-partner-service/internal/app"
	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	backgroundopshttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/backgroundops/http"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/scheduler"
)

// buildWorkerOperationsInternalRoutes mounts the Section 27 worker_operations
// surface: service-global (organization_id IS NULL) job and scheduled-task
// operations, reachable only by an HMAC-authenticated internal caller on the
// configured allowlist -- the "existing internal/platform-operator
// authorization mechanism" the master specification requires, since no
// tenant-independent human-operator IAM exists anywhere in this codebase.
//
// Returns nil when no caller is allowlisted. A nil registrar leaves the
// route absent entirely rather than present-and-failing, matching every
// other internal route builder in this file.
func buildWorkerOperationsInternalRoutes(cfg config.Config, pool *pgxpool.Pool) app.RouteRegistrar {
	callers := parseAllowedCallers(cfg.WorkerOperationsAllowedCallers)
	if len(callers) == 0 {
		logging.Notice(config.ServiceName,
			"worker-operations internal routes not mounted: WORKER_OPERATIONS_ALLOWED_CALLERS is empty")
		return nil
	}
	handler := backgroundopshttp.NewInternalHandler(jobs.NewQueue(pool), scheduler.NewRepository(pool), events.NewOutboxRepository(pool), callers)
	return func(group *gin.RouterGroup) {
		backgroundopshttp.RegisterInternalRoutes(group, handler)
		logging.Success(config.ServiceName, "worker-operations internal routes mounted")
	}
}
