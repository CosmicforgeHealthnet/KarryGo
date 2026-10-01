package engine

import (
	"errors"
	"fmt"

	"cosmicforge/logistics/services/pricing-service/internal/platform/money"
)

// PolicySet is the resolved set of bodies a single price is computed from. Every
// quote records which versions these were, so the same set can be reloaded and
// re-evaluated years later.
type PolicySet struct {
	Tariff     Tariff
	Commission CommissionV1
	Regulatory RegulatoryChargesV1
}

// ErrConservation is returned when funding and allocation do not balance. It
// should be impossible by construction; it is checked anyway because a price that
// does not balance would hand the wallet an instruction that cannot be executed
// without inventing or losing money.
var ErrConservation = errors.New("engine: funding and allocation do not balance")

// Evaluate produces the full economic decision for req.
//
// The order is deliberate: the tariff decides the economic amount, then funding
// decides who pays it, then commission divides what is received. Discounts never
// reach the tariff, which is what stops a promotion quietly reducing the
// provider's share -- a rule the promotions milestone will lean on.
func Evaluate(req Request, policies PolicySet) (Result, error) {
	if err := req.Validate(); err != nil {
		return Result{}, err
	}
	if policies.Tariff == nil {
		return Result{}, errors.New("engine: no tariff policy resolved")
	}

	expectedKind, err := TariffKindFor(req.Scope)
	if err != nil {
		return Result{}, err
	}
	if policies.Tariff.Kind() != expectedKind {
		return Result{}, fmt.Errorf("engine: scope %s needs %s but got %s", req.Scope, expectedKind, policies.Tariff.Kind())
	}

	components, gross, flags, err := policies.Tariff.Evaluate(req)
	if err != nil {
		return Result{}, err
	}

	// Components must sum to gross exactly. The builder maintains this, so a
	// mismatch means an evaluator returned an inconsistent set and the price must
	// not be used.
	sum, err := sumComponents(components)
	if err != nil {
		return Result{}, err
	}
	if sum != gross {
		return Result{}, fmt.Errorf("%w: components sum to %d but gross is %d", ErrConservation, sum, gross)
	}
	if gross < 0 {
		return Result{}, fmt.Errorf("engine: gross cannot be negative (%d)", gross)
	}

	// V1 funding: the customer pays the whole economic amount. Merchant funding
	// and KarryGo subsidy arrive with the promotions milestone, and will change
	// only this block -- never the tariff above it.
	funding := Funding{CustomerKobo: gross}

	base := gross
	if policies.Commission.AppliedTo == CommissionOnCustomerPayable {
		base = funding.CustomerKobo
	}
	allocation, bps, err := policies.Commission.Split(base, req.Scope, req.Channel)
	if err != nil {
		return Result{}, err
	}

	result := Result{
		Currency:   currencyOf(policies),
		Components: components,
		GrossKobo:  gross,
		Funding:    funding,
		Allocation: allocation,
		Flags:      flags,
	}
	result.Explanation = explain(req, result, bps)

	if err := result.checkConservation(); err != nil {
		return Result{}, err
	}
	return result, nil
}

// checkConservation asserts every kobo is accounted for on both sides.
func (r Result) checkConservation() error {
	if r.Funding.Total() != r.GrossKobo {
		return fmt.Errorf("%w: funding totals %d but gross is %d", ErrConservation, r.Funding.Total(), r.GrossKobo)
	}
	if r.Allocation.Total() != r.GrossKobo {
		return fmt.Errorf("%w: allocation totals %d but gross is %d", ErrConservation, r.Allocation.Total(), r.GrossKobo)
	}
	if r.Funding.CustomerKobo < 0 || r.Funding.MerchantKobo < 0 || r.Funding.PlatformSubsidyKobo < 0 {
		return fmt.Errorf("%w: a funding source is negative", ErrConservation)
	}
	if r.Allocation.ProviderKobo < 0 || r.Allocation.PlatformFeeKobo < 0 || r.Allocation.TaxKobo < 0 {
		return fmt.Errorf("%w: an allocation is negative", ErrConservation)
	}
	return nil
}

func sumComponents(components []Component) (int64, error) {
	amounts := make([]int64, 0, len(components))
	for _, component := range components {
		amounts = append(amounts, component.AmountKobo)
	}
	return money.Sum(amounts...)
}

func currencyOf(policies PolicySet) string {
	switch tariff := policies.Tariff.(type) {
	case TaxiTariffV1:
		return tariff.Currency
	case DispatchTariffV1:
		return tariff.Currency
	case HaulageTariffV1:
		return tariff.Currency
	default:
		return "NGN"
	}
}
