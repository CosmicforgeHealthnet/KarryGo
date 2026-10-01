package providercompat

// Section 14 VIS-007 / VIS-014 / VIS-015: what a provider is shown about money.
//
// The registry in visibility.go decides whether a given field may be shown. It
// cannot decide what a response contains in the first place -- and a response
// assembled from whatever a service happened to have in hand, then filtered, is
// one forgotten field away from leaking. Every such leak looks like an ordinary
// successful response.
//
// So these are constructed views. A field reaches a provider because it was
// deliberately placed in one of them and the policy allows its class, not
// because it survived a filter.

import (
	"sort"
	"strings"
	"time"
)

// MoneyField is one monetary figure offered to a provider.
//
// Visible carries the decision explicitly rather than the field being absent,
// so a client can say "your organization does not share this" instead of
// rendering a blank where a number should be -- and so a test can tell a
// withheld field from a forgotten one.
type MoneyField struct {
	Label       string              `json:"label"`
	Class       FinancialFieldClass `json:"class"`
	AmountMinor int64               `json:"amountMinor,omitempty"`
	Currency    string              `json:"currency,omitempty"`
	Visible     bool                `json:"visible"`
}

// ProviderJobFinancialView is what a provider may see about one job (VIS-007).
//
// There is deliberately no map, no passthrough and no "extra" field. A provider
// sees these figures and nothing else, whatever the calling service happens to
// hold about the job.
type ProviderJobFinancialView struct {
	JobID         string `json:"jobId"`
	SourceService string `json:"sourceService"`
	Currency      string `json:"currency"`

	Fields []MoneyField `json:"fields"`

	// Explanation is the typed account of how the provider's own figure was
	// arrived at (VIS-015). Empty when policy does not permit it.
	Explanation []CompensationExplanationLine `json:"explanation,omitempty"`
}

// JobFinancialInputs is everything a service knows about one job's money.
//
// The organization and platform figures are accepted here precisely so the view
// builder can be the single place that refuses to emit them. A builder that
// never saw them could not be tested for withholding them.
type JobFinancialInputs struct {
	JobID         string
	SourceService string
	Currency      string

	CustomerFareMinor  int64
	OwnEarningMinor    int64
	ProviderShareMinor int64
	CommissionRateBps  int
	TipsMinor          int64
	AllowancesMinor    int64
	BonusesMinor       int64

	// Never shown to a provider under any policy. Present so the refusal is
	// enforced in code rather than by remembering not to pass them.
	OrganizationShareMinor int64
	PlatformShareMinor     int64
}

// BuildProviderJobFinancialView assembles one job's provider-facing figures.
func BuildProviderJobFinancialView(policy FinancialVisibilityPolicy, inputs JobFinancialInputs) ProviderJobFinancialView {
	view := ProviderJobFinancialView{
		JobID: inputs.JobID, SourceService: inputs.SourceService, Currency: inputs.Currency,
	}
	add := func(label string, class FinancialFieldClass, amount int64) {
		field := MoneyField{Label: label, Class: class, Visible: policy.Allows(class)}
		if field.Visible {
			field.AmountMinor, field.Currency = amount, inputs.Currency
		}
		view.Fields = append(view.Fields, field)
	}
	add("Customer fare", ClassCustomerFare, inputs.CustomerFareMinor)
	add("Your earning", ClassOwnEarning, inputs.OwnEarningMinor)
	add("Your share", ClassProviderShare, inputs.ProviderShareMinor)
	add("Tips", ClassTips, inputs.TipsMinor)
	add("Allowances", ClassAllowances, inputs.AllowancesMinor)
	add("Bonuses", ClassBonuses, inputs.BonusesMinor)

	// The commission rate is a rate, not an amount, so it is offered as a
	// figure in basis points rather than money.
	rateField := MoneyField{
		Label: "Commission rate (bps)", Class: ClassCommissionRate,
		Visible: policy.Allows(ClassCommissionRate),
	}
	if rateField.Visible {
		rateField.AmountMinor = int64(inputs.CommissionRateBps)
	}
	view.Fields = append(view.Fields, rateField)
	return view
}

