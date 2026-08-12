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
	// ApprovalInvalidated is distinct from expired and rejected on purpose. It
	// means the thing that was approved is no longer the thing being executed:
	// the payload changed, or the subject moved on. Collapsing it into
	// "expired" would tell an operator to wait, when what they must do is ask
	// again for the new shape.
	ApprovalInvalidated         apperrors.Code = "approval_invalidated"
	ApprovalSubjectVersionStale apperrors.Code = "approval_subject_version_stale"
	ApprovalQuorumNotMet        apperrors.Code = "approval_quorum_not_met"
	ApprovalConflictingDecision apperrors.Code = "approval_conflicting_decision"
	ApprovalNotFound            apperrors.Code = "approval_not_found"
	ApprovalNotExecutable       apperrors.Code = "approval_not_executable"
	ApprovalPolicyMissing       apperrors.Code = "approval_policy_missing"
	ApprovalSeparationOfDuties  apperrors.Code = "approval_separation_of_duties"
)

// Eligibility codes.
//
// EligibilityUnknown and EligibilityStale are 503-shaped because they mean
// "ask again", while EligibilityBlocked is 422 because asking again will not
// help until something is fixed. A caller that cannot tell those apart will
// either retry forever or give up when it should retry.
const (
	EligibilityBlocked          apperrors.Code = "eligibility_blocked"
	EligibilityUnknown          apperrors.Code = "eligibility_unknown"
	EligibilityStale            apperrors.Code = "eligibility_stale"
	EligibilityPolicyMissing    apperrors.Code = "eligibility_policy_missing"
	EligibilityDecisionNotFound apperrors.Code = "eligibility_decision_not_found"
	EligibilityDecisionConflict apperrors.Code = "eligibility_decision_conflict"
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

	// Assignment acknowledgement. Kept distinct from the acceptance codes on
	// purpose: a client that cannot tell "you have not confirmed receipt" from
	// "you have not accepted" will eventually treat one as the other.
	AssignmentNotAcknowledgeable      apperrors.Code = "assignment_not_acknowledgeable"
	AssignmentAlreadyAcknowledged     apperrors.Code = "assignment_already_acknowledged"
	AssignmentAcknowledgementRequired apperrors.Code = "assignment_acknowledgement_required"
	ActorNotAuthorized                apperrors.Code = "actor_not_authorized"

	// Safety incident and dispute references.
	SafetyReferenceNotFound     apperrors.Code = "safety_reference_not_found"
	SafetyReferenceConflict     apperrors.Code = "safety_reference_conflict"
	SafetyReferenceInvalid      apperrors.Code = "safety_reference_invalid"
	SafetyReferenceUnverified   apperrors.Code = "safety_reference_unverified"
	SafetyValidationUnavailable apperrors.Code = "safety_validation_unavailable"

	// Document references to authoritative media objects.
	//
	// DocumentNotDownloadable and DocumentNotVerifiable are separate because
	// they answer different questions: the first is "you may not have this
	// file", the second is "this file cannot be signed off in its current
	// state". A client that cannot tell them apart will retry the wrong one.
	DocumentNotFound          apperrors.Code = "document_not_found"
	DocumentInvalid           apperrors.Code = "document_invalid"
	DocumentConflict          apperrors.Code = "document_conflict"
	DocumentNotDownloadable   apperrors.Code = "document_not_downloadable"
	DocumentNotVerifiable     apperrors.Code = "document_not_verifiable"
	DocumentAlreadyVerified   apperrors.Code = "document_already_verified"
	DocumentRevoked           apperrors.Code = "document_revoked"
	DocumentReplaced          apperrors.Code = "document_replaced"
	DocumentExpiredCode       apperrors.Code = "document_expired"
	DocumentDeleted           apperrors.Code = "document_deleted"
	DocumentProjectionStale   apperrors.Code = "document_projection_stale"
	DocumentLegalHoldActive   apperrors.Code = "document_legal_hold_active"
	DocumentRetentionActive   apperrors.Code = "document_retention_active"
	DocumentSelfVerification  apperrors.Code = "document_self_verification_forbidden"
	MediaAuthorityUnavailable apperrors.Code = "media_authority_unavailable"
)

