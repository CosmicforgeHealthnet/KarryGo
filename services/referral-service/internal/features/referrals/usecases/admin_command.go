package referralusecases

import (
	"context"
	"errors"
	"strings"

	referralmodels "cosmicforge/logistics/services/referral-service/internal/features/referrals/models"
	referralrepositories "cosmicforge/logistics/services/referral-service/internal/features/referrals/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
	"github.com/jackc/pgx/v5"
)

// ApplyAdminCommandInTx runs the established Referral transition inside the
// caller's transaction. This is the only adapter used by the shared immutable
// Admin command ledger; it does not introduce a generic status setter.
func (s *Service) ApplyAdminCommandInTx(ctx context.Context, tx pgx.Tx, action, subjectID string, expectedVersion int64, req AdminRequest, params map[string]string) (int64, string, string, error) {
	if err := req.validate(); err != nil {
		return 0, "", "", err
	}
	var version int64
	var prior string
	kind := ""
	switch action {
	case "FREEZE_PERSON", "CLEAR_PERSON", "BLOCK_PERSON":
		kind = "person"
		err := tx.QueryRow(ctx, `SELECT version,status FROM referral_persons WHERE id=$1 FOR UPDATE`, subjectID).Scan(&version, &prior)
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, "", "", apperrors.NotFound("No such person in the referral programme.", nil)
		}
		if err != nil {
			return 0, "", "", err
		}
	case "FREEZE_REFERRAL", "CLEAR_REFERRAL", "DISQUALIFY_REFERRAL":
		kind = "referral"
		err := tx.QueryRow(ctx, `SELECT version,status FROM referrals WHERE id=$1 FOR UPDATE`, subjectID).Scan(&version, &prior)
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, "", "", apperrors.NotFound("No such referral.", nil)
		}
		if err != nil {
			return 0, "", "", err
		}
	case "RESOLVE_FRAUD_FLAG":
		kind = "flag"
		err := tx.QueryRow(ctx, `SELECT version,status FROM fraud_flags WHERE id=$1 FOR UPDATE`, subjectID).Scan(&version, &prior)
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, "", "", apperrors.NotFound("No such flag.", nil)
		}
		if err != nil {
			return 0, "", "", err
		}
	case "REVERSE_REWARD":
		kind = "reward"
		err := tx.QueryRow(ctx, `SELECT version,status FROM reward_ledger WHERE id=$1 FOR UPDATE`, subjectID).Scan(&version, &prior)
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, "", "", apperrors.NotFound("No such reward.", nil)
		}
		if err != nil {
			return 0, "", "", err
		}
	default:
		return 0, "", "", apperrors.BadRequest("Unsupported referral administrative action.", nil)
	}
	if version != expectedVersion {
		return version, prior, prior, apperrors.Conflict("The referral record changed; read it again before deciding.", nil)
	}
	details := map[string]any{}
	switch action {
	case "FREEZE_PERSON":
		person, err := s.lockPerson(ctx, tx, subjectID)
		if err != nil {
			return version, prior, prior, err
		}
		if person.Status != referralmodels.PersonActive {
			return version, prior, prior, apperrors.Conflict("Only an active person can be frozen.", nil)
		}
		if err = s.store.SetPersonStatus(ctx, tx, person.ID, referralmodels.PersonFrozen, strings.TrimSpace(req.Reason)); err != nil {
			return version, prior, prior, err
		}
		queued, err := s.store.RewardsForPerson(ctx, tx, person.ID, []string{referralmodels.RewardAvailable})
		if err != nil {
			return version, prior, prior, err
		}
		if err = s.freezeQueued(ctx, tx, queued); err != nil {
			return version, prior, prior, err
		}
	case "CLEAR_PERSON":
		person, err := s.lockPerson(ctx, tx, subjectID)
		if err != nil {
			return version, prior, prior, err
		}
		if person.Status != referralmodels.PersonFrozen {
			return version, prior, prior, apperrors.Conflict("This person is not frozen.", nil)
		}
		if err = s.store.SetPersonStatus(ctx, tx, person.ID, referralmodels.PersonActive, ""); err != nil {
			return version, prior, prior, err
		}
		frozen, err := s.store.RewardsForPerson(ctx, tx, person.ID, []string{referralmodels.RewardFrozen})
		if err != nil {
			return version, prior, prior, err
		}
		released, err := s.releaseFrozen(ctx, tx, frozen)
		if err != nil {
			return version, prior, prior, err
		}
		details["released_rewards"] = released
	case "BLOCK_PERSON":
		person, err := s.lockPerson(ctx, tx, subjectID)
		if err != nil {
			return version, prior, prior, err
		}
		if person.Status == referralmodels.PersonBlocked {
			return version, prior, prior, apperrors.Conflict("This person is already blocked.", nil)
		}
		if err = s.store.SetPersonStatus(ctx, tx, person.ID, referralmodels.PersonBlocked, strings.TrimSpace(req.Reason)); err != nil {
			return version, prior, prior, err
		}
		rewards, err := s.store.RewardsForPerson(ctx, tx, person.ID, nil)
		if err != nil {
			return version, prior, prior, err
		}
		cancelled, reversed, err := s.withdrawRewards(ctx, tx, rewards, referralmodels.ReasonPersonBlocked, params["reverse_paid"] == "true")
		if err != nil {
			return version, prior, prior, err
		}
		details["cancelled_rewards"], details["reversed_rewards"] = cancelled, reversed
	case "FREEZE_REFERRAL":
		referral, err := s.lockReferral(ctx, tx, subjectID)
		if err != nil {
			return version, prior, prior, err
		}
		if referral.Status != referralmodels.ReferralActive {
			return version, prior, prior, apperrors.Conflict("Only an active referral can be frozen.", nil)
		}
		if err = s.store.SetReferralStatus(ctx, tx, referral.ID, referralmodels.ReferralUnderReview, strings.TrimSpace(req.Reason)); err != nil {
			return version, prior, prior, err
		}
		queued, err := s.store.RewardsForReferral(ctx, tx, referral.ID, []string{referralmodels.RewardAvailable})
		if err != nil {
			return version, prior, prior, err
		}
		if err = s.freezeQueued(ctx, tx, queued); err != nil {
			return version, prior, prior, err
		}
	case "CLEAR_REFERRAL":
		referral, err := s.lockReferral(ctx, tx, subjectID)
		if err != nil {
			return version, prior, prior, err
		}
		if referral.Status != referralmodels.ReferralUnderReview && referral.Status != referralmodels.ReferralSuspended {
			return version, prior, prior, apperrors.Conflict("This referral is not under review.", nil)
		}
		if err = s.store.SetReferralStatus(ctx, tx, referral.ID, referralmodels.ReferralActive, ""); err != nil {
			return version, prior, prior, err
		}
		frozen, err := s.store.RewardsForReferral(ctx, tx, referral.ID, []string{referralmodels.RewardFrozen})
		if err != nil {
			return version, prior, prior, err
		}
		released, err := s.releaseFrozen(ctx, tx, frozen)
		if err != nil {
			return version, prior, prior, err
		}
		details["released_rewards"] = released
		if err = s.checkMilestones(ctx, tx, referral.ReferrerPersonID); err != nil {
			return version, prior, prior, err
		}
	case "DISQUALIFY_REFERRAL":
		referral, err := s.lockReferral(ctx, tx, subjectID)
		if err != nil {
			return version, prior, prior, err
		}
		if referral.Status == referralmodels.ReferralDisqualified {
			return version, prior, prior, apperrors.Conflict("This referral is already disqualified.", nil)
		}
		if err = s.store.SetReferralStatus(ctx, tx, referral.ID, referralmodels.ReferralDisqualified, strings.TrimSpace(req.Reason)); err != nil {
			return version, prior, prior, err
		}
		rewards, err := s.store.RewardsForReferral(ctx, tx, referral.ID, nil)
		if err != nil {
			return version, prior, prior, err
		}
		cancelled, reversed, err := s.withdrawRewards(ctx, tx, rewards, referralmodels.ReasonDisqualified, params["reverse_paid"] == "true")
		if err != nil {
			return version, prior, prior, err
		}
		details["cancelled_rewards"], details["reversed_rewards"] = cancelled, reversed
	case "RESOLVE_FRAUD_FLAG":
		flag, err := s.store.GetFlag(ctx, tx, subjectID, true)
		if err != nil {
			return version, prior, prior, err
		}
		if flag.Status != referralmodels.FlagOpen {
			return version, prior, prior, apperrors.Conflict("This flag was already resolved.", nil)
		}
		outcome := strings.ToLower(params["outcome"])
		if outcome != referralmodels.FlagCleared && outcome != referralmodels.FlagUpheld {
			return version, prior, prior, apperrors.BadRequest("Unsupported fraud review outcome.", nil)
		}
		if err = s.store.ResolveFlag(ctx, tx, flag.ID, outcome, req.PerformedBy, req.Reason, s.now()); err != nil {
			return version, prior, prior, err
		}
		details["outcome"] = outcome
	case "REVERSE_REWARD":
		reward, err := s.store.GetReward(ctx, tx, subjectID, true)
		if err != nil {
			return version, prior, prior, err
		}
		if reward.RewardType == referralmodels.RewardReversal || reward.Status == referralmodels.RewardRejected {
			return version, prior, prior, apperrors.Conflict("This reward cannot be reversed.", nil)
		}
		changed, err := s.takeBackTo(ctx, tx, reward, reward.AmountKobo, referralmodels.ReasonAdminReversal)
		if err != nil {
			return version, prior, prior, err
		}
		if !changed {
			return version, prior, prior, apperrors.Conflict("This reward has already been taken back in full.", nil)
		}
		details["amount_kobo"] = reward.AmountKobo
	}
	var next string
	var actual int64
	table := map[string]string{"person": "referral_persons", "referral": "referrals", "flag": "fraud_flags", "reward": "reward_ledger"}[kind]
	if err := tx.QueryRow(ctx, `SELECT version,status FROM `+table+` WHERE id=$1`, subjectID).Scan(&actual, &next); err != nil {
		return version, prior, prior, err
	}
	if actual != expectedVersion+1 {
		return version, prior, next, apperrors.Conflict("The owner transition produced an unexpected version.", nil)
	}
	if err := s.store.RecordAdminAction(ctx, tx, referralrepositories.AdminAction{Action: strings.ToLower(action), SubjectKind: kind, SubjectID: subjectID, PerformedBy: strings.TrimSpace(req.PerformedBy), Caller: req.Caller, Reason: strings.TrimSpace(req.Reason), Details: details}); err != nil {
		return version, prior, next, err
	}
	return version, prior, next, nil
}
