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
			"review-work-items":  makeDef("customer", "review-work-items", "customer.review.work_item.state.read", nil, nil, []string{"customer"}),
			"review-states":      makeDef("customer", "review-states", "customer.review.read", nil, nil, nil),
			"customers":          makeDef("customer", "customers", "customer.directory.read", []string{"onboarding", "verification", "analytics_consent", "marketing_consent"}, nil, nil),
			"consents":           makeDef("customer", "consents", "customer.consent.read", []string{"active", "trip_sharing_enabled", "emergency_alerts_enabled"}, nil, []string{"customer"}),
			"booking-references": makeDef("customer", "booking-references", "customer.references.read", []string{"authority", "product", "movement_type"}, nil, []string{"customer", "booking", "mission"}),
		}},
		{Owner: "driver-taxi-service", Domain: "taxi", Prefix: "TAXI", Path: "/internal/taxi/v1/admin", Definitions: map[string]Definition{
			"recoveries":        makeDef("taxi", "recoveries", "taxi.recovery.read", []string{"subject_type", "trigger", "custody_transfer_required"}, nil, []string{"booking", "goods_mission", "original_provider", "original_vehicle", "replacement_provider", "replacement_vehicle"}),
			"providers":         makeDef("taxi", "providers", "taxi.providers.read", providerDimensions, nil, nil),
			"vehicles":          makeDef("taxi", "vehicles", "taxi.vehicles.read", assetDimensions, []string{"capacity_kg", "seat_capacity"}, []string{"provider"}),
			"independent-rides": makeDef("taxi", "independent-rides", "taxi.independent_rides.read", missionDimensions, nil, []string{"provider", "vehicle"}),
		}},
		{Owner: "driver-dispatch-delivery-service", Domain: "dispatch", Prefix: "DISPATCH", Path: "/internal/dispatch/v1/admin", Definitions: map[string]Definition{
			"incident-recovery-status": makeDef("dispatch", "incident-recovery-status", "dispatch.incident_recovery.read", []string{"recovery_result", "replacement_strategy", "next_attempt_at_utc", "deadline_utc"}, []string{"recovery_attempts", "captured_trip_revision"}, []string{"trip", "original_provider", "replacement_provider"}),
			"return-execution-status":  makeDef("dispatch", "return-execution-status", "dispatch.return_execution.read", []string{"assignment_state", "next_attempt_at_utc"}, []string{"assignment_attempts"}, []string{"trip", "recovery_action", "return_mission", "return_booking"}),
			"handover-states":          makeDef("dispatch", "handover-states", "dispatch.handover.read", []string{"stage", "notification"}, nil, []string{"trip"}),
			"incidents":                makeDef("dispatch", "incidents", "dispatch.incidents.read", []string{"reason", "replacement_strategy", "recovery_result"}, []string{"recovery_attempts"}, []string{"trip", "original_provider", "replacement_provider"}),
			"delivery-recoveries":      makeDef("dispatch", "delivery-recoveries", "dispatch.recovery.read", []string{"action"}, nil, []string{"trip", "delivery_attempt"}),
			"returns":                  makeDef("dispatch", "returns", "dispatch.returns.read", nil, nil, []string{"trip", "recovery_action", "return_mission", "return_booking"}),
			"riders":                   makeDef("dispatch", "riders", "dispatch.riders.read", providerDimensions, nil, nil),
			"motorcycles":              makeDef("dispatch", "motorcycles", "dispatch.motorcycles.read", assetDimensions, []string{"capacity_kg"}, []string{"provider"}),
			"missions":                 makeDef("dispatch", "missions", "dispatch.missions.read", missionDimensions, nil, []string{"provider", "vehicle", "fleet_assignment", "fleet_response"}),
			"custody-missions":         makeDef("dispatch", "custody-missions", "dispatch.custody.read", []string{"phase", "handover", "return"}, nil, []string{"mission"}),
		}},
		{Owner: "driver-hauling-service", Domain: "hauling", Prefix: "HAULING", Path: "/internal/hauling/v1/admin", Definitions: map[string]Definition{
			"booking-cargo-inputs": makeDef("hauling", "booking-cargo-inputs", "hauling.cargo_inputs.read", []string{"preferred_truck_type", "fragile_handling_required"}, []string{"recorded_weight_kg", "normalized_weight_grams"}, nil),
			"product-eligibility":  makeDef("hauling", "product-eligibility", "hauling.product_eligibility.read", []string{"product", "policy_revision", "effective_window", "effective_at_utc", "expires_at_utc", "record_window_usable", "inspection_reference_recorded", "certification_reference_recorded"}, []string{"authority_source_revision"}, []string{"provider", "truck"}),
			"fit-decisions":        makeDef("hauling", "fit-decisions", "hauling.fit_decisions.read", []string{"shadow", "policy_revision"}, []string{"captured_capability_revision"}, []string{"booking", "provider", "truck", "mission"}),
			"restrictions":         makeDef("hauling", "restrictions", "hauling.restrictions.read", []string{"subject_type", "reason"}, []string{"source_revision"}, []string{"subject"}),
			"custody-missions":     makeDef("hauling", "custody-missions", "hauling.custody.read", []string{"phase", "handover", "return"}, nil, []string{"mission"}),
			"providers":            makeDef("hauling", "providers", "hauling.providers.read", providerDimensions, nil, nil),
			"trucks":               makeDef("hauling", "trucks", "hauling.trucks.read", append(append([]string{}, assetDimensions...), []string{"enclosed", "weather_protected", "upright_safe", "fragile_safe", "refrigerated", "tail_lift", "crane", "hydraulic", "loading_ramp", "trailer_capable", "insurance_valid"}...), []string{"capacity_kg", "max_payload_grams", "internal_length_mm", "internal_width_mm", "internal_height_mm", "usable_volume_ml", "opening_width_mm", "opening_height_mm", "axle_count", "capability_version", "capability_source_version"}, []string{"provider"}),
			"recoveries":           makeDef("hauling", "recoveries", "hauling.recovery.read", []string{"trigger", "custody_transfer_required"}, nil, []string{"booking", "mission", "original_provider", "original_truck", "replacement_provider", "replacement_truck"}),
			"missions":             makeDef("hauling", "missions", "hauling.missions.read", missionDimensions, []string{"cargo_weight_kg"}, []string{"provider", "vehicle", "fleet_assignment", "fleet_response"}),
		}},
		{Owner: "fleet-partner-service", Domain: "fleet", Prefix: "FLEET_PARTNER", Path: "/internal/fleet-partner/v1/admin", Definitions: map[string]Definition{
			"shipments":           makeDef("fleet", "shipments", "fleet.shipments.read", []string{"planning", "custody", "return"}, nil, []string{"order", "branch"}),
			"private-assignments": makeDef("fleet", "private-assignments", "fleet.private_assignments.read", []string{"execution_service", "work_origin"}, nil, []string{"provider", "vehicle", "shipment", "order"}),
			"execution-links":     makeDef("fleet", "execution-links", "fleet.execution_links.read", []string{"execution_service"}, nil, []string{"provider", "vehicle", "shipment", "assignment", "response", "canonical_mission"}),
		}},
	}
	for i := range profiles {
		if profiles[i].Domain == "dispatch" {
			for kind, contract := range map[string]string{"incident-recovery-status": "dispatch.admin-incident-recovery-status.v1", "return-execution-status": "dispatch.admin-return-execution.v1"} {
				definition := profiles[i].Definitions[kind]
				definition.SourceContract = contract
				profiles[i].Definitions[kind] = definition
			}
			definition := profiles[i].Definitions["handover-states"]
			definition.SourceContract = "dispatch.admin-handover-state.v1"
			profiles[i].Definitions["handover-states"] = definition
		}
		if profiles[i].Domain == "customer" {
			work := profiles[i].Definitions["review-work-items"]
			work.SourceContract = "customer.admin-review-work-item-state.v1"
			profiles[i].Definitions["review-work-items"] = work
			definition := profiles[i].Definitions["review-states"]
			definition.SourceContract = "customer.admin-review-state.v1"
			profiles[i].Definitions["review-states"] = definition
		}
		if profiles[i].Domain == "hauling" {
			cargo := profiles[i].Definitions["booking-cargo-inputs"]
			cargo.SourceContract = "hauling.admin-booking-cargo-inputs.v1"
			profiles[i].Definitions["booking-cargo-inputs"] = cargo
			eligibility := profiles[i].Definitions["product-eligibility"]
			eligibility.SourceContract = "hauling.admin-product-eligibility.v1"
			profiles[i].Definitions["product-eligibility"] = eligibility
			fit := profiles[i].Definitions["fit-decisions"]
			fit.SourceContract = "hauling.admin-fit-decision.v1"
			profiles[i].Definitions["fit-decisions"] = fit
			restriction := profiles[i].Definitions["restrictions"]
			restriction.SourceContract = "hauling.admin-authority-restriction.v1"
			profiles[i].Definitions["restrictions"] = restriction
			definition := profiles[i].Definitions["trucks"]
			definition.SourceContract = "hauling.admin-truck-capability.v2"
			profiles[i].Definitions["trucks"] = definition
		}
		if profiles[i].Domain == "dispatch" || profiles[i].Domain == "hauling" {
			definition := profiles[i].Definitions["custody-missions"]
			definition.Custody = true
			// The larger disclosure requires a new approved source contract;
			// an old v1 owner/governance approval cannot authorize this shape.
			definition.SourceContract = profiles[i].Domain + ".admin-custody.v2"
			profiles[i].Definitions["custody-missions"] = definition
		}
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
