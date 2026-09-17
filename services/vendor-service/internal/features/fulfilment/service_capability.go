package fulfilment

import "context"

// ServiceCapability is one logistics authority's execution envelope, read
// from the authority itself rather than assumed here.
//
// It answers only what Vendor Service is allowed to know: whether a
// multi-stop collection is executable, how many collection points the
// authority will accept, and whether it performs a secure handover. It never
// carries a price, a provider or a vehicle.
type ServiceCapability struct {
	// Supported is whether a multi-stop collection is executable today.
	Supported bool

	// MaximumPickups is the authority's stop limit. Zero means the authority
	// stated no limit.
	MaximumPickups int

	SecurePickupHandover   bool
	SecureDeliveryHandover bool

	// Version is the authority's own capability version, kept as candidate
	// evidence so a historical plan explains the capability it was decided
	// under.
	Version string
}

// CapabilityAuthority reads one authority's live capability.
//
// An authority that cannot be read is NOT treated as capable: consolidation
// only opens on a capability that was actually stated.
type CapabilityAuthority interface {
	Capability(ctx context.Context) (ServiceCapability, error)
}

// serviceCapabilities resolves every authority's current envelope.
//
// Deployment configuration supplies the baseline for authorities that expose
// no capability contract. A wired contract always wins, including when it
// refuses: a stale configured "true" must never outlive the authority's own
// answer.
func (p *Planner) serviceCapabilities(ctx context.Context) map[Service]ServiceCapability {
	capabilities := map[Service]ServiceCapability{
		ServiceDispatch: {}, ServiceTaxi: {}, ServiceHauling: {},
	}
	for service, supported := range p.multiPickup {
		capabilities[service] = ServiceCapability{Supported: supported, Version: "configured"}
	}
	for service, authority := range p.capabilityAuthorities {
		if authority == nil {
			capabilities[service] = ServiceCapability{}
			continue
		}
		capability, err := authority.Capability(ctx)
		if err != nil {
			capabilities[service] = ServiceCapability{}
			continue
		}
		capabilities[service] = capability
	}
	return capabilities
}

// multiPickupEvidence flattens the envelopes to the boolean capability map
// stored on a candidate.
func multiPickupEvidence(capabilities map[Service]ServiceCapability) map[Service]bool {
	evidence := map[Service]bool{ServiceDispatch: false, ServiceTaxi: false, ServiceHauling: false}
	for service, capability := range capabilities {
		evidence[service] = capability.Supported
	}
	return evidence
}

// requiresSecureHandover reports that a cluster carries load whose handover
// must be verified at collection. High-value goods are the case Section 27
// names, and consolidating them onto an authority without a secure handover
// would lose the per-vendor accountability a split plan keeps.
func requiresSecureHandover(members []planned) bool {
	for _, member := range members {
		if member.group.Value.HighValue() {
			return true
		}
	}
	return false
}
