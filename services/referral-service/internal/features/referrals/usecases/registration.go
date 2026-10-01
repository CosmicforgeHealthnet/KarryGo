package referralusecases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	referralmodels "cosmicforge/logistics/services/referral-service/internal/features/referrals/models"
	referralrepositories "cosmicforge/logistics/services/referral-service/internal/features/referrals/repositories"
	"cosmicforge/logistics/shared/go/referralcontract"
)

// handleAccountRegistered records a new (or backfilled) account: matches it to
// a person, makes sure that person has a code, and decides whether the
// sign-up is a referral.
func (s *Service) handleAccountRegistered(ctx context.Context, tx pgx.Tx, caller, eventType string, raw json.RawMessage) (Outcome, error) {
	var payload referralcontract.AccountRegistered
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", reject("invalid_payload")
	}
	if err := payload.Validate(); err != nil {
		return "", reject("invalid_account")
	}
	// Each service may only announce its own kind of account.
	switch eventType {
	case referralcontract.EventCustomerRegistered:
		if payload.ActorType != referralcontract.ActorCustomer {
			return "", reject("actor_type_mismatch")
		}
	case referralcontract.EventProviderRegistered:
		if payload.ActorType != providerActorFor(caller) {
			return "", reject("actor_type_mismatch")
		}
	}

	// Serialize every sign-up sharing this phone or email before looking
	// anything up, so two of them cannot both create "the" person.
	if err := s.store.LockIdentities(ctx, tx, "phone:"+payload.PhoneHash, "email:"+payload.EmailHash); err != nil {
		return "", err
	}
	if _, err := s.store.GetActor(ctx, tx, payload.ActorType, payload.ActorID); err == nil {
		return OutcomeDuplicate, nil
	} else if !errors.Is(err, referralrepositories.ErrNotFound) {
		return "", err
	}

	person, personIsNew, sharedDevice, err := s.resolvePerson(ctx, tx, payload)
	if err != nil {
		return "", err
	}
	actor := referralmodels.Actor{
		ActorType: payload.ActorType, ActorID: payload.ActorID, PersonID: person.ID,
		RegisteredAt: payload.SignupAt.UTC(), IsNewAccount: payload.IsNewAccount, LocationState: payload.LocationState,
	}
	if err := s.store.InsertActor(ctx, tx, actor); err != nil {
		return "", err
	}
	outcome, err := s.attribute(ctx, tx, person, personIsNew, actor, payload)
	if err != nil {
		return "", err
	}
	if err := s.store.SetAttributionOutcome(ctx, tx, actor.ActorType, actor.ActorID, outcome); err != nil {
		return "", err
	}
	if payload.IsNewAccount {
		if err := s.reviewSharedDevice(ctx, tx, person, actor, sharedDevice); err != nil {
			return "", err
		}
	}
	// The account's paid jobs, and a driver's approval, may have arrived
	// before its announcement.
	if err := s.catchUp(ctx, tx, actor); err != nil {
		return "", err
	}
	if referralcontract.IsProviderActor(actor.ActorType) {
		if err := s.refreshProviderTrack(ctx, tx, actor); err != nil {
			return "", err
		}
	}
	return OutcomeProcessed, nil
}

// resolvePerson finds the person an account belongs to, by phone first (the
// primary identifier) and then email, creating one -- with a code -- if
// nobody matches. It also reports the other people the sign-up's device has
// been seen with.
func (s *Service) resolvePerson(ctx context.Context, tx pgx.Tx, payload referralcontract.AccountRegistered) (referralmodels.Person, bool, []string, error) {
	person, personIsNew, err := s.matchPerson(ctx, tx, payload)
	if err != nil {
		return referralmodels.Person{}, false, nil, err
	}
	sharedDevice, err := s.store.PersonsSharingFingerprint(ctx, tx, "device", payload.DeviceHash, person.ID)
	if err != nil {
		return referralmodels.Person{}, false, nil, err
	}
	for kind, hash := range map[string]string{"phone": payload.PhoneHash, "email": payload.EmailHash, "device": payload.DeviceHash} {
		if _, err := s.store.AddFingerprint(ctx, tx, kind, hash, person.ID); err != nil {
			return referralmodels.Person{}, false, nil, err
		}
	}
	return person, personIsNew, sharedDevice, nil
}

func (s *Service) matchPerson(ctx context.Context, tx pgx.Tx, payload referralcontract.AccountRegistered) (referralmodels.Person, bool, error) {
	phoneOwner, err := s.lookupFingerprint(ctx, tx, "phone", payload.PhoneHash)
	if err != nil {
		return referralmodels.Person{}, false, err
	}
	emailOwner, err := s.lookupFingerprint(ctx, tx, "email", payload.EmailHash)
	if err != nil {
		return referralmodels.Person{}, false, err
	}

	var person referralmodels.Person
	personIsNew := false
	switch {
	case phoneOwner == "" && emailOwner == "":
		person, err = s.store.CreatePerson(ctx, tx, payload.ActorType, payload.ActorID)
		if err != nil {
			return referralmodels.Person{}, false, err
		}
		if err := s.issueCode(ctx, tx, person.ID); err != nil {
			return referralmodels.Person{}, false, err
		}
		personIsNew = true
	case phoneOwner != "":
		person, err = s.store.GetPerson(ctx, tx, phoneOwner, false)
		if err != nil {
			return referralmodels.Person{}, false, err
		}
		if emailOwner != "" && emailOwner != phoneOwner {
			// The phone says one person, the email another. Link by phone and
			// let a human decide whether these are really the same person.
			if err := s.store.RaiseFlag(ctx, tx, person.ID, "", referralmodels.FlagIdentityConflict, referralmodels.SeverityReview,
				map[string]any{"actor_type": payload.ActorType, "actor_id": payload.ActorID, "email_person_id": emailOwner}); err != nil {
				return referralmodels.Person{}, false, err
			}
		}
	default:
		person, err = s.store.GetPerson(ctx, tx, emailOwner, false)
		if err != nil {
			return referralmodels.Person{}, false, err
		}
	}
	return person, personIsNew, nil
}

