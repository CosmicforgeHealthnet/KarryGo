package app

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"

	marketusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/usecases"
	private "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/models"
	privaterepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
)

type privateAcceptanceReader struct{ repo *privaterepos.Repository }

func NewPrivateAcceptanceReader(pool *pgxpool.Pool) marketusecases.PrivateAcceptanceReader {
	return &privateAcceptanceReader{repo: privaterepos.NewRepository(pool)}
}

func (r *privateAcceptanceReader) VerifyPrivateAcceptance(ctx context.Context,
	assignmentID string) (*marketusecases.ProviderAcceptanceView, error) {
	a, err := r.repo.GetAssignmentByID(ctx, nil, assignmentID)
	if errors.Is(err, privaterepos.ErrNotFound) {
		return nil, errorcodes.Conflict(errorcodes.FleetAssignmentNotFound, "The assignment was not found.")
	}
	if err != nil {
		return nil, err
	}
	if a.Status != private.StatusAccepted && a.Status != private.StatusReady {
		return nil, errorcodes.Conflict(errorcodes.ProviderAcceptanceNotConfirmed,
			"The private assignment is not ready for execution.")
	}
	d, err := r.repo.GetDecisionByAssignment(ctx, nil, assignmentID)
	if errors.Is(err, privaterepos.ErrNotFound) || d.Decision != "ACCEPT" {
		return nil, errorcodes.Conflict(errorcodes.ProviderAcceptanceNotConfirmed,
			"The provider did not accept this private assignment.")
	}
	if err != nil {
		return nil, err
	}
	acceptedAt := d.DecidedAt
	return &marketusecases.ProviderAcceptanceView{
		AssignmentID: a.ID, OrganizationID: a.OrganizationID, ProviderID: a.ProviderID,
		ProviderService: a.ExecutionService, OrganizationMembershipID: a.ProviderMembershipID,
		ProviderResponseID: d.ID, ResponseStatus: "ACCEPTED", AcceptedAt: &acceptedAt,
		AssignmentStatus: a.Status, AssignmentVersion: a.Version,
		SourceService: "fleet-partner-service",
	}, nil
}
