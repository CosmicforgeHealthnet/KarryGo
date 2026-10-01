package taxi

import (
	"context"

	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilment"
	"cosmicforge/logistics/shared/go/commercecapability"
)

// CapabilityPath is Taxi's internal capability route under its API base.
// CapabilityPath is Taxi's execution envelope (contract v2). It is
// deliberately NOT /internal/commerce/capabilities: Taxi already serves a
// differently shaped capability document there, and two contracts on one
// path either collide at registration or answer the wrong shape.
const CapabilityPath = "/api/v1/taxi/internal/commerce/execution-capability"

// CapabilityAuthority reads Taxi's live execution envelope over the shared
// capability contract and translates it into the planner's own vocabulary.
//
// Nothing here decides what Taxi can do: Taxi states it, and a refusal or an
// unreachable authority leaves the planner with no multi-pickup capability
// rather than a remembered one.
type CapabilityAuthority struct {
	client *commercecapability.Client
}

// NewCapabilityAuthority returns nil when the integration is disabled or
// unconfigured, which the planner reads as "no capability contract wired".
func NewCapabilityAuthority(enabled bool, baseURL, serviceName, secret string) *CapabilityAuthority {
	if !enabled || baseURL == "" || secret == "" {
		return nil
	}
	return &CapabilityAuthority{client: &commercecapability.Client{
		BaseURL: baseURL, Path: CapabilityPath, ServiceName: serviceName, Secret: []byte(secret),
	}}
}

// Capability implements fulfilment.CapabilityAuthority.
func (a *CapabilityAuthority) Capability(ctx context.Context) (fulfilment.ServiceCapability, error) {
	if a == nil || a.client == nil {
		return fulfilment.ServiceCapability{}, commercecapability.ErrUnavailable
	}
	capability, err := a.client.Capability(ctx)
	if err != nil {
		return fulfilment.ServiceCapability{}, err
	}
	return fulfilment.ServiceCapability{
		Supported:              capability.MultiPickupSupported,
		MaximumPickups:         capability.MaximumPickups,
		SecurePickupHandover:   capability.SecurePickupHandover,
		SecureDeliveryHandover: capability.SecureDeliveryHandover,
		Version:                capability.CapabilityVersion,
	}, nil
}
