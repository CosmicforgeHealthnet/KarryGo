package referralusecases

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/referral-service/internal/database"
	referralmodels "cosmicforge/logistics/services/referral-service/internal/features/referrals/models"
	referralrepositories "cosmicforge/logistics/services/referral-service/internal/features/referrals/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/referralcontract"
)

// AdminRequest says who is acting and why. Every admin action needs both,
// is recorded in referral_admin_actions in the same transaction as its
// effect, and never moves money directly: take-backs go through the reward
// ledger and reach the wallet like any other posting.
type AdminRequest struct {
	// Caller is the signed service the request came through.
	Caller string
	// PerformedBy is the operator, as the calling service identifies them.
	PerformedBy string
	Reason      string
	// ReversePaid, on disqualify and block, also takes back rewards that
	// were already paid (by default only unpaid ones are cancelled).
	ReversePaid bool
}

func (r AdminRequest) validate() error {
	var fields []apperrors.FieldViolation
	if strings.TrimSpace(r.PerformedBy) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "performed_by", Message: "Say who is taking this action."})
	}
	if strings.TrimSpace(r.Reason) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "reason", Message: "A reason is required."})
	}
	if len(fields) > 0 {
		return apperrors.Validation("Check the request.", fields)
	}
	return nil
}

// Admin action names, as recorded.
const (
	ActionFreezePerson       = "freeze_person"
	ActionClearPerson        = "clear_person"
	ActionBlockPerson        = "block_person"
	ActionFreezeReferral     = "freeze_referral"
	ActionClearReferral      = "clear_referral"
	ActionDisqualifyReferral = "disqualify_referral"
	ActionReverseReward      = "reverse_reward"
	ActionResolveFlag        = "resolve_flag"
)

// adminTx runs one admin action and its audit record in a transaction.
func (s *Service) adminTx(ctx context.Context, req AdminRequest, action, subjectKind, subjectID string, apply func(tx pgx.Tx, details map[string]any) error) error {
	if err := req.validate(); err != nil {
		return err
	}
	return database.WithinTx(ctx, s.store.Pool(), func(tx pgx.Tx) error {
		details := map[string]any{}
		if err := apply(tx, details); err != nil {
			return err
		}
		return s.store.RecordAdminAction(ctx, tx, referralrepositories.AdminAction{
			Action: action, SubjectKind: subjectKind, SubjectID: subjectID,
			PerformedBy: strings.TrimSpace(req.PerformedBy), Caller: req.Caller, Reason: strings.TrimSpace(req.Reason),
			Details: details,
		})
	})
}

func (s *Service) lockPerson(ctx context.Context, tx pgx.Tx, personID string) (referralmodels.Person, error) {
	person, err := s.store.GetPerson(ctx, tx, personID, true)
	if errors.Is(err, referralrepositories.ErrNotFound) {
		return person, apperrors.NotFound("No such person in the referral programme.", nil)
	}
	return person, err
}

func (s *Service) lockReferral(ctx context.Context, tx pgx.Tx, referralID string) (referralmodels.Referral, error) {
	referral, err := s.store.GetReferral(ctx, tx, referralID, true)
	if errors.Is(err, referralrepositories.ErrNotFound) {
		return referral, apperrors.NotFound("No such referral.", nil)
	}
	return referral, err
}

// FreezePerson puts everything a person earns on hold during a review:
// approved rewards not yet in the wallet are frozen and new ones freeze
// instead of paying. Their withdrawals are refused while frozen.
func (s *Service) FreezePerson(ctx context.Context, personID string, req AdminRequest) error {
	return s.adminTx(ctx, req, ActionFreezePerson, "person", personID, func(tx pgx.Tx, details map[string]any) error {
		person, err := s.lockPerson(ctx, tx, personID)
		if err != nil {
			return err
		}
		switch person.Status {
		case referralmodels.PersonBlocked:
			return apperrors.Conflict("This person is blocked from the programme.", nil)
		case referralmodels.PersonFrozen:
			return apperrors.Conflict("This person is already frozen.", nil)
		}
		if err := s.store.SetPersonStatus(ctx, tx, person.ID, referralmodels.PersonFrozen, strings.TrimSpace(req.Reason)); err != nil {
			return err
		}
		queued, err := s.store.RewardsForPerson(ctx, tx, person.ID, []string{referralmodels.RewardAvailable})
		if err != nil {
			return err
		}
		return s.freezeQueued(ctx, tx, queued)
	})
}

