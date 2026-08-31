package jobs

import (
	"context"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
)

// ShopCommerceRecoveryJob is the durable at-least-once sweep for expired
// substitution cases. The mutation itself is idempotent and OCC guarded in
// Orders; this adapter only schedules it through the existing worker runner.
func ShopCommerceRecoveryJob(name string, interval time.Duration, store *orders.Store, services ...*orders.Service) Job {
	return Job{Name: name, Interval: interval, Pass: func(ctx context.Context) error {
		if store == nil {
			return nil
		}
		now := time.Now().UTC()
		if _, err := store.ExpireSubstitutionCases(ctx, now, 500); err != nil {
			return err
		}
		if len(services) > 0 && services[0] != nil {
			if _, err := services[0].RecoverApprovedSubstitutionInventory(ctx, 100); err != nil {
				return err
			}
		}
		_, err := store.ExpireSubstitutionProposals(ctx, now, 500)
		return err
	}}
}
