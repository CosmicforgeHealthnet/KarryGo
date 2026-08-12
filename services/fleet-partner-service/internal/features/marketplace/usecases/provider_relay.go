package usecases

import (
	"context"
	"errors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
)

// AcceptAssignmentForProvider records an acceptance relayed by the execution
// service that authenticated the provider.
//
// The relay carries the provider's platform identity, not their organization
// membership — the owning service knows who the provider is, but nothing about
// this organization's internal records. Resolving membership here, from the
// assignment itself, is also the ownership check: an assignment can only be
// answered by the provider it was actually issued to.
func (s *Service) AcceptAssignmentForProvider(ctx context.Context, input AcceptAssignmentInput) (*AssignmentResult, error) {
	resolved, err := s.bindProviderToAssignment(ctx, input.AssignmentID, input.Actor)
	if err != nil {
		return nil, err
	}
	input.Actor = resolved
	return s.AcceptAssignment(ctx, input)
}

// DeclineAssignmentForProvider records a decline relayed by the owning service.
func (s *Service) DeclineAssignmentForProvider(ctx context.Context, input DeclineAssignmentInput) (*AssignmentResult, error) {
	resolved, err := s.bindProviderToAssignment(ctx, input.AssignmentID, input.Actor)
	if err != nil {
		return nil, err
	}
	input.Actor = resolved
	return s.DeclineAssignment(ctx, input)
}

// bindProviderToAssignment resolves and verifies the asserted provider.
//
// Three things must all hold, and each rules out a distinct attack:
//   - the assignment exists (otherwise there is nothing to answer);
//   - it belongs to the same source service the caller speaks for, so one
//     execution service cannot answer for another's provider;
//   - its executing provider is exactly the asserted provider, so a valid
//     service signature cannot be used to answer on someone else's behalf.
func (s *Service) bindProviderToAssignment(ctx context.Context, assignmentID string,
	actor ProviderActor) (ProviderActor, error) {
	assignment, err := s.opts.Assignments.GetByIDUnscoped(ctx, assignmentID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return ProviderActor{}, errorcodes.NotFound(errorcodes.AssignmentNotFound,
				"Assignment not found.")
		}
		return ProviderActor{}, err
	}

	if actor.SourceService != "" && assignment.SourceService != actor.SourceService {
		return ProviderActor{}, errorcodes.Forbidden(errorcodes.AssignmentNotOwnedByProvider,
			"This assignment belongs to another service.")
	}
	// The decisive check: the asserted provider must be the assigned provider.
	if assignment.ExecutingProviderID == "" ||
		assignment.ExecutingProviderID != actor.ExecutingProviderID {
		return ProviderActor{}, errorcodes.Forbidden(errorcodes.AssignmentNotOwnedByProvider,
			"This assignment does not belong to that provider.")
	}

	return ProviderActor{
		MembershipID:        assignment.MembershipID,
		ExecutingProviderID: assignment.ExecutingProviderID,
		SourceService:       assignment.SourceService,
		OrganizationID:      assignment.OrganizationID,
	}, nil
}
