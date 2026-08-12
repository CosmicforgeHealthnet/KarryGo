package usecases

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/payment-wallet-service/internal/database"
	"cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/models"
	"cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/repositories"
	"cosmicforge/logistics/services/payment-wallet-service/internal/platform/audit"
	"cosmicforge/logistics/services/payment-wallet-service/internal/platform/financeauth"
	"cosmicforge/logistics/services/payment-wallet-service/internal/platform/money"
)

// Section 13 events.
const (
	EventSubWalletCreated       = "wallet.provider_subwallet.created.v1"
	EventSubWalletStatusChanged = "wallet.provider_subwallet.status_changed.v1"
	EventAllocationTransferred  = "wallet.provider_allocation.posted.v1"
	EventAllocationReversed     = "wallet.provider_allocation.reversed.v1"
	EventWithdrawalRequested    = "wallet.withdrawal.requested.v1"
	EventWithdrawalApproved     = "wallet.withdrawal.approved.v1"
	EventWithdrawalRejected     = "wallet.withdrawal.rejected.v1"
	EventWithdrawalCompleted    = "wallet.withdrawal.completed.v1"
	EventWithdrawalFailed       = "wallet.withdrawal.failed.v1"
)

// ErrWithdrawalsDisabled means the asserted organization policy forbids the
// request outright.
var ErrWithdrawalsDisabled = errors.New("finance: withdrawals are disabled for this account")

// ErrAccountNotOperational means the wallet or sub-wallet is not in a state
// that may move money.
var ErrAccountNotOperational = errors.New("finance: account is not operational")

// ErrWithdrawalGatewayUnavailable means no payout gateway is wired, so no
// withdrawal may be executed. Refusing to run is deliberate: a worker that
// "succeeds" against a missing gateway would mark payouts complete that never
// left the platform.
var ErrWithdrawalGatewayUnavailable = errors.New("finance: provider payout gateway is not configured")

// EnsureProviderSubWalletInput carries the corporate ownership a Section 13
// sub-wallet must associate.
type EnsureProviderSubWalletInput struct {
	Meta                     Meta
	OrganizationID           string
	OrganizationWalletID     string
	ProviderID               string
	OrganizationMembershipID string
	ProviderService          string
	ProviderServiceCategory  string
	ProviderReference        string
	Currency                 string
}

