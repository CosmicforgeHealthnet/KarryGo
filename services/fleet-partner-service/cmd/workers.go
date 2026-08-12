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
// Delegates to the shared internal/app implementation so both this binary
// and the dedicated worker binary run the identical loop.
func runProviderReconciler(ctx context.Context, components *app.Components, interval time.Duration) {
	app.RunProviderReconciler(ctx, components, interval)
}