func (s *Service) lookupFingerprint(ctx context.Context, tx pgx.Tx, kind, hash string) (string, error) {
	personID, err := s.store.PersonIDByFingerprint(ctx, tx, kind, hash)
	if errors.Is(err, referralrepositories.ErrNotFound) {
		return "", nil
	}
	return personID, err
}

// issueCode gives a new person a unique code, retrying the (vanishingly
// rare) collision.
func (s *Service) issueCode(ctx context.Context, tx pgx.Tx, personID string) error {
	for attempt := 0; attempt < 8; attempt++ {
		code, err := referralmodels.GenerateCode(s.random)
		if err != nil {
			return err
		}
		inserted, err := s.store.InsertCode(ctx, tx, code, personID)
		if err != nil {
			return err
		}
		if inserted {
			return nil
		}
	}
	return fmt.Errorf("could not issue a unique referral code for %s", personID)
}

// attribute decides what a sign-up's code means. The order matters:
//  1. A person who was referred before keeps that one referral forever; a new
//     kind of account (customer turned driver, or the reverse) joins it as
//     another track, whatever code was typed this time.
//  2. Backfilled accounts and sign-ups without a code are never referrals.
//  3. A code must exist and must not be the person's own.
//  4. Only people genuinely new to the platform can be referred.
func (s *Service) attribute(ctx context.Context, tx pgx.Tx, person referralmodels.Person, personIsNew bool, actor referralmodels.Actor, payload referralcontract.AccountRegistered) (string, error) {
	code := referralmodels.NormalizeCode(payload.ReferralCode())

	existing, err := s.store.ReferralForReferred(ctx, tx, person.ID, true)
	if err == nil {
		if code != "" {
			if owner, lookupErr := s.store.PersonIDByCode(ctx, tx, code); lookupErr == nil && owner != existing.ReferrerPersonID && owner != person.ID {
				if err := s.store.RaiseFlag(ctx, tx, person.ID, existing.ID, referralmodels.FlagDuplicateIdentity, referralmodels.SeverityReview,
					map[string]any{"actor_type": actor.ActorType, "actor_id": actor.ActorID, "second_referrer_person_id": owner}); err != nil {
					return "", err
				}
			}
		}
		if _, err := s.ensureTrack(ctx, tx, existing, actor); err != nil {
			return "", err
		}
		return referralmodels.AttributionAlreadyReferred, nil
	}
	if !errors.Is(err, referralrepositories.ErrNotFound) {
		return "", err
	}

	if !payload.IsNewAccount {
		return referralmodels.AttributionBackfill, nil
	}
	if code == "" {
		return referralmodels.AttributionNoCode, nil
	}
	if referralmodels.ValidateCode(code) != nil {
		return referralmodels.AttributionInvalidCode, nil
	}
	referrerID, err := s.store.PersonIDByCode(ctx, tx, code)
	if errors.Is(err, referralrepositories.ErrNotFound) {
		return referralmodels.AttributionInvalidCode, nil
	}
	if err != nil {
		return "", err
	}
	if referrerID == person.ID {
		if err := s.store.RaiseFlag(ctx, tx, person.ID, "", referralmodels.FlagSelfReferral, referralmodels.SeverityInfo,
			map[string]any{"actor_type": actor.ActorType, "actor_id": actor.ActorID}); err != nil {
			return "", err
		}
		return referralmodels.AttributionSelfReferral, nil
	}
	if !personIsNew {
		return referralmodels.AttributionExistingPerson, nil
	}
	referrer, err := s.store.GetPerson(ctx, tx, referrerID, true)
	if err != nil {
		return "", err
	}
	if referrer.Status == referralmodels.PersonBlocked {
		return referralmodels.AttributionReferrerBlocked, nil
	}

	source := "typed"
	if payload.LinkCode != "" {
		source = "link"
	}
	referral, err := s.store.CreateReferral(ctx, tx, referralmodels.Referral{
		ReferrerPersonID: referrerID, ReferredPersonID: person.ID, Code: code, CodeSource: source,
		AttributedAt: actor.RegisteredAt,
	})
	if err != nil {
		return "", err
	}
	if _, err := s.ensureTrack(ctx, tx, referral, actor); err != nil {
		return "", err
	}
	return referralmodels.AttributionAttributed, nil
}

// ensureTrack opens a referral's track for the model of the account that
// triggered it, once, and grants whatever the referred person receives when
// that track starts.
func (s *Service) ensureTrack(ctx context.Context, tx pgx.Tx, referral referralmodels.Referral, actor referralmodels.Actor) (bool, error) {
	model := referralmodels.ModelForActor(actor.ActorType)
	onboardedAt, err := s.personOnboardedAt(ctx, tx, referral.ReferredPersonID)
	if err != nil {
		return false, err
	}
	var trackOnboarded = onboardedAt
	if model != referralmodels.ModelProvider {
		trackOnboarded = nil
	}
	created, err := s.store.EnsureTrack(ctx, tx, referral.ID, model, s.rules.JobsRequired(model), trackOnboarded)
	if err != nil || !created {
		return created, err
	}
	return true, s.onTrackOpened(ctx, tx, referral, actor)
}

// personOnboardedAt is when the person's first driver account was fully
// approved, if any.
func (s *Service) personOnboardedAt(ctx context.Context, tx pgx.Tx, personID string) (*time.Time, error) {
	return s.store.PersonOnboardedAt(ctx, tx, personID)
}
