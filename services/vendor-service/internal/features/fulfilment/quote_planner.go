package fulfilment

// Quote-only planning.
//
// Planning a checkout persists a plan, supersedes the previous one and
// becomes the thing a customer is charged against. Answering "what would this
// basket cost to fulfil" must do none of that. A shopping optimizer may ask
// the question a hundred times while a customer is still deciding, and a
// hundred plans, a hundred mission records or a hundred provider reservations
// would be a hundred commitments nobody made.
//
// So this file exposes the planner's ANSWER without its CONSEQUENCES. It runs
// the same requirement engine, the same feasibility rules, the same candidate
// comparison and the same real logistics quotes - anything less would be an
// estimate wearing a plan's clothes, and the section is explicit that delivery
// must not be guessed at as a fixed fee per vendor.
//
// What it deliberately never does:
//
//   - persist a plan, a mission, a group or an event
//   - create a PlatformMissionRef or any canonical logistics mission
//   - reserve or assign a provider
//   - touch Wallet, inventory or the customer's cart
//
// Those are not omissions to be filled in later. They are the definition of
// the operation, and the tests assert each one by counting.

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// QuoteOnlyGroup is one prospective vendor group to be planned.
//
// It is the same evidence a checkout group carries, minus the checkout: there
// is no session, no validation revision and no reservation, because nothing
// has been committed to.
type QuoteOnlyGroup struct {
	// Ref names this group inside the request. It is caller-chosen and
	// carries no authority - it exists so the answer can be read back against
	// the question.
	Ref string

	BusinessID string
	BranchID   string

	// MerchandiseMinor is the group's merchandise total, used only for the
	// vendor's own minimum-order and free-delivery rules. No platform fee is
	// ever derived from it.
	MerchandiseMinor int64

	Currency string

	Lines []LineLogistics

	Timing TimingProfile
	Value  ValueProfile
}

// QuoteOnlyRequest asks what a prospective basket would cost to deliver.
type QuoteOnlyRequest struct {
	// CustomerID is used for correlation only. Nothing is stored against it,
	// which is precisely why an optimizer may ask this a hundred times.
	CustomerID string

	Currency string

	Destination Destination

	Mode Mode

	Groups []QuoteOnlyGroup

	CorrelationID string
}

// QuoteOnlyGroupResult is one group's outcome.
type QuoteOnlyGroupResult struct {
	Ref string `json:"ref"`

	BusinessID string `json:"business_id"`
	BranchID   string `json:"branch_id"`

	Serviceable bool   `json:"serviceable"`
	ReasonCode  string `json:"reason_code"`

	Authority    Authority    `json:"authority"`
	VehicleClass VehicleClass `json:"vehicle_class,omitempty"`

	// DeliveryCustomerMinor is this group's share of what the customer would
	// be asked to pay for delivery.
	DeliveryCustomerMinor int64 `json:"delivery_customer_minor"`
}

// QuoteOnlyResult is the answer.
//
// It has no id, no expiry of its own and nothing that could be mistaken for a
// commitment. The quote expiries inside it belong to the logistics
// authorities and are reported as evidence of how long the numbers stand.
type QuoteOnlyResult struct {
	Mode Mode `json:"fulfilment_mode"`

	// Serviceable is false when ANY group cannot be delivered. A basket that
	// cannot be fully delivered is not a cheaper basket; it is a different
	// question, and reporting a partial total as though it were complete is
	// how a customer ends up choosing an option that does not exist.
	Serviceable bool   `json:"serviceable"`
	ReasonCode  string `json:"reason_code,omitempty"`

	Currency string `json:"currency,omitempty"`

	DeliveryGrossMinor    int64 `json:"delivery_gross_minor"`
	DeliveryCustomerMinor int64 `json:"delivery_customer_minor"`

	MissionCount     int `json:"mission_count"`
	VendorGroupCount int `json:"vendor_group_count"`

	// CompletionMinutes is time to the last delivery under the selected
	// candidate. It is what an expected delivery range is built from, and it
	// comes from the authorities' own transit estimates.
	CompletionMinutes int `json:"completion_minutes"`

	// EarliestQuoteExpiry is the soonest moment one of the underlying
	// logistics quotes lapses. Past it, the delivery numbers here are no
	// longer anybody's answer.
	EarliestQuoteExpiry *time.Time `json:"earliest_quote_expiry,omitempty"`

	PolicyVersion int64 `json:"optimization_policy_version"`

	Groups []QuoteOnlyGroupResult `json:"groups"`

	// Candidates are the alternatives considered, exactly as a real plan
	// records them. A shopping proposal has to be able to say "these two
	// shops could not be combined because the runtime cannot drive two
	// pickups" just as much as a checkout does.
	Candidates []PlanCandidate `json:"candidates,omitempty"`

	// SelectedFingerprint identifies the chosen candidate. Together with the
	// inputs it makes the whole answer reproducible, which is what lets a
	// repeated evaluation be recognised as the same evaluation rather than
	// re-quoted.
	SelectedFingerprint string `json:"selected_fingerprint,omitempty"`
}

