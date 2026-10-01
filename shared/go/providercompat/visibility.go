package providercompat

import "strings"

// FinancialFieldClass is the typed classification of one monetary field in a
// provider-facing response.
//
// This registry is the PRIMARY protection for corporate financial privacy.
// Name-substring heuristics remain in the provider services as
// defence-in-depth for fields that are not yet classified here, but a
// classified field's visibility is decided by policy, never by its spelling.
type FinancialFieldClass string

const (
	// ClassCustomerFare is what the customer paid for the job.
	ClassCustomerFare FinancialFieldClass = "CUSTOMER_FARE"
	// ClassOwnEarning is the provider's own earning for the job.
	ClassOwnEarning FinancialFieldClass = "OWN_EARNING"
	// ClassCommissionRate is the commission rate applied to the provider.
	ClassCommissionRate FinancialFieldClass = "COMMISSION_RATE"
	// ClassProviderShare is the provider's resolved share of the basis.
	ClassProviderShare FinancialFieldClass = "PROVIDER_SHARE"
	// ClassTips is customer-given gratuity.
	ClassTips FinancialFieldClass = "TIPS"
	// ClassAllowances is an organization allowance line.
	ClassAllowances FinancialFieldClass = "ALLOWANCES"
	// ClassBonuses is an organization bonus line.
	ClassBonuses FinancialFieldClass = "BONUSES"
	// ClassPayroll is payroll-cadence compensation context.
	ClassPayroll FinancialFieldClass = "PAYROLL"
	// ClassOrganizationOnly is organization-internal commercial data
	// (organization gross, organization margin, organization share). It is
	// NEVER shown to a provider under any policy in Sections 8/9.
	ClassOrganizationOnly FinancialFieldClass = "ORGANIZATION_ONLY"
	// ClassPlatformOnly is platform-internal commercial data (platform
	// margin, platform share, internal settlement splits). NEVER shown.
	ClassPlatformOnly FinancialFieldClass = "PLATFORM_ONLY"
)

// AlwaysConcealed reports whether a class is withheld from providers
// regardless of the organization's visibility policy. Section 8/9 forbids a
// policy from authorizing organization or platform margin.
func (c FinancialFieldClass) AlwaysConcealed() bool {
	return c == ClassOrganizationOnly || c == ClassPlatformOnly
}

// Allows reports whether the policy authorizes this class for the provider.
//
// The default is deny: an unrecognised class is concealed, and the
// organization/platform classes are concealed even when a policy flag would
// otherwise appear to permit them.
func (p FinancialVisibilityPolicy) Allows(class FinancialFieldClass) bool {
	if class.AlwaysConcealed() {
		return false
	}
	switch class {
	case ClassCustomerFare:
		return p.ShowCustomerFare
	case ClassOwnEarning:
		return p.ShowOwnEarning
	case ClassCommissionRate:
		return p.ShowCommissionRate
	case ClassProviderShare:
		return p.ShowProviderShare
	case ClassTips:
		return p.ShowTips
	case ClassAllowances:
		return p.ShowAllowances
	case ClassBonuses:
		return p.ShowBonuses
	case ClassPayroll:
		return p.ShowPayroll
	default:
		return false
	}
}

// IndependentVisibilityPolicy is the policy applied to an independent
// provider: everything they legitimately already saw before corporate privacy
// existed stays visible. The organization/platform classes are still withheld
// because they simply do not exist for an independent provider.
//
// This exists so an independent provider never loses information to a rule
// that was written for corporate privacy (Section 8 TAX-019B / DSP-013).
func IndependentVisibilityPolicy() FinancialVisibilityPolicy {
	return FinancialVisibilityPolicy{
		ShowCustomerFare: true, ShowOwnEarning: true, ShowCommissionRate: true,
		ShowProviderShare: true, ShowPayroll: true, ShowTips: true,
		ShowAllowances: true, ShowBonuses: true,
	}
}

// ConservativeVisibilityPolicy is the fail-closed policy: only payroll
// context, which is the provider's own employment compensation. Used whenever
// a managed provider's real policy could not be established.
func ConservativeVisibilityPolicy() FinancialVisibilityPolicy {
	return FinancialVisibilityPolicy{ShowPayroll: true}
}

