package fleetpartner

// CMP-021 / CMP-033: what the organization's share means, and what happens when
// one provider works across several services.
//
// The organization's share is a residual, not a rate: it is whatever a job's
// eligible money is not owed to the provider. Stating it that way is what makes
// economic conservation checkable -- provider plus organization is the job, and
// there is nowhere else for a unit to go or come from.
//
// The failure this guards against is subtle. A component can be counted once as
// provider compensation, once as an organization share, and once again as a
// payroll accrual, and every individual figure will look defensible. Only the
// conservation check catches it.

import (
	"fmt"
	"sort"
	"strings"
)

// EconomicConservation is the check that one job's money adds up.
type EconomicConservation struct {
	EligibleBasisMinor     int64 `json:"eligibleBasisMinor"`
	ProviderShareMinor     int64 `json:"providerShareMinor"`
	OrganizationShareMinor int64 `json:"organizationShareMinor"`

	// PayrollAccruedMinor is what this job's plan deferred to payroll. It is
	// deliberately NOT part of the job's arithmetic: it is paid out of the
	// organization's own money later, not out of this job. Reporting it here
	// is what lets a reviewer see that it was excluded on purpose rather than
	// forgotten.
	PayrollAccruedMinor int64 `json:"payrollAccruedMinor"`
}

// Balanced reports whether the job's money is fully and exactly accounted for.
func (c EconomicConservation) Balanced() bool {
	return c.ProviderShareMinor >= 0 &&
		c.OrganizationShareMinor >= 0 &&
		c.ProviderShareMinor+c.OrganizationShareMinor == c.EligibleBasisMinor
}

// CheckEconomicConservation proves a resolution neither created nor lost money.
//
// It also proves the two levels do not overlap: a component settled with the
// job must not also be accrued to payroll, which is the double payment CMP-027
// exists to prevent and this is the arithmetic half of the same guarantee.
func CheckEconomicConservation(resolution WorkResolution, eligibleBasisMinor int64) (EconomicConservation, error) {
	conservation := EconomicConservation{
		EligibleBasisMinor:     eligibleBasisMinor,
		ProviderShareMinor:     resolution.ProviderShareMinor,
		OrganizationShareMinor: resolution.OrganizationShareMinor,
	}

	var workLevelTotal int64
	seen := map[string]string{}
	for _, matched := range resolution.Matched {
		// One rule cannot be settled in two places for one job. If it were,
		// the same entitlement would be paid with the job AND accrued to
		// payroll, and both figures would look correct in isolation.
		if previous, repeated := seen[matched.RuleID]; repeated && previous != matched.Settlement {
			return EconomicConservation{}, fmt.Errorf(
				"%w: rule %s is settled both at %s and at %s level",
				ErrValidation, matched.RuleID, previous, matched.Settlement)
		}
		seen[matched.RuleID] = matched.Settlement

		switch matched.Settlement {
		case SettlementWork:
			workLevelTotal += matched.AmountMinor
		case SettlementPayroll:
			conservation.PayrollAccruedMinor += matched.AmountMinor
		default:
			return EconomicConservation{}, fmt.Errorf(
				"%w: rule %s has no settlement level", ErrValidation, matched.RuleID)
		}
	}

	if workLevelTotal != resolution.ProviderShareMinor {
		return EconomicConservation{}, fmt.Errorf(
			"%w: work-level components total %d but the provider share is %d",
			ErrValidation, workLevelTotal, resolution.ProviderShareMinor)
	}
	if !conservation.Balanced() {
		return EconomicConservation{}, fmt.Errorf(
			"%w: provider %d plus organization %d does not account for the eligible basis %d",
			ErrValidation, conservation.ProviderShareMinor,
			conservation.OrganizationShareMinor, conservation.EligibleBasisMinor)
	}
	return conservation, nil
}

// ServiceContext is one service a provider is compensated under.
const (
	ServiceContextTaxi     = "TAXI"
	ServiceContextDispatch = "DISPATCH"
	ServiceContextHauling  = "HAULING"
	ServiceContextCommerce = "COMMERCE"
)

// CanonicalServiceContext normalises a service name to the one this service
// stores.
//
// Identity must not split by spelling: a provider recorded under "taxi" in one
// place and "TAXI" in another would be paid a salary in each (CMP-033).
func CanonicalServiceContext(service string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(service)) {
	case ServiceContextTaxi, "TAXI_GOODS", "DRIVER-TAXI-SERVICE", "TAXI-SERVICE":
		return ServiceContextTaxi, nil
	case ServiceContextDispatch, "DRIVER-DISPATCH-DELIVERY-SERVICE", "DISPATCH-DELIVERY-SERVICE":
		return ServiceContextDispatch, nil
	case ServiceContextHauling, "DRIVER-HAULING-SERVICE", "HAULING-SERVICE":
		return ServiceContextHauling, nil
	case ServiceContextCommerce, "VENDOR", "FLEET":
		return ServiceContextCommerce, nil
	default:
		return "", fmt.Errorf("%w: service context %q is not recognised", ErrValidation, service)
	}
}