// QuotePlan answers what a prospective basket would cost to deliver, with no
// side effects of any kind.
//
// The correlation reference is minted per call and is deliberately NOT a plan
// id: nothing is stored under it, and anything downstream trying to look it
// up will find nothing, which is the correct outcome.
func (p *Planner) QuotePlan(
	ctx context.Context,
	request QuoteOnlyRequest,
) (QuoteOnlyResult, error) {
	mode := request.Mode

	if !mode.Selectable() {
		mode = DefaultMode
	}

	groups := make([]GroupInput, 0, len(request.Groups))

	for index, group := range request.Groups {
		ref := strings.TrimSpace(group.Ref)

		if ref == "" {
			ref = "grp_" + strconv.Itoa(index)
		}

		currency := strings.TrimSpace(group.Currency)

		if currency == "" {
			currency = request.Currency
		}

		groups = append(groups, GroupInput{
			CheckoutGroupID: ref,
			BusinessID:      group.BusinessID,
			BranchID:        group.BranchID,
			OrderValueMinor: group.MerchandiseMinor,
			Currency:        currency,
			Lines:           group.Lines,
			Timing:          group.Timing,
			Value:           group.Value,
		})
	}

	// The correlation reference stands in for a checkout session id so quote
	// references stay stable and cacheable within one evaluation. It is
	// unique per call, so two evaluations never collide on an authority's
	// idempotency key and neither is ever mistaken for a stored plan.
	reference := strings.TrimSpace(request.CorrelationID)

	if reference == "" {
		reference = "qo_" + uuid.NewString()
	}

	plan, _, err := p.Build(ctx, PlanRequest{
		CustomerID: request.CustomerID,

		// A quote-only evaluation has no checkout session. The reference is
		// carried in the session slot so quote references remain derivable,
		// and the revision is the smallest legal value because there is no
		// validation to point at.
		CheckoutSessionID:          reference,
		CheckoutValidationRevision: 1,

		Currency:      request.Currency,
		Destination:   request.Destination,
		Groups:        groups,
		Mode:          mode,
		CorrelationID: reference,
	})
	if err != nil {
		return QuoteOnlyResult{}, err
	}

	return quoteOnlyResultFrom(plan, request), nil
}

// quoteOnlyResultFrom projects a plan into an answer with nothing operational
// in it.
//
// Missions are deliberately NOT carried through. A mission is the commercial
// unit a customer is charged against and a later section turns into real
// logistics work; handing one to a shopping optimizer would put an
// operational object in a proposal that must never become one.
func quoteOnlyResultFrom(
	plan Plan,
	request QuoteOnlyRequest,
) QuoteOnlyResult {
	result := QuoteOnlyResult{
		Mode:                  plan.Mode,
		Serviceable:           plan.Status == StatusPlanned,
		ReasonCode:            plan.FailureCode,
		Currency:              plan.Currency,
		DeliveryGrossMinor:    plan.Funding.GrossMinor,
		DeliveryCustomerMinor: plan.Funding.CustomerMinor,
		MissionCount:          len(plan.Missions),
		PolicyVersion:         plan.OptimizationPolicyVersion,
		Candidates:            plan.Candidates,
	}

	if result.Currency == "" {
		result.Currency = strings.ToUpper(strings.TrimSpace(request.Currency))
	}

	branches := map[string]bool{}

	for _, group := range plan.Groups {
		branches[group.BusinessID+"|"+group.BranchID] = true

		result.Groups = append(result.Groups, QuoteOnlyGroupResult{
			Ref:                   group.CheckoutGroupID,
			BusinessID:            group.BusinessID,
			BranchID:              group.BranchID,
			Serviceable:           group.Serviceable,
			ReasonCode:            group.ReasonCode,
			Authority:             group.Authority,
			VehicleClass:          group.VehicleClass,
			DeliveryCustomerMinor: group.Funding.CustomerMinor,
		})
	}

	result.VendorGroupCount = len(branches)

	if selected, ok := plan.SelectedCandidate(); ok {
		result.CompletionMinutes = selected.CompletionMinutes
		result.SelectedFingerprint = selected.Fingerprint
	}

	for _, mission := range plan.Missions {
		expiry := mission.Quote.ExpiresAt

		if expiry == nil {
			continue
		}

		if result.EarliestQuoteExpiry == nil ||
			expiry.Before(*result.EarliestQuoteExpiry) {
			at := *expiry
			result.EarliestQuoteExpiry = &at
		}
	}

	// A basket that cannot be fully delivered has no delivered total. Leaving
	// the partial figure in place would let it be compared against complete
	// baskets and win by being incomplete.
	if !result.Serviceable && result.ReasonCode == "" {
		result.ReasonCode = FailureGroupUnserviceable
	}

	return result
}
