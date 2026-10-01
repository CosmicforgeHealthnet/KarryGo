package profileclients

import (
	"context"

	"github.com/jackc/pgx/v5"

	profilemodels "cosmicforge/logistics/services/customer-service/internal/features/profile/models"
	"cosmicforge/logistics/shared/go/referralcontract"
)

// ReferralAnnouncer tells the referral programme about newly created
// customers by appending to this service's referral_outbox in the
// account-creation transaction. Only keyed hashes of the phone, email and
// device leave this service.
type ReferralAnnouncer struct {
	feed *referralcontract.Feed
}

// NewReferralAnnouncer wraps the service's referral feed.
func NewReferralAnnouncer(feed *referralcontract.Feed) *ReferralAnnouncer {
	return &ReferralAnnouncer{feed: feed}
}

// AnnounceCustomer implements profilerepositories.SignupAnnouncer.
func (a *ReferralAnnouncer) AnnounceCustomer(ctx context.Context, tx pgx.Tx, customer profilemodels.Customer, signup profilemodels.Signup) error {
	return a.feed.AccountRegistered(ctx, tx, referralcontract.Account{
		ActorType:    referralcontract.ActorCustomer,
		ActorID:      customer.ID,
		Phone:        customer.Phone,
		Email:        customer.Email,
		DeviceID:     signup.DeviceID,
		LinkCode:     signup.LinkCode,
		TypedCode:    signup.TypedCode,
		SignupAt:     customer.CreatedAt,
		IsNewAccount: true,
	})
}
