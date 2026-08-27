package taxonomy

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

// RestrictedGoodsPolicyVersion identifies the server-owned Vendor Commerce
// projection of the platform taxonomy policy. Decisions persist this version
// in evidence where the calling domain supports it.
const RestrictedGoodsPolicyVersion = "vendor_commerce_restricted_goods_v1"

type GoodsClassification string

const (
	GoodsAllowed                 GoodsClassification = "ALLOWED"
	GoodsRestricted              GoodsClassification = "RESTRICTED"
	GoodsProhibited              GoodsClassification = "PROHIBITED"
	GoodsRegulatedExternalDomain GoodsClassification = "REGULATED_EXTERNAL_DOMAIN"
)

const (
	RestrictedGoodsSourcePlatformTaxonomy = "PLATFORM_TAXONOMY"
	ReasonMedicineExternalDomain          = "PHARMACY_MEDICINE_EXCLUDED"
	ReasonProhibitedGoods                 = "PROHIBITED_GOODS"
	ReasonRestrictedGoodsReview           = "RESTRICTED_GOODS_REVIEW_REQUIRED"
)

// RestrictedGoodsDecision is deliberately typed and server-owned. Vendor and
// customer request fields are facts to evaluate, never policy overrides.
type RestrictedGoodsDecision struct {
	Classification GoodsClassification `json:"classification"`
	Allowed        bool                `json:"allowed"`
	ReasonCode     string              `json:"reason_code,omitempty"`
	PolicyVersion  string              `json:"policy_version"`
	Source         string              `json:"source"`
}

type RestrictedGoodsFacts struct {
	CategoryCode        string
	ProductClass        string
	Name                string
	Description         string
	Brand               string
	Manufacturer        string
	RegulatoryReference string
	ManufacturerCode    string
}

type restrictedRule struct {
	classification GoodsClassification
	reason         string
	terms          []string
}

// These rules are the minimum fail-closed Vendor-Commerce projection. Global
// taxonomy governance remains platform/admin-owned; Vendor actors cannot edit
// this set through catalogue or order payloads.
var restrictedGoodsRules = []restrictedRule{
	{
		classification: GoodsRegulatedExternalDomain,
		reason:         ReasonMedicineExternalDomain,
		terms: []string{
			"prescription medicine", "prescription drug", "otc medicine",
			"over the counter medicine", "pharmaceutical product",
			"pharmaceutical", "medicinal product", "regulated medicinal",
			"pharmacy product", "pharmacy", "medicine", "medication",
		},
	},
	{
		classification: GoodsProhibited,
		reason:         ReasonProhibitedGoods,
		terms: []string{
			"illegal drug", "controlled substance", "firearm", "ammunition",
			"explosive device", "human trafficking", "stolen goods",
		},
	},
	{
		classification: GoodsRestricted,
		reason:         ReasonRestrictedGoodsReview,
		terms: []string{
			"tobacco product", "alcoholic beverage", "hazardous chemical",
			"live animal", "weapon",
		},
	},
}

var nonAlphaNumeric = regexp.MustCompile(`[^a-z0-9]+`)

func normalizedPolicyText(value string) string {
	return strings.TrimSpace(nonAlphaNumeric.ReplaceAllString(strings.ToLower(value), " "))
}

func EvaluateRestrictedGoods(facts RestrictedGoodsFacts) RestrictedGoodsDecision {
	values := []string{
		facts.CategoryCode, facts.ProductClass, facts.Name, facts.Description,
		facts.Brand, facts.Manufacturer, facts.RegulatoryReference,
		facts.ManufacturerCode,
	}
	return evaluateRestrictedStrings(values)
}

// EvaluateRestrictedGoodsPayload walks a bounded structured request snapshot.
// It exists for RFQ requirements, whose historical contract stores JSON maps.
// It is a safety gate, not a replacement for structured catalogue taxonomy.
func EvaluateRestrictedGoodsPayload(payload any) RestrictedGoodsDecision {
	values := make([]string, 0, 16)
	collectPolicyStrings(reflect.ValueOf(payload), 0, &values)
	sort.Strings(values)
	return evaluateRestrictedStrings(values)
}

func collectPolicyStrings(value reflect.Value, depth int, out *[]string) {
	if depth > 8 || !value.IsValid() || len(*out) >= 256 {
		return
	}
	if value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return
		}
		collectPolicyStrings(value.Elem(), depth+1, out)
		return
	}
	switch value.Kind() {
	case reflect.String:
		*out = append(*out, value.String())
	case reflect.Map:
		iter := value.MapRange()
		for iter.Next() {
			collectPolicyStrings(iter.Key(), depth+1, out)
			collectPolicyStrings(iter.Value(), depth+1, out)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < value.Len(); i++ {
			collectPolicyStrings(value.Index(i), depth+1, out)
		}
	}
}

func evaluateRestrictedStrings(values []string) RestrictedGoodsDecision {
	decision := RestrictedGoodsDecision{
		Classification: GoodsAllowed,
		Allowed:        true,
		PolicyVersion:  RestrictedGoodsPolicyVersion,
		Source:         RestrictedGoodsSourcePlatformTaxonomy,
	}
	for _, value := range values {
		normalized := normalizedPolicyText(value)
		if normalized == "" {
			continue
		}
		padded := " " + normalized + " "
		for _, rule := range restrictedGoodsRules {
			for _, term := range rule.terms {
				needle := " " + normalizedPolicyText(term) + " "
				if strings.Contains(padded, needle) {
					return RestrictedGoodsDecision{
						Classification: rule.classification,
						Allowed:        false,
						ReasonCode:     rule.reason,
						PolicyVersion:  RestrictedGoodsPolicyVersion,
						Source:         RestrictedGoodsSourcePlatformTaxonomy,
					}
				}
			}
		}
	}
	return decision
}

func (d RestrictedGoodsDecision) Error() error {
	if d.Allowed {
		return nil
	}
	return fmt.Errorf("restricted goods policy %s: %s", d.PolicyVersion, d.ReasonCode)
}
