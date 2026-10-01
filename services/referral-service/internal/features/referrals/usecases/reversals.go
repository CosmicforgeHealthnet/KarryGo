package referralusecases

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/jackc/pgx/v5"

	referralmodels "cosmicforge/logistics/services/referral-service/internal/features/referrals/models"
	referralrepositories "cosmicforge/logistics/services/referral-service/internal/features/referrals/repositories"
	"cosmicforge/logistics/shared/go/referralcontract"
)

// handleJobSettlementReversed records a refund, chargeback or other reversal
// of a paid-out job and takes back what the job earned.
func (s *Service) handleJobSettlementReversed(ctx context.Context, tx pgx.Tx, raw json.RawMessage) (Outcome, error) {
	var payload referralcontract.JobSettlementReversed
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", reject("invalid_payload")
	}
	if err := payload.Validate(); err != nil {
		return "", reject("invalid_reversal")
	}
	// The settlement and its reversals travel as separate events and may be
	// handled at the same moment; serializing on the job makes sure one of
	// them always sees the other.
	if err := s.store.LockJob(ctx, tx, payload.SourceService, payload.SourceReference); err != nil {
		return "", err
	}
	inserted, err := s.store.InsertJobReversal(ctx, tx, payload)
	if err != nil {
		return "", err
	}
	if !inserted {
		return OutcomeDuplicate, nil
	}
	job, err := s.store.GetSettledJob(ctx, tx, payload.SourceService, payload.SourceReference, true)
	if errors.Is(err, referralrepositories.ErrNotFound) {
		// The settlement has not arrived yet; it applies this reversal when it
		// does.
		return OutcomeProcessed, nil
	}
	if err != nil {
		return "", err
	}
	return s.applyReversals(ctx, tx, job)
}

// applyReversals brings a paid job's referral effects in line with every
// reversal recorded against it. It is idempotent: what to take back is worked
// out from cumulative totals, so applying it twice changes nothing.
//
// A fully reversed job stops counting as anyone's activity, un-qualifies a
// customer referral it qualified (the customer's next genuine paid booking
// qualifies it instead) and stops counting towards a driver track that has
// not qualified yet. A qualified driver track keeps its qualification and its
// window: only that job's commission is taken back.
func (s *Service) applyReversals(ctx context.Context, tx pgx.Tx, job referralmodels.SettledJob) (Outcome, error) {
	totals, err := s.store.JobReversalTotals(ctx, tx, job.SourceService, job.SourceReference)
	if err != nil {
		return "", err
	}
	if totals.Count == 0 {
		return OutcomeProcessed, nil
	}
	now := s.now()
	if err := s.store.MarkJobReversalsApplied(ctx, tx, job.SourceService, job.SourceReference, now); err != nil {
		return "", err
	}
	if !job.InProgramme {
		return OutcomeIgnored, nil
	}
	fully := totals.AnyFull || totals.AmountKobo >= job.AmountKobo
	if fully {
		if err := s.store.MarkJobReversed(ctx, tx, job.SourceService, job.SourceReference, now); err != nil {
			return "", err
		}
	}

	touched := map[string]bool{}
	rewards, err := s.store.RewardsForSource(ctx, tx, job.SourceService, job.SourceReference)
	if err != nil {
		return "", err
	}
	for _, reward := range rewards {
		changed, err := s.takeBackTo(ctx, tx, reward, referralmodels.ReversalTarget(reward, totals.PlatformFeeKobo, fully), referralmodels.ReasonJobReversed)
		if err != nil {
			return "", err
		}
		if changed && reward.ReferralID != "" {
			touched[reward.ReferralID] = true
		}
	}
	if fully {
		for _, undo := range []func(context.Context, pgx.Tx, referralmodels.SettledJob) (string, error){s.unqualifyCustomer, s.uncountDriverJob} {
			referralID, err := undo(ctx, tx, job)
			if err != nil {
				return "", err
			}
			if referralID != "" {
				touched[referralID] = true
			}
		}
	}
	for referralID := range touched {
		if err := s.reviewRepeatedReversals(ctx, tx, referralID); err != nil {
			return "", err
		}
	}
	return OutcomeProcessed, nil
}

