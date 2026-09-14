// Package apiresponse enforces corporate financial privacy on Taxi's
// provider-facing responses, server-side (Section 8 TAX-018, TAX-019).
//
// The rule this package exists to make structural: a corporate provider's
// app is never sent a rich finance object that it is merely expected to hide.
// What the policy does not authorize is not serialized at all.
package apiresponse

import (
	"bytes"
	"encoding/json"
	"strings"

	"cosmicforge/logistics/shared/go/providercompat"
)

// Context keys set by the provider-privacy middleware.
const (
	// KeyProviderID marks a response as provider-facing.
	KeyProviderID = "taxi_provider_id"
	// KeyManaged records whether Fleet reported this provider as
	// organization-managed.
	KeyManaged = "taxi_provider_managed"
	// KeyPolicy carries the typed visibility policy.
	KeyPolicy = "taxi_financial_visibility_policy"
)

// ProviderFinancialProjection is the typed, server-side projection of what a
// provider may be told about the money on a job (Section 8 TAX-019A).
//
// Every field is a pointer: absent means "not authorized", which is
// materially different from zero. Organization margin, platform margin,
// organization gross and internal settlement splits have no field here at
// all -- they are not omitted by policy, they are structurally absent, so no
// future policy change can accidentally expose them.
type ProviderFinancialProjection struct {
	CustomerFareMinor  *int64 `json:"customerFareMinor,omitempty"`
	OwnEarningMinor    *int64 `json:"ownEarningMinor,omitempty"`
	CommissionRateBps  *int   `json:"commissionRateBps,omitempty"`
	ProviderShareMinor *int64 `json:"providerShareMinor,omitempty"`
	TipsMinor          *int64 `json:"tipsMinor,omitempty"`
	Currency           string `json:"currency,omitempty"`
	// Mode is INDEPENDENT or ORGANIZATION_MANAGED, so a client can render the
	// correct earnings surface without inferring it from which fields are
	// missing (Section 8 TAX-019B).
	Mode string `json:"mode"`
}

// FinancialInputs are the raw values a caller holds before projection. They
// are never serialized directly.
type FinancialInputs struct {
	CustomerFareMinor  int64
	OwnEarningMinor    int64
	CommissionRateBps  int
	ProviderShareMinor int64
	TipsMinor          int64
	Currency           string
}

// ProjectFinancials builds the typed projection for one provider.
//
// An independent provider gets the independent policy, so nothing they could
// legitimately see before corporate privacy existed is lost (TAX-019B). A
// managed provider gets their organization's typed policy, defaulting to the
// conservative policy when none is available.
func ProjectFinancials(in FinancialInputs, managed bool, policy *providercompat.FinancialVisibilityPolicy) ProviderFinancialProjection {
	effective := providercompat.IndependentVisibilityPolicy()
	mode := providercompat.ModeIndependent
	if managed {
		mode = providercompat.ModeOrganizationManaged
		effective = providercompat.ConservativeVisibilityPolicy()
		if policy != nil {
			effective = *policy
		}
	}
	out := ProviderFinancialProjection{Currency: in.Currency, Mode: mode}
	if effective.Allows(providercompat.ClassCustomerFare) {
		value := in.CustomerFareMinor
		out.CustomerFareMinor = &value
	}
	if effective.Allows(providercompat.ClassOwnEarning) {
		value := in.OwnEarningMinor
		out.OwnEarningMinor = &value
	}
	if effective.Allows(providercompat.ClassCommissionRate) {
		value := in.CommissionRateBps
		out.CommissionRateBps = &value
	}
	if effective.Allows(providercompat.ClassProviderShare) {
		value := in.ProviderShareMinor
		out.ProviderShareMinor = &value
	}
	if effective.Allows(providercompat.ClassTips) {
		value := in.TipsMinor
		out.TipsMinor = &value
	}
	return out
}

// ProjectResponse applies the typed policy to an arbitrary provider-facing
// response body.
//
// Typed classification is the PRIMARY protection: every field the shared
// registry recognises is decided by policy. The name heuristic below is
// defence-in-depth only -- it catches money-looking fields the registry does
// not yet classify, and it runs ONLY for a managed provider, so an
// independent provider can never lose a legitimate value to a spelling
// coincidence.
func ProjectResponse(data any, managed bool, policy *providercompat.FinancialVisibilityPolicy) any {
	raw, err := json.Marshal(data)
	if err != nil {
		return nil
	}
	var view any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&view) != nil {
		return nil
	}
	project(view, managed, policy)
	return view
}

func project(value any, managed bool, policy *providercompat.FinancialVisibilityPolicy) {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			project(item, managed, policy)
		}
	case map[string]any:
		for key, item := range typed {
			visible, classified := providercompat.FieldVisible(key, managed, policy)
			if classified {
				if !visible {
					// Null, not zero: a concealed amount must never be
					// mistaken for an amount of zero.
					typed[key] = nil
				}
				continue
			}
			if managed && looksMonetary(key) {
				typed[key] = nil
				continue
			}
			project(item, managed, policy)
		}
	}
}

// monetaryHints are the defence-in-depth substrings. They exist to fail
// closed on an unclassified field, never to be the primary rule.
var monetaryHints = []string{
	"fare", "price", "commission", "payable", "margin", "settlement",
	"earning", "allocation", "payout", "gross", "revenue", "tip",
}

func looksMonetary(key string) bool {
	normalized := providercompat.NormalizeFieldName(key)
	for _, hint := range monetaryHints {
		if strings.Contains(normalized, hint) {
			return true
		}
	}
	return false
}
