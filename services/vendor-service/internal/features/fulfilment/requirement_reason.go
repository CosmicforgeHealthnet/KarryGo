package fulfilment

// The controlled reason vocabulary for delivery requirement and vehicle
// selection.
//
// Every refusal and every selection names one of these. The set is closed
// because reason codes are read by Support to answer a customer, counted by
// operations to find a systemic problem, and stored on an immutable snapshot
// that has to still mean the same thing a year later. A free-text reason
// satisfies none of those.
//
// The codes are grouped by the question they answer:
//
//   - CANDIDATE codes say why one vehicle class was or was not usable.
//   - DECISION codes say what the engine concluded overall.
//   - EVIDENCE codes say something about the quality of the input rather than
//     about a vehicle.

// Candidate outcome codes. Exactly one is recorded per evaluated class.
const (
	// ReasonCandidateSelected is the class the engine chose.
	ReasonCandidateSelected = "VEHICLE_SELECTED"

	// ReasonCandidateFitsNotSmallest means the class could carry the load but
	// a smaller one already could. It is recorded so an operator can see the
	// whole ladder, not just the rung that was taken.
	ReasonCandidateFitsNotSmallest = "LARGER_THAN_REQUIRED"

	ReasonWeightExceeded        = "WEIGHT_EXCEEDED"
	ReasonVolumeExceeded        = "VOLUME_EXCEEDED"
	ReasonDimensionExceeded     = "PACKAGE_DIMENSION_EXCEEDED"
	ReasonPackageCountExceeded  = "PACKAGE_COUNT_EXCEEDED"
	ReasonSinglePackageTooHeavy = "SINGLE_PACKAGE_WEIGHT_EXCEEDED"
	ReasonLargeLoadUnsupported  = "LARGE_LOAD_UNSUPPORTED"

	ReasonHandlingUnsupported        = "HANDLING_UNSUPPORTED"
	ReasonSpecialHandlingUnsupported = "SPECIAL_HANDLING_UNSUPPORTED"
	ReasonHighValueUnsupported       = "HIGH_VALUE_UNSUPPORTED"

	ReasonRouteDistanceExceeded = "ROUTE_DISTANCE_EXCEEDED"

	// ReasonMultiPickupUnsupported means the class is physically fine but the
	// owning service cannot actually drive a multi-stop collection today.
	// This is a RUNTIME capability statement, not a physical one.
	ReasonMultiPickupUnsupported = "MULTI_PICKUP_UNSUPPORTED"

	// ReasonGoodsModeDisabled means the class exists and fits, but the owning
	// service has no production goods-delivery mode. This is the car case.
	ReasonGoodsModeDisabled = "GOODS_MODE_DISABLED"

	// ReasonIntegrationDisabled means the deployment has switched the owning
	// integration off.
	ReasonIntegrationDisabled = ReasonServiceNotAvailable

	// ReasonOutOfServiceArea means the owning service does not operate this
	// route, however suitable the vehicle is.
	ReasonOutOfServiceArea = "OUT_OF_SERVICE_AREA"

	// ReasonSLANotMet means the class is physically and operationally fine
	// but cannot arrive by the promised time.
	ReasonSLANotMet = "SLA_NOT_MET"

	// ReasonBelowVendorHintFloor means the class is smaller than the vendor's
	// own stated minimum for this load.
	ReasonBelowVendorHintFloor = "BELOW_VENDOR_HINT_FLOOR"
)