// EnsureProviderSubWallet creates or returns the provider's sub-wallet and
// binds its corporate ownership.
//
// It reuses the existing earning account rather than introducing a second
// provider account model: the account already carries the currency, the
// status, the ledger link and the version. What Section 13 adds is who owns
// it, and that is recorded on the same row so the two can never disagree.
func (s *Service) EnsureProviderSubWallet(ctx context.Context, input EnsureProviderSubWalletInput) (*models.EarningAccount, error) {
	if err := input.Meta.validate(); err != nil {
		return nil, err
	}
	currency, err := money.New(0, input.Currency)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.OrganizationID) == "" ||
		strings.TrimSpace(input.OrganizationMembershipID) == "" {
		return nil, fmt.Errorf("%w: organization and membership are required", ErrInvalid)
	}
	if input.Meta.Actor.OrganizationID != "" && input.Meta.Actor.OrganizationID != input.OrganizationID {
		return nil, financeauth.ErrForbidden
	}
	if err := s.require(ctx, input.Meta, financeauth.PermWalletView); err != nil {
		return nil, err
	}
	providerService := defaulted(input.ProviderService, "fleet-partner-service")
	providerReference := defaulted(input.ProviderReference, input.OrganizationMembershipID)
	category := strings.ToUpper(defaulted(input.ProviderServiceCategory, "UNSPECIFIED"))

	var result *models.EarningAccount
	err = database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		// The organization wallet is ensured first: a provider sub-wallet that
		// is not attached to a funding wallet cannot receive an allocation, and
		// discovering that at allocation time is too late.
		wallet, _, walletErr := s.accounts.EnsureWallet(ctx, tx, input.OrganizationID, currency.Currency)
		if walletErr != nil {
			return walletErr
		}
		if input.OrganizationWalletID != "" && input.OrganizationWalletID != wallet.ID {
			return fmt.Errorf("%w: organization wallet does not match", financeauth.ErrForbidden)
		}
		account, created, accountErr := s.accounts.EnsureEarningAccount(ctx, tx,
			providerService, providerReference, input.OrganizationID, currency.Currency)
		if accountErr != nil {
			return accountErr
		}
		if ownershipErr := s.providerOps.SetSubWalletOwnership(ctx, tx, account.ID, wallet.ID,
			input.ProviderID, input.OrganizationMembershipID, category); ownershipErr != nil {
			return ownershipErr
		}
		refreshed, refreshErr := s.accounts.GetEarningAccountByID(ctx, tx, account.ID)
		if refreshErr != nil {
			return refreshErr
		}
		result = refreshed
		if !created {
			return nil
		}
		if historyErr := repositories.AppendHistory(ctx, tx, input.OrganizationID, "PROVIDER_SUBWALLET",
			account.ID, "", account.Status, input.Meta.Actor.Type, input.Meta.Actor.ID, "created",
			map[string]any{"membershipId": input.OrganizationMembershipID, "serviceCategory": category},
			input.Meta.CorrelationID, input.Meta.CausationID); historyErr != nil {
			return historyErr
		}
		if auditErr := s.audit.Record(ctx, tx, audit.Entry{OrganizationID: input.OrganizationID,
			ActorType: input.Meta.Actor.Type, ActorID: input.Meta.Actor.ID,
			Action: "provider_subwallet.created", ResourceType: "provider_subwallet",
			ResourceID: account.ID, NewValues: map[string]any{"currency": currency.Currency,
				"serviceCategory": category, "status": account.Status},
			RequestID: input.Meta.RequestID, CorrelationID: input.Meta.CorrelationID}); auditErr != nil {
			return auditErr
		}
		return s.events.Publish(ctx, tx, EventSubWalletCreated, "PROVIDER_SUBWALLET", account.ID,
			refreshed.Version, input.OrganizationID, input.Meta.CorrelationID, input.Meta.CausationID,
			map[string]any{"providerSubWalletId": account.ID, "organizationWalletId": wallet.ID,
				"organizationMembershipId": input.OrganizationMembershipID,
				"providerServiceCategory":  category, "currency": currency.Currency,
				"status": account.Status})
	})
	return result, err
}

