package referralusecases

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	referralmodels "cosmicforge/logistics/services/referral-service/internal/features/referrals/models"
	referralrepositories "cosmicforge/logistics/services/referral-service/internal/features/referrals/repositories"
	"cosmicforge/logistics/shared/go/referralcontract"
)

// Why a paid job did not count towards a driver track.
const (
	notCountedNoPlatformFee    = "no_platform_fee"
	notCountedAwaitingApproval = "awaiting_approval"
	notCountedOutsideWindow    = "outside_window"
	notCountedCollusive        = "collusive_job"
)

// handleProviderOnboardingCompleted records that a driver account became
// fully approved and re-checks the driver track of whoever referred them.
func (s *Service) handleProviderOnboardingCompleted(ctx context.Context, tx pgx.Tx, caller string, raw json.RawMessage) (Outcome, error) {
	var payload referralcontract.ProviderOnboardingCompleted
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", reject("invalid_payload")
	}
	if err := payload.Validate(); err != nil {
		return "", reject("invalid_onboarding")
	}
	if payload.ActorType != providerActorFor(caller) {
		return "", reject("actor_type_mismatch")
	}
	if err := s.store.RecordOnboarding(ctx, tx, payload.ActorType, payload.ActorID, payload.CompletedAt.UTC()); err != nil {
		return "", err
	}
	actor, err := s.store.GetActor(ctx, tx, payload.ActorType, payload.ActorID)
	if errors.Is(err, referralrepositories.ErrNotFound) {
		// The account's announcement has not arrived yet; its registration
		// picks this approval up.
		return OutcomeProcessed, nil
	}
	if err != nil {
		return "", err
	}
	return OutcomeProcessed, s.refreshProviderTrack(ctx, tx, actor)
}