// ClearPerson ends a person's review: their frozen rewards are paid unless a
// referral they belong to is still held.
func (s *Service) ClearPerson(ctx context.Context, personID string, req AdminRequest) error {
	return s.adminTx(ctx, req, ActionClearPerson, "person", personID, func(tx pgx.Tx, details map[string]any) error {
		person, err := s.lockPerson(ctx, tx, personID)
		if err != nil {
			return err
		}
		if person.Status != referralmodels.PersonFrozen {
			return apperrors.Conflict("This person is not frozen.", nil)
		}
		if err := s.store.SetPersonStatus(ctx, tx, person.ID, referralmodels.PersonActive, ""); err != nil {
			return err
		}
		frozen, err := s.store.RewardsForPerson(ctx, tx, person.ID, []string{referralmodels.RewardFrozen})
		if err != nil {
			return err
		}
		released, err := s.releaseFrozen(ctx, tx, frozen)
		details["released_rewards"] = released
		return err
	})
}

// BlockPerson removes a person from the programme for good: their code stops
// working, nothing more is paid to them and their unpaid rewards are
// cancelled (ReversePaid also takes back what was already paid).
func (s *Service) BlockPerson(ctx context.Context, personID string, req AdminRequest) error {
	return s.adminTx(ctx, req, ActionBlockPerson, "person", personID, func(tx pgx.Tx, details map[string]any) error {
		person, err := s.lockPerson(ctx, tx, personID)
		if err != nil {
			return err
		}
		if person.Status == referralmodels.PersonBlocked {
			return apperrors.Conflict("This person is already blocked.", nil)
		}
		if err := s.store.SetPersonStatus(ctx, tx, person.ID, referralmodels.PersonBlocked, strings.TrimSpace(req.Reason)); err != nil {
			return err
		}
		rewards, err := s.store.RewardsForPerson(ctx, tx, person.ID, nil)
		if err != nil {
			return err
		}
		cancelled, reversed, err := s.withdrawRewards(ctx, tx, rewards, referralmodels.ReasonPersonBlocked, req.ReversePaid)
		details["cancelled_rewards"], details["reversed_rewards"] = cancelled, reversed
		return err
	})
}

// FreezeReferral puts one referral under review.
func (s *Service) FreezeReferral(ctx context.Context, referralID string, req AdminRequest) error {
	return s.adminTx(ctx, req, ActionFreezeReferral, "referral", referralID, func(tx pgx.Tx, details map[string]any) error {
		referral, err := s.lockReferral(ctx, tx, referralID)
		if err != nil {
			return err
		}
		if referral.Status != referralmodels.ReferralActive {
			return apperrors.Conflict("Only an active referral can be frozen; this one is "+referral.Status+".", nil)
		}
		if err := s.store.SetReferralStatus(ctx, tx, referral.ID, referralmodels.ReferralUnderReview, strings.TrimSpace(req.Reason)); err != nil {
			return err
		}
		queued, err := s.store.RewardsForReferral(ctx, tx, referral.ID, []string{referralmodels.RewardAvailable})
		if err != nil {
			return err
		}
		return s.freezeQueued(ctx, tx, queued)
	})
}

// ClearReferral ends a referral's review. Its frozen rewards are paid (unless
// the person they belong to is still held) and it counts towards its
// referrer's milestones again.
func (s *Service) ClearReferral(ctx context.Context, referralID string, req AdminRequest) error {
	return s.adminTx(ctx, req, ActionClearReferral, "referral", referralID, func(tx pgx.Tx, details map[string]any) error {
		referral, err := s.lockReferral(ctx, tx, referralID)
		if err != nil {
			return err
		}
		if referral.Status != referralmodels.ReferralUnderReview && referral.Status != referralmodels.ReferralSuspended {
			return apperrors.Conflict("This referral is not under review.", nil)
		}
		if err := s.store.SetReferralStatus(ctx, tx, referral.ID, referralmodels.ReferralActive, ""); err != nil {
			return err
		}
		frozen, err := s.store.RewardsForReferral(ctx, tx, referral.ID, []string{referralmodels.RewardFrozen})
		if err != nil {
			return err
		}
		released, err := s.releaseFrozen(ctx, tx, frozen)
		if err != nil {
			return err
		}
		details["released_rewards"] = released
		return s.checkMilestones(ctx, tx, referral.ReferrerPersonID)
	})
}

