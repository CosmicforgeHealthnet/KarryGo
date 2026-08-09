// Package usecases implements marketplace acceptance, assignment,
// provider Accept/Decline, reassignment and trip projection.
package usecases

import (
	"context"
	"errors"
	"time"

	providermodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// ErrMembershipNotFound is returned by a MembershipReader when the membership
// does not exist or belongs to another organization.
//
// The marketplace feature owns this sentinel rather than reusing the providers
// repository error, so the two features stay coupled only through this port.
var ErrMembershipNotFound = errors.New("marketplace: managed provider membership not found")

// MembershipReader is the marketplace feature's port onto managed provider
// memberships.
//
// Marketplace must not reach into the providers feature's repository package.
// The composition root supplies an adapter that satisfies this interface and
// translates the providers repository's not-found error into
// ErrMembershipNotFound.
type MembershipReader interface {
	GetMembership(ctx context.Context, organizationID string, membershipID string) (*providermodels.Membership, error)
}

// MembershipHistoryReader is the marketplace feature's port onto a
// provider's own membership history across organizations and time --
// distinct from MembershipReader above, which looks up exactly one
// already-known membership by ID rather than listing every membership a
// provider identity has ever held.
//
// Marketplace must not reach into the providers feature's repository
// package. The composition root supplies an adapter that satisfies this
// interface.
type MembershipHistoryReader interface {
	ListByProvider(ctx context.Context, providerService string, providerID string) ([]providermodels.Membership, error)
}

// EligibilityDecision is the outcome of a full eligibility recheck.
type EligibilityDecision struct {
	Eligible   bool
	ReasonCode string
	Detail     string
}

func ineligible(code string, detail string) EligibilityDecision {
	return EligibilityDecision{Eligible: false, ReasonCode: code, Detail: detail}
}

// ShiftEligibility is the workforce port.
//
// The workforce module arrives in a later work package. Until then this
// interface lets an organization that has switched shift enforcement ON fail
// CLOSED rather than silently skipping the check.
type ShiftEligibility interface {
	// HasValidShift reports whether the provider has an active shift with
	// enough remaining time and no approved leave conflict.
	HasValidShift(ctx context.Context, organizationID string, membershipID string, requiredUntil time.Time) (bool, string, error)
	// Enforced reports whether this organization requires shift checks.
	Enforced(ctx context.Context, organizationID string) (bool, error)
}

// UnavailableShiftEligibility is the default port implementation.
//
// It reports "not enforced" for organizations that have not turned shift
// enforcement on, and fails closed for those that have — the workforce module
// that would answer properly does not exist yet.
type UnavailableShiftEligibility struct{}

// Enforced reports false: no organization can enable enforcement until the
// workforce module exists to answer it.
func (UnavailableShiftEligibility) Enforced(context.Context, string) (bool, error) {
	return false, nil
}

// HasValidShift fails closed if it is ever consulted.
func (UnavailableShiftEligibility) HasValidShift(context.Context, string, string, time.Time) (bool, string, error) {
	return false, "shift_enforcement_unavailable", nil
}

// EligibilityChecker performs the full recheck required before an assignment
// is created and again before a provider's acceptance is recorded.
//
// The recheck runs twice deliberately: eligibility at assignment time does not
// prove eligibility at response time, and the provider may have gone offline,
// lost verification, or started other work in between.
type EligibilityChecker struct {
	Providers  MembershipReader
	Executions platformclients.ExecutionSet
	Shifts     ShiftEligibility
}

// NewEligibilityChecker builds the checker.
func NewEligibilityChecker(providers MembershipReader, executions platformclients.ExecutionSet, shifts ShiftEligibility) *EligibilityChecker {
	if shifts == nil {
		shifts = UnavailableShiftEligibility{}
	}
	return &EligibilityChecker{Providers: providers, Executions: executions, Shifts: shifts}
}

// MembershipInput carries what the caller already knows.
type MembershipInput struct {
	OrganizationID  string
	MembershipID    string
	SourceService   string
	ServiceCategory string
	SourceRequestID string
	AssetReference  string
	RequiredUntil   time.Time
	// SkipSourceCheck is used when the source service has just answered (for
	// example inside a reservation flow) to avoid a redundant round trip.
	SkipSourceCheck bool
}

// CheckMembership validates the organization-side eligibility of a provider.
func (c *EligibilityChecker) CheckMembership(ctx context.Context, input MembershipInput) (*providermodels.Membership, EligibilityDecision, error) {
	membership, err := c.Providers.GetMembership(ctx, input.OrganizationID, input.MembershipID)
	if err != nil {
		if errors.Is(err, ErrMembershipNotFound) {
			return nil, ineligible(string(errorcodes.ProviderNotEligible), "membership not found"), nil
		}
		return nil, EligibilityDecision{}, err
	}

	// Membership must be active — not invited, pending, suspended, ending or
	// removed.
	if membership.Status != providermodels.StatusActive {
		return membership, ineligible(string(errorcodes.ProviderMembershipNotActive),
			"membership status is "+membership.Status), nil
	}
	if membership.AssociationAcceptedAt == nil {
		return membership, ineligible(string(errorcodes.ProviderAssociationPending),
			"provider has not accepted the organization association"), nil
	}
	// The provider must belong to the service that owns this request.
	if input.ServiceCategory != "" && membership.ServiceCategory != input.ServiceCategory {
		return membership, ineligible(string(errorcodes.ProviderNotEligible),
			"provider service category does not match the request"), nil
	}
	if membership.ProviderID == nil || *membership.ProviderID == "" {
		return membership, ineligible(string(errorcodes.ProviderProvisioningIncomplete),
			"provider identity is not provisioned"), nil
	}

	// Shift enforcement, when the organization has switched it on.
	enforced, err := c.Shifts.Enforced(ctx, input.OrganizationID)
	if err != nil {
		return membership, EligibilityDecision{}, err
	}
	if enforced {
		valid, reason, err := c.Shifts.HasValidShift(ctx, input.OrganizationID, input.MembershipID, input.RequiredUntil)
		if err != nil {
			return membership, EligibilityDecision{}, err
		}
		if !valid {
			return membership, ineligible(string(errorcodes.ProviderShiftInvalid), reason), nil
		}
	}

	return membership, EligibilityDecision{Eligible: true}, nil
}

// CheckSource asks the owning execution service whether the provider and
// vehicle may take this work.
//
// Vehicle verification, licence status and platform suspension stay owned by
// the execution service — organization sponsorship never bypasses them.
func (c *EligibilityChecker) CheckSource(ctx context.Context, meta platformclients.CallMeta, input MembershipInput, providerID string) (EligibilityDecision, error) {
	if input.SkipSourceCheck {
		return EligibilityDecision{Eligible: true}, nil
	}
	contract, ok := c.Executions.For(input.ServiceCategory)
	if !ok || !contract.Configured() {
		// No live contract: fail closed rather than assuming the provider is
		// allowed to work.
		return ineligible(string(errorcodes.SourceServiceUnavailable),
			"execution service contract is not available"), nil
	}

	eligibility, err := contract.ValidateProviderEligibility(ctx, meta, input.SourceRequestID, providerID, input.AssetReference)
	if err != nil {
		return EligibilityDecision{}, err
	}
	if !eligibility.Eligible {
		code := eligibility.ReasonCode
		if code == "" {
			code = string(errorcodes.ProviderNotEligible)
		}
		return ineligible(code, "execution service rejected eligibility"), nil
	}
	if input.AssetReference != "" && !eligibility.VehicleOK {
		return ineligible(string(errorcodes.VehicleNotEligible), "vehicle is not eligible"), nil
	}
	return EligibilityDecision{Eligible: true}, nil
}

// CheckNoConflictingWork verifies the provider is not already holding live
// capacity for other work.
func (c *EligibilityChecker) CheckNoConflictingWork(ctx context.Context, counter ReservationCounter, membershipID string, excludeAssignmentID string) (EligibilityDecision, error) {
	held, err := counter.CountHeldExcluding(ctx, "PROVIDER", membershipID, excludeAssignmentID)
	if err != nil {
		return EligibilityDecision{}, err
	}
	if held > 0 {
		return ineligible(string(errorcodes.ProviderHasConflictingWork),
			"provider already holds another live assignment"), nil
	}
	return EligibilityDecision{Eligible: true}, nil
}

// ReservationCounter reports live capacity holds.
type ReservationCounter interface {
	CountHeldExcluding(ctx context.Context, resourceType string, resourceID string, excludeAssignmentID string) (int, error)
}
