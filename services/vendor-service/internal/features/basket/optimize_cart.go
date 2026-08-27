package basket

// Optimize My Cart, and accepting the result.
//
// The rule that governs this whole file is one sentence: optimization returns
// a PROPOSAL, and it never silently mutates the customer's cart.
//
// That is not a nicety. A shopper who opens a "could this be cheaper?" panel
// has not asked for their basket to be rearranged, and an optimizer that
// helpfully applies its own suggestion has substituted its judgement for
// theirs on something they are about to pay for. So the flow is strictly:
//
//	evaluate  ->  propose  ->  customer accepts  ->  NEW cart version  ->  requote
//
// Each arrow is a separate, refusable step. The proposal expires. Acceptance
// checks that the cart has not moved underneath it. Acceptance twice produces
// one cart version, not two. And the proposal's own totals are never the
// money - the authoritative number is the CheckoutQuote produced afterwards
// from the new cart version, because prices and stock may have moved between
// the offer and the acceptance.

import (
	"context"
	"strings"
	"time"
)

// OptimizeCart proposes a cheaper equivalent of the customer's current cart.
//
// Requirements are DERIVED from the cart rather than stated by the customer:
// each line becomes "this product, this quantity, this unit". The customer's
// substitution allowance decides how far the optimizer may stray, and the
// caller passes it explicitly rather than this package assuming one.
func (s *Service) OptimizeCart(
	ctx context.Context,
	customerID string,
	allowance SubstitutionAllowance,
	preference string,
	objectives []Objective,
) (Result, error) {
	if s.carts == nil {
		return Result{}, ErrDependencyMissing
	}

	customerID = strings.TrimSpace(customerID)

	if customerID == "" {
		return Result{}, ErrProposalNotFound
	}

	if !allowance.Valid() {
		allowance = AllowExactOnly
	}

	cart, err := s.carts.LoadForOptimization(ctx, customerID)
	if err != nil {
		return Result{}, err
	}

	if len(cart.Lines) == 0 {
		return Result{}, ErrNoRequirements
	}

	if !cart.Destination.Resolved() {
		return Result{}, ErrNoDestination
	}

	request := Request{
		CustomerID:           customerID,
		Currency:             cart.Currency,
		Destination:          cart.Destination,
		FulfilmentPreference: preference,
		Objectives:           objectives,
	}

	originals := make(map[string]Listing, len(cart.Lines))

	for _, line := range cart.Lines {
		ref := line.LineID

		request.Requirements = append(request.Requirements, Requirement{
			Ref:         ref,
			ProductRef:  line.ProductRef,
			VariantRef:  line.VariantRef,
			ListingRef:  line.ListingRef,
			SellUnitRef: line.SellUnitRef,
			Quantity:    line.Quantity,

			// The customer's stated allowance, applied uniformly. Deriving a
			// per-line allowance from the cart would be inventing consent
			// line by line.
			Substitution: allowance,
		})

		// The line the requirement came from. It is what makes a
		// manufacturer trade-identifier match possible, and what lets a
		// substitution be described as a change rather than as a choice.
		originals[ref] = Listing{
			ListingRef:     line.ListingRef,
			ProductRef:     line.ProductRef,
			VariantRef:     line.VariantRef,
			SellUnitRef:    line.SellUnitRef,
			BusinessID:     line.BusinessID,
			BranchID:       line.BranchID,
			DisplayName:    line.DisplayName,
			UnitPriceMinor: line.UnitPriceMinor,
			Currency:       line.Currency,
		}
	}

	result, err := s.optimize(ctx, request, originals)
	if err != nil {
		return Result{}, err
	}

	// Persist the variants as acceptable offers, bound to the cart version
	// they were computed against.
	//
	// A variant identical to the current cart is not offered. There is
	// nothing to accept, and presenting "keep what you have" as a proposal
	// would invite a customer to click Accept and be told nothing changed.
	current := currentCartFingerprint(cart)

	kept := result.Variants[:0]

	for _, variant := range result.Variants {
		if variant.Fingerprint == current {
			continue
		}

		// Without a store there is nothing to accept, so the variant is
		// reported as information and carries no proposal id. The
		// identical-to-current filter still applies: "keep what you have" is
		// not a suggestion however it is being returned.
		if s.store == nil {
			kept = append(kept, variant)

			continue
		}

		proposal, saveErr := s.store.CreateProposal(ctx, Proposal{
			CustomerID:               customerID,
			BaseCartID:               cart.CartID,
			BaseCartVersion:          cart.Version,
			Objective:                variant.Objective,
			State:                    ProposalOpen,
			Currency:                 variant.Currency,
			MerchandiseTotalMinor:    variant.MerchandiseTotalMinor,
			DeliveryTotalMinor:       variant.DeliveryTotalMinor,
			FinalDeliveredTotalMinor: variant.FinalDeliveredTotalMinor,
			VendorCount:              variant.VendorCount,
			Lines:                    variant.Lines,
			Substitutions:            variant.Substitutions,
			Fingerprint:              variant.Fingerprint,
			ExpiresAt:                s.now().UTC().Add(s.proposalTTL),
		})
		if saveErr != nil {
			return Result{}, saveErr
		}

		variant.ProposalID = proposal.ID
		kept = append(kept, variant)
	}

	result.Variants = kept

	return result, nil
}

