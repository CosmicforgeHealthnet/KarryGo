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
			if corporate, ok := reader.(providercompat.CorporateContextReader); ok {
				context, err := corporate.ReadCorporateContext(c.Request.Context(), "DISPATCH", c.GetString("dispatch_rider_id"))
				if err != nil {
					return apperrors.Unavailable("Provider visibility authority is unavailable.", nil)
				}
				if context.IsManagedProvider {
					c.Set("dispatch_financial_visibility_policy", context.FinancialVisibilityPolicy)
					c.Set("dispatch_conceal_finance", true)
				}
				return nil
			}
			summary, err := reader.ReadMembershipSummary(c.Request.Context(), "driver-dispatch-delivery-service", c.GetString("dispatch_rider_id"))
			if err != nil {
				return apperrors.Unavailable("Provider visibility authority is unavailable.", nil)
			}
			switch summary.ProviderMode {
			case providercompat.ModeIndependent:
			case providercompat.ModeOrganizationManaged:
				c.Set("dispatch_conceal_finance", true)
			default:
				return apperrors.Unavailable("Provider visibility authority is unavailable.", nil)
			}
			return nil
		})
		c.Next()
	}
}

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
			k := strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(key))
			if conceal && !financialFieldAllowed(k, policy) && (strings.Contains(k, "fare") || strings.Contains(k, "price") || strings.Contains(k, "commission") || strings.Contains(k, "payable") || strings.Contains(k, "margin") || strings.Contains(k, "settlement") || strings.Contains(k, "earning") || strings.Contains(k, "allocation") || k == "additionalfeekobo") {
				v[key] = nil // not zero money, and never changes persisted amounts
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

func financialFieldAllowed(key string, policy *providercompat.FinancialVisibilityPolicy) bool {
	if policy == nil {
		return false
	}
	switch {
	case strings.Contains(key, "customerfare"), strings.Contains(key, "fareamount"), strings.Contains(key, "price"):
		return policy.ShowCustomerFare
	case strings.Contains(key, "commission"):
		return policy.ShowCommissionRate
	case strings.Contains(key, "providershare"), strings.Contains(key, "payable"):
		return policy.ShowProviderShare
	case strings.Contains(key, "earning"):
		return policy.ShowOwnEarning
	case strings.Contains(key, "allowance"), key == "additionalfeekobo":
		return policy.ShowAllowances
	case strings.Contains(key, "bonus"):
		return policy.ShowBonuses
	case strings.Contains(key, "margin"), strings.Contains(key, "settlement"), strings.Contains(key, "allocation"):
		return false
	default:
		return false
	}
}