// PeriodComponent is one payroll component a provider earned in one period.
type PeriodComponent struct {
	// MembershipID is the canonical provider identity. Two services reporting
	// the same human must report the same membership, or the provider is two
	// people as far as payroll is concerned and is paid twice.
	MembershipID string

	ServiceContext string
	Component      string
	RuleID         string
	AmountMinor    int64
	Currency       string
}

// MultiServicePayroll is one provider's payroll across every service they
// worked in a period.
type MultiServicePayroll struct {
	MembershipID string            `json:"membershipId"`
	Currency     string            `json:"currency"`
	TotalMinor   int64             `json:"totalMinor"`
	ByService    map[string]int64  `json:"byService"`
	Components   []PeriodComponent `json:"components"`
}

// ResolveMultiServicePayroll combines one provider's components across
// services, refusing the collisions that would pay them twice (CMP-033).
//
// A provider driving a taxi on Monday and a truck on Thursday is one person
// with one salary. The rules that must hold, and each of which has a way of
// failing quietly:
//
//   - A period-level component -- a salary, a fixed allowance -- is paid once
//     for the provider, not once per service. Paying it per service is the
//     most expensive possible bug and looks entirely normal on each service's
//     own report.
//   - Per-unit components are genuinely per service and accumulate.
//   - Every component must belong to the same provider and the same currency;
//     combining across either is meaningless.
func ResolveMultiServicePayroll(membershipID, currency string, components []PeriodComponent) (MultiServicePayroll, error) {
	if strings.TrimSpace(membershipID) == "" {
		return MultiServicePayroll{}, fmt.Errorf("%w: payroll needs a canonical provider identity", ErrValidation)
	}
	payroll := MultiServicePayroll{
		MembershipID: membershipID, Currency: strings.ToUpper(strings.TrimSpace(currency)),
		ByService: map[string]int64{},
	}
	if payroll.Currency == "" {
		return MultiServicePayroll{}, fmt.Errorf("%w: payroll needs a currency", ErrValidation)
	}

	// A period-level component is identified by its rule. The same rule
	// appearing under two services is the same entitlement reported twice, not
	// two entitlements.
	periodLevelRule := map[string]string{}

	for _, component := range components {
		if component.MembershipID != membershipID {
			return MultiServicePayroll{}, fmt.Errorf(
				"%w: component for %s cannot be paid on %s's payroll",
				ErrValidation, component.MembershipID, membershipID)
		}
		if !strings.EqualFold(component.Currency, payroll.Currency) {
			return MultiServicePayroll{}, fmt.Errorf(
				"%w: component in %s cannot be combined with a %s payroll",
				ErrValidation, component.Currency, payroll.Currency)
		}
		service, err := CanonicalServiceContext(component.ServiceContext)
		if err != nil {
			return MultiServicePayroll{}, err
		}

		if isPeriodLevelComponent(component.Component) {
			if previous, duplicated := periodLevelRule[component.RuleID]; duplicated {
				return MultiServicePayroll{}, fmt.Errorf(
					"%w: %s rule %s is claimed by both %s and %s; a period component is paid once per provider",
					ErrValidation, component.Component, component.RuleID, previous, service)
			}
			periodLevelRule[component.RuleID] = service
		}

		payroll.TotalMinor += component.AmountMinor
		payroll.ByService[service] += component.AmountMinor
		normalized := component
		normalized.ServiceContext = service
		payroll.Components = append(payroll.Components, normalized)
	}

	sort.SliceStable(payroll.Components, func(i, j int) bool {
		if payroll.Components[i].ServiceContext != payroll.Components[j].ServiceContext {
			return payroll.Components[i].ServiceContext < payroll.Components[j].ServiceContext
		}
		return payroll.Components[i].RuleID < payroll.Components[j].RuleID
	})
	return payroll, nil
}

// isPeriodLevelComponent reports whether a component is earned once per period
// for the provider rather than once per service.
func isPeriodLevelComponent(component string) bool {
	switch strings.ToUpper(strings.TrimSpace(component)) {
	case RuleSalary, RuleAllowance:
		return true
	default:
		return false
	}
}