// DisqualifyReferral invalidates a referral permanently: it stops earning,
// drops out of its referrer's milestone count, and its unpaid rewards --
// including the referred person's own sign-up credits -- are cancelled
// (ReversePaid also takes back what was already paid).
func (s *Service) DisqualifyReferral(ctx context.Context, referralID string, req AdminRequest) error {
	return s.adminTx(ctx, req, ActionDisqualifyReferral, "referral", referralID, func(tx pgx.Tx, details map[string]any) error {
		referral, err := s.lockReferral(ctx, tx, referralID)
		if err != nil {
			return err
		}
		if referral.Status == referralmodels.ReferralDisqualified {
			return apperrors.Conflict("This referral is already disqualified.", nil)
		}
		if err := s.store.SetReferralStatus(ctx, tx, referral.ID, referralmodels.ReferralDisqualified, strings.TrimSpace(req.Reason)); err != nil {
			return err
		}
		rewards, err := s.store.RewardsForReferral(ctx, tx, referral.ID, nil)
		if err != nil {
			return err
		}
		cancelled, reversed, err := s.withdrawRewards(ctx, tx, rewards, referralmodels.ReasonDisqualified, req.ReversePaid)
		details["cancelled_rewards"], details["reversed_rewards"] = cancelled, reversed
		return err
	})
}

// ReverseReward takes back what is left of one reward: a fraud finding, a
// duplicate or a miscalculation. An unpaid reward is simply cancelled.
func (s *Service) ReverseReward(ctx context.Context, rewardID string, req AdminRequest) error {
	return s.adminTx(ctx, req, ActionReverseReward, "reward", rewardID, func(tx pgx.Tx, details map[string]any) error {
		reward, err := s.store.GetReward(ctx, tx, rewardID, true)
		if errors.Is(err, referralrepositories.ErrNotFound) {
			return apperrors.NotFound("No such reward.", nil)
		}
		if err != nil {
			return err
		}
		if reward.RewardType == referralmodels.RewardReversal {
			return apperrors.BadRequest("A take-back cannot itself be reversed.", nil)
		}
		if reward.Status == referralmodels.RewardRejected {
			return apperrors.Conflict("This reward was never paid.", nil)
		}
		changed, err := s.takeBackTo(ctx, tx, reward, reward.AmountKobo, referralmodels.ReasonAdminReversal)
		if err != nil {
			return err
		}
		if !changed {
			return apperrors.Conflict("This reward has already been taken back in full.", nil)
		}
		details["amount_kobo"] = reward.AmountKobo
		return nil
	})
}

// ResolveFlag records a reviewer's outcome for a flag: cleared (a false
// alarm) or upheld. It changes nothing else; freezing, clearing and
// disqualifying are separate, explicit actions.
func (s *Service) ResolveFlag(ctx context.Context, flagID, outcome string, req AdminRequest) error {
	if outcome != referralmodels.FlagCleared && outcome != referralmodels.FlagUpheld {
		return apperrors.Validation("Check the request.", []apperrors.FieldViolation{{Field: "outcome", Message: "Use cleared or upheld."}})
	}
	return s.adminTx(ctx, req, ActionResolveFlag, "flag", flagID, func(tx pgx.Tx, details map[string]any) error {
		flag, err := s.store.GetFlag(ctx, tx, flagID, true)
		if errors.Is(err, referralrepositories.ErrNotFound) {
			return apperrors.NotFound("No such flag.", nil)
		}
		if err != nil {
			return err
		}
		if flag.Status != referralmodels.FlagOpen {
			return apperrors.Conflict("This flag was already resolved.", nil)
		}
		details["outcome"] = outcome
		return s.store.ResolveFlag(ctx, tx, flag.ID, outcome, strings.TrimSpace(req.PerformedBy), strings.TrimSpace(req.Reason), s.now())
	})
}

// releaseFrozen pays frozen rewards whose holds have all been lifted and
// reports how many it released. A reward stays frozen while its person is
// frozen or its referral is still held, and is cancelled if either has since
// been blocked or disqualified.
func (s *Service) releaseFrozen(ctx context.Context, tx pgx.Tx, rewards []referralmodels.Reward) (int, error) {
	now := s.now()
	released := 0
	for _, reward := range rewards {
		if reward.AmountKobo <= 0 || reward.Status != referralmodels.RewardFrozen {
			continue
		}
		person, err := s.store.GetPerson(ctx, tx, reward.PersonID, false)
		if err != nil {
			return released, err
		}
		status, reason := referralmodels.RewardAvailable, referralmodels.ReasonCleared
		switch person.Status {
		case referralmodels.PersonBlocked:
			status, reason = referralmodels.RewardRejected, referralmodels.ReasonPersonBlocked
		case referralmodels.PersonFrozen:
			continue
		}
		if status == referralmodels.RewardAvailable && reward.ReferralID != "" {
			referral, err := s.store.GetReferral(ctx, tx, reward.ReferralID, false)
			if err != nil {
				return released, err
			}
			switch referral.Status {
			case referralmodels.ReferralDisqualified:
				status, reason = referralmodels.RewardRejected, referralmodels.ReasonDisqualified
			case referralmodels.ReferralUnderReview, referralmodels.ReferralSuspended:
				continue
			}
		}
		if err := s.store.SetRewardStatus(ctx, tx, reward.ID, status, reason, now); err != nil {
			return released, err
		}
		if status == referralmodels.RewardAvailable {
			released++
		}
	}
	return released, nil
}