// CompensationExplanationLine is one typed step in how a figure was reached
// (VIS-015).
//
// Typed rather than free text or an internal calculation blob, because a blob
// is whatever the calculation happened to contain -- which on a corporate job
// includes the organization's share and the plan that produced it.
type CompensationExplanationLine struct {
	Component     string              `json:"component"`
	Class         FinancialFieldClass `json:"class"`
	Basis         string              `json:"basis,omitempty"`
	BasisQuantity int64               `json:"basisQuantity,omitempty"`
	BasisUnit     string              `json:"basisUnit,omitempty"`
	RateBps       int                 `json:"rateBps,omitempty"`
	AmountMinor   int64               `json:"amountMinor"`
	Currency      string              `json:"currency"`
	Reason        string              `json:"reason,omitempty"`
	PeriodStart   *time.Time          `json:"periodStart,omitempty"`
	PeriodEnd     *time.Time          `json:"periodEnd,omitempty"`
}

// ExplanationInput is one component a provider might be told about.
type ExplanationInput struct {
	Component     string
	Class         FinancialFieldClass
	Basis         string
	BasisQuantity int64
	BasisUnit     string
	RateBps       int
	AmountMinor   int64
	Currency      string
	Reason        string
	PeriodStart   *time.Time
	PeriodEnd     *time.Time
}

// BuildCompensationExplanation returns only the lines a provider may see.
//
// A component whose class the policy withholds is omitted entirely rather than
// shown with a blank amount: "you earned something, we won't say what" is worse
// than silence, and a zero would be a lie.
func BuildCompensationExplanation(policy FinancialVisibilityPolicy, inputs []ExplanationInput) []CompensationExplanationLine {
	var lines []CompensationExplanationLine
	for _, input := range inputs {
		if input.Class.AlwaysConcealed() || !policy.Allows(input.Class) {
			continue
		}
		lines = append(lines, CompensationExplanationLine{
			Component: input.Component, Class: input.Class,
			Basis: input.Basis, BasisQuantity: input.BasisQuantity, BasisUnit: input.BasisUnit,
			RateBps: input.RateBps, AmountMinor: input.AmountMinor, Currency: input.Currency,
			Reason: input.Reason, PeriodStart: input.PeriodStart, PeriodEnd: input.PeriodEnd,
		})
	}
	sort.SliceStable(lines, func(i, j int) bool { return lines[i].Component < lines[j].Component })
	return lines
}

// ProviderPayrollView is a provider's own payroll for one period (VIS-014).
//
// Scoped to one provider by construction: there is no field in which another
// provider's figures, or an organization-wide total, could travel.
type ProviderPayrollView struct {
	MembershipID string    `json:"membershipId"`
	PeriodStart  time.Time `json:"periodStart"`
	PeriodEnd    time.Time `json:"periodEnd"`
	Currency     string    `json:"currency"`

	// NetMinor is what this provider is paid for the period. Present only
	// when policy permits payroll at all.
	NetMinor int64 `json:"netMinor"`
	Visible  bool  `json:"visible"`

	Lines []CompensationExplanationLine `json:"lines,omitempty"`
}

// PayrollViewInput is one provider's payroll, as the owning service holds it.
type PayrollViewInput struct {
	MembershipID string
	PeriodStart  time.Time
	PeriodEnd    time.Time
	Currency     string
	NetMinor     int64
	Components   []ExplanationInput
}

// BuildProviderPayrollView assembles a provider's own payroll view.
//
// requesterMembershipID is the provider asking. A mismatch returns an empty,
// invisible view rather than the payroll: authorization is enforced here as
// well as at the route, because a read model that only refuses when someone
// remembers to check it will eventually be read by something that forgot.
func BuildProviderPayrollView(
	policy FinancialVisibilityPolicy, requesterMembershipID string, input PayrollViewInput,
) ProviderPayrollView {
	view := ProviderPayrollView{
		MembershipID: input.MembershipID, PeriodStart: input.PeriodStart,
		PeriodEnd: input.PeriodEnd, Currency: input.Currency,
	}
	if strings.TrimSpace(requesterMembershipID) == "" ||
		requesterMembershipID != input.MembershipID ||
		!policy.Allows(ClassPayroll) {
		view.MembershipID = ""
		return view
	}
	view.Visible = true
	view.NetMinor = input.NetMinor
	view.Lines = BuildCompensationExplanation(policy, input.Components)
	return view
}