func defaulted(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// ProviderAllocationInput is one company-authorized movement request.
type ProviderAllocationInput struct {
	Meta                     Meta
	OrganizationID           string
	EarningAccountID         string
	OrganizationMembershipID string
	ProviderID               string
	AllocationType           string
	BusinessReferenceType    string
	BusinessReferenceID      string
	AmountMinor              int64
	Currency                 string
	ReasonCode               string
	ProviderExplanation      string
	ApprovalReference        string
	SourceService            string
	SourceReference          string
	ExpectedVersion          int64
}

// PostProviderAllocation moves value between an organization wallet and one
// provider sub-wallet.
//
// This is deliberately a different operation from PostEarningAllocation. That
// one splits a customer's payment across the platform, the organization and
// the provider at the moment work is paid for. This one is a later, separately
// authorized company decision, and the two must remain distinct journals so a
// provider statement can never be used to reconstruct the customer's fare.
func (s *Service) PostProviderAllocation(ctx context.Context, input ProviderAllocationInput) (*models.ProviderAllocation, error) {
	if err := input.Meta.validate(); err != nil {
		return nil, err
	}
	if err := s.require(ctx, input.Meta, financeauth.PermProviderAllocationPost); err != nil {
		return nil, err
	}
	currency, err := money.New(0, input.Currency)
	if err != nil {
		return nil, err
	}
	allocationType := strings.ToUpper(strings.TrimSpace(input.AllocationType))
	referenceType := strings.ToUpper(strings.TrimSpace(input.BusinessReferenceType))
	if err := models.ValidateAllocation(allocationType, referenceType, input.BusinessReferenceID,
		currency.Currency, input.AmountMinor); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalid, err.Error())
	}
	if input.Meta.Actor.OrganizationID != "" && input.Meta.Actor.OrganizationID != input.OrganizationID {
		return nil, financeauth.ErrForbidden
	}
	if strings.TrimSpace(input.ReasonCode) == "" {
		return nil, fmt.Errorf("%w: reason code is required", ErrInvalid)
	}
	// A deduction takes money away from someone who is not in the room. It gets
	// its own permission on top of the allocation permission, so an operator who
	// may pay a provider cannot thereby also charge one.
	if allocationType == models.AllocationApprovedDeduction || allocationType == models.AllocationReversal {
		if err := s.require(ctx, input.Meta, financeauth.PermProviderAllocationReverse); err != nil {
			return nil, err
		}
		if strings.TrimSpace(input.ProviderExplanation) == "" {
			return nil, fmt.Errorf("%w: a deduction requires a provider visible explanation", ErrInvalid)
		}
	}

	fp := fingerprint(input.OrganizationID, input.EarningAccountID, allocationType, referenceType,
		input.BusinessReferenceID, input.AmountMinor, currency.Currency, input.ReasonCode)
	if existing, findErr := s.providerOps.FindAllocationByKey(ctx, nil, input.OrganizationID,
		input.Meta.IdempotencyKey); findErr == nil {
		if existing.RequestFingerprint != fp {
			return nil, ErrIdempotencyReuse
		}
		return existing, nil
	} else if !errors.Is(findErr, repositories.ErrNotFound) {
		return nil, findErr
	}

	var allocation *models.ProviderAllocation
	err = database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		account, accountErr := s.accounts.LockEarningAccount(ctx, tx, input.EarningAccountID)
		if accountErr != nil {
			return accountErr
		}
		if account.OrganizationID != input.OrganizationID || account.Currency != currency.Currency {
			return financeauth.ErrForbidden
		}
		if !account.Payable() {
			return fmt.Errorf("%w: provider sub-wallet is %s", ErrAccountNotOperational, account.Status)
		}
		ownership, ownershipErr := s.providerOps.GetSubWalletOwnership(ctx, tx, account.ID)
		if ownershipErr != nil {
			return ownershipErr
		}
		if input.OrganizationMembershipID != "" && ownership.OrganizationMembershipID != "" &&
			ownership.OrganizationMembershipID != input.OrganizationMembershipID {
			return financeauth.ErrForbidden
		}
		wallet, walletErr := s.accounts.GetWallet(ctx, tx, input.OrganizationID, currency.Currency)
		if walletErr != nil {
			return walletErr
		}
		locked, lockErr := s.accounts.LockWallet(ctx, tx, wallet.ID)
		if lockErr != nil {
			return lockErr
		}
		wallet = locked
		if !wallet.Spendable() {
			return fmt.Errorf("%w: organization wallet is %s", ErrAccountNotOperational, wallet.Status)
		}
		if input.ExpectedVersion != 0 && input.ExpectedVersion != account.Version {
			return repositories.ErrVersionMismatch
		}

		debitsProvider := models.DebitsProvider(allocationType)
		journalType := models.JournalProviderAllocation
		lines := []repositories.PostingLine{
			{LedgerAccountID: wallet.LedgerAccountID, Side: models.SideDebit,
				AmountMinor: input.AmountMinor, Memo: "provider allocation"},
			{LedgerAccountID: account.LedgerAccountID, Side: models.SideCredit,
				AmountMinor: input.AmountMinor, Memo: "provider sub-wallet credit"},
		}
		if debitsProvider {
			journalType = models.JournalProviderDeduction
			if allocationType == models.AllocationReversal {
				journalType = models.JournalProviderAllocationReversal
			}
			lines = []repositories.PostingLine{
				{LedgerAccountID: account.LedgerAccountID, Side: models.SideDebit,
					AmountMinor: input.AmountMinor, Memo: "provider sub-wallet debit"},
				{LedgerAccountID: wallet.LedgerAccountID, Side: models.SideCredit,
					AmountMinor: input.AmountMinor, Memo: "approved deduction"},
			}
			if account.PayableMinor < input.AmountMinor {
				return ErrInsufficientFunds
			}
		} else if wallet.AvailableMinor < input.AmountMinor {
			return ErrInsufficientFunds
		}

		journal, postErr := s.ledger.Post(ctx, tx, repositories.Posting{
			OrganizationID: input.OrganizationID, JournalType: journalType,
			Currency: currency.Currency, Reference: input.BusinessReferenceID,
			SourceService:   defaulted(input.SourceService, "fleet-partner-service"),
			SourceReference: defaulted(input.SourceReference, input.BusinessReferenceID),
			IdempotencyKey:  input.Meta.IdempotencyKey + ":journal", Fingerprint: fp,
			CorrelationID: input.Meta.CorrelationID, CausationID: input.Meta.CausationID,
			ActorType: input.Meta.Actor.Type, ActorID: input.Meta.Actor.ID,
			Memo: input.ReasonCode, Lines: lines})
		if postErr != nil {
			return postErr
		}

		walletDelta, providerDelta := -input.AmountMinor, input.AmountMinor
		if debitsProvider {
			walletDelta, providerDelta = input.AmountMinor, -input.AmountMinor
		}
		if moveErr := s.accounts.MoveWalletBalances(ctx, tx, wallet.ID, wallet.Version,
			walletDelta, 0, 0); moveErr != nil {
			return moveErr
		}
		if moveErr := s.accounts.MoveEarningBalances(ctx, tx, account.ID, account.Version,
			providerDelta, 0, 0); moveErr != nil {
			return moveErr
		}

		allocation = &models.ProviderAllocation{
			OrganizationID: input.OrganizationID, OrganizationWalletID: wallet.ID,
			EarningAccountID: account.ID, ProviderID: defaulted(input.ProviderID, ownership.ProviderID),
			OrganizationMembershipID: defaulted(input.OrganizationMembershipID, ownership.OrganizationMembershipID),
			AllocationType:           allocationType, BusinessReferenceType: referenceType,
			BusinessReferenceID: input.BusinessReferenceID, AmountMinor: input.AmountMinor,
			Currency: currency.Currency, ReasonCode: input.ReasonCode,
			ProviderExplanation: input.ProviderExplanation, JournalID: journal.ID,
			SourceService:   defaulted(input.SourceService, "fleet-partner-service"),
			SourceReference: input.SourceReference, ApprovalReference: input.ApprovalReference,
			RequestedByActorType: input.Meta.Actor.Type, RequestedByActorID: input.Meta.Actor.ID,
			IdempotencyKey: input.Meta.IdempotencyKey, RequestFingerprint: fp,
			CorrelationID: input.Meta.CorrelationID, CausationID: input.Meta.CausationID,
		}
		if createErr := s.providerOps.CreateAllocation(ctx, tx, allocation); createErr != nil {
			return createErr
		}
		if historyErr := repositories.AppendHistory(ctx, tx, input.OrganizationID, "PROVIDER_ALLOCATION",
			allocation.ID, "", allocation.Status, input.Meta.Actor.Type, input.Meta.Actor.ID,
			"posted", map[string]any{"journalId": journal.ID, "allocationType": allocationType},
			input.Meta.CorrelationID, input.Meta.CausationID); historyErr != nil {
			return historyErr
		}
		if auditErr := s.audit.Record(ctx, tx, audit.Entry{OrganizationID: input.OrganizationID,
			ActorType: input.Meta.Actor.Type, ActorID: input.Meta.Actor.ID,
			Action: "provider_allocation.posted", ResourceType: "provider_allocation",
			ResourceID: allocation.ID, NewValues: map[string]any{"amountMinor": input.AmountMinor,
				"currency": currency.Currency, "allocationType": allocationType,
				"reasonCode": input.ReasonCode},
			RequestID: input.Meta.RequestID, CorrelationID: input.Meta.CorrelationID}); auditErr != nil {
			return auditErr
		}
		// The published payload carries no wallet balance and no customer
		// amount: a consumer that learns an allocation happened must not thereby
		// learn what the organization earned on the job.
		return s.events.Publish(ctx, tx, EventAllocationTransferred, "PROVIDER_ALLOCATION",
			allocation.ID, allocation.Version, input.OrganizationID, input.Meta.CorrelationID,
			input.Meta.CausationID, map[string]any{"allocationId": allocation.ID,
				"providerSubWalletId": account.ID, "allocationType": allocationType,
				"businessReferenceType": referenceType, "businessReferenceId": input.BusinessReferenceID,
				"amountMinor": input.AmountMinor, "currency": currency.Currency,
				"reasonCode": input.ReasonCode, "status": allocation.Status})
	})
	if err != nil {
		return nil, err
	}
	return allocation, nil
}

