package usecases

import (
	"context"
	"errors"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
)

// ProviderAcceptanceView is what dispatch is told about a provider's decision.
//
// It is deliberately minimal. Dispatch needs exactly enough to decide whether
// creating a canonical trip is legitimate; it does not need the organization's
// commercial position, the decline catalogue, or anything about the customer.
// Every field here is either an identifier dispatch must match against its own
// command, or a status it must gate on.
type ProviderAcceptanceView struct {
	AssignmentID             string     `json:"assignmentId"`
	OrganizationID           string     `json:"organizationId"`
	ProviderID               string     `json:"providerId"`
	ProviderService          string     `json:"providerService"`
	OrganizationMembershipID string     `json:"organizationMembershipId"`
	ProviderResponseID       string     `json:"providerResponseId"`
	ResponseStatus           string     `json:"responseStatus"`
	AcceptedAt               *time.Time `json:"acceptedAt"`
	AssignmentStatus         string     `json:"assignmentStatus"`
	AssignmentVersion        int64      `json:"assignmentVersion"`
	SourceService            string     `json:"sourceService"`
	SourceRequestID          string     `json:"sourceRequestId"`
	SourceBookingID          string     `json:"sourceBookingId"`
}

// ExecutableAssignmentStatuses are the states in which a canonical trip may be
// created. Anything else means the assignment is not (or is no longer) live
// work: cancelled, declined, expired or already completed.
var ExecutableAssignmentStatuses = map[string]bool{
	models.AssignmentAccepted:              true,
	models.AssignmentExecutionStartPending: true,
	models.AssignmentReadyForExecution:     true,
}

// VerifyProviderAcceptance answers dispatch's question: did this provider
// actually accept, and is that acceptance still valid right now?
//
// This is the authoritative answer to a question dispatch cannot answer
// locally. Dispatch holds a provider_response_id column, but a column value
// only proves that somebody supplied a string. The provider's actual decision
// lives here, and this is the only place that can confirm it.
func (s *Service) VerifyProviderAcceptance(ctx context.Context, assignmentID string) (*ProviderAcceptanceView, error) {
	assignment, err := s.opts.Assignments.GetByIDUnscoped(ctx, assignmentID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return nil, errorcodes.Conflict(errorcodes.FleetAssignmentNotFound,
				"The assignment was not found.")
		}
		return nil, err
	}

	// The assignment must currently be in a state where execution is allowed.
	// Checking the CURRENT status — not a status captured when the provider
	// answered — is what makes a later cancellation visible here.
	if !ExecutableAssignmentStatuses[assignment.Status] {
		switch assignment.Status {
		case models.AssignmentCancelled:
			return nil, errorcodes.Conflict(errorcodes.ProviderAcceptanceCancelled,
				"This assignment has been cancelled.")
		case models.AssignmentExpired:
			return nil, errorcodes.Conflict(errorcodes.ProviderAcceptanceExpired,
				"This assignment expired before it could start.")
		default:
			return nil, errorcodes.Conflict(errorcodes.ProviderAcceptanceNotConfirmed,
				"This assignment is not ready for execution.")
		}
	}

	response, err := s.opts.Assignments.GetResponse(ctx, nil, assignment.ID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			// No response row at all: the provider has not answered. This is
			// the case that must never be mistaken for acceptance.
			return nil, errorcodes.Conflict(errorcodes.ProviderAcceptanceNotConfirmed,
				"The provider has not responded to this assignment.")
		}
		return nil, err
	}
	if response.Response != models.ResponseAccepted {
		return nil, errorcodes.Conflict(errorcodes.ProviderAcceptanceNotConfirmed,
			"The provider did not accept this assignment.")
	}

	projection, err := s.opts.Projections.GetByID(ctx, assignment.OrganizationID,
		assignment.RequestProjectionID)
	if err != nil {
		return nil, err
	}

	view := &ProviderAcceptanceView{
		AssignmentID:             assignment.ID,
		OrganizationID:           assignment.OrganizationID,
		ProviderID:               assignment.ExecutingProviderID,
		ProviderService:          providerServiceFor(assignment.SourceService),
		OrganizationMembershipID: assignment.MembershipID,
		ProviderResponseID:       response.ID,
		ResponseStatus:           models.ResponseAccepted,
		AcceptedAt:               &response.OccurredAt,
		AssignmentStatus:         assignment.Status,
		AssignmentVersion:        assignment.Version,
		SourceService:            assignment.SourceService,
		SourceRequestID:          projection.PlatformRequestID,
		SourceBookingID:          orEmpty(projection.PlatformBookingID),
	}
	return view, nil
}

func orEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// providerServiceFor maps a source service onto the provider-service token
// dispatch asserts against, so a taxi provider can never be presented as a
// dispatch provider.
func providerServiceFor(sourceService string) string {
	switch sourceService {
	case models.SourceDispatch:
		return "DISPATCH"
	case models.SourceTaxi:
		return "TAXI"
	case models.SourceHauling:
		return "HAULING"
	default:
		return "UNKNOWN"
	}
}
