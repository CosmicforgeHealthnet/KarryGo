package usecases

import (
	"context"
	"errors"
	"strings"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/providerreceipt"
	"github.com/jackc/pgx/v5"
)

func offerIntent(input OfferResponseInput, decision string) providerreceipt.Intent {
	intent := providerreceipt.Intent{OfferID: input.OfferID, ProviderID: input.Actor.ExecutingProviderID, ClientResponseID: input.ClientResponseID, Decision: decision, ReasonCode: input.ReasonCode, Note: input.Note, Evidence: input.EvidenceFileIDs}
	if decision == models.ResponseAccepted {
		intent.ReservationID = input.DispatchReservationID
		intent.RequestVersion = input.DispatchRequestVersion
	}
	return intent
}

func validOfferDecisionIdentity(input OfferResponseInput) bool {
	return input.ClientResponseID != "" && len(input.ClientResponseID) <= 128 && input.ClientResponseID == strings.TrimSpace(input.ClientResponseID) && input.Actor.ExecutingProviderID != "" && len(input.Actor.ExecutingProviderID) <= 128 && (input.Actor.SourceService == models.SourceDispatch || input.Actor.SourceService == models.SourceTaxi || input.Actor.SourceService == models.SourceHauling)
}

func (s *Service) recordOfferReceipt(ctx context.Context, tx pgx.Tx, offer *models.Offer, recipient *models.OfferRecipient, input OfferResponseInput, decision string) error {
	_, err := tx.Exec(ctx, `INSERT INTO fleet_partner_provider_offer_decision_receipts(id,organization_id,offer_id,recipient_id,membership_id,provider_id,source_service,client_response_id,decision,intent_digest) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, ids.New("por"), offer.OrganizationID, offer.ID, recipient.ID, recipient.MembershipID, input.Actor.ExecutingProviderID, input.Actor.SourceService, input.ClientResponseID, decision, offerIntent(input, decision).Digest())
	return err
}

func (s *Service) ProviderOfferDecisionReceipt(ctx context.Context, offerID, providerID, sourceService, key string) (*providerreceipt.Receipt, error) {
	input := OfferResponseInput{OfferID: offerID, ClientResponseID: key, Actor: ProviderActor{ExecutingProviderID: providerID, SourceService: sourceService}}
	if !validOfferDecisionIdentity(input) || offerID == "" || len(offerID) > 128 {
		return nil, apperrors.BadRequest("Offer decision identity is invalid.", nil)
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var out providerreceipt.Receipt
	err := s.opts.Pool.QueryRow(ctx, `SELECT r.id,r.offer_id,r.provider_id,r.client_response_id,r.decision,r.intent_digest,r.recorded_at FROM fleet_partner_provider_offer_decision_receipts r JOIN fleet_partner_organization_request_offers o ON o.id=r.offer_id AND o.organization_id=r.organization_id WHERE r.offer_id=$1 AND r.provider_id=$2 AND r.source_service=$3 AND r.client_response_id=$4`, offerID, providerID, sourceService, key).Scan(&out.ReceiptID, &out.OfferID, &out.ProviderID, &out.ClientResponseID, &out.Decision, &out.IntentDigest, &out.RecordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperrors.NotFound("Offer decision receipt not found.", nil)
	}
	if err != nil {
		return nil, err
	}
	out.ContractVersion = providerreceipt.OfferVersion
	return &out, nil
}

func offerReplayConflict() error {
	return errorcodes.Conflict(errorcodes.IdempotencyConflictCode, "The original offer decision cannot be replayed with different or unproven intent.")
}
