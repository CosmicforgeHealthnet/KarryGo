// Package referralmodels holds the referral programme's domain types and its
// pure rules: no I/O, so every rule is unit-testable on its own.
package referralmodels

import (
	"time"

	"cosmicforge/logistics/shared/go/referralcontract"
)

// SettledJob is a paid-out job as recorded by referral-service.
type SettledJob struct {
	SourceService        string
	SourceReference      string
	PaymentReference     string
	CustomerID           string
	ProviderActorType    string // "" when the job is outside the programme
	ProviderID           string
	WalletProviderType   string
	AmountKobo           int64
	PlatformFeeKobo      int64
	Currency             string
	JobCompletedAt       time.Time
	JobCompletedAtSource string
	SettledAt            time.Time
	InProgramme          bool
}

// SettledJobFrom converts the wallet's event into a job record. Only rides,
// deliveries and hauls (a driver actor type exists for the provider) are in
// the programme; vendor shop orders and anything else are recorded but have no
// referral effect.
func SettledJobFrom(p referralcontract.JobSettled) SettledJob {
	actor := referralcontract.ActorTypeForWalletProvider(p.ProviderType)
	return SettledJob{
		SourceService:        p.SourceService,
		SourceReference:      p.SourceReference,
		PaymentReference:     p.PaymentReference,
		CustomerID:           p.CustomerID,
		ProviderActorType:    actor,
		ProviderID:           p.ProviderID,
		WalletProviderType:   p.ProviderType,
		AmountKobo:           p.AmountKobo,
		PlatformFeeKobo:      p.PlatformFeeKobo,
		Currency:             p.Currency,
		JobCompletedAt:       p.JobCompletedAt.UTC(),
		JobCompletedAtSource: p.JobCompletedAtSource,
		SettledAt:            p.SettledAt.UTC(),
		InProgramme:          actor != "" && p.ProviderID != "",
	}
}

// ActivityRules is the "active referrer" rule: enough paid jobs within the
// window ending at the moment in question.
type ActivityRules struct {
	CustomerThreshold int
	ProviderThreshold int
	Window            time.Duration
}

// ActivityCounts are one person's paid jobs inside one window: bookings made
// as a customer, and jobs done across all of their driver accounts.
type ActivityCounts struct {
	CustomerJobs int
	ProviderJobs int
}

// IsActive applies the rule. Someone with both kinds of account is active if
// either side meets its own threshold.
func (r ActivityRules) IsActive(counts ActivityCounts) bool {
	return counts.CustomerJobs >= r.CustomerThreshold || counts.ProviderJobs >= r.ProviderThreshold
}

// Rules are the programme's numbers, snapshotted onto every reward so a later
// settings change never re-prices an old one.
type Rules struct {
	Activity                    ActivityRules
	CustomerReferralBonusKobo   int64
	CustomerWelcomeCreditKobo   int64
	FirstBookingCreditKobo      int64
	ProviderWelcomeCreditKobo   int64
	ProviderWelcomeWithdrawable bool
	ProviderCommissionBPS       int64
	ProviderActivationJobs      int
	ProviderWindow              time.Duration
	Milestone5CreditKobo        int64
	Milestone10BonusKobo        int64
	ActivityDecisionDelay       time.Duration
	// ReversalReviewThreshold is how many reversed jobs put a referral under
	// review; 0 turns the check off.
	ReversalReviewThreshold int
}

// JobsRequired is how many paid jobs qualify a track of the given model.
func (r Rules) JobsRequired(model string) int {
	if model == ModelProvider {
		return r.ProviderActivationJobs
	}
	return 1
}