// GetProviderAllocation reads one allocation inside the tenant's scope.
func (s *Service) GetProviderAllocation(ctx context.Context, meta Meta, organizationID,
	id string) (*models.ProviderAllocation, error) {
	if err := s.AuthorizeRead(ctx, meta, financeauth.PermWalletView); err != nil {
		return nil, err
	}
	if meta.Actor.OrganizationID != "" && meta.Actor.OrganizationID != organizationID {
		return nil, financeauth.ErrForbidden
	}
	return s.providerOps.GetAllocation(ctx, nil, organizationID, id)
}

// FindProviderAllocationByKey is the authoritative read-back used after an
// ambiguous cross-service POST. It resolves the same tenant-scoped idempotency
// key instead of asking the caller to retry a money-moving command blindly.
func (s *Service) FindProviderAllocationByKey(ctx context.Context, meta Meta, organizationID,
	key string) (*models.ProviderAllocation, error) {
	if err := s.AuthorizeRead(ctx, meta, financeauth.PermTransactionView); err != nil {
		return nil, err
	}
	if meta.Actor.OrganizationID != "" && meta.Actor.OrganizationID != organizationID {
		return nil, financeauth.ErrForbidden
	}
	return s.providerOps.FindAllocationByKey(ctx, nil, organizationID, key)
}

