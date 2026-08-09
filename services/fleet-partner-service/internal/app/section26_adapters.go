package app

import (
	"context"
	"encoding/json"
	"errors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/synchronization"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

type providerSyncAuthority struct {
	authorities []platformclients.ProviderAuthority
}

func (a providerSyncAuthority) Read(ctx context.Context, org, ref string) (synchronization.AuthoritativeState, error) {
	var last error
	for _, authority := range a.authorities {
		state, err := authority.LookupProvider(ctx, platformclients.CallMeta{OrganizationID: org}, platformclients.ProviderLookup{ProviderID: ref})
		if err == nil {
			raw, _ := json.Marshal(state)
			return synchronization.AuthoritativeState{SourceVersion: state.SourceVersion, SafeState: raw}, nil
		}
		last = err
	}
	if last == nil {
		last = errors.New("provider authority unavailable")
	}
	return synchronization.AuthoritativeState{}, last
}

type vehicleSyncAuthority struct {
	authorities []platformclients.VehicleAuthority
}

func (a vehicleSyncAuthority) Read(ctx context.Context, org, ref string) (synchronization.AuthoritativeState, error) {
	var last error
	for _, authority := range a.authorities {
		state, err := authority.LookupVehicle(ctx, platformclients.CallMeta{OrganizationID: org}, platformclients.VehicleLookup{VehicleID: ref})
		if err == nil {
			raw, _ := json.Marshal(state)
			return synchronization.AuthoritativeState{SourceVersion: state.SourceVersion, SafeState: raw}, nil
		}
		last = err
	}
	if last == nil {
		last = errors.New("vehicle authority unavailable")
	}
	return synchronization.AuthoritativeState{}, last
}

type walletSyncAuthority struct {
	client         *platformclients.WalletClient
	projectionType string
}

func (a walletSyncAuthority) Read(ctx context.Context, org, ref string) (synchronization.AuthoritativeState, error) {
	state, err := a.client.ReadProjection(ctx, platformclients.CallMeta{OrganizationID: org}, org, a.projectionType, ref)
	if err != nil {
		return synchronization.AuthoritativeState{}, err
	}
	raw, _ := json.Marshal(state)
	return synchronization.AuthoritativeState{SourceVersion: state.Version, SafeState: raw}, nil
}

type supportSyncAuthority struct{ port engagement.SupportPort }

func (a supportSyncAuthority) Read(ctx context.Context, org, ref string) (synchronization.AuthoritativeState, error) {
	state, err := a.port.Read(ctx, org, ref)
	if err != nil {
		return synchronization.AuthoritativeState{}, err
	}
	raw, _ := json.Marshal(state)
	return synchronization.AuthoritativeState{SourceVersion: state.SourceVersion, SafeState: raw}, nil
}

type mediaReadPort interface {
	ReadObject(context.Context, platformclients.CallMeta, string) (platformclients.MediaObjectState, error)
	Configured() bool
}

type mediaSyncAuthority struct{ port mediaReadPort }

func (a mediaSyncAuthority) Read(ctx context.Context, org, ref string) (synchronization.AuthoritativeState, error) {
	state, err := a.port.ReadObject(ctx, platformclients.CallMeta{OrganizationID: org}, ref)
	if err != nil {
		return synchronization.AuthoritativeState{}, err
	}
	raw, _ := json.Marshal(state)
	return synchronization.AuthoritativeState{SourceVersion: state.SourceVersion, SafeState: raw}, nil
}

func registerSection26Authorities(cfg config.Config, metrics *observability.Metrics, service *synchronization.Service, support engagement.SupportPort) {
	if service == nil {
		return
	}
	providers := providerSyncAuthority{}
	vehicles := vehicleSyncAuthority{}
	for _, name := range []string{"driver-dispatch-delivery-service", "driver-hauling-service"} {
		clientCfg := cfg.Clients[name]
		if !clientCfg.Configured() {
			continue
		}
		opts := platformclients.Options{ServiceName: name, BaseURL: clientCfg.BaseURL, Secret: clientCfg.Secret, Timeout: cfg.HTTPClientTimeout, Metrics: metrics}
		if name == "driver-hauling-service" {
			providers.authorities = append(providers.authorities, platformclients.NewHaulingVehicleAuthorityClient(opts))
			vehicles.authorities = append(vehicles.authorities, platformclients.NewHaulingVehicleAuthorityClient(opts))
		} else {
			providers.authorities = append(providers.authorities, platformclients.NewDispatchVehicleAuthorityClient(opts))
			vehicles.authorities = append(vehicles.authorities, platformclients.NewDispatchVehicleAuthorityClient(opts))
		}
	}
	service.RegisterAuthority("provider.availability.changed.v1", providers)
	service.RegisterAuthority("provider.verification.changed.v1", providers)
	service.RegisterAuthority("vehicle.verification.changed.v1", vehicles)
	walletCfg := cfg.Clients["payment-wallet-service"]
	if walletCfg.Configured() {
		wallet := platformclients.NewWalletClient(platformclients.Options{ServiceName: "payment-wallet-service", BaseURL: walletCfg.BaseURL, Secret: walletCfg.Secret, Timeout: cfg.HTTPClientTimeout, Metrics: metrics})
		service.RegisterAuthority("wallet.entry.posted.v1", walletSyncAuthority{wallet, "WALLET_ENTRY"})
		service.RegisterAuthority("settlement.created.v1", walletSyncAuthority{wallet, "SETTLEMENT"})
		service.RegisterAuthority("settlement.completed.v1", walletSyncAuthority{wallet, "SETTLEMENT"})
	}
	if support != nil && support.Configured() {
		service.RegisterAuthority("support.case.changed.v1", supportSyncAuthority{support})
	}
	mediaCfg := cfg.Clients["media-file-service"]
	if mediaCfg.Configured() {
		media := platformclients.NewMediaFileClient(platformclients.Options{ServiceName: "media-file-service", BaseURL: mediaCfg.BaseURL, Secret: mediaCfg.Secret, Timeout: cfg.HTTPClientTimeout, Metrics: metrics})
		service.RegisterAuthority("media.asset.status_changed.v1", mediaSyncAuthority{media})
	}
}
