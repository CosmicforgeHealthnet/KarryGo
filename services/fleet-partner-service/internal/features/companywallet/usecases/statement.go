package usecases

import (
	"context"
	"errors"
	"sort"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/companywallet/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/companywallet/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// ProviderStatement is the provider-facing view of one sub-wallet.
//
// This type and ProviderStatementEntry are the entire data-minimization
// boundary for Section 13 reads. Every field is assigned explicitly from an
// allocation or a withdrawal record. No internal struct is ever serialized and
// then pruned: with a prune-based approach, a field added upstream next year
// would start reaching providers the day it was added, and nobody would notice
// until it had.
//
// Specifically absent, and absent by construction rather than by filtering:
// customer fare, customer price, customer payment or payment method,
// organization settlement amount, organization wallet balance, company margin,
// platform commission or its percentage, contract price, quotation price,
// organization profit, internal finance notes, internal approval discussion,
// other providers' transactions, and any bank detail beyond the fact that a
// withdrawal happened.
type ProviderStatement struct {
	ProviderSubWalletID string `json:"providerSubWalletId"`
	MembershipID        string `json:"membershipId"`
	Currency            string `json:"currency,omitempty"`
	Status              string `json:"status"`
	// Balance is what payment-wallet-service reported. It is a pointer so that
	// "we could not ask" is representable and is not rendered as zero.
	AvailableBalanceMinor *int64                          `json:"availableBalanceMinor,omitempty"`
	SourceVersion         int64                           `json:"sourceVersion"`
	FreshnessState        string                          `json:"freshnessState"`
	SyncedAt              *time.Time                      `json:"syncedAt,omitempty"`
	GeneratedAt           time.Time                       `json:"generatedAt"`
	Entries               []models.ProviderStatementEntry `json:"entries"`
}

// StatementInput narrows a provider statement.
type StatementInput struct {
	Tenant       tenant.Context
	MembershipID string
	Limit        int
	// ProviderScoped marks a provider-authenticated read. It changes nothing
	// about which fields are returned — the DTO is already minimal — but it is
	// recorded so a corporate read and a provider read are distinguishable in
	// the audit trail.
	ProviderScoped bool
}

// ProviderStatement builds the provider-facing statement.
func (s *Service) ProviderStatement(ctx context.Context, in StatementInput) (ProviderStatement, error) {
	var zero ProviderStatement
	if !s.opts.Flags.SubWalletEnabled {
		return zero, notFound("Provider sub-wallet")
	}
	link, err := s.opts.Repository.GetSubWalletLink(ctx, nil, in.Tenant.OrganizationID,
		in.MembershipID)
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			return zero, notFound("Provider sub-wallet")
		}
		return zero, err
	}

	limit := in.Limit
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	// Both listings are scoped to this one membership. A provider reading their
	// statement must not be able to widen it to a colleague's by any parameter.
	allocations, err := s.opts.Repository.ListAllocations(ctx, in.Tenant.OrganizationID, nil,
		repositories.AllocationFilter{MembershipID: in.MembershipID, Limit: limit})
	if err != nil {
		return zero, err
	}
	withdrawals, err := s.opts.Repository.ListWithdrawals(ctx, in.Tenant.OrganizationID, nil,
		repositories.WithdrawalFilter{MembershipID: in.MembershipID, Limit: limit})
	if err != nil {
		return zero, err
	}

	statement := ProviderStatement{
		ProviderSubWalletID: link.WalletAccountID, MembershipID: link.ProviderMembershipID,
		Currency: link.Currency, Status: link.Status, SourceVersion: link.SourceVersion,
		FreshnessState: link.FreshnessState, SyncedAt: link.SyncedAt, GeneratedAt: s.now(),
		Entries: make([]models.ProviderStatementEntry, 0, len(allocations)+len(withdrawals)),
	}

	for _, allocation := range allocations {
		// A draft or rejected allocation is not a fact about the provider's
		// money and is not shown; showing one would promise a payment that the
		// company has not made.
		if allocation.Status == models.AllocationDraft ||
			allocation.Status == models.AllocationRejected {
			continue
		}
		direction := models.DirectionCredit
		if models.DebitsProvider(allocation.AllocationType) {
			direction = models.DirectionDebit
		}
		statement.Entries = append(statement.Entries, models.ProviderStatementEntry{
			ID:             allocation.ID,
			EntryType:      models.EntryAllocation,
			AllocationType: allocation.AllocationType,
			Direction:      direction,
			AmountMinor:    allocation.AmountMinor,
			Currency:       allocation.Currency,
			// The business reference type and ID are safe: they name the work,
			// not what the customer paid for it.
			BusinessReferenceType: allocation.BusinessReferenceType,
			BusinessReferenceID:   allocation.BusinessReferenceID,
			ReasonCode:            allocation.ReasonCode,
			// The provider-visible explanation is copied; the private note is
			// not referenced anywhere in this function.
			Explanation:   allocation.ProviderExplanation,
			Status:        allocation.Status,
			OccurredAt:    allocation.CreatedAt,
			SourceVersion: allocation.Version,
		})
	}

	for _, withdrawal := range withdrawals {
		statement.Entries = append(statement.Entries, models.ProviderStatementEntry{
			ID:            withdrawal.ID,
			EntryType:     models.EntryWithdrawal,
			Direction:     models.DirectionDebit,
			AmountMinor:   withdrawal.AmountMinor,
			Currency:      withdrawal.Currency,
			Status:        withdrawal.Status,
			OccurredAt:    withdrawal.CreatedAt,
			SourceVersion: withdrawal.Version,
		})
	}

	sort.SliceStable(statement.Entries, func(i, j int) bool {
		return statement.Entries[i].OccurredAt.After(statement.Entries[j].OccurredAt)
	})
	if len(statement.Entries) > limit {
		statement.Entries = statement.Entries[:limit]
	}

	// The statement is refused rather than served stale when the sub-wallet
	// link has never been confirmed: a provider looking at their own money
	// deserves an honest "unavailable" over a plausible but unbacked list.
	if link.WalletAccountID == "" {
		return zero, errorcodes.Unavailable(errorcodes.ProviderWalletStatementUnavailable,
			"The provider statement is not available yet.")
	}
	return statement, nil
}