// ProviderWithdrawalInput is one payout request against a provider sub-wallet.
type ProviderWithdrawalInput struct {
	Meta                     Meta
	OrganizationID           string
	EarningAccountID         string
	OrganizationMembershipID string
	ProviderID               string
	AmountMinor              int64
	Currency                 string
	DestinationReference     string
	PolicyMode               string
	RequiresApproval         bool
	ExpectedVersion          int64
}

// RequestProviderWithdrawal opens a durable withdrawal and reserves the funds.
//
// Reserving at request time rather than at execution time is what makes two
// concurrent requests against the same sub-wallet safe: the second one finds
// the payable balance already moved into held and is refused, instead of both
// passing a check that was true when each of them read it.
func (s *Service) RequestProviderWithdrawal(ctx context.Context, input ProviderWithdrawalInput) (*models.ProviderWithdrawal, error) {
	if err := input.Meta.validate(); err != nil {
		return nil, err
	}
	if err := s.require(ctx, input.Meta, financeauth.PermProviderWithdrawalRequest); err != nil {
		return nil, err
	}
	currency, err := money.New(0, input.Currency)
	if err != nil {
		return nil, err
	}
	if input.AmountMinor <= 0 {
		return nil, fmt.Errorf("%w: amount must be positive", ErrInvalid)
	}
	if strings.TrimSpace(input.DestinationReference) == "" {
		return nil, fmt.Errorf("%w: a payout destination reference is required", ErrInvalid)
	}
	if input.Meta.Actor.OrganizationID != "" && input.Meta.Actor.OrganizationID != input.OrganizationID {
		return nil, financeauth.ErrForbidden
	}
	policy := strings.ToUpper(defaulted(input.PolicyMode, models.WithdrawalPolicyProviderDirect))
	if policy == models.WithdrawalPolicyDisabled {
		return nil, ErrWithdrawalsDisabled
	}

	fp := fingerprint(input.OrganizationID, input.EarningAccountID, input.AmountMinor,
		currency.Currency, input.DestinationReference, policy)
	if existing, findErr := s.providerOps.FindWithdrawalByKey(ctx, nil, input.OrganizationID,
		input.Meta.IdempotencyKey); findErr == nil {
		if existing.RequestFingerprint != fp {
			return nil, ErrIdempotencyReuse
		}
		return existing, nil
	} else if !errors.Is(findErr, repositories.ErrNotFound) {
		return nil, findErr
	}

	status := models.WithdrawalApproved
	if input.RequiresApproval || policy == models.WithdrawalPolicyCompanyApproval {
		status = models.WithdrawalPendingApproval
	}

	var withdrawal *models.ProviderWithdrawal
	err = database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		account, accountErr := s.accounts.LockEarningAccount(ctx, tx, input.EarningAccountID)
		if accountErr != nil {
			return accountErr
		}
		if account.OrganizationID != input.OrganizationID || account.Currency != currency.Currency {
			return financeauth.ErrForbidden
		}
		if !account.Payable() {
			return fmt.Errorf("%w: provider sub-wallet is %s", ErrAccountNotOperational, account.Status)
		}
		if input.ExpectedVersion != 0 && input.ExpectedVersion != account.Version {
			return repositories.ErrVersionMismatch
		}
		ownership, ownershipErr := s.providerOps.GetSubWalletOwnership(ctx, tx, account.ID)
		if ownershipErr != nil {
			return ownershipErr
		}
		if input.OrganizationMembershipID != "" && ownership.OrganizationMembershipID != "" &&
			ownership.OrganizationMembershipID != input.OrganizationMembershipID {
			return financeauth.ErrForbidden
		}
		if account.PayableMinor < input.AmountMinor {
			return ErrInsufficientFunds
		}
		if moveErr := s.accounts.MoveEarningBalances(ctx, tx, account.ID, account.Version,
			-input.AmountMinor, input.AmountMinor, 0); moveErr != nil {
			return moveErr
		}
		withdrawal = &models.ProviderWithdrawal{
			OrganizationID: input.OrganizationID, EarningAccountID: account.ID,
			OrganizationMembershipID: defaulted(input.OrganizationMembershipID, ownership.OrganizationMembershipID),
			ProviderID:               defaulted(input.ProviderID, ownership.ProviderID),
			AmountMinor:              input.AmountMinor, Currency: currency.Currency,
			DestinationReference: input.DestinationReference, PolicyMode: policy, Status: status,
			RequestedByActorType: input.Meta.Actor.Type, RequestedByActorID: input.Meta.Actor.ID,
			IdempotencyKey: input.Meta.IdempotencyKey, RequestFingerprint: fp,
			CorrelationID: input.Meta.CorrelationID, CausationID: input.Meta.CausationID,
		}
		if createErr := s.providerOps.CreateWithdrawal(ctx, tx, withdrawal); createErr != nil {
			return createErr
		}
		if historyErr := repositories.AppendHistory(ctx, tx, input.OrganizationID, "PROVIDER_WITHDRAWAL",
			withdrawal.ID, "", status, input.Meta.Actor.Type, input.Meta.Actor.ID, "requested",
			map[string]any{"policyMode": policy}, input.Meta.CorrelationID, input.Meta.CausationID); historyErr != nil {
			return historyErr
		}
		if auditErr := s.audit.Record(ctx, tx, audit.Entry{OrganizationID: input.OrganizationID,
			ActorType: input.Meta.Actor.Type, ActorID: input.Meta.Actor.ID,
			Action: "provider_withdrawal.requested", ResourceType: "provider_withdrawal",
			ResourceID: withdrawal.ID, NewValues: map[string]any{"amountMinor": input.AmountMinor,
				"currency": currency.Currency, "policyMode": policy, "status": status},
			RequestID: input.Meta.RequestID, CorrelationID: input.Meta.CorrelationID}); auditErr != nil {
			return auditErr
		}
		return s.events.Publish(ctx, tx, EventWithdrawalRequested, "PROVIDER_WITHDRAWAL",
			withdrawal.ID, withdrawal.Version, input.OrganizationID, input.Meta.CorrelationID,
			input.Meta.CausationID, map[string]any{"withdrawalId": withdrawal.ID,
				"providerSubWalletId": account.ID, "amountMinor": input.AmountMinor,
				"currency": currency.Currency, "policyMode": policy, "status": status})
	})
	if err != nil {
		return nil, err
	}
	return withdrawal, nil
}

