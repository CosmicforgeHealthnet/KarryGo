package referralusecases

import (
	"context"
	"errors"

	referralmodels "cosmicforge/logistics/services/referral-service/internal/features/referrals/models"
	referralrepositories "cosmicforge/logistics/services/referral-service/internal/features/referrals/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
)

// MyCodeView is what an account holder sees about their own code.
type MyCodeView struct {
	Code      string `json:"code"`
	ShareLink string `json:"share_link"`
	// Status is the person's standing in the programme: active, frozen
	// (under review) or blocked.
	Status string `json:"status"`
	// QualifiedCustomerReferrals is the count the 5- and 10-referral
	// milestones use.
	QualifiedCustomerReferrals int `json:"qualified_customer_referrals"`
	// PowerUser is the badge earned at 10 qualified customer referrals.
	PowerUser bool `json:"power_user"`
}

// MyCode returns the code and share link of the person behind an account.
func (s *Service) MyCode(ctx context.Context, actorType, actorID string) (MyCodeView, error) {
	pool := s.store.Pool()
	actor, err := s.store.GetActor(ctx, pool, actorType, actorID)
	if errors.Is(err, referralrepositories.ErrNotFound) {
		// The sign-up announcement has not arrived yet (it is relayed
		// asynchronously) or the account predates the programme and the
		// backfill has not run. Either way it resolves on its own.
		return MyCodeView{}, apperrors.NotFound("Your referral code is being set up. Please try again shortly.", nil)
	}
	if err != nil {
		return MyCodeView{}, err
	}
	person, err := s.store.GetPerson(ctx, pool, actor.PersonID, false)
	if err != nil {
		return MyCodeView{}, err
	}
	code, err := s.store.CodeForPerson(ctx, pool, person.ID)
	if err != nil {
		return MyCodeView{}, err
	}
	qualified, err := s.store.CountQualifiedCustomerReferrals(ctx, pool, person.ID)
	if err != nil {
		return MyCodeView{}, err
	}
	return MyCodeView{
		Code: code, ShareLink: s.shareLinkBase + code, Status: person.Status,
		QualifiedCustomerReferrals: qualified, PowerUser: person.PowerUserSince != nil,
	}, nil
}

// CheckCode tells a sign-up screen whether a code exists. It is advisory
// only -- the sign-up itself never waits on it -- and deliberately reveals
// nothing about who owns the code.
func (s *Service) CheckCode(ctx context.Context, raw string) (bool, error) {
	code := referralmodels.NormalizeCode(raw)
	if referralmodels.ValidateCode(code) != nil {
		return false, nil
	}
	personID, err := s.store.PersonIDByCode(ctx, s.store.Pool(), code)
	if errors.Is(err, referralrepositories.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	person, err := s.store.GetPerson(ctx, s.store.Pool(), personID, false)
	if err != nil {
		return false, err
	}
	return person.Status != referralmodels.PersonBlocked, nil
}
