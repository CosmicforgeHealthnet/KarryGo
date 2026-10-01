package apiresponse

import (
	"bytes"
	"encoding/json"
	"strings"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/providercompat"
	"github.com/gin-gonic/gin"
)

// ProviderPrivacy installs a policy hook consumed after authentication, before
// domain mutation. Fleet outages cannot result in an unmasked success response.
func ProviderPrivacy(reader providercompat.Reader) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("dispatch_provider_privacy_check", func(c *gin.Context) error {
			if reader == nil {
				return nil
			} // organization rollout disabled
			corporate, err := resolveCorporateContext(c, reader)
			if err != nil {
				return err
			}
			if corporate.IsManagedProvider {
				c.Set("dispatch_financial_visibility_policy", corporate.FinancialVisibilityPolicy)
				c.Set("dispatch_conceal_finance", true)
			}
			return nil
		})
		c.Next()
	}
}

// resolveCorporateContext produces ONE canonical corporate context for the
// provider, whichever Fleet surface is available (Section 9 DSP-002).
//
// The canonical Section 7 corporate-context endpoint is authoritative. The
// legacy membership-summary read remains only as a compatibility fallback for
// a Fleet deployment that predates that endpoint, and it is NORMALIZED onto
// the same canonical model rather than interpreted separately -- so there is
// never a second, conflicting idea of what "organization-managed" means.
//
// The fallback deliberately carries no visibility policy: the summary
// contract has no typed policy to carry, and the projection treats a managed
// provider with no policy as conservative. Inventing a permissive default
// there would make the compatibility path leakier than the canonical one.
func resolveCorporateContext(c *gin.Context, reader providercompat.Reader) (providercompat.CorporateContext, error) {
	riderID := c.GetString("dispatch_rider_id")
	if canonical, ok := reader.(providercompat.CorporateContextReader); ok {
		corporate, err := canonical.ReadCorporateContext(c.Request.Context(), serviceDispatch, riderID)
		if err != nil {
			return providercompat.CorporateContext{},
				apperrors.Unavailable("Provider visibility authority is unavailable.", nil)
		}
		return corporate, nil
	}
	summary, err := reader.ReadMembershipSummary(c.Request.Context(), "driver-dispatch-delivery-service", riderID)
	if err != nil {
		return providercompat.CorporateContext{},
			apperrors.Unavailable("Provider visibility authority is unavailable.", nil)
	}
	switch summary.ProviderMode {
	case providercompat.ModeIndependent:
		return providercompat.CorporateContext{ProviderID: riderID}, nil
	case providercompat.ModeOrganizationManaged:
		out := providercompat.CorporateContext{
			ProviderID: riderID, IsManagedProvider: true,
			FinancialVisibilityPolicy: providercompat.ConservativeVisibilityPolicy(),
			ServiceEligibility:        map[string]bool{serviceDispatch: true},
		}
		if summary.Organization != nil {
			out.OrganizationID = summary.Organization.ID
		}
		if summary.Membership != nil {
			out.MembershipStatus = summary.Membership.Status
			out.OrganizationRole = summary.Membership.Role
		}
		if summary.AssignmentPolicy != nil {
			out.OperatingMode = summary.AssignmentPolicy.AssignmentMode
		}
		return out, nil
	default:
		return providercompat.CorporateContext{},
			apperrors.Unavailable("Provider visibility authority is unavailable.", nil)
	}
}

// serviceDispatch is Dispatch's Fleet service category.
const serviceDispatch = "DISPATCH"

func providerView(data any, conceal bool) any {
	return providerViewWithPolicy(data, conceal, nil)
}

func providerViewWithPolicy(data any, conceal bool, policy *providercompat.FinancialVisibilityPolicy) any {
	raw, err := json.Marshal(data)
	if err != nil {
		return nil
	}
	var view any
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if d.Decode(&view) != nil {
		return nil
	}
	minimizeProviderView(view, conceal, false, policy)
	return view
}

func minimizeProviderView(value any, conceal, terminal bool, policy *providercompat.FinancialVisibilityPolicy) {
	switch v := value.(type) {
	case []any:
		for _, item := range v {
			minimizeProviderView(item, conceal, terminal, policy)
		}
	case map[string]any:
		// Standalone proof responses are evidence, not a contact directory.
		_, isProof := v["submitted_at"]
		_, hasProofRef := v["photo_ref"]
		if isProof && hasProofRef {
			terminal = true
		}
		if status, ok := v["status"].(string); ok {
			switch strings.ToLower(status) {
			case "completed", "cancelled", "failed", "returned":
				terminal = true
			}
		}
		// TripDetailResponse wraps proof next to its Trip, not inside it.
		if trip, ok := v["trip"].(map[string]any); ok {
			if status, ok := trip["status"].(string); ok {
				switch status {
				case "completed", "cancelled", "failed", "returned":
					terminal = true
				}
			}
		}
		for key, item := range v {
			k := providercompat.NormalizeFieldName(key)
			// PRIMARY protection (Section 9 DSP-011..013): the shared typed
			// registry decides, by policy, whether this field is visible.
			// Organization and platform classes are concealed under every
			// policy, including a fully permissive one.
			if visible, classified := providercompat.FieldVisible(key, conceal, policy); classified {
				if !visible {
					v[key] = nil // not zero money, and never changes persisted amounts
					continue
				}
				minimizeProviderView(item, conceal, terminal, policy)
				continue
			}
			// DEFENCE IN DEPTH only: a money-looking field the registry does
			// not yet classify still fails closed for a managed provider. It
			// is deliberately no longer the primary rule, and it never
			// applies to an independent provider -- conceal is false there,
			// so no legitimate value is lost to a spelling coincidence.
			if conceal && looksMonetary(k) {
				v[key] = nil
				continue
			}
			if terminal && (k == "receiverphone" || k == "receiveremail" || k == "receivername") {
				v[key] = ""
				continue
			}
			minimizeProviderView(item, conceal, terminal, policy)
		}
	}
}

// monetaryHints back the defence-in-depth sweep above. They exist to fail
// closed on an unclassified field, never to be the primary rule.
var monetaryHints = []string{
	"fare", "price", "commission", "payable", "margin", "settlement",
	"earning", "allocation", "payout", "gross", "revenue", "tip",
}

func looksMonetary(normalizedKey string) bool {
	for _, hint := range monetaryHints {
		if strings.Contains(normalizedKey, hint) {
			return true
		}
	}
	return false
}
