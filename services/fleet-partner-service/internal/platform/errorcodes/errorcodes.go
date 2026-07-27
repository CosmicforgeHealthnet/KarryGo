// Package errorcodes defines the stable machine-readable error codes for
// Sections 8–10 and maps them onto the canonical error envelope.
//
// Messages are safe public text: they never reveal whether an account exists,
// never carry downstream detail and never include credential material.
package errorcodes

import (
	"net/http"

	"cosmicforge/logistics/shared/go/apperrors"
)

// Authentication codes.
const (
	OTPDeliveryUnavailable        apperrors.Code = "otp_delivery_unavailable"
	OTPChallengeNotFound          apperrors.Code = "otp_challenge_not_found"
	OTPChallengeExpired           apperrors.Code = "otp_challenge_expired"
	OTPChallengeConsumed          apperrors.Code = "otp_challenge_consumed"
	OTPInvalid                    apperrors.Code = "otp_invalid"
	OTPAttemptLimitExceeded       apperrors.Code = "otp_attempt_limit_exceeded"
	OTPResendTooSoon              apperrors.Code = "otp_resend_too_soon"
	AuthenticationRateLimited     apperrors.Code = "authentication_rate_limited"
	IdentityPairConflict          apperrors.Code = "identity_pair_conflict"
	IdentitySuspended             apperrors.Code = "identity_suspended"
	SessionExpired                apperrors.Code = "session_expired"
	SessionRevoked                apperrors.Code = "session_revoked"
	RefreshTokenReused            apperrors.Code = "refresh_token_reused"
	OrganizationSelectionRequired apperrors.Code = "organization_selection_required"
	StepUpRequired                apperrors.Code = "step_up_required"
	StepUpExpired                 apperrors.Code = "step_up_expired"
)

// Organization onboarding and lifecycle codes.
const (
	OrganizationOnboardingIncomplete apperrors.Code = "organization_onboarding_incomplete"
	OrganizationAlreadySubmitted     apperrors.Code = "organization_already_submitted"
	OrganizationVerificationPending  apperrors.Code = "organization_verification_pending"
	OrganizationChangesRequired      apperrors.Code = "organization_changes_required"
	OrganizationActivationBlocked    apperrors.Code = "organization_activation_blocked"
	OrganizationWalletNotReady       apperrors.Code = "organization_wallet_not_ready"
	OrganizationClosureBlocked       apperrors.Code = "organization_closure_blocked"
)

// Staff and IAM codes.
const (
	StaffInvitationExpired        apperrors.Code = "staff_invitation_expired"
	StaffInvitationConsumed       apperrors.Code = "staff_invitation_consumed"
	StaffAlreadyMember            apperrors.Code = "staff_already_member"
	LastOwnerRequired             apperrors.Code = "last_owner_required"
	RoleScopeInvalid              apperrors.Code = "role_scope_invalid"
	RoleIsSystemManaged           apperrors.Code = "role_is_system_managed"
	PermissionEscalationForbidden apperrors.Code = "permission_escalation_forbidden"
)

// Approval codes.
const (
	ApprovalRequired              apperrors.Code = "approval_required"
	ApprovalExpired               apperrors.Code = "approval_expired"
	ApprovalAlreadyResolved       apperrors.Code = "approval_already_resolved"
	ApprovalSelfDecisionForbidden apperrors.Code = "approval_self_decision_forbidden"
	ApprovalActorNotEligible      apperrors.Code = "approval_actor_not_eligible"
	ApprovalFingerprintMismatch   apperrors.Code = "approval_fingerprint_mismatch"
)

// Managed provider codes.
const (
	ProviderIdentityConflict       apperrors.Code = "provider_identity_conflict"
	ProviderAlreadyManaged         apperrors.Code = "provider_already_managed"
	ProviderAssociationPending     apperrors.Code = "provider_association_pending"
	ProviderAssociationDeclined    apperrors.Code = "provider_association_declined"
	ProviderMembershipNotActive    apperrors.Code = "provider_membership_not_active"
	ProviderProvisioningIncomplete apperrors.Code = "provider_provisioning_incomplete"
	ProviderSubwalletNotReady      apperrors.Code = "provider_subwallet_not_ready"
	ProviderServiceUnavailable     apperrors.Code = "provider_service_unavailable"
)

// New builds an *apperrors.Error carrying one of the codes above.
func New(status int, code apperrors.Code, message string, cause error) *apperrors.Error {
	return apperrors.New(status, code, message, cause)
}

// Conflict builds a 409 with a specific code.
func Conflict(code apperrors.Code, message string) *apperrors.Error {
	return apperrors.New(http.StatusConflict, code, message, nil)
}

// BadRequest builds a 400 with a specific code.
func BadRequest(code apperrors.Code, message string) *apperrors.Error {
	return apperrors.New(http.StatusBadRequest, code, message, nil)
}

// Forbidden builds a 403 with a specific code.
func Forbidden(code apperrors.Code, message string) *apperrors.Error {
	return apperrors.New(http.StatusForbidden, code, message, nil)
}

