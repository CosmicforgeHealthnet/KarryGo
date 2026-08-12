package usecases

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/enforcement"
	eligibilitymodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// The durable-decision gate for marketplace operations.
//
// The marketplace feature already has its own eligibility checker, and it
// stays: it answers the immediate operational question quickly and with
// marketplace-specific reasons. What it does not do is leave a record. A month
// later, "why was this provider allowed on that job?" has no answer, and
// "which authority said the vehicle was verified, at what version?" has none
// either.
//
// These helpers add that record. They run in addition to the existing checks,
// not instead of them, and they refuse on exactly the same principle: an
// answer nobody can currently vouch for blocks.

// gateProviderOperation records and enforces a durable decision about a
// managed provider taking work.
func (s *Service) gateProviderOperation(ctx context.Context, tenantCtx tenant.Context,
	membershipID string, serviceCategory string, operationType string,
	excludeAssignmentID string) error {
	subject := enforcement.Provider(tenantCtx.OrganizationID, membershipID, operationType)
	subject.ServiceCategory = serviceCategory
	subject.ExcludeAssignmentID = excludeAssignmentID
	subject.ActorType = actorTypeFor(tenantCtx)
	subject.ActorID = tenantCtx.ActorID
	subject.CorrelationID = tenantCtx.CorrelationID
	_, err := enforcement.Require(ctx, s.opts.DurableEligibility, subject)
	return err
}

// gateVehicleOperation records and enforces a durable decision about a vehicle
// being committed to work.
//
// It is a no-op only when no vehicle was nominated. That is not a skipped
// check: dispatch work legitimately runs without a named vehicle, and there is
// nothing to decide about a vehicle that does not exist. The moment a caller
// names one, the authority is asked and an unreachable authority blocks.
func (s *Service) gateVehicleOperation(ctx context.Context, tenantCtx tenant.Context,
	assetReference string, membershipID string, operationType string,
	excludeAssignmentID string) error {
	if assetReference == "" {
		return nil
	}
	subject := enforcement.Vehicle(tenantCtx.OrganizationID, assetReference,
		membershipID, operationType)
	subject.ExcludeAssignmentID = excludeAssignmentID
	subject.ActorType = actorTypeFor(tenantCtx)
	subject.ActorID = tenantCtx.ActorID
	subject.CorrelationID = tenantCtx.CorrelationID
	_, err := enforcement.Require(ctx, s.opts.DurableEligibility, subject)
	return err
}

// gateAssignment gates both halves of putting a provider and a vehicle on
// work. The provider is checked first: a vehicle question about a provider who
// may not work at all is a question that did not need asking.
func (s *Service) gateAssignment(ctx context.Context, tenantCtx tenant.Context,
	membershipID string, assetReference string, serviceCategory string,
	operationType string, excludeAssignmentID string) error {
	if err := s.gateProviderOperation(ctx, tenantCtx, membershipID, serviceCategory,
		operationType, excludeAssignmentID); err != nil {
		return err
	}
	return s.gateVehicleOperation(ctx, tenantCtx, assetReference, membershipID,
		operationType, excludeAssignmentID)
}

// gateExecution gates the provider-side transitions that start real work:
// acceptance, departure, arrival, pickup and trip start.
//
// The same durable decision covers all of them under the EXECUTION policy,
// because they are the same question asked at successive moments — "may this
// person, on this vehicle, still be doing this job?" — and the decision's own
// expiry is what makes asking again meaningful.
func (s *Service) gateExecution(ctx context.Context, assignment *models.Assignment,
	actorType string, actorID string, correlationID string) error {
	tenantCtx := tenant.Context{
		OrganizationID: assignment.OrganizationID,
		ActorType:      tenant.ActorType(actorType),
		ActorID:        actorID,
		CorrelationID:  correlationID,
	}
	if err := s.gateProviderOperation(ctx, tenantCtx, assignment.MembershipID,
		assignment.SourceService, eligibilitymodels.OperationExecution,
		assignment.ID); err != nil {
		return err
	}
	return s.gateVehicleOperation(ctx, tenantCtx, stringValue(assignment.AssetReference),
		assignment.MembershipID, eligibilitymodels.OperationExecution, assignment.ID)
}

// actorTypeFor maps the resolved tenant actor onto the eligibility vocabulary.
func actorTypeFor(tenantCtx tenant.Context) string {
	switch tenantCtx.ActorType {
	case tenant.ActorTypeOrganizationUser:
		return eligibilitymodels.ActorOrganizationUser
	case tenant.ActorTypePlatformService:
		return eligibilitymodels.ActorPlatformService
	default:
		return eligibilitymodels.ActorSystem
	}
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
