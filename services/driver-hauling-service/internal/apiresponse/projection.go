// Package apiresponse enforces corporate financial privacy on Hauling's
// provider-facing responses, server-side (HLG-021 .. HLG-024).
//
// The rule this package exists to make structural: a corporate driver's app
// is never sent a rich finance object it is merely expected to hide. What the
// organization's policy does not authorize is not serialized at all.
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
	KeyProviderID = "hauling_provider_id"
	// KeyManaged records whether Fleet reported this provider as
	// organization-managed.
	KeyManaged = "hauling_provider_managed"
	// KeyPolicy carries the typed visibility policy.
	KeyPolicy = "hauling_financial_visibility_policy"
)

// ProjectResponse applies the typed policy to an arbitrary provider-facing
// response body.
//
// Typed classification is the PRIMARY protection: every field the shared
// registry recognises is decided by policy. The name heuristic below is
// defence-in-depth only -- it catches money-looking fields the registry does
// not yet classify, and it runs ONLY for a managed provider, so an
// independent provider can never lose a legitimate value to a spelling
// coincidence (HLG-024).
func ProjectResponse(data any, managed bool, policy *providercompat.FinancialVisibilityPolicy) any {
	raw, err := json.Marshal(data)
	if err != nil {
		return nil
	}
	return projectRaw(raw, managed, policy)
}

// ProjectJSON applies the policy to an already-encoded JSON body, returning
// the re-encoded body. It returns the input unchanged when the body is not
// JSON, so a non-JSON response (a stream, an empty body) passes through.
func ProjectJSON(raw []byte, managed bool, policy *providercompat.FinancialVisibilityPolicy) []byte {
	if len(bytes.TrimSpace(raw)) == 0 {
		return raw
	}
	projected := projectRaw(raw, managed, policy)
	if projected == nil {
		return raw
	}
	encoded, err := json.Marshal(projected)
	if err != nil {
		return raw
	}
	return encoded
}

func projectRaw(raw []byte, managed bool, policy *providercompat.FinancialVisibilityPolicy) any {
	var view any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	// UseNumber keeps an untouched amount byte-identical: decoding money
	// through float64 and re-encoding would silently reformat values this
	// projection never intended to change.
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

// monetaryHints back the defence-in-depth sweep. They exist to fail closed on
// an unclassified field, never to be the primary rule.
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
