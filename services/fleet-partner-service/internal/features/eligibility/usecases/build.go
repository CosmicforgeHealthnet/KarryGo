package usecases

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// Build assembles the eligibility service from configuration.
//
// Kept here rather than duplicated at each composition site because there are
// two of them — the API binary and the marketplace worker — and a gate that is
// wired at one site and forgotten at the other is worse than no gate: the same
// operation would be checked or unchecked depending on which process handled
// it.
//
// The authority client is built only when a base URL and secret are present.
// Absent configuration leaves it nil, which makes every vehicle and provider
// decision UNKNOWN and blocks the operations that need one. That is the
// intended behaviour of an incomplete deployment.
func Build(cfg config.Config, pool *pgxpool.Pool, metrics *observability.Metrics) *Service {
	var vehicles platformclients.VehicleAuthority
	var providers platformclients.ProviderAuthority
	vehicleAuthorities := map[string]platformclients.VehicleAuthority{}
	providerAuthorities := map[string]platformclients.ProviderAuthority{}

	dispatchCfg := cfg.Clients["driver-dispatch-delivery-service"]
	baseURL, secret := dispatchCfg.BaseURL, dispatchCfg.Secret
	if baseURL == "" || len(secret) == 0 {
		// The marketplace surface configures dispatch through its own pair of
		// settings; either is a legitimate way to name the same service.
		baseURL, secret = cfg.DispatchServiceURL, cfg.DispatchServiceSecret
	}
	if baseURL != "" && len(secret) > 0 {
		client := platformclients.NewDispatchVehicleAuthorityClient(platformclients.Options{
			ServiceName: "driver-dispatch-delivery-service",
			BaseURL:     baseURL,
			Secret:      secret,
			Timeout:     cfg.HTTPClientTimeout,
			Metrics:     metrics,
		})
		vehicles = client
		providers = client
		vehicleAuthorities[client.AuthorityService()] = client
		providerAuthorities[client.AuthorityService()] = client
	}
	haulingCfg := cfg.Clients["driver-hauling-service"]
	if cfg.ExecutionCapabilities().Hauling && haulingCfg.Configured() {
		client := platformclients.NewHaulingVehicleAuthorityClient(platformclients.Options{
			ServiceName: "driver-hauling-service", BaseURL: haulingCfg.BaseURL,
			Secret: haulingCfg.Secret, Timeout: cfg.HTTPClientTimeout, Metrics: metrics,
		})
		vehicleAuthorities[client.AuthorityService()] = client
		providerAuthorities[client.AuthorityService()] = client
	}

	return NewService(Options{
		Pool:                pool,
		Decisions:           repositories.NewRepository(pool),
		Vehicles:            vehicles,
		Providers:           providers,
		VehicleAuthorities:  vehicleAuthorities,
		ProviderAuthorities: providerAuthorities,
		Events:              events.NewDomainPublisher(events.NewOutboxRepository(pool)),
		Metrics:             metrics,
	})
}
