package usecases

import (
	"context"
	"errors"
	"strings"
	"time"

	orgmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/models"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/repositories"
	providermodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// The checks.
//
// Each check runs only when the governing policy names it, and each has
// exactly one way to pass. Every other outcome — including "the query failed"
// and "the authority is unreachable" — blocks. There is no branch anywhere in
// this file where an error or an absent record results in the check being
// skipped, because a check that can be skipped by breaking something is not a
// check.

// authoritySourceKind values recorded on a decision.
const (
	sourceKindOrganization = "ORGANIZATION"
	sourceKindVerification = "VERIFICATION"
	sourceKindSubscription = "SUBSCRIPTION"
	sourceKindMembership   = "PROVIDER_MEMBERSHIP"
	sourceKindProvider     = "PROVIDER"
	sourceKindVehicle      = "VEHICLE"
)

const authorityFleet = "fleet-partner-service"

// checkCompany evaluates the organization half of every decision.
//
// It runs for provider and vehicle subjects too. A suspended organization
// cannot link a provider or a vehicle, and checking the subject without
// checking who is asking would let a closed company keep operating through its
// assets.
func (s *Service) checkCompany(ctx context.Context, input EvaluateInput,
	policy *models.Policy, result *evaluation) {
	organizationID := input.Actor.OrganizationID

	state, err := s.opts.Decisions.CompanyState(ctx, nil, organizationID)
	if err != nil {
		result.blockUnknown(models.ReasonAuthorityUnavailable)
		return
	}
	if !state.Found {
		result.block(models.ReasonCompanyNotFound)
		return
	}
	result.source(models.DecisionSource{
		AuthorityService: authorityFleet,
		SourceKind:       sourceKindOrganization,
		SourceReference:  organizationID,
		SourceVersion:    state.Version,
		FreshnessState:   models.FreshnessCurrent,
		ObservedAt:       s.opts.Now().UTC(),
	})

	if policy.RequiresCompanyCheck(models.CheckLifecycle) {
		switch state.Status {
		case orgmodels.OrganizationStatusActive:
		case orgmodels.OrganizationStatusSuspended:
			result.block(models.ReasonCompanySuspended)
		case orgmodels.OrganizationStatusArchived:
			result.block(models.ReasonCompanyArchived)
		default:
			result.block(models.ReasonCompanyNotActive)
		}
		if state.ArchivedAt != nil {
			result.block(models.ReasonCompanyArchived)
		}
	}

	if policy.RequiresCompanyCheck(models.CheckVerification) {
		s.checkCompanyVerification(ctx, input, policy, result)
	}

	if policy.RequiresCompanyCheck(models.CheckApproval) {
		complete, onboardingErr := s.opts.Decisions.OnboardingComplete(ctx, nil, organizationID)
		switch {
		case onboardingErr != nil:
			result.blockUnknown(models.ReasonAuthorityUnavailable)
		case !complete:
			result.block(models.ReasonCompanyNotApproved)
		}
	}

	if policy.RequiresCompanyCheck(models.CheckRestrictions) {
		restrictions, restrictionErr := s.opts.Decisions.RestrictionState(ctx, nil, organizationID)
		switch {
		case restrictionErr != nil:
			result.blockUnknown(models.ReasonAuthorityUnavailable)
		case restrictions.Restricted():
			result.block(models.ReasonCompanyRestricted)
		}
	}

	if policy.RequiresCompanyCheck(models.CheckDocuments) {
		s.checkCompanyDocuments(ctx, input, policy, result)
	}

	if policy.RequiresCompanyCheck(models.CheckBranch) && input.BranchID != "" {
		branch, branchErr := s.opts.Decisions.BranchState(ctx, nil, organizationID, input.BranchID)
		switch {
		case branchErr != nil:
			result.blockUnknown(models.ReasonAuthorityUnavailable)
		case !branch.Found:
			// The branch belonging to another tenant and the branch not
			// existing produce the same reason, so the decision record cannot
			// be used to probe another organization's structure.
			result.block(models.ReasonCompanyBranchInvalid)
		case branch.Status != orgmodels.BranchStatusActive:
			result.block(models.ReasonCompanyBranchInvalid)
		}
	}

	if policy.RequiresCompanyCheck(models.CheckCapability) && input.ServiceCategory != "" {
		approved, capabilityErr := s.opts.Decisions.CapabilityApproved(ctx, nil,
			organizationID, input.ServiceCategory)
		switch {
		case capabilityErr != nil:
			result.blockUnknown(models.ReasonAuthorityUnavailable)
		case !approved:
			result.block(models.ReasonCompanyCapabilityMissing)
		}
	}

	if policy.RequiresCompanyCheck(models.CheckSubscription) || policy.SubscriptionRequired {
		s.checkSubscription(ctx, input, policy, result)
	}
}

// checkCompanyVerification reads the authoritative verification outcome
// projection.
func (s *Service) checkCompanyVerification(ctx context.Context, input EvaluateInput,
	policy *models.Policy, result *evaluation) {
	verification, err := s.opts.Decisions.VerificationState(ctx, nil, input.Actor.OrganizationID)
	if err != nil {
		result.blockUnknown(models.ReasonAuthorityUnavailable)
		return
	}
	if !verification.Found {
		result.block(models.ReasonCompanyVerificationPending)
		return
	}

	freshness := models.FreshnessCurrent
	if verification.SyncedAt == nil {
		// Never synced means the projection is a local record of what was sent,
		// not a record of what the authority decided.
		freshness = models.FreshnessUnknown
	} else if s.opts.Now().UTC().Sub(verification.SyncedAt.UTC()) > policy.SourceMaxAge() {
		freshness = models.FreshnessStale
	}
	result.source(models.DecisionSource{
		AuthorityService: "verification-compliance-service",
		SourceKind:       sourceKindVerification,
		SourceReference:  input.Actor.OrganizationID,
		SourceVersion:    verification.SourceVersion,
		FreshnessState:   freshness,
		ObservedAt:       s.opts.Now().UTC(),
	})

	switch verification.Status {
	case "APPROVED":
	case "REJECTED":
		result.block(models.ReasonCompanyVerificationRejected)
		return
	case "UNKNOWN_OUTCOME":
		result.blockUnknown(models.ReasonCompanyVerificationUnknown)
		return
	default:
		result.block(models.ReasonCompanyVerificationPending)
		return
	}

	// An APPROVED verdict that the platform has not been able to confirm
	// recently is stale, not approved. The distinction matters most for the
	// case it exists to catch: an approval that was later revoked while the
	// projection went quiet.
	switch freshness {
	case models.FreshnessUnknown:
		result.blockUnknown(models.ReasonCompanyVerificationUnknown)
	case models.FreshnessStale:
		result.blockStale(models.ReasonSourceVersionStale)
	}
}

// checkCompanyDocuments enforces the policy's required document list.
//
// Two record sets can satisfy a requirement, and both are consulted: the
// legacy onboarding attachment and the WP10 document reference to an
// authoritative media object. A requirement is satisfied when EITHER holds,
// because an organization onboarded before WP10 must not be blocked for having
// used the record that existed at the time.
//
// What is not negotiable is the standard the WP10 record has to meet: verified
// by a human, confirmed SAFE by media, unexpired, not revoked, not superseded,
// not deleted, and backed by a projection nobody is reconciling. An
// organization cannot satisfy a compliance requirement by uploading a file
// nobody has vouched for.
func (s *Service) checkCompanyDocuments(ctx context.Context, input EvaluateInput,
	policy *models.Policy, result *evaluation) {
	if len(policy.RequiredDocuments) == 0 {
		return
	}
	s.checkDocumentRequirements(ctx, result, input.Actor.OrganizationID,
		"COMPANY", input.Actor.OrganizationID, policy.RequiredDocuments, true)
}

// checkDocumentRequirements evaluates one owner's required documents.
//
// legacyFallback consults the pre-WP10 onboarding attachments as an
// alternative source of satisfaction; it applies only to the organization
// itself, because no other owner type ever had those records.
func (s *Service) checkDocumentRequirements(ctx context.Context, result *evaluation,
	organizationID string, ownerType string, ownerID string, requiredTypes []string,
	legacyFallback bool) {
	references, err := s.opts.Decisions.ReferenceDocumentStates(ctx, nil,
		organizationID, ownerType, ownerID, requiredTypes)
	if err != nil {
		result.blockUnknown(models.ReasonAuthorityUnavailable)
		return
	}

	legacy := map[string]repositories.DocumentState{}
	if legacyFallback {
		states, legacyErr := s.opts.Decisions.DocumentStates(ctx, nil, organizationID, requiredTypes)
		if legacyErr != nil {
			result.blockUnknown(models.ReasonAuthorityUnavailable)
			return
		}
		for _, state := range states {
			legacy[state.DocumentType] = state
		}
	}

	for _, reference := range references {
		if reference.Satisfied {
			continue
		}
		if legacyFallback {
			if state, ok := legacy[reference.DocumentType]; ok && state.Present && !state.Expired && !state.Rejected {
				continue
			}
			// No WP10 document and no usable legacy one: report the legacy
			// vocabulary, which is what an operator looking at an onboarding
			// checklist will recognise.
			if !reference.Present {
				state := legacy[reference.DocumentType]
				switch {
				case state.Rejected:
					result.block(models.ReasonCompanyDocumentRejected)
				case state.Present && state.Expired:
					result.block(models.ReasonCompanyDocumentExpired)
				default:
					result.block(models.ReasonCompanyDocumentMissing)
				}
				continue
			}
		}
		s.blockForDocument(result, reference.Reason)
	}
}

// blockForDocument routes a document reason onto the right decision status.
//
// The distinction matters downstream: RECONCILING and STALE mean "ask again",
// while a missing or revoked document means "asking again will not help".
func (s *Service) blockForDocument(result *evaluation, reason string) {
	if reason == "" {
		reason = models.ReasonDocumentMissing
	}
	switch reason {
	case models.ReasonDocumentReconciling:
		result.blockReconciling(reason)
	case models.ReasonDocumentProjectionStale:
		result.blockStale(reason)
	default:
		result.block(reason)
	}
}

// checkSubscription enforces the paid-subscription requirement.
func (s *Service) checkSubscription(ctx context.Context, input EvaluateInput,
	policy *models.Policy, result *evaluation) {
	subscription, err := s.opts.Decisions.SubscriptionState(ctx, nil, input.Actor.OrganizationID)
	if err != nil {
		result.blockUnknown(models.ReasonAuthorityUnavailable)
		return
	}
	if !subscription.Found {
		result.block(models.ReasonCompanySubscriptionInactive)
		return
	}
	result.source(models.DecisionSource{
		AuthorityService: "payment-wallet-service",
		SourceKind:       sourceKindSubscription,
		SourceReference:  input.Actor.OrganizationID,
		SourceVersion:    subscription.SourceVersion,
		FreshnessState:   freshnessFor(subscription.FreshnessState, subscription.ReconciliationStatus),
		ObservedAt:       s.opts.Now().UTC(),
	})

	if !subscription.Usable() {
		if subscription.ReconciliationStatus != "IDLE" {
			result.blockReconciling(models.ReasonCompanySubscriptionStale)
			return
		}
		result.blockStale(models.ReasonCompanySubscriptionStale)
		return
	}
	if s.opts.Now().UTC().Sub(subscription.LastSyncedAt.UTC()) > policy.SourceMaxAge() {
		result.blockStale(models.ReasonCompanySubscriptionStale)
		return
	}
	if !strings.EqualFold(subscription.SafeStatus, "ACTIVE") {
		result.block(models.ReasonCompanySubscriptionInactive)
	}
}

// freshnessFor maps a projection's two state fields onto the decision source
// vocabulary. Reconciliation outranks freshness: a CURRENT projection with an
// open reconciliation case is not current in any sense that matters.
func freshnessFor(freshness string, reconciliation string) string {
	if reconciliation != "" && reconciliation != "IDLE" {
		return models.FreshnessReconciling
	}
	switch freshness {
	case models.FreshnessCurrent, models.FreshnessStale, models.FreshnessGapDetected,
		models.FreshnessUnknown, models.FreshnessReconciling:
		return freshness
	default:
		return models.FreshnessUnknown
	}
}

// checkProvider evaluates a provider membership against both halves of the
// truth: fleet's relationship record and the owning service's authority.
func (s *Service) checkProvider(ctx context.Context, input EvaluateInput,
	policy *models.Policy, result *evaluation) {
	membership, err := s.opts.Decisions.MembershipState(ctx, nil,
		input.Actor.OrganizationID, input.SubjectID)
	if err != nil {
		result.blockUnknown(models.ReasonAuthorityUnavailable)
		return
	}
	if !membership.Found {
		// Another tenant's membership and a non-existent one answer
		// identically, so a decision cannot be used to enumerate providers.
		result.block(models.ReasonProviderNotFound)
		return
	}
	result.source(models.DecisionSource{
		AuthorityService: authorityFleet,
		SourceKind:       sourceKindMembership,
		SourceReference:  membership.ID,
		SourceVersion:    membership.Version,
		FreshnessState:   models.FreshnessCurrent,
		ObservedAt:       s.opts.Now().UTC(),
	})

	if policy.RequiresProviderCheck(models.CheckMembership) {
		switch membership.Status {
		case providermodels.StatusActive:
		case providermodels.StatusSuspended:
			result.block(models.ReasonProviderSuspended)
		default:
			result.block(models.ReasonProviderMembershipNotActive)
		}
	}

	if policy.RequiresProviderCheck(models.CheckConsent) && membership.AssociationAcceptedAt == nil {
		// The provider never agreed to work for this organization. No amount
		// of organization-side state substitutes for that.
		result.block(models.ReasonProviderConsentMissing)
	}

	if policy.RequiresProviderCheck(models.CheckServiceType) && input.ServiceCategory != "" &&
		!strings.EqualFold(membership.ServiceCategory, input.ServiceCategory) {
		result.block(models.ReasonProviderServiceMismatch)
	}

	if policy.RequiresProviderCheck(models.CheckBranch) && input.BranchID != "" {
		if membership.HomeBranchID == nil || *membership.HomeBranchID != input.BranchID {
			result.block(models.ReasonProviderBranchMismatch)
		}
	}

	if policy.RequiresProviderCheck(models.CheckAssignmentConflict) {
		live, conflictErr := s.opts.Decisions.MembershipHasLiveAssignment(ctx, nil,
			input.Actor.OrganizationID, membership.ID, input.ExcludeAssignmentID)
		switch {
		case conflictErr != nil:
			result.blockUnknown(models.ReasonAuthorityUnavailable)
		case live:
			result.block(models.ReasonProviderAssignmentConflict)
		}
	}

	// Fleet-side documents attached to the provider link. These are separate
	// from the documents the owning service holds about the provider: an
	// organization can require its own paperwork (a signed contract, a local
	// permit) that the platform authority knows nothing about.
	if policy.RequiresProviderCheck(models.CheckDocuments) && len(policy.RequiredDocuments) > 0 {
		s.checkDocumentRequirements(ctx, result, input.Actor.OrganizationID,
			"PROVIDER_LINK", membership.ID, policy.RequiredDocuments, false)
	}

	if policy.RequiresProviderCheck(models.CheckProjectionFreshness) {
		if membership.SyncedAt == nil {
			result.blockStale(models.ReasonProviderProjectionStale)
		} else if s.opts.Now().UTC().Sub(membership.SyncedAt.UTC()) > policy.SourceMaxAge() {
			result.blockStale(models.ReasonProviderProjectionStale)
		}
	}

	if policy.RequiresProviderCheck(models.CheckExistence) ||
		policy.RequiresProviderCheck(models.CheckVerification) ||
		policy.RequiresProviderCheck(models.CheckAvailability) ||
		policy.RequiresProviderCheck(models.CheckShift) ||
		policy.RequiresProviderCheck(models.CheckDocuments) ||
		policy.RequiresProviderCheck(models.CheckRestrictions) {
		s.checkProviderAuthority(ctx, input, policy, membership, result)
	}
}

// checkProviderAuthority asks the owning service about platform standing.
func (s *Service) checkProviderAuthority(ctx context.Context, input EvaluateInput,
	policy *models.Policy, membership repositories.MembershipState, result *evaluation) {
	providerID := membership.ProviderID
	if providerID == "" {
		// A membership with no authoritative identity has nobody to ask about.
		// That is a definitive block, not an unknown.
		result.block(models.ReasonProviderNotLinked)
		return
	}
	authority := s.opts.Providers
	if candidate := s.opts.ProviderAuthorities[strings.ToLower(membership.ProviderService)]; candidate != nil {
		authority = candidate
	}
	if authority == nil || !authority.Configured() {
		result.blockUnknown(models.ReasonAuthorityNotConfigured)
		return
	}

	state, err := authority.LookupProvider(ctx, platformclients.CallMeta{
		OrganizationID: input.Actor.OrganizationID,
		CorrelationID:  input.CorrelationID,
		CausationID:    input.CausationID,
	}, platformclients.ProviderLookup{ProviderID: providerID})
	if err != nil {
		if errors.Is(err, platformclients.ErrProviderNotFound) {
			result.block(models.ReasonProviderNotFound)
			return
		}
		// Unreachable is not evidence. It is recorded as UNKNOWN with the
		// authority named, so an operator can see which integration was down
		// when the block happened.
		result.source(models.DecisionSource{
			AuthorityService: authority.AuthorityService(),
			SourceKind:       sourceKindProvider,
			SourceReference:  providerID,
			FreshnessState:   models.FreshnessUnknown,
			ObservedAt:       s.opts.Now().UTC(),
		})
		result.blockUnknown(models.ReasonAuthorityUnavailable)
		return
	}

	result.source(models.DecisionSource{
		AuthorityService: authority.AuthorityService(),
		SourceKind:       sourceKindProvider,
		SourceReference:  state.ProviderID,
		SourceVersion:    state.SourceVersion,
		FreshnessState:   authorityFreshness(state.ObservedAt, policy.SourceMaxAge(), s.opts.Now().UTC()),
		ObservedAt:       s.opts.Now().UTC(),
	})

	if policy.RequiresProviderCheck(models.CheckExistence) &&
		state.AccountState != platformclients.ProviderAccountActive {
		result.block(models.ReasonProviderMembershipNotActive)
	}
	if policy.RequiresProviderCheck(models.CheckVerification) &&
		state.VerificationState != platformclients.VehicleVerificationVerified {
		result.block(models.ReasonProviderUnverified)
	}
	// The service-type check at the authority is not a string comparison. The
	// provider's approved service IS the service that owns them, so what must
	// hold is that the authority which just answered is the same service
	// fleet's membership names. A dispatch authority vouching for a provider
	// fleet believes is a taxi provider is a mismatch however verified they
	// are.
	if policy.RequiresProviderCheck(models.CheckServiceType) &&
		membership.ProviderService != "" &&
		!strings.EqualFold(membership.ProviderService, authority.AuthorityService()) {
		result.block(models.ReasonProviderServiceMismatch)
	}
	if policy.RequiresProviderCheck(models.CheckRestrictions) && state.Restricted {
		result.block(models.ReasonProviderSuspended)
	}
	if policy.RequiresProviderCheck(models.CheckAvailability) && !state.Available {
		result.block(models.ReasonProviderNotAvailable)
	}
	if policy.RequiresProviderCheck(models.CheckShift) && !state.ShiftOpen {
		result.block(models.ReasonProviderShiftBlocked)
	}
	if policy.RequiresProviderCheck(models.CheckDocuments) && !state.DocumentsSatisfied {
		for _, document := range state.RequiredDocuments {
			switch {
			case !document.Present:
				result.block(models.ReasonProviderDocumentMissing)
			case document.Expired:
				result.block(models.ReasonProviderDocumentExpired)
			}
		}
	}
	if policy.RequiresProviderCheck(models.CheckAssignmentConflict) && state.ActiveAssignmentConflict {
		result.block(models.ReasonProviderAssignmentConflict)
	}
}

// checkVehicle evaluates a vehicle against the authority that owns it.
//
// Fleet holds no vehicle table, so there is no fleet-side half to fall back on
// and no possibility of a local answer. If the authority cannot be reached the
// decision is UNKNOWN and everything downstream blocks.
func (s *Service) checkVehicle(ctx context.Context, input EvaluateInput,
	policy *models.Policy, result *evaluation) {
	authority := s.opts.Vehicles
	if service, ok := platformclients.OwningService(strings.ToUpper(input.ServiceCategory)); ok {
		if candidate := s.opts.VehicleAuthorities[strings.ToLower(service)]; candidate != nil {
			authority = candidate
		}
	}
	if authority == nil || !authority.Configured() {
		result.blockUnknown(models.ReasonAuthorityNotConfigured)
		return
	}

	// The provider the vehicle must belong to comes from fleet's own
	// membership record, never from the request: otherwise a caller could name
	// whichever provider makes the ownership check pass.
	expectedProviderID := ""
	if input.MembershipID != "" {
		membership, err := s.opts.Decisions.MembershipState(ctx, nil,
			input.Actor.OrganizationID, input.MembershipID)
		if err != nil {
			result.blockUnknown(models.ReasonAuthorityUnavailable)
			return
		}
		if !membership.Found {
			result.block(models.ReasonProviderNotFound)
			return
		}
		if membership.ProviderID == "" {
			result.block(models.ReasonProviderNotLinked)
			return
		}
		expectedProviderID = membership.ProviderID
	}

	state, err := authority.LookupVehicle(ctx, platformclients.CallMeta{
		OrganizationID: input.Actor.OrganizationID,
		CorrelationID:  input.CorrelationID,
		CausationID:    input.CausationID,
	}, platformclients.VehicleLookup{
		VehicleID:           input.SubjectID,
		ExpectedProviderID:  expectedProviderID,
		ExpectedServiceType: input.ExpectedVehicleType,
	})
	if err != nil {
		if errors.Is(err, platformclients.ErrVehicleNotFound) {
			result.block(models.ReasonVehicleNotFound)
			return
		}
		result.source(models.DecisionSource{
			AuthorityService: authority.AuthorityService(),
			SourceKind:       sourceKindVehicle,
			SourceReference:  input.SubjectID,
			FreshnessState:   models.FreshnessUnknown,
			ObservedAt:       s.opts.Now().UTC(),
		})
		result.blockUnknown(models.ReasonAuthorityUnavailable)
		return
	}

	result.source(models.DecisionSource{
		AuthorityService: authority.AuthorityService(),
		SourceKind:       sourceKindVehicle,
		SourceReference:  state.VehicleID,
		SourceVersion:    state.SourceVersion,
		FreshnessState:   authorityFreshness(state.ObservedAt, policy.SourceMaxAge(), s.opts.Now().UTC()),
		ObservedAt:       s.opts.Now().UTC(),
	})

	if policy.RequiresVehicleCheck(models.CheckSourceVersion) && state.SourceVersion <= 0 {
		// An authority that cannot state a version for its own answer cannot
		// be checked for staleness later.
		result.blockStale(models.ReasonSourceVersionStale)
	}

	if policy.RequiresVehicleCheck(models.CheckOwnership) {
		if expectedProviderID == "" {
			// Ownership was required and there is nothing to check it against.
			result.block(models.ReasonVehicleOwnershipMismatch)
		} else if state.ProviderMatch == nil || !*state.ProviderMatch {
			result.block(models.ReasonVehicleOwnershipMismatch)
		}
	}
	if policy.RequiresVehicleCheck(models.CheckProviderAssociation) &&
		expectedProviderID != "" && state.ProviderID != expectedProviderID {
		result.block(models.ReasonVehicleProviderMismatch)
	}
	if policy.RequiresVehicleCheck(models.CheckServiceType) &&
		state.ServiceTypeMatch != nil && !*state.ServiceTypeMatch {
		result.block(models.ReasonVehicleServiceMismatch)
	}
	if policy.RequiresVehicleCheck(models.CheckVerification) &&
		state.VerificationState != platformclients.VehicleVerificationVerified {
		result.block(models.ReasonVehicleUnverified)
	}
	if policy.RequiresVehicleCheck(models.CheckAvailability) && !state.Available {
		result.block(models.ReasonVehicleNotAvailable)
	}
	if policy.RequiresVehicleCheck(models.CheckRestrictions) && state.Restricted {
		result.block(models.ReasonVehicleRestricted)
	}
	if policy.RequiresVehicleCheck(models.CheckDocuments) && !state.DocumentsSatisfied {
		for _, document := range state.RequiredDocuments {
			switch {
			case !document.Present:
				result.block(models.ReasonVehicleDocumentMissing)
			case document.Expired:
				result.block(models.ReasonVehicleDocumentExpired)
			}
		}
	}
	if policy.RequiresVehicleCheck(models.CheckAssignmentConflict) {
		if state.ActiveAssignmentConflict {
			result.block(models.ReasonVehicleAssignmentConflict)
		}
		// Fleet's own commitment of the same asset is a second, independent
		// source of conflict: the authority cannot see an assignment this
		// organization has made but not yet started.
		live, conflictErr := s.opts.Decisions.AssetHasLiveAssignment(ctx, nil,
			input.Actor.OrganizationID, assetReferenceFor(input, state),
			input.ExcludeAssignmentID)
		switch {
		case conflictErr != nil:
			result.blockUnknown(models.ReasonAuthorityUnavailable)
		case live:
			result.block(models.ReasonVehicleAssignmentConflict)
		}
	}
}

// assetReferenceFor picks the reference fleet records for this vehicle.
func assetReferenceFor(input EvaluateInput, state platformclients.VehicleAuthorityState) string {
	if input.AssetReference != "" {
		return input.AssetReference
	}
	return state.VehicleID
}

// authorityFreshness classifies how recent an authority's answer is.
func authorityFreshness(observedAt time.Time, maxAge time.Duration, now time.Time) string {
	if observedAt.IsZero() {
		return models.FreshnessUnknown
	}
	if now.Sub(observedAt.UTC()) > maxAge {
		return models.FreshnessStale
	}
	return models.FreshnessCurrent
}