// withdrawRewards cancels the unpaid rewards among these and, if asked, takes
// back the paid ones. It reports how many of each.
func (s *Service) withdrawRewards(ctx context.Context, tx pgx.Tx, rewards []referralmodels.Reward, reason string, reversePaid bool) (int, int, error) {
	now := s.now()
	cancelled, reversed := 0, 0
	for _, reward := range rewards {
		switch {
		case reward.PositiveOpen(now):
			if err := s.store.SetRewardStatus(ctx, tx, reward.ID, referralmodels.RewardRejected, reason, now); err != nil {
				return cancelled, reversed, err
			}
			cancelled++
		case reversePaid && reward.AmountKobo > 0 && reward.CommittedToWallet():
			changed, err := s.takeBackTo(ctx, tx, reward, reward.AmountKobo, referralmodels.ReasonAdminReversal)
			if err != nil {
				return cancelled, reversed, err
			}
			if changed {
				reversed++
			}
		}
	}
	return cancelled, reversed, nil
}

// ListFlags lists review flags, newest first (status "" lists all).
func (s *Service) ListFlags(ctx context.Context, status string, limit int) ([]referralmodels.Flag, error) {
	switch status {
	case "", referralmodels.FlagOpen, referralmodels.FlagCleared, referralmodels.FlagUpheld:
	default:
		return nil, apperrors.BadRequest("status must be open, cleared or upheld.", nil)
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.store.ListFlags(ctx, s.store.Pool(), status, limit)
}

// AccountOverview is what an operator sees about the person behind an
// account: their standing, accounts, referrals both ways, milestones, reward
// totals and recent rewards, and review flags.
type AccountOverview struct {
	Person                     PersonView                          `json:"person"`
	Code                       string                              `json:"code"`
	Accounts                   []AccountView                       `json:"accounts"`
	ReferredBy                 *ReferralView                       `json:"referred_by,omitempty"`
	Referrals                  []ReferralView                      `json:"referrals"`
	QualifiedCustomerReferrals int                                 `json:"qualified_customer_referrals"`
	Milestones                 []referralrepositories.Milestone    `json:"milestones"`
	RewardTotals               []referralrepositories.RewardTotals `json:"reward_totals"`
	RecentRewards              []RewardView                        `json:"recent_rewards"`
	Flags                      []referralmodels.Flag               `json:"flags"`
}

// PersonView is a person as operators see them.
type PersonView struct {
	ID             string     `json:"id"`
	Status         string     `json:"status"`
	StatusReason   string     `json:"status_reason,omitempty"`
	HomeActorType  string     `json:"home_actor_type"`
	HomeActorID    string     `json:"home_actor_id"`
	PowerUserSince *time.Time `json:"power_user_since,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// AccountView is one of a person's accounts.
type AccountView struct {
	ActorType          string    `json:"actor_type"`
	ActorID            string    `json:"actor_id"`
	RegisteredAt       time.Time `json:"registered_at"`
	IsNewAccount       bool      `json:"is_new_account"`
	AttributionOutcome string    `json:"attribution_outcome"`
}

// ReferralView is a referral as operators see it.
type ReferralView struct {
	ID               string    `json:"id"`
	ReferrerPersonID string    `json:"referrer_person_id"`
	ReferredPersonID string    `json:"referred_person_id"`
	CodeSource       string    `json:"code_source"`
	AttributedAt     time.Time `json:"attributed_at"`
	Status           string    `json:"status"`
	StatusReason     string    `json:"status_reason,omitempty"`
}

// RewardView is one reward ledger row.
type RewardView struct {
	ID                   string    `json:"id"`
	RewardType           string    `json:"reward_type"`
	Balance              string    `json:"balance"`
	AmountKobo           int64     `json:"amount_kobo"`
	Status               string    `json:"status"`
	StatusReason         string    `json:"status_reason,omitempty"`
	WalletState          string    `json:"wallet_state"`
	BeneficiaryActorType string    `json:"beneficiary_actor_type"`
	BeneficiaryActorID   string    `json:"beneficiary_actor_id"`
	ReferralID           string    `json:"referral_id,omitempty"`
	SourceService        string    `json:"source_service,omitempty"`
	SourceReference      string    `json:"source_reference,omitempty"`
	ReversesRewardID     string    `json:"reverses_reward_id,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
}

func referralView(r referralmodels.Referral) ReferralView {
	return ReferralView{ID: r.ID, ReferrerPersonID: r.ReferrerPersonID, ReferredPersonID: r.ReferredPersonID,
		CodeSource: r.CodeSource, AttributedAt: r.AttributedAt, Status: r.Status, StatusReason: r.StatusReason}
}

// AccountOverview gathers everything an operator needs about the person
// behind one account.
func (s *Service) AccountOverview(ctx context.Context, actorType, actorID string) (AccountOverview, error) {
	if !referralcontract.ValidActorType(actorType) || strings.TrimSpace(actorID) == "" {
		return AccountOverview{}, apperrors.BadRequest("A valid account type and id are required.", nil)
	}
	pool := s.store.Pool()
	actor, err := s.store.GetActor(ctx, pool, actorType, actorID)
	if errors.Is(err, referralrepositories.ErrNotFound) {
		return AccountOverview{}, apperrors.NotFound("This account is not in the referral programme yet.", nil)
	}
	if err != nil {
		return AccountOverview{}, err
	}
	var view AccountOverview
	err = func(tx database.Querier) error {
		person, err := s.store.GetPerson(ctx, tx, actor.PersonID, false)
		if err != nil {
			return err
		}
		view.Person = PersonView{ID: person.ID, Status: person.Status, StatusReason: person.StatusReason,
			HomeActorType: person.HomeActorType, HomeActorID: person.HomeActorID, PowerUserSince: person.PowerUserSince, CreatedAt: person.CreatedAt}
		if view.Code, err = s.store.CodeForPerson(ctx, tx, person.ID); err != nil && !errors.Is(err, referralrepositories.ErrNotFound) {
			return err
		}
		actors, err := s.store.ActorsForPerson(ctx, tx, person.ID)
		if err != nil {
			return err
		}
		for _, a := range actors {
			view.Accounts = append(view.Accounts, AccountView{ActorType: a.ActorType, ActorID: a.ActorID,
				RegisteredAt: a.RegisteredAt, IsNewAccount: a.IsNewAccount, AttributionOutcome: a.AttributionOutcome})
		}
		if referredBy, err := s.store.ReferralForReferred(ctx, tx, person.ID, false); err == nil {
			rv := referralView(referredBy)
			view.ReferredBy = &rv
		} else if !errors.Is(err, referralrepositories.ErrNotFound) {
			return err
		}
		referrals, err := s.store.ReferralsByReferrer(ctx, tx, person.ID, 100)
		if err != nil {
			return err
		}
		view.Referrals = []ReferralView{}
		for _, r := range referrals {
			view.Referrals = append(view.Referrals, referralView(r))
		}
		if view.QualifiedCustomerReferrals, err = s.store.CountQualifiedCustomerReferrals(ctx, tx, person.ID); err != nil {
			return err
		}
		if view.Milestones, err = s.store.MilestonesForPerson(ctx, tx, person.ID); err != nil {
			return err
		}
		if view.RewardTotals, err = s.store.RewardTotalsForPerson(ctx, tx, person.ID); err != nil {
			return err
		}
		rewards, err := s.store.RecentRewardsForPerson(ctx, tx, person.ID, 50)
		if err != nil {
			return err
		}
		view.RecentRewards = []RewardView{}
		for _, r := range rewards {
			view.RecentRewards = append(view.RecentRewards, RewardView{ID: r.ID, RewardType: r.RewardType, Balance: r.Balance,
				AmountKobo: r.AmountKobo, Status: r.Status, StatusReason: r.StatusReason, WalletState: r.WalletState,
				BeneficiaryActorType: r.BeneficiaryActorType, BeneficiaryActorID: r.BeneficiaryActorID, ReferralID: r.ReferralID,
				SourceService: r.SourceService, SourceReference: r.SourceReference, ReversesRewardID: r.ReversesRewardID, CreatedAt: r.CreatedAt})
		}
		view.Flags, err = s.store.FlagsForPerson(ctx, tx, person.ID)
		return err
	}(pool)
	return view, err
}
