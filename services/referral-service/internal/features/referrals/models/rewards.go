package referralmodels

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

// Reward types.
const (
	RewardCustomerReferralBonus = "customer_referral_bonus"
	RewardCustomerWelcomeCredit = "customer_welcome_credit"
	RewardFirstBookingCredit    = "first_booking_credit"
	RewardProviderCommission    = "provider_commission"
	RewardProviderWelcomeCredit = "provider_welcome_credit"
	RewardMilestoneFreeDelivery = "milestone_free_delivery"
	RewardMilestoneBonus        = "milestone_bonus"
	RewardReversal              = "reversal"
)

// Reward statuses.
const (
	// RewardPendingQualification: worked out, waiting for the referral to
	// qualify (a new driver's first jobs).
	RewardPendingQualification = "pending_qualification"
	// RewardPendingActivity: waiting for the "was the referrer active?"
	// decision.
	RewardPendingActivity = "pending_activity"
	// RewardAvailable: approved; posted to the wallet once wallet_state says so.
	RewardAvailable = "available"
	// RewardFrozen: approved but the person or referral is under review.
	RewardFrozen = "frozen"
	// RewardRejected: will not be paid (inactive referrer, disqualified, ...).
	RewardRejected = "rejected"
	// RewardReversed: was paid, then taken back by a reversal row.
	RewardReversed = "reversed"
)

// Balances a reward lands in.
const (
	BalanceEarnings    = "earnings"
	BalancePromotional = "promotional"
)

// Wallet posting states.
const (
	WalletNotSent = "not_sent"
	WalletPending = "pending"
	WalletPosted  = "posted"
)

// Reasons a reward was rejected, frozen or reversed.
const (
	ReasonReferrerInactive = "referrer_inactive"
	ReasonPersonBlocked    = "person_blocked"
	ReasonUnderReview      = "under_review"
	ReasonDisqualified     = "referral_disqualified"
	ReasonWindowExpired    = "window_expired"
	// ReasonJobReversed: the paid job behind the reward was refunded,
	// charged back or otherwise reversed.
	ReasonJobReversed = "job_reversed"
	// ReasonOriginalRejected: a take-back of a reward that was never paid.
	ReasonOriginalRejected = "original_rejected"
	// ReasonAdminReversal: an operator took the reward back (the reason they
	// gave is in referral_admin_actions).
	ReasonAdminReversal = "admin_reversal"
	// ReasonCleared: released from review by an operator.
	ReasonCleared = "cleared"
)

// Reward is one row of the reward ledger.
type Reward struct {
	ID                   string
	PersonID             string
	BeneficiaryActorType string
	BeneficiaryActorID   string
	ReferralID           string
	Model                string
	RewardType           string
	Balance              string
	AmountKobo           int64
	Currency             string
	Status               string
	StatusReason         string
	BasisKobo            int64
	RateBPS              int64
	SourceService        string
	SourceReference      string
	ActivityAsOf         *time.Time
	DecideAfter          *time.Time
	ReversesRewardID     string
	IdempotencyKey       string
	WalletState          string
	WalletReference      string
	WalletAttempts       int
	// WalletDueAt is when a reward queued for the wallet becomes postable.
	WalletDueAt *time.Time
	// WalletLeaseUntil is set while a worker is posting the reward.
	WalletLeaseUntil *time.Time
	CreatedAt        time.Time
}

// PositiveOpen reports a reward that has not been paid and may still be:
// held, waiting for a decision, frozen, or approved but still queued (and not
// being posted right now).
func (r Reward) PositiveOpen(now time.Time) bool {
	if r.AmountKobo <= 0 {
		return false
	}
	switch r.Status {
	case RewardPendingQualification, RewardPendingActivity, RewardFrozen:
		return r.WalletState != WalletPosted
	case RewardAvailable:
		return r.WalletState == WalletPending && !r.Leased(now)
	}
	return false
}

// Leased reports a reward a wallet worker is posting right now.
func (r Reward) Leased(now time.Time) bool {
	return r.WalletLeaseUntil != nil && r.WalletLeaseUntil.After(now)
}

// CommittedToWallet reports a reward the wallet has, or is about to have:
// posted, or approved and queued. Taking one back needs a wallet reversal;
// anything else can simply be cancelled.
func (r Reward) CommittedToWallet() bool {
	if r.WalletState == WalletPosted {
		return true
	}
	return r.WalletState == WalletPending && (r.Status == RewardAvailable || r.Status == RewardReversed)
}

// RewardKey derives a reward's idempotency key from its business identity
// (reward type, referral or person, and source job), never from an event id:
// the same job arriving again under a new event id -- an operator replay, a
// re-emit -- produces the same key and so the same single reward.
func RewardKey(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:])
}

// ReversalTarget is how much of a reward should have been taken back in
// total, given everything reversed so far on the job behind it. A fully
// reversed job takes all of it back. A partial reversal of a commission takes
// back the commission on the platform fee that was reversed, worked out from
// the cumulative reversed fee (never summed per reversal, so rounding cannot
// drift) with the rate snapshotted on the reward, and never more than the
// reward itself. A flat reward (the ₦500 bonus) only goes on a full reversal:
// a partly refunded booking was still a paid booking.
func ReversalTarget(reward Reward, cumulativeFeeReversedKobo int64, fullyReversed bool) int64 {
	if reward.AmountKobo <= 0 {
		return 0
	}
	if fullyReversed {
		return reward.AmountKobo
	}
	if reward.RewardType != RewardProviderCommission {
		return 0
	}
	fee := min(cumulativeFeeReversedKobo, reward.BasisKobo)
	return min(Commission(fee, reward.RateBPS), reward.AmountKobo)
}

// Commission is the referrer's share of the platform's cut on one job,
// rounded down. It can never exceed the platform fee itself.
func Commission(platformFeeKobo int64, rateBPS int64) int64 {
	if platformFeeKobo <= 0 || rateBPS <= 0 {
		return 0
	}
	amount := platformFeeKobo * rateBPS / 10_000
	return min(amount, platformFeeKobo)
}
