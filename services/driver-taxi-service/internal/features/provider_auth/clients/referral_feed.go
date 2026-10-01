// Package providerauthclients holds provider_auth's adapters to other
// services.
package providerauthclients

import (
	"context"

	"github.com/jackc/pgx/v5"

	providerauthmodels "cosmicforge/logistics/services/taxi-service/internal/features/provider_auth/models"
	"cosmicforge/logistics/shared/go/referralcontract"
)

// ReferralAnnouncer tells the referral programme about newly created taxi
// drivers by appending to this service's referral_outbox in the
// account-creation transaction. Only keyed hashes of the phone and device
// leave this service.
type ReferralAnnouncer struct {
	feed *referralcontract.Feed
}

// NewReferralAnnouncer wraps the service's referral feed.
func NewReferralAnnouncer(feed *referralcontract.Feed) *ReferralAnnouncer {
	return &ReferralAnnouncer{feed: feed}
}

// AnnounceDriver implements providerauthrepositories.SignupAnnouncer.
func (a *ReferralAnnouncer) AnnounceDriver(ctx context.Context, tx pgx.Tx, provider providerauthmodels.Provider, signup providerauthmodels.Signup) error {
	return a.feed.AccountRegistered(ctx, tx, referralcontract.Account{
		ActorType:     referralcontract.ActorTaxi,
		ActorID:       provider.ID,
		Phone:         provider.Phone,
		Email:         provider.Email,
		DeviceID:      signup.DeviceID,
		LinkCode:      signup.LinkCode,
		TypedCode:     signup.TypedCode,
		LocationState: provider.LocationState,
		SignupAt:      provider.CreatedAt,
		IsNewAccount:  true,
	})
}

// TaxiApprovals is what "fully approved" means for a taxi driver: the same
// status dimensions a driver must satisfy to go available (account active,
// onboarding complete, identity and vehicle verified, documents compliant).
var TaxiApprovals = referralcontract.ApprovalSource{
	ActorType: referralcontract.ActorTaxi,
	Table:     "taxi_providers",
	Approved: "account_status = 'active' AND onboarding_status = 'complete' AND identity_verification_status = 'verified' " +
		"AND vehicle_verification_status = 'verified' AND document_compliance_status = 'compliant'",
}
