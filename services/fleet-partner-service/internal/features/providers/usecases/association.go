package usecases

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// The provider's acceptance of an organization association (FLEET-10.2).
//
// The provider accepts or declines in their own app, against the owning
// service (Dispatch, Taxi or Hauling). Fleet never receives that request, so
// it learns the decision by READING the association back: the id is stored on
// the provisioning saga when the association is created, and the reconciler
// asks the owning service for its current state until the provider decides.
// Only an ACCEPTED association moves the membership to PENDING_SETUP (and
// requests the sub-wallet); only a DECLINED one moves it to DECLINED.

// requestAssociation asks the owning service for the association and records
// its id. The idempotency key is per membership, so repeating the request
// returns the same association in its current state.
func (s *Service) requestAssociation(ctx context.Context, membership *models.Membership, organizationName string,
	contract platformclients.ProviderContract, meta platformclients.CallMeta) (platformclients.ProviderAssociation, bool) {
	meta.OrganizationID = membership.OrganizationID
	association, err := contract.CreateAssociation(ctx, meta, platformclients.ProviderAssociationRequest{
		OrganizationID:   membership.OrganizationID,
		OrganizationName: organizationName,
		ProviderID:       *membership.ProviderID,
		MembershipID:     membership.ID,
		RelationshipType: membership.RelationshipType,
		IdempotencyKey:   "assoc:" + membership.ID,
	})
	if err != nil || association.AssociationID == "" {
		s.markSagaStep(ctx, membership.ID, "association_status", "UNAVAILABLE",
			models.SagaReconciliationRequired, "association_unavailable")
		return platformclients.ProviderAssociation{}, false
	}
	if err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		return s.opts.Providers.SetSagaAssociation(ctx, tx, membership.ID, association.AssociationID, association.SourceVersion)
	}); err != nil {
		s.markSagaStep(ctx, membership.ID, "association_status", "UNKNOWN_OUTCOME",
			models.SagaReconciliationRequired, "association_persistence_failed")
		return platformclients.ProviderAssociation{}, false
	}
	s.markSagaStep(ctx, membership.ID, "association_status", "AWAITING_PROVIDER", models.SagaInProgress, "")
	return association, true
}

// SyncAssociation reads the provider's decision back from the owning service
// and applies it. It returns true when it changed anything.
//
// It is safe to call repeatedly: a decision already applied is not applied
// again, an undecided association changes nothing, and a missing association
// is (re)requested with the same idempotency key.
func (s *Service) SyncAssociation(ctx context.Context, organizationID string, membershipID string) (bool, error) {
	membership, err := s.opts.Providers.GetMembership(ctx, organizationID, membershipID)
	if err != nil {
		return false, err
	}
	if membership.ProviderID == nil || membership.Status != models.StatusPendingProviderAcceptance {
		return false, nil
	}
	contract, ok := s.opts.Contracts.Providers[membership.ServiceCategory]
	if !ok {
		return false, nil
	}
	saga, err := s.opts.Providers.GetSaga(ctx, organizationID, membershipID)
	if err != nil {
		return false, err
	}
	meta := platformclients.CallMeta{OrganizationID: organizationID,
		CorrelationID: "provider-association:" + membershipID}

	progressed := false
	var association platformclients.ProviderAssociation
	if saga.AssociationID == "" {
		organizationName, _, _ := s.opts.Organizations.OrganizationStatus(ctx, organizationID)
		var created bool
		association, created = s.requestAssociation(ctx, membership, organizationName, contract, meta)
		if !created {
			return false, nil
		}
		progressed = true
	} else {
		association, err = contract.ReadAssociation(ctx, meta, saga.AssociationID)
		if err != nil {
			var clientErr *platformclients.Error
			if errors.As(err, &clientErr) && clientErr.Outcome == platformclients.OutcomeNotFound {
				s.markSagaStep(ctx, membershipID, "association_status", "FAILED",
					models.SagaReconciliationRequired, "association_not_found")
			}
			return false, nil
		}
		if association.SourceVersion > saga.AssociationSourceVersion {
			_ = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
				return s.opts.Providers.SetSagaAssociation(ctx, tx, membershipID, saga.AssociationID, association.SourceVersion)
			})
		}
	}

	switch association.Status {
	case platformclients.AssociationAccepted:
		s.markSagaStep(ctx, membershipID, "association_status", "ACCEPTED", models.SagaInProgress, "")
		if err := s.RecordAssociationOutcome(ctx, organizationID, membershipID, true, meta.CorrelationID); err != nil {
			return progressed, err
		}
		if current, err := s.opts.Providers.GetMembership(ctx, organizationID, membershipID); err == nil &&
			current.Status == models.StatusActive {
			s.markSagaStep(ctx, membershipID, "association_status", "ACCEPTED", models.SagaCompleted, "")
		}
		return true, nil
	case platformclients.AssociationDeclined:
		if err := s.RecordAssociationOutcome(ctx, organizationID, membershipID, false, meta.CorrelationID); err != nil {
			return progressed, err
		}
		s.markSagaStep(ctx, membershipID, "association_status", "DECLINED", models.SagaFailedPermanent, "provider_declined")
		return true, nil
	}
	return progressed, nil
}
