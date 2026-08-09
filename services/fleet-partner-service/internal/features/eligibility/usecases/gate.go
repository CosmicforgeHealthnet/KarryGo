package usecases

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/enforcement"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/models"
)

// Authorize implements the enforcement port.
//
// It reuses a current, unexpired ELIGIBLE decision over the same inputs and
// evaluates a fresh one otherwise, then refuses unless the decision authorizes.
// Domains therefore get the durable-decision semantics without knowing they
// exist: the call either returns a decision they may act on, or an error.
func (s *Service) Authorize(ctx context.Context,
	subject enforcement.Subject) (*models.Decision, error) {
	actorType := models.NormalizeActorType(subject.ActorType)
	return s.Require(ctx, EvaluateInput{
		Actor: Actor{
			OrganizationID: subject.OrganizationID,
			ActorType:      actorType,
			ActorID:        subject.ActorID,
		},
		OperationType:       subject.OperationType,
		SubjectType:         subject.SubjectType,
		SubjectID:           subject.SubjectID,
		BranchID:            subject.BranchID,
		MembershipID:        subject.MembershipID,
		ServiceCategory:     subject.ServiceCategory,
		ExpectedVehicleType: subject.ExpectedVehicleType,
		AssetReference:      subject.AssetReference,
		ExcludeAssignmentID: subject.ExcludeAssignmentID,
		CorrelationID:       subject.CorrelationID,
		CausationID:         subject.CausationID,
	})
}

// Compile-time proof that the service satisfies the enforcement port.
var _ enforcement.Gate = (*Service)(nil)