// Section 13: company wallets, provider sub-wallets, allocations and
// withdrawals.
//
// WalletProjectionStale and WalletOperationUnknown are deliberately distinct.
// Stale means "Fleet's copy is behind, ask again"; unknown means "the
// authoritative service was asked and did not say", which must be reconciled
// by read-back rather than by retrying the write. A caller that conflates them
// will either duplicate a payment or give up on one that has to be resolved.
const (
	WalletProjectionStale              apperrors.Code = "wallet_projection_stale"
	WalletOperationConflict            apperrors.Code = "wallet_operation_conflict"
	WalletOperationUnknown             apperrors.Code = "wallet_operation_unknown"
	InsufficientOrganizationFunds      apperrors.Code = "insufficient_organization_wallet_funds"
	ProviderAllocationNotAllowed       apperrors.Code = "provider_allocation_not_allowed"
	ProviderAllocationDuplicate        apperrors.Code = "provider_allocation_duplicate"
	ProviderDeductionApprovalRequired  apperrors.Code = "provider_deduction_approval_required"
	ProviderWithdrawalDisabled         apperrors.Code = "provider_withdrawal_disabled"
	ProviderWithdrawalApprovalRequired apperrors.Code = "provider_withdrawal_approval_required"
	ProviderWithdrawalAlreadyResolved  apperrors.Code = "provider_withdrawal_already_resolved"
	ProviderWithdrawalNotFound         apperrors.Code = "provider_withdrawal_not_found"
	ProviderWalletStatementUnavailable apperrors.Code = "provider_wallet_statement_unavailable"
)

// Section 14: CRM, sales, quotations, contracts, SLAs and credit policy.
const (
	CustomerNotFound             apperrors.Code = "customer_not_found"
	CustomerAlreadyExists        apperrors.Code = "customer_already_exists"
	CustomerOnHold               apperrors.Code = "customer_on_hold"
	CustomerRiskRestricted       apperrors.Code = "customer_risk_restricted"
	CustomerArchived             apperrors.Code = "customer_archived"
	CustomerInvalidState         apperrors.Code = "customer_invalid_state"
	CustomerContactNotFound      apperrors.Code = "customer_contact_not_found"
	CustomerContactConflict      apperrors.Code = "customer_contact_conflict"
	CustomerLocationNotFound     apperrors.Code = "customer_location_not_found"
	CustomerLocationInvalidTZ    apperrors.Code = "customer_location_invalid_timezone"
	CustomerLocationInvalidCoord apperrors.Code = "customer_location_invalid_coordinates"
	OpportunityNotFound          apperrors.Code = "opportunity_not_found"
	OpportunityInvalidTransition apperrors.Code = "opportunity_invalid_transition"
	QuotationNotFound            apperrors.Code = "quotation_not_found"
	QuotationInvalidState        apperrors.Code = "quotation_invalid_state"
	QuotationExpired             apperrors.Code = "quotation_expired"
	QuotationAlreadyResolved     apperrors.Code = "quotation_already_resolved"
	QuotationApprovalRequired    apperrors.Code = "quotation_approval_required"
	QuotationVersionConflict     apperrors.Code = "quotation_version_conflict"
	QuotationDocumentNotReady    apperrors.Code = "quotation_document_not_ready"
	QuotationAcceptanceConflict  apperrors.Code = "quotation_acceptance_conflict"
	QuotationTokenInvalid        apperrors.Code = "quotation_token_invalid"
	QuotationTokenExpired        apperrors.Code = "quotation_token_expired"
	ContractNotFound             apperrors.Code = "contract_not_found"
	ContractInvalidState         apperrors.Code = "contract_invalid_state"
	ContractNotActive            apperrors.Code = "contract_not_active"
	ContractActivationBlocked    apperrors.Code = "contract_activation_blocked"
	ContractApprovalRequired     apperrors.Code = "contract_approval_required"
	ContractDocumentNotReady     apperrors.Code = "contract_document_not_ready"
	ContractVersionConflict      apperrors.Code = "contract_version_conflict"
	SLANotFound                  apperrors.Code = "sla_not_found"
	SLAInvalidState              apperrors.Code = "sla_invalid_state"
	SLAObservationConflict       apperrors.Code = "sla_observation_conflict"
	SLASourceUnknown             apperrors.Code = "sla_source_unknown"
	CreditProfileNotFound        apperrors.Code = "credit_profile_not_found"
	CustomerCreditHold           apperrors.Code = "customer_credit_hold"
	CreditChangeApprovalRequired apperrors.Code = "credit_change_approval_required"
)

