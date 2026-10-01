// Package providerauthclients holds provider_auth's adapters to other
// services.
package providerauthclients

import (
	"context"

	"github.com/jackc/pgx/v5"

	providerauthmodels "cosmicforge/logistics/services/hauling-service/internal/features/provider_auth/models"
	"cosmicforge/logistics/shared/go/referralcontract"
)

// ReferralAnnouncer tells the referral programme about providers who sign up
// themselves, by appending to this service's referral_outbox in the
// account-creation transaction. Only keyed hashes of the phone, email and
// device leave this service. Drivers created by a fleet organization are not
// announced: fleets are outside the personal referral programme.
type ReferralAnnouncer struct {
	feed *referralcontract.Feed
}

// NewReferralAnnouncer wraps the service's referral feed.
func NewReferralAnnouncer(feed *referralcontract.Feed) *ReferralAnnouncer {
	return &ReferralAnnouncer{feed: feed}
}

// AnnounceProvider implements providerauthrepositories.SignupAnnouncer.
func (a *ReferralAnnouncer) AnnounceProvider(ctx context.Context, tx pgx.Tx, provider providerauthmodels.Provider, signup providerauthmodels.Signup) error {
	return a.feed.AccountRegistered(ctx, tx, referralcontract.Account{
		ActorType:     referralcontract.ActorHauling,
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

// HaulingApprovals is what "fully approved" means for a truck provider: an
// active, self-registered provider whose onboarding is complete. (Profile
// submission moves a provider to pending_verification; only approval moves
// them to complete.)
var HaulingApprovals = referralcontract.ApprovalSource{
	ActorType: referralcontract.ActorHauling,
	Table:     "truck_providers",
	Approved:  "onboarding_status = 'complete' AND status = 'active' AND managed_by_organization_id IS NULL",
}
