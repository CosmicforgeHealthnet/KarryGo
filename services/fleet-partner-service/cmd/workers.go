package main

import (
	"context"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/app"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
)

// Expirer aliases the maintenance sweep port.
type Expirer = jobs.Expirer

// providerInvitationSweep adapts the provider repository to the sweep port
// (its expiry method is named for the record type it owns).
type providerInvitationSweep struct {
	repo *repositories.Repository
}

func (s providerInvitationSweep) ExpireDue(ctx context.Context, limit int) (int64, error) {
	return s.repo.ExpireDueInvitations(ctx, limit)
}

// runProviderReconciler re-drives incomplete provider provisioning sagas.
// Unknown outcomes are resolved by read-back inside Reconcile.
func runProviderReconciler(ctx context.Context, components *app.Components, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = components.Providers.Reconcile(ctx, 25)
		}
	}
}
