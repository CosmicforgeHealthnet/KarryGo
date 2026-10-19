package adminowner

// Profiles is the finite owner contract dictionary, shared by typed callers
// and source adapters. It contains no origin, credential, policy value or grant.
type Profile struct {
	Owner, Domain, Prefix, Path string
	Definitions                 map[string]Definition
}

func Profiles() []Profile {
	providerDimensions := []string{"account", "onboarding", "identity_verification", "vehicle_verification", "document_compliance", "eligibility", "availability", "license_expiry"}
	assetDimensions := []string{"verification", "document_compliance", "eligibility", "license_expiry", "inspection"}
	missionDimensions := []string{"mode", "source_context", "execution", "custody", "handover", "return"}
	makeDef := func(domain, kind, capability string, dimensions, counters, references []string) Definition {
		return Definition{Kind: kind, Capability: capability, SourceContract: domain + ".admin-resources.v1", Dimensions: append([]string{}, dimensions...), Counters: append([]string{}, counters...), References: append([]string{}, references...)}
	}
	profiles := []Profile{
		{Owner: "customer-service", Domain: "customer", Prefix: "CUSTOMER", Path: "/internal/customer/v1/admin", Definitions: map[string]Definition{
			"customers":          makeDef("customer", "customers", "customer.directory.read", []string{"onboarding", "verification", "analytics_consent", "marketing_consent"}, nil, nil),
			"consents":           makeDef("customer", "consents", "customer.consent.read", []string{"active", "trip_sharing_enabled", "emergency_alerts_enabled"}, nil, []string{"customer"}),
			"booking-references": makeDef("customer", "booking-references", "customer.references.read", []string{"authority", "product", "movement_type"}, nil, []string{"customer", "booking", "mission"}),
		}},
		{Owner: "driver-taxi-service", Domain: "taxi", Prefix: "TAXI", Path: "/internal/taxi/v1/admin", Definitions: map[string]Definition{
			"providers":         makeDef("taxi", "providers", "taxi.providers.read", providerDimensions, nil, nil),
			"vehicles":          makeDef("taxi", "vehicles", "taxi.vehicles.read", assetDimensions, []string{"capacity_kg", "seat_capacity"}, []string{"provider"}),
			"independent-rides": makeDef("taxi", "independent-rides", "taxi.independent_rides.read", missionDimensions, nil, []string{"provider", "vehicle"}),
		}},
		{Owner: "driver-dispatch-delivery-service", Domain: "dispatch", Prefix: "DISPATCH", Path: "/internal/dispatch/v1/admin", Definitions: map[string]Definition{
			"riders":           makeDef("dispatch", "riders", "dispatch.riders.read", providerDimensions, nil, nil),
			"motorcycles":      makeDef("dispatch", "motorcycles", "dispatch.motorcycles.read", assetDimensions, []string{"capacity_kg"}, []string{"provider"}),
			"missions":         makeDef("dispatch", "missions", "dispatch.missions.read", missionDimensions, nil, []string{"provider", "vehicle", "fleet_assignment", "fleet_response"}),
			"custody-missions": makeDef("dispatch", "custody-missions", "dispatch.custody.read", []string{"phase", "handover", "return"}, nil, []string{"mission"}),
		}},
		{Owner: "driver-hauling-service", Domain: "hauling", Prefix: "HAULING", Path: "/internal/hauling/v1/admin", Definitions: map[string]Definition{
			"providers": makeDef("hauling", "providers", "hauling.providers.read", providerDimensions, nil, nil),
			"trucks":    makeDef("hauling", "trucks", "hauling.trucks.read", assetDimensions, []string{"capacity_kg"}, []string{"provider"}),
			"missions":  makeDef("hauling", "missions", "hauling.missions.read", missionDimensions, []string{"cargo_weight_kg"}, []string{"provider", "vehicle", "fleet_assignment", "fleet_response"}),
		}},
		{Owner: "fleet-partner-service", Domain: "fleet", Prefix: "FLEET_PARTNER", Path: "/internal/fleet-partner/v1/admin", Definitions: map[string]Definition{
			"shipments":           makeDef("fleet", "shipments", "fleet.shipments.read", []string{"planning", "custody", "return"}, nil, []string{"order", "branch"}),
			"private-assignments": makeDef("fleet", "private-assignments", "fleet.private_assignments.read", []string{"execution_service", "work_origin"}, nil, []string{"provider", "vehicle", "shipment", "order"}),
			"execution-links":     makeDef("fleet", "execution-links", "fleet.execution_links.read", []string{"execution_service"}, nil, []string{"provider", "vehicle", "shipment", "assignment", "response", "canonical_mission"}),
		}},
	}
	return profiles
}

func ForOwner(owner string) (Profile, bool) {
	for _, profile := range Profiles() {
		if profile.Owner == owner {
			return profile, true
		}
	}
	return Profile{}, false
}
func ForDomain(domain string) (Profile, bool) {
	for _, profile := range Profiles() {
		if profile.Domain == domain {
			return profile, true
		}
	}
	return Profile{}, false
}