// DecideProviderWithdrawal approves or rejects a pending withdrawal.
//
// Rejecting returns the reserved funds; approving leaves them reserved for the
// execution worker. Either way the decision is recorded once: the status guard
// in the repository makes a second decision a version mismatch rather than a
// silent overwrite of the first.
func (s *Service) DecideProviderWithdrawal(ctx context.Context, meta Meta, organizationID, id string,
	expectedVersion int64, approve bool, approvalReference, reasonCode string) (*models.ProviderWithdrawal, error) {
	if err := meta.validate(); err != nil {
		return nil, err
	}
	existing, err := s.providerOps.GetWithdrawal(ctx, nil, organizationID, id)
	if err != nil {
		return nil, err
	}
	if meta.Actor.OrganizationID != "" && meta.Actor.OrganizationID != organizationID {
		return nil, financeauth.ErrForbidden
	}
	if existing.Resolved() {
		return nil, repositories.ErrConflict
	}
	if existing.Version != expectedVersion {
		return nil, repositories.ErrVersionMismatch
	}
	if _, err := s.authorizer.RequireApproval(ctx, meta.Actor, financeauth.PermProviderWithdrawalApprove,
		models.OperationProviderWithdrawalApproval, existing.RequesterIdentity()); err != nil {
		return nil, err
	}

	status := models.WithdrawalApproved
	event := EventWithdrawalApproved
	if !approve {
		status = models.WithdrawalRejected
		event = EventWithdrawalRejected
	}
	var result *models.ProviderWithdrawal
	err = database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		locked, lockErr := s.providerOps.LockWithdrawal(ctx, tx, organizationID, id)
		if lockErr != nil {
			return lockErr
		}
		if locked.Resolved() {
			return repositories.ErrConflict
		}
		if locked.Version != expectedVersion {
			return repositories.ErrVersionMismatch
		}
		if decideErr := s.providerOps.DecideWithdrawal(ctx, tx, id, expectedVersion, status,
			meta.Actor.Type, meta.Actor.ID, approvalReference); decideErr != nil {
			return decideErr
		}
		if !approve {
			account, accountErr := s.accounts.LockEarningAccount(ctx, tx, locked.EarningAccountID)
			if accountErr != nil {
				return accountErr
			}
			if moveErr := s.accounts.MoveEarningBalances(ctx, tx, account.ID, account.Version,
				locked.AmountMinor, -locked.AmountMinor, 0); moveErr != nil {
				return moveErr
			}
		}
		if historyErr := repositories.AppendHistory(ctx, tx, organizationID, "PROVIDER_WITHDRAWAL",
			id, locked.Status, status, meta.Actor.Type, meta.Actor.ID, defaulted(reasonCode, "decided"),
			nil, meta.CorrelationID, meta.CausationID); historyErr != nil {
			return historyErr
		}
		if auditErr := s.audit.Record(ctx, tx, audit.Entry{OrganizationID: organizationID,
			ActorType: meta.Actor.Type, ActorID: meta.Actor.ID,
			Action: "provider_withdrawal.decided", ResourceType: "provider_withdrawal", ResourceID: id,
			PreviousValues: map[string]any{"status": locked.Status},
			NewValues:      map[string]any{"status": status, "reasonCode": reasonCode},
			RequestID:      meta.RequestID, CorrelationID: meta.CorrelationID}); auditErr != nil {
			return auditErr
		}
		if publishErr := s.events.Publish(ctx, tx, event, "PROVIDER_WITHDRAWAL", id,
			locked.Version+1, organizationID, meta.CorrelationID, meta.CausationID,
			map[string]any{"withdrawalId": id, "providerSubWalletId": locked.EarningAccountID,
				"amountMinor": locked.AmountMinor, "currency": locked.Currency,
				"status": status}); publishErr != nil {
			return publishErr
		}
		refreshed, readErr := s.providerOps.GetWithdrawal(ctx, tx, organizationID, id)
		if readErr != nil {
			return readErr
		}
		result = refreshed
		return nil
	})
	return result, err
}

