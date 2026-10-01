package referralusecases

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/referral-service/internal/database"
	referralmodels "cosmicforge/logistics/services/referral-service/internal/features/referrals/models"
	referralrepositories "cosmicforge/logistics/services/referral-service/internal/features/referrals/repositories"
	"cosmicforge/logistics/shared/go/referralcontract"
	"cosmicforge/logistics/shared/go/walletclient"
)

// WalletClient is the slice of shared/go/walletclient this service uses.
type WalletClient interface {
	CreditReferral(ctx context.Context, request walletclient.ReferralPostingRequest) (walletclient.ReferralPosting, error)
	ReverseReferral(ctx context.Context, request walletclient.ReferralPostingRequest) (walletclient.ReferralPosting, error)
	ReferralTotals(ctx context.Context, ownerType, ownerID, providerType string) (walletclient.ReferralTotals, error)
}

// onTrackOpened grants what a referred person receives when a track starts.
// A referred customer gets the welcome credit and the first-booking credit
// straight away (spend-only; no referrer activity rule applies to them).
func (s *Service) onTrackOpened(ctx context.Context, tx pgx.Tx, referral referralmodels.Referral, actor referralmodels.Actor) error {
	if referralmodels.ModelForActor(actor.ActorType) != referralmodels.ModelCustomer {
		return nil
	}
	for rewardType, amount := range map[string]int64{
		referralmodels.RewardCustomerWelcomeCredit: s.rules.CustomerWelcomeCreditKobo,
		referralmodels.RewardFirstBookingCredit:    s.rules.FirstBookingCreditKobo,
	} {
		if amount <= 0 {
			continue
		}
		if _, err := s.grant(ctx, tx, referralmodels.Reward{
			PersonID: referral.ReferredPersonID, BeneficiaryActorType: actor.ActorType, BeneficiaryActorID: actor.ActorID,
			ReferralID: referral.ID, Model: referralmodels.ModelCustomer, RewardType: rewardType,
			Balance: referralmodels.BalancePromotional, AmountKobo: amount, Status: referralmodels.RewardAvailable,
			WalletState:    referralmodels.WalletPending,
			IdempotencyKey: referralmodels.RewardKey(rewardType, referral.ID),
		}); err != nil {
			return err
		}
	}
	return nil
}

// grant records a reward (once) and stamps the service clock on it.
func (s *Service) grant(ctx context.Context, tx pgx.Tx, reward referralmodels.Reward) (bool, error) {
	if reward.WalletState == referralmodels.WalletPending && reward.WalletDueAt == nil {
		now := s.now()
		reward.WalletDueAt = &now
	}
	return s.store.InsertReward(ctx, tx, reward)
}

// applyJobToReferrals runs a paid job through both sides of the programme:
// the customer who booked it and the driver who did it.
func (s *Service) applyJobToReferrals(ctx context.Context, tx pgx.Tx, job referralmodels.SettledJob) error {
	if err := s.applyCustomerSide(ctx, tx, job); err != nil {
		return err
	}
	return s.applyProviderSide(ctx, tx, job)
}