// refreshProviderTrack brings a referred person's driver track up to date
// with their approval time and qualifies it if it is now ready.
func (s *Service) refreshProviderTrack(ctx context.Context, tx pgx.Tx, actor referralmodels.Actor) error {
	referral, err := s.store.ReferralForReferred(ctx, tx, actor.PersonID, true)
	if errors.Is(err, referralrepositories.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if referral.Status == referralmodels.ReferralDisqualified {
		return nil
	}
	track, err := s.store.GetTrack(ctx, tx, referral.ID, referralmodels.ModelProvider, true)
	if errors.Is(err, referralrepositories.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if track.Status != referralmodels.TrackPendingQualification {
		return nil
	}
	onboardedAt, err := s.store.PersonOnboardedAt(ctx, tx, actor.PersonID)
	if err != nil {
		return err
	}
	if onboardedAt != nil && (track.OnboardingCompletedAt == nil || onboardedAt.Before(*track.OnboardingCompletedAt)) {
		track.OnboardingCompletedAt = onboardedAt
		if err := s.store.UpdateTrack(ctx, tx, track); err != nil {
			return err
		}
	}
	return s.tryQualifyProvider(ctx, tx, referral, track, actor.ActorType, actor.ActorID)
}

// applyProviderSide runs a paid job through the driver half of the
// programme, for whoever referred the driver who did it.
//
// Before qualification, each of the driver's first N eligible jobs counts
// towards activation and holds the referrer's commission, released when the
// referral qualifies. After qualification, jobs completed inside the 90-day
// window earn commission. Only jobs with a platform fee are eligible: the
// commission is a share of that fee.
func (s *Service) applyProviderSide(ctx context.Context, tx pgx.Tx, job referralmodels.SettledJob) error {
	if job.ProviderActorType == "" || job.ProviderID == "" {
		return nil
	}
	referral, ok, err := s.referralForAccount(ctx, tx, job.ProviderActorType, job.ProviderID)
	if err != nil || !ok {
		return err
	}
	track, err := s.store.GetTrack(ctx, tx, referral.ID, referralmodels.ModelProvider, true)
	if errors.Is(err, referralrepositories.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if job.PlatformFeeKobo <= 0 {
		_, err := s.store.InsertTrackJob(ctx, tx, referral.ID, referralmodels.ModelProvider, job, "not_counted", notCountedNoPlatformFee)
		return err
	}
	collusive, err := s.collusive(ctx, tx, referral, referralcontract.ActorCustomer, job.CustomerID)
	if err != nil {
		return err
	}
	if collusive {
		// The referrer (or the driver themselves) was the customer: the job
		// neither counts towards activation nor earns commission.
		if _, err := s.store.InsertTrackJob(ctx, tx, referral.ID, referralmodels.ModelProvider, job, "not_counted", notCountedCollusive); err != nil {
			return err
		}
		return s.flagCollusion(ctx, tx, referral, job)
	}

	switch track.Status {
	case referralmodels.TrackPendingQualification:
		if track.JobsCounted >= track.JobsRequired {
			// Activation is complete but the driver is not approved yet: this
			// job earns nothing (only the first N are held for release).
			_, err := s.store.InsertTrackJob(ctx, tx, referral.ID, referralmodels.ModelProvider, job, "not_counted", notCountedAwaitingApproval)
			return err
		}
		inserted, err := s.store.InsertTrackJob(ctx, tx, referral.ID, referralmodels.ModelProvider, job, "activation", "")
		if err != nil || !inserted {
			return err
		}
		track.JobsCounted++
		if err := s.store.UpdateTrack(ctx, tx, track); err != nil {
			return err
		}
		if err := s.grantCommission(ctx, tx, referral, job, referralmodels.RewardPendingQualification); err != nil {
			return err
		}
		return s.tryQualifyProvider(ctx, tx, referral, track, job.ProviderActorType, job.ProviderID)

	case referralmodels.TrackQualified, referralmodels.TrackExpired:
		// Judged by when the job finished, not when it was paid out: a job
		// finished inside the window still pays if it settles after expiry.
		if track.WindowStartsAt == nil || track.WindowEndsAt == nil ||
			job.JobCompletedAt.Before(*track.WindowStartsAt) || job.JobCompletedAt.After(*track.WindowEndsAt) {
			_, err := s.store.InsertTrackJob(ctx, tx, referral.ID, referralmodels.ModelProvider, job, "not_counted", notCountedOutsideWindow)
			return err
		}
		inserted, err := s.store.InsertTrackJob(ctx, tx, referral.ID, referralmodels.ModelProvider, job, "window", "")
		if err != nil || !inserted {
			return err
		}
		return s.grantCommission(ctx, tx, referral, job, referralmodels.RewardPendingActivity)
	}
	return nil
}

// grantCommission records the referrer's share of one job's platform fee.
func (s *Service) grantCommission(ctx context.Context, tx pgx.Tx, referral referralmodels.Referral, job referralmodels.SettledJob, status string) error {
	return s.grantReferrerReward(ctx, tx, referral, referralmodels.Reward{
		Model: referralmodels.ModelProvider, RewardType: referralmodels.RewardProviderCommission,
		Balance:    referralmodels.BalanceEarnings,
		AmountKobo: referralmodels.Commission(job.PlatformFeeKobo, s.rules.ProviderCommissionBPS),
		BasisKobo:  job.PlatformFeeKobo, RateBPS: s.rules.ProviderCommissionBPS,
		SourceService: job.SourceService, SourceReference: job.SourceReference,
		IdempotencyKey: referralmodels.RewardKey(referralmodels.RewardProviderCommission, referral.ID, job.SourceService, job.SourceReference),
	}, job.JobCompletedAt, status)
}

// tryQualifyProvider qualifies a driver track once it has its activation jobs
// and the driver is approved -- in whichever order those arrive. The 90-day
// window opens at the later of the two, the held commission is released to
// the activity decision, and the driver receives the welcome credit.
func (s *Service) tryQualifyProvider(ctx context.Context, tx pgx.Tx, referral referralmodels.Referral, track referralmodels.Track, actorType, actorID string) error {
	if track.Status != referralmodels.TrackPendingQualification || track.JobsCounted < track.JobsRequired || track.OnboardingCompletedAt == nil {
		return nil
	}
	lastActivation, err := s.store.LastActivationJobAt(ctx, tx, referral.ID)
	if err != nil {
		return err
	}
	qualifiedAt := *track.OnboardingCompletedAt
	if lastActivation != nil && lastActivation.After(qualifiedAt) {
		qualifiedAt = *lastActivation
	}
	windowEnds := qualifiedAt.Add(s.rules.ProviderWindow)
	track.Status = referralmodels.TrackQualified
	track.QualifiedAt = &qualifiedAt
	track.WindowStartsAt = &qualifiedAt
	track.WindowEndsAt = &windowEnds
	if err := s.store.UpdateTrack(ctx, tx, track); err != nil {
		return err
	}
	if _, err := s.store.ReleaseHeldRewards(ctx, tx, referral.ID, referralmodels.ModelProvider, s.now().Add(s.rules.ActivityDecisionDelay)); err != nil {
		return err
	}
	if s.rules.ProviderWelcomeCreditKobo <= 0 {
		return nil
	}
	balance := referralmodels.BalancePromotional
	if s.rules.ProviderWelcomeWithdrawable {
		balance = referralmodels.BalanceEarnings
	}
	_, err = s.grant(ctx, tx, referralmodels.Reward{
		PersonID: referral.ReferredPersonID, BeneficiaryActorType: actorType, BeneficiaryActorID: actorID,
		ReferralID: referral.ID, Model: referralmodels.ModelProvider, RewardType: referralmodels.RewardProviderWelcomeCredit,
		Balance: balance, AmountKobo: s.rules.ProviderWelcomeCreditKobo, Status: referralmodels.RewardAvailable,
		WalletState:    referralmodels.WalletPending,
		IdempotencyKey: referralmodels.RewardKey(referralmodels.RewardProviderWelcomeCredit, referral.ID),
	})
	return err
}

// ExpireDriverWindows marks driver tracks whose 90-day window has ended. It
// only changes what the track shows: jobs finished inside the window still
// pay however late they settle, because eligibility is judged by job time.
func (s *Service) ExpireDriverWindows(ctx context.Context) (int64, error) {
	return s.store.ExpireProviderWindows(ctx, s.store.Pool(), s.now())
}
