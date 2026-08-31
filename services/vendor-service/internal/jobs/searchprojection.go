package jobs

import (
	"context"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/searchprojection"
)

// SearchProjectionWorkerJob adapts Section 13H's persistent
// lease/retry worker to vendor-service's existing Job runner.
//
// It deliberately does not introduce another scheduler,
// goroutine framework or task-queue dependency.
func SearchProjectionWorkerJob(
	name string,
	interval time.Duration,
	worker *searchprojection.ProjectionWorker,
) Job {
	return Job{
		Name: name,

		Interval: interval,

		Pass: func(
			ctx context.Context,
		) error {
			if worker == nil {
				return nil
			}

			return worker.Pass(ctx)
		},
	}
}