// GetProviderWithdrawal reads one withdrawal inside the tenant's scope.
func (s *Service) GetProviderWithdrawal(ctx context.Context, meta Meta, organizationID,
	id string) (*models.ProviderWithdrawal, error) {
	if err := s.AuthorizeRead(ctx, meta, financeauth.PermProviderWithdrawalView); err != nil {
		return nil, err
	}
	if meta.Actor.OrganizationID != "" && meta.Actor.OrganizationID != organizationID {
		return nil, financeauth.ErrForbidden
	}
	return s.providerOps.GetWithdrawal(ctx, nil, organizationID, id)
}

// ReplayProviderWithdrawal returns a dead-lettered withdrawal to execution.
func (s *Service) ReplayProviderWithdrawal(ctx context.Context, meta Meta, organizationID, id string) error {
	if err := meta.validate(); err != nil {
		return err
	}
	if err := s.require(ctx, meta, financeauth.PermProviderWithdrawalApprove); err != nil {
		return err
	}
	return database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.providerOps.ReplayWithdrawal(ctx, tx, organizationID, id); err != nil {
			return err
		}
		return s.audit.Record(ctx, tx, audit.Entry{OrganizationID: organizationID,
			ActorType: meta.Actor.Type, ActorID: meta.Actor.ID,
			Action: "provider_withdrawal.replayed", ResourceType: "provider_withdrawal",
			ResourceID: id, RequestID: meta.RequestID, CorrelationID: meta.CorrelationID})
	})
}