// NotFound builds a 404 with a stable code.
func NotFound(code apperrors.Code, message string) *apperrors.Error {
	return apperrors.New(http.StatusNotFound, code, message, nil)
}

// Unauthorized builds a 401 with a specific code.
func Unauthorized(code apperrors.Code, message string) *apperrors.Error {
	return apperrors.New(http.StatusUnauthorized, code, message, nil)
}

// TooManyRequests builds a 429 with a specific code.
func TooManyRequests(code apperrors.Code, message string) *apperrors.Error {
	return apperrors.New(http.StatusTooManyRequests, code, message, nil)
}

// Unavailable builds a 503 with a specific code.
func Unavailable(code apperrors.Code, message string) *apperrors.Error {
	return apperrors.New(http.StatusServiceUnavailable, code, message, nil)
}

// UnprocessableEntity builds a 422 with a specific code.
func UnprocessableEntity(code apperrors.Code, message string) *apperrors.Error {
	return apperrors.New(http.StatusUnprocessableEntity, code, message, nil)
}

// Sections 11–12: marketplace, assignment and trip integration codes.
const (
	MarketplaceOfferNotFound        apperrors.Code = "marketplace_offer_not_found"
	MarketplaceOfferExpired         apperrors.Code = "marketplace_offer_expired"
	MarketplaceOfferCancelled       apperrors.Code = "marketplace_offer_cancelled"
	MarketplaceOfferAlreadyResolved apperrors.Code = "marketplace_offer_already_resolved"
	ValidationFailed                apperrors.Code = "validation_error"
	MarketplaceModeNotAllowed       apperrors.Code = "marketplace_mode_not_allowed"
	MarketplaceProjectionStale      apperrors.Code = "marketplace_projection_stale"
	MarketplaceProjectionGap        apperrors.Code = "marketplace_projection_gap"
	RequestAlreadyClaimed           apperrors.Code = "request_already_claimed"
	RequestNotAssignable            apperrors.Code = "request_not_assignable"
	OrganizationNotEligible         apperrors.Code = "organization_not_eligible"

	AssignmentNotFound             apperrors.Code = "assignment_not_found"
	AssignmentAlreadyResolved      apperrors.Code = "assignment_already_resolved"
	AssignmentResponseExpired      apperrors.Code = "assignment_response_expired"
	AssignmentNotOwnedByProvider   apperrors.Code = "assignment_not_owned_by_provider"
	AssignmentProviderMismatch     apperrors.Code = "assignment_provider_mismatch"
	AssignmentConflict             apperrors.Code = "assignment_conflict"
	AssignmentReassignmentRequired apperrors.Code = "assignment_reassignment_required"
	AssignmentCannotCancel         apperrors.Code = "assignment_cannot_cancel"
	AssignmentCannotReassign       apperrors.Code = "assignment_cannot_reassign"

	DeclineReasonRequired  apperrors.Code = "decline_reason_required"
	DeclineReasonInvalid   apperrors.Code = "decline_reason_invalid"
	DeclineNoteRequired    apperrors.Code = "decline_note_required"
	DeclineEvidenceInvalid apperrors.Code = "decline_evidence_invalid"

	ProviderNotEligible        apperrors.Code = "provider_not_eligible"
	ProviderNotAvailable       apperrors.Code = "provider_not_available"
	ProviderShiftInvalid       apperrors.Code = "provider_shift_invalid"
	ProviderHasConflictingWork apperrors.Code = "provider_has_conflicting_work"
	ProviderRestrictionActive  apperrors.Code = "provider_restriction_active"

	VehicleNotEligible      apperrors.Code = "vehicle_not_eligible"
	VehicleCapacityMismatch apperrors.Code = "vehicle_capacity_mismatch"
	VehicleSafetyBlocked    apperrors.Code = "vehicle_safety_blocked"
	VehicleConflict         apperrors.Code = "vehicle_conflict"

	SourceVersionMismatch    apperrors.Code = "source_version_mismatch"
	SourceServiceUnavailable apperrors.Code = "source_service_unavailable"

	// Cross-service provider-acceptance verification. Dispatch branches on
	// these to tell "not accepted yet" apart from "will never be accepted".
	FleetAssignmentNotFound              apperrors.Code = "fleet_assignment_not_found"
	ProviderAcceptanceNotConfirmed       apperrors.Code = "provider_acceptance_not_confirmed"
	ProviderAcceptanceMismatch           apperrors.Code = "provider_acceptance_mismatch"
	ProviderAcceptanceExpired            apperrors.Code = "provider_acceptance_expired"
	ProviderAcceptanceCancelled          apperrors.Code = "provider_acceptance_cancelled"
	FleetServiceUnavailable              apperrors.Code = "fleet_service_unavailable"
	SourceOutcomeUnknown                 apperrors.Code = "source_outcome_unknown"
	ExecutionStartReconciliationRequired apperrors.Code = "execution_start_reconciliation_required"
	ExpectedVersionMismatch              apperrors.Code = "expected_version_mismatch"
	IdempotencyConflictCode              apperrors.Code = "idempotency_conflict"
)