// classifiedFinancialFields is the canonical typed registry, keyed by the
// normalized field name (lower-cased, separators removed). Entries are added
// for the fields the provider surfaces actually emit.
var classifiedFinancialFields = map[string]FinancialFieldClass{
	// Customer-paid fare.
	"customerfare":      ClassCustomerFare,
	"customerfareminor": ClassCustomerFare,
	"customerfarekobo":  ClassCustomerFare,
	"fareamount":        ClassCustomerFare,
	"fareamountkobo":    ClassCustomerFare,
	"farekobo":          ClassCustomerFare,
	"fareestimatekobo":  ClassCustomerFare,
	"farefinalkobo":     ClassCustomerFare,
	"fare":              ClassCustomerFare,
	"price":             ClassCustomerFare,
	"pricekobo":         ClassCustomerFare,
	"totalprice":        ClassCustomerFare,
	"quotedfarekobo":    ClassCustomerFare,
	"estimatedfarekobo": ClassCustomerFare,
	"declaredvaluekobo": ClassCustomerFare,
	"deliveryfeekobo":   ClassCustomerFare,

	// Provider's own earning.
	"ownearning":          ClassOwnEarning,
	"ownearningminor":     ClassOwnEarning,
	"ownearningkobo":      ClassOwnEarning,
	"earning":             ClassOwnEarning,
	"earnings":            ClassOwnEarning,
	"earningskobo":        ClassOwnEarning,
	"netearningkobo":      ClassOwnEarning,
	"providerearningkobo": ClassOwnEarning,
	"payoutkobo":          ClassOwnEarning,

	// Commission.
	"commission":         ClassCommissionRate,
	"commissionrate":     ClassCommissionRate,
	"commissionratebps":  ClassCommissionRate,
	"commissionkobo":     ClassCommissionRate,
	"commissionminor":    ClassCommissionRate,
	"platformcommission": ClassCommissionRate,

	// Resolved provider share.
	"providershare":      ClassProviderShare,
	"providershareminor": ClassProviderShare,
	"providersharekobo":  ClassProviderShare,
	"payable":            ClassProviderShare,
	"payablekobo":        ClassProviderShare,
	"payableminor":       ClassProviderShare,

	// Gratuity / allowances / bonuses / payroll.
	"tips":              ClassTips,
	"tipskobo":          ClassTips,
	"tipsminor":         ClassTips,
	"tipamountkobo":     ClassTips,
	"allowance":         ClassAllowances,
	"allowances":        ClassAllowances,
	"allowancekobo":     ClassAllowances,
	"additionalfee":     ClassAllowances,
	"additionalfeekobo": ClassAllowances,
	"bonus":             ClassBonuses,
	"bonuses":           ClassBonuses,
	"bonuskobo":         ClassBonuses,
	"payroll":           ClassPayroll,
	"salaryminor":       ClassPayroll,
	"salarykobo":        ClassPayroll,

	// Organization-internal: never provider-visible.
	"organizationshare":      ClassOrganizationOnly,
	"organizationshareminor": ClassOrganizationOnly,
	"organizationgross":      ClassOrganizationOnly,
	"organizationgrossminor": ClassOrganizationOnly,
	"organizationmargin":     ClassOrganizationOnly,
	"organizationrevenue":    ClassOrganizationOnly,
	"orgshareminor":          ClassOrganizationOnly,
	"grossbasisminor":        ClassOrganizationOnly,
	"eligiblebasisminor":     ClassOrganizationOnly,

	// Platform-internal: never provider-visible.
	"platformshare":        ClassPlatformOnly,
	"platformshareminor":   ClassPlatformOnly,
	"platformmargin":       ClassPlatformOnly,
	"platformrevenue":      ClassPlatformOnly,
	"settlementsplit":      ClassPlatformOnly,
	"settlementallocation": ClassPlatformOnly,
	"internalsplit":        ClassPlatformOnly,
}

// NormalizeFieldName lowercases a field name and removes the separators the
// platform mixes across snake_case, camelCase and kebab-case surfaces, so one
// registry entry covers every spelling of the same field.
func NormalizeFieldName(name string) string {
	return strings.ToLower(strings.NewReplacer("_", "", "-", "", " ", "").Replace(name))
}

// moneyUnitSuffixes are the interchangeable minor-unit suffixes the platform
// uses for the same quantity. Stripping them means one registry entry covers
// every spelling (platformMargin / platform_margin_minor / platformMarginKobo)
// instead of needing a row per variant -- a missing variant would otherwise
// silently fall through to unclassified.
var moneyUnitSuffixes = []string{"minor", "kobo", "amount", "total", "value"}

// ClassifyFinancialField returns the typed class of a response field, and
// whether it is classified at all. An unclassified field is not automatically
// safe: callers apply their own default-deny heuristic to it.
func ClassifyFinancialField(name string) (FinancialFieldClass, bool) {
	normalized := NormalizeFieldName(name)
	if class, ok := classifiedFinancialFields[normalized]; ok {
		return class, true
	}
	// Try progressively stripping money-unit suffixes, so a compound such as
	// "platformmarginminorkobo" still resolves to its base class.
	for changed := true; changed; {
		changed = false
		for _, suffix := range moneyUnitSuffixes {
			if len(normalized) > len(suffix) && strings.HasSuffix(normalized, suffix) {
				normalized = strings.TrimSuffix(normalized, suffix)
				changed = true
				if class, ok := classifiedFinancialFields[normalized]; ok {
					return class, true
				}
			}
		}
	}
	return "", false
}

// FieldVisible is the single decision function both provider services call.
//
// managed=false means an independent provider: the independent policy applies,
// so no legitimate pre-existing financial information is lost. managed=true
// applies the organization's typed policy, with organization/platform classes
// always concealed.
func FieldVisible(name string, managed bool, policy *FinancialVisibilityPolicy) (visible bool, classified bool) {
	class, ok := ClassifyFinancialField(name)
	if !ok {
		return false, false
	}
	if !managed {
		return IndependentVisibilityPolicy().Allows(class), true
	}
	effective := ConservativeVisibilityPolicy()
	if policy != nil {
		effective = *policy
	}
	return effective.Allows(class), true
}
