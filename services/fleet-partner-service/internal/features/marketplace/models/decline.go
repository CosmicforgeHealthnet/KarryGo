package models

// Decline reason codes. This is a closed set: an unknown value is rejected.
const (
	DeclineIllness              = "ILLNESS"
	DeclineFatigueOrRest        = "FATIGUE_OR_REST_REQUIRED"
	DeclineShiftEnding          = "SHIFT_ENDING"
	DeclineUnsafeVehicle        = "UNSAFE_VEHICLE"
	DeclineVehicleDefect        = "VEHICLE_DEFECT"
	DeclineExcessiveDistance    = "EXCESSIVE_DISTANCE"
	DeclineLocationMismatch     = "LOCATION_MISMATCH"
	DeclineServiceMismatch      = "SERVICE_MISMATCH"
	DeclineCapacityMismatch     = "CAPACITY_MISMATCH"
	DeclineCargoOrSafetyConcern = "CARGO_OR_SAFETY_CONCERN"
	DeclineSecurityConcern      = "SECURITY_CONCERN"
	DeclinePersonalEmergency    = "PERSONAL_EMERGENCY"
	DeclineAlreadyEngaged       = "ALREADY_ENGAGED"
	DeclineDeviceOrNetworkIssue = "DEVICE_OR_NETWORK_ISSUE"
	DeclineOther                = "OTHER"
)

// MaxDeclineNoteLength bounds free text so a note cannot become an
// unbounded data channel.
const MaxDeclineNoteLength = 500

// declineReasons is the closed catalogue.
var declineReasons = map[string]bool{
	DeclineIllness: true, DeclineFatigueOrRest: true, DeclineShiftEnding: true,
	DeclineUnsafeVehicle: true, DeclineVehicleDefect: true, DeclineExcessiveDistance: true,
	DeclineLocationMismatch: true, DeclineServiceMismatch: true, DeclineCapacityMismatch: true,
	DeclineCargoOrSafetyConcern: true, DeclineSecurityConcern: true, DeclinePersonalEmergency: true,
	DeclineAlreadyEngaged: true, DeclineDeviceOrNetworkIssue: true, DeclineOther: true,
}

// safetyProtectedReasons are welfare or safety declines.
//
// A decline for any of these must never produce an automatic penalty of any
// kind — no deduction, fine, suspension, removal, reduced payout,
// disciplinary finding or fraud decision. They exist so a provider can refuse
// unsafe work without fearing the system.
var safetyProtectedReasons = map[string]bool{
	DeclineIllness:              true,
	DeclineFatigueOrRest:        true,
	DeclineUnsafeVehicle:        true,
	DeclineVehicleDefect:        true,
	DeclineCargoOrSafetyConcern: true,
	DeclineSecurityConcern:      true,
	DeclinePersonalEmergency:    true,
}

// sensitiveReasons carry personal or security detail whose free-text note is
// restricted to safety/compliance readers rather than every dispatcher.
var sensitiveReasons = map[string]bool{
	DeclineIllness:           true,
	DeclinePersonalEmergency: true,
	DeclineSecurityConcern:   true,
}

// ValidDeclineReason reports whether the code is in the catalogue.
func ValidDeclineReason(code string) bool {
	return declineReasons[code]
}

// SafetyProtected reports whether a decline reason is safety or welfare
// protected.
func SafetyProtected(code string) bool {
	return safetyProtectedReasons[code]
}

// SensitiveReason reports whether the note should be access-restricted.
func SensitiveReason(code string) bool {
	return sensitiveReasons[code]
}

// RequiresNote reports whether free text is mandatory. OTHER is unusable as a
// catch-all without an explanation.
func RequiresNote(code string) bool {
	return code == DeclineOther
}