// takeBackTo takes back as much of a reward as it takes for the total taken
// back to reach target, and reports whether anything changed.
//
// A reward the wallet never received is simply cancelled when nothing of it
// is left. Otherwise the take-back is a new negative row, posted to the wallet
// only after the reward itself (see ClaimRewardsToPost); the wallet lets it
// push the balance below zero, so money already spent or withdrawn is
// recovered from future earnings. Its key is the cumulative target, so the
// same take-back can never be recorded twice.
func (s *Service) takeBackTo(ctx context.Context, tx pgx.Tx, reward referralmodels.Reward, target int64, reason string) (bool, error) {
	if reward.RewardType == referralmodels.RewardReversal || reward.AmountKobo <= 0 || target <= 0 ||
		reward.Status == referralmodels.RewardRejected {
		return false, nil
	}
	target = min(target, reward.AmountKobo)
	already, err := s.store.ReversedAmount(ctx, tx, reward.ID)
	if err != nil {
		return false, err
	}
	if target <= already {
		return false, nil
	}
	now := s.now()
	if !reward.CommittedToWallet() && target == reward.AmountKobo {
		return true, s.store.SetRewardStatus(ctx, tx, reward.ID, referralmodels.RewardRejected, reason, now)
	}
	if _, err := s.grant(ctx, tx, referralmodels.Reward{
		PersonID: reward.PersonID, BeneficiaryActorType: reward.BeneficiaryActorType, BeneficiaryActorID: reward.BeneficiaryActorID,
		ReferralID: reward.ReferralID, Model: reward.Model, RewardType: referralmodels.RewardReversal,
		Balance: reward.Balance, AmountKobo: -(target - already), Currency: reward.Currency,
		Status: referralmodels.RewardAvailable, StatusReason: reason,
		SourceService: reward.SourceService, SourceReference: reward.SourceReference, ReversesRewardID: reward.ID,
		WalletState:    referralmodels.WalletPending,
		IdempotencyKey: referralmodels.RewardKey(referralmodels.RewardReversal, reward.ID, strconv.FormatInt(target, 10)),
	}); err != nil {
		return false, err
	}
	if target == reward.AmountKobo {
		if err := s.store.SetRewardStatus(ctx, tx, reward.ID, referralmodels.RewardReversed, reason, now); err != nil {
			return false, err
		}
	}
	return true, nil
}

// unqualifyCustomer undoes a customer referral's qualification when the
// booking that qualified it was fully reversed -- a refunded booking was never
// a qualifying one -- and lets the customer's next genuine paid booking
// qualify it instead. It returns the referral it changed, if any.
func (s *Service) unqualifyCustomer(ctx context.Context, tx pgx.Tx, job referralmodels.SettledJob) (string, error) {
	referral, ok, err := s.referralForAccount(ctx, tx, referralcontract.ActorCustomer, job.CustomerID)
	if err != nil || !ok {
		return "", err
	}
	track, err := s.store.GetTrack(ctx, tx, referral.ID, referralmodels.ModelCustomer, true)
	if errors.Is(err, referralrepositories.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if track.Status != referralmodels.TrackQualified ||
		track.QualifyingSourceService != job.SourceService || track.QualifyingSourceReference != job.SourceReference {
		return "", nil
	}
	track.Status = referralmodels.TrackPendingQualification
	track.JobsCounted = 0
	track.QualifiedAt = nil
	track.QualifyingSourceService = ""
	track.QualifyingSourceReference = ""
	if err := s.store.UpdateTrack(ctx, tx, track); err != nil {
		return "", err
	}
	actor, err := s.store.GetActor(ctx, tx, referralcontract.ActorCustomer, job.CustomerID)
	if err != nil {
		return "", err
	}
	// Reversed jobs are skipped, so this qualifies on the next genuine one.
	return referral.ID, s.catchUp(ctx, tx, actor)
}

// uncountDriverJob stops a fully reversed job counting towards a driver
// track that has not qualified yet: the driver needs another genuine job to
// reach the activation count. It returns the referral it changed, if any.
func (s *Service) uncountDriverJob(ctx context.Context, tx pgx.Tx, job referralmodels.SettledJob) (string, error) {
	if job.ProviderActorType == "" || job.ProviderID == "" {
		return "", nil
	}
	referral, ok, err := s.referralForAccount(ctx, tx, job.ProviderActorType, job.ProviderID)
	if err != nil || !ok {
		return "", err
	}
	track, err := s.store.GetTrack(ctx, tx, referral.ID, referralmodels.ModelProvider, true)
	if errors.Is(err, referralrepositories.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	trackJob, err := s.store.GetTrackJob(ctx, tx, referral.ID, referralmodels.ModelProvider, job.SourceService, job.SourceReference)
	if errors.Is(err, referralrepositories.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	marked, err := s.store.MarkTrackJobReversed(ctx, tx, referral.ID, referralmodels.ModelProvider, job.SourceService, job.SourceReference, s.now())
	if err != nil || !marked {
		return "", err
	}
	if trackJob.CountedAs == "activation" && track.Status == referralmodels.TrackPendingQualification && track.JobsCounted > 0 {
		track.JobsCounted--
		if err := s.store.UpdateTrack(ctx, tx, track); err != nil {
			return "", err
		}
	}
	return referral.ID, nil
}

// reviewRepeatedReversals puts a referral under review each time the number
// of its reversed jobs reaches another multiple of the threshold (3, 6, 9 ...
// by default): repeated refunds are a classic sign of fake jobs.
func (s *Service) reviewRepeatedReversals(ctx context.Context, tx pgx.Tx, referralID string) error {
	threshold := s.rules.ReversalReviewThreshold
	if threshold <= 0 {
		return nil
	}
	n, err := s.store.CountReversedJobs(ctx, tx, referralID)
	if err != nil || n < threshold || n%threshold != 0 {
		return err
	}
	referral, err := s.store.GetReferral(ctx, tx, referralID, true)
	if err != nil {
		return err
	}
	if referral.Status != referralmodels.ReferralActive {
		return nil
	}
	return s.holdReferral(ctx, tx, referral, referralmodels.FlagRepeatedReversals, map[string]any{"reversed_jobs": n})
}
