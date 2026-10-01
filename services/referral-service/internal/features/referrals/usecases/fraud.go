package referralusecases

import (
	"context"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5"

	referralmodels "cosmicforge/logistics/services/referral-service/internal/features/referrals/models"
	referralrepositories "cosmicforge/logistics/services/referral-service/internal/features/referrals/repositories"
)

// holdReferral puts a referral under review. Nothing it has earned is paid
// until an operator clears it: approved rewards still queued for the wallet
// are frozen, and later decisions freeze instead of paying. A flag records
// why (once, however often the same evidence turns up).
func (s *Service) holdReferral(ctx context.Context, tx pgx.Tx, referral referralmodels.Referral, flagType string, evidence map[string]any) error {
	if referral.Status == referralmodels.ReferralActive {
		if err := s.store.SetReferralStatus(ctx, tx, referral.ID, referralmodels.ReferralUnderReview, flagType); err != nil {
			return err
		}
		queued, err := s.store.RewardsForReferral(ctx, tx, referral.ID, []string{referralmodels.RewardAvailable})
		if err != nil {
			return err
		}
		if err := s.freezeQueued(ctx, tx, queued); err != nil {
			return err
		}
	}
	return s.flagOnce(ctx, tx, referral.ReferredPersonID, referral.ID, flagType, evidence)
}

// freezeQueued holds approved rewards that have not reached the wallet yet.
// A reward being posted at this very moment completes; the hold stops
// everything after it. Take-backs are never frozen: holding one would let
// its owner keep money that is being recovered.
func (s *Service) freezeQueued(ctx context.Context, tx pgx.Tx, rewards []referralmodels.Reward) error {
	now := s.now()
	for _, reward := range rewards {
		if reward.AmountKobo <= 0 || reward.Status != referralmodels.RewardAvailable ||
			reward.WalletState != referralmodels.WalletPending || reward.Leased(now) {
			continue
		}
		if err := s.store.SetRewardStatus(ctx, tx, reward.ID, referralmodels.RewardFrozen, referralmodels.ReasonUnderReview, now); err != nil {
			return err
		}
	}
	return nil
}

// flagOnce raises a review flag about a referral unless one of the same kind
// is already open.
func (s *Service) flagOnce(ctx context.Context, tx pgx.Tx, personID, referralID, flagType string, evidence map[string]any) error {
	open, err := s.store.HasOpenFlag(ctx, tx, referralID, flagType)
	if err != nil || open {
		return err
	}
	return s.store.RaiseFlag(ctx, tx, personID, referralID, flagType, referralmodels.SeverityReview, evidence)
}

// collusive reports a paid job whose other party is the referrer, or the
// referred person on both sides: a booking the two could have arranged to
// mint a reward. Such a job never qualifies a referral or earns commission.
// An account whose announcement has not arrived yet cannot be matched and is
// given the benefit of the doubt.
func (s *Service) collusive(ctx context.Context, tx pgx.Tx, referral referralmodels.Referral, counterpartyType, counterpartyID string) (bool, error) {
	if counterpartyType == "" || counterpartyID == "" {
		return false, nil
	}
	counterparty, err := s.store.GetActor(ctx, tx, counterpartyType, counterpartyID)
	if errors.Is(err, referralrepositories.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return counterparty.PersonID == referral.ReferrerPersonID || counterparty.PersonID == referral.ReferredPersonID, nil
}

func (s *Service) flagCollusion(ctx context.Context, tx pgx.Tx, referral referralmodels.Referral, job referralmodels.SettledJob) error {
	return s.flagOnce(ctx, tx, referral.ReferredPersonID, referral.ID, referralmodels.FlagCollusiveJob, map[string]any{
		"source_service": job.SourceService, "source_reference": job.SourceReference,
		"customer_id": job.CustomerID, "provider_actor_type": job.ProviderActorType, "provider_id": job.ProviderID,
	})
}

// reviewSharedDevice looks at a sign-up from a device other people have
// used. Sharing it with the person's own referrer, or with someone else that
// referrer brought in, is the pattern of one person farming referrals with
// extra identities: the referral is held for review. Any other sharing (a
// family phone) is only flagged.
func (s *Service) reviewSharedDevice(ctx context.Context, tx pgx.Tx, person referralmodels.Person, actor referralmodels.Actor, sharedWith []string) error {
	if len(sharedWith) == 0 {
		return nil
	}
	evidence := map[string]any{"actor_type": actor.ActorType, "actor_id": actor.ActorID, "shared_with_person_ids": sharedWith}
	referral, err := s.store.ReferralForReferred(ctx, tx, person.ID, true)
	if errors.Is(err, referralrepositories.ErrNotFound) {
		// Not a referral, so nothing is paid on it; worth knowing only.
		return s.store.RaiseFlag(ctx, tx, person.ID, "", referralmodels.FlagDuplicateDevice, referralmodels.SeverityInfo, evidence)
	}
	if err != nil {
		return err
	}
	if slices.Contains(sharedWith, referral.ReferrerPersonID) {
		evidence["shared_with_referrer"] = true
		return s.holdReferral(ctx, tx, referral, referralmodels.FlagDuplicateDevice, evidence)
	}
	for _, other := range sharedWith {
		sibling, err := s.store.ReferralForReferred(ctx, tx, other, false)
		if errors.Is(err, referralrepositories.ErrNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		if sibling.ReferrerPersonID == referral.ReferrerPersonID {
			evidence["shared_with_same_referrers_referral"] = sibling.ID
			return s.holdReferral(ctx, tx, referral, referralmodels.FlagDuplicateDevice, evidence)
		}
	}
	return s.flagOnce(ctx, tx, person.ID, referral.ID, referralmodels.FlagDuplicateDevice, evidence)
}