// Decision codes. Exactly one is the primary reason on every decision.
const (
	// ReasonNoSuitableVehicle means nothing in the catalogue can physically
	// carry this load.
	ReasonNoSuitableVehicle = "NO_SUITABLE_VEHICLE"

	// ReasonNoExecutableService means something can carry it but nothing that
	// can carry it can currently be executed.
	ReasonNoExecutableService = "NO_SUITABLE_EXECUTABLE_SERVICE"

	// ReasonTemperatureConflict means the load carries temperature
	// requirements that cannot be held together in one compartment. It is
	// never resolved by choosing a bigger vehicle.
	ReasonTemperatureConflict = "TEMPERATURE_CONFLICT"

	// ReasonSeparationConflict means the vendor declared that items in this
	// load must not travel together.
	ReasonSeparationConflict = "SEPARATION_CONFLICT"

	// ReasonManualReviewRequired means the configured missing-metadata policy
	// refuses to decide automatically.
	ReasonManualReviewRequired = "MANUAL_REVIEW_REQUIRED"

	// ReasonMetadataIncomplete means the configured policy requires
	// measurements the vendor has not supplied.
	ReasonMetadataIncomplete = "METADATA_INCOMPLETE"
)

// Supporting evidence codes. These accompany a decision; they are never the
// whole answer on their own.
const (
	// ReasonCarPhysicallySuitable is recorded when a car is the smallest
	// class that fits but taxi goods mode is not available, so the engine had
	// to fall through to the next executable class. Without it, an operator
	// looking at a van cannot tell an oversized load from a disabled
	// integration.
	ReasonCarPhysicallySuitable = "CAR_PHYSICALLY_SUITABLE_BUT_GOODS_MODE_DISABLED"

	// ReasonEscalatedAboveMinimum records that the executed class is larger
	// than the minimum physical requirement.
	ReasonEscalatedAboveMinimum = "ESCALATED_ABOVE_MINIMUM_PHYSICAL_CLASS"

	// ReasonHintFloorApplied records that the vendor's own hints, not the
	// measurements, raised the class.
	ReasonHintFloorApplied = "VENDOR_HINT_FLOOR_APPLIED"

	// ReasonWeakEvidence records that the decision was taken on incomplete
	// measurements, which are a LOWER BOUND on the real load.
	ReasonWeakEvidence = "MEASUREMENT_EVIDENCE_INCOMPLETE"

	// ReasonUnrecognizedHandling records that the load carried a handling
	// classification outside the controlled vocabulary.
	ReasonUnrecognizedHandling = "UNRECOGNIZED_SPECIAL_HANDLING"

	// ReasonConfirmedMeasurement records that the decision was taken against
	// packed evidence rather than a catalogue estimate.
	ReasonConfirmedMeasurement = "PACKING_CONFIRMED_MEASUREMENT"

	// ReasonNonStackableDerate records that usable volume was reduced because
	// the load may not be stacked.
	ReasonNonStackableDerate = "NON_STACKABLE_VOLUME_DERATED"
)

// KnownReasonCode reports whether a code is in the controlled vocabulary.
//
// It exists so persistence and tests can assert that nothing outside the set
// ever reaches a stored snapshot. A code that fails this check is a bug, not
// an unusual load.
func KnownReasonCode(code string) bool {
	switch code {
	case ReasonCandidateSelected, ReasonCandidateFitsNotSmallest,
		ReasonWeightExceeded, ReasonVolumeExceeded, ReasonDimensionExceeded,
		ReasonPackageCountExceeded, ReasonSinglePackageTooHeavy,
		ReasonLargeLoadUnsupported, ReasonHandlingUnsupported,
		ReasonSpecialHandlingUnsupported, ReasonHighValueUnsupported,
		ReasonRouteDistanceExceeded, ReasonMultiPickupUnsupported,
		ReasonGoodsModeDisabled, ReasonIntegrationDisabled,
		ReasonOutOfServiceArea, ReasonSLANotMet, ReasonBelowVendorHintFloor,
		ReasonNoSuitableVehicle, ReasonNoExecutableService,
		ReasonTemperatureConflict, ReasonSeparationConflict,
		ReasonManualReviewRequired, ReasonMetadataIncomplete,
		ReasonCarPhysicallySuitable, ReasonEscalatedAboveMinimum,
		ReasonHintFloorApplied, ReasonWeakEvidence,
		ReasonUnrecognizedHandling, ReasonConfirmedMeasurement,
		ReasonNonStackableDerate:
		return true
	default:
		return false
	}
}