// ProviderStatementInput narrows a provider statement.
type ProviderStatementInput struct {
	Meta             Meta
	OrganizationID   string
	EarningAccountID string
	From, To         time.Time
	Limit            int
}

// GenerateProviderStatement builds the provider-facing view of one sub-wallet.
//
// Every field of every entry is assigned explicitly from an allocation or a
// withdrawal. Nothing is copied wholesale and then stripped: a field added to
// the allocation record later would have to be deliberately mapped here before
// a provider could ever see it, which is the point.
func (s *Service) GenerateProviderStatement(ctx context.Context, input ProviderStatementInput) (*models.ProviderStatement, error) {
	if err := s.AuthorizeRead(ctx, input.Meta, financeauth.PermWalletView); err != nil {
		return nil, err
	}
	if input.Meta.Actor.OrganizationID != "" && input.Meta.Actor.OrganizationID != input.OrganizationID {
		return nil, financeauth.ErrForbidden
	}
	account, err := s.accounts.GetEarningAccountByID(ctx, nil, input.EarningAccountID)
	if err != nil {
		return nil, err
	}
	if account.OrganizationID != input.OrganizationID {
		// Indistinguishable from "does not exist": a cross-tenant probe must not
		// be able to tell the difference.
		return nil, repositories.ErrNotFound
	}
	allocations, err := s.providerOps.ListAllocations(ctx, nil, input.OrganizationID,
		repositories.AllocationFilter{EarningAccountID: account.ID, From: input.From,
			To: input.To, Limit: input.Limit})
	if err != nil {
		return nil, err
	}
	withdrawals, err := s.providerOps.ListWithdrawals(ctx, nil, input.OrganizationID,
		repositories.WithdrawalFilter{EarningAccountID: account.ID, From: input.From,
			To: input.To, Limit: input.Limit})
	if err != nil {
		return nil, err
	}
	statement := &models.ProviderStatement{
		EarningAccountID: account.ID, Currency: account.Currency, Status: account.Status,
		AvailableMinor: account.PayableMinor, HeldMinor: account.HeldMinor,
		SettledMinor: account.SettledMinor, SourceVersion: account.Version,
		GeneratedAt: s.now(), Entries: make([]models.ProviderStatementEntry, 0,
			len(allocations)+len(withdrawals)),
	}
	for _, allocation := range allocations {
		direction := models.DirectionCredit
		if models.DebitsProvider(allocation.AllocationType) {
			direction = models.DirectionDebit
		}
		statement.Entries = append(statement.Entries, models.ProviderStatementEntry{
			ID: allocation.ID, EntryType: models.StatementEntryAllocation,
			AllocationType: allocation.AllocationType, Direction: direction,
			AmountMinor: allocation.AmountMinor, Currency: allocation.Currency,
			BusinessReferenceType: allocation.BusinessReferenceType,
			BusinessReferenceID:   allocation.BusinessReferenceID,
			ReasonCode:            allocation.ReasonCode, Explanation: allocation.ProviderExplanation,
			Status: allocation.Status, OccurredAt: allocation.CreatedAt,
			SourceVersion: allocation.Version,
		})
	}
	for _, withdrawal := range withdrawals {
		statement.Entries = append(statement.Entries, models.ProviderStatementEntry{
			ID: withdrawal.ID, EntryType: models.StatementEntryWithdrawal,
			Direction: models.DirectionDebit, AmountMinor: withdrawal.AmountMinor,
			Currency: withdrawal.Currency, Status: withdrawal.Status,
			OccurredAt: withdrawal.CreatedAt, SourceVersion: withdrawal.Version,
		})
	}
	return statement, nil
}

// ProviderOperations exposes the repository for composition and tests.
func (s *Service) ProviderOperations() *repositories.ProviderOperationsRepository {
	return s.providerOps
}
