package authclients

import (
	"context"

	"github.com/jackc/pgx/v5"

	authmodels "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/models"
	"cosmicforge/logistics/shared/go/referralcontract"
)

// ReferralAnnouncer tells the referral programme about newly created riders
// by appending to this service's referral_outbox in the identity-creation
// transaction. Only keyed hashes of the phone, email and device leave this
// service.
type ReferralAnnouncer struct {
	feed *referralcontract.Feed
}

// NewReferralAnnouncer wraps the service's referral feed.
func NewReferralAnnouncer(feed *referralcontract.Feed) *ReferralAnnouncer {
	return &ReferralAnnouncer{feed: feed}
}

// AnnounceRider implements authrepositories.SignupAnnouncer.
func (a *ReferralAnnouncer) AnnounceRider(ctx context.Context, tx pgx.Tx, identity authmodels.Identity, signup authmodels.Signup) error {
	email := ""
	if identity.Email != nil {
		email = *identity.Email
	}
	return a.feed.AccountRegistered(ctx, tx, referralcontract.Account{
		ActorType:    referralcontract.ActorDispatch,
		ActorID:      identity.ID,
		Phone:        identity.PhoneNumber,
		Email:        email,
		DeviceID:     signup.DeviceID,
		LinkCode:     signup.LinkCode,
		TypedCode:    signup.TypedCode,
		SignupAt:     identity.CreatedAt,
		IsNewAccount: true,
	})
}

// DispatchApprovals is what "fully approved" means for a rider: an active
// provider whose onboarding is complete and whose verification is
// "verified". The approval time is the fully_approved audit row when there
// is one.
var DispatchApprovals = referralcontract.ApprovalSource{
	ActorType: referralcontract.ActorDispatch,
	Table:     "providers",
	Approved:  "providers.verification_status = 'verified' AND providers.onboarding_complete AND providers.is_active",
	ApprovedAt: `COALESCE((SELECT min(a.created_at) FROM verification_audit a
		WHERE a.provider_id = providers.id AND a.action = 'fully_approved'), now())`,
}