// Section 15: service catalogue and private orders.
const (
	ServiceCatalogItemNotFound       apperrors.Code = "service_catalog_item_not_found"
	ServiceCatalogItemInvalidState   apperrors.Code = "service_catalog_item_invalid_state"
	ServiceCatalogCodeConflict       apperrors.Code = "service_catalog_code_conflict"
	ServiceCatalogCapabilityRequired apperrors.Code = "service_catalog_capability_required"
	// ServiceCatalogCategoryDisabled is returned for Taxi/PassengerTransport
	// activation regardless of capability state: Taxi execution is fail-closed
	// platform-wide, so a catalogue item for it is never allowed to become
	// active, independent of whatever an organization's own capability flags say.
	ServiceCatalogCategoryDisabled apperrors.Code = "service_catalog_category_disabled"
	ServiceCatalogSchemaInvalid    apperrors.Code = "service_catalog_schema_invalid"

	OrderNotFound             apperrors.Code = "order_not_found"
	OrderInvalidTransition    apperrors.Code = "order_invalid_transition"
	OrderNotConfirmable       apperrors.Code = "order_not_confirmable"
	OrderNotCancellable       apperrors.Code = "order_not_cancellable"
	OrderApprovalRequired     apperrors.Code = "order_approval_required"
	OrderCatalogItemNotActive apperrors.Code = "order_catalog_item_not_active"
	OrderContractNotActive    apperrors.Code = "order_contract_not_active"
	OrderStopsRequired        apperrors.Code = "order_stops_required"
	OrderStopInvalid          apperrors.Code = "order_stop_invalid"
	OrderStopSequenceConflict apperrors.Code = "order_stop_sequence_conflict"

	ConversionIntentNotFound        apperrors.Code = "conversion_intent_not_found"
	ConversionIntentAlreadyConsumed apperrors.Code = "conversion_intent_already_consumed"
	ConversionIntentMismatch        apperrors.Code = "conversion_intent_mismatch"
)

// Section 15 slice A4: shipments and cargo items.
const (
	ShipmentNotFound           apperrors.Code = "shipment_not_found"
	ShipmentInvalidState       apperrors.Code = "shipment_invalid_state"
	ShipmentLockedCode         apperrors.Code = "shipment_locked"
	ShipmentOriginInvalid      apperrors.Code = "shipment_origin_invalid"
	ShipmentDestinationInvalid apperrors.Code = "shipment_destination_invalid"
)

// Section 25: search projection.
const (
	SearchNotAvailable apperrors.Code = "search_not_available"
)

// Section 25D: generic export framework.
const (
	ExportNotFound      apperrors.Code = "export_not_found"
	ExportTypeInvalid   apperrors.Code = "export_type_invalid"
	ExportNotCancelable apperrors.Code = "export_not_cancelable"
	ExportNotReplayable apperrors.Code = "export_not_replayable"
	ExportNotReady      apperrors.Code = "export_not_ready"
	ExportExpiredCode   apperrors.Code = "export_expired"
	ExportCancelledCode apperrors.Code = "export_cancelled"
	ExportFailedCode    apperrors.Code = "export_failed"
)
