package shareworkers

import (
	"context"
	u "cosmicforge/logistics/services/customer-service/internal/features/journey_sharing/usecases"
	"cosmicforge/logistics/shared/go/logging"
	"time"
)

func Expiry(service *u.Service, enabled bool) func(context.Context) error {
	return func(ctx context.Context) error {
		if !enabled {
			return nil
		}
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			err := service.Sweep(ctx)
			if err != nil && ctx.Err() == nil {
				logging.Structured("error", "journey_share_expiry", map[string]any{"outcome": "dependency_error"})
			}
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
			}
		}
	}
}
