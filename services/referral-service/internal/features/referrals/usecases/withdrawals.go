package referralusecases

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/referral-service/internal/database"
	referralmodels "cosmicforge/logistics/services/referral-service/internal/features/referrals/models"
	referralrepositories "cosmicforge/logistics/services/referral-service/internal/features/referrals/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/referralcontract"
)

// CheckWithdrawal decides whether a driver may cash out referral earnings
// now; payment-wallet asks just before it reserves the money, and pays
// nothing unless the answer is yes.
//
// It also records the destination bank account against the person. The
// first time someone cashes out to an account that other people have used, a
// review flag is raised about them and about each earlier user, and any open
// review flag about a person -- like a freeze or a block -- stops their cash
// outs until a reviewer resolves it. Once a reviewer clears it, reusing the
// same account does not raise it again. The fingerprint and flags are kept
// even when the answer is no.
func (s *Service) CheckWithdrawal(ctx context.Context, request referralcontract.WithdrawalEligibilityRequest) (referralcontract.WithdrawalEligibility, error) {
	if err := request.Validate(); err != nil {
		return referralcontract.WithdrawalEligibility{}, apperrors.BadRequest("A driver account, an amount and a bank account hash are required.", err)
	}
	var answer referralcontract.WithdrawalEligibility
	err := database.WithinTx(ctx, s.store.Pool(), func(tx pgx.Tx) error {
		actor, err := s.store.GetActor(ctx, tx, request.ActorType, request.ActorID)
		if errors.Is(err, referralrepositories.ErrNotFound) {
			answer = referralcontract.WithdrawalEligibility{Reason: referralcontract.IneligibleUnknownAccount}
			return nil
		}
		if err != nil {
			return err
		}
		// Two people cashing out to one account at the same moment must see
		// each other.
		if err := s.store.LockIdentities(ctx, tx, "bank:"+request.BankAccountHash); err != nil {
			return err
		}
		person, err := s.store.GetPerson(ctx, tx, actor.PersonID, true)
		if err != nil {
			return err
		}
		others, err := s.store.PersonsSharingFingerprint(ctx, tx, "bank", request.BankAccountHash, person.ID)
		if err != nil {
			return err
		}
		isNew, err := s.store.RecordFingerprint(ctx, tx, "bank", request.BankAccountHash, person.ID)
		if err != nil {
			return err
		}
		if isNew && len(others) > 0 {
			if err := s.store.RaiseFlag(ctx, tx, person.ID, "", referralmodels.FlagDuplicateBank, referralmodels.SeverityReview,
				map[string]any{"actor_type": actor.ActorType, "actor_id": actor.ActorID, "shared_with_person_ids": others}); err != nil {
				return err
			}
			// The account's earlier users are held too: it is now shared.
			for _, other := range others {
				flagged, err := s.store.HasOpenPersonFlag(ctx, tx, other, referralmodels.FlagDuplicateBank)
				if err != nil {
					return err
				}
				if flagged {
					continue
				}
				if err := s.store.RaiseFlag(ctx, tx, other, "", referralmodels.FlagDuplicateBank, referralmodels.SeverityReview,
					map[string]any{"shared_with_person_ids": []string{person.ID}}); err != nil {
					return err
				}
			}
		}
		switch person.Status {
		case referralmodels.PersonBlocked:
			answer = referralcontract.WithdrawalEligibility{Reason: referralcontract.IneligibleBlocked}
			return nil
		case referralmodels.PersonFrozen:
			answer = referralcontract.WithdrawalEligibility{Reason: referralcontract.IneligibleUnderReview}
			return nil
		}
		held, err := s.store.HasOpenPersonFlag(ctx, tx, person.ID, "")
		if err != nil {
			return err
		}
		if held {
			answer = referralcontract.WithdrawalEligibility{Reason: referralcontract.IneligibleUnderReview}
			return nil
		}
		answer = referralcontract.WithdrawalEligibility{Eligible: true}
		return nil
	})
	return answer, err
}

// ReconciliationResult summarizes one reconciliation run.
type ReconciliationResult struct {
	Checked    int
	Mismatched int
}

// ReconcileWithWallet compares, account by account, what referral-service
// has sent with what payment-wallet's ledger holds. The wallet must hold at
// least every posting confirmed as done and at most that plus the postings
// still in flight (a crash between the wallet's answer and our record leaves
// one of those). Anything else is recorded for an operator and logged; a
// mismatch that clears is resolved automatically. It compares postings, not
// balances: balances also move with spending and withdrawals, which are the
// wallet's business.
func (s *Service) ReconcileWithWallet(ctx context.Context, pageSize int) (ReconciliationResult, error) {
	var result ReconciliationResult
	if s.wallet == nil {
		return result, nil
	}
	if pageSize <= 0 {
		pageSize = 200
	}
	pool := s.store.Pool()
	afterType, afterID := "", ""
	for {
		page, err := s.store.PostedTotalsPage(ctx, pool, afterType, afterID, pageSize)
		if err != nil {
			return result, err
		}
		for _, sent := range page {
			ownerType, providerType := "customer", ""
			if referralcontract.IsProviderActor(sent.ActorType) {
				ownerType, providerType = "provider", referralcontract.WalletProviderTypeForActor(sent.ActorType)
			}
			held, err := s.wallet.ReferralTotals(ctx, ownerType, sent.ActorID, providerType)
			if err != nil {
				return result, err
			}
			result.Checked++
			now := s.now()
			if within(held.CreditedKobo, sent.PostedCreditKobo, sent.InFlightCreditKobo) &&
				within(held.ReversedKobo, sent.PostedReversalKobo, sent.InFlightReversalKobo) {
				if err := s.store.ResolveMismatch(ctx, pool, sent.ActorType, sent.ActorID, now); err != nil {
					return result, err
				}
				continue
			}
			result.Mismatched++
			logging.Error("reconciliation", "%s %s: sent credits %d (+%d in flight), reversals %d (+%d in flight); wallet holds credits %d, reversals %d",
				sent.ActorType, sent.ActorID, sent.PostedCreditKobo, sent.InFlightCreditKobo, sent.PostedReversalKobo, sent.InFlightReversalKobo,
				held.CreditedKobo, held.ReversedKobo)
			if err := s.store.RecordMismatch(ctx, pool, sent, held.CreditedKobo, held.ReversedKobo, now); err != nil {
				return result, err
			}
		}
		if len(page) < pageSize {
			return result, nil
		}
		afterType, afterID = page[len(page)-1].ActorType, page[len(page)-1].ActorID
	}
}

func within(actual, confirmed, inFlight int64) bool {
	return actual >= confirmed && actual <= confirmed+inFlight
}

// OpenMismatches lists unresolved reconciliation mismatches for operators.
func (s *Service) OpenMismatches(ctx context.Context, limit int) ([]referralrepositories.Mismatch, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	return s.store.OpenMismatches(ctx, s.store.Pool(), limit)
}
