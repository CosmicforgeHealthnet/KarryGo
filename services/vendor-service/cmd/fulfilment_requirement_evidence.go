package main

// Requirement evidence at plan activation.
//
// The composition root where the delivery-requirement engine and the
// fulfilment aggregate meet. The engine lives in internal/features/fulfilment
// and speaks about loads and vehicle classes; the aggregate lives in
// internal/features/fulfilmentgroup and speaks about plan versions and stored
// rows. Neither imports the other, and the translation is here.
//
// WHY THE COMMITTED CLASS IS A FLOOR, NOT AN INPUT TO BE RE-DERIVED.
//
// The authoritative vehicle decision is taken at checkout, where the full
// catalogue metadata is available: temperature classes, fragility, upright and
// spill flags, separation groups, vendor hints. By the time an order activates,
// the order carries the DECISION - weight, package count and the class that was
// chosen - but not all the metadata behind it.
//
// Re-deriving a class from weight and package count alone would therefore be
// able to answer MOTORCYCLE for a load that checkout correctly sent by van
// because it was frozen. Recording that as evidence would not merely be
// useless; it would be a stored claim that the platform over-provisioned a
// delivery it actually got right.
//
// So the committed class is fed to the engine as a FLOOR. The engine can
// escalate above it - a confirmed weight that outgrew the estimate must be able
// to - and it can never go below it. The evidence that results is honest about
// what it knew: the measurements are real, the floor is recorded as a hint
// floor, and no capability the order does not carry is invented.

import (
	"encoding/json"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilment"
	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilmentgroup"
	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
)

// requirementEvidenceFor produces the stored vehicle-selection evidence for
// every platform mission in an activating order.
//
// A mission whose evidence cannot be produced is SKIPPED rather than failing
// the activation. Evidence is an explanation of a delivery, not a precondition
// for one: refusing to activate a paid order because its explanation could not
// be assembled would turn a documentation problem into a customer problem.
func requirementEvidenceFor(
	plans []orders.MissionPlan,
	policy fulfilment.CapabilityPolicy,
	now time.Time,
) []fulfilmentgroup.RequirementDecision {
	if len(plans) == 0 {
		return nil
	}

	if len(policy.Classes) == 0 {
		policy = fulfilment.DefaultCapabilityPolicy()
	}

	// Both authorities are reachable by definition here: the order has already
	// been quoted and paid, which could not have happened through an
	// integration that was switched off. Stating it explicitly keeps the
	// engine pure - it never reads a global to discover what is enabled.
	serviceability := fulfilment.ServiceabilityFrom(map[fulfilment.Service]bool{
		fulfilment.ServiceDispatch: true,
		fulfilment.ServiceHauling:  true,
	}, now)

	decisions := make([]fulfilmentgroup.RequirementDecision, 0, len(plans))

	for _, plan := range plans {
		requirement := fulfilment.DeliveryRequirement{
			BusinessID: plan.PickupBusiness,
			BranchID:   plan.PickupBranch,
			Estimated: fulfilment.PhysicalMeasurement{
				State:            fulfilment.MeasurementEstimated,
				Source:           fulfilment.SourceCatalogEstimate,
				PackageCount:     plan.PackageCount,
				TotalWeightGrams: plan.WeightGrams,
				Stackable:        true,
			},
			// One pickup per mission. A consolidated slot covers vendors
			// inside one tight shared radius, which is one physical
			// collection, not a multi-stop route.
			Route: fulfilment.RouteProfile{PickupCount: 1},
		}

		// The prior decision as a floor. See the file note.
		if plan.VehicleClass != "" {
			requirement.VehicleHints = []string{plan.VehicleClass}
		}

		snapshot, err := fulfilment.EvaluateAndSnapshot(
			fulfilment.SelectionRequest{
				Requirement:    requirement,
				Policy:         policy,
				Serviceability: serviceability,
				Now:            now,
			},
		)
		if err != nil {
			continue
		}

		decision := requirementDecisionFrom(
			snapshot.Attach("", "", plan.PlanMissionRef),
		)

		// A decision that contradicts the plan it is filed against is worse
		// than no decision: it would tell Support the delivery ran on a
		// vehicle it did not run on. Only consistent evidence is stored.
		if decision.Executable &&
			plan.VehicleClass != "" &&
			decision.SelectedVehicleClass != plan.VehicleClass {
			continue
		}

		decisions = append(decisions, decision)
	}

	return decisions
}

// requirementDecisionFrom translates the engine's snapshot into the row the
// fulfilment aggregate stores.
//
// The evidence document is carried whole. Storing a summary instead would be a
// second implementation of the decision, and the two would eventually disagree
// about what was decided.
func requirementDecisionFrom(
	snapshot fulfilment.RequirementSnapshot,
) fulfilmentgroup.RequirementDecision {
	measurement := snapshot.Requirement.Authoritative()
	decision := snapshot.Decision

	record := fulfilmentgroup.RequirementDecision{
		SlotRef:    snapshot.SlotRef,
		BusinessID: snapshot.BusinessID,
		BranchID:   snapshot.BranchID,

		MinimumPhysicalClass: string(decision.MinimumPhysicalClass),
		SelectedVehicleClass: string(decision.SelectedClass),
		SelectedService:      string(decision.SelectedService),
		Executable:           decision.Executable,

		PrimaryReason:     decision.PrimaryReason,
		SupportingReasons: decision.SupportingReasons,

		MeasurementState:  string(decision.MeasurementState),
		MeasurementSource: string(decision.MeasurementSource),

		TotalWeightGrams:   measurement.TotalWeightGrams,
		TotalVolumeMl:      measurement.TotalVolumeMl,
		EffectiveVolumeMl:  decision.EffectiveVolumeMl,
		LongestDimensionMm: measurement.LongestDimensionMm,
		PackageCount:       measurement.PackageCount,

		RouteDistanceMeters: snapshot.Requirement.Route.DistanceMeters,
		PickupCount:         snapshot.Requirement.Route.PickupCount,

		RiskClass:          string(snapshot.Requirement.Value.RiskClass),
		DeclaredValueMinor: snapshot.Requirement.Value.DeclaredValueMinor,

		PolicyVersion: decision.PolicyVersion,
		Fingerprint:   snapshot.Fingerprint,
	}

	if record.DeclaredValueMinor > 0 {
		record.DeclaredValueCurrency = snapshot.Requirement.Value.Currency
	}

	if payload, err := snapshot.MarshalEvidence(); err == nil {
		record.Evidence = payload
	}

	if payload, err := json.Marshal(decision.Candidates); err == nil {
		record.Candidates = payload
	}

	return record
}