// applyCustomerSide qualifies a referred customer on their first paid
// booking, queues the referrer's bonus for the activity decision and counts
// the referral towards the referrer's milestones.
func (s *Service) applyCustomerSide(ctx context.Context, tx pgx.Tx, job referralmodels.SettledJob) error {
	referral, ok, err := s.referralForAccount(ctx, tx, referralcontract.ActorCustomer, job.CustomerID)
	if err != nil || !ok {
		return err
	}
	track, err := s.store.GetTrack(ctx, tx, referral.ID, referralmodels.ModelCustomer, true)
	if errors.Is(err, referralrepositories.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if track.Status != referralmodels.TrackPendingQualification {
		return nil // only the first paid booking qualifies
	}
	collusive, err := s.collusive(ctx, tx, referral, job.ProviderActorType, job.ProviderID)
	if err != nil {
		return err
	}
	if collusive {
		// A ride with their own referrer (or with themselves) cannot qualify
		// the referral; the next genuine booking can.
		return s.flagCollusion(ctx, tx, referral, job)
	}
	completedAt := job.JobCompletedAt
	track.Status = referralmodels.TrackQualified
	track.JobsCounted = 1
	track.QualifiedAt = &completedAt
	track.QualifyingSourceService = job.SourceService
	track.QualifyingSourceReference = job.SourceReference
	if err := s.store.UpdateTrack(ctx, tx, track); err != nil {
		return err
	}
	// Keyed by the qualifying booking: if that booking is later refunded the
	// bonus is taken back, and the booking that qualifies the referral next
	// earns its own.
	if err := s.grantReferrerReward(ctx, tx, referral, referralmodels.Reward{
		Model: referralmodels.ModelCustomer, RewardType: referralmodels.RewardCustomerReferralBonus,
		Balance: referralmodels.BalanceEarnings, AmountKobo: s.rules.CustomerReferralBonusKobo,
		SourceService: job.SourceService, SourceReference: job.SourceReference,
		IdempotencyKey: referralmodels.RewardKey(referralmodels.RewardCustomerReferralBonus, referral.ID, job.SourceService, job.SourceReference),
	}, completedAt, referralmodels.RewardPendingActivity); err != nil {
		return err
	}
	return s.checkMilestones(ctx, tx, referral.ReferrerPersonID)
}

// grantReferrerReward records a reward for the referrer, paid into the
// account that first received their code, gated on the referrer having been
// active when the job was completed.
func (s *Service) grantReferrerReward(ctx context.Context, tx pgx.Tx, referral referralmodels.Referral, reward referralmodels.Reward, activityAsOf time.Time, status string) error {
	if reward.AmountKobo <= 0 {
		return nil
	}
	referrer, err := s.store.GetPerson(ctx, tx, referral.ReferrerPersonID, false)
	if err != nil {
		return err
	}
	asOf := activityAsOf
	reward.PersonID = referrer.ID
	reward.BeneficiaryActorType = referrer.HomeActorType
	reward.BeneficiaryActorID = referrer.HomeActorID
	reward.ReferralID = referral.ID
	reward.Status = status
	reward.ActivityAsOf = &asOf
	if status == referralmodels.RewardPendingActivity {
		decideAfter := s.now().Add(s.rules.ActivityDecisionDelay)
		reward.DecideAfter = &decideAfter
	}
	_, err = s.grant(ctx, tx, reward)
	return err
}

// referralForAccount finds the referral an account's person joined through.
func (s *Service) referralForAccount(ctx context.Context, tx pgx.Tx, actorType, actorID string) (referralmodels.Referral, bool, error) {
	if actorID == "" {
		return referralmodels.Referral{}, false, nil
	}
	actor, err := s.store.GetActor(ctx, tx, actorType, actorID)
	if errors.Is(err, referralrepositories.ErrNotFound) {
		return referralmodels.Referral{}, false, nil // announcement not here yet; catch-up handles it
	}
	if err != nil {
		return referralmodels.Referral{}, false, err
	}
	referral, err := s.store.ReferralForReferred(ctx, tx, actor.PersonID, true)
	if errors.Is(err, referralrepositories.ErrNotFound) {
		return referralmodels.Referral{}, false, nil
	}
	if err != nil {
		return referralmodels.Referral{}, false, err
	}
	if referral.Status == referralmodels.ReferralDisqualified {
		return referralmodels.Referral{}, false, nil
	}
	return referral, true, nil
}

// catchUp replays an account's already-recorded paid jobs through its side of
// the programme. It runs when an announcement arrives after some of the
// account's jobs (the two travel through different services' relays). Every
// effect is idempotent, so replaying is always safe.
func (s *Service) catchUp(ctx context.Context, tx pgx.Tx, actor referralmodels.Actor) error {
	jobs, err := s.store.SettledJobsForActor(ctx, tx, actor.ActorType, actor.ActorID)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if actor.ActorType == referralcontract.ActorCustomer {
			err = s.applyCustomerSide(ctx, tx, job)
		} else {
			err = s.applyProviderSide(ctx, tx, job)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// isActiveAsOf applies the "active referrer" rule to a person at a moment:
// enough paid bookings as a customer, or enough paid jobs across all of their
// driver accounts, in the window ending then.
func (s *Service) isActiveAsOf(ctx context.Context, q database.Querier, personID string, at time.Time) (bool, error) {
	actors, err := s.store.ActorsForPerson(ctx, q, personID)
	if err != nil {
		return false, err
	}
	var counts referralmodels.ActivityCounts
	for _, actor := range actors {
		limit := s.rules.Activity.ProviderThreshold
		if actor.ActorType == referralcontract.ActorCustomer {
			limit = s.rules.Activity.CustomerThreshold
		}
		n, err := s.store.CountActivity(ctx, q, actor.ActorType, actor.ActorID, at, s.rules.Activity.Window, limit)
		if err != nil {
			return false, err
		}
		if actor.ActorType == referralcontract.ActorCustomer {
			counts.CustomerJobs += n
		} else {
			counts.ProviderJobs += n
		}
	}
	return s.rules.Activity.IsActive(counts), nil
}

// DecidePendingRewards makes the activity decision for every reward whose
// waiting time is up. It returns how many rewards it decided.
func (s *Service) DecidePendingRewards(ctx context.Context, batch int) (int, error) {
	if batch <= 0 {
		batch = 100
	}
	now := s.now()
	decided := 0
	err := database.WithinTx(ctx, s.store.Pool(), func(tx pgx.Tx) error {
		rewards, err := s.store.ClaimRewardsToDecide(ctx, tx, now, batch)
		if err != nil {
			return err
		}
		for _, reward := range rewards {
			status, reason, err := s.decide(ctx, tx, reward)
			if err != nil {
				return err
			}
			if err := s.store.SetRewardStatus(ctx, tx, reward.ID, status, reason, now); err != nil {
				return err
			}
			decided++
		}
		return nil
	})
	return decided, err
}

// decide is the rule for one pending reward.
func (s *Service) decide(ctx context.Context, tx pgx.Tx, reward referralmodels.Reward) (string, string, error) {
	underReview := false
	if reward.ReferralID != "" {
		referral, err := s.store.GetReferral(ctx, tx, reward.ReferralID, false)
		if err != nil {
			return "", "", err
		}
		switch referral.Status {
		case referralmodels.ReferralDisqualified:
			return referralmodels.RewardRejected, referralmodels.ReasonDisqualified, nil
		case referralmodels.ReferralUnderReview, referralmodels.ReferralSuspended:
			underReview = true
		}
	}
	person, err := s.store.GetPerson(ctx, tx, reward.PersonID, false)
	if err != nil {
		return "", "", err
	}
	if person.Status == referralmodels.PersonBlocked {
		return referralmodels.RewardRejected, referralmodels.ReasonPersonBlocked, nil
	}
	if reward.ActivityAsOf != nil {
		active, err := s.isActiveAsOf(ctx, tx, person.ID, *reward.ActivityAsOf)
		if err != nil {
			return "", "", err
		}
		if !active {
			// Lost, not saved for later: the spec's "no back-payment" rule.
			return referralmodels.RewardRejected, referralmodels.ReasonReferrerInactive, nil
		}
	}
	if underReview || person.Status == referralmodels.PersonFrozen {
		return referralmodels.RewardFrozen, referralmodels.ReasonUnderReview, nil
	}
	return referralmodels.RewardAvailable, "", nil
}

// walletRetryDelay backs off failed wallet postings: 10s, 20s, 40s ... up to
// an hour. The idempotency key makes every retry safe.
func walletRetryDelay(attempts int) time.Duration {
	delay := 10 * time.Second
	for i := 0; i < attempts && delay < time.Hour; i++ {
		delay *= 2
	}
	return min(delay, time.Hour)
}

// PostRewardsToWallet sends approved rewards (and reversals) to
// payment-wallet. It returns how many were posted. Without a wallet client it
// does nothing: rewards wait, durably, until one is configured.
func (s *Service) PostRewardsToWallet(ctx context.Context, batch int) (int, error) {
	if s.wallet == nil {
		return 0, nil
	}
	if batch <= 0 {
		batch = 100
	}
	now := s.now()
	rewards, err := s.store.ClaimRewardsToPost(ctx, now, time.Minute, batch)
	if err != nil {
		return 0, err
	}
	posted := 0
	var firstErr error
	for _, reward := range rewards {
		request := walletPostingRequest(reward)
		var result walletclient.ReferralPosting
		var postErr error
		if reward.AmountKobo > 0 {
			result, postErr = s.wallet.CreditReferral(ctx, request)
		} else {
			result, postErr = s.wallet.ReverseReferral(ctx, request)
		}
		if postErr != nil {
			if firstErr == nil {
				firstErr = postErr
			}
			if err := s.store.MarkRewardPostFailed(ctx, reward.ID, postErr, now.Add(walletRetryDelay(reward.WalletAttempts))); err != nil {
				return posted, err
			}
			continue
		}
		if err := s.store.MarkRewardPosted(ctx, reward.ID, result.TransactionReference); err != nil {
			return posted, err
		}
		posted++
	}
	return posted, firstErr
}

// walletPostingRequest maps a reward onto the wallet's referral API: the
// beneficiary's ledger, the balance kind, and the reward's own idempotency key.
func walletPostingRequest(reward referralmodels.Reward) walletclient.ReferralPostingRequest {
	amount := reward.AmountKobo
	if amount < 0 {
		amount = -amount
	}
	request := walletclient.ReferralPostingRequest{
		OwnerID:        reward.BeneficiaryActorID,
		Balance:        reward.Balance,
		AmountKobo:     amount,
		Currency:       reward.Currency,
		IdempotencyKey: reward.IdempotencyKey,
		Reference:      reward.ID,
		Reason:         reward.RewardType,
	}
	if reward.BeneficiaryActorType == referralcontract.ActorCustomer {
		request.OwnerType = "customer"
	} else {
		request.OwnerType = "provider"
		request.ProviderType = referralcontract.WalletProviderTypeForActor(reward.BeneficiaryActorType)
	}
	return request
}