// Follow-up action types. These are typed commands for modules that arrive in
// later work packages (workforce, maintenance, safety, support); this package
// only records them durably and emits the event.
const (
	FollowupRequestUnavailability     = "REQUEST_TEMPORARY_UNAVAILABILITY"
	FollowupWelfareNotification       = "WELFARE_NOTIFICATION"
	FollowupNotifyDispatcher          = "NOTIFY_DISPATCHER"
	FollowupRefreshShiftCapacity      = "REFRESH_SHIFT_CAPACITY"
	FollowupSafetyAlert               = "SAFETY_ALERT"
	FollowupInspectionCandidate       = "INSPECTION_CANDIDATE"
	FollowupRequestVehicleBlock       = "REQUEST_VEHICLE_BLOCK"
	FollowupDefectCandidate           = "DEFECT_CANDIDATE"
	FollowupMaintenanceFollowup       = "MAINTENANCE_FOLLOWUP"
	FollowupRouteContextReview        = "ROUTE_CONTEXT_REVIEW"
	FollowupRefreshLocation           = "REFRESH_PROVIDER_LOCATION"
	FollowupCapabilityDataReview      = "CAPABILITY_DATA_REVIEW"
	FollowupVehicleDataReview         = "VEHICLE_DATA_REVIEW"
	FollowupHumanSafetyReview         = "HUMAN_SAFETY_REVIEW"
	FollowupNotifySafetyOperations    = "NOTIFY_SAFETY_AND_OPERATIONS"
	FollowupReconcileProviderWorkload = "RECONCILE_PROVIDER_WORKLOAD"
	FollowupSupportTelemetry          = "SUPPORT_TELEMETRY"
	FollowupDispatcherReview          = "DISPATCHER_REVIEW"
)

// FollowupsForReason maps a decline reason to its required durable follow-up
// actions.
//
// None of these is a penalty. Every entry either protects the provider,
// refreshes stale platform data, or asks a human to look at something.
func FollowupsForReason(code string) []string {
	switch code {
	case DeclineIllness:
		return []string{FollowupRequestUnavailability, FollowupWelfareNotification}
	case DeclineFatigueOrRest:
		return []string{FollowupRequestUnavailability, FollowupNotifyDispatcher}
	case DeclineShiftEnding:
		return []string{FollowupRefreshShiftCapacity}
	case DeclineUnsafeVehicle:
		return []string{FollowupSafetyAlert, FollowupInspectionCandidate, FollowupRequestVehicleBlock}
	case DeclineVehicleDefect:
		return []string{FollowupDefectCandidate, FollowupMaintenanceFollowup}
	case DeclineExcessiveDistance:
		return []string{FollowupRouteContextReview}
	case DeclineLocationMismatch:
		return []string{FollowupRefreshLocation}
	case DeclineServiceMismatch:
		return []string{FollowupCapabilityDataReview}
	case DeclineCapacityMismatch:
		return []string{FollowupVehicleDataReview}
	case DeclineCargoOrSafetyConcern:
		return []string{FollowupHumanSafetyReview}
	case DeclineSecurityConcern:
		return []string{FollowupNotifySafetyOperations, FollowupHumanSafetyReview}
	case DeclinePersonalEmergency:
		return []string{FollowupRequestUnavailability, FollowupWelfareNotification}
	case DeclineAlreadyEngaged:
		return []string{FollowupReconcileProviderWorkload}
	case DeclineDeviceOrNetworkIssue:
		// Explicitly telemetry only: a connectivity problem is never treated
		// as a refusal, let alone misconduct.
		return []string{FollowupSupportTelemetry}
	case DeclineOther:
		return []string{FollowupDispatcherReview}
	default:
		return nil
	}
}

// FollowupAction is a durable, retryable consequence of a decline.
type FollowupAction struct {
	ID             string
	OrganizationID string
	ResponseID     string
	MembershipID   string
	ActionType     string
	ReasonCode     string
	SafetyRelated  bool
	Payload        map[string]any
	Status         string
	Attempts       int
	LastErrorCode  *string
}

// Follow-up statuses.
const (
	FollowupPending      = "PENDING"
	FollowupDispatched   = "DISPATCHED"
	FollowupCompleted    = "COMPLETED"
	FollowupFailed       = "FAILED"
	FollowupDeadLettered = "DEAD_LETTERED"
)

// Organization-level decline reasons (dispatcher declining for the whole
// organization). Distinct from a provider declining one offer.
const (
	OrgDeclineOutsideCapability = "OUTSIDE_OPERATING_CAPABILITY"
	OrgDeclineNoCapacity        = "NO_AVAILABLE_CAPACITY"
	OrgDeclineOutsideZone       = "OUTSIDE_OPERATING_ZONE"
	OrgDeclineCommercial        = "COMMERCIAL_DECISION"
	OrgDeclineOther             = "OTHER"
)

var organizationDeclineReasons = map[string]bool{
	OrgDeclineOutsideCapability: true, OrgDeclineNoCapacity: true,
	OrgDeclineOutsideZone: true, OrgDeclineCommercial: true, OrgDeclineOther: true,
}

// ValidOrganizationDeclineReason reports whether the code is allowed.
func ValidOrganizationDeclineReason(code string) bool {
	return organizationDeclineReasons[code]
}