// currentCartFingerprint is the fingerprint of the cart exactly as it stands.
//
// It is computed with the same function proposals use, so "this proposal is
// the cart you already have" is a string comparison rather than a judgement.
func currentCartFingerprint(cart CartSnapshot) string {
	lines := make([]ProposedLine, 0, len(cart.Lines))

	for _, line := range cart.Lines {
		lines = append(lines, ProposedLine{
			RequirementRef: line.LineID,
			ListingRef:     line.ListingRef,
			SellUnitRef:    line.SellUnitRef,
			Quantity:       line.Quantity,
			UnitPriceMinor: line.UnitPriceMinor,
		})
	}

	return fingerprintOf(lines)
}

// GetProposal reads one proposal the customer owns.
func (s *Service) GetProposal(
	ctx context.Context,
	customerID, proposalID string,
) (Proposal, error) {
	if s.store == nil {
		return Proposal{}, ErrDependencyMissing
	}

	proposal, err := s.store.GetProposal(ctx, customerID, proposalID)
	if err != nil {
		return Proposal{}, err
	}

	// A lapsed proposal is reported as expired rather than as open. Its
	// prices and stock were true when it was made; presenting it as current
	// would present numbers nobody stands behind.
	if proposal.State == ProposalOpen && proposal.Expired(s.now().UTC()) {
		proposal.State = ProposalExpired
	}

	return proposal, nil
}

// AcceptProposal applies a proposal, creating a NEW cart version.
//
// Four things are checked before anything changes, and each protects against
// a different way of silently doing the wrong thing:
//
//  1. OWNERSHIP. A proposal id alone must never be enough to rewrite somebody
//     else's shopping. Enforced by a customer-scoped read.
//
//  2. ALREADY ACCEPTED. Returns the SAME cart version rather than making a
//     second one, so a double-tapped Accept button is harmless.
//
//  3. EXPIRY. A proposal past its life quoted prices and stock that are no
//     longer evidence.
//
//  4. CART VERSION. The proposal was built against one cart. If the customer
//     has changed it since, applying the proposal would discard whatever they
//     did, so the acceptance is refused and a fresh evaluation is required.
func (s *Service) AcceptProposal(
	ctx context.Context,
	customerID, proposalID string,
) (AcceptanceResult, error) {
	if s.store == nil || s.carts == nil {
		return AcceptanceResult{}, ErrDependencyMissing
	}

	proposal, err := s.store.GetProposal(ctx, customerID, proposalID)
	if err != nil {
		return AcceptanceResult{}, err
	}

	// Idempotent replay. The cart version this proposal already produced is
	// returned, and no second version is created.
	if proposal.State == ProposalAccepted {
		return AcceptanceResult{
			ProposalID:      proposal.ID,
			CartID:          proposal.BaseCartID,
			CartVersion:     proposal.AcceptedCartVersion,
			Replayed:        true,
			RequoteRequired: true,
		}, nil
	}

	now := s.now().UTC()

	if proposal.Expired(now) {
		_ = s.store.MarkProposalState(ctx, customerID, proposal.ID, ProposalExpired)

		return AcceptanceResult{}, ErrProposalExpired
	}

	if proposal.State != ProposalOpen {
		return AcceptanceResult{}, ErrProposalStale
	}

	cart, err := s.carts.LoadForOptimization(ctx, customerID)
	if err != nil {
		return AcceptanceResult{}, err
	}

	if cart.CartID != proposal.BaseCartID ||
		cart.Version != proposal.BaseCartVersion {
		// The cart moved. Applying the proposal now would silently discard
		// whatever the customer did in between, which is precisely the
		// failure this whole design exists to prevent.
		_ = s.store.MarkProposalState(
			ctx, customerID, proposal.ID, ProposalSuperseded,
		)

		return AcceptanceResult{}, ErrProposalStale
	}

	updated, err := s.carts.ApplyProposal(
		ctx, customerID, proposal.BaseCartVersion, proposal.Lines,
	)
	if err != nil {
		return AcceptanceResult{}, err
	}

	// The acceptance is recorded with the cart version it produced. The
	// unique constraint behind this write is what makes two concurrent
	// acceptances resolve to one, rather than relying on the check above
	// having won a race.
	if err = s.store.MarkProposalAccepted(
		ctx, customerID, proposal.ID, updated.Version, now,
	); err != nil {
		return AcceptanceResult{}, err
	}

	return AcceptanceResult{
		ProposalID:  proposal.ID,
		CartID:      updated.CartID,
		CartVersion: updated.Version,

		// Always. The proposal's totals were an estimate against prices and
		// stock at the moment it was built; the authoritative money is the
		// CheckoutQuote produced from this new cart version.
		RequoteRequired: true,
	}, nil
}

// SweepExpiredProposals marks lapsed offers expired.
//
// The durable counterpart to the read-time check: a proposal nobody reads
// must still stop being acceptable, or a customer returning to a stale tab
// tomorrow would find yesterday's prices still clickable.
func (s *Service) SweepExpiredProposals(
	ctx context.Context,
	limit int,
) (int64, error) {
	if s.store == nil {
		return 0, nil
	}

	return s.store.ExpireDueProposals(ctx, s.now().UTC(), limit)
}

// Now exposes the service clock for callers that need to agree with it.
func (s *Service) Now() time.Time { return s.now().UTC() }
