package referralusecases

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	referralmodels "cosmicforge/logistics/services/referral-service/internal/features/referrals/models"
	"cosmicforge/logistics/shared/go/referralcontract"
)

// checkMilestones grants a referrer each milestone their count of qualified
// customer referrals has reached. Only referrals in good standing count, so a
// disqualified or reversed referral drops out of the count; each milestone is
// still granted once, ever, and is not taken back automatically if the count
// later falls (an operator can reverse its reward).
//
// A milestone is reached when its Nth referral qualified. Its reward is
// referral income like any other, so it is only paid if the referrer was
// active at that moment; the Power User badge recognises the count itself and
// is kept either way.
func (s *Service) checkMilestones(ctx context.Context, tx pgx.Tx, referrerPersonID string) error {
	count, err := s.store.CountQualifiedCustomerReferrals(ctx, tx, referrerPersonID)
	if err != nil {
		return err
	}
	for _, milestone := range s.rules.Milestones() {
		if count < milestone.Referrals {
			return nil
		}
		reachedAt, err := s.store.NthQualificationTime(ctx, tx, referrerPersonID, milestone.Referrals)
		if err != nil {
			return err
		}
		inserted, err := s.store.InsertMilestone(ctx, tx, referrerPersonID, milestone.Key, reachedAt)
		if err != nil {
			return err
		}
		if !inserted {
			continue
		}
		if milestone.Badge {
			if err := s.store.SetPowerUser(ctx, tx, referrerPersonID, reachedAt); err != nil {
				return err
			}
		}
		if milestone.AmountKobo <= 0 {
			continue
		}
		beneficiaryType, beneficiaryID, err := s.beneficiaryFor(ctx, tx, referrerPersonID, milestone.Balance)
		if err != nil {
			return err
		}
		asOf := reachedAt
		decideAfter := s.now().Add(s.rules.ActivityDecisionDelay)
		reward := referralmodels.Reward{
			ID: "rw_" + uuid.NewString(), PersonID: referrerPersonID,
			BeneficiaryActorType: beneficiaryType, BeneficiaryActorID: beneficiaryID,
			Model: referralmodels.ModelCustomer, RewardType: milestone.RewardType, Balance: milestone.Balance,
			AmountKobo: milestone.AmountKobo, Status: referralmodels.RewardPendingActivity,
			ActivityAsOf: &asOf, DecideAfter: &decideAfter,
			IdempotencyKey: referralmodels.RewardKey(milestone.RewardType, referrerPersonID),
		}
		granted, err := s.grant(ctx, tx, reward)
		if err != nil {
			return err
		}
		if granted {
			if err := s.store.SetMilestoneReward(ctx, tx, referrerPersonID, milestone.Key, reward.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// beneficiaryFor picks the account a person's reward is paid into. Earnings
// go to the account that first received their code, like all earnings.
// Spend-only credit can only be spent on bookings, so it goes to their
// customer account when they have one.
func (s *Service) beneficiaryFor(ctx context.Context, tx pgx.Tx, personID, balance string) (string, string, error) {
	person, err := s.store.GetPerson(ctx, tx, personID, false)
	if err != nil {
		return "", "", err
	}
	if balance == referralmodels.BalancePromotional && person.HomeActorType != referralcontract.ActorCustomer {
		actors, err := s.store.ActorsForPerson(ctx, tx, personID)
		if err != nil {
			return "", "", err
		}
		for _, actor := range actors {
			if actor.ActorType == referralcontract.ActorCustomer {
				return actor.ActorType, actor.ActorID, nil
			}
		}
	}
	return person.HomeActorType, person.HomeActorID, nil
}
